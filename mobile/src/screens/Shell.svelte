<script module lang="ts">
  // Session id kept across tab switches and app backgrounds so a reattach
  // replays the ring (desktop pane does the same in session.ts).
  let keptSessionId: string | null = null
</script>

<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import Screen from '../ui/Screen.svelte'
  import KeyBar from '../ui/KeyBar.svelte'
  import { t } from '../lib/i18n'
  import { app, terminalSession } from '../lib/store.svelte'
  import { createShellSession } from '../lib/terminal/api'
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
  // wails transport, which the phone must not resolve.
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
  let ro: ResizeObserver | null = null
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
    if (fitTimer !== undefined) clearTimeout(fitTimer)
    if (openTimer !== undefined) clearTimeout(openTimer)
    if (reconnectTimer !== undefined) clearTimeout(reconnectTimer)
    fitTimer = undefined
    openTimer = undefined
    reconnectTimer = undefined
  }

  const detachSocket = () => {
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

  const sendResize = () => {
    if (!renderer || !socket || phase !== 'live') return
    const { cols, rows } = fittedSize()
    if (cols === lastCols && rows === lastRows) return
    lastCols = cols
    lastRows = rows
    socket.resize(cols, rows)
  }

  const scheduleFit = () => {
    if (fitTimer !== undefined) clearTimeout(fitTimer)
    fitTimer = setTimeout(sendResize, 100)
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
    keptSessionId = doc.id
    attachSocket(doc.id, { afterCreate: true })
  }

  function attachSocket(id: string, opts: { afterCreate: boolean; recreateOnFail?: boolean }): void {
    detachSocket()
    let opened = false
    phase = 'live'
    const session = terminalSession()
    const handle = openShellSocket(
      id,
      {
        onOpen() {
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
          const { cols, rows } = fittedSize()
          lastCols = cols
          lastRows = rows
          handle.resize(cols, rows)
        },
        onBytes(data) {
          renderer?.write(data)
        },
        onExit(code) {
          phase = 'ended'
          status = { kind: 'exited', code }
          keptSessionId = null
        },
        onDropped(reason) {
          phase = 'ended'
          status = { kind: 'dropped', reason: coerceDroppedReason(reason) }
          if (reason === 'token_revoked' || reason === 'server_shutdown' || reason === 'idle_timeout') {
            keptSessionId = null
          }
        },
        onClose(neverOpened) {
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
    renderer.fit()
    renderer.onResize((cols, rows) => {
      lastCols = cols
      lastRows = rows
      socket?.resize(cols, rows)
    })
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
    else if (g.kind === 'inject') sendBytes(g.bytes)
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

<style>
  .head {
    display: flex;
    align-items: baseline;
    padding: 12px 0 10px;
    min-width: 0;
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
