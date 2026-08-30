/**
 * The 0.19 round-trip take (GDK-1159) — scratch/roundtrip/take1.webm.
 *
 * G1, the sentence the film has to say without the product's name:
 *   **A command typed in the terminal moves the card on the board, right
 *   there — the shell and the tracker are one screen.**
 *
 * One issue, one shell, one window, four beats and no feature walk:
 *
 *   1. THE MONEY SHOT. The board and the terminal pane are in one frame.
 *      `gadak claim STD-7` goes in, Enter, and in the same breath three
 *      things move: the card leaves the New group for In progress, the shell
 *      prints `bound to session …`, and the session strip row stops being a
 *      hex id and starts being `STD-7`. No cut between the Enter and the
 *      card — the causality IS the argument, so it has to survive in one
 *      continuous frame.
 *   2. THE RETURN LEG. STD-7's own body carries a command. The ▶ beside it
 *      places that line at the prompt of the shell bound to this issue — it
 *      does not run it — and a person presses Enter. The tracker hands work
 *      to the shell; beat 1 was the shell handing work to the tracker.
 *   3. THE LINK BACK. The keys in that output are underlined, and clicking
 *      one opens that issue — the third leg of "one screen": the shell wrote
 *      to the tracker, the tracker wrote to the shell, and now the shell's
 *      own output is a way back into the tracker.
 *   4. `gadak close STD-7`, and the card lands in Done.
 *
 * ── Why every wait here is a named constant and not a guess ───────────────
 *
 * Three different clocks decide when a beat is filmable, and all three were
 * measured in the tree rather than assumed:
 *
 *   UI_FOCUS_POLL_MS 500 — the board learns a write happened on the ui-focus
 *     poll (web/src/App.svelte:334-345 → issues.refresh()), keyed on the
 *     mirror's WAL version (internal/server/focus.go:60). So "instantly" has
 *     a floor: the card cannot move faster than the next tick. The take
 *     records the real elapsed ms between Enter and the row changing group
 *     (mark `card_moved`), so the cut can be judged on a measurement instead
 *     of on a feeling. e2e/mirror-instant.spec.ts asserts the same path
 *     end-to-end under 3s.
 *   SHELL_POLL_MS 4000 — web/src/lib/issue-shells.svelte.ts:25, and it only
 *     runs while a drawing surface is mounted (shells.track() in
 *     DetailPanel.svelte:68). Until that poll comes back the ▶ is in the DOM
 *     and inert (AdfContent.svelte:90 `if (!shell) return`). Beat 2 therefore
 *     waits for `data-shell="attached"` on the body rather than for a
 *     timeout — a click filmed against an inert button is a beat that shows
 *     nothing happening.
 *   ROSTER_POLL_MS 2000 — web/src/lib/terminal/sessions.svelte.ts:27, which
 *     is what renames the strip row (sessionLabel prefers issue_key,
 *     web/src/lib/terminal/strip.ts:68).
 *
 * ── Why a second shell is opened before the money shot ────────────────────
 *
 * Not staging: the strip hides itself when there is exactly one session
 * (`stripShowsRows: count !== 1`, strip.ts:133), so with a single shell the
 * rename beat is not merely subtle, it is not rendered at all. Opening a
 * second shell (the pane's own `terminal-new` button) is the only way the
 * strip exists to be renamed — and it improves the argument rather than
 * padding it: two anonymous shells, and the one that claimed the issue takes
 * the issue's name.
 *
 * ── The fixture ──────────────────────────────────────────────────────────
 *
 * record-roundtrip.sh owns it. The workspace is the standalone one
 * seed_hero_home() builds (record-hero-desk.sh --serve-only) — writable,
 * credential-free, fictional (MEDIA.md) — with STD-7's description replaced
 * afterwards — `gadak edit -m` with a fence, the way any agent would write it
 * — so the shared seed is untouched. No live model anywhere in this take:
 * `gadak claim` and `gadak close` are ordinary CLI writes, which is why a
 * retake costs seconds and the choreography can be tuned frame by frame.
 *
 * ── What / how ───────────────────────────────────────────────────────────
 *
 * This file is the CHOREOGRAPHY — which beat happens when, what has to be
 * true before the camera moves on, and where the marks land. It names no
 * selectors: every "which pixels say so" question lives in
 * ./roundtrip-surface.ts, so the day the kanban board lands (GDK-761) the
 * climax is re-shot by swapping that one module. Seeding, recording and the
 * cut pipeline (record-roundtrip.sh / cut-roundtrip.sh) are the third,
 * already-separate piece and do not move either.
 *
 * Marks: every beat appends {mark, epoch_ms, note} to $GADAK_RT_PROOF (JSONL);
 * record-roundtrip.sh turns them into video-relative seconds for the keyframes
 * and cut-roundtrip.sh reads the same file for its cut list.
 */
