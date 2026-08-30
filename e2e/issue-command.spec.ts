import { test, expect } from '@playwright/test'
import {
  apiURL,
  appConsoleErrors,
  attachConsoleErrors,
  drainTerminalSessions,
  forceLocale,
  readTerm,
} from './helpers'
import {
  BOUND_ISSUE,
  COMMAND,
  LONE_ISSUE,
  OUTPUT,
  focusTerm,
  injectCommandBodies,
  openIssue,
  openPaneBoundTo,
  sessions,
} from './issue-command-fixture'

/*
 * GDK-1162 / GDK-1164-A — a command block in an issue body, and the shell it
 * reaches.
 *
 * The contract under test is a refusal as much as an action: ▶ *places* the
 * line at the prompt of the shell bound to this issue, and does not run it.
 * Both halves are asserted positively — the text appears without its output,
 * and then a human's Enter produces the output — because an absence check
 * alone cannot tell "it did not run" from "nothing happened at all".
 *
 * Everything the captures also need — the injected bodies, the two issue
 * keys, the marker command — lives in ./issue-command-fixture. The captures
 * themselves are e2e/demo/issue-command-shots.spec.ts: this file is a merge
 * gate, and a screenshot helper is not one.
 */

/* Typed after the placement to order the buffer read against it. A plain
 * word, not a `# comment`: interactive comments are on in bash and off in
 * zsh, and the shell behind the pane is whichever one the machine has. As
 * printf's second argument it changes nothing about the first one. */
const BARRIER = 'GDK1162SETTLED'

