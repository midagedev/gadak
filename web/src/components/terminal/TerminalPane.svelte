<script lang="ts">
  /*
   * Terminal pane (GDK-864, GDK-1194). Dock: the bottom of the content row,
   * spanning list/board and a docked detail panel both. Overlay: below 900px,
   * covers the content track (sidebar stays clickable).
   *
   * Closing the pane closes the WebSocket and keeps the session id; a reopen
   * reattaches and the ring replay is the first binary frame. Page unload
   * does nothing — sendBeacon is a POST, DELETE is the close verb, and the
   * grace reaps an abandoned session. Abandoned means idle: a session with
   * something still running under its shell keeps re-arming the grace
   * instead (GDK-994), so closing the pane on a running agent is safe.
   */
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import LoadingState from '../ui/LoadingState.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { createRenderer, type BehaviorTerminalRenderer } from '../../lib/terminal/renderer'
  import { settleResize } from '../../lib/terminal/resize'
  import {
    createSession,
    coerceDroppedReason,
    classifyCreateFail,
    droppedAllowsRestart,
    firstAttachRetryDelayMs,
    openSessionSocket,
    unavailableAllowsRestart,
    UNAVAILABLE_KEYS,
    TERMINAL_GRACE_MS,
    TERMINAL_RECONNECT_BACKOFF_MS,
    TERMINAL_WS_OPEN_MS,
    TERMINAL_SCROLLBACK_FALLBACK,
    TERMINAL_CURSOR_BLINK_FALLBACK,
    type DroppedReason,
    type SocketHandle,
    type UnavailableCause,
  } from '../../lib/terminal/session'
  import {
    TERMINAL_MIN_HEIGHT_PX,
    TERMINAL_MIN_WIDTH_PX,
    terminalChrome,
  } from '../../lib/terminal/pane.svelte'
  import { terminalSessions } from '../../lib/terminal/sessions.svelte'
  import { sessionLabel } from '../../lib/terminal/strip'
  import TerminalStrip from './TerminalStrip.svelte'
  import { config } from '../../lib/config'
  import { issues } from '../../stores/issues.svelte'
  import { knownProjectKeys } from '../../lib/terminal/issue-links'
  import { selection } from '../../stores/selection.svelte'

  let { overlay = false }: { overlay?: boolean } = $props()

  type Status =
    | { kind: 'none' }
    | { kind: 'reconnecting' }
    | { kind: 'exited'; code: number }
    | { kind: 'dropped'; reason: DroppedReason }
    | { kind: 'unavailable'; cause: UnavailableCause; detail?: string }

  let hostEl = $state<HTMLElement | null>(null)
  let status = $state<Status>({ kind: 'none' })
  let attached = $state(false)
  let dragging = $state(false)
  // The pane's one data sink, filled by onMount. Keystrokes and the status
  // line's click arrive here as the same bytes, so there is one Enter path
  // (GDK-991), not an Enter branch plus a click branch to keep in step.
  let sendTerminalData: ((bytes: Uint8Array) => void) | null = null

  const heightPx = $derived(terminalChrome.heightPx)
  const connectingGrace = createSkeletonGrace(() => !attached && status.kind === 'none')

  const DROPPED_KEYS = {
    slow_client: 'terminal.dropped.slow_client',
    token_revoked: 'terminal.dropped.token_revoked',
    idle_timeout: 'terminal.dropped.idle_timeout',
    server_shutdown: 'terminal.dropped.server_shutdown',
    closed: 'terminal.dropped.closed',
  } as const

  function droppedKey(reason: DroppedReason): (typeof DROPPED_KEYS)[DroppedReason] {
    return DROPPED_KEYS[reason]
  }

  function unavailableKey(cause: UnavailableCause): (typeof UNAVAILABLE_KEYS)[UnavailableCause] {
    return UNAVAILABLE_KEYS[cause]
  }

  /*
   * GDK-991: where a restart can actually succeed, the status line is the
   * click affordance the phone's status bar already is. Same verdicts the
   * Enter path applies — every status these gates reject stays plain text
   * (and announced, role="status").
   */
  const statusRestartable = $derived(
    status.kind === 'exited' ||
      (status.kind === 'unavailable' && unavailableAllowsRestart(status.cause)) ||
      (status.kind === 'dropped' && droppedAllowsRestart(status.reason)),
  )

  /*
   * GDK-1153: the rail names the session the pane is holding, so the answer
   * to "what is this terminal for" is on screen without opening anything.
   * The roster row is preferred because it carries the issue binding a
   * claim wrote (GDK-1158); before the first poll lands, the id stands in.
   */
  const currentName = $derived.by(() => {
    const id = terminalSessions.selectedId
    if (!id) return ''
    return sessionLabel(terminalSessions.selected ?? { id })
  })

  /*
   * The pane's two hooks into the driver below. They are set inside
   * onMount, so the effect that reads them tolerates a null: its first run
   * is the mount-time value, which boot() is already handling.
   */
  let switchTo: ((id: string | null) => void) | null = null
  let newSession: (() => void) | null = null

  // The one place a selection becomes an attachment. Whatever moves the
  // selected id — a strip row, a create, an exit, a reopen inside the grace
  // — arrives here, so "which session is the pane on" has a single answer
  // and a single code path to be wrong in.
  $effect(() => {
    const want = terminalSessions.selectedId
    switchTo?.(want)
  })

  /*
   * GDK-1160: which project keys in the output are real. The judgment itself
   * lives in issue-links.ts (pure, tested); this only memoizes it, because a
   * link provider is asked once per line under the pointer and the pool can
   * hold five figures of issues.
   */
  const PROJECT_CACHE_MS = 5_000
  let projectCache: { at: number; keys: Set<string> } | null = null

  function paneProjectKeys(): Set<string> {
    const now = Date.now()
    if (projectCache && now - projectCache.at < PROJECT_CACHE_MS) return projectCache.keys
    const keys = knownProjectKeys(config().projects, issues.pool.values())
    projectCache = { at: now, keys }
    return keys
  }

  onMount(() => {
    let cancelled = false
    let renderer: BehaviorTerminalRenderer | null = null
    let socket: SocketHandle | null = null
    let ro: ResizeObserver | null = null
    let stopSettle: (() => void) | null = null
    let fitTimer: ReturnType<typeof setTimeout> | undefined
    let openTimer: ReturnType<typeof setTimeout> | undefined
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined
    let reconnectAttempt = 0
    let reconnectSince = 0
    let lastCols = 0
    let lastRows = 0
    let phase: 'live' | 'ended' | 'unavailable' | 'reconnecting' = 'live'
    // Which session this pane's socket is on. The selected id is the wish;
    // this is what actually happened, and the two being separate is what
    // makes the switch idempotent.
    let attachedId: string | null = null
    let stopIssueLinks: (() => void) | null = null

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

    /*
     * GDK-1153: a socket the pane has moved off is not allowed to speak.
     *
     * Every attach stamps a generation; every callback checks it first, so
     * a socket closed on the way to another session cannot run the pane's
     * reconnect for the session it just left. Measured before this guard:
     * switching sessions closed a *live* socket, its onClose read
     * phase === 'live' and reconnected the old id, and the two attachments
     * then took turns replaying their rings into one buffer — the pane
     * showed both shells' scrollback, spliced, and neither session's
     * keystrokes landed where they were aimed.
     *
     * The class this closes is wider than the switch: any late callback
     * from a socket the pane no longer holds — a slow close, a drop that
     * arrives after a reattach — used to be indistinguishable from the
     * current one's.
     */
    let socketGen = 0

    const detachSocket = () => {
      socketGen += 1
      socket?.close()
      socket = null
      attached = false
    }

    const fittedSize = (): { cols: number; rows: number } => {
      renderer?.fit()
      const cols = renderer?.cols || 80
      const rows = renderer?.rows || 24
      return { cols, rows }
    }

    /**
     * A pane with no box has no size to report: xterm's fit answers from a
     * zero rect with its floor — 10x5 — and shipping that to the PTY tells
     * every child in it to lay out for a ten-column terminal. Measured on
     * the phone pane, which is display:none behind another tab (GDK-1154).
     * This pane is destroyed on close rather than hidden, so the shape is
     * rarer here; the guard is the same because the mistake is the same.
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
      fitTimer = setTimeout(sendResize, 100)
    }

    async function startNew(): Promise<void> {
      const { cols, rows } = fittedSize()
      lastCols = cols
      lastRows = rows
      // What the pane was on when this create was asked for. A person can
      // move the selection while the POST is in flight — a strip row is live
      // the moment the *server* has the session, which is before its response
      // gets back here — and a create landing into that window used to take
      // the pane off the row they clicked (GDK-1185). Their choice is newer
      // than this request, so it wins: the session is still created and the
      // strip still shows it, this pane just does not go there.
      const from = terminalSessions.selectedId
      const doc = await createSession(cols, rows)
      if (cancelled || terminalSessions.selectedId !== from) return
      // The create response is the one road terminal behavior reaches the
      // pane on (GDK-896 R2): apply before attaching, so the ring replay
      // lands in a buffer already sized to the configured scrollback.
      renderer?.applyBehavior({ scrollback: doc.scrollback, cursorBlink: doc.cursorBlink })
      terminalSessions.select(doc.id)
      attachSocket(doc.id, { afterCreate: true })
    }

    function attachSocket(id: string, opts: { afterCreate: boolean; recreateOnFail?: boolean }): void {
      detachSocket()
      let opened = false
      phase = 'live'
      attachedId = id
      // Claimed after detachSocket bumped it: this closure owns the pane
      // only while the counter still reads its own number.
      const gen = socketGen
      const stale = () => gen !== socketGen
      const handle = openSessionSocket(id, {
        onOpen() {
          if (stale()) return
          opened = true
          if (openTimer !== undefined) {
            clearTimeout(openTimer)
            openTimer = undefined
          }
          attached = true
          reconnectAttempt = 0
          reconnectSince = 0
          if (status.kind === 'reconnecting') status = { kind: 'none' }
          // The size may have changed while the socket was down, and the
          // cache is "what the server was told" — a new socket has been
          // told nothing. Empty it, then let the one owner send, guard and
          // all. This used to call handle.resize() with its own unguarded
          // fittedSize(), the side door a hidden pane's 10x5 reached the
          // PTY through on a late reattach (GDK-1154).
          lastCols = 0
          lastRows = 0
          sendResize()
          // …and again across the window in which layout settles: one read
          // at open used to be the last word on the size for the life of
          // the session. sendResize no-ops once the sizes agree.
          stopSettle?.()
          stopSettle = settleResize(sendResize)
          renderer?.focus()
        },
        onBytes(data) {
          if (stale()) return
          renderer?.write(data)
        },
        onExit(code) {
          if (stale()) return
          phase = 'ended'
          status = { kind: 'exited', code }
          terminalSessions.select(null)
        },
        onDropped(reason) {
          if (stale()) return
          phase = 'ended'
          status = { kind: 'dropped', reason: coerceDroppedReason(reason) }
          if (reason === 'token_revoked' || reason === 'server_shutdown' || reason === 'idle_timeout') {
            terminalSessions.select(null)
          }
        },
        onClose(neverOpened) {
          if (stale()) return
          attached = false
          if (cancelled || phase === 'ended' || phase === 'unavailable') return
          if (neverOpened && opts.recreateOnFail) {
            terminalSessions.select(null)
            void startNew().catch(onCreateFail)
            return
          }
          if (neverOpened && opts.afterCreate) {
            const delay = firstAttachRetryDelayMs(reconnectAttempt)
            if (delay !== null) {
              phase = 'reconnecting'
              status = { kind: 'reconnecting' }
              reconnectAttempt += 1
              reconnectTimer = setTimeout(() => {
                if (cancelled) return
                attachSocket(id, { afterCreate: true })
              }, delay)
              return
            }
            phase = 'unavailable'
            status = { kind: 'unavailable', cause: 'network' }
            terminalSessions.select(null)
            return
          }
          scheduleReconnect(id)
        },
      })
      socket = handle
      if (openTimer !== undefined) clearTimeout(openTimer)
      openTimer = setTimeout(() => {
        if (opened || cancelled) return
        handle.close()
        // onClose handles recreate / unavailable / reconnect.
      }, TERMINAL_WS_OPEN_MS)
    }

    function scheduleReconnect(id: string): void {
      if (reconnectSince === 0) reconnectSince = Date.now()
      if (Date.now() - reconnectSince >= TERMINAL_GRACE_MS) {
        phase = 'ended'
        status = { kind: 'dropped', reason: 'idle_timeout' }
        terminalSessions.select(null)
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

    /*
     * The one data sink (GDK-991): the renderer's keystrokes and the status
     * line's click — which synthesizes the same CR byte a pressed Enter
     * produces — run the same gates and the same restart. A click can only
     * arrive where statusRestartable already said yes, but the gates stay:
     * the sink does not trust its caller.
     */
    function handleTerminalData(bytes: Uint8Array): void {
      if (phase === 'ended' || phase === 'unavailable') {
        const enter = bytes.length === 1 && (bytes[0] === 13 || bytes[0] === 10)
        if (enter) {
          if (status.kind === 'unavailable' && !unavailableAllowsRestart(status.cause)) return
          if (status.kind === 'dropped' && !droppedAllowsRestart(status.reason)) return
          status = { kind: 'none' }
          phase = 'live'
          reconnectSince = 0
          reconnectAttempt = 0
          void startNew().catch(onCreateFail)
        }
        return
      }
      if (phase !== 'live') return
      socket?.send(bytes)
    }
    sendTerminalData = handleTerminalData

    async function boot(): Promise<void> {
      if (!hostEl) return
      renderer = await createRenderer()
      if (cancelled) {
        renderer.dispose()
        return
      }
      renderer.open(hostEl)
      // Behavior starts at the server's default values (GDK-896 R2); a
      // create response overrides them below. The kept-session path never
      // creates, so without this a reopen-in-grace pane would run on
      // xterm's own 1000-line default until its next fresh session.
      renderer.applyBehavior({
        scrollback: TERMINAL_SCROLLBACK_FALLBACK,
        cursorBlink: TERMINAL_CURSOR_BLINK_FALLBACK,
      })
      renderer.fit()
      renderer.onData(handleTerminalData)
      // Route xterm's own resize through the single sender rather than
      // repeating it here. The duplicate advanced lastCols/lastRows and
      // *then* `socket?.resize(...)`, which is a no-op before the socket is
      // live — so the cache recorded a size the server had never been told,
      // every later check found "no change", and the PTY kept its
      // pre-layout size for the life of the session. Measured on the phone
      // pane, which carried the same six lines (GDK-1154): cols 10 rows 5
      // under a pane rendering 48x34, which is the size SIGWINCH hands
      // every TUI running in it.
      renderer.onResize(() => sendResize())
      // GDK-1160: issue keys already flow through this pane in git logs,
      // build output and agent reports. Opening one goes through the app's
      // existing verb — there is no second route to an issue here.
      stopIssueLinks = renderer.registerIssueLinks({
        projects: paneProjectKeys,
        open: (key) => selection.select(key),
      })
      ro = new ResizeObserver(scheduleFit)
      ro.observe(hostEl)

      const kept = terminalSessions.selectedId
      try {
        if (kept) {
          attachSocket(kept, { afterCreate: false, recreateOnFail: true })
        } else {
          await startNew()
        }
      } catch (err) {
        onCreateFail(err)
      }
    }

    function onCreateFail(err: unknown): void {
      if (cancelled) return
      const classified = classifyCreateFail(err)
      phase = 'unavailable'
      status = classified.detail
        ? { kind: 'unavailable', cause: classified.cause, detail: classified.detail }
        : { kind: 'unavailable', cause: classified.cause }
      terminalSessions.select(null)
    }

    /*
     * The switch (GDK-1153). A strip row does not open a second pane — it
     * moves this one. The socket is dropped, the buffer is emptied, and the
     * new session's ring replay is its own complete scrollback; without the
     * reset the two shells would be spliced into one history and the
     * "scrollback follows the row" contract would be a lie by concatenation.
     *
     * Idempotent on the id already attached, because the selected id is
     * also set by create and by a reopen inside the grace — the paths that
     * attach for themselves.
     */
    switchTo = (want: string | null) => {
      // No renderer yet means boot() is still in its dynamic import; it
      // reads the selected id after that, so a wish made in this window is
      // honoured there rather than attaching a socket with nowhere to draw.
      if (cancelled || !renderer || want === null || want === attachedId) return
      clearTimers()
      reconnectAttempt = 0
      reconnectSince = 0
      status = { kind: 'none' }
      renderer?.reset()
      attachSocket(want, { afterCreate: false })
    }

    /*
     * A second shell, from the rail. The server has never had a session
     * ceiling; this is the surface that finally admits it. Same path a
     * restart takes, so nothing here is a second way to create a session.
     */
    newSession = () => {
      // Same window as switchTo: boot() is about to create one of its own.
      if (cancelled || !renderer) return
      clearTimers()
      reconnectAttempt = 0
      reconnectSince = 0
      status = { kind: 'none' }
      phase = 'live'
      renderer?.reset()
      // Leave the old session *now*, not when the new one's POST comes back
      // (GDK-1185). The buffer is already blank, so a socket still on the
      // previous shell paints that shell's live output into a pane that is
      // supposed to be showing a brand new one — and the session the person
      // walked away from goes on counting this pane as a watcher.
      //
      // The selection has to go with it. Leaving it on the old session made
      // that session's row *un-clickable* for the whole width of the create:
      // `select()` is idempotent, so clicking the row it still points at
      // changed nothing and no attach happened. Null is the honest value —
      // the pane is between shells.
      detachSocket()
      attachedId = null
      terminalSessions.select(null)
      void startNew().catch(onCreateFail)
    }

    void boot()

    return () => {
      cancelled = true
      switchTo = null
      newSession = null
      clearTimers()
      stopIssueLinks?.()
      stopIssueLinks = null
      ro?.disconnect()
      detachSocket()
      renderer?.dispose()
      renderer = null
      sendTerminalData = null
      // Keep the session id. The grace reaps it if nobody reopens and
      // nothing is running under it.
    }
  })

  function onHandlePointerDown(e: PointerEvent): void {
    if (overlay) return
    e.preventDefault()
    dragging = true
    const startY = e.clientY
    const startH = heightPx
    // Up is taller: the handle is on the dock's top edge (GDK-1194).
    const move = (ev: PointerEvent) => {
      terminalChrome.persistHeight(startH + (startY - ev.clientY))
    }
    const up = () => {
      dragging = false
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }

  /*
   * GDK-991: a click on the status line is a pressed Enter key — the same
   * byte through the same data sink, so the restart gates and the restart
   * itself cannot drift apart. The phone's status bar does the same
   * (mobile/src/screens/Shell.svelte, onStatusActivate).
   */
  function onStatusActivate(): void {
    sendTerminalData?.(new Uint8Array([13]))
  }
</script>

<aside
  class="flex min-h-0 w-full min-w-0 flex-col overflow-hidden bg-bg-base {overlay
    ? 'fixed top-0 right-0 bottom-0 h-full border-l border-border-subtle'
    : 'relative border-t border-border-subtle'}"
  class:select-none={dragging}
  style={overlay
    ? `left: var(--layout-sidebar, 272px); z-index: 48; min-width: ${TERMINAL_MIN_WIDTH_PX}px`
    : `height: ${heightPx}px; min-height: ${TERMINAL_MIN_HEIGHT_PX}px`}
  role="region"
  aria-label={t('terminal.title')}
  data-testid="terminal-pane"
  data-attached={attached ? 'true' : 'false'}
  data-overlay={overlay ? 'true' : undefined}