import { test, expect, type Page } from '@playwright/test'
import { appendFileSync, rmSync, writeFileSync } from 'node:fs'
import { forceLocale, readTerm } from '../helpers'
import {
  BOARD_ROUTE,
  PANE_WIDTH,
  boardReady,
  card,
  categoryOf,
  columnOf,
  movedByOther,
  revealDone,
} from './roundtrip-surface'

const isMedia = !!process.env.GADAK_MEDIA
const TARGET_KEY = process.env.GADAK_RT_TARGET || 'STD-7'
const PROOF = process.env.GADAK_RT_PROOF || ''


function mark(name: string, note = ''): void {
  if (!PROOF) return
  appendFileSync(PROOF, `${JSON.stringify({ mark: name, epoch_ms: Date.now(), note })}\n`)
}

/** Pause between beats so a human can read the frame. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/** Focus the pane the way the e2e suite does: click the host, then focus the
 *  helper textarea explicitly. The renderer paints on a canvas, so the click
 *  alone lands on something that cannot hold a caret. (Copied from
 *  hero-desk.spec.ts, which shares the pane — keep the two in step.) */
async function focusPane(page: Page): Promise<void> {
  const pane = page.getByTestId('terminal-pane')
  const host = pane.locator('[data-gadak-editable]')
  if (await host.count()) {
    await host.first().click({ position: { x: 24, y: 24 } })
  } else {
    await pane.click({ position: { x: 24, y: 24 } })
  }
  await page.evaluate(() => {
    document
      .querySelector<HTMLTextAreaElement>('[data-testid="terminal-pane"] textarea')
      ?.focus()
  })
}

/** Type a line at the shell prompt and press Enter, returning the epoch ms of
 *  the Enter itself — the zero point every "did it move yet" measurement in
 *  this take is relative to. The gap before Enter is deliberate: it is the
 *  frame where a viewer reads the command, and it is also what the tapes
 *  learned about letting a re-render settle before the key lands. */
async function runLine(page: Page, line: string, fast = false): Promise<number> {
  await focusPane(page)
  // The pane has just been clicked and the helper textarea focused in the
  // same tick; typing immediately loses the first keystroke. Measured
  // 2026-08-30: a take typed `adak close STD-7` and the shell answered
  // `sh: adak: command not found` on camera — the beat looked like the
  // product failing. So: settle, type, and then *read the prompt back*
  // before committing. A line that did not land whole is retyped once
  // rather than sent, because Enter is the one key this take cannot take
  // back.
  await beat(page, fast ? 200 : 350)
  // `fast` is for lines the cut never shows — the crew's armed commands are
  // ~120 characters each and six of them at reading speed added 40s to every
  // take. The money shot's own line stays slow: that one is read on camera.
  const flat = (s: string) => s.replace(/\s+/g, '')
  for (let attempt = 0; attempt < 2; attempt++) {
    await page.keyboard.type(line, { delay: fast ? 8 : 60 })
    await beat(page, fast ? 350 : 800)
    if (flat(await readTerm(page)).includes(flat(line))) break
    // Clear whatever partial line is sitting there (Ctrl-U) and try again.
    await page.keyboard.press('Control+u')
    await beat(page, 300)
  }
  expect(flat(await readTerm(page)), `"${line}" never landed at the prompt`)
    .toContain(flat(line))
  await beat(page, fast ? 250 : 800)
  const at = Date.now()
  await page.keyboard.press('Enter')
  return at
}

/**
 * Click an issue key printed in the terminal, the way a person does.
 *
 * xterm paints on a canvas, so there is no element to click: the link lives
 * at a cell coordinate. This finds the key in the buffer, converts (col,row)
 * to viewport pixels through the renderer's own measured cell size, and
 * clicks there. Returns false — never throws — when the key is not on screen
 * or the geometry cannot be read, because this beat is the optional one and
 * a hunt on camera is worse than a missing second.
 *
 * GDK-1160 is the link provider (web/src/lib/terminal/renderer.ts:278-303);
 * only keys whose project prefix the mirror knows are linkified
 * (issue-links.ts:38), which is why the fixture's STD keys work and nothing
 * else in the frame does. The mouse is moved in a separate step first: a link
 * is hover-activated, and a click delivered to a cell the pointer never
 * entered lands on a link that was never built.
 */
