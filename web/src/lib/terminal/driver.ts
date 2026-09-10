/*
 * The pane-socket driver (GDK-1767): attach, the generation guard, the
 * reconnect backoff ladder, the 60 s reattach grace, and the "tell the
 * server how big the pane is" protocol.
 *
 * This skeleton used to live twice — web/src/components/terminal/
 * TerminalPane.svelte and mobile/src/screens/Shell.svelte each carried an
 * attachSocket of ~95 lines, a scheduleReconnect, the timer pile and the
 * resize cache, with the GDK-1153/GDK-1154 incident comments maintained
 * verbatim in both. The phone cannot import the web pane's helpers (they
 * sit behind ./session, which pulls the wails transport), so the shared
 * part moves here the way ./protocol and ./resize already did: runes-free,
 * importable from both trees, transport-agnostic.
 *
 * A surface hands over two *measurements* (fittedSize, measurable — "is
 * there a renderer on a laid-out box to read"), one *transport* (open —
 * the web pane picks ws/wails, the phone the paired-home dial), and
 * *notifications*: bytes to draw, statuses to paint, and the pane-owned
 * reactions (which selection id to drop, what a recreate starts). The
 * driver owns every timer, the socket handle, and the answer to "is this
 * callback from the socket the pane still holds".
 *
 * The timing constants live here rather than ./session for the same
 * reason: the phone must be able to import them without resolving the
 * wails transport. ./session re-exports them so its existing importers
 * (gdk-944.test.ts) do not move.
 */

import { coerceDroppedReason } from './protocol'
import type { DroppedReason, SocketHandle, SocketHandlers, UnavailableCause } from './protocol'
import { settleResize } from './resize'

export const TERMINAL_GRACE_MS = 60_000
export const TERMINAL_RECONNECT_BACKOFF_MS = [500, 1000, 2000, 4000] as const
/** Bound for "WS that never opens" (desktop / wails:// has no TCP socket). */
export const TERMINAL_WS_OPEN_MS = 8_000

/** Where the one live attachment is, from the driver's side. */
export type DriverPhase = 'live' | 'ended' | 'unavailable' | 'reconnecting'

/**
 * What the driver tells the surface to paint. The phone's Status is this
 * union plus a `connecting` kind it sets itself before an attach (and a
 * refusal field its create-fail path adds); the web pane's is exactly this.
 * Every attach path has already painted `connecting`/`none` by the time a
 * socket opens, so the driver's open event lands on `none` unconditionally
 * — measured against both panes' old conditional forms, which only ever
 * saw those two kinds here.
 */
export type DriverStatus =
  | { kind: 'none' }
  | { kind: 'reconnecting' }
  | { kind: 'exited'; code: number }
  | { kind: 'dropped'; reason: DroppedReason }
  | { kind: 'unavailable'; cause: UnavailableCause }

/**
 * What a surface supplies. Every hook is small on purpose: the alternative
 * — flags for "is this the phone" — is how the two copies drifted apart in
 * the first place.
 */
export interface TerminalDriverHost {
  /** The transport: open a socket for `id`. The web pane picks ws/wails
   *  (./session), the phone the paired-home dial (mobile transport). */
  open(id: string, handlers: SocketHandlers): SocketHandle
  /** Measured cell size of the pane. `measurable` false means this
   *  answer is a fallback, not a measurement — do not call then. */
  fittedSize(): { cols: number; rows: number }
  /** A renderer on a laid-out box exists (GDK-1154: xterm's fit on a zero
   *  rect answers 10x5, and a hidden pane must not ship that as gospel). */
  measurable(): boolean
  /** Surface effects of a socket the pane still holds being open — the
   *  web pane focuses its renderer, the phone resets the buffer. Runs
   *  before the resize protocol below. */
  onAttached(): void
  /** PTY bytes for the current attachment; stale sockets never reach this. */
  onBytes(data: Uint8Array): void
  /** The shell under the pane exited; the driver has already painted. */
  onExit(code: number): void
  /** The serve dropped the session; the driver has already painted. */
  onDropped(reason: DroppedReason): void
  /** The selection must stop naming a session: the reconnect grace ran
   *  out, or a create's socket never came up. */
  onSessionGone(): void
  /** A kept-session socket never opened on activation: the surface
   *  recreates a session (its own startNew, with its own fail path). */
  onRecreate(): void
  /** Paint hook — the surface assigns this to its status state. */
  onStatus(status: DriverStatus): void
  /** The pane's "a socket is attached" flag (data-attached, and the
   *  phone's reattach guard). */
  onLive(live: boolean): void
  /** True while the surface suspends reconnects — the phone backgrounds
   *  and reconnects on visibility instead of on a timer. Absent = never. */
  hidden?(): boolean
  /** The web pane's first-attach ladder: delay in ms to retry a
   *  never-opened afterCreate socket, or null to give up. Absent = a
   *  never-opened create is `unavailable` immediately (the phone's
   *  verdict). See firstAttachRetryDelayMs (./session). */
  firstAttachRetry?(attempt: number): number | null
}

