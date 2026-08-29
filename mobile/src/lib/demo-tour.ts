// DEV-only self-driving tour for media capture (lead tool, 2026-08-25).
//
// Synthetic touches from the host (cliclick) proved unreliable against the
// dev shell — the WKScrollView anchor mood shifts per launch (see the
// safe-area note in App.svelte), so screen→page coordinates cannot be
// trusted across takes. This drives the same journey from inside instead:
// store calls and DOM events, deterministic timings, no coordinates.
//
// GDK-1117 rewrote the walk as one story (the 6-bit hero): the tracker
// delta with the agent as its subject, the terminal as a tab of the
// tracker — never a fullscreen cut — and the issue from the opening list
// standing done at the end. The pairing tab is out of the timeline
// (plumbing earns no seconds), and no bit leaves the app chrome: the
// Shell pane lives inside the same .tabs column as Issues, with the tab
// bar as its sibling (App.svelte), so a tab switch can never remove it.
//
// Arming: `VITE_DEMO_TOUR=1 npm run dev` (what the phone rig uses — the
// iOS dev window's URL is fixed at build time, so a query string is not
// reachable there), or open the DEV origin with `?demo-tour` and reload.
// A HEAD/file probe at /__demo-tour__ was abandoned: mobile/public/ does
// not exist, and vite's SPA fallback serves index.html with 200 for every
// unmatched path, so r.ok was always true and the tour drove every
// `npm run dev` boot with no way to disarm. A query parameter cannot be
// forged by a route. Omit the param to disarm. Packaged builds never run
// this (`import.meta.env.DEV` below). The module is imported from main.ts
// in DEV builds only.
//
// Environment the story assumes (the tour warns, never adapts — a take
// with missing bits is a broken take, which the operator should see):
//   - A reachable shell: the Shell tab exists only while app.terminal is
//     set (App.svelte/TabBar.svelte). In DEV that no longer needs a QR
//     dance — with VITE_DEV_SHELL=1, loadTerminal() adopts the vite proxy
//     the way boot() adopts it for the serve session (store.svelte.ts), so
//     a dev server pointed at a live serve is the whole precondition. The
//     bundled demo workspace still cannot show the shell bits —
//     enterDemo() resets the terminal state — so capture runs against the
//     proxy, never the demo.
//   - A scrubbed snapshot serve (MEDIA.md's standing rule): bit 4 types
//     `gadak close <KEY>`, a real CLI write (cmdClose → applyTransitionWrite
//     with target "done"), against whatever origin that serve fronts.
//
// Two premises from the storyboard, corrected against the code:
//   - "The issue stands done on the board": a done row cannot stand in
//     the Issues list — both hardcoded scopes are open-only by design
//     (domain.ts openIssues); done is what leaves the open queue. The
//     punchline therefore lands twice: the glance strip carries the
//     status-change delta (the board's own grammar for "it is done now"),
//     then the issue's own page holds it standing Done (chip + spine,
//     staged in the store — the snapshot itself is static).
//   - "The shell replays its scrollback": a ring replay happens on
//     reattach of a kept session, and an armed reload always starts a
//     fresh one (keptSessionId is Shell.svelte module state; activation
//     POSTs a new session). Bit 3 shows the honest fresh-boot catch-up:
//     the session comes up and the prompt lands, in frame with the chrome.
//
// The dark-mode flip stays outside (xcrun simctl ui booted appearance dark)
// — the app follows the system appearance. The steady beat reserved for it
// is the board return in bit 5 (window in the table below).

import { app, openIssue, switchTab } from './store.svelte'
import { openIssues, sortIssues, type FeedItem, type FeedResponse } from './domain'

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms))

/** Smooth-scroll the visible screen's scroller by `dy`, over ~600ms. */
function glide(dy: number): void {
  const el = document.querySelector<HTMLElement>(
    '.detail-layer .scroll, .detail-layer [data-scroll], .pane:not(.off) .scroll, .pane:not(.off) [data-scroll], .detail-layer main, .pane:not(.off) main',
  )
  const scroller = el ?? scrollableIn(document.body)
  scroller?.scrollBy({ top: dy, behavior: 'smooth' })
}

/** First descendant that actually scrolls vertically. */
function scrollableIn(root: HTMLElement): HTMLElement | null {
  const all = root.querySelectorAll<HTMLElement>('*')
  for (const el of all) {
    if (el.scrollHeight > el.clientHeight + 40) {
      const oy = getComputedStyle(el).overflowY
      if (oy === 'auto' || oy === 'scroll') return el
    }
  }
  return null
}

