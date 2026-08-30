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
  PANE_HEIGHT,
  boardReady,
  card,
  categoryOf,
  columnOf,
  movedByOther,
  revealDone,
  sessionTab,
  sessionTabNames,
  tabsBelowFold,
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
        localStorage.setItem('gadak.terminal.height', w)
      } catch {
        /* private mode */
      }
    }, PANE_HEIGHT)
    await forceLocale(page, 'en')

    // ── SET-UP (off camera) ──────────────────────────────────────────────
    // Five shells, each claimed onto a different issue and left with a bit of
    // work in it. The cut opens after all of this: the film is about coming
    // BACK to sessions, so the leaving has already happened when it starts.
    //
    // Identity is exported and cleared before each claim (two-pass, see the
    // note further down): a card wearing "Claude Code" reads as an agent
    // fleet, which is the opposite of this film's subject — a person who left
    // five terminals lying around.
    //
    // Every command is chosen to print nothing about the machine it runs on:
    // no `ls -la`, no `pwd`. A storyboard still shot with `ls -la` had the
    // operator's account name in frame, which MEDIA.md keeps out of a
    // recording.
    const CREW = [
      {
        key: 'STD-4',
        actor: 'human:ada|Ada Lovelace',
        files: [
          `printf 'preview builds its column list in the view\\nexport builds its own in the writer\\ntax column only exists in the first\\nboth read the same field map\\n' > notes.txt`,
          `printf 'FAIL export/columns  tax missing (2 of 14)\\nFAIL export/totals   net != gross - tax\\nok   export/header\\n' > run.log`,
        ],
        work: [
          'cat notes.txt',
          'cat run.log',
          'grep -n column notes.txt',
        ],
      },
      {
        key: 'STD-9',
        actor: 'human:grace|Grace Hopper',
        files: [
          `printf 'search, then open the sidebar: highlight stays\\nsidebar keeps the last query mark\\nclearing the query does not clear it\\nonly a second search repaints\\n' > repro.txt`,
        ],
        work: [
          'cat repro.txt',
          'grep -n sidebar repro.txt',
          'wc -l repro.txt',
        ],
      },
      {
        key: 'STD-14',
        actor: 'human:katherine|Katherine Johnson',
        files: [
          `printf 'focus lands one frame before the box mounts\\nfirst keystroke goes to the document\\nsecond onward are fine\\nonly on a freshly rendered comment box\\n' > focus.txt`,
        ],
        work: [
          'cat focus.txt',
          'grep -n keystroke focus.txt',
        ],
      },
      {
        key: 'STD-1',
        actor: 'human:ada|Ada Lovelace',
        files: [
          `printf 'declined once, retried, charged twice\\nretry sends a new request id each time\\nneeds an idempotency key on the retry\\ntest card 4000 0000 0000 0002 reproduces\\n' > retry.txt`,
        ],
        work: [
          'cat retry.txt',
          'grep -n idempotency retry.txt',
        ],
      },
    ]
    // Four, not five. The strip is 144px (GDK-1193) and a row is ~30px at the
    // fixture's 16px body text — five rows are 150px and the last one sits
    // below the fold (measured, the take failed its own assertion). Four fill
    // the strip exactly and the roster still reads as "I left work in all of
    // these".

    if (PROOF) mark('start')
    await page.goto(BOARD_ROUTE)
    await boardReady(page)

    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await page.keyboard.type('terminal', { delay: 40 })
    await expect(palette.getByTestId('palette-action-terminal')).toBeVisible()
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()
    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    // Not a beat — the clock mark. rt-marks.py finds the offset between the
    // spec's wall clock and the recorder's by looking for the luma cliff the
    // pane makes when it opens, and it needs this mark to compare against.
    mark('pane_open')
    await beat(page, 800)

    for (const [i, m] of CREW.entries()) {
      if (i > 0) {
        await page.getByTestId('terminal-new').click()
        await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
        await beat(page, 400)
      }
      await runLine(page, `export GADAK_ACTOR='${m.actor}'; clear`, true)
      // `clear` right after the claim, so the session's ring holds only the
      // work. A replay that opens on `gadak claim …` puts "drive the tracker
      // from a terminal" in the most readable spot in the film — the exact
      // thing this concept replaced — and the claim's reply folds the issue
      // summary mid-word on top of that.
      // Files first, then claim-and-clear, then the reads. Everything that
      // writes the fixture happens before the wipe, so the ring the camera
      // replays holds only a person reading their own notes — no `printf`
      // scaffolding, no `gadak claim`.
      for (const c of m.files) await runLine(page, c, true)
      await runLine(page, `gadak claim ${m.key} && clear`, true)
      for (const c of m.work) await runLine(page, c, true)
    }

    // Every shell wears its issue's key, and every one of those issues shows a
    // shell on its card. This is the premise the film needs true before the
    // first frame.
    await expect
      .poll(async () => sessionTabNames(page),
        { timeout: 20_000, intervals: [300] })
      .toEqual(expect.arrayContaining(CREW.map((m) => expect.stringContaining(m.key))))
    for (const m of CREW) {
      expect(await categoryOf(page, m.key)).toBe('inprogress')
    }
    // The strip is 144px now (GDK-1193) and five rows are 131px, so the whole
    // roster is on screen — before the fix the active session sat below a fold
    // nobody chose, which is what made the chaos beat unfilmable.
    const offscreen = await tabsBelowFold(page)
    expect(offscreen, 'strip rows below the fold').toBe(0)
    await beat(page, 1800)
    mark('chaos')
    await beat(page, 1800)

    // ── RECOVERY A ───────────────────────────────────────────────────────
    // One click on a row, and the shell that was doing that issue's work is
    // back with its scrollback. Then a line typed into it, because a replay
    // that cannot be typed into is a screenshot.
    // The replay is proved by something only THAT shell's history contains —
    // the claim used to be the marker, but it is cleared out of the ring now
    // (see the set-up), and a note from the issue's own work is the honest
    // witness anyway.
    const recover = async (key: string, line: string, tag: string, seen: string) => {
      await sessionTab(page, key).click()
      await expect
        .poll(async () => readTerm(page), { timeout: 15_000, intervals: [200] })
        .toContain(seen)
      mark(`${tag}_replay`, `key=${key}`)
      await beat(page, 900)
      await runLine(page, line)
      await beat(page, 900)
      mark(`${tag}_alive`)
    }
    await recover(CREW[0].key, 'grep -n field notes.txt', 'a', 'FAIL export/columns')

    // ── RECOVERY B ───────────────────────────────────────────────────────
    // Straight into another one. Two in a row is what makes it a system
    // rather than a trick.
    await recover(CREW[1].key, 'grep -n repaint repro.txt', 'b', 'sidebar keeps the last query')
    mark('end_frame')
    await beat(page, 2000)
    mark('end')
  })
})
