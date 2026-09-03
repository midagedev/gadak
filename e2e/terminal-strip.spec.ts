import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync } from 'node:child_process'
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
 * The session strip (GDK-1153 / GDK-1163 / GDK-1160).
 *
 * The server has been fully multi-session since GDK-864; what made gadak
 * single-session was the UI. The contract this file pins is the one that
 * breaks silently: with more than one shell alive, the pane must be on the
 * session the strip says it is on, and switching must bring that session's
 * scrollback with it — not the previous one's, and not both spliced.
 *
 * Every wait below is on the state being asserted (the buffer text, the
 * row's own selected flag). A proxy wait would only prove the test ran.
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
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) {
    await host.first().click({ position: { x: 24, y: 24 } })
  } else {
    await pane.click({ position: { x: 24, y: 24 } })
  }
  await page.evaluate(() => {
    const el = document.querySelector<HTMLTextAreaElement>('[data-testid="terminal-pane"] textarea')
    el?.focus()
  })
}

async function typeLine(page: Page, line: string): Promise<void> {
  await focusTerm(page)
  await page.keyboard.type(line, { delay: 15 })
  await page.keyboard.press('Enter')
}

/** Session ids the server holds right now, oldest first — the strip's own order. */
async function sessionIds(page: Page): Promise<string[]> {
  const res = await page.request.get(apiURL('/api/v1/terminal/sessions/'))
  const body = (await res.json()) as { sessions?: { id: string }[] }
  return (body.sessions ?? []).map((s) => s.id)
}

/** Same dataset surface terminal.spec.ts reads: what did the keystroke do? */
function lastKeyCmdOf(page: Page): Promise<string | null> {
  return page.locator('html').getAttribute('data-last-key-cmd')
}