>
  <!--
    A rail, not a title bar: the same strip the status line already is, at
    the other end. It exists because the pane swallows every keystroke on
    purpose, so the key that closes it cannot also be the only way out —
    someone who opened this with a shortcut they half-remember needs
    something to click. Micro-caps and hairline, the app's own idiom.
  -->
  <div
    class="flex flex-none items-center justify-between gap-2 border-b border-border-subtle bg-bg-panel py-1 pr-1 pl-3"
  >
    <span class="flex min-w-0 items-center gap-1.5">
      <Icon name="terminal" size={13} class="flex-none text-text-muted" />
      <span class="flex-none text-micro tracking-wide text-text-muted uppercase"
        >{t('terminal.title')}</span
      >
      <!--
        GDK-1153: whose terminal this is. The name is the issue a claim in
        this shell took, falling back to a short id — so a pane holding one
        session answers "what is this for" without a strip row under it, and
        the one-session case costs no chrome at all.
      -->
      {#if currentName}
        <span class="flex-none text-micro text-text-muted" aria-hidden="true">·</span>
        <span class="truncate text-micro text-text-secondary" data-testid="terminal-rail-name"
          >{currentName}</span
        >
      {/if}
    </span>
    <span class="flex flex-none items-center">
      <button
        type="button"
        class="flex h-6 w-6 flex-none items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
        aria-label={t('terminal.strip.new')}
        title={t('terminal.strip.new')}
        data-testid="terminal-new"
        onclick={() => newSession?.()}
      >
        <Icon name="plus" size={14} />
      </button>
      <button
        type="button"
        class="flex h-6 w-6 flex-none items-center justify-center rounded text-text-muted hover:bg-bg-hover hover:text-text-primary"
        aria-label={t('terminal.close')}
        title="{t('terminal.close')} ({t('terminal.shortcut')})"
        data-testid="terminal-close"
        onclick={() => terminalChrome.toggle()}
      >
        <Icon name="x" size={14} />
      </button>
    </span>
  </div>
  <TerminalStrip offerStart={statusRestartable} onstart={onStatusActivate} />
  <div
    class="relative min-h-0 min-w-0 flex-1 overflow-hidden"
    data-skeleton={connectingGrace.attr}
  >
    <div
      class="h-full min-h-0 min-w-0 overflow-hidden"
      class:invisible={connectingGrace.visible}
      bind:this={hostEl}
    ></div>
    {#if connectingGrace.visible}
      <div class="absolute inset-0" data-testid="terminal-connecting">
        <LoadingState label={t('common.loading')} />
      </div>
    {/if}
  </div>
  {#if status.kind !== 'none'}
    {#snippet statusLine()}
      {#if status.kind === 'reconnecting'}
        {t('terminal.reconnecting')}
      {:else if status.kind === 'exited'}
        {t('terminal.exited', { code: status.code })}
        <span class="text-text-muted"> · {t('terminal.restartHint')}</span>
      {:else if status.kind === 'dropped'}
        {t(droppedKey(status.reason))}
        {#if droppedAllowsRestart(status.reason)}
          <span class="text-text-muted"> · {t('terminal.restartHint')}</span>
        {:else}
          <span class="text-text-muted"> · {t('terminal.mintHint')}</span>
        {/if}
      {:else if status.kind === 'unavailable'}
        {#if status.cause === 'failed'}
          {t('terminal.unavailable.failed', { message: status.detail ?? '' })}
        {:else}
          {t(unavailableKey(status.cause))}
        {/if}
        {#if unavailableAllowsRestart(status.cause)}
          <span class="text-text-muted"> · {t('terminal.restartHint')}</span>
        {/if}
      {/if}
    {/snippet}
    <!--
      GDK-991: where a restart can succeed, the status line is the button the
      phone's status bar already is — same copy, same classes; the additions
      are the pointer cursor and text-left, which undoes the button's own
      centered default so the div rendering is preserved. The states a
      restart cannot help (reconnecting, token_revoked, unsupported) stay a
      div role="status" — the live announcement a button role cannot carry
      (mobile Shell.svelte splits the same way).
    -->
    {#if statusRestartable}
      <button
        type="button"
        class="flex-none cursor-pointer border-t border-border-subtle bg-bg-panel px-3 py-1.5 text-left text-body text-text-secondary"
        data-testid="terminal-status"
        onclick={onStatusActivate}
      >
        {@render statusLine()}
      </button>
    {:else}
      <div
        class="flex-none border-t border-border-subtle bg-bg-panel px-3 py-1.5 text-body text-text-secondary"
        data-testid="terminal-status"
        role="status"
      >
        {@render statusLine()}
      </div>
    {/if}
  {/if}
  {#if !overlay}
    <button
      type="button"
      class="absolute top-0 right-0 left-0 h-1 cursor-row-resize border-0 bg-transparent p-0 hover:bg-border-subtle"
      aria-label={t('terminal.resize')}
      data-testid="terminal-resize"
      onpointerdown={onHandlePointerDown}
    ></button>
  {/if}
</aside>