/** The row the camera reads as "the" issue: first of the list the screen
 *  itself paints (same selection modules Issues.svelte routes through
 *  buildList → openIssues + sortIssues — same input, same row). */
function storyIssueKey(): string {
  return sortIssues(openIssues(app.issues))[0]?.issue_key ?? ''
}

/** Staged unread deltas for the glance strip. The bundled demo answers
 *  issues/feed/ with an empty feed by design (demo.ts) and a serve's real
 *  feed is whatever happened lately — neither is a deterministic shot, so
 *  the tour stages the rows itself: fixture keys, fixture people, event
 *  words from the catalog's own feed grammar. Nothing here reaches origin. */
function glanceFeed(rows: { key: string; actor: string; event: string; ageMs: number }[]): FeedResponse {
  const now = Date.now()
  return {
    items: rows.map((r, i) => ({
      event_id: `tour-${i}-${r.key}`,
      issue_key: r.key,
      event_type: r.event,
      occurred_at: new Date(now - r.ageMs).toISOString(),
      actor_name: r.actor,
      reasons: ['assignee'],
      read_at: null,
    }) satisfies FeedItem),
    unread_counts: { all: rows.length, assignee: rows.length, reporter: 0, mention: 0 },
  }
}

/** Stage the story's issue as done. The display name and status id are
 *  copied from a real done row of the same snapshot — never authored (the
 *  display-name trap: localized serves say something other than "Done"). */
function flipDone(key: string): void {
  const row = app.issues.find((i) => i.issue_key === key)
  if (!row) return
  const donePeer = app.issues.find((i) => i.status_category === 'done')
  row.status_category = 'done'
  if (donePeer) {
    row.status = donePeer.status
    row.status_id = donePeer.status_id
  }
}

/** Boot's first sync cycle overwrites app.feed wholesale (store.sync); the
 *  staged strip must land after it. The 60s re-sync timer and the
 *  visibilitychange-triggered sync are the only later writers — both
 *  outside the tour's ~16s window. In the demo workspace sync no-ops and
 *  app.syncing never rises; the floor keeps staging out of first paint. */
async function settleFirstSync(): Promise<void> {
  await wait(300)
  const deadline = Date.now() + 1300
  while (app.syncing && Date.now() < deadline) await wait(100)
}

/** Type one line through the phone's real keystroke path: the IME textarea
 *  (Shell.svelte's dock, DESIGN.md §10.3). Its own oninput → flushIme →
 *  imeReduce maps non-composing input to 'plain' and emits; onkeydown with
 *  key Enter → sendText('\r'). The bytes reach the PTY exactly as a
 *  thumb's would. One caveat the take inherits: WebKit raises the OS
 *  keyboard only inside a user gesture, so programmatic focus types over
 *  the KeyBar without the system keyboard — the echo in the terminal is
 *  the visible typing. */
async function typeLine(line: string): Promise<void> {
  const ime = document.querySelector<HTMLTextAreaElement>('[data-testid="shell-ime"]')
  if (!ime) return
  ime.focus()
  for (const chunk of line.match(/.{1,3}/g) ?? []) {
    ime.dispatchEvent(
      new InputEvent('input', { data: chunk, isComposing: false, bubbles: true }),
    )
    await wait(200)
  }
  await wait(250)
  ime.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
}