export interface TerminalDriver {
  /** Open the socket for `id`, ending any previous attachment first. */
  attach(id: string, opts: { afterCreate: boolean; recreateOnFail?: boolean }): void
  /** End the current attachment on purpose; its callbacks go stale. */
  detach(): void
  /** Unmount: timers down, socket closed, later attaches refused. */
  dispose(): void
  /** Keystroke path. Non-live phases drop the bytes; the surface's
   *  restart-on-Enter branch runs before this, on its own status. */
  send(bytes: Uint8Array): void
  /** Close the live socket *without* detaching — the close path stays
   *  armed, so the driver reconnects and replays. The phone's e2e drop
   *  hook rides this (GDK-865). */
  drop(): void
  /** The surface is starting a session on purpose (Enter-restart, +): the
   *  phase goes live and the backoff counters clear, so a second Enter
   *  during the create POST is a no-op rather than a second create. */
  revive(): void
  /** The surface's create POST failed: nothing is attached, and the pane
   *  must not read as live to its own keystroke-restart gate. The surface
   *  keeps the status (a refusal carries detail the driver cannot know);
   *  the driver records the phase, because the phase is its column. */
  markUnavailable(): void
  /** The surface is killing the shown session: ended now, so the socket's
   *  own close cannot read as a reconnectable drop. */
  end(reason: DroppedReason): void
  /** Visibility returned: cancel the pending reconnect timer and attach
   *  right away (the phone's reattachNow). */
  reattach(id: string): void
  /** Send the pane's size to the PTY if it changed and can be measured. */
  resizeNow(): void
  /** Coalesce a resize report (ResizeObserver) into one send. */
  scheduleFit(): void
  /** Stop every timer (fit, open, reconnect, settle). Surfaces call this
   *  where their old code did — switching sessions and unmounting. */
  clearTimers(): void
  /** Zero the backoff ladder and its grace clock. */
  resetBackoff(): void
  /** Measure and advance the create-size cache, so the POST and the
   *  later socket agree on what the server was first told. */
  measureForCreate(): { cols: number; rows: number }
  readonly phase: DriverPhase
  /** The id of the current attachment, null between attachments (after
   *  detach/dispose) — the switch guard's "what actually happened" side. */
  readonly currentId: string | null
  /** Is a socket of the pane's open right now. */
  readonly live: boolean
}

export function createTerminalDriver(host: TerminalDriverHost): TerminalDriver {
  return new Driver(host)
}

class Driver implements TerminalDriver {
  #host: TerminalDriverHost
  #socket: SocketHandle | null = null
  #gen = 0
  #currentId: string | null = null
  #liveNow = false
  #phase: DriverPhase = 'live'
  #disposed = false
  #stopSettle: (() => void) | null = null
  #fitTimer: ReturnType<typeof setTimeout> | undefined
  #openTimer: ReturnType<typeof setTimeout> | undefined
  #reconnectTimer: ReturnType<typeof setTimeout> | undefined
  #attempt = 0
  #since = 0
  #lastCols = 0
  #lastRows = 0

  constructor(host: TerminalDriverHost) {
    this.#host = host
  }

  get phase(): DriverPhase {
    return this.#phase
  }

  get currentId(): string | null {
    return this.#currentId
  }

  get live(): boolean {
    return this.#liveNow
  }

