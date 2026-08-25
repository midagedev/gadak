import { test, type Page } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

/*
 * The capture walk (GDK-904). One picture per state a person actually
 * lands in, at the phone's own size, so a review round judges the app and
 * not a desktop window narrowed by hand.
 *
 * The walk deliberately mirrors e2e/viewport.spec.ts step for step: that
 * file is the geometry gate and this one is its camera. When a selector
 * moves, both break together, which is the point — a camera pointed at a
 * screen that no longer exists is how a stale verdict happens.
 */

const here = dirname(fileURLToPath(import.meta.url))
const repoRoot = join(here, '..', '..')
const cycle = process.env.SHOTS_CYCLE || 'c1'
const outDir = join(repoRoot, 'scratch', 'mobile-shots', cycle)

const shots: { label: string; file: string; note: string }[] = []

async function shoot(page: Page, label: string, note: string): Promise<void> {
  // Settle any transition first: a fly() caught mid-flight photographs a
  // screen the person never sees, and the reviewer will report its
  // half-offset layout as a defect.
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  const file = `${String(shots.length + 1).padStart(2, '0')}-${label}.png`
  await page.screenshot({ path: join(outDir, file) })
  shots.push({ label, file, note })
}

async function waitPaired(page: Page): Promise<void> {
  await page.locator('nav.safe-bottom').waitFor()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
}

/**
 * Seed a dev terminal pairing so the Shell tab exists.
 *
 * In dev the token lives in localStorage (secure.ts refuses to write the
 * device Keychain from a dev webview) and the endpoint is '' — the vite
 * proxy carries /api to the demo serve on 7899, which registers the
 * terminal routes unconditionally and admits a loopback peer. So this is
 * not a mock: the shell that appears is a real PTY.
 */
async function seedTerminal(page: Page): Promise<void> {
  await page.addInitScript(() => {
    localStorage.setItem('gadak.dev.token.terminal', 'dev-proxy-loopback')
    localStorage.setItem(
      'gadak.pairing.meta.terminal',
      JSON.stringify({ endpoint: '', label: 'dev shell', expires_at: '2099-01-01T00:00:00Z' }),
    )
  })
}

test('walk', async ({ page }) => {
  rmSync(outDir, { recursive: true, force: true })
  mkdirSync(outDir, { recursive: true })

  await seedTerminal(page)
  // No ?demo-tour — the tour must stay disarmed or it moves the app out
  // from under the camera (GDK-869).
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await waitPaired(page)

  await shoot(page, 'issues', 'landing: the list a person sees first')

  await page.locator('.pane:not(.off) h1 button.scope').click()
  await page.locator('button.cancel').waitFor()
  await shoot(page, 'scope-sheet', 'the heading is the scope control')
  await page.getByRole('button', { name: /cancel/i }).first().click()
  await page.locator('button.cancel').waitFor({ state: 'hidden' })

  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  await shoot(page, 'detail', 'issue detail, top')
  await page.locator('.detail-layer main, .detail-layer .body').first().evaluate((el) => {
    el.scrollTop = el.scrollHeight
  }).catch(() => {})
  await shoot(page, 'detail-bottom', 'issue detail scrolled to the end')

  const chip = page.locator('button.status').first()
  if ((await chip.count()) > 0) {
    await chip.click()
    await page.locator('button.cancel').waitFor()
    await shoot(page, 'status-sheet', 'transition picker')
    await page.getByRole('button', { name: /cancel/i }).first().click()
    await page.locator('button.cancel').waitFor({ state: 'hidden' })
  }
  await page.locator('button.back').first().click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  await page.locator('.pane:not(.off) h1 button.scope').click()
  await page.locator('button.cancel').waitFor()
  await page.locator('.sheet button.row', { hasText: 'Updated' }).click()
  await page.locator('button.cancel').waitFor({ state: 'hidden' })
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()
  await shoot(page, 'docs', 'documents plate')

  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().click()
  await page.locator('.page-detail button.back').waitFor()
  await shoot(page, 'page-detail', 'wiki page detail')
  await page.locator('.page-detail button.back').first().click()
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()

  const tabs = page.locator('nav.safe-bottom button.tab')
  await tabs.filter({ hasText: 'Search' }).click()
  await page.locator('.pane:not(.off) input').first().waitFor()
  await shoot(page, 'search-empty', 'search before a query')
  await page.locator('.pane:not(.off) input').first().fill('tenant')
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await shoot(page, 'search-results', 'search results')

  const shellTab = tabs.filter({ hasText: /Terminal|Shell/ })
  if ((await shellTab.count()) === 0) throw new Error('shots: no Shell tab — the seed did not take')
  await shellTab.click()
  // Wait for the real attachment, not a timeout: a picture of a pane that
  // had not connected yet is a picture of nothing, and it photographs
  // identically to a pane that failed.
  await page.locator('[data-testid="terminal-pane"][data-attached="true"]').waitFor({ timeout: 30_000 })
  await page.locator('.xterm-rows').waitFor()
  await page.waitForTimeout(1200) // first prompt paints
  await shoot(page, 'shell', 'terminal attached, first prompt')
  // Give the reviewer something to read: a command with wrapping output at
  // this width is where the 40-column question (GDK-900) is decided.
  const sink = page.locator('[data-testid="terminal-pane"] textarea').first()
  await sink.focus()
  await sink.type('echo hello from the phone; ls', { delay: 12 })
  await page.keyboard.press('Enter')
  await page.waitForTimeout(1500)
  await shoot(page, 'shell-output', 'a command run and its output at 402pt')
  const bar = page.locator('button', { hasText: /^ctrl$/i }).first()
  if ((await bar.count()) > 0) {
    await bar.click()
    await shoot(page, 'shell-ctrl', 'sticky Ctrl armed')
    await bar.click()
  }

  await tabs.filter({ hasText: 'Pairing' }).click()
  await page.getByRole('heading', { name: 'Pairing' }).waitFor()
  await shoot(page, 'pairing', 'pairing tab — two pairings, one screen')

  await tabs.first().click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await page.emulateMedia({ colorScheme: 'dark' })
  await shoot(page, 'issues-dark', 'the same landing, dark')
  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  await shoot(page, 'detail-dark', 'issue detail, dark')
  await page.locator('button.back').first().click()
  await page.emulateMedia({ colorScheme: 'light' })

  // No PairGate shot. In dev the app is paired by construction (the vite
  // proxy is the endpoint), so clearing storage and reloading lands back
  // on the list — the first attempt produced a byte-identical copy of
  // 01-issues labelled `pair-gate`, which is exactly the mislabelled
  // capture that poisons a review round. PairGate needs a packaged build
  // or a dev flag it does not have yet; capture it there, not here.

  const hash = execFileSync('git', ['rev-parse', '--short', 'HEAD'], {
    cwd: repoRoot,
    encoding: 'utf8',
  }).trim()
  const dirty = execFileSync('git', ['status', '--porcelain'], {
    cwd: repoRoot,
    encoding: 'utf8',
  }).trim()
  const source = dirty ? `${hash} + uncommitted edits (${dirty.split('\n').length} files)` : hash
  writeFileSync(
    join(outDir, 'MANIFEST.md'),
    [
      `# mobile shots — ${cycle}`,
      '',
      `source: ${source}`,
      `viewport: 402×874 @3x, isMobile, hasTouch, locale en-US`,
      `fixture: gadak demo on 127.0.0.1:7899 through the vite proxy`,
      '',
      ...shots.map((s) => `- \`${s.file}\` — ${s.note}`),
      '',
    ].join('\n'),
  )
  console.log(`shots: ${shots.length} → ${outDir}`)
})
