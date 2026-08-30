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
 *   3. THE LINK BACK — cut. The keys in that output were to be underlined and
 *      clickable; in a standalone workspace they are not (GDK-1177), and a
 *      thing that is not there is not a thing to film. Kept behind
 *      GADAK_RT_LINKIFY so the beat comes back the day the defect does not.
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
 * afterwards so the shared seed is untouched. That replacement goes through
 * the origin, not the mirror (`gadak api PUT`), because a real ADF codeBlock
 * is the only thing that renders a ▶ and the CLI's `-m` cannot make one:
 * plain-text ``` fences come back as literal paragraphs (measured — see the
 * script). No live model anywhere in this take: `gadak claim` and `gadak
 * close` are ordinary CLI writes, which is why a retake costs seconds.
 *
 * Marks: every beat appends {mark, epoch_ms, note} to $GADAK_RT_PROOF (JSONL);
 * record-roundtrip.sh turns them into video-relative seconds for the keyframes
 * and cut-roundtrip.sh reads the same file for its cut list.
 */
import { test, expect, type Page } from '@playwright/test'
import { appendFileSync } from 'node:fs'
import { forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA
const TARGET_KEY = process.env.GADAK_RT_TARGET || 'STD-7'
const PROOF = process.env.GADAK_RT_PROOF || ''

/** Pane width for the take, and only for the beats where the layout lets it
 *  have one: 520px is where the money shot's own lines (`gadak claim STD-7`,
 *  `bound to session …`) fit without wrapping at 15px while the list keeps
 *  648px — enough to print a summary instead of an ellipsis. From beat 2 on,
 *  with the detail panel docked at 1440, the layout hands the pane its
 *  TERMINAL_MIN_WIDTH_PX floor of 320 no matter what is stored here (measured;
 *  see roundtrip.config.ts), which is what the fixture's one-column command
 *  is chosen around. */
const PANE_WIDTH = '520'

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

/** The whole terminal buffer, as text. */
async function readTerm(page: Page): Promise<string> {
  return page.evaluate(() => {
    const t = (
      window as unknown as {
        __gadakTerm?: {
          buffer: {
            active: {
              length: number
              getLine: (y: number) => { translateToString: (t?: boolean) => string } | undefined
            }
          }
        }
      }
    ).__gadakTerm
    if (!t) return ''
    const buf = t.buffer.active
    const lines: string[] = []
    for (let i = 0; i < buf.length; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? '')
    }
    return lines.join('\n')
  })
}

/** Where the target row sits: the label of the nearest group header above it,
 *  by *visual* order (bounding boxes, not DOM order — the virtual scroller is
 *  free to nest). null = row not rendered. Copied from hero-desk.spec.ts. */
async function groupOfTarget(page: Page, key: string): Promise<string | null> {
  return page.evaluate((k) => {
    const row = document.querySelector<HTMLElement>(`[data-issue-key="${k}"]`)
    if (!row) return null
    const rowTop = row.getBoundingClientRect().top
    const headers = Array.from(
      document.querySelectorAll<HTMLElement>('[data-testid="group-header"]'),
    )
      .map((h) => ({
        label: (h.querySelector('span.uppercase')?.textContent ?? '').trim(),
        top: h.getBoundingClientRect().top,
      }))
      .filter((h) => h.label !== '' && h.top < rowTop)
      .sort((a, b) => b.top - a.top)
    return headers[0]?.label ?? null
  }, key)
}

/** The target's status_category straight off the bootstrap API. The
 *  display-name-free half of every gate here — a group header's text is what
 *  the camera sees, this is what is true (CLAUDE.md: never key on a display
 *  name). */
async function targetCategory(page: Page, key: string): Promise<string | null> {
  const res = await page.request.get('/api/v1/issues/bootstrap/')
  expect(res.ok(), `bootstrap GET: ${res.status()}`).toBeTruthy()
  const body = (await res.json()) as {
    issues?: Array<{ key?: string; status_category?: string }>
  }
  return (body.issues ?? []).find((i) => i.key === key)?.status_category ?? null
}

/** Type a line at the shell prompt and press Enter, returning the epoch ms of
 *  the Enter itself — the zero point every "did it move yet" measurement in
 *  this take is relative to. The gap before Enter is deliberate: it is the
 *  frame where a viewer reads the command, and it is also what the tapes
 *  learned about letting a re-render settle before the key lands. */
async function runLine(page: Page, line: string): Promise<number> {
  await focusPane(page)
  // The pane has just been clicked and the helper textarea focused in the
  // same tick; typing immediately loses the first keystroke. Measured
  // 2026-08-30: a take typed `adak close STD-7` and the shell answered
  // `sh: adak: command not found` on camera — the beat looked like the
  // product failing. So: settle, type, and then *read the prompt back*
  // before committing. A line that did not land whole is retyped once
  // rather than sent, because Enter is the one key this take cannot take
  // back.
  await beat(page, 350)
  const flat = (s: string) => s.replace(/\s+/g, '')
  for (let attempt = 0; attempt < 2; attempt++) {
    await page.keyboard.type(line, { delay: 60 })
    await beat(page, 800)
    if (flat(await readTerm(page)).includes(flat(line))) break
    // Clear whatever partial line is sitting there (Ctrl-U) and try again.
    await page.keyboard.press('Control+u')
    await beat(page, 300)
  }
  expect(flat(await readTerm(page)), `"${line}" never landed at the prompt`)
    .toContain(flat(line))
  await beat(page, 800)
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
    // sc=new,inprogress,done is not decoration. The default view filters
    // done OUT, so beat 4 would end with the card *vanishing* instead of
    // landing — the exact trap two hero takes fell into (hero-desk.spec.ts
    // §7). Grouping needs no param: defaultGroupBy is already status_category
    // (web/src/lib/view-config.ts:443), so New / In progress / Done are the
    // groups the card crosses.
    if (PROOF) mark('start')
    await page.goto('/#/?sc=new,inprogress,done')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })
    await expect(page.locator(`[data-issue-key="${TARGET_KEY}"]`)).toBeVisible()
    // The premise of the shot: the card starts outside In progress.
    expect(await targetCategory(page, TARGET_KEY)).toBe('new')
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
    await beat(page, 1500)

    // ── BEAT 1: the money shot ───────────────────────────────────────────
    const enterAt = await runLine(page, `gadak claim ${TARGET_KEY}`)
    mark('claim_enter')

    // The card. Polled tightly (200ms) because the number this take exists
    // to produce is *how long it took*, and a 2s poll would report the poll.
    await expect
      .poll(async () => groupOfTarget(page, TARGET_KEY), { timeout: 20_000, intervals: [200] })
      .toMatch(/^in progress$/i)
    const moveMs = Date.now() - enterAt
    mark('card_moved', `ms_after_enter=${moveMs}`)
    // The mirror-side truth, not the header text.
    expect(await targetCategory(page, TARGET_KEY)).toBe('inprogress')

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
    await page.locator(`[data-issue-key="${TARGET_KEY}"]`).click()
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

    // ── BEAT 3: the link back — NOT FILMED, and not by choice ────────────
    //
    // The shot list had a third leg: the issue keys in that output are
    // underlined, and clicking one opens the issue. It is cut because in a
    // standalone workspace the feature is not there to film, and release-
    // video.md's G2 is exactly that rule — absence cannot be photographed.
    //
    // Measured 2026-08-30, filed as GDK-1177. knownProjectKeys()
    // (TerminalPane.svelte:141-147) takes config().projects and falls back to
    // the pool's distinct source_project. On this workspace BOTH are empty:
    // /api/v1/config/ carries no `projects` key at all, and every row from
    // /api/v1/issues/bootstrap/ has `source_project: null`. An empty set
    // links nothing (issue-links.ts:38), so hovering STD-2 in the pane
    // produced no underline and clicking it opened no issue — confirmed at
    // the cell's own measured coordinates, so this is not a missed click.
    //
    // clickTerminalKey() below is kept, unused by the pass but ready: when
    // GDK-1177 lands, restoring the beat is deleting this comment and calling
    // it. Re-shooting costs seconds, which is the whole point of this rig.
    const term = await readTerm(page)
    const other = (term.match(/\bSTD-\d+\b/g) ?? []).find((k) => k !== TARGET_KEY)
    let linked = false
    if (other && process.env.GADAK_RT_LINKIFY) {
      linked = await clickTerminalKey(page, other)
      if (linked) {
        try {
          // Which issue the panel is on, asked of the panel. selection.select
          // routes to panel.show('issue', key) and the panel store touches no
          // URL at all (web/src/stores/panel.svelte.ts) — an earlier draft
          // polled `?issue=` in the hash and recorded a skip on a click that
          // had very likely worked.
          //
          // Budgeted at 3s, not the file default. This beat is the optional
          // one and a failed attempt is dead footage: the first take spent
          // ten seconds on two generous polls and the cut threw all ten away.
          await expect(page.getByTestId('issue-detail-panel'))
            .toContainText(other, { timeout: 3_000 })
          mark('linkify_opened', `key=${other}`)
          await beat(page, 1600)
        } catch {
          linked = false
        }
      }
    }
    if (!linked) mark('linkify_skipped', `candidate=${other ?? 'none'} (GDK-1177)`)

    // ── BEAT 4: close, and the card lands in Done ────────────────────────
    // Close the detail panel first: while it is docked the pane is squeezed
    // to its 320px minimum and the list to ~340px, where every summary is an
    // ellipsis — and the last frame of this film is the board. Its own close
    // button, not Escape: measured, Escape left the panel open and the take
    // ended on a squeezed list. In-app only, never a reload — that would drop
    // both sessions.
    await page.getByTestId('issue-detail-close').click()
    await expect(page.getByTestId('issue-body')).toHaveCount(0, { timeout: 10_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await beat(page, 1200)

    const closeAt = await runLine(page, `gadak close ${TARGET_KEY}`)
    mark('close_enter')
    await expect
      .poll(async () => groupOfTarget(page, TARGET_KEY), { timeout: 20_000, intervals: [200] })
      .toMatch(/^done$/i)
    mark('card_done', `ms_after_enter=${Date.now() - closeAt}`)
    expect(await targetCategory(page, TARGET_KEY)).toBe('done')
    await beat(page, 1300)

    // The card has landed, but at the bottom of a list that is taller than the
    // window — measured, the payoff row sat half-clipped on the frame's last
    // 12 pixels, which is the film's final image arriving cropped. The Done
    // chip is the way to that section (GDK-1057 made the chips the reveal
    // affordance) and it also defeats the virtual scroller, so the row is
    // rendered rather than merely present in the data.
    await page.locator('[data-testid="breakdown-strip"] button[title="Done"]').click()
    await expect
      .poll(async () => groupOfTarget(page, TARGET_KEY), { timeout: 10_000, intervals: [200] })
      .toMatch(/^done$/i)
    const row = page.locator(`[data-issue-key="${TARGET_KEY}"]`)
    await expect(row).toBeInViewport({ ratio: 0.9 })
    mark('done_revealed')
    // The closing hold, and the mark lands *inside* it: the video ends with
    // the test, so a mark past the last frame is a keyframe ffmpeg cannot
    // deliver (hero-desk.spec.ts learned this).
    mark('end_frame')
    await beat(page, 2600)
    mark('end')
  })
})
