/**
 * Hero desk bits 1 & 6, one continuous take (0.19, GDK-1037 B-loop: "자리를
 * 비워도 일은 계속된다") — scratch/hero/desk-take1.webm.
 *
 * Bit 1 (~3s of the cut): gadak's own window; one Korean sentence handed to a
 * live Claude Code agent in gadak's terminal pane; the pane closes — the
 * person walks away, the session stays — and the cut lands on the list.
 * Bit 6 (~2s): the pane reopens onto the SAME session, the ring replay
 * scrolls the interim work back in, and the board has the issue in Done.
 *
 * The two bookend beats are one take on purpose: the thing the clip claims is
 * *time actually passing while nobody watches*, and a splice of two sessions
 * would claim it falsely. The away-wait (45-70s) is real clock, the agent
 * works for real, and everything on camera happened in that one window.
 *
 * ── Detach and reattach, proved from source (not from hope) ──────────────
 *
 * What closing the pane actually does, walking web/src:
 *   TerminalPane.svelte:410-411 — the rail's close button
 *     (`data-testid="terminal-close"`) calls terminalChrome.toggle().
 *   App.svelte:796-797 — `{#if terminalChrome.open}<TerminalPane …/>{/if}`:
 *     toggle unmounts the pane. No page reload, so module state survives.
 *   TerminalPane.svelte:328-338 — the onMount cleanup runs detachSocket()
 *     (WS close only) under the comment "Keep the session id". It never
 *     deletes anything: the client has no close verb at all (GDK-922 — the
 *     REST DELETE exists for the server's own tests and e2e), so the pane
 *     physically cannot kill the session it is leaving.
 *   session.ts:41 — the only reaper is TERMINAL_GRACE_MS (60s), and
 *     internal/term reaps *idle* abandoned sessions; a session with a live
 *     child under its shell re-arms the grace instead of expiring (GDK-994,
 *     proven by internal/term/session_unix_test.go:803-824). `claude` still
 *     running under the shell is exactly that live child, so the 60s grace
 *     does not apply to this take's away-wait.
 *
 * Reopening walks back up the same path:
 *   App.svelte mounts a fresh TerminalPane → boot() (TerminalPane.svelte:278-313)
 *   → peekSessionId() reads the module-level keptSessionId
 *   (session.ts:222-230) → attachSocket(kept, {afterCreate:false}) reattaches
 *   instead of creating, and the server's ring replay arrives as the first
 *   binary frames — which is the scrollback the camera needs to see.
 *
 * The spec proves the contract from outside the UI too: GET
 * /api/v1/terminal/sessions/ (internal/server/terminal.go:373-386) is
 * snapshotted after the detach and again before the reattach — same id, still
 * alive — and after the reattach the list must still hold exactly that one
 * session with `attached` back at ≥1. A new session there would mean the
 * reopen created instead of reattaching, which is the exact failure this
 * take exists to rule out.
 *
 * ── The mirror ────────────────────────────────────────────────────────────
 *
 * The workspace is a standalone one (record-hero-desk.sh builds it:
 * `gadak init --local`, seed issues only, no real data — MEDIA.md).
 * Standalone is load-bearing here, not a convenience: the frozen demo home
 * the terminal-claude league uses rejects every write (origin.ErrWorkspaceFrozen,
 * internal/origin/origin.go:94-100), and this take needs the agent to really
 * transition an issue. The standalone origin is issuetap in-process, on
 * loopback, so the write path stays inside the machine and the fixture stays
 * fictional.
 *
 * Done-detection never keys on a display name: the poll reads
 * GET /api/v1/issues/bootstrap/ and checks `status_category === 'done'` for
 * the target key. The final UI beat switches the list to Jira-status grouping
 * and checks the target row sits *below the Done header* — a display name is
 * fine there because the fixture owns these names (seeded To Do / In
 * Progress / Done — the whole standalone transition graph, en locale), and
 * the correctness gate is the category poll, not the header text.
 *
 * ── Modes ─────────────────────────────────────────────────────────────────
 *
 * GADAK_MEDIA=1 gates the whole thing (media pipeline recording, not CI).
 * GADAK_HERO_DRY_RUN=1 replaces the live agent with `echo …`: same open /
 * detach / wait / reattach / grouping choreography, no model call — that is
 * the tuning loop the spec's budget lives on (one live rehearsal take, at
 * most one retry). The dry-run's idle shell has no live child, so its wait
 * stays well inside the 60s grace (8s).
 *
 * Marks: every beat appends {mark, epoch_ms} to $GADAK_HERO_PROOF (JSONL).
 * record-hero-desk.sh turns those into video-relative seconds and extracts
 * the four review keyframes from the finished webm — extraction is re-run
 * without a new take, so a mistimed frame never costs a live call.
 */
