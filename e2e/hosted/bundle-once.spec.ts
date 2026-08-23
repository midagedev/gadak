import { createHash } from 'node:crypto'
import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { test, expect, type ConsoleMessage, type Page } from '@playwright/test'
import { dismissHostedFirstFrame } from './helpers'

/**
 * GDK-673 — one Vite bundle at /demo/ and /backlog/. The gate is the
 * artifact (same hashed bytes at both paths), not the build log.
 */

const root = join(dirname(fileURLToPath(import.meta.url)), '../..')
const hosted = join(root, 'dist/hosted')

function sha256(file: string): string {
  return createHash('sha256').update(readFileSync(file)).digest('hex')
}

function hashedAssets(app: 'demo' | 'backlog'): string[] {
  return readdirSync(join(hosted, app, 'assets')).filter((n) => n.startsWith('index-')).sort()
}

function readIndex(app: 'demo' | 'backlog'): string {
  return readFileSync(join(hosted, app, 'index.html'), 'utf8')
}

test.describe('GDK-673 one Vite bundle', () => {
  test('demo and backlog serve the same hashed JS/CSS bytes', () => {
    const demo = hashedAssets('demo')
    const backlog = hashedAssets('backlog')
    expect(demo, 'each app must emit hashed index-* assets').not.toEqual([])
    expect(backlog, 'copied bundle must keep the same asset names').toEqual(demo)
    for (const name of demo) {
      expect(
        sha256(join(hosted, 'backlog', 'assets', name)),
        `${name} must be the same bytes under /demo/ and /backlog/ — two Vite builds inlined different BASE_URL values`,
      ).toBe(sha256(join(hosted, 'demo', 'assets', name)))
    }
  })

  test('each app HTML names its own <base href> and keeps relative asset URLs', () => {
    const demo = readIndex('demo')
    const backlog = readIndex('backlog')
    expect(demo).toMatch(/<base\s+href="\/demo\/"\s*\/?>/i)
    expect(backlog).toMatch(/<base\s+href="\/backlog\/"\s*\/?>/i)
    expect(demo).toContain('src="./assets/')
    expect(backlog).toContain('src="./assets/')
    expect(demo).toContain('href="./assets/')
    expect(backlog).toContain('href="./assets/')
  })

  test('backlog config.json apiBase is the runtime mount, not a compile-time leftover', () => {
    const demoCfg = JSON.parse(readFileSync(join(hosted, 'demo', 'config.json'), 'utf8')) as {
      apiBase: string
    }
    const backlogCfg = JSON.parse(
      readFileSync(join(hosted, 'backlog', 'config.json'), 'utf8'),
    ) as { apiBase: string }
    expect(demoCfg.apiBase).toBe('/demo/api/v1/issues/')
    expect(backlogCfg.apiBase).toBe('/backlog/api/v1/issues/')
  })
})

function isBundle404(line: string): boolean {
  if (/favicon/i.test(line)) return false
  if (/\/api\/v1\/workspaces/.test(line)) return false
  if (line.startsWith('console:')) {
    return !/Failed to load resource|service.?worker/i.test(line)
  }
  return /\/(assets\/|config\.json|bootstrap\.json)/.test(line)
}

test.describe('GDK-673 each mount draws its own snapshot', () => {
  test('demo paints NMB issues; backlog paints GDK issues', async ({ page }) => {
    let bucket: 'demo' | 'backlog' = 'demo'
    const demoHits: string[] = []
    const backlogHits: string[] = []
    page.on('response', (res) => {
      if (res.status() !== 404) return
      ;(bucket === 'demo' ? demoHits : backlogHits).push(`${res.status()} ${res.url()}`)
    })
    page.on('console', (msg: ConsoleMessage) => {
      if (msg.type() !== 'error') return
      ;(bucket === 'demo' ? demoHits : backlogHits).push(`console: ${msg.text()}`)
    })

    await page.goto('/demo/')
    await dismissHostedFirstFrame(page)
    await expect(page.locator('html')).toHaveAttribute('data-base-path', '/demo/')
    // Visible rows are virtualized; the demo fixture is NMA/NMB/NMS, not GDK.
    await expect(page.getByText(/NM[ABS]-\d+/).first()).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText(/GDK-\d+/)).toHaveCount(0)
    const seriousDemo = demoHits.filter(isBundle404)
    expect(seriousDemo, `demo 404/console:\n${seriousDemo.join('\n')}`).toEqual([])

    bucket = 'backlog'
    await page.goto('/backlog/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 60_000 })
    await expect(page.locator('html')).toHaveAttribute('data-base-path', '/backlog/')
    await expect(page.getByText(/GDK-\d+/).first()).toBeVisible({ timeout: 60_000 })
    await expect(page.getByText(/NMB-\d+/)).toHaveCount(0)
    const seriousBacklog = backlogHits.filter(isBundle404)
    expect(seriousBacklog, `backlog 404/console:\n${seriousBacklog.join('\n')}`).toEqual([])
  })
})