async function clickTerminalKey(page: Page, key: string): Promise<boolean> {
  const pt = await page.evaluate((k) => {
    const t = (
      window as unknown as {
        __gadakTerm?: {
          rows: number
          buffer: {
            active: {
              length: number
              viewportY: number
              getLine: (y: number) => { translateToString: (t?: boolean) => string } | undefined
            }
          }
        }
      }
    ).__gadakTerm
    const screen = document.querySelector<HTMLElement>(
      '[data-testid="terminal-pane"] .xterm-screen',
    )
    if (!t || !screen) return null
    const buf = t.buffer.active
    const top = buf.viewportY
    for (let row = t.rows - 1; row >= 0; row--) {
      const text = buf.getLine(top + row)?.translateToString(true) ?? ''
      const col = text.indexOf(k)
      if (col < 0) continue
      const box = screen.getBoundingClientRect()
      const cols = (t as unknown as { cols?: number }).cols || 80
      const w = box.width / cols
      const h = box.height / t.rows
      return {
        x: Math.round(box.left + (col + k.length / 2) * w),
        y: Math.round(box.top + (row + 0.5) * h),
      }
    }
    return null
  }, key)
  if (!pt) return false
  // Two moves: the first enters the cell and lets the provider build the
  // link, the second is the tiny motion a resting row needs before the click
  // registers at all.
  await page.mouse.move(pt.x, pt.y)
  await beat(page, 500)
  await page.mouse.move(pt.x + 1, pt.y)
  await beat(page, 700)
  await page.mouse.click(pt.x + 1, pt.y)
  return true
}

