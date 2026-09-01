/*
 * The single owner of "which session is the pane showing" (GDK-1153).
 *
 * The server has been fully multi-session since GDK-864 and has no ceiling
 * on how many shells it will hold. What made gadak single-session was one
 * module-level `let keptSessionId` in ./session and one <TerminalPane>
 * mount in App.svelte. The mount stays — this is a selector bolted onto the
 * singleton, not a tiling window manager — but the id moves here, so every
 * surface that can choose a session (the strip, a reopen inside the grace,
 * a create, an exit) goes through the same door.
 *
 * That is the class this file closes: not "there is no tab bar", but "the
 * answer to *which session* is derived in more than one place". A second
 * copy of that answer is how a pane ends up attached to a session the strip
 * says is not selected.
 *
 * Nothing here is persisted. A session id is runtime state that dies with
 * the serve, and gadak stores no originals of its own (CLAUDE.md).
 */

import { deleteSession, terminalBase } from './session'
import { terminalChrome } from './pane.svelte'
import { nextSelectedAfterKill, type TerminalSessionInfo } from './strip'

/** How often the roster refreshes while the pane is open. Fast enough that
 *  a shell that just started printing is marked running before the eye
 *  gets there, slow enough to be free on a loopback socket. */
export const ROSTER_POLL_MS = 2_000

async function fetchSessions(): Promise<TerminalSessionInfo[]> {
  const res = await fetch(`${terminalBase()}sessions/`, { credentials: 'same-origin' })
  if (!res.ok) throw new Error(`terminal list HTTP ${res.status}`)
  const body = (await res.json()) as { sessions?: TerminalSessionInfo[] }
  return body.sessions ?? []
}

class TerminalSessions {
  /** The id the pane is attached to, or should attach to next. */
  selectedId = $state<string | null>(null)
  /** The roster as the server last described it. */
  list = $state<TerminalSessionInfo[]>([])
  /** True once a list has come back — so the strip can tell "no sessions"
   *  from "have not asked yet" and refuse to draw an empty state at boot. */
  loaded = $state(false)

  #watchers = 0
  #timer: ReturnType<typeof setInterval> | undefined
  #inFlight = false

  /**
   * Choose a session. Idempotent: re-selecting the attached session is not
   * a reattach, which matters because the pane reacts to this value and a
   * spurious change would drop and replay the pane it is already showing.
   */
  select(id: string | null): void {
    if (this.selectedId === id) return
    this.selectedId = id
  }

  /** The row for the selected session, when the roster has caught up. */
  get selected(): TerminalSessionInfo | null {
    const id = this.selectedId
    if (!id) return null
    return this.list.find((s) => s.id === id) ?? null
  }

  /**
   * Move the selection one session along the roster (GDK-1250): the strip's
   * own order, wrapping at both ends. False — nothing moved — when the pane
   * is closed or fewer than two sessions are alive; a selection the roster
   * no longer lists anchors at the first session, the first tab the eye
   * would fall on. One synchronous pass over the roster, so a poll that
   * swaps the list between two chords can never strand an index.
   */
  cycle(dir: 1 | -1): boolean {
    if (!terminalChrome.open) return false
    const ids = this.list.map((s) => s.id)
    if (ids.length < 2) return false
    const at = ids.indexOf(this.selectedId ?? '')
    this.select(at === -1 ? ids[0] : ids[(at + dir + ids.length) % ids.length])
    return true
  }

  /**
   * End a session on purpose (GDK-1200): a tab's ×, no confirmation. The
   * selection moves first and synchronously — to the right-hand neighbour
   * when the killed session is the shown one — so the pane has already left
   * before the server tears the shell down; killing the last session leaves
   * null and the pane's own exit/dropped path narrates the end. The DELETE
   * is the same verb the server has kept since GDK-922, and the refresh
   * pulls the roster the poll would otherwise take two seconds to notice.
   */
  async kill(id: string): Promise<void> {
    this.select(
      nextSelectedAfterKill(
        this.list.map((s) => s.id),
        id,
        this.selectedId,
      ),
    )
    await deleteSession(id)
    await this.refresh()
  }

  /**
   * Start polling while a surface is watching. Reference-counted so two
   * surfaces (the strip and, later, anything else) share one poll rather
   * than racing two. Returns the stop.
   */
  watch(): () => void {
    this.#watchers += 1
    if (this.#watchers === 1) {
      void this.refresh()
      this.#timer = setInterval(() => void this.refresh(), ROSTER_POLL_MS)
    }
    let stopped = false
    return () => {
      if (stopped) return
      stopped = true
      this.#watchers -= 1
      if (this.#watchers === 0 && this.#timer !== undefined) {
        clearInterval(this.#timer)
        this.#timer = undefined
      }
    }
  }

  /**
   * A mirror write elsewhere — `gadak claim` renaming a session — should
   * reach the strip on the tick it happened, not the next 2s grid line
   * (GDK-1182: the skew was uniform 0–2000ms). The ui-focus poll calls this
   * on the same mirrorVersion signal the board already rides (GDK-1170).
   * Only while a surface is watching: a closed pane learns at open.
   */
  nudge(): void {
    if (this.#watchers > 0) void this.refresh()
  }

  /**
   * One roster read. A failure leaves the last known list standing: the
   * strip going blank because one poll lost a race is worse than a row
   * being two seconds stale, and the pane's own socket is the authority on
   * whether the server is reachable.
   */
  async refresh(): Promise<void> {
    if (this.#inFlight) return
    this.#inFlight = true
    try {
      this.list = await fetchSessions()
      this.loaded = true
    } catch {
      /* keep the last roster */
    } finally {
      this.#inFlight = false
    }
  }

  /**
   * The client half of "what is going on in there" (GDK-1153 §7.3). The
   * server half is already answerable in one line — `curl
   * .../terminal/sessions/`, or tools/terminal-probe.sh — but only this
   * side knows which of those rows the pane is actually holding, and that
   * is exactly the half a session-switching bug lives in.
   */
  debugSnapshot(): { selectedId: string | null; sessions: TerminalSessionInfo[] } {
    return { selectedId: this.selectedId, sessions: [...this.list] }
  }
}

export const terminalSessions = new TerminalSessions()

/**
 * Show the shell bound to an issue (GDK-1196/GDK-1197).
 *
 * The one channel every "enter this issue's session" affordance goes through
 * — the palette row and the board card both call this, and neither invents
 * state: select() is already the sole owner of which session the pane holds,
 * toggle() the sole owner of whether it is open. Unlike the body's ▶ this
 * re-targets an already-open pane on purpose: being shown that session *is*
 * the whole request here, and select() only changes what is visible — it
 * detaches nothing.
 */
export function enterShell(id: string): void {
  terminalSessions.select(id)
  if (!terminalChrome.open) terminalChrome.toggle()
}

if (typeof window !== 'undefined') {
  ;(window as unknown as { __gadakTermSessions?: () => unknown }).__gadakTermSessions = () =>
    terminalSessions.debugSnapshot()
}