import { test, expect, type Page } from '@playwright/test'
import { appendFileSync } from 'node:fs'
import { forceLocale } from '../helpers'

const isMedia = !!process.env.GADAK_MEDIA
const DRY = !!process.env.GADAK_HERO_DRY_RUN

/** The issue the agent is handed. Must match the seed in
 *  record-hero-desk.sh (which is the single owner of the fixture). */
const TARGET_KEY = process.env.GADAK_HERO_TARGET || 'STD-7'

/** One sentence, the way a person asks — no JQL, no column names.
 *  Comment-then-done is two verbs the installed skill teaches (`gadak
 *  comment <KEY> -m …`, `gadak transition <KEY> done`), and the comment is
 *  what makes the reattached scrollback worth reading: interim work, not
 *  just a status flip.
 *
 *  English by default, and overridable rather than hardcoded. This line is
 *  the one piece of text the hero cut asks a stranger to read — it is the
 *  whole premise of the film in a sentence — and the audience for the clip
 *  is wider than the audience for any one language. The Korean phrasing
 *  this replaced (`… 버그 조사해서 요약 코멘트 남기고 완료로 바꿔줘`) is a
 *  GADAK_HERO_PROMPT away, for a take aimed at a Korean audience. */
const PROMPT =
  process.env.GADAK_HERO_PROMPT ||
  `look into ${TARGET_KEY}, leave a short comment with what you find, then close it`

/** The dry-run stand-in typed at the idle shell instead of `claude`. */
const DRY_ECHO = 'echo gadak-hero-dry-run'

const PROOF = process.env.GADAK_HERO_PROOF || ''

/** Away-wait window, in real clock. The floor is edit room for the phone
 *  bits that interleave with this one; the cap rejects a take where the
 *  agent outran the story's patience. Both from the shot list (GDK-1037). */
const WAIT_FLOOR_MS = 45_000
// 2026-08-29: raised 70s → 150s. Measured FAIL-first at 70s on an
// interleaved shoot — the agent was still working when the cap fired
// ("Expected: done / Received: new", proof-take-1.jsonl), and the take was
// rejected for the model's pace rather than for anything the story shows.
// The cap's job is to reject a take the CUT cannot use, and the cut
// compresses the away-wait to a beat either way; what it must not do is
// re-roll a live call because Opus took two minutes to read an issue,
// decide a rule, comment, and transition. The floor is untouched, so the
// phone-interleave window it exists for is exactly as before.
const WAIT_CAP_MS = 150_000
const DRY_WAIT_MS = Number(process.env.GADAK_HERO_DRY_WAIT || 8_000)

type SessionRow = { id: string; attached: number; exited: boolean }

function mark(name: string, note = ''): void {
  if (!PROOF) return
  appendFileSync(PROOF, `${JSON.stringify({ mark: name, epoch_ms: Date.now(), note })}\n`)
}

/** Pause between beats so a human can read the frame. */
async function beat(page: Page, ms = 700): Promise<void> {
  await page.waitForTimeout(ms)
}

/* ── Helpers below are copied from terminal-claude-demo.spec.ts, which they
 * fit identically; keep them in step with it — the two specs share the pane,
 * so divergence here is drift in the media pipeline, not local style. ── */

/** Focus the pane the way the e2e suite does: click the host, then focus the
 *  helper textarea explicitly. The renderer paints on a canvas, so the click
 *  alone lands on something that cannot hold a caret. */
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

/** Type a prompt into Claude's TUI and submit it. The gap before Enter is
 *  what the tapes learned the hard way: the input box re-renders as it
 *  grows, and an Enter that lands mid-render is swallowed. */