async function tour(): Promise<void> {
  // Phone bits of the 6-bit hero (the desktop camera owns bits 1 and 6).
  // The schedule is absolute from tour start — staging work absorbs into
  // the holds, so the boundaries below are the cut-sync contract (±50ms
  // on a quiet machine; the desktop lead cuts against this table).
  //
  // | bit | t≈    | duration | on screen                                            |
  // |-----|-------|----------|------------------------------------------------------|
  // | 2   | 0.0s  | 4.2s     | Issues list from cold: staged glance strip (3 unread  |
  // |     |       |          | deltas), one restrained glide at 2.6s                |
  // | 3   | 4.2s  | 2.8s     | Shell tab — session boots, prompt lands; tab bar in   |
  // |     |       |          | frame (no fullscreen transition)                      |
  // | 4   | 7.0s  | 4.2s     | thumb types `gadak close <KEY>` via the IME path; the |
  // |     |       |          | CLI's confirmation prints                             |
  // | 5   | 11.2s | 4.2s     | board return: the row has left the open queue, the    |
  // |     |       |          | strip carries the Done delta; at 12.8s the issue's    |
  // |     |       |          | own page holds it standing Done until 15.4s          |
  //
  // External dark flip window: 11.2–12.8s (the board beat) — the punchline
  // hold then renders dark from its first frame.
  const t0 = Date.now()
  const at = (ms: number) => wait(Math.max(0, ms - (Date.now() - t0)))

  // Bit 2 — list + glance strip hold.
  //
  // Nothing about the session may be read before this await. Both reads
  // that used to sit above it were answered by an empty store and both
  // were wrong (measured 2026-08-29, first armed take): app.issues was
  // still [] so the story's key came back '' — bit 4 announced "no open
  // issue" and skipped the only write in the film, and bit 5's openIssue('')
  // put a 404 where the punchline goes — while app.terminal was still null
  // because loadTerminal()'s dev adoption had not resolved, so the take
  // warned about a missing shell it then showed working. The store settles
  // here; every read of it is below.
  await settleFirstSync()
  const key = storyIssueKey()
  const open = sortIssues(openIssues(app.issues)).slice(0, 3)
  app.feed = glanceFeed(
    open.map((issue, i) => ({
      key: issue.issue_key,
      actor: issue.assignee ?? '',
      event: (['status_changed', 'comment_added', 'assigned'] as const)[i] ?? 'fields_changed',
      ageMs: [0, 4 * 60_000, 23 * 60_000][i] ?? 60_000,
    })),
  )
  await at(2600)
  glide(560) // the one restrained glide — the old walk's four were too many
  await at(4200)

  // Bit 3 — the terminal is a tab of the tracker. The warning belongs to
  // the beat it is about: by now the shell has either been adopted or it
  // has not, and the operator is told the take is broken exactly once.
  if (!app.terminal) {
    console.warn(
      'gadak demo tour: no shell reachable — the shell bits will show a blank tab (is the dev proxy pointed at a live serve?)',
    )
  }
  switchTab('shell')
  await at(7000)

  // Bit 4 — one line by thumb, and it is the punchline's cause: the real
  // close verb (cmdClose) moving the story's issue to done.
  //
  // This is a WRITE, and it lands on whatever origin the paired serve
  // fronts — that is the point (a faked delta would make the hero's
  // central claim a prop), but it is also the one irreversible thing this
  // tool does. The header's scrubbed-snapshot assumption is stated where
  // nobody reads it at take time, so it is said aloud here instead.
  if (key) {
    console.warn(`gadak demo tour: bit 4 runs a REAL write — \`gadak close ${key}\` on the paired origin`)
    await typeLine(`gadak close ${key}`)
  } else {
    // No open row to tell a story about. Typing `gadak close` bare would
    // put a usage error in the punchline's cause; a take without bit 4 is
    // visibly broken, which is the outcome the operator should see.
    console.warn('gadak demo tour: no open issue in the list — skipping bit 4 (the take has no punchline)')
  }
  await at(11200)

  // Bit 5 — the board answers. The story issue's bit-2 delta is dropped:
  // superseded by the fresh Done line, not shown beside it.
  flipDone(key)
  const staged = app.feed ?? glanceFeed([])
  app.feed = glanceFeed([
    ...staged.items
      .filter((item) => item.issue_key !== key)
      .map((item) => ({
        key: item.issue_key,
        actor: item.actor_name,
        event: item.event_type,
        ageMs: Date.now() - new Date(item.occurred_at ?? Date.now()).getTime(),
      })),
    { key, actor: '', event: 'status_changed', ageMs: 0 },
  ])
  switchTab('issues')
  await at(12800)
  openIssue(key) // the issue itself, standing Done — hold to the end
  await at(15400)
}

/**
 * True only in DEV, and only when the operator armed this dev server or
 * this page: `VITE_DEMO_TOUR=1 npm run dev`, or `?demo-tour` on the URL.
 *
 * The env arm exists because the phone rig cannot reach the query string.
 * The iOS dev window's URL is fixed at build time (tauri.conf devUrl), so
 * the only way to add a parameter is to rebuild the rust binary for every
 * take — and under a proxied dev shell the page is not even served from
 * that origin. The env var is set by one script for one dev-server start
 * and is baked into that server's DEV bundle only, which is the same
 * "explicit, per-launch, unforgeable by a route" property that made the
 * query param win over the abandoned /__demo-tour__ probe.
 */
export function isDemoTourArmed(): boolean {
  if (!import.meta.env.DEV) return false
  if (import.meta.env.VITE_DEMO_TOUR === '1') return true
  if (typeof location === 'undefined') return false
  return new URLSearchParams(location.search).has('demo-tour')
}

export function armDemoTourInDev(): void {
  if (!import.meta.env.DEV) return
  if (!isDemoTourArmed()) return
  console.info('gadak demo tour: driving the app (armed by ?demo-tour)')
  void tour()
}