test.describe('roundtrip demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('a command in the terminal moves the card on the board', async ({ page }) => {
    await page.addInitScript((w) => {
      try {
        localStorage.setItem('gadak.terminal.width', w)
      } catch {
        /* private mode */
      }
    }, PANE_WIDTH)
    await forceLocale(page, 'en')

    // ── The board at rest ────────────────────────────────────────────────
    if (PROOF) mark('start')
    await page.goto(BOARD_ROUTE)
    await boardReady(page)
    await expect(card(page, TARGET_KEY)).toBeVisible()
    // The premise of the shot: the card starts outside In progress.
    expect(await categoryOf(page, TARGET_KEY)).toBe('new')
    mark('board_at_rest')
    await beat(page, 1800)

    // ── The terminal arrives in the same window ──────────────────────────
    // ⌘K rather than the chord: the palette is the discovery path a viewer
    // with no prior knowledge can follow (G4).
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await beat(page, 500)
    await page.keyboard.type('terminal', { delay: 55 })
    await expect(palette.getByTestId('palette-action-terminal')).toBeVisible()
    await beat(page, 600)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toBeVisible()
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    // The beta mark came off in 0.19 (GDK-1024) — asserted absent so a revert
    // cannot quietly put it back into a recording.
    await expect(page.getByTestId('terminal-beta')).toHaveCount(0)
    mark('pane_open')
    await beat(page, 1200)

    // A second shell, so the strip exists to be renamed (see the header).
    await page.getByTestId('terminal-new').click()
    await expect(page.getByTestId('terminal-strip')).toBeVisible({ timeout: 15_000 })
    await expect(page.getByTestId('terminal-strip-row')).toHaveCount(2, { timeout: 15_000 })
    // Both rows are anonymous right now — that is the "before" the rename
    // beat needs, and asserting it means a take where one was already named
    // cannot pass as a rename.
    const namesBefore = await page.getByTestId('terminal-strip-name').allTextContents()
    expect(namesBefore.some((n) => n.includes(TARGET_KEY))).toBe(false)
    mark('strip_two_shells', `names=${JSON.stringify(namesBefore)}`)
    await beat(page, 900)

    // One real question before the claim, for two reasons. It gives the claim
    // a motive a stranger can follow — *these* are open, so I take one — and
    // it fills the pane: an empty 828px-tall terminal beside a full board is
    // half a frame of nothing, which is what the first board take looked
    // like. Two narrow columns on purpose; `gadak list`'s ~95-character table
    // wraps three times per row at this width and reads as noise.
    await runLine(page, `gadak sql "select key, status from issues_full where status_category='new'"`)
    await expect
      .poll(async () => readTerm(page), { timeout: 20_000, intervals: [300] })
      .toContain(TARGET_KEY)
    mark('queue_listed')
    await beat(page, 1700)

    // ── BEAT 1: the money shot ───────────────────────────────────────────
    const enterAt = await runLine(page, `gadak claim ${TARGET_KEY}`)
    mark('claim_enter')

    // The card. Polled tightly (200ms) because the number this take exists
    // to produce is *how long it took*, and a 2s poll would report the poll.
    await expect
      .poll(async () => columnOf(page, TARGET_KEY), { timeout: 20_000, intervals: [200] })
      .toBe('inprogress')
    const moveMs = Date.now() - enterAt
    mark('card_moved', `ms_after_enter=${moveMs}`)
    // The mirror-side truth, not the header text.
    expect(await categoryOf(page, TARGET_KEY)).toBe('inprogress')

    // The shell said so too, and the strip row took the issue's name.
    await expect
      .poll(async () => readTerm(page), { timeout: 15_000, intervals: [300] })
      .toContain('bound to session')
    mark('shell_bound')
    await expect
      .poll(async () => page.getByTestId('terminal-strip-name').allTextContents(),
        { timeout: 15_000, intervals: [300] })
      .toEqual(expect.arrayContaining([expect.stringContaining(TARGET_KEY)]))
    mark('strip_renamed')
    // The hold the cut lands on: all three changes are on screen at once.
    await beat(page, 2600)
    mark('moneyshot_hold')

    // ── BEAT 2: the return leg ───────────────────────────────────────────
    await card(page, TARGET_KEY).click()
    const body = page.getByTestId('issue-body')
    await expect(body).toBeVisible({ timeout: 20_000 })
    mark('detail_open')
    // Wait for the binding to be *seen*, not for a stopwatch: SHELL_POLL_MS
    // is 4s and the poll only starts when this panel mounts.
    await expect(body).toHaveAttribute('data-shell', 'attached', { timeout: 20_000 })
    mark('shell_attached_seen')
    await beat(page, 1200)

    const run = page.locator('[data-run-command]').first()
    await expect(run).toBeVisible()
    const command = (await run.getAttribute('data-run-command')) ?? ''
    expect(command.length).toBeGreaterThan(0)
    await run.click()
    // Placed, not run — the server refuses any payload with a newline
    // (internal/server/terminal.go:471), so this is a contract, not a
    // convention. The frame has to show the line sitting at the prompt.
    // Compared against the buffer with its line breaks removed. A 320px pane
    // is ~35 columns and the command is longer than that, so xterm wraps it
    // across buffer rows — the first take failed this assertion on a command
    // that was sitting at the prompt perfectly well. What is being proved is
    // that the text arrived, not how the renderer folded it.
    const flat = (s: string) => s.replace(/[\s ]+/g, '')
    await expect
      .poll(async () => flat(await readTerm(page)), { timeout: 15_000, intervals: [300] })
      .toContain(flat(command))
    mark('command_placed')
    await beat(page, 1800)

    // A person presses Enter. That is the beat: the tool put the line there,
    // the human decided to run it.
    await focusPane(page)
    await page.keyboard.press('Enter')
    mark('command_run')
    // The gate has to be output only *this* command produces. `STD-7` is
    // already in the buffer twice from beat 1 (the claim and its reply), so
    // "two issue keys are on screen" would pass the moment Enter was pressed
    // and film an empty result. A key that is not the target is the evidence.
    await expect
      .poll(async () => {
        const t = await readTerm(page)
        return (t.match(/\bSTD-\d+\b/g) ?? []).some((k) => k !== TARGET_KEY)
      }, { timeout: 20_000, intervals: [300] })
      .toBe(true)
    mark('command_output')
    await beat(page, 1600)

    // ── BEAT 3: the link back ────────────────────────────────────────────
    // Whichever key the output actually printed — never a guess. The target
    // is excluded so the click demonstrably *changes* the open issue.
    //
    // This beat was cut once: linkify linked nothing in a standalone
    // workspace because knownProjectKeys() found neither config().projects
    // nor any issue's source_project (GDK-1177, now fixed). It is still
    // wrapped in a try: the beat is decoration, and a hunt on camera is worse
    // than a missing second.
    const term = await readTerm(page)
    const other = (term.match(/\bSTD-\d+\b/g) ?? []).find((k) => k !== TARGET_KEY)
    let linked = false
    if (other) {
      linked = await clickTerminalKey(page, other)
      if (linked) {
        try {
          // Which issue the panel is on, asked of the panel: selection.select
          // routes to panel.show('issue', key) and the panel store touches no
          // URL at all (web/src/stores/panel.svelte.ts).
          await expect(page.getByTestId('issue-detail-panel'))
            .toContainText(other, { timeout: 5_000 })
          mark('linkify_opened', `key=${other}`)
          await beat(page, 1700)
        } catch {
          linked = false
        }
      }
    }
    if (!linked) mark('linkify_skipped', `candidate=${other ?? 'none'}`)

    // ── BEAT 4: close, and the card crosses to Done ─────────────────────
    // Close the detail panel first: while it is docked the pane is squeezed
    // to its 320px minimum, and the rest of this film is the board. Its own
    // close button, not Escape: measured, Escape left the panel open and a
    // take ended on a squeezed stage. In-app only, never a reload — that
    // would drop every session.
    await page.getByTestId('issue-detail-close').click()
    await expect(page.getByTestId('issue-body')).toHaveCount(0, { timeout: 10_000 })
    await boardReady(page)
    await beat(page, 1200)

    const closeAt = await runLine(page, `gadak close ${TARGET_KEY}`)
    mark('close_enter')
    await expect
      .poll(async () => columnOf(page, TARGET_KEY), { timeout: 20_000, intervals: [200] })
      .toBe('done')
    mark('card_done', `ms_after_enter=${Date.now() - closeAt}`)
    expect(await categoryOf(page, TARGET_KEY)).toBe('done')
    await revealDone(page)
    await expect(card(page, TARGET_KEY)).toBeInViewport({ ratio: 0.9 })
    mark('done_revealed')
    await beat(page, 1600)

    // ── BEAT 6: the issue's own shell, found by its name ─────────────────
    // Two shells: one anonymous, one wearing STD-7 because it claimed it.
    // Click away, then click the named row — the pane swaps and the server
    // replays that session's scrollback, so the work done on this issue is
    // back on screen. This is the film's point in one gesture: you do not
    // hunt a tab, you pick the issue.
    const rows = page.getByTestId('terminal-strip-row')
    await rows.filter({ hasNotText: TARGET_KEY }).first().click()
    await beat(page, 1400)
    mark('session_away')
    await rows.filter({ hasText: TARGET_KEY }).first().click()
    await expect
      .poll(async () => readTerm(page), { timeout: 15_000, intervals: [300] })
      .toContain(`claim ${TARGET_KEY}`)
    mark('session_pick')
    await beat(page, 2200)
    await beat(page, 1600)

    // ── BEAT 5: THE CLIMAX — the hand leaves and the board keeps moving ──
    //
    // Three shells, three actors, three cards crossing on their own. What
    // makes this honest rather than a mockup is that nothing here is
    // simulated: each write is a real `gadak close` run by a real shell, and
    // the shell runs it because a `sleep` planted earlier expired — not
    // because the camera asked at that moment.
    //
    // Why sleeps and not three CLI calls from the recorder: the film claims
    // the RIGHT side caused the LEFT side. A write fired from the record
    // script would move the cards while the pane sat frozen, and the strip
    // dots — the only thing that says "three agents are alive" — would never
    // light. There is no way to press Enter in a shell that is not selected
    // (the input API refuses newlines, internal/server/terminal.go:471, the
    // same contract that makes ▶ place-but-not-run), so the command has to be
    // armed while the shell IS selected and fire itself later.
    //
    // The volley is SEQUENTIAL, 150ms apart. Concurrent writes are still
    // broken (GDK-1179: two of three come back exit 0 with the mirror
    // refresh failed on `UNIQUE constraint failed: changelog.id`, and only
    // one card ever moves). 150ms × 3 = 450ms, still inside one 500ms
    // ui-focus tick, so the board can still show all three moving on one
    // frame — measured spread 0ms for a serial trio.
    const CREW: Array<{ key: string; actor: string }> = [
      { key: 'STD-15', actor: 'claude:a3f10c2b|Claude Code' },
      { key: 'STD-13', actor: 'claude:7b2e9d14|Claude Code' },
      { key: 'STD-12', actor: 'claude:c19f4a06|Claude Code' },
    ]

    // The starting pistol is a file, not a stopwatch.
    //
    // The first version armed each shell with `sleep <computed>` so the three
    // would expire together. It does not survive contact: arming a shell —
    // new session, claim, wait for the card, type the armed line — measured
    // 8 seconds, so by the time the third was armed the first two sleeps had
    // long expired and fired one at a time, eight seconds apart. There was no
    // volley to film (proof-take: `volley_landed t_after_arm_ms=31916`,
    // `data_moved_seen=0`).
    //
    // Waiting on a file instead decouples the fire time from the arming time
    // completely: the shells spin on a path that does not exist yet, the spec
    // creates it once every shell is armed, and they all wake inside one
    // 50ms poll. The per-member `sleep` after the wait is the 150ms stagger —
    // concurrent writes are still broken (GDK-1179), and 3 × 150ms = 450ms
    // still fits one 500ms ui-focus tick, which is what lets the board show
    // the cards moving on a single frame.
    const GO = `${process.env.GADAK_RT_GO || '/private/tmp/gadak-hero-desk/go'}-${Date.now()}`
    const VOLLEY_GAP_S = 0.15

    // Claim first, arm second — the order is what makes the bridge filmable.
    //
    // The strip is three rows tall (max-h-24) and scrolls, so a fourth
    // session pushes a crew member out of frame. There is no close verb on
    // the client (GDK-922), so the way to exactly three rows is to not make a
    // fourth: crew member 0 takes over the person's own shell and member 1
    // takes the one beat B opened. `claim` rebinds a session, so the row that
    // said STD-7 simply starts saying STD-15. Nothing is left over and no
    // ghost row is ever on screen.
    for (const [i, member] of CREW.entries()) {
      if (i < 2) {
        await page.locator('[data-testid="terminal-strip-row"]').nth(i).click()
      } else {
        await page.getByTestId('terminal-new').click()
        await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
      }
      await beat(page, 500)
      // Identity first, in its own pass over the crew. A blind reviewer read
      // an on-camera `GADAK_ACTOR='claude:…' gadak claim …` as a person
      // LABELLING himself an agent — the opposite of what the film claims.
      // Setting it inline fixed the claim line but left `export GADAK_…`
      // being typed inside the bridge window instead (measured, twice). So
      // every shell gets its identity here, before any of them claims
      // anything, and `clear` takes the line with it. The actor still reaches
      // the write; its evidence stays where it belongs — the row's rename and
      // the chip that appears on the card.
      await runLine(page, `export GADAK_ACTOR='${member.actor}'; clear`, true)
    }

    // Now the claims, and the bridge opens one claim before the last: the
    // camera catches the renames themselves — a row stops being a hex id and
    // starts being an issue key, twice, ~2s apart because that is the roster
    // cadence (GDK-1182). Before this, three named rows appeared with no
    // visible cause and a blind reviewer asked who had renamed them.
    for (const [i, member] of CREW.entries()) {
      await page.locator('[data-testid="terminal-strip-row"]').nth(i).click()
      await beat(page, 500)
      if (i === CREW.length - 2) mark('bridge_in')
      await runLine(page, `gadak claim ${member.key}`, true)
      await beat(page, 500)
    }

    await expect
      .poll(async () => page.getByTestId('terminal-strip-name').allTextContents(),
        { timeout: 15_000, intervals: [300] })
      .toEqual(CREW.map((m) => expect.stringContaining(m.key)))
    for (const member of CREW) {
      expect(await categoryOf(page, member.key)).toBe('inprogress')
    }
    mark('crew_claimed')

    // ── THE BRIDGE ───────────────────────────────────────────────────────
    // Three shells wearing three issue keys, their dots still lit from the
    // claims that just ran, beside a board that has not moved yet. This
    // roster is the climax's CAUSE, and without it a blind reviewer could
    // only infer the cause from the end card — release-video.md's G3 failure
    // ("the argument lives off-frame"). It is filmed BEFORE the shells are
    // armed, which is why no `while [ ! -f … ]` scaffolding is in frame and
    // why the dots still read running: RUNNING_WINDOW_MS is 6s from the last
    // output, and a shell blocked on a silent wait loop goes quiet (measured
    // — the first bridge take filmed three "Quiet" rows).
    //
    // The tail hold is short now: the beat's own middle is two rows flipping
    // from hex ids to issue keys, so it no longer has to sit still long
    // enough for the 2s roster cadence (GDK-1182) to be trusted — the flips
    // ARE the evidence.
    await beat(page, 900)
    mark('bridge')

    // Now arm them, off camera as far as the cut is concerned: the segment
    // after this one opens at the starting pistol.
    for (const [i, member] of CREW.entries()) {
      await page.locator('[data-testid="terminal-strip-row"]').nth(i).click()
      await beat(page, 400)
      await runLine(
        page,
        `while [ ! -f ${GO} ]; do sleep 0.05; done; clear; ` +
          `sleep ${(i * VOLLEY_GAP_S).toFixed(2)}; ` +
          `gadak close ${member.key}`,
        true,
      )
      mark(`armed_${member.key}`, `actor=${member.actor}`)
    }
    mark('crew_armed')

    // Nothing but the board and the pane in the last shot. A take shipped
    // with the STD-3 detail panel still docked through the whole climax,
    // covering the Done column the three cards were flying into — beat 4
    // closes the panel, and something between there and here re-opened it.
    // Rather than hunt the re-open, the frame states its own requirement:
    // if a panel is on screen at the pistol, close it and prove it is gone.
    const detailClose = page.getByTestId('issue-detail-close')
    if (await detailClose.count()) {
      await detailClose.first().click()
    }
    // The panel's own frame (`issue-detail-panel`) is always mounted, empty
    // or not — measured, asserting on it fails on a closed panel. The close
    // button only exists while something is open, so it is the honest gate.
    await expect(detailClose).toHaveCount(0, { timeout: 10_000 })
    await beat(page, 600)

    // Stay on the last crew shell, so the volley's own output prints on
    // camera in F. Its armed line clears itself the instant the pistol
    // fires, so nothing of the rig survives into that segment.

    // Watch for the board's own proof that a move came from outside this
    // browser: BoardView stamps `data-moved="1"` for LANDED_MS on a card the
    // mirror tick moved, and only then does it fly and glow the actor chip
    // (BoardView.svelte:130-139). The marker is short-lived, so it is
    // observed continuously rather than sampled once after the fact — the
    // first climax take read 0 purely because it looked afterwards.
    await page.evaluate((keys) => {
      const w = window as unknown as { __flew?: Set<string> }
      w.__flew = new Set<string>()
      new MutationObserver(() => {
        for (const k of keys) {
          const el = document.querySelector(`[data-board-key="${k}"]`)
          if (el instanceof HTMLElement && el.dataset.moved === '1') w.__flew!.add(k)
        }
      }).observe(document.body, { subtree: true, attributes: true, attributeFilter: ['data-moved'] })
    }, CREW.map((m) => m.key))

    // The hand leaves. The pointer parks off every interactive thing and
    // nothing is clicked or typed again — the only input from here on is the
    // starting pistol, which is a file the shells are already waiting on.
    //
    // What prints next in this pane is written by the shell itself — and it
    // prints alone, because the armed line clears the screen the moment the
    // pistol fires. That `clear` is inside the armed command on purpose: an
    // earlier take typed it as a separate line and the shell, still blocked
    // on the wait, buffered it as stdin and ran it AFTER the close printed,
    // wiping the one output the climax exists to show. The cut starts this
    // segment just after the clear, so the scaffolding — a while-loop and a
    // temp path — never reaches a frame.
    await page.mouse.move(1200, 820)
    await beat(page, 1400)
    mark('hands_off')

    const goAt = Date.now()
    writeFileSync(GO, '')
    for (const member of CREW) {
      await expect
        .poll(async () => columnOf(page, member.key), { timeout: 30_000, intervals: [120] })
        .toBe('done')
    }
    mark('volley_landed', `ms_after_go=${Date.now() - goAt}`)
    const flew = await page.evaluate(
      () => [...((window as unknown as { __flew?: Set<string> }).__flew ?? [])],
    )
    // A note, never a gate: the film is not rejected because a 260ms marker
    // was missed, but a take where none of the three flew is a take whose
    // climax the cut should not use, and this is how that is known.
    mark('flight_marker', `flew=${JSON.stringify(flew)}`)
    for (const member of CREW) {
      expect(await categoryOf(page, member.key)).toBe('done')
    }
    rmSync(GO, { force: true })
    await beat(page, 2200)
    mark('end_frame')
    await beat(page, 2400)
    mark('end')
  })
})