test.describe('command blocks in an issue body', () => {
  /*
   * Wide enough that the pane is a split rather than an overlay. Below
   * TERMINAL_SPLIT_WITH_DETAIL_MIN_PX a detail panel and an open pane cannot
   * share the row, so the pane covers the body — and an overlay that sits on
   * top of the ▶ makes the button unclickable, which is a layout fact, not a
   * fact about this feature. (Measured at the 1280 default: every click
   * retried against "xterm-screen intercepts pointer events".) The narrow
   * case is real, and it is the one where the pane is *closed* when ▶ is
   * pressed — which is what the third test below walks.
   */
  test.use({ viewport: { width: 1600, height: 900 } })

  test.beforeEach(async ({ page }) => {
    await forceLocale(page, 'en')
    await injectCommandBodies(page)
  })

  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  test('▶ places the command at the prompt and does not run it', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = attachConsoleErrors(page)

    await openIssue(page, BOUND_ISSUE)
    await openPaneBoundTo(page, BOUND_ISSUE)

    const body = page.getByTestId('issue-body')
    // The wait is on the asserted state itself: the body's own shell verdict
    // flips to `attached` when the poll sees the binding. Waiting on the poll
    // interval instead would only prove that time passed.
    await expect(body).toHaveAttribute('data-shell', 'attached', { timeout: 20_000 })
    await expect(page.getByTestId('no-shell-hint')).toHaveCount(0)

    // Exactly one ▶: the multi-line block does not get one, because the serve
    // refuses a payload carrying a newline.
    const runners = body.locator('[data-run-command]')
    await expect(runners).toHaveCount(1)
    await expect(runners.first()).toHaveAttribute('data-run-command', COMMAND)

    /*
     * The shell must own the tty before anything is placed in it. `attached`
     * only says the socket is open, and bytes written before the shell has
     * taken the terminal over are echoed by the line discipline and then
     * dropped — measured: against a deliberately broken build (client
     * appending \n, serve with the newline guard removed) the command still
     * did not run, because the whole line was swallowed at startup. That made
     * this test pass for the wrong reason, which is worse than failing. The
     * printf below is the handshake terminal.spec.ts already uses.
     */
    await focusTerm(page)
    await page.keyboard.type("printf 'GDK1162%s\\n' RDY")
    await page.keyboard.press('Enter')
    await expect.poll(async () => readTerm(page)).toContain('GDK1162RDY')

    await runners.first().click()

    // Half one: the line is at the prompt.
    await expect
      .poll(async () => readTerm(page), 'the command should be sitting at the prompt')
      .toContain(COMMAND)

    // Half two: it has not run — but only after a barrier, because "the
    // output is not there yet" and "the output is never coming" look the same
    // in a buffer read one millisecond after the write. BARRIER is typed by
    // the keyboard *after* the placement, and a PTY is FIFO: once its echo is
    // on screen, anything the earlier bytes were going to produce already is
    // too. (Measured: without this the assertion passed against a deliberately
    // broken build that appended \n and a serve with the newline guard
    // removed — the poll simply won the race.)
    await focusTerm(page)
    await page.keyboard.type(` ${BARRIER}`)
    await expect.poll(async () => readTerm(page)).toContain(BARRIER)
    expect(await readTerm(page), 'the placed line must not have run itself').not.toContain(OUTPUT)

    // Half three, the positive proof that half two meant anything: a person's
    // Enter runs it. BARRIER rides along as printf's second argument, so the
    // line is still the command it was.
    await page.keyboard.press('Enter')
    await expect
      .poll(async () => readTerm(page), 'Enter should run what the ▶ placed')
      .toContain(OUTPUT)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('with no shell on the issue the ▶ is inert and says why', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = attachConsoleErrors(page)

    // A pane is open and bound elsewhere: this is the interesting case, since
    // "no session at all" and "a session that is not yours" look the same to
    // a reader and must not.
    await openIssue(page, BOUND_ISSUE)
    await openPaneBoundTo(page, BOUND_ISSUE)
    await expect(page.getByTestId('issue-body')).toHaveAttribute('data-shell', 'attached', {
      timeout: 20_000,
    })

    await openIssue(page, LONE_ISSUE)
    const body = page.getByTestId('issue-body')
    await expect(body).toHaveAttribute('data-shell', 'none')

    const hint = page.getByTestId('no-shell-hint')
    await expect(hint).toBeVisible()
    await expect(hint).toContainText(LONE_ISSUE)

    // Inert, not hidden: it is where someone learns shells bind to issues.
    const runner = body.locator('[data-run-command]').first()
    await expect(runner).toBeVisible()
    const before = await readTerm(page)
    await runner.click()
    // Nothing was placed. Compared against the buffer as it was, so a shell
    // that printed something of its own does not read as a false pass.
    expect(await readTerm(page)).toBe(before)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('with the pane closed, ▶ opens it on the bound shell — not the last one it drew', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const errors = attachConsoleErrors(page)

    /*
     * Placing a line into a shell nobody can see makes the whole design
     * ("read it, then press Enter") impossible, so a closed pane has to come
     * back on the *bound* session.
     *
     * The setup makes that a real choice rather than a coincidence. The pane
     * opens its own session A and remembers it; a second session B is created
     * over REST and is the one bound to the issue — which is what "a shell
     * claimed in another window, or before this pane was opened" looks like.
     * With the pane's own memory left alone, reopening lands on A and the
     * placed line is nowhere on screen. (Measured: an earlier form of this
     * test bound the pane's own session, so A and B were the same row and it
     * passed against a build with the re-targeting removed.)
     */
    await openIssue(page, BOUND_ISSUE)
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    const paneSession = (await sessions(page))[0]
    expect(paneSession, 'the pane should have created a session').toBeTruthy()

    const created = await page.request.post(apiURL('/api/v1/terminal/sessions/'), {
      data: { cols: 90, rows: 30 },
    })
    expect(created.ok(), `creating the second session: ${created.status()}`).toBe(true)
    const other = ((await created.json()) as { id: string }).id
    expect(other).not.toBe(paneSession.id)
    const bind = await page.request.post(apiURL(`/api/v1/terminal/sessions/${other}/issue/`), {
      data: { issue_key: BOUND_ISSUE },
    })
    expect(bind.ok(), `binding ${BOUND_ISSUE}: ${bind.status()}`).toBe(true)

    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)

    const body = page.getByTestId('issue-body')
    await expect(body).toHaveAttribute('data-shell', 'attached', { timeout: 20_000 })
    await body.locator('[data-run-command]').first().click()

    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    await expect
      .poll(async () => readTerm(page), 'the reopened pane should be showing the bound shell')
      .toContain(COMMAND)
    // And it reattached rather than opening a third shell.
    expect((await sessions(page)).length).toBe(2)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  test('the unattended mark lands on the claim with no shell, not on the one with a shell', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const errors = attachConsoleErrors(page)

    await openIssue(page, BOUND_ISSUE)
    await openPaneBoundTo(page, BOUND_ISSUE)
    await expect(page.getByTestId('issue-body')).toHaveAttribute('data-shell', 'attached', {
      timeout: 20_000,
    })
    // The issue a shell is on carries no mark — that is the half that makes
    // the mark mean something.
    await expect(page.getByTestId('unattended-chip')).toHaveCount(0)

    await openIssue(page, LONE_ISSUE)
    const chip = page.getByTestId('unattended-chip')
    await expect(chip).toBeVisible({ timeout: 20_000 })
    // The wording is the feature: it reports what this serve can see, never
    // that the work is dead.
    const hint = await chip.getAttribute('title')
    expect(hint).toContain('no shell this serve knows about')
    expect(hint?.toLowerCase()).not.toContain('dead')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})
