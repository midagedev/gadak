import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync } from 'node:child_process'
import { test, expect, type Page } from '@playwright/test'
import {
  apiURL,
  appConsoleErrors,
  attachConsoleErrors,
  DEMO_ISSUE_COUNT_EN,
  DEMO_ISSUE_COUNT_EN_RE,
  forceLocale,
} from './helpers'

const SHOT_DIR = join(dirname(fileURLToPath(import.meta.url)), '..', 'scratch', 'terminal-shots')

const KINDS = ['ghostty', 'xterm'] as const
type Kind = (typeof KINDS)[number]

type TermHook = {
  buffer: {
    active: {
      length: number
      getLine: (y: number) => { translateToString: (trimRight?: boolean) => string } | undefined
    }
  }
}

async function boot(page: Page, kind: Kind): Promise<string[]> {
  const errors = attachConsoleErrors(page)
  await forceLocale(page, 'en')
  await page.goto(`/?term=${kind}`)
  await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
  await expect(page.getByText(DEMO_ISSUE_COUNT_EN_RE).first()).toBeVisible({ timeout: 30_000 })
  await expect(page).toHaveURL(/[#?&]sc=/, { timeout: 30_000 })
  await expect(page.getByTestId('list-count')).not.toHaveText(DEMO_ISSUE_COUNT_EN)
  return errors
}

async function openPane(page: Page): Promise<void> {
  await page.keyboard.press('Control+Backquote')
  await expect(page.getByTestId('terminal-pane')).toBeVisible()
  await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
    timeout: 20_000,
  })
}

async function readTerm(page: Page): Promise<string> {
  return page.evaluate(() => {
    const t = (window as unknown as { __gadakTerm?: TermHook }).__gadakTerm
    if (!t) return ''
    const buf = t.buffer.active
    const lines: string[] = []
    for (let i = 0; i < buf.length; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? '')
    }
    return lines.join('\n')
  })
}

async function focusTerm(page: Page): Promise<void> {
  const pane = page.getByTestId('terminal-pane')
  // The renderer helper textarea is positioned off-canvas (xterm
  // `.xterm-helper-textarea`). Click the host, then focus the textarea.
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) {
    await host.first().click({ position: { x: 24, y: 24 } })
  } else {
    await pane.click({ position: { x: 24, y: 24 } })
  }
  await page.evaluate(() => {
    const el = document.querySelector<HTMLTextAreaElement>(
      '[data-testid="terminal-pane"] textarea',
    )
    el?.focus()
  })
}

async function typeLine(page: Page, line: string): Promise<void> {
  await focusTerm(page)
  await page.keyboard.type(line, { delay: 15 })
  await page.keyboard.press('Enter')
}

function lastStty(text: string): string | null {
  const matches = [...text.matchAll(/(\d+)\s+(\d+)/g)]
  if (matches.length === 0) return null
  return matches[matches.length - 1][0]
}

async function drainSessions(page: Page): Promise<void> {
  const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
  if (!res.ok()) return
  const body = (await res.json()) as { sessions?: { id: string }[] }
  for (const s of body.sessions ?? []) {
    await page.request.delete(apiURL(`/api/v1/terminal/sessions/${s.id}/`))
  }
}

test.describe('terminal pane', () => {
  for (const kind of KINDS) {
    test.describe(kind, () => {
      test.afterEach(async ({ page }) => {
        await drainSessions(page)
        const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
        const body = (await res.json()) as { sessions?: unknown[] }
        expect(body.sessions ?? []).toEqual([])
      })

      test(`echo, resize, Esc, replay, exit (${kind})`, async ({ page }) => {
        test.setTimeout(90_000)
        const errors = await boot(page, kind)
        await openPane(page)

        await typeLine(page, "printf 'spike-864-%s\\n' ok")
        await expect.poll(async () => readTerm(page)).toContain('spike-864-ok')

        await typeLine(page, 'stty size')
        let sizeBefore: string | null = null
        await expect
          .poll(async () => {
            sizeBefore = lastStty(await readTerm(page))
            return sizeBefore
          })
          .toMatch(/\d+\s+\d+/)

        const viewport = page.viewportSize() ?? { width: 1280, height: 720 }
        await page.setViewportSize({
          width: viewport.width + 280,
          height: viewport.height + 80,
        })
        await typeLine(page, 'stty size')
        await expect
          .poll(async () => {
            const after = lastStty(await readTerm(page))
            return after !== null && after !== sizeBefore
          })
          .toBe(true)

        await page.keyboard.press('Escape')
        await expect(page.getByTestId('terminal-pane')).toBeVisible()
        await expect(page.getByTestId('shortcuts-dialog')).toHaveCount(0)
        await expect(page.locator('[data-testid="issue-layout"]')).toHaveAttribute(
          'data-detail-open',
          'false',
        )

        await page.keyboard.press('Control+Backquote')
        await expect(page.getByTestId('terminal-pane')).toHaveCount(0)
        await page.keyboard.press('Control+Backquote')
        await expect(page.getByTestId('terminal-pane')).toBeVisible()
        await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
          timeout: 20_000,
        })
        await expect.poll(async () => readTerm(page)).toContain('spike-864-ok')

        await focusTerm(page)
        await page.keyboard.press('Control+C')
        await typeLine(page, 'exit')
        await expect(page.getByTestId('terminal-status')).toContainText('Shell exited', {
          timeout: 20_000,
        })

        expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
      })
    })
  }
})