test.describe('terminal session strip', () => {
  test.beforeEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })
  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  /*
   * The whole point, and the thing a "just draw another tab" implementation
   * gets wrong: two live shells, and the pane is on the one that was picked,
   * carrying that shell's history and not the other's.
   *
   * FAIL-first, measured 2026-08-30 against the first draft of the switch:
   * `expect(received).toContain("MARKER-TWO")` — the buffer instead held
   * MARKER-ONE a dozen times over. Closing a live socket to move sessions
   * ran the old socket's own onClose, which read phase === 'live' and
   * reconnected the session the pane had just left; the two attachments
   * then took turns replaying their rings into one buffer. The generation
   * guard in TerminalPane's detachSocket is what this pins.
   */
  test('two shells: clicking a row moves the pane, and the scrollback follows', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await openPane(page)

    // Session one, marked. The split literal keeps the marker out of the
    // echoed command line, so a hit means the shell actually printed it.
    await typeLine(page, "printf 'MARKER-''ONE\\n'")
    await expect.poll(async () => readTerm(page)).toContain('MARKER-ONE')
    const [firstId] = await sessionIds(page)
    expect(firstId, 'the pane should have created a session').toBeTruthy()

    // One session is still a tab (GDK-1199): the strip is resident chrome
    // now, and the tab carries the name the rail's own slot used to
    // (GDK-1153) — same name, new place, never blank.
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '1', {
      timeout: 20_000,
    })
    await expect(page.getByTestId('terminal-strip-name')).toBeVisible()
    await expect(page.getByTestId('terminal-strip-name')).not.toHaveText('')
    // …and the dock's chrome is that one column (GDK-1355; one row before
    // it): the rows and both verbs share a single container, and the old
    // rail (label + name slot) is gone.
    const chrome = page.getByTestId('terminal-chrome')
    await expect(chrome.getByTestId('terminal-strip')).toHaveCount(1)
    await expect(chrome.getByTestId('terminal-new')).toBeVisible()
    await expect(chrome.getByTestId('terminal-close')).toBeVisible()
    await expect(page.getByTestId('terminal-rail-name')).toHaveCount(0)

    // A second shell from the rail.
    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '2', {
      timeout: 20_000,
    })

    // The new session starts with its own empty history, not the old one's.
    await typeLine(page, "printf 'MARKER-''TWO\\n'")
    await expect.poll(async () => readTerm(page)).toContain('MARKER-TWO')
    expect(await readTerm(page), 'a fresh session must not inherit a scrollback').not.toContain(
      'MARKER-ONE',
    )

    const rowOne = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${firstId}"]`,
    )
    await expect(rowOne).toHaveAttribute('data-selected', 'false')

    // The switch. Both waits are on the asserted state itself: the row's own
    // selected flag, and the buffer that must have followed it.
    await rowOne.click()
    await expect(rowOne).toHaveAttribute('data-selected', 'true')
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    await expect
      .poll(async () => readTerm(page), 'session one’s scrollback should follow the row')
      .toContain('MARKER-ONE')
    expect(
      await readTerm(page),
      'the previous session’s output must not be spliced into this one',
    ).not.toContain('MARKER-TWO')

    // …and back, so the move is a switch and not a one-way replay.
    const ids = await sessionIds(page)
    const secondId = ids.find((id) => id !== firstId)
    expect(secondId, 'two sessions should be alive').toBeTruthy()
    const rowTwo = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${secondId}"]`,
    )
    await rowTwo.click()
    await expect(rowTwo).toHaveAttribute('data-selected', 'true')
    await expect.poll(async () => readTerm(page)).toContain('MARKER-TWO')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1364: the × is hover-revealed, and revealed by opacity, not display —
   * a display:none element is out of the tab order, so a "focus-visible"
   * class on it can never fire. The keyboard path is: focus the row, Tab
   * onto the ×, Enter. The first cut of the roster column (GDK-1355) used
   * `hidden … focus-visible:flex`, and this test could not focus it.
   */
  test('the × is reachable from the keyboard: Tab from the row, Enter kills', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await openPane(page)
    const [firstId] = await sessionIds(page)
    expect(firstId).toBeTruthy()
    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '2', {
      timeout: 20_000,
    })
    const secondId = (await sessionIds(page)).find((id) => id !== firstId)
    const rowTwo = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${secondId}"]`,
    )
    // The click above left the pointer where "+ New shell" was — which is
    // where the new row now sits, so the × would be hover-revealed by
    // accident and this test would pass on a display:none control
    // (measured: it did). Park the pointer first; the keyboard is on its own.
    await page.mouse.move(0, 0)
    await expect(rowTwo).toHaveAttribute('data-selected', 'true', { timeout: 20_000 })
    await rowTwo.focus()
    await page.keyboard.press('Tab')
    await expect(rowTwo.getByTestId('terminal-strip-kill')).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(rowTwo).toHaveCount(0, { timeout: 20_000 })
    await expect
      .poll(async () => sessionIds(page), 'the DELETE should have reached the server')
      .toEqual([firstId])

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1200: a tab's × ends that session — the client finally calls the
   * DELETE the server has kept since GDK-922. Killing the shown session
   * must hand the pane to a neighbour, not leave it attached to a shell
   * the server is tearing down; with two sessions and the right-hand one
   * killed, the neighbour is the left one.
   */
  test('a tab’s × ends that session and the pane moves to the neighbour (GDK-1200)', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await openPane(page)
    const [firstId] = await sessionIds(page)
    expect(firstId, 'the pane should have created a session').toBeTruthy()

    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '2', {
      timeout: 20_000,
    })
    const ids = await sessionIds(page)
    const secondId = ids.find((id) => id !== firstId)
    expect(secondId, 'two sessions should be alive').toBeTruthy()

    const rowTwo = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${secondId}"]`,
    )
    await expect(rowTwo).toHaveAttribute('data-selected', 'true', { timeout: 20_000 })

    // The × is hover-revealed, so hover is how it is reached — but it is a
    // real element with a role, not a chord.
    await rowTwo.hover()
    await rowTwo.getByTestId('terminal-strip-kill').click()

    // The session is gone — from the strip and from the server both — and
    // the neighbour tab holds the pane.
    await expect(rowTwo).toHaveCount(0, { timeout: 20_000 })
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '1')
    await expect
      .poll(async () => sessionIds(page), 'the DELETE should have reached the server')
      .toEqual([firstId])
    const rowOne = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${firstId}"]`,
    )
    await expect(rowOne).toHaveAttribute('data-selected', 'true')
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1163: the row's state comes from the terminal's own signals. A BEL
   * is the one that means "a person is wanted", and it must survive the
   * session being left — that is the entire reason the bit exists rather
   * than being derived from "is something running", which says the opposite
   * about an agent parked at a prompt.
   */
  test('a shell that rings the bell is marked as wanting a person (GDK-1163)', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await openPane(page)
    const [firstId] = await sessionIds(page)

    // Ring in session one, then walk away to session two. The bit is read
    // from the strip, which is where a person would read it.
    await typeLine(page, "printf '\\a'")
    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '2', {
      timeout: 20_000,
    })

    const rowOne = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${firstId}"]`,
    )
    await expect(rowOne).toHaveAttribute('data-state', 'needs', { timeout: 20_000 })

    // Going back is the answer to the question the bell asked.
    await rowOne.click()
    await expect(rowOne).not.toHaveAttribute('data-state', 'needs', { timeout: 20_000 })

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1185. The test above is that story at the speed of this machine;
   * this one holds the create open so the window is a fact rather than a
   * hope. A strip row is live the moment the *server* has the session —
   * data-count reaches 2 off the roster poll — which is before the create
   * response gets back to the pane, so a person can and will click a row
   * while the pane is still between shells.
   *
   * FAIL-first, measured 2026-08-30 against the build before the fix: the
   * row stayed `needs` for the full 20s, and the roster showed session one
   * with `attached: 0` the whole time. The pane never left it, so the
   * selection still pointed there, so clicking its row was `select()` on the
   * value already held — idempotent, no attach, nothing at all. The create
   * landing afterwards then took the pane off the row the person chose.
   */
  test('a row clicked while a new shell is still being created is not ignored (GDK-1185)', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await openPane(page)
    const [firstId] = await sessionIds(page)

    await typeLine(page, "printf '\\a'")

    // The create takes longer than the roster poll — the ordering a slower
    // machine produces on its own.
    await page.route('**/api/v1/terminal/sessions/', async (route) => {
      if (route.request().method() !== 'POST') return route.fallback()
      const res = await route.fetch()
      await new Promise((done) => setTimeout(done, 6_000))
      await route.fulfill({ response: res })
    })

    // The create's own response is the barrier the last assertion waits on:
    // "the pane was not taken back" is only a claim once the thing that would
    // have taken it has landed.
    const created = page.waitForResponse(
      (res) =>
        res.url().includes('/api/v1/terminal/sessions/') && res.request().method() === 'POST',
    )
    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '2', {
      timeout: 20_000,
    })

    const rowOne = page.locator(`[data-testid="terminal-strip-row"][data-session-id="${firstId}"]`)
    await expect(rowOne).toHaveAttribute('data-state', 'needs', { timeout: 20_000 })
    await rowOne.click()
    // The bell is answered, which can only happen by attaching…
    await expect(rowOne).not.toHaveAttribute('data-state', 'needs', { timeout: 20_000 })
    // …and the create landing afterwards leaves the person's row alone.
    await expect(rowOne).toHaveAttribute('data-selected', 'true', { timeout: 20_000 })
    await created
    await expect(rowOne).toHaveAttribute('data-selected', 'true')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1160: an issue key printed into the terminal is a link into this
   * app's own issue view — not a Jira deep link, and not a new route. The
   * key is taken from the list so the test cannot pass on a key the mirror
   * does not hold, which is also the false-positive control being exercised
   * from the other side.
   */
  test('an issue key in the output opens that issue in the app (GDK-1160)', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)

    const firstRow = page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first()
    await expect(firstRow).toBeVisible({ timeout: 20_000 })
    const key = await firstRow.getAttribute('data-issue-key')
    expect(key, 'the demo list should have an issue key').toBeTruthy()

    await openPane(page)
    // Alone on its line, so the rendered span is exactly the key and a click
    // in its middle lands inside the link's range.
    await typeLine(page, `printf '%s\\n' ${key}`)
    await expect.poll(async () => readTerm(page)).toContain(key!)

    // xterm's own screen layer sits over the glyph rows and eats pointer
    // events, so Playwright's actionability check never clears on the span.
    // Aim the real mouse at the span's box instead — which is also how a
    // person reaches an xterm link: hover to arm the linkifier, then click.
    const glyphs = page
      .locator('[data-testid="terminal-pane"] .xterm-rows span')
      .filter({ hasText: new RegExp(`^${key}$`) })
      .last()
    await expect(glyphs).toBeVisible({ timeout: 10_000 })
    const box = await glyphs.boundingBox()
    expect(box, 'the key should be rendered as its own glyph run').not.toBeNull()
    const x = box!.x + box!.width / 2
    const y = box!.y + box!.height / 2
    /*
     * The pointer arrives across a neighbouring row, not out of nowhere.
     *
     * xterm asks a link provider once per buffer line and keeps the answer
     * against that line: Linkifier._handleHover takes the cached branch
     * whenever the pointer's row equals `_activeLine`, and that branch never
     * calls provideLinks again (@xterm/xterm 6.0.0). typeLine's focus click
     * already parked the pointer on a row of the freshly opened pane — a row
     * that was empty then, so the cached answer for it is "no links". On a
     * shell that prints no startup banner the key lands on exactly that row,
     * and the click below would consult that stale answer and open nothing.
     *
     * Measured 2026-08-30 with e2e/ci-shell.sh (`npm run
     * test:e2e:wide-prompt`), which is the CI runner's shell: without this
     * first move the detail panel stayed hidden for the full 20s — the same
     * failure the Linux CI job had and macOS never did, because Apple's bash
     * prints a three-line deprecation banner that pushed the key one row
     * further down. One row up is always a different line from the key's own
     * (the echoed command line sits above it), so the hover that follows is
     * always a fresh ask.
     */
    await page.mouse.move(x, y - box!.height)
    await page.mouse.move(x, y)
    await page.mouse.click(x, y)

    const panel = page.getByTestId('issue-detail-panel')
    await expect(
      panel,
      `clicking the key should have opened it; the terminal held:\n${await readTerm(page)}`,
    ).toBeVisible({ timeout: 20_000 })
    await expect(panel).toContainText(key!, { timeout: 20_000 })

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1250: the roster chords work from inside the VT — where the pane
   * spends its whole life. The contract is the strip's own: the selection
   * lands on the tab the roster order says it should, the scrollback that
   * follows is the new session's alone, and both ends wrap.
   */
  test('Ctrl+Shift+[ / ] walk the roster from inside the VT (GDK-1250)', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await openPane(page)

    await typeLine(page, "printf 'MARKER-''ONE\\n'")
    await expect.poll(async () => readTerm(page)).toContain('MARKER-ONE')
    const [firstId] = await sessionIds(page)
    expect(firstId, 'the pane should have created a session').toBeTruthy()

    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '2', {
      timeout: 20_000,
    })
    await typeLine(page, "printf 'MARKER-''TWO\\n'")
    await expect.poll(async () => readTerm(page)).toContain('MARKER-TWO')

    const rowOne = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${firstId}"]`,
    )
    const ids = await sessionIds(page)
    const secondId = ids.find((id) => id !== firstId)
    expect(secondId, 'two sessions should be alive').toBeTruthy()
    const rowTwo = page.locator(
      `[data-testid="terminal-strip-row"][data-session-id="${secondId}"]`,
    )

    // Previous, from the pane's seat in session two: one step back the
    // roster order is session one.
    await focusTerm(page)
    await page.keyboard.press('Control+Shift+BracketLeft')
    expect(await lastKeyCmdOf(page)).toBe('terminal-prev-session')
    await expect(rowOne).toHaveAttribute('data-selected', 'true', { timeout: 20_000 })
    await expect
      .poll(async () => readTerm(page), 'the scrollback should follow the chord')
      .toContain('MARKER-ONE')
    expect(
      await readTerm(page),
      'the chord is a switch, not a splice — two histories must not merge',
    ).not.toContain('MARKER-TWO')

    // Next: forward to session two again.
    await focusTerm(page)
    await page.keyboard.press('Control+Shift+BracketRight')
    expect(await lastKeyCmdOf(page)).toBe('terminal-next-session')
    await expect(rowTwo).toHaveAttribute('data-selected', 'true', { timeout: 20_000 })
    await expect.poll(async () => readTerm(page)).toContain('MARKER-TWO')

    // Next once more wraps around to the top of the roster.
    await focusTerm(page)
    await page.keyboard.press('Control+Shift+BracketRight')
    await expect(rowOne).toHaveAttribute('data-selected', 'true', { timeout: 20_000 })
    await expect
      .poll(async () => readTerm(page), 'the wrap should land on the first session')
      .toContain('MARKER-ONE')

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1250's focus escape: Ctrl+Shift+` leaves the VT's focus without
   * closing the pane — the toggle's old conflict-fallback slot doing the
   * job it was kept for. After it, the strip's selected tab holds focus,
   * ordinary app keys work again, and Enter on that tab is the tab's own
   * click: a re-select of the session already shown, which the selector is
   * contractually idempotent about.
   */
  test('Ctrl+Shift+` moves focus to the selected tab and the pane stays (GDK-1250)', async ({
    page,
  }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)
    await openPane(page)

    const row = page.locator('[data-testid="terminal-strip-row"][data-selected="true"]')
    await expect(row).toHaveCount(1, { timeout: 20_000 })

    await focusTerm(page)
    await page.keyboard.press('Control+Shift+Backquote')

    // Focus is on the strip's selected tab, not the VT and not a close.
    await expect
      .poll(async () =>
        page.evaluate(() =>
          document.activeElement?.matches(
            '[data-testid="terminal-strip-row"][data-selected="true"]',
          ),
        ),
      )
      .toBe(true)
    await expect(page.getByTestId('terminal-pane')).toBeVisible()
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true')

    // Enter on the tab is the tab's own activation: the same select() the
    // click makes, on the id already held — nothing reattaches, nothing
    // moves, the pane is still the same session. Pressed while focus is
    // provably on the tab, before anything below can move it.
    await page.keyboard.press('Enter')
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true')
    await expect(row).toHaveAttribute('data-selected', 'true')

    // An ordinary app key answers from there: ? is chrome's again, and Esc
    // retires it — the ladder a focused VT could not offer. (The sheet has
    // its own Escape handler, so this close does not depend on focus.)
    await page.keyboard.press('?')
    await expect(page.getByTestId('shortcuts-dialog')).toBeVisible({ timeout: 20_000 })
    await page.keyboard.press('Escape')
    await expect(page.getByTestId('shortcuts-dialog')).toHaveCount(0)

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })

  /*
   * GDK-1251: Ctrl+Shift+O opens the selected session's bound issue — the
   * same verb the pane's own link activation uses (selection.select), so
   * the panel is the app's issue view, and an unbound session is a no-op,
   * not an error.
   */
  test('Ctrl+Shift+O opens the selected session’s issue (GDK-1251)', async ({ page }) => {
    test.setTimeout(120_000)
    const errors = await boot(page)

    // From the list, like the GDK-1160 test: the key is one the mirror
    // holds, so the open has somewhere to land.
    const firstRow = page.locator('[data-testid="issue-list-scroller"] [data-issue-key]').first()
    await expect(firstRow).toBeVisible({ timeout: 20_000 })
    const key = await firstRow.getAttribute('data-issue-key')
    expect(key, 'the demo list should have an issue key').toBeTruthy()

    await openPane(page)
    const [sessionId] = await sessionIds(page)
    expect(sessionId, 'the pane should have created a session').toBeTruthy()
    const row = page.locator(`[data-testid="terminal-strip-row"][data-session-id="${sessionId}"]`)
    await expect(row).toHaveAttribute('data-selected', 'true', { timeout: 20_000 })

    // Unbound: the chord resolves (the app saw it) and does nothing — no
    // detail opens over the list.
    await focusTerm(page)
    await page.keyboard.press('Control+Shift+KeyO')
    expect(await lastKeyCmdOf(page)).toBe('terminal-open-issue')
    await expect(page.locator('[data-testid="issue-layout"]')).toHaveAttribute(
      'data-detail-open',
      'false',
    )

    // The binding the strip exists for, landed the same way `gadak claim`
    // lands it. The tab carries it as an attribute — the read side of the
    // same contract the chord dispatches from.
    await page.request.post(apiURL(`/api/v1/terminal/sessions/${sessionId}/issue/`), {
      data: { issue_key: key! },
    })
    await expect(row).toHaveAttribute('data-issue-key', key!, { timeout: 20_000 })

    await focusTerm(page)
    await page.keyboard.press('Control+Shift+KeyO')
    const panel = page.getByTestId('issue-detail-panel')
    await expect(
      panel,
      'the bound session’s issue should have opened; the terminal held:\n' +
        (await readTerm(page)),
    ).toBeVisible({ timeout: 20_000 })
    await expect(panel).toContainText(key!, { timeout: 20_000 })
    await expect(page.getByTestId('terminal-pane')).toBeVisible()

    expect(appConsoleErrors(errors), `console errors:\n${errors.join('\n')}`).toEqual([])
  })
})