async function ask(page: Page, prompt: string): Promise<void> {
  await focusPane(page)
  await page.keyboard.type(prompt, { delay: 55 })
  await beat(page, 900)
  await page.keyboard.press('Enter')
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

/** Live session rows from the serve (internal/server/terminal.go:373-386).
 *  `attached` is the current WebSocket count, so the snapshots carry their
 *  own detach/reattach evidence: 0 after the close, back to ≥1 after the
 *  reopen, same id throughout. */
async function sessions(page: Page): Promise<SessionRow[]> {
  const res = await page.request.get('/api/v1/terminal/sessions/')
  expect(res.ok(), `terminal sessions GET: ${res.status()}`).toBeTruthy()
  const body = (await res.json()) as { sessions?: Array<Partial<SessionRow>> }
  return (body.sessions ?? []).map((s) => ({
    id: String(s.id),
    attached: Number(s.attached ?? 0),
    exited: !!s.exited,
  }))
}

/** Target issue's status_category straight off the bootstrap API — the
 *  display-name-free way to know the agent really moved it. */
async function targetCategory(page: Page): Promise<string | null> {
  const res = await page.request.get('/api/v1/issues/bootstrap/')
  expect(res.ok(), `bootstrap GET: ${res.status()}`).toBeTruthy()
  const body = (await res.json()) as {
    issues?: Array<{ key?: string; status_category?: string }>
  }
  const row = (body.issues ?? []).find((i) => i.key === TARGET_KEY)
  return row?.status_category ?? null
}

/** Where the target row sits once the list is grouped: the label of the
 *  header it falls under, by *visual* order (bounding boxes, not DOM order —
 *  the virtual scroller is free to nest). null = row not rendered. */
async function groupOfTarget(page: Page): Promise<string | null> {
  return page.evaluate(
    (key) => {
      const row = document.querySelector<HTMLElement>(`[data-issue-key="${key}"]`)
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
    },
    TARGET_KEY,
  )
}

test.describe('hero desk demo', () => {
  test.skip(!isMedia, 'GADAK_MEDIA=1 only — media pipeline recording')

  test('desk bits 1+6: walk away mid-task, come back to it done', async ({ page }) => {
    if (PROOF) mark('start')

    // The pane's stored width is a decision, not a ratio (see the sibling
    // spec): 640 is where the pane stops being clamped by the list minimum.
    await page.addInitScript(() => {
      try {
        localStorage.setItem('gadak.terminal.height', '340')
      } catch {
        /* private mode */
      }
    })
    await forceLocale(page, 'en')

    // ── 1. The list at rest, on a real (seeded, standalone) mirror. ──────
    await page.goto('/#/')
    await expect(page.getByTestId('issue-layout')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible({ timeout: 30_000 })
    await expect(page.getByTestId('list-count')).toContainText(/\d/)
    mark('bit1_list_stable')
    await beat(page, 2000)

    // ── 2. ⌘K → "Terminal": the agent arrives in gadak's own pane. ───────
    //    The palette (not the chord) is the discovery path a first-time
    //    viewer can follow.
    await page.keyboard.press('ControlOrMeta+k')
    const palette = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette).toBeVisible()
    await beat(page, 600)
    await page.keyboard.type('terminal', { delay: 60 })
    await expect(palette.getByTestId('palette-action-terminal')).toBeVisible()
    await beat(page, 700)
    await page.keyboard.press('Enter')
    await expect(palette).toBeHidden()

    const pane = page.getByTestId('terminal-pane')
    await expect(pane).toBeVisible()
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    // The beta mark came off in 0.19 (GDK-1024) — asserted absent, so a
    // revert cannot quietly put it back into a recording.
    await expect(page.getByTestId('terminal-beta')).toHaveCount(0)
    await beat(page, 1000)

    // Exactly one session exists and this is it — the id every later
    // snapshot has to keep showing.
    let live = await sessions(page)
    expect(live).toHaveLength(1)
    const sessionId = live[0].id
    expect(sessionId).not.toBe('')
    mark('pane_open', `session=${sessionId} attached=${live[0].attached}`)

    // ── 3. Hand over the issue. ──────────────────────────────────────────
    //    Live: boot Claude, wait for its input box, send the prompt, then
    //    wait for the *first* agent output — leaving before the model has
    //    said anything would film a handoff that never happened.
    //    Dry-run: the same typing rhythm against an idle shell, so the
    //    choreography is what gets tuned, not the model.
    if (DRY) {
      await focusPane(page)
      await page.keyboard.type(DRY_ECHO, { delay: 55 })
      await beat(page, 500)
      await page.keyboard.press('Enter')
      await expect
        .poll(async () => readTerm(page), { timeout: 15_000, intervals: [500] })
        .toContain('gadak-hero-dry-run')
      mark('bit1_echo_seen')
      await beat(page, 800)
    } else {
      await focusPane(page)
      await page.keyboard.type('claude', { delay: 90 })
      await beat(page, 500)
      await page.keyboard.press('Enter')
      await expect
        .poll(async () => readTerm(page), { timeout: 90_000, intervals: [1000] })
        .toMatch(/Welcome to Claude Code|for shortcuts|\? for shortcuts/i)
      mark('claude_booted')

      // First-run dialogs sit between the banner and the input box, and the
      // banner poll above matches the theme picker's own title — so
      // dismissing them IS the rest of boot (measured: two takes typed a
      // Korean prompt into "Choose the text style" and the agent never saw
      // it). The preselected option is what settings already chose, so one
      // Enter accepts. Bounded: three Enters is every onboarding step any
      // version has shown; a fourth means it is not onboarding, and flailing
      // at an unknown dialog on camera is worse than failing the take.
      for (let i = 0; i < 3; i++) {
        const term = await readTerm(page)
        if (/text style|trust the files/i.test(term)) {
          await page.keyboard.press('Enter')
          await beat(page, 800)
          continue
        }
        break
      }
      const afterDialogs = await readTerm(page)
      if (/select login method/i.test(afterDialogs)) {
        throw new Error(
          'claude booted to the login picker — the isolated HOME has no usable login',
        )
      }
      // The input box, not a dialog: this is what "ready to be typed into"
      // actually looks like in the buffer.
      await expect
        .poll(async () => readTerm(page), { timeout: 30_000, intervals: [500] })
        .toMatch(/❯|\? for shortcuts/)
      await beat(page, 2000)

      await ask(page, PROMPT)
      mark('bit1_prompt_sent')
      // The gate has to be output only the *working* agent emits. 'gadak'
      // cannot be in this pattern: the banner's status line carries the cwd,
      // which contains "gadak-claude-drive" — a gate that matches the boot
      // chrome passes takes where the model never answered (measured).
      await expect
        .poll(async () => readTerm(page), { timeout: 120_000, intervals: [1000] })
        .toMatch(/⏺|Esc to interrupt|✳|Bash\(|Read\(|Update\(/i)
      mark('bit1_agent_responding')
      // A beat of visible work before the walk-away — the frame the cut
      // starts on is the agent mid-task, not the Enter key.
      await beat(page, 2500)
    }

    // ── 4. Walk away: close the pane. ────────────────────────────────────
    //    The button is the detach path proven in the header comment —
    //    TerminalPane.svelte:410 → terminalChrome.toggle() → App.svelte's
    //    {#if} unmounts the pane → cleanup closes the WebSocket and keeps
    //    the session id. The model keeps running under the PTY: the grace
    //    reaper only takes *idle* abandoned sessions (GDK-994).
    await page.getByTestId('terminal-close').click()
    await expect(pane).toBeHidden()
    mark('bit1_detached')

    // Criterion 3, first half — the session survived the close, outside the
    // UI: same id, no WebSocket attached, not exited.
    live = await sessions(page)
    expect(live.map((s) => s.id)).toContain(sessionId)
    const detachedRow = live.find((s) => s.id === sessionId)
    expect(detachedRow?.attached).toBe(0)
    expect(detachedRow?.exited).toBe(false)
    mark('proof_post_detach', `sessions=${JSON.stringify(live)}`)

    // The cut's list frame: the pane is gone and the board fills the window.
    await expect(page.getByTestId('issue-list-scroller')).toBeVisible()
    await beat(page, 2500)
    mark('bit1_list_frame')

    // ── 5. The away-wait. Real clock, polled, bounded. ────────────────────
    //    Dry-run: fixed short wait (the idle shell has no live child, so
    //    the 60s grace is the only clock running).
    //    Live: the agent works unseen; the poll is display-name-free
    //    (status_category off the bootstrap API). The floor keeps the
    //    walk-away long enough to interleave with the phone bits; the cap
    //    rejects a take that outran the story.
    const waitStart = Date.now()
    if (DRY) {
      await beat(page, DRY_WAIT_MS)
    } else {
      await expect
        .poll(async () => targetCategory(page), {
          timeout: WAIT_CAP_MS,
          intervals: [2000],
        })
        .toBe('done')
      for (;;) {
        const elapsed = Date.now() - waitStart
        if (elapsed >= WAIT_FLOOR_MS) break
        await beat(page, Math.min(2000, WAIT_FLOOR_MS - elapsed))
      }
    }
    const waitedS = Math.round((Date.now() - waitStart) / 1000)
    mark('wait_done', `waited_s=${waitedS}`)

    // Criterion 3, second half — still the same session, still alive, right
    // before the pane comes back.
    live = await sessions(page)
    expect(live.map((s) => s.id)).toContain(sessionId)
    expect(live.find((s) => s.id === sessionId)?.exited).toBe(false)
    mark('proof_pre_reattach', `sessions=${JSON.stringify(live)}`)

    // ── 6. Come back: reopen the pane onto the SAME session. ─────────────
    //    Same discovery path as the open. boot() → peekSessionId() →
    //    attachSocket(kept) reattaches; the ring replay is the first binary
    //    frame, so the scrollback — the interim work — scrolls back in.
    await page.keyboard.press('ControlOrMeta+k')
    const palette2 = page.getByRole('dialog', { name: 'Command palette' })
    await expect(palette2).toBeVisible()
    await beat(page, 600)
    await page.keyboard.type('terminal', { delay: 60 })
    await expect(palette2.getByTestId('palette-action-terminal')).toBeVisible()
    await beat(page, 700)
    await page.keyboard.press('Enter')
    await expect(palette2).toBeHidden()

    await expect(pane).toBeVisible()
    await expect(pane).toHaveAttribute('data-attached', 'true', { timeout: 30_000 })
    mark('bit6_pane_reopened')

    // Reattach, not create: the same id, alive, with a WebSocket attached
    // again — and no session that was not already there before the pane
    // came back. A NEW row here is the failure mode this take exists to
    // rule out.
    //
    // 2026-08-29: this was `expect(live).toHaveLength(1)`, and an
    // interleaved two-camera shoot failed it with two live rows — the
    // second belonging to the PHONE's shell on the same serve, which is
    // the whole point of that shoot. "Exactly one row" was only ever a
    // proxy for "the desk did not reopen"; the set difference against the
    // pre-reattach snapshot says that directly, and says it whatever else
    // is attached to the mirror.
    const before = new Set(live.map((s) => s.id))
    live = await sessions(page)
    expect(live.map((s) => s.id)).toContain(sessionId)
    expect(live.find((s) => s.id === sessionId)?.attached).toBeGreaterThanOrEqual(1)
    expect(live.filter((s) => !before.has(s.id))).toHaveLength(0)
    mark('proof_post_reattach', `sessions=${JSON.stringify(live)}`)

    // The replay: the buffer carries the interim work again. The dry-run
    // looks for its echo; the live take looks for the issue key, which the
    // prompt echo and every gadak line in between both contain.
    await expect
      .poll(async () => readTerm(page), { timeout: 15_000, intervals: [500] })
      .toContain(DRY ? 'gadak-hero-dry-run' : TARGET_KEY)
    mark('bit6_replay_seen')
    await beat(page, 4000)

    // ── 7. Shift the view to the board's own summary and end on done. ────
    //    In-app controls only, never a page.goto: a reload would close the
    //    pane (terminalChrome.open is $state, not persisted) and drop the
    //    kept session id with it.
    //
    //    The default view is open issues — ?sc=new,inprogress, done filtered
    //    OUT — so ending there would end the clip with the finished issue
    //    *missing*, the opposite of the beat (found the hard way: two takes
    //    hunted a Done section that could not exist). Including done is a
    //    real operator action: Add filter → Progress → Done.
    await page.getByTestId('filter-add').click()
    await page.getByTestId('filter-axis-status_category').click()
    await page
      .locator('[data-testid="filter-value-row"][data-filter-value="done"]')
      .click()
    await page.keyboard.press('Escape')

    // Then the board's own summary: section the list by Jira status.
    await page.getByRole('button', { name: /Breakdown/ }).click()
    await page.getByRole('button', { name: 'Jira status', exact: true }).click()
    mark('bit6_grouped_status')

    if (DRY) {
      // Nothing moved in a dry run — assert the shift took (the strip names
      // the seeded statuses, Done among them now) and stop. The
      // done-position check is the live take's to make. The mark lands
      // *inside* the closing hold, not after it: the video ends with the
      // test, and a mark past the last frame is a keyframe ffmpeg cannot
      // deliver.
      await expect(page.locator('[data-testid="breakdown-strip"]')).toBeVisible()
      await expect(
        page.locator('[data-testid="breakdown-strip"] button[title="Done"]'),
      ).toBeVisible()
      mark('bit6_end_frame')
      await beat(page, 3000)
      mark('end')
      return
    }

    // The Done chip is the way to that section (GDK-1057 made the chips the
    // reveal affordance), and the reveal also defeats the virtual scroller:
    // it puts the Done section in the viewport so the row is rendered.
    await page.locator('[data-testid="breakdown-strip"] button[title="Done"]').click()

    // Criterion 4's camera-side half: the target row sits below the Done
    // header. (The mirror-side half is record-hero-desk.sh's `gadak sql`
    // check on status_category — display names prove nothing.)
    await expect
      .poll(async () => groupOfTarget(page), { timeout: 15_000, intervals: [500] })
      .toMatch(/^done$/i)
    await expect(page.locator(`[data-issue-key="${TARGET_KEY}"]`)).toBeVisible()
    mark('bit6_done_frame')
    await beat(page, 3000)
    mark('end')
  })
})