  /*
   * GDK-1153: a socket the pane has moved off is not allowed to speak.
   * Every attach stamps a generation; every callback checks it first, so a
   * socket closed on the way to another session cannot run the pane's
   * reconnect for the session it just left. Measured before this guard:
   * switching sessions closed a *live* socket, its onClose read
   * phase === 'live' and reconnected the old id, and the two attachments
   * then took turns replaying their rings into one buffer — the pane showed
   * both shells' scrollback, spliced, and neither session's keystrokes
   * landed where they were aimed. The class this closes is wider than the
   * switch: any late callback from a socket the pane no longer holds — a
   * slow close, a drop that arrives after a reattach — is indistinguishable
   * from the current one's without it.
   */
  attach(id: string, opts: { afterCreate: boolean; recreateOnFail?: boolean }): void {
    if (this.#disposed) return
    this.detach()
    let opened = false
    this.#phase = 'live'
    this.#currentId = id
    // Claimed after detach bumped the counter: this closure owns the pane
    // only while the counter still reads its own number.
    const gen = this.#gen
    const mine = (): boolean => gen === this.#gen
    const handle = this.#host.open(id, {
      onOpen: () => {
        if (!mine()) return
        opened = true
        if (this.#openTimer !== undefined) {
          clearTimeout(this.#openTimer)
          this.#openTimer = undefined
        }
        this.#liveNow = true
        this.#host.onLive(true)
        this.#attempt = 0
        this.#since = 0
        this.#host.onStatus({ kind: 'none' })
        this.#host.onAttached()
        // The size may have changed while the socket was down, and the
        // cache is "what the server was told" — a new socket has been told
        // nothing. Empty it, then let the one owner send, guard and all.
        // This used to be handle.resize() with an unguarded fittedSize(),
        // the side door a hidden pane's 10x5 reached the PTY through on a
        // late reattach (GDK-1154).
        this.#lastCols = 0
        this.#lastRows = 0
        this.resizeNow()
        // …and again across the window in which layout settles: one read at
        // open used to be the last word on the size for the life of the
        // session (resize.ts). resizeNow no-ops once the sizes agree.
        this.#stopSettle?.()
        this.#stopSettle = settleResize(() => this.resizeNow())
      },
      onBytes: (data) => {
        if (!mine()) return
        this.#host.onBytes(data)
      },
      onExit: (code) => {
        if (!mine()) return
        this.#phase = 'ended'
        this.#host.onStatus({ kind: 'exited', code })
        this.#host.onExit(code)
      },
      onDropped: (reason) => {
        if (!mine()) return
        const coerced = coerceDroppedReason(reason)
        this.#phase = 'ended'
        this.#host.onStatus({ kind: 'dropped', reason: coerced })
        this.#host.onDropped(coerced)
      },
      onClose: (neverOpened) => {
        if (!mine()) return
        this.#liveNow = false
        this.#host.onLive(false)
        if (this.#disposed || this.#phase === 'ended' || this.#phase === 'unavailable') return
        if (this.#host.hidden?.() ?? false) {
          // The phone: backgrounded sockets wait for visibility, not for a
          // timer — a ladder that fires in the background burns battery to
          // reconnect a pane nobody can see.
          this.#phase = 'reconnecting'
          this.#host.onStatus({ kind: 'reconnecting' })
          return
        }
        if (neverOpened && opts.recreateOnFail) {
          this.#host.onRecreate()
          return
        }
        if (neverOpened && opts.afterCreate) {
          const delay = this.#host.firstAttachRetry?.(this.#attempt) ?? null
          if (delay !== null) {
            this.#phase = 'reconnecting'
            this.#host.onStatus({ kind: 'reconnecting' })
            this.#attempt += 1
            this.#reconnectTimer = setTimeout(() => {
              if (this.#disposed) return
              this.attach(id, { afterCreate: true })
            }, delay)
            return
          }
          // The POST was accepted, so this is not a permission verdict:
          // the socket itself never came up.
          this.#phase = 'unavailable'
          this.#host.onStatus({ kind: 'unavailable', cause: 'network' })
          this.#host.onSessionGone()
          return
        }
        this.#scheduleReconnect(id)
      },
    })
    this.#socket = handle
    if (this.#openTimer !== undefined) clearTimeout(this.#openTimer)
    this.#openTimer = setTimeout(() => {
      if (opened || this.#disposed) return
      handle.close()
      // onClose handles recreate / unavailable / reconnect.
    }, TERMINAL_WS_OPEN_MS)
  }

