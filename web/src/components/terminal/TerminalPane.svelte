<script lang="ts">
  /*
   * Terminal pane (GDK-864). Split: docks left of the main column. Overlay:
   * below 900px, covers the content track (sidebar stays clickable).
   *
   * Closing the pane closes the WebSocket and keeps the session id; a reopen
   * inside the 60 s grace reattaches and the ring replay is the first binary
   * frame. Page unload does nothing — sendBeacon is a POST, DELETE is the
   * close verb, and the grace reaps an abandoned session.
   */
  import { onMount } from 'svelte'
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'
  import LoadingState from '../ui/LoadingState.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { createRenderer, type TerminalRenderer } from '../../lib/terminal/renderer'
  import {
    createSession,
    coerceDroppedReason,
    classifyCreateFail,
    droppedAllowsRestart,
    firstAttachRetryDelayMs,
    openSessionSocket,
    peekSessionId,
    rememberSessionId,
    unavailableAllowsRestart,
    TERMINAL_GRACE_MS,
    TERMINAL_RECONNECT_BACKOFF_MS,
    TERMINAL_WS_OPEN_MS,
    type DroppedReason,
    type SocketHandle,
    type UnavailableCause,
  } from '../../lib/terminal/session'
  import {
    TERMINAL_MIN_WIDTH_PX,
    TERMINAL_SPLIT_MAX_PCT,
    terminalChrome,
  } from '../../lib/terminal/pane.svelte'

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

  const widthPx = $derived(terminalChrome.widthPx)
  const connectingGrace = createSkeletonGrace(() => !attached && status.kind === 'none')

  const DROPPED_KEYS = {
    slow_client: 'terminal.dropped.slow_client',
    token_revoked: 'terminal.dropped.token_revoked',
    idle_timeout: 'terminal.dropped.idle_timeout',
    server_shutdown: 'terminal.dropped.server_shutdown',
    closed: 'terminal.dropped.closed',
  } as const

  const UNAVAILABLE_KEYS = {
    unsupported: 'terminal.unavailable.unsupported',
    forbidden: 'terminal.unavailable.forbidden',
    failed: 'terminal.unavailable.failed',
    network: 'terminal.unavailable.network',
  } as const

  function droppedKey(reason: DroppedReason): (typeof DROPPED_KEYS)[DroppedReason] {
    return DROPPED_KEYS[reason]
  }

  function unavailableKey(cause: UnavailableCause): (typeof UNAVAILABLE_KEYS)[UnavailableCause] {
    return UNAVAILABLE_KEYS[cause]
  }

  onMount(() => {
    let cancelled = false
    let renderer: TerminalRenderer | null = null
    let socket: SocketHandle | null = null
    let ro: ResizeObserver | null = null
    let fitTimer: ReturnType<typeof setTimeout> | undefined
    let openTimer: ReturnType<typeof setTimeout> | undefined
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined
    let reconnectAttempt = 0
    let reconnectSince = 0
    let lastCols = 0
    let lastRows = 0
    let phase: 'live' | 'ended' | 'unavailable' | 'reconnecting' = 'live'

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

    async function startNew(): Promise<void> {
      const { cols, rows } = fittedSize()
      lastCols = cols
      lastRows = rows
      const doc = await createSession(cols, rows)
      rememberSessionId(doc.id)
      attachSocket(doc.id, { afterCreate: true })
    }

    function attachSocket(id: string, opts: { afterCreate: boolean; recreateOnFail?: boolean }): void {
      detachSocket()
      let opened = false
      phase = 'live'
      const handle = openSessionSocket(id, {
        onOpen() {
          opened = true
          if (openTimer !== undefined) {
            clearTimeout(openTimer)
            openTimer = undefined
          }
          attached = true
          reconnectAttempt = 0
          reconnectSince = 0
          if (status.kind === 'reconnecting') status = { kind: 'none' }
          // Fitted size may have changed while the socket was down.
          const { cols, rows } = fittedSize()
          lastCols = cols
          lastRows = rows
          handle.resize(cols, rows)
          renderer?.focus()
        },
        onBytes(data) {
          renderer?.write(data)
        },
        onExit(code) {
          phase = 'ended'
          status = { kind: 'exited', code }
          rememberSessionId(null)
        },
        onDropped(reason) {
          phase = 'ended'
          status = { kind: 'dropped', reason: coerceDroppedReason(reason) }
          if (reason === 'token_revoked' || reason === 'server_shutdown' || reason === 'idle_timeout') {
            rememberSessionId(null)
          }
        },
        onClose(neverOpened) {
          attached = false
          if (cancelled || phase === 'ended' || phase === 'unavailable') return
          if (neverOpened && opts.recreateOnFail) {
            rememberSessionId(null)
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
            rememberSessionId(null)
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
        rememberSessionId(null)
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

    async function boot(): Promise<void> {
      if (!hostEl) return
      renderer = await createRenderer()
      if (cancelled) {
        renderer.dispose()
        return
      }
      renderer.open(hostEl)
      renderer.fit()
      renderer.onData((bytes) => {
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
      })
      renderer.onResize((cols, rows) => {
        lastCols = cols
        lastRows = rows
        socket?.resize(cols, rows)
      })
      ro = new ResizeObserver(scheduleFit)
      ro.observe(hostEl)

      const kept = peekSessionId()
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
      rememberSessionId(null)
    }

    void boot()

    return () => {
      cancelled = true
      clearTimers()
      ro?.disconnect()
      detachSocket()
      renderer?.dispose()
      renderer = null
      // Keep the session id. The grace reaps it if nobody reopens.
    }
  })

  function onHandlePointerDown(e: PointerEvent): void {
    if (overlay) return
    e.preventDefault()
    dragging = true
    const startX = e.clientX
    const startW = widthPx
    const move = (ev: PointerEvent) => {
      terminalChrome.persistWidth(startW + (ev.clientX - startX))
    }
    const up = () => {
      dragging = false
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
</script>

<aside
  class="flex h-full min-h-0 flex-none flex-col overflow-hidden bg-bg-base {overlay
    ? 'fixed top-0 right-0 bottom-0 border-l border-border-subtle'
    : 'relative border-r border-border-subtle'}"
  class:select-none={dragging}
  style={overlay
    ? `left: var(--layout-sidebar, 272px); z-index: 48; min-width: ${TERMINAL_MIN_WIDTH_PX}px`
    : `width: ${widthPx}px; min-width: ${TERMINAL_MIN_WIDTH_PX}px; max-width: min(${TERMINAL_SPLIT_MAX_PCT}%, calc(100% - var(--layout-list-min, 390px)))`}
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
      <span class="truncate text-micro tracking-wide text-text-muted uppercase"
        >{t('terminal.title')}</span
      >
      <span
        class="flex-none rounded-full border border-border-subtle px-1.5 text-micro font-medium tracking-wide text-text-muted uppercase"
        title={t('terminal.betaHint')}
        data-testid="terminal-beta">{t('terminal.beta')}</span
      >
    </span>
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
  </div>
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
    <div
      class="flex-none border-t border-border-subtle bg-bg-panel px-3 py-1.5 text-body text-text-secondary"
      data-testid="terminal-status"
      role="status"
    >
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
    </div>
  {/if}
  {#if !overlay}
    <button
      type="button"
      class="absolute top-0 right-0 h-full w-1 cursor-col-resize border-0 bg-transparent p-0 hover:bg-border-subtle"
      aria-label={t('terminal.title')}
      data-testid="terminal-resize"
      onpointerdown={onHandlePointerDown}
    ></button>
  {/if}
</aside>
