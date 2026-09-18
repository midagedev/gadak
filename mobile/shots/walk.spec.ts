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
  // half-offset layout as a defect. Measured in cycle 1: awaiting
  // getAnimations() alone was not enough — the page-detail frame came back
  // as a translucent overlay on the live list and was reported as a
  // high-severity layout defect that does not exist in the code (both
  // pushes are `background: var(--color-bg-base)`). The extra settle is
  // what makes the picture the screen.
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  await page.waitForTimeout(400)
  await page.evaluate(async () => {
    await Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {})))
  })
  const file = `${String(shots.length + 1).padStart(2, '0')}-${label}.png`
  await page.screenshot({ path: join(outDir, file) })
  shots.push({ label, file, note })
}

async function waitPaired(page: Page): Promise<void> {
  await page.locator('h1 button.scope').waitFor()
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

  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await shoot(page, 'scope-sheet', 'the heading is the scope control')
  // button.cancel, not the role query: the scrim shares the aria-label and
  // a taller sheet (the sprint row) puts the panel over the scrim's centre.
  await page.locator('button.palette-cancel').click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })

  await page.locator('.pane:not(.off) button.row').first().click()
  await page.locator('button.back').waitFor()
  await shoot(page, 'detail', 'issue detail, top')
  await page.locator('.detail-layer main, .detail-layer .body').first().evaluate((el) => {
    el.scrollTop = el.scrollHeight
  }).catch(() => {})
  await shoot(page, 'detail-bottom', 'issue detail scrolled to the end')

  // This used to shoot the transition picker. `gadak demo` is a serve with no
  // origin credential, so the transitions GET answers 409 and — since GDK-906
  // — the control refuses at the control instead of opening a sheet it cannot
  // fill. The frame worth having on this fixture is that refusal, which is a
  // real state a reviewer should look at. The picker itself is unreachable
  // here; GDK-911 tracks the fixture that would bring it back.
  const chip = page.locator('button.status').first()
  if ((await chip.count()) > 0) {
    await chip.click()
    await chip.and(page.locator(':disabled')).waitFor()
    await shoot(page, 'detail-writes-off', 'status control refused: serve has no origin credential')
  }
  await page.locator('button.back').first().click()
  await page.locator('.pane:not(.off) button.row').first().waitFor()

  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'Updated' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()
  await shoot(page, 'docs', 'documents plate')

  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().click()
  await page.locator('.page-detail button.back').waitFor()
  await shoot(page, 'page-detail', 'wiki page detail')
  await page.locator('.page-detail button.back').first().click()
  await page.locator('.pane:not(.off) button.row[data-testid="doc-row"]').first().waitFor()

  // GDK-902 2026-09-15: search is the palette with a query, and the shell
  // is a row in the same palette rather than a tab beside it.
  await page.locator('button.search').click()
  await page.locator('.pane:not(.off) input').first().waitFor()
  await shoot(page, 'search-empty', 'search before a query')
  await page.locator('.pane:not(.off) input').first().fill('tenant')
  await page.locator('.pane:not(.off) button.row').first().waitFor()
  await shoot(page, 'search-results', 'search results')

  await page.locator('.pane:not(.off) input').first().fill('')
  const shellRow = page.locator('button.palette-row', { hasText: /Terminal|Shell/ })
  if ((await shellRow.count()) === 0) {
    throw new Error('shots: no Terminal row in the palette — the seed did not take')
  }
  await shellRow.click()
  // Wait for the real attachment, not a timeout: a picture of a pane that
  // had not connected yet is a picture of nothing, and it photographs
  // identically to a pane that failed.
  await page.locator('[data-testid="terminal-pane"][data-attached="true"]').waitFor({ timeout: 30_000 })
  await page.locator('.xterm-rows').waitFor()
  // Wait for the shell to have *said* something. `data-attached` is the
  // socket; the first prompt is a byte the PTY still has to send, and
  // cycle 1 photographed the gap between them as an empty pane — reported,
  // reasonably, as "a live shell that does not admit it is live".
  await page
    .locator('.xterm-rows')
    .filter({ hasText: /\S/ })
    .first()
    .waitFor({ timeout: 20_000 })
  await shoot(page, 'shell', 'terminal attached, first prompt')
  // Give the reviewer something to read: a command with wrapping output at
  // this width is where the 40-column question (GDK-900) is decided.
  // Focus and type exactly the way e2e/shell.spec.ts does. `locator.type()`
  // on the sink dispatches at the element and the line never reached the PTY;
  // the pane keys off the host click plus real keyboard events.
  const host = page.locator('[data-gadak-editable]')
  if ((await host.count()) > 0) await host.first().click({ position: { x: 24, y: 24 } })
  await page.getByTestId('shell-ime').focus()
  await page.keyboard.type('echo hello from the phone; ls -la', { delay: 15 })
  await page.keyboard.press('Enter')
  // Assert the output arrived rather than trusting a timeout: a frame of a
  // prompt with nothing under it is indistinguishable from a broken sink.
  await page
    .locator('.xterm-rows')
    .filter({ hasText: /hello from the phone/ })
    .first()
    .waitFor({ timeout: 20_000 })
  await page.waitForTimeout(600)
  await shoot(page, 'shell-output', 'a command run and its output at 402pt')
  const bar = page.locator('button', { hasText: /^ctrl$/i }).first()
  if ((await bar.count()) > 0) {
    await bar.click()
    await shoot(page, 'shell-ctrl', 'sticky Ctrl armed')
    await bar.click()
  }

  // GDK-902 2026-09-15: the shell owns the whole column, so the way to
  // anything else is its own back control; the `pairing` shoot label is
  // then the Settings push layer behind the gear in the list's heading —
  // the label is the gate's (it compares labels), the screen is Settings.
  await page.locator('.pane:not(.off) .head button.back').click()
  await page.locator('h1 button.scope').waitFor()
  await page.locator('button.gear').click()
  await page.locator('.settings-layer button.back').waitFor()
  await shoot(page, 'pairing', 'pairing — two pairings, one screen')

  await page.locator('.settings-layer button.back').click()
  await page.locator('.settings-layer').waitFor({ state: 'detached' })
  // Put the scope back. The Documents pick above is sticky, so without this
  // the two dark frames photograph the *docs* list and its page detail —
  // cycle 1 shipped exactly that, and the reviewer correctly reported that
  // the light landing had no dark pair at all.
  await page.locator('.pane:not(.off) button.search').click()
  await page.locator('.palette-field input').waitFor()
  await page.locator('button.palette-row', { hasText: 'All open' }).click()
  await page.locator('.palette-field input').waitFor({ state: 'detached' })
  await page.locator('.pane:not(.off) button.row:not([data-testid="doc-row"])').first().waitFor()
  await page.emulateMedia({ colorScheme: 'dark' })
  await shoot(page, 'issues-dark', 'the same landing, dark')
  await page.locator('.pane:not(.off) button.row').first().click()
  // GDK-902 2026-09-15: scoped to the layer. Once the shell has been the
  // owner its pane stays mounted (hidden) with a back control of its own in
  // the header, so a bare `button.back` resolves to two elements and
  // Playwright's strict mode refuses it. The detail's back is the one this
  // walk means.
  await page.locator('.detail-layer button.back').waitFor()
  await shoot(page, 'detail-dark', 'issue detail, dark')
  await page.locator('.detail-layer button.back').first().click()
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
