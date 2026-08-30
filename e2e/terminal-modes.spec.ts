import { test, expect, type Page } from '@playwright/test'
import {
  apiURL,
  appConsoleErrors,
  attachConsoleErrors,
  drainTerminalSessions,
  DEMO_ISSUE_COUNT_EN,
  DEMO_ISSUE_COUNT_EN_RE,
  forceLocale,
  readTerm,
} from './helpers'

/*
 * GDK-1045 (2026-08-27). A TUI that probes modes with DECRQM (`CSI ? Pn $ p` —
 * crush sends `ESC[?2026$p` as part of its synchronized-output handshake)
 * used to blank the pane entirely. Root cause was never in xterm or in our
 * source: the vite build downleveled the `||=` in xterm's requestMode for the
 * default es2020-ish target into `(void 0 || (i = {}))` with `i` undeclared,
 * so the ESM bundle threw ReferenceError inside _innerWrite and every byte
 * after the query in that write chunk was dropped. Fixed by pinning
 * build.target in vite.config.ts — see the attribution comment there.
 *
 * This gate runs against the built bundle (e2e/serve.sh runs `npm run build`
 * and serves dist/app); the defect does not exist in dev-server output, so a
 * dev-server run of this spec proves nothing.
 *
 * FAIL-first, measured 2026-08-27 with build.target unset: the poll below
 * timed out with the buffer ending at the echoed command line — no marker
 * and not even the returning prompt (the whole chunk after the query was
 * dropped) — and the page had logged `ReferenceError: i is not defined`
 * from xterm.
 */

async function boot(page: Page): Promise<string[]> {
  const errors = attachConsoleErrors(page)
  await forceLocale(page, 'en')
  await page.goto('/')
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

test.describe('terminal mode reports', () => {
  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
    const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
    const body = (await res.json()) as { sessions?: unknown[] }
    expect(body.sessions ?? []).toEqual([])
  })

  test('DECRQM does not kill the parser for the rest of the chunk', async ({ page }) => {
    test.setTimeout(90_000)
    const errors = attachConsoleErrors(page)
    // An uncaught exception is not a console event, so it needs its own
    // collector — and its own assertion at the end. Printing it was not
    // enough: a regression that throws inside the parser but still lets the
    // marker through would leave this gate green on the axis that names the
    // cause.
    const pageErrors: string[] = []
    page.on('pageerror', (err) => {
      const first = String(err).split('\n')[0]
      pageErrors.push(first)
      console.log(`[pageerror] ${first}`)
    })
    await boot(page)
    await openPane(page)

    // The pane renders ordinary output before the probe — pins the failure
    // mode to "died on DECRQM", not "pane never attached".
    await typeLine(page, 'echo GDK1045-BEFORE:$((6*7))')
    await expect.poll(async () => readTerm(page)).toContain('GDK1045-BEFORE:42')

    // Two DECRQM probes (synchronized output 2026 — crush's handshake — and
    // DECCKM 1) followed by a marker, all emitted by one printf: one write
    // chunk. The marker rides the %s argument, computed by the shell in
    // double quotes OUTSIDE the format string — an arithmetic expansion
    // inside the single-quoted format would not expand, and a literal marker
    // would let the echoed command line satisfy the assertion. Only parsed
    // printf output can produce `GDK1045-AFTER:42`.
    await typeLine(page, "printf '\\033[?2026$p\\033[?1$pGDK1045-AFTER:%s\\n' \"$((6*7))\"")
    await expect.poll(async () => readTerm(page)).toContain('GDK1045-AFTER:42')

    // Later chunks still parse. A healthy xterm answers each DECRQM with a
    // DECRPM the shell reads as input, so fragments like `/1;2$y` are sitting
    // on the current line — Ctrl-C discards it, and a second computed marker
    // then proves the pane is still parsing whole chunks.
    //
    // Not the returning prompt: that was the first shape of this check and it
    // asserted a `>`, which is this machine's prompt and not the Linux
    // runner's. A gate may not depend on what someone's PS1 looks like.
    await focusTerm(page)
    await page.keyboard.press('Control+c')
    await page.keyboard.press('Enter')
    await typeLine(page, "printf 'GDK1045-LIVE:%s\\n' \"$((7*8))\"")
    await expect.poll(async () => readTerm(page)).toContain('GDK1045-LIVE:56')

    expect(pageErrors, `uncaught page errors:\n${pageErrors.join('\n')}`).toEqual([])
    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