const SHOT_DIR = join(
  dirname(fileURLToPath(import.meta.url)),
  '..',
  'scratch',
  'terminal-strip-shots',
)

/*
 * The strip's own captures, next to the pane's (terminal.spec.ts). The three
 * counts are the ones the design is decided by: none, one, several. Zero is
 * the failure path this feature has — a person who runs no agents must not
 * be handed an empty table — and one is where the resident row (GDK-1199)
 * has to earn its keep with a single named tab.
 */
test.describe('terminal strip shots', () => {
  test.use({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 })

  test.beforeEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })
  test.afterEach(async ({ page }) => {
    await drainTerminalSessions(page)
  })

  test('capture none, one, three', async ({ page }) => {
    test.setTimeout(180_000)
    mkdirSync(SHOT_DIR, { recursive: true })
    const hash = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim()
    const dirty = execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim()
    const source = dirty ? `${hash} + uncommitted edits (${dirty.split('\n').length} files)` : hash
    const shoot = (name: string) => page.screenshot({ path: join(SHOT_DIR, name), fullPage: false })

    await boot(page)
    await openPane(page)
    await typeLine(page, "printf 'one ses''sion\\n'")
    await expect.poll(async () => readTerm(page)).toContain('one session')
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '1', {
      timeout: 20_000,
    })
    await shoot('01-one.png')

    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '2', {
      timeout: 20_000,
    })
    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toHaveAttribute('data-count', '3', {
      timeout: 20_000,
    })
    await typeLine(page, "printf 'thi''rd\\n'")
    await expect.poll(async () => readTerm(page)).toContain('third')

    // The headline the strip exists for: a row named after its ticket. The
    // binding is the loopback POST `gadak claim` already makes when a claim
    // lands (cmd/gadak/agent.go, postTerminalIssueBinding) — the capture
    // stands in for the CLI, not for the mechanism.
    const claimed = (await sessionIds(page))[0]
    await page.request.post(apiURL(`/api/v1/terminal/sessions/${claimed}/issue/`), {
      data: { issue_key: 'NMA-140' },
    })
    await expect(
      page.locator(`[data-testid="terminal-strip-row"][data-session-id="${claimed}"]`),
    ).toContainText('NMA-140', { timeout: 20_000 })
    await shoot('02-three.png')

    // Zero: the last shell exits and the strip becomes the one thing worth
    // offering there. Drain first so no other session is left standing.
    await drainTerminalSessions(page)
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveCount(0)
    await page.keyboard.press('Control+Backquote')
    await expect(page.getByTestId('terminal-pane')).toHaveAttribute('data-attached', 'true', {
      timeout: 20_000,
    })
    await focusTerm(page)
    await page.keyboard.press('Control+C')
    await typeLine(page, 'exit')
    await expect(page.getByTestId('terminal-status')).toContainText('Shell exited', {
      timeout: 20_000,
    })
    await expect(page.getByTestId('terminal-strip-start')).toBeVisible({ timeout: 20_000 })
    await shoot('03-none.png')

    writeFileSync(
      join(SHOT_DIR, 'MANIFEST.md'),
      [
        '# terminal session strip captures (GDK-1153 / GDK-1163)',
        '',
        `- source: \`${source}\``,
        '- viewport: 1440×900, deviceScaleFactor 2',
        '',
        '| file | notes |',
        '| --- | --- |',
        '| `01-one.png` | one session: a single named tab in the one chrome row |',
        '| `02-three.png` | three sessions: the strip is the selector |',
        '| `03-none.png` | no session: the row list is the start action |',
        '',
      ].join('\n'),
      'utf8',
    )
  })
})