  detach(): void {
    // Bump first: close() can call back synchronously, and a stale handler
    // that runs before the counter moves is exactly the race this closes.
    this.#gen += 1
    this.#socket?.close()
    this.#socket = null
    // Between shells the driver names no session (GDK-1185): the web pane's
    // switchTo skips `want === currentId`, and a detach that kept the old id
    // made the row just left un-clickable for the width of a create POST.
    // attach() sets the id again right after this.
    this.#currentId = null
    this.#liveNow = false
    this.#host.onLive(false)
  }

  dispose(): void {
    this.#disposed = true
    this.clearTimers()
    this.detach()
  }

  send(bytes: Uint8Array): void {
    if (this.#phase !== 'live') return
    this.#socket?.send(bytes)
  }

  drop(): void {
    this.#socket?.close()
  }

  revive(): void {
    this.#phase = 'live'
    this.resetBackoff()
  }

  markUnavailable(): void {
    this.#phase = 'unavailable'
  }

  end(reason: DroppedReason): void {
    this.#phase = 'ended'
    this.#host.onStatus({ kind: 'dropped', reason })
  }

  reattach(id: string): void {
    if (this.#reconnectTimer !== undefined) {
      clearTimeout(this.#reconnectTimer)
      this.#reconnectTimer = undefined
    }
    this.#attempt = 0
    this.attach(id, { afterCreate: false, recreateOnFail: true })
  }

  resizeNow(): void {
    // The single owner of "tell the server how big the pane is" (GDK-1154).
    // lastCols/lastRows mean "what the server was last told", so they
    // advance here and nowhere else — a cache that runs ahead of an actual
    // send turns every later check into a false negative.
    if (!this.#socket || this.#phase !== 'live') return
    if (!this.#host.measurable()) return
    const { cols, rows } = this.#host.fittedSize()
    if (cols === this.#lastCols && rows === this.#lastRows) return
    this.#lastCols = cols
    this.#lastRows = rows
    this.#socket.resize(cols, rows)
  }

  scheduleFit(): void {
    if (this.#fitTimer !== undefined) clearTimeout(this.#fitTimer)
    this.#fitTimer = setTimeout(() => this.resizeNow(), 100)
  }

  clearTimers(): void {
    this.#stopSettle?.()
    this.#stopSettle = null
    if (this.#fitTimer !== undefined) clearTimeout(this.#fitTimer)
    if (this.#openTimer !== undefined) clearTimeout(this.#openTimer)
    if (this.#reconnectTimer !== undefined) clearTimeout(this.#reconnectTimer)
    this.#fitTimer = undefined
    this.#openTimer = undefined
    this.#reconnectTimer = undefined
  }

  resetBackoff(): void {
    this.#attempt = 0
    this.#since = 0
  }

  measureForCreate(): { cols: number; rows: number } {
    const { cols, rows } = this.#host.fittedSize()
    this.#lastCols = cols
    this.#lastRows = rows
    return { cols, rows }
  }

  #scheduleReconnect(id: string): void {
    if (this.#since === 0) this.#since = Date.now()
    if (Date.now() - this.#since >= TERMINAL_GRACE_MS) {
      this.#phase = 'ended'
      this.#host.onStatus({ kind: 'dropped', reason: 'idle_timeout' })
      this.#host.onSessionGone()
      return
    }
    this.#phase = 'reconnecting'
    this.#host.onStatus({ kind: 'reconnecting' })
    const delay =
      TERMINAL_RECONNECT_BACKOFF_MS[
        Math.min(this.#attempt, TERMINAL_RECONNECT_BACKOFF_MS.length - 1)
      ]
    this.#attempt += 1
    this.#reconnectTimer = setTimeout(() => {
      if (this.#disposed) return
      this.attach(id, { afterCreate: false })
    }, delay)
  }
}
