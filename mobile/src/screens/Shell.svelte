<script module lang="ts">
  // Which session the pane is showing — kept across tab switches and app
  // backgrounds so a reattach replays the ring (desktop pane does the same,
  // in sessions.svelte.ts). Since GDK-1497 A6 this is a *selection*, not the
  // only session there is: the serve has been multi-session since GDK-864,
  // and this was the last thing on the phone that assumed otherwise.
  //
  // Reactive, and module-scoped: reactive because the header and the sheet
  // now render *from* it, so a plain `let` would leave both showing the
  // previous shell until some other signal happened to re-run them; module
  // -scoped because it must outlive the component, which unmounts on a tab
  // switch. One owner of the answer, and the phone mounts one Shell.
  let keptSessionId = $state<string | null>(null)
</script>

<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import Screen from '../ui/Screen.svelte'
  import KeyBar from '../ui/KeyBar.svelte'
  import Sheet from '../ui/Sheet.svelte'
  import { t } from '../lib/i18n'
  import { app, terminalSession } from '../lib/store.svelte'
  import {
    createShellSession,
    TERMINAL_CURSOR_BLINK_FALLBACK,
    TERMINAL_SCROLLBACK_FALLBACK,
  } from '../lib/terminal/api'
  import {
    bindSessionIssue,
    deleteSession,
    listSessions,
    nextSelectedAfterKill,
    placeSessionInput,
    renameSession,
    ROSTER_POLL_MS,
    sessionLabel,
    stripRows,
    type StripRow,
    type TerminalSessionInfo,
  } from '../lib/terminal/sessions'
  import { openShellSocket } from '../lib/terminal/transport'
  import {
    StickyModifiers,
    bytesForBarKey,
    bytesForText,
    encoderMods,
    modifierIdForBarKey,
    stepsForBarKey,
    stickySlots,
    type BarKey,
    type ModifierId,
    type StickySlots,
  } from '../lib/terminal/keys'
  import { imeReduce, IME_INPUT_ATTRS, type ImeState } from '../lib/terminal/ime'
  import { createRenderer, type PhoneTerminalRenderer } from '../lib/terminal/renderer'
  import { scrollGesture } from '../lib/terminal/scroll-gesture'
  import { settleResize } from '../../../web/src/lib/terminal/resize'
  import {
    classifyUnavailable,
    coerceDroppedReason,
    droppedAllowsRestart,
    unavailableAllowsRestart,
    UNAVAILABLE_KEYS,
  } from '../../../web/src/lib/terminal/protocol'
  import type {
    DroppedReason,
    SocketHandle,
    UnavailableCause,
  } from '../../../web/src/lib/terminal/protocol'
  import { ApiError } from '../lib/api'

  // Copied from web/src/lib/terminal/session.ts — that module imports the
  // wails transport, which the phone must not resolve. The behavior
  // fallbacks are not copies: they live next to the response they
  // normalize, in ../lib/terminal/api (imported above).
  const TERMINAL_GRACE_MS = 60_000
  const TERMINAL_RECONNECT_BACKOFF_MS = [500, 1000, 2000, 4000] as const
  const TERMINAL_WS_OPEN_MS = 8_000

  type Status =
    | { kind: 'none' }
    | { kind: 'connecting' }
    | { kind: 'reconnecting' }
    | { kind: 'exited'; code: number }
    | { kind: 'dropped'; reason: DroppedReason }
    | { kind: 'unavailable'; cause: UnavailableCause; detail?: string }

  const DROPPED_KEYS: Record<DroppedReason, 'terminal.dropped.slow_client' | 'terminal.dropped.token_revoked' | 'terminal.dropped.idle_timeout' | 'terminal.dropped.server_shutdown' | 'terminal.dropped.closed'> = {
    slow_client: 'terminal.dropped.slow_client',
    token_revoked: 'terminal.dropped.token_revoked',
    idle_timeout: 'terminal.dropped.idle_timeout',
    server_shutdown: 'terminal.dropped.server_shutdown',
    closed: 'terminal.dropped.closed',
  }

  let hostEl = $state<HTMLElement | null>(null)
  let imeEl = $state<HTMLTextAreaElement | null>(null)
  let status = $state<Status>({ kind: 'connecting' })
  let attached = $state(false)
  const sticky = new StickyModifiers()
  let mods = $state<StickySlots>(stickySlots(sticky))

  function syncMods() {
    mods = stickySlots(sticky)
  }

  const heading = $derived(machineName())

  /* ── The roster (GDK-1497 A6) ────────────────────────────────────────────
   *
   * The serve has answered GET sessions/ since GDK-864 and the phone never
   * asked. It asks now, and the answer is what the header and the sheet are
   * both derived from — there is no second copy of "what shells exist".
   *
   * One socket at a time stays the rule (DESIGN.md §10: one pane, one
   * keyboard, one thumb). Switching closes the current stream and attaches
   * the chosen one; the serve's 256 KiB reconnect ring is replayed on
   * attach, so what comes back is the tail of that session, not a local
   * per-session scrollback. The phone keeps one xterm instance and resets it
   * on every attach, so scrollback *older* than the ring does not survive a
   * switch — the renderer has no per-session buffer to keep it in, and
   * giving it one is a bigger change than this round (reported).
   */
  let roster = $state<TerminalSessionInfo[]>([])
  /** False until a list has come back — "no shells" must not paint at boot. */
  let rosterLoaded = $state(false)
  let sheetOpen = $state(false)
  /** Which row has its action drawer open, and which verb it is showing. */
  let openRowId = $state<string | null>(null)
  let rowMode = $state<'menu' | 'rename' | 'issue' | 'send' | 'kill'>('menu')
  let draft = $state('')
  /** One line of feedback under the open row: a placement, or a refusal. */
  let rowNotice = $state<string | null>(null)
  let rowBusy = $state(false)
  /** Ticks the rows' relative state (running / quiet) while the sheet is up. */
  let rosterAt = $state(Date.now())

  const defaultName = (n: number): string => t('terminal.strip.defaultName', { n })

  /** The four row states, in the desktop's words (strip.ts decides which). */
  const STATE_KEYS = {
    needs: 'terminal.strip.state.needs',
    running: 'terminal.strip.state.running',
    quiet: 'terminal.strip.state.quiet',
    ghost: 'terminal.strip.state.ghost',
  } as const

  const rows = $derived<StripRow[]>(stripRows(roster, keptSessionId, rosterAt, defaultName))

  /** The shown session's row, when the roster has caught up with it. */
  const current = $derived(roster.find((s) => s.id === keptSessionId) ?? null)
  const currentLabel = $derived(current ? sessionLabel(current, defaultName) : null)
  const currentIssue = $derived(current?.issue_key?.trim() || null)

  /**
   * The issue the app has open, when it has one — the desktop binds by
   * opening a shell *from* an issue and this is the phone's nearest thing
   * (store.svelte.ts `app.detail`). In practice it is almost always null
   * here: the detail layer paints over the whole tab column (App.svelte), so
   * reaching the Terminal tab means the detail was closed, and closeIssue()
   * nulls it. It is read anyway rather than invented — when it is set, the
   * key field opens already filled — and the missing piece (a last-viewed
   * issue the store remembers) is reported, not added to the store here.
   */
  const openIssueKey = $derived(app.detail?.kind === 'issue' ? app.detail.key : null)

  async function refreshRoster(): Promise<void> {
    try {
      roster = await listSessions(terminalSession())
      rosterLoaded = true
      rosterAt = Date.now()
    } catch {
      // Keep the last roster. The pane's own socket is the authority on
      // whether the host is reachable; a list that lost one race must not
      // blank the sheet under a thumb.
    }
  }

  function machineName(): string {
    const label = app.terminal?.label?.trim()
    if (label) return label
    const endpoint = app.terminal?.endpoint || app.meta?.endpoint || ''
    if (endpoint === '') return app.meta?.label || 'this machine (dev proxy)'
    try {
      return new URL(endpoint).host
    } catch {
      return endpoint
    }
  }

  let renderer: PhoneTerminalRenderer | null = null
  let socket: SocketHandle | null = null
  /**
   * Which attachment is the pane's (GDK-1497 A6). Every socket callback
   * closes over the id it was opened for, so a socket the pane has left
   * behind still knows how to reconnect itself — and before switching
   * existed there was only ever one, so nothing had to say which. With a
   * switch there always are two for a moment: the old close arrives after
   * the new attach, `phase` is 'live', and the old handler would schedule a
   * reconnect to the session the person just navigated away from. This
   * counter is the single owner of "is this socket still ours"; every
   * handler below reads it before touching pane state.
   */
  let socketSeq = 0
  let ro: ResizeObserver | null = null
  let stopSettle: (() => void) | null = null
  let fitTimer: ReturnType<typeof setTimeout> | undefined
  let openTimer: ReturnType<typeof setTimeout> | undefined
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined
  let reconnectAttempt = 0
  let reconnectSince = 0
  let lastCols = 0
  let lastRows = 0
  let phase: 'idle' | 'live' | 'ended' | 'unavailable' | 'reconnecting' = 'idle'
  let cancelled = false
  let actSeq = 0
  let ime: ImeState = { composing: false }
  let lastComposeEmit = ''

  const clearTimers = () => {
    stopSettle?.()
    stopSettle = null
    if (fitTimer !== undefined) clearTimeout(fitTimer)
    if (openTimer !== undefined) clearTimeout(openTimer)
    if (reconnectTimer !== undefined) clearTimeout(reconnectTimer)
    fitTimer = undefined
    openTimer = undefined
    reconnectTimer = undefined
  }

  const detachSocket = () => {
    // Bump first: close() can call back synchronously, and a stale handler
    // that runs before the counter moves is exactly the race this closes.
    socketSeq += 1
    socket?.close()
    socket = null
    attached = false
    if (typeof window !== 'undefined') delete window.__gadakShellDrop
  }

  const fittedSize = (): { cols: number; rows: number } => {
    renderer?.fit()
    const cols = renderer?.cols || 80
    const rows = renderer?.rows || 24
    return { cols, rows }
  }

  /**
   * A hidden pane has no size to report. The Shell lives inside the tabs
   * column and is display:none while another tab is up (App.svelte), and
   * xterm's fit on a zero box answers with its floor — 10x5 — which the
   * pane then shipped to the PTY as gospel. Measured (GDK-1154): a session
   * created correctly at 48x34 ended the take at 10x5 with resizes=1, the
   * one resize being the tour switching back to the Issues tab. The size
   * the child sees does not change because a tab did.
   */
  const paneLaidOut = (): boolean => !!hostEl && hostEl.clientWidth > 0 && hostEl.clientHeight > 0

  // The single owner of "tell the server how big the pane is" (GDK-1154).
  // lastCols/lastRows mean "what the server was last told", so they advance
  // here and nowhere else — a cache that runs ahead of an actual send turns
  // every later check into a false negative.
  const sendResize = () => {
    if (!renderer || !socket || phase !== 'live') return
    if (!paneLaidOut()) return
    const { cols, rows } = fittedSize()
    if (cols === lastCols && rows === lastRows) return
    lastCols = cols
    lastRows = rows
    socket.resize(cols, rows)
  }

  const scheduleFit = () => {
    if (fitTimer !== undefined) clearTimeout(fitTimer)
    fitTimer = setTimeout(() => sendResize(), 100)
  }

  function sendBytes(bytes: Uint8Array) {
    // ended *and* unavailable: the session is gone; Enter/tap starts a new
    // one (DESIGN.md §10.4 — the desktop pane's ended-state contract).
    if (phase === 'ended' || phase === 'unavailable') {
      const enter = bytes.length === 1 && (bytes[0] === 13 || bytes[0] === 10)
      if (enter) {
        // No PTY on the host and no shell for a revoked token: a restart
        // cannot succeed, so it is not offered and not performed.
        if (status.kind === 'unavailable' && !unavailableAllowsRestart(status.cause)) return
        if (status.kind === 'dropped' && !droppedAllowsRestart(status.reason)) return
        status = { kind: 'connecting' }
        phase = 'live'
        reconnectSince = 0
        reconnectAttempt = 0
        void startNew().catch(onCreateFail)
      }
      return
    }
    if (phase !== 'live') return
    if (bytes.length === 0) return
    socket?.send(bytes)
  }

  function sendText(text: string) {
    sendBytes(bytesForText(text, encoderMods(sticky.activeModifiers())))
    sticky.consume()
    syncMods()
  }

  function onBarKey(key: BarKey) {
    // The panic exit (GDK-953): every slot to idle, no bytes. glasskeys'
    // contract — "Any UI that offers lock must also offer this" — and armed
    // had no single-gesture way back. Before the modifier branch so a
    // future BarKey can never reach the encoder as an emission.
    if (key === 'clear') {
      sticky.clear()
      syncMods()
      imeEl?.focus()
      return
    }
    const mod = modifierIdForBarKey(key)
    if (mod) {
      sticky.tap(mod, Date.now())
      syncMods()
      imeEl?.focus()
      return
    }
    const steps = stepsForBarKey(key, ime.composing, sticky.activeModifiers())
    for (const step of steps) {
      if (step.op === 'commit-marked') {
        // Composition flush only: the bar key is the emission that spends
        // an armed slot. Marked text goes out with no modifiers.
        flushIme({ kind: 'compositionend', data: imeEl?.value ?? '' }, { spend: false, mods: [] })
      } else if (step.op === 'emit-key') {
        // The renderer, not a default: DECCKM is whatever the application
        // running right now set it to (GDK-899). No renderer means no
        // application, so 'normal' is the honest answer there.
        sendBytes(
          bytesForBarKey(key, encoderMods(step.mods), renderer?.cursorKeyMode() ?? 'normal'),
        )
        sticky.consume()
        syncMods()
      }
    }
    imeEl?.focus()
  }

  function flushIme(
    ev:
      | { kind: 'compositionstart' }
      | { kind: 'compositionupdate'; data: string }
      | { kind: 'compositionend'; data: string }
      | { kind: 'input'; data: string; isComposing: boolean },
    opts?: { spend?: boolean; mods?: readonly ModifierId[] },
  ) {
    const active = opts?.mods ?? sticky.activeModifiers()
    const spend = opts?.spend ?? true
    const out = imeReduce(ime, ev, active)
    ime = out.state
    if (ev.kind === 'compositionend') {
      lastComposeEmit = out.emit
      if (out.emit) {
        sendBytes(bytesForText(out.emit, encoderMods(active)))
        if (spend) {
          sticky.consume()
          syncMods()
        }
      }
      if (imeEl) imeEl.value = ''
      return
    }
    // Chrome fires input after compositionend with the same data; a PTY
    // would type the syllable twice if we forwarded both (ime.ts header).
    if (ev.kind === 'input' && lastComposeEmit && ev.data === lastComposeEmit) {
      lastComposeEmit = ''
      return
    }
    lastComposeEmit = ''
    if (out.emit) {
      sendBytes(bytesForText(out.emit, encoderMods(active)))
      if (spend) {
        sticky.consume()
        syncMods()
      }
      if (imeEl) imeEl.value = ''
    }
  }

  function onImeKeydown(e: KeyboardEvent) {
    if (ime.composing || e.isComposing) return
    if (e.key === 'Enter') {
      e.preventDefault()
      sendText('\r')
      if (imeEl) imeEl.value = ''
      return
    }
    if (e.key === 'Backspace') {
      e.preventDefault()
      sendBytes(new Uint8Array([0x7f]))
      sticky.consume()
      syncMods()
      return
    }
    if (e.key === 'Tab') {
      e.preventDefault()
      onBarKey('tab')
    }
  }

  async function startNew(): Promise<void> {
    const { cols, rows } = fittedSize()
    lastCols = cols
    lastRows = rows
    const doc = await createShellSession(cols, rows, terminalSession())
    // The create response is the one road terminal behavior reaches the
    // pane on (GDK-896 R3, the web pane does the same): apply before
    // attaching, so the ring replay lands in a buffer already sized to
    // the configured scrollback.
    renderer?.applyBehavior({ scrollback: doc.scrollback, cursorBlink: doc.cursorBlink })
    keptSessionId = doc.id
    attachSocket(doc.id, { afterCreate: true })
    void refreshRoster()
  }

  function attachSocket(id: string, opts: { afterCreate: boolean; recreateOnFail?: boolean }): void {
    detachSocket()
    let opened = false
    phase = 'live'
    // This attachment's ticket. `mine()` is false the moment anything else
    // detaches or attaches — a switch, a tab leaving, an unmount — and every
    // handler below leads with it, so a socket the pane no longer holds can
    // neither paint into the renderer nor schedule its own return.
    const mySeq = ++socketSeq
    const mine = (): boolean => mySeq === socketSeq
    const session = terminalSession()
    const handle = openShellSocket(
      id,
      {
        onOpen() {
          if (!mine()) return
          opened = true
          if (openTimer !== undefined) {
            clearTimeout(openTimer)
            openTimer = undefined
          }
          attached = true
          reconnectAttempt = 0
          reconnectSince = 0
          if (status.kind === 'reconnecting' || status.kind === 'connecting') {
            status = { kind: 'none' }
          }
          renderer?.reset()
          // A new socket has been told nothing, so the cache — "what the
          // server was told" — is empty for it; then the one owner does the
          // sending, guard and all. This used to call handle.resize() with
          // its own unguarded fittedSize(), which is how a hidden pane's
          // 10x5 reached the PTY through the side door on a late reattach
          // (GDK-1154; the guarded path had already refused it).
          lastCols = 0
          lastRows = 0
          sendResize()
          // …and again across the window in which layout settles: one read
          // at open used to be the last word on the size for the life of the
          // session (lib/terminal/resize.ts). sendResize no-ops once the
          // sizes agree, and refuses while the pane has no box.
          stopSettle?.()
          stopSettle = settleResize(sendResize)
        },
        onBytes(data) {
          if (!mine()) return
          renderer?.write(data)
        },
        onExit(code) {
          if (!mine()) return
          phase = 'ended'
          status = { kind: 'exited', code }
          keptSessionId = null
          void refreshRoster()
        },
        onDropped(reason) {
          if (!mine()) return
          phase = 'ended'
          status = { kind: 'dropped', reason: coerceDroppedReason(reason) }
          if (reason === 'token_revoked' || reason === 'server_shutdown' || reason === 'idle_timeout') {
            keptSessionId = null
          }
        },
        onClose(neverOpened) {
          if (!mine()) return
          attached = false
          if (cancelled || phase === 'ended' || phase === 'unavailable') return
          if (document.visibilityState === 'hidden') {
            phase = 'reconnecting'
            status = { kind: 'reconnecting' }
            return
          }
          if (neverOpened && opts.recreateOnFail) {
            keptSessionId = null
            void startNew().catch(onCreateFail)
            return
          }
          if (neverOpened && opts.afterCreate) {
            // The POST was accepted, so this is not a permission verdict:
            // the socket itself never came up.
            phase = 'unavailable'
            status = { kind: 'unavailable', cause: 'network' }
            keptSessionId = null
            return
          }
          scheduleReconnect(id)
        },
      },
      { endpoint: session.endpoint, token: session.token },
    )
    socket = handle
    if (typeof window !== 'undefined') {
      window.__gadakShellDrop = () => handle.close()
    }
    if (openTimer !== undefined) clearTimeout(openTimer)
    openTimer = setTimeout(() => {
      if (opened || cancelled) return
      handle.close()
    }, TERMINAL_WS_OPEN_MS)
  }

  function scheduleReconnect(id: string): void {
    if (reconnectSince === 0) reconnectSince = Date.now()
    if (Date.now() - reconnectSince >= TERMINAL_GRACE_MS) {
      phase = 'ended'
      status = { kind: 'dropped', reason: 'idle_timeout' }
      keptSessionId = null
      return
    }
    phase = 'reconnecting'
    status = { kind: 'reconnecting' }
    const delay =
      TERMINAL_RECONNECT_BACKOFF_MS[
        Math.min(reconnectAttempt, TERMINAL_RECONNECT_BACKOFF_MS.length - 1)
      ]
    reconnectAttempt += 1
    reconnectTimer = setTimeout(() => {
      if (cancelled) return
      attachSocket(id, { afterCreate: false })
    }, delay)
  }

  // The phone's adapter onto the shared classifier. A scope_rejected here is
  // the common one: a serve QR scanned into the terminal slot.
  function onCreateFail(err?: unknown): void {
    if (cancelled) return
    phase = 'unavailable'
    if (err instanceof ApiError) {
      const cause = classifyUnavailable(err.status, err.code)
      status =
        cause === 'failed'
          ? { kind: 'unavailable', cause, detail: err.message }
          : { kind: 'unavailable', cause }
    } else {
      status = { kind: 'unavailable', cause: 'network' }
    }
    keptSessionId = null
  }

  /* ── The session verbs (GDK-1497 A6) ─────────────────────────────────────
   *
   * Every one of these goes through lib/terminal/sessions.ts, which is the
   * single owner of the routes and of the bodies. Nothing here builds a URL
   * or names a wire field; this file only decides what the pane does with
   * the answer.
   */

  /**
   * Move the pane onto a session. One socket: the current stream ends first,
   * and the epoch above keeps the outgoing one from reconnecting itself.
   */
  function attachTo(id: string): void {
    if (id === keptSessionId && attached) return
    clearTimers()
    reconnectAttempt = 0
    reconnectSince = 0
    keptSessionId = id
    status = { kind: 'connecting' }
    phase = 'live'
    // afterCreate:false — the POST was somebody else's, possibly weeks of
    // uptime ago, so a socket that never opens here is a reconnect case and
    // not a "the host refused to start a shell" verdict.
    attachSocket(id, { afterCreate: false })
  }

  /** A row was tapped: show that session, and get out of the way. */
  function switchTo(id: string): void {
    closeSheet()
    attachTo(id)
  }

  /** `+` — a new shell, and the pane moves to it. */
  async function createAndSwitch(): Promise<void> {
    closeSheet()
    clearTimers()
    detachSocket()
    reconnectAttempt = 0
    reconnectSince = 0
    keptSessionId = null
    status = { kind: 'connecting' }
    phase = 'live'
    try {
      await startNew()
    } catch (err) {
      onCreateFail(err)
    }
  }

  /**
   * End a session on purpose. The selection moves first and synchronously —
   * to the right-hand neighbour when the ended one is the shown one — so the
   * pane has already left before the shell is torn down (the desktop's
   * nextSelectedAfterKill, shared). Killing the last one leaves the pane on
   * its own ended line rather than an empty sheet with nothing to say.
   */
  async function killRow(id: string): Promise<void> {
    const next = nextSelectedAfterKill(
      roster.map((s) => s.id),
      id,
      keptSessionId,
    )
    const wasShown = id === keptSessionId
    if (wasShown && next === null) {
      // Set the end state before the socket closes: the close handler reads
      // `phase` to decide whether to reconnect, and this shell is not coming
      // back.
      phase = 'ended'
      status = { kind: 'dropped', reason: 'closed' }
      keptSessionId = null
      clearTimers()
      detachSocket()
    }
    const ended = await run(id, () => deleteSession(id, terminalSession()))
    await refreshRoster()
    // A refusal keeps the drawer open on its own line: closing it here would
    // wipe the only thing that says the shell is still running.
    if (!ended) return
    // The sheet stays open: ending one shell of several is a housekeeping
    // gesture, and closing the list under the thumb that just used it is the
    // opposite of what the next tap wants.
    closeRow()
    if (wasShown && next !== null) attachTo(next)
  }

  async function renameRow(id: string): Promise<void> {
    const name = draft
    if (await run(id, () => renameSession(id, name, terminalSession()))) {
      await refreshRoster()
      closeRow()
    }
  }

  async function bindRow(id: string, key: string): Promise<void> {
    if (await run(id, () => bindSessionIssue(id, key, terminalSession()))) {
      await refreshRoster()
      closeRow()
    }
  }

  /**
   * Place a line in a shell without being its socket (GDK-1162).
   *
   * This is the one verb here that is not a convenience: the phone holds a
   * single socket, so a line for a session it is not showing has no other
   * road. Keystrokes are unaffected — they keep going over the socket, and
   * this route refuses a newline, so Enter stays a thing a person presses in
   * front of the line they can read.
   */
  async function sendRow(id: string, name: string): Promise<void> {
    const text = draft
    if (await run(id, () => placeSessionInput(id, text, terminalSession()))) {
      draft = ''
      rowNotice = t('terminal.strip.sendPlaced', { name })
      await refreshRoster()
    }
  }

  /** One in-flight verb, with the refusal surfaced on the row that asked. */
  async function run(id: string, verb: () => Promise<unknown>): Promise<boolean> {
    rowBusy = true
    rowNotice = null
    try {
      await verb()
      return true
    } catch (err) {
      // The server's own words when it sent any (`message` on failMsg —
      // input_not_a_line explains itself better than this app could), the
      // generic line otherwise. Never the raw code, never the token.
      const detail = err instanceof ApiError ? err.serverMessage : null
      rowNotice = detail?.trim() || t('terminal.strip.failed')
      openRowId = id
      return false
    } finally {
      rowBusy = false
    }
  }

  function openSheet(): void {
    sheetOpen = true
    closeRow()
    void refreshRoster()
  }

  function closeSheet(): void {
    sheetOpen = false
    closeRow()
  }

  function closeRow(): void {
    openRowId = null
    rowMode = 'menu'
    draft = ''
    rowNotice = null
  }

  /** Open a row's drawer on one verb, with the field primed for it. */
  function openRow(row: StripRow, mode: typeof rowMode): void {
    openRowId = row.id
    rowMode = mode
    rowNotice = null
    const info = roster.find((s) => s.id === row.id)
    if (mode === 'rename') draft = info?.name?.trim() ?? ''
    else if (mode === 'issue') draft = info?.issue_key?.trim() || openIssueKey || ''
    else draft = ''
  }

  async function ensureRenderer(): Promise<boolean> {
    if (renderer) return true
    if (!hostEl) return false
    renderer = await createRenderer()
    if (cancelled) {
      renderer.dispose()
      renderer = null
      return false
    }
    renderer.open(hostEl)
    // Behavior starts at the server's default values (GDK-896 R3); a
    // create response overrides them in startNew. The kept-session paths
    // (activate/reattachNow) never create, so without this a reattach
    // would run on xterm's own 1000-line default until the next fresh
    // session — the web pane's boot path applies the same fallback.
    renderer.applyBehavior({
      scrollback: TERMINAL_SCROLLBACK_FALLBACK,
      cursorBlink: TERMINAL_CURSOR_BLINK_FALLBACK,
    })
    renderer.fit()
    // Route xterm's own resize through the single sender rather than
    // repeating it here. The duplicate used to advance lastCols/lastRows
    // and *then* `socket?.resize(...)` — which is a no-op before the socket
    // is live, so the cache recorded a size the server had never been told.
    // Every later check compared against that phantom, found no change, and
    // sent nothing: the PTY kept the pre-layout size for the life of the
    // session (measured 2026-08-29 on the iOS simulator — cols 10 rows 5
    // under a pane rendering 48x34, which is the size SIGWINCH hands every
    // TUI in the pane). One writer, and it advances only after a send.
    renderer.onResize(() => sendResize())
    ro = new ResizeObserver(scheduleFit)
    ro.observe(hostEl)
    return true
  }

  async function activate(): Promise<void> {
    const seq = ++actSeq
    // untrack, and not by taste: activate() is called from the $effect below,
    // so any $state it *reads* becomes that effect's dependency. Reading
    // `status` here subscribed the attach effect to the very field attaching
    // updates — onOpen sets status to 'none', the effect re-runs, actSeq bumps,
    // and attachSocket() detaches the socket that had just opened. The pane sat
    // at connecting → reconnecting forever (shell.spec.ts, 5 tests). Writing a
    // fresh {kind:'connecting'} on each pass made the same cycle synchronous
    // and fatal: effect_update_depth_exceeded killed the whole pane on the
    // first tap of the Terminal tab.
    untrack(() => {
      if (status.kind !== 'reconnecting' && status.kind !== 'connecting') {
        status = { kind: 'connecting' }
      }
    })
    if (!(await ensureRenderer())) return
    if (seq !== actSeq || cancelled) return
    // The header names the session, not just the machine, so the roster is
    // read once on every activation — not only when the sheet opens.
    void refreshRoster()
    try {
      if (keptSessionId) {
        attachSocket(keptSessionId, { afterCreate: false, recreateOnFail: true })
      } else {
        await startNew()
      }
    } catch {
      if (seq !== actSeq) return
      onCreateFail()
    }
  }

  function reattachNow(): void {
    if (cancelled || phase === 'ended' || phase === 'unavailable') return
    if (attached && socket) return
    const id = keptSessionId
    if (!id) return
    if (reconnectTimer !== undefined) {
      clearTimeout(reconnectTimer)
      reconnectTimer = undefined
    }
    reconnectAttempt = 0
    attachSocket(id, { afterCreate: false, recreateOnFail: true })
  }

  function onVisibility(): void {
    if (document.visibilityState !== 'visible') return
    if (app.tab !== 'shell') return
    reattachNow()
  }

  function onStatusActivate(): void {
    if (status.kind === 'exited' || status.kind === 'dropped' || status.kind === 'unavailable') {
      sendBytes(new Uint8Array([13]))
    }
  }

  function focusIme(): void {
    imeEl?.focus()
  }

  // A tap on the terminal focuses the IME (existing behaviour); a drag must
  // not, so pointerdown records whether the keyboard was already up before
  // focusing — that provenance is what lets a later drag undo exactly this
  // focus and never one the user brought up themselves.
  function onHostPointerDown(): void {
    imeHadFocus = document.activeElement === imeEl
    focusIme()
  }

  // --- Touch scroll (GDK-899) ---------------------------------------------
  // Ported from orca's surface gesture handlers (terminal-webview-html.ts):
  // touch pixels accumulate in a sub-row offset and only whole rows are
  // committed, so a slow drag keeps its fraction of a row instead of losing
  // it at each commit. Sign: deltaY is `lastY - y`, so a downward finger
  // pull (older rows revealed) is negative — the convention of xterm's
  // scrollLines and of scroll-gesture.ts. The routing decision itself is the
  // frozen pure module; this block only converts pixels to lines and
  // dispatches the module's result.
  const TAP_SLOP_PX = 8
  const SCROLL_FRICTION = 0.972
  const SCROLL_MIN_VEL_PX_PER_MS = 0.012
  const SCROLL_FRAME_MS = 16
  const SCROLL_INDICATOR_HIDE_MS = 550

  let touchStartY = 0
  let touchLastY = 0
  let touchLastTime = 0
  let touchAccumPx = 0
  let scrollVelocity = 0
  let touchMoved = false
  let touchDead = false
  let imeHadFocus = true
  let momentumId: number | null = null
  let scrollHideTimer: ReturnType<typeof setTimeout> | undefined
  // What the last dispatched gesture was — decides indicator thumb vs hint
  // without re-deriving the module's predicate over here.
  let lastScrollKind: 'none' | 'scrollback' | 'inject' | 'hint' = 'none'

  let scrollTrackEl = $state<HTMLElement | null>(null)
  let scrollThumbEl = $state<HTMLElement | null>(null)
  let scrollBadgeEl = $state<HTMLElement | null>(null)

  function scrollCellHeight(): number {
    const rows = renderer?.rows || 24
    const h = hostEl?.clientHeight ?? 0
    return rows > 0 && h > 0 ? h / rows : 0
  }

  function dispatchScrollLines(lines: number): void {
    if (!renderer || lines === 0) return
    const cols = renderer.cols || 80
    const rows = renderer.rows || 24
    const g = scrollGesture(lines, {
      buffer: renderer.bufferType(),
      mouse: renderer.mouseTrackingMode(),
      // Centre cell: the module only needs it encodable (1..9999) — the
      // gesture has no meaningful position of its own.
      cell: { col: Math.max(1, Math.ceil(cols / 2)), row: Math.max(1, Math.ceil(rows / 2)) },
    })
    lastScrollKind = g.kind
    if (g.kind === 'scrollback') renderer.scrollLines(g.lines)
    else if (g.kind === 'inject') {
      sendBytes(g.bytes)
      // A wheel report in the alternate screen: if the TUI honoured it, it
      // painted its own scroll; if it ignored it (crush), the arrow keys are
      // the recourse, so surface the same hint as the no-wheel case.
      if (g.hint) lastScrollKind = 'hint'
    }
    updateScrollIndicator()
  }

  function commitWholeRows(): void {
    const cellH = scrollCellHeight()
    if (cellH <= 0) return
    const lines = Math.trunc(touchAccumPx / cellH)
    if (lines === 0) return
    touchAccumPx -= lines * cellH
    dispatchScrollLines(lines)
  }

  function updateTouchVelocity(deltaY: number, dtMs: number): void {
    if (dtMs <= 0) return
    const v = deltaY / dtMs
    if (!Number.isFinite(v)) return
    // Blend samples (orca): touchmove cadence is uneven, so momentum must
    // not inherit a one-frame spike or stall.
    scrollVelocity = scrollVelocity === 0 ? v : scrollVelocity * 0.55 + v * 0.45
  }

  function stopMomentum(): void {
    if (momentumId !== null) {
      cancelAnimationFrame(momentumId)
      momentumId = null
    }
  }

  function onTouchStart(e: TouchEvent): void {
    stopMomentum()
    touchAccumPx = 0
    scrollVelocity = 0
    touchMoved = false
    // A second finger ends the gesture rather than starting a pinch: there is
    // no zoom surface here (orca scales a transformed surface; gadak's xterm
    // sits in the webview directly).
    if (e.touches.length !== 1) {
      touchDead = true
      return
    }
    touchDead = false
    touchStartY = e.touches[0].clientY
    touchLastY = e.touches[0].clientY
    touchLastTime = Date.now()
  }

  function onTouchMove(e: TouchEvent): void {
    if (touchDead || e.touches.length !== 1) return
    if (!renderer || !hostEl) return
    e.preventDefault()
    const y = e.touches[0].clientY
    const now = Date.now()
    const deltaY = touchLastY - y
    if (!touchMoved && Math.abs(y - touchStartY) > TAP_SLOP_PX) {
      touchMoved = true
      // Undo this gesture's tap-focus: pointerdown runs before the first
      // move can prove the touch is a drag, so without this a scroll started
      // from a keyboard-down state would raise the keyboard mid-swipe.
      if (!imeHadFocus) imeEl?.blur()
    }
    if (!touchMoved) {
      touchLastY = y
      touchLastTime = now
      return
    }
    updateTouchVelocity(deltaY, now - touchLastTime)
    touchLastY = y
    touchLastTime = now
    touchAccumPx += deltaY
    commitWholeRows()
  }

  function onTouchEnd(e: TouchEvent): void {
    if (e.touches.length > 0) return
    const moved = touchMoved
    touchDead = false
    if (!moved || Math.abs(scrollVelocity) <= SCROLL_MIN_VEL_PX_PER_MS) return
    momentumId = requestAnimationFrame(momentumStep)
  }

  function onTouchCancel(): void {
    stopMomentum()
    scrollVelocity = 0
    touchDead = false
    touchMoved = false
  }

  function momentumStep(): void {
    momentumId = null
    scrollVelocity *= SCROLL_FRICTION
    if (Math.abs(scrollVelocity) < SCROLL_MIN_VEL_PX_PER_MS) return
    touchAccumPx += scrollVelocity * SCROLL_FRAME_MS
    commitWholeRows()
    momentumId = requestAnimationFrame(momentumStep)
  }

  // Transient scroll affordance (GDK-899 decision 3), geometry ported from
  // orca's #scroll-indicator. A swipe on the phone terminal is a pure scroll
  // gesture — it never injects arrows, because the arrow keys are their own
  // dedicated bar (keys.ts). So there are three outcomes to surface:
  //   scrollback → a position thumb from the live viewport (normal buffer);
  //   hint       → an "arrow keys scroll this" badge (an alternate-screen TUI
  //                with no scrollback and no wheel target — touch cannot move
  //                it, the ↑↓ keys can);
  //   inject     → nothing (a wheel report went to a mouse-aware TUI, which
  //                paints its own scroll; a local overlay would just lie).
  function updateScrollIndicator(): void {
    const track = scrollTrackEl
    const thumb = scrollThumbEl
    const badge = scrollBadgeEl
    if (!track || !thumb || !badge || lastScrollKind === 'none') return
    if (lastScrollKind === 'inject') {
      thumb.style.display = 'none'
      badge.style.display = 'none'
      track.classList.remove('visible')
      return
    }
    if (lastScrollKind === 'hint') {
      thumb.style.display = 'none'
      badge.style.display = ''
      badge.textContent = '↑↓'
    } else {
      badge.style.display = 'none'
      thumb.style.display = ''
      if (hostEl) {
        const vp = renderer?.viewport() ?? { viewportY: 0, baseY: 0 }
        const rows = renderer?.rows || 24
        const trackH = Math.max(0, hostEl.clientHeight - 8)
        const totalRows = vp.baseY + rows
        if (trackH > 0 && vp.baseY > 0 && totalRows > 0) {
          const thumbH = Math.max(24, (trackH * rows) / totalRows)
          const maxTop = Math.max(0, trackH - thumbH)
          thumb.style.height = `${thumbH}px`
          thumb.style.transform = `translateY(${(vp.viewportY / vp.baseY) * maxTop}px)`
        }
      }
    }
    track.classList.add('visible')
    if (scrollHideTimer !== undefined) clearTimeout(scrollHideTimer)
    scrollHideTimer = setTimeout(() => {
      track.classList.remove('visible')
      scrollHideTimer = undefined
    }, SCROLL_INDICATOR_HIDE_MS)
  }

  $effect(() => {
    if (app.tab !== 'shell') return
    const el = hostEl
    if (!el) return
    // Non-passive touchmove: preventDefault is what keeps the webview's own
    // rubber-banding out of the gesture (orca attaches the same way).
    el.addEventListener('touchstart', onTouchStart, { passive: true })
    el.addEventListener('touchmove', onTouchMove, { passive: false })
    el.addEventListener('touchend', onTouchEnd, { passive: true })
    el.addEventListener('touchcancel', onTouchCancel, { passive: true })
    return () => {
      el.removeEventListener('touchstart', onTouchStart)
      el.removeEventListener('touchmove', onTouchMove)
      el.removeEventListener('touchend', onTouchEnd)
      el.removeEventListener('touchcancel', onTouchCancel)
      stopMomentum()
      // A tab switch inside the 550ms hide window would otherwise leave the
      // indicator stuck visible when the tab comes back.
      if (scrollHideTimer !== undefined) {
        clearTimeout(scrollHideTimer)
        scrollHideTimer = undefined
      }
      scrollTrackEl?.classList.remove('visible')
    }
  })

  $effect(() => {
    if (app.tab !== 'shell') return
    if (!hostEl) return
    void activate()
    return () => {
      actSeq += 1
      detachSocket()
    }
  })

  // The roster poll runs only while the sheet is up (GDK-1497 A6). The
  // desktop polls for as long as its pane is open; this list travels over a
  // tailnet on a battery, and the one row a closed sheet still shows — the
  // attached session's own label — changes only when this device changes it,
  // in which case the verb refreshes.
  $effect(() => {
    if (!sheetOpen) return
    const timer = setInterval(() => void refreshRoster(), ROSTER_POLL_MS)
    return () => clearInterval(timer)
  })

  // A tab switch away closes the sheet. The condition reads app.tab only and
  // never sheetOpen, which this writes: an effect that reads the state it
  // sets is a loop waiting for a second writer, and the guard it would buy
  // is worth nothing — closeSheet is idempotent.
  $effect(() => {
    if (app.tab === 'shell') return
    closeSheet()
  })

  onMount(() => {
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      cancelled = true
      document.removeEventListener('visibilitychange', onVisibility)
      clearTimers()
      ro?.disconnect()
      detachSocket()
      renderer?.dispose()
      renderer = null
    }
  })
