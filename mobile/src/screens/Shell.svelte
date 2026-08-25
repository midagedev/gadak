<script module lang="ts">
  // Session id kept across tab switches and app backgrounds so a reattach
  // replays the ring (desktop pane does the same in session.ts).
  let keptSessionId: string | null = null
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import Screen from '../ui/Screen.svelte'
  import KeyBar from '../ui/KeyBar.svelte'
  import { t } from '../lib/i18n'
  import { app, terminalSession } from '../lib/store.svelte'
  import { createShellSession } from '../lib/terminal/api'
  import { openShellSocket } from '../lib/terminal/transport'
  import {
    applyStickyPress,
    bytesForBarKey,
    bytesForText,
    type BarKey,
    type StickyMods,
  } from '../lib/terminal/keys'
  import { imeReduce, IME_INPUT_ATTRS, type ImeState } from '../lib/terminal/ime'
  import { createRenderer, type PhoneTerminalRenderer } from '../lib/terminal/renderer'
  import { coerceDroppedReason } from '../../../web/src/lib/terminal/protocol'
  import type { DroppedReason, SocketHandle } from '../../../web/src/lib/terminal/protocol'

  // Copied from web/src/lib/terminal/session.ts — that module imports the
  // wails transport, which the phone must not resolve.
  const TERMINAL_GRACE_MS = 60_000
  const TERMINAL_RECONNECT_BACKOFF_MS = [500, 1000, 2000, 4000] as const
  const TERMINAL_WS_OPEN_MS = 8_000

  type Status =
    | { kind: 'none' }
    | { kind: 'reconnecting' }
    | { kind: 'exited'; code: number }
    | { kind: 'dropped'; reason: DroppedReason }
    | { kind: 'unavailable' }

  const DROPPED_KEYS: Record<DroppedReason, 'terminal.dropped.slow_client' | 'terminal.dropped.token_revoked' | 'terminal.dropped.idle_timeout' | 'terminal.dropped.server_shutdown' | 'terminal.dropped.closed'> = {
    slow_client: 'terminal.dropped.slow_client',
    token_revoked: 'terminal.dropped.token_revoked',
    idle_timeout: 'terminal.dropped.idle_timeout',
    server_shutdown: 'terminal.dropped.server_shutdown',
    closed: 'terminal.dropped.closed',
  }

  let hostEl = $state<HTMLElement | null>(null)
  let imeEl = $state<HTMLTextAreaElement | null>(null)
  let status = $state<Status>({ kind: 'none' })
  let attached = $state(false)
  let mods = $state<StickyMods>({ ctrl: false, alt: false })

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
    if (phase === 'ended') {
      const enter = bytes.length === 1 && (bytes[0] === 13 || bytes[0] === 10)
      if (enter) {
        status = { kind: 'none' }
        phase = 'live'
        reconnectSince = 0
        reconnectAttempt = 0
        void startNew()
      }
      return
    }
    if (phase !== 'live') return
    if (bytes.length === 0) return
    socket?.send(bytes)
  }

  function sendText(text: string) {
    const bytes = bytesForText(text, mods)
    mods = applyStickyPress(mods, text)
    sendBytes(bytes)
  }

  function onBarKey(key: BarKey) {
    const bytes = bytesForBarKey(key, mods)
    mods = applyStickyPress(mods, key)
    sendBytes(bytes)
    imeEl?.focus()
  }

  function flushIme(
    ev:
      | { kind: 'compositionstart' }
      | { kind: 'compositionupdate'; data: string }
      | { kind: 'compositionend'; data: string }
      | { kind: 'input'; data: string; isComposing: boolean },
  ) {
    const out = imeReduce(ime, ev)
    ime = out.state
    if (ev.kind === 'compositionend') {
      lastComposeEmit = out.emit
      if (out.emit) sendText(out.emit)
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
      sendText(out.emit)
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
      mods = applyStickyPress(mods, 'backspace')
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
          if (status.kind === 'reconnecting') status = { kind: 'none' }
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
            phase = 'unavailable'
            status = { kind: 'unavailable' }
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

  function onCreateFail(): void {
    if (cancelled) return
    phase = 'unavailable'
    status = { kind: 'unavailable' }
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
    if (status.kind === 'exited' || status.kind === 'dropped') {
      sendBytes(new Uint8Array([13]))
    }
  }

  function focusIme(): void {
    imeEl?.focus()
  }

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
    role="region"
    aria-label={t('terminal.title')}
  >
    {#if status.kind === 'unavailable'}
      <div class="ended" data-testid="terminal-status">
        {t('terminal.unavailable')}
      </div>
    {:else}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="host" bind:this={hostEl} onpointerdown={focusIme}></div>
      {#if status.kind !== 'none'}
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <div
          class="status"
          data-testid="terminal-status"
          role="status"
          onclick={onStatusActivate}
          onkeydown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') onStatusActivate()
          }}
        >
          {#if status.kind === 'reconnecting'}
            {t('terminal.reconnecting')}
          {:else if status.kind === 'exited'}
            {t('terminal.exited', { code: status.code })}
            <span class="hint"> · {t('terminal.restartHint')}</span>
          {:else if status.kind === 'dropped'}
            {t(DROPPED_KEYS[status.reason])}
            <span class="hint"> · {t('terminal.restartHint')}</span>
          {/if}
        </div>
      {/if}
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
  .host {
    flex: 1 1 auto;
    min-height: 0;
    min-width: 0;
    overflow: hidden;
    background: var(--color-bg-base);
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
  .ended,
  .status {
    flex: none;
    width: 100%;
    text-align: left;
    border-top: 1px solid var(--color-border-subtle);
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