test.describe('terminal shots', () => {
  test.use({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 2,
  })

  test.afterEach(async ({ page }) => {
    await drainSessions(page)
  })

  /*
   * 2026-08-25 — GDK-864 (lead review). The default width is 44% of the
   * *window*, but the pane is a flex child of the layout's main track, which
   * is capped at 1360px. On a wide display those two disagree and the issue
   * list is squeezed. TERMINAL_SPLIT_MAX_PCT is the ceiling; this pins it
   * where the disagreement is largest.
   *
   * FAIL-first, measured: with the max-width removed from TerminalPane's
   * split style, this test failed here at `pane took 71% of the track`
   * (ratio 0.7118) — the list kept 392px of a 1360px track.
   */
  test('a wide display does not let the pane eat the list', async ({ page }) => {
    await page.setViewportSize({ width: 2200, height: 900 })
    await boot(page, 'xterm')
    await openPane(page)
    const trackBox = await page.getByTestId('terminal-split').boundingBox()
    const paneBox = await page.getByTestId('terminal-pane').boundingBox()
    expect(trackBox, 'split wrapper box').not.toBeNull()
    expect(paneBox, 'pane box').not.toBeNull()
    const ratio = paneBox!.width / trackBox!.width
    expect(ratio, `pane took ${Math.round(ratio * 100)}% of the track`).toBeLessThanOrEqual(0.62)
    expect(trackBox!.width - paneBox!.width).toBeGreaterThan(400)
  })

  test('capture split, exited, overlay, dark', async ({ page }) => {
    test.setTimeout(90_000)
    mkdirSync(SHOT_DIR, { recursive: true })
    const hash = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()

    const shoot = async (name: string) => {
      await page.screenshot({
        path: join(SHOT_DIR, name),
        fullPage: false,
      })
    }

    await boot(page, 'ghostty')
    await openPane(page)
    await typeLine(page, 'ls -la')
    await expect.poll(async () => readTerm(page)).toMatch(/total\s+\d+/)
    await shoot('01-split-ghostty.png')

    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)

    await boot(page, 'xterm')
    await openPane(page)
    await typeLine(page, 'ls -la')
    await expect.poll(async () => readTerm(page)).toMatch(/total\s+\d+/)
    await shoot('02-split-xterm.png')

    await focusTerm(page)
    await page.keyboard.press('Control+C')
    await typeLine(page, 'exit')
    await expect(page.getByTestId('terminal-status')).toContainText('Shell exited')
    await shoot('03-exited.png')

    await boot(page, 'ghostty')
    await openPane(page)
    await typeLine(page, "printf 'overlay\\n'")
    await page.setViewportSize({ width: 820, height: 900 })
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-overlay', 'true')
    await shoot('04-narrow-overlay.png')
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)

    await page.setViewportSize({ width: 1440, height: 900 })
    await boot(page, 'ghostty')
    await page.evaluate(() => {
      document.documentElement.setAttribute('data-theme', 'dark')
    })
    await openPane(page)
    await typeLine(page, 'ls -la')
    await expect.poll(async () => readTerm(page)).toMatch(/total\s+\d+/)
    await shoot('05-dark-ghostty.png')

    writeFileSync(
      join(SHOT_DIR, 'MANIFEST.md'),
      [
        '# terminal pane captures (GDK-864)',
        '',
        `- source: \`${hash}\``,
        `- viewport: 1440×900, deviceScaleFactor 2 (04 is 820×900)`,
        '',
        '| file | renderer | notes |',
        '| --- | --- | --- |',
        '| `01-split-ghostty.png` | ghostty | pane + list, `ls -la` |',
        '| `02-split-xterm.png` | xterm | same, `?term=xterm` |',
        '| `03-exited.png` | xterm | exited status line |',
        '| `04-narrow-overlay.png` | ghostty | 820px overlay |',
        '| `05-dark-ghostty.png` | ghostty | `data-theme=dark` |',
        '',
      ].join('\n'),
      'utf8',
    )
  })
})