</script>

<Screen>
  {#snippet header()}
    <div class="head">
      <h1 class="type-subject">{heading}</h1>
      <!-- The way into the roster, and the only place the pane says which
           shell it is showing. Absent until a list has come back, so it
           never claims "1" before it has asked. -->
      {#if rosterLoaded}
        <button
          type="button"
          class="sessions"
          data-testid="shell-sessions"
          aria-label={t('terminal.strip.list')}
          aria-haspopup="dialog"
          aria-expanded={sheetOpen}
          onclick={openSheet}
        >
          {#if currentLabel}
            <span class="cur">{currentLabel}</span>
          {/if}
          {#if currentIssue && currentIssue !== currentLabel}
            <span class="key">{currentIssue}</span>
          {/if}
          <span class="count">{roster.length}</span>
        </button>
      {/if}
    </div>
  {/snippet}

  <div
    class="body"
    data-testid="terminal-pane"
    data-attached={attached ? 'true' : 'false'}
    data-status={status.kind}
    role="region"
    aria-label={t('terminal.title')}
    aria-busy={status.kind === 'connecting' || status.kind === 'reconnecting'}
  >
    <div class="host-wrap">
      <div
        class="host"
        bind:this={hostEl}
        role="textbox"
        aria-multiline="true"
        aria-label={t('terminal.title')}
        tabindex="0"
        onpointerdown={onHostPointerDown}
        onfocus={focusIme}
      ></div>
      <div class="scroll-indicator" bind:this={scrollTrackEl} aria-hidden="true">
        <div class="scroll-thumb" bind:this={scrollThumbEl}></div>
        <div class="scroll-badge" bind:this={scrollBadgeEl}></div>
      </div>
      {#if status.kind === 'connecting'}
        <div class="connecting" data-testid="terminal-status" role="status">
          <span class="paper"></span>
          <span class="paper short"></span>
          <span class="paper"></span>
        </div>
      {/if}
    </div>
    {#if status.kind === 'reconnecting'}
      <div class="status" data-testid="terminal-status" role="status">
        {t('terminal.reconnecting')}
      </div>
    {:else if status.kind === 'exited' || status.kind === 'dropped' || status.kind === 'unavailable'}
      <button type="button" class="status" data-testid="terminal-status" onclick={onStatusActivate}>
        {#if status.kind === 'exited'}
          {t('terminal.exited', { code: status.code })}
          <span class="hint"> · {t('terminal.restartHint')}</span>
        {:else if status.kind === 'dropped'}
          {t(DROPPED_KEYS[status.reason])}
          {#if droppedAllowsRestart(status.reason)}
            <span class="hint"> · {t('terminal.restartHint')}</span>
          {:else}
            <span class="hint"> · {t('terminal.mintHint')}</span>
          {/if}
        {:else}
          {#if status.cause === 'failed'}
            {t('terminal.unavailable.failed', { message: status.detail ?? '' })}
          {:else}
            {t(UNAVAILABLE_KEYS[status.cause])}
          {/if}
          {#if unavailableAllowsRestart(status.cause)}
            <span class="hint"> · {t('terminal.restartHint')}</span>
          {/if}
        {/if}
      </button>
    {/if}
  </div>

  {#snippet footer()}
    <div class="dock">
      <textarea
        class="ime"
        bind:this={imeEl}
        data-testid="shell-ime"
        {...IME_INPUT_ATTRS}
        oncompositionstart={() => flushIme({ kind: 'compositionstart' })}
        oncompositionupdate={(e) =>
          flushIme({ kind: 'compositionupdate', data: e.data ?? '' })}
        oncompositionend={(e) => flushIme({ kind: 'compositionend', data: e.data ?? '' })}
        oninput={(e) => {
          const ev = e as unknown as InputEvent
          flushIme({
            kind: 'input',
            data: ev.data ?? '',
            isComposing: ev.isComposing,
          })
        }}
        onkeydown={onImeKeydown}
      ></textarea>
      <KeyBar {mods} onkey={onBarKey} />
    </div>
  {/snippet}
</Screen>

<!--
  The session sheet (GDK-1497 A6). Outside <Screen> on purpose: Sheet is
  absolutely positioned and the nearest positioned ancestor is `.tabs`
  (App.svelte), which is the geometry app.css already documents for a sheet
  that ends at the tab bar rather than at the home indicator.

  Every destructive step is inside this sheet, including the confirmation.
  The browser's own blocking dialog is never used anywhere in this app: it is
  unthemed, untranslated by our catalog, and on iOS it reads as the OS asking
  rather than gadak. Ending a shell asks in the row it belongs to.
-->
{#if sheetOpen}
  <Sheet title={t('terminal.strip.list')} onclose={closeSheet}>
    <div class="roster" data-testid="session-sheet">
      <button
        type="button"
        class="new"
        data-testid="session-new"
        onclick={() => void createAndSwitch()}
      >
        <span class="plus" aria-hidden="true">+</span>
        {t('terminal.strip.new')}
      </button>

      {#if rows.length === 0}
        <p class="empty">{t('terminal.strip.empty')}</p>
      {/if}

      {#each rows as row (row.id)}
        <div
          class="entry"
          class:on={row.selected}
          data-testid="session-row"
          data-session-id={row.id}
          data-state={row.state}
        >
          <div class="line">
            <button
              type="button"
              class="pick"
              aria-label={t('terminal.strip.show', { name: row.label })}
              aria-current={row.selected ? 'true' : undefined}
              onclick={() => switchTo(row.id)}
            >
              <span class="dot" data-state={row.state} aria-hidden="true"></span>
              <span class="label" class:bound={row.namedByIssue}>{row.label}</span>
              {#if row.issueAside}
                <span class="key">{row.issueAside}</span>
              {/if}
              <span class="state">{t(STATE_KEYS[row.state])}</span>
            </button>
            <button
              type="button"
              class="more"
              data-testid="session-more"
              aria-label={t('terminal.strip.actions', { name: row.label })}
              aria-expanded={openRowId === row.id}
              onclick={() => (openRowId === row.id ? closeRow() : openRow(row, 'menu'))}
            >
              <span aria-hidden="true">⋯</span>
            </button>
          </div>

          {#if openRowId === row.id}
            <div class="drawer">
              {#if rowMode === 'menu'}
                <div class="verbs">
                  <button
                    type="button"
                    data-testid="verb-rename"
                    aria-label={t('terminal.strip.rename', { name: row.label })}
                    onclick={() => openRow(row, 'rename')}
                  >
                    {t('terminal.strip.renameVerb')}
                  </button>
                  <button type="button" data-testid="verb-issue" onclick={() => openRow(row, 'issue')}>
                    {t('terminal.strip.bind')}
                  </button>
                  <button type="button" data-testid="verb-send" onclick={() => openRow(row, 'send')}>
                    {t('terminal.strip.send')}
                  </button>
                  <button
                    type="button"
                    class="danger"
                    data-testid="verb-kill"
                    aria-label={t('terminal.strip.kill', { name: row.label })}
                    onclick={() => openRow(row, 'kill')}
                  >
                    {t('terminal.strip.killVerb')}
                  </button>
                </div>
              {:else if rowMode === 'kill'}
                <div class="verbs confirm">
                  <span class="ask">{t('terminal.strip.killConfirm')}</span>
                  <button
                    type="button"
                    class="danger"
                    data-testid="session-kill-confirm"
                    disabled={rowBusy}
                    onclick={() => void killRow(row.id)}
                  >
                    {t('terminal.strip.killVerb')}
                  </button>
                  <button type="button" onclick={closeRow}>{t('common.cancel')}</button>
                </div>
              {:else}
                <form
                  class="field"
                  onsubmit={(e) => {
                    e.preventDefault()
                    if (rowMode === 'rename') void renameRow(row.id)
                    else if (rowMode === 'issue') void bindRow(row.id, draft)
                    else void sendRow(row.id, row.label)
                  }}
                >
                  <input
                    bind:value={draft}
                    data-testid="session-field"
                    autocomplete="off"
                    autocapitalize={rowMode === 'issue' ? 'characters' : 'off'}
                    autocorrect="off"
                    spellcheck="false"
                    placeholder={rowMode === 'rename'
                      ? t('terminal.strip.namePlaceholder')
                      : rowMode === 'issue'
                        ? t('terminal.strip.bindPlaceholder')
                        : t('terminal.strip.sendPlaceholder')}
                  />
                  <button type="submit" class="go" data-testid="session-submit" disabled={rowBusy}>
                    {rowMode === 'send' ? t('terminal.strip.send') : t('common.save')}
                  </button>
                  {#if rowMode === 'issue' && row.namedByIssue}
                    <button type="button" disabled={rowBusy} onclick={() => void bindRow(row.id, '')}>
                      {t('terminal.strip.bindClear')}
                    </button>
                  {/if}
                  <button type="button" onclick={closeRow}>{t('common.cancel')}</button>
                </form>
              {/if}
              {#if rowNotice}
                <p class="notice" data-testid="session-notice" role="status">{rowNotice}</p>
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  </Sheet>
{/if}

<style>
  .head {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 12px 0 10px;
    min-width: 0;
  }
  /* The roster's door. Reads as a label, not a button chrome: what it says
     is which shell is on screen, and the count is the only hint that there
     are others. */
  .sessions {
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    max-width: 55%;
    padding: 0 8px;
    border-radius: 6px;
    background: var(--color-bg-elevated);
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
    min-width: 0;
  }
  .sessions .cur {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .sessions .count {
    flex: none;
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-muted);
  }
  .key {
    flex: none;
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    color: var(--color-accent-text);
    white-space: nowrap;
  }

  /* ── The session sheet ── */
  .roster {
    overflow-y: auto;
    padding: 0 8px 8px;
  }
  .new {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 6px 8px;
    color: var(--color-accent-text);
    text-align: left;
  }
  .new .plus {
    font-family: var(--font-mono);
    font-size: var(--text-title);
    line-height: 1;
  }
  .empty {
    margin: 0;
    padding: 12px 8px;
    font-size: var(--text-body);
    color: var(--color-text-muted);
  }
  .entry {
    border-radius: 8px;
  }
  .entry.on {
    background: var(--color-bg-elevated);
  }
  .line {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .pick {
    flex: 1 1 auto;
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    padding: 6px 8px;
    text-align: left;
  }
  .more {
    flex: none;
    width: var(--spacing-control);
    color: var(--color-text-muted);
    font-size: var(--text-title);
    line-height: 1;
  }
  /* The four states carry a colour *and* a shape of their own: `needs` is
     the only one that is a request, so it is the only filled ring. Colour
     alone would be the defect GDK-951 is about, one surface over. */
  .dot {
    flex: none;
    width: 8px;
    height: 8px;
    border-radius: 999px;
    background: var(--color-text-muted);
  }
  .dot[data-state='needs'] {
    background: var(--color-status-reopen);
    box-shadow: 0 0 0 3px var(--color-accent-subtle);
  }
  .dot[data-state='running'] {
    background: var(--color-status-inprogress);
  }
  .dot[data-state='ghost'] {
    background: none;
    box-shadow: inset 0 0 0 1px var(--color-border-strong);
  }
  .label {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
  }
  .label.bound {
    font-family: var(--font-mono);
  }
  .entry.on .label {
    font-weight: 600;
  }
  .state {
    flex: none;
    font-size: var(--text-micro);
    color: var(--color-text-muted);
  }
  .drawer {
    padding: 0 8px 8px;
  }
  .verbs {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .verbs button {
    padding: 0 12px;
    border-radius: 6px;
    background: var(--color-bg-panel);
    box-shadow: inset 0 0 0 1px var(--color-border-subtle);
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
  }
  .verbs button.danger {
    color: var(--color-status-reopen);
  }
  .verbs.confirm .ask {
    display: flex;
    align-items: center;
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
  }
  .field {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .field input {
    flex: 1 1 160px;
    min-width: 0;
    min-height: var(--spacing-control);
    padding: 0 10px;
    border-radius: 6px;
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-base);
    /* --text-body is 16px by token: below that iOS zooms a focused field. */
    font-size: var(--text-body);
  }
  .field button {
    padding: 0 12px;
    border-radius: 6px;
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
  }
  .field button.go {
    color: var(--color-accent-text);
    font-weight: 600;
  }
  .field button:disabled {
    opacity: 0.45;
  }
  .notice {
    margin: 6px 0 0;
    font-size: var(--text-micro);
    color: var(--color-text-secondary);
  }
  h1 {
    margin: 0;
    font-size: var(--text-heading);
    line-height: var(--text-heading--line-height);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .body {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    min-width: 0;
    overflow: hidden;
  }
  .host-wrap {
    flex: 1 1 auto;
    min-height: 0;
    min-width: 0;
    position: relative;
    overflow: hidden;
  }
  .host {
    height: 100%;
    min-height: 0;
    min-width: 0;
    overflow: hidden;
    background: var(--color-bg-base);
  }
  .connecting {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 16px;
    background: var(--color-bg-base);
    pointer-events: none;
  }
  .paper {
    height: 12px;
    width: 56%;
    border-radius: 4px;
    background: var(--color-bg-elevated);
  }
  .paper.short {
    width: 32%;
  }
  .host :global(.xterm),
  .host :global(.xterm-viewport),
  .host :global(.xterm-screen) {
    height: 100%;
    width: 100%;
    background: var(--color-bg-base);
  }
  .host :global(.xterm-viewport) {
    overflow-x: hidden;
  }
  /* xterm's own scrollbar is hidden (orca's rule, both selectors): the
     transient indicator below replaces it, and a native scrollbar under a
     touch gesture is the double-scroll this task removes. */
  .host :global(.xterm-scrollable-element > .xterm-scrollbar),
  .host :global(.xterm-scrollbar) {
    display: none !important;
    width: 0 !important;
    opacity: 0 !important;
    pointer-events: none !important;
  }
  .scroll-indicator {
    position: absolute;
    top: 4px;
    right: 3px;
    bottom: 4px;
    width: 3px;
    pointer-events: none;
    opacity: 0;
    transition: opacity 120ms linear;
    z-index: 7;
  }
  .scroll-indicator.visible {
    opacity: 0.72;
  }
  .scroll-thumb {
    position: absolute;
    top: 0;
    right: 0;
    width: 3px;
    min-height: 24px;
    border-radius: 999px;
    background: var(--color-text-secondary);
    will-change: transform, height;
  }
  .scroll-badge {
    position: absolute;
    top: 50%;
    right: 2px;
    transform: translateY(-50%);
    padding: 2px 5px;
    border-radius: 999px;
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-micro);
    white-space: nowrap;
  }
  .status {
    flex: none;
    width: 100%;
    text-align: left;
    border-top: 1px solid var(--color-border-subtle);
    border-radius: 0;
    background: var(--color-bg-panel);
    padding: 12px 16px;
    font-size: var(--text-body);
    color: var(--color-text-secondary);
  }
  .hint {
    color: var(--color-text-muted);
  }
  .dock {
    position: relative;
  }
  .ime {
    position: absolute;
    width: 1px;
    height: 16px;
    font-size: var(--text-body);
    opacity: 0;
    border: 0;
    padding: 0;
    overflow: hidden;
  }
</style>
