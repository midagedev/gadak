<script lang="ts">
  /*
   * Terminal pane (GDK-864, GDK-1194, GDK-1352, GDK-1355). Dock: a band
   * across the bottom of the whole window — sidebar, list/board and a docked
   * detail panel all sit above it. Inside, two columns: the session roster on
   * the left, as wide as the app sidebar above it (the window's left column
   * is navigation, its right column is content), and the shell taking every
   * row of the band. Overlay: below 900px, a right-hand sheet over the
   * content track (sidebar stays clickable) with a narrower roster.
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
  import LoadingState from '../ui/LoadingState.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { createRenderer, type BehaviorTerminalRenderer } from '../../lib/terminal/renderer'
  import { createTerminalDriver } from '../../lib/terminal/driver'
  import {
    createSession,
    classifyCreateFail,
    droppedAllowsRestart,
    firstAttachRetryDelayMs,
    openSessionSocket,
    unavailableAllowsRestart,
    UNAVAILABLE_KEYS,
    TERMINAL_SCROLLBACK_FALLBACK,
    TERMINAL_CURSOR_BLINK_FALLBACK,
    type DroppedReason,
    type UnavailableCause,
  } from '../../lib/terminal/session'
  import {
    TERMINAL_MIN_HEIGHT_PX,
    TERMINAL_MIN_WIDTH_PX,
    terminalChrome,
    terminalHeightGrip,
  } from '../../lib/terminal/pane.svelte'
  import LayoutResizeHandle from '../shell/LayoutResizeHandle.svelte'
  import { terminalSessions } from '../../lib/terminal/sessions.svelte'
  import TerminalRoster from './TerminalRoster.svelte'
  import { config } from '../../lib/config'
  import { issues } from '../../stores/issues.svelte'
  import { knownProjectKeys } from '../../lib/terminal/issue-links'
  import { selection } from '../../stores/selection.svelte'

  let { overlay = false }: { overlay?: boolean } = $props()

  // An issue's surface asked for a shell while this pane is up (GDK-1388):
  // the same new-shell verb the roster's + row runs, and startNew takes the
  // request. On a pane that is still booting the boot create takes it.
  $effect(() => {
    if (terminalSessions.createRequest && newSession) newSession()
  })

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
  // One grip object for the pane's lifetime: lib/layout-resize.ts holds the
  // key-burst commit timer under its axis, and a fresh object per render
  // would be fine for that but pointless churn.
  const heightGrip = terminalHeightGrip()
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
    let ro: ResizeObserver | null = null
    let stopIssueLinks: (() => void) | null = null

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

    /*
     * The socket skeleton — generation guard, backoff ladder, grace, open
     * timeout, resize cache — lives in lib/terminal/driver (GDK-1767),
     * shared with the phone. This pane supplies the transport (ws or
     * wails, via ./session), the measurements, and the reactions that are
     * pane-owned: which selection id to drop and what a recreate starts.
     */
    const driver = createTerminalDriver({
      open: (id, handlers) => openSessionSocket(id, handlers),
      fittedSize,
      measurable: () => renderer !== null && paneLaidOut(),
      onAttached: () => renderer?.focus(),
      onBytes: (data) => renderer?.write(data),
      onExit: () => terminalSessions.select(null),
      onDropped: (reason) => {
        if (reason === 'token_revoked' || reason === 'server_shutdown' || reason === 'idle_timeout') {
          terminalSessions.select(null)
        }
      },
      onSessionGone: () => terminalSessions.select(null),
      onRecreate: () => {
        terminalSessions.select(null)
        void startNew().catch(onCreateFail)
      },
      onStatus: (s) => {
        status = s
      },
      onLive: (live) => {
        attached = live
      },
      firstAttachRetry: (attempt) => firstAttachRetryDelayMs(attempt),
    })

    async function startNew(): Promise<void> {
      const { cols, rows } = driver.measureForCreate()
      // What the pane was on when this create was asked for. A person can
      // move the selection while the POST is in flight — a strip row is live
      // the moment the *server* has the session, which is before its response
      // gets back here — and a create landing into that window used to take
      // the pane off the row they clicked (GDK-1185). Their choice is newer
      // than this request, so it wins: the session is still created and the
      // strip still shows it, this pane just does not go there.
      const from = terminalSessions.selectedId
      // A shell asked for from an issue's surface is bound at birth
      // (GDK-1388); any other create takes nothing.
      const req = terminalSessions.takeCreateRequest()
      const doc = await createSession(cols, rows, req ? { issueKey: req.issueKey } : {})
      if (cancelled || terminalSessions.selectedId !== from) return
      // The create response is the one road terminal behavior reaches the
      // pane on (GDK-896 R2): apply before attaching, so the ring replay
      // lands in a buffer already sized to the configured scrollback.
      renderer?.applyBehavior({ scrollback: doc.scrollback, cursorBlink: doc.cursorBlink })
      terminalSessions.select(doc.id)
      // The strip learns of sessions by polling (ROSTER_POLL_MS); without
      // this the shell is at its prompt for up to two seconds before its
      // own tab appears above it (measured 2026-09-02 — 1.2s after attach
      // the row was still empty). kill() already refreshes on its own verb;
      // create is the other one.
      terminalSessions.nudge()
      driver.attach(doc.id, { afterCreate: true })
    }

    /*
     * The one data sink (GDK-991): the renderer's keystrokes and the status
     * line's click — which synthesizes the same CR byte a pressed Enter
     * produces — run the same gates and the same restart. A click can only
     * arrive where statusRestartable already said yes, but the gates stay:
     * the sink does not trust its caller.
     */
    function handleTerminalData(bytes: Uint8Array): void {
      if (driver.phase === 'ended' || driver.phase === 'unavailable') {
        const enter = bytes.length === 1 && (bytes[0] === 13 || bytes[0] === 10)
        if (enter) {
          if (status.kind === 'unavailable' && !unavailableAllowsRestart(status.cause)) return
          if (status.kind === 'dropped' && !droppedAllowsRestart(status.reason)) return
          status = { kind: 'none' }
          driver.revive()
          void startNew().catch(onCreateFail)
        }
        return
      }
      driver.send(bytes)
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
      // Route xterm's own resize through the driver's single sender rather
      // than repeating it here. A duplicate used to advance the size cache
      // and *then* try to send on a socket that was not live — so the cache
      // recorded a size the server had never been told, every later check
      // found "no change", and the PTY kept its pre-layout size for the
      // life of the session (GDK-1154).
      renderer.onResize(() => driver.resizeNow())
      // GDK-1160: issue keys already flow through this pane in git logs,
      // build output and agent reports. Opening one goes through the app's
      // existing verb — there is no second route to an issue here.
      stopIssueLinks = renderer.registerIssueLinks({
        projects: paneProjectKeys,
        open: (key) => selection.select(key, 'terminal-link'),
      })
      ro = new ResizeObserver(() => driver.scheduleFit())
      ro.observe(hostEl)

      const kept = terminalSessions.selectedId
      try {
        if (kept) {
          driver.attach(kept, { afterCreate: false, recreateOnFail: true })
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
      // The phase column is the driver's, and the Enter gate reads it: a
      // pane whose create failed must not look live to its own keystrokes.
      driver.markUnavailable()
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
      if (cancelled || !renderer || want === null || want === driver.currentId) return
      driver.clearTimers()
      driver.resetBackoff()
      status = { kind: 'none' }
      renderer?.reset()
      driver.attach(want, { afterCreate: false })
    }

    /*
     * A second shell, from the rail. The server has never had a session
     * ceiling; this is the surface that finally admits it. Same path a
     * restart takes, so nothing here is a second way to create a session.
     */
    newSession = () => {
      // Same window as switchTo: boot() is about to create one of its own.
      if (cancelled || !renderer) return
      driver.clearTimers()
      driver.revive()
      status = { kind: 'none' }
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
      driver.detach()
      terminalSessions.select(null)
      void startNew().catch(onCreateFail)
    }

    void boot()

    return () => {
      cancelled = true
      switchTo = null
      newSession = null
      stopIssueLinks?.()
      stopIssueLinks = null
      ro?.disconnect()
      driver.dispose()
      renderer?.dispose()
      renderer = null
      sendTerminalData = null
      // Keep the session id. The grace reaps it if nobody reopens and
      // nothing is running under it.
    }
  })

  /*
   * GDK-991: a click on the status line is a pressed Enter key — the same
   * byte through the same data sink, so the restart gates and the restart
   * itself cannot drift apart. The phone's status bar does the same
   * (mobile/src/screens/Shell.svelte, onStatusActivate).
   */
  function onStatusActivate(): void {
    sendTerminalData?.(new Uint8Array([13]))
  }

  /*
   * GDK-1835: the roster can be rendered in the app sidebar, outside this
   * component, so what it needs cannot be a local closure any more. The pane
   * is still the only thing that can do either job — it owns the driver —
   * so it publishes them rather than the roster reaching in. Cleared on
   * destroy: a roster outliving the pane must render a dead verb disabled,
   * not call into a torn-down driver.
   */
  $effect(() => {
    terminalChrome.restartable = statusRestartable
  })
  $effect(() => {
    terminalChrome.newSession = () => newSession?.()
    terminalChrome.restart = onStatusActivate
    return () => {
      terminalChrome.newSession = null
      terminalChrome.restart = null
      terminalChrome.restartable = false
    }
  })
</script>

<aside
  class="flex min-h-0 w-full min-w-0 flex-row overflow-hidden bg-bg-base {overlay
    ? 'terminal-sheet fixed top-0 right-0 bottom-0 h-full border-l border-border-strong'
    : 'relative border-t border-border-strong'}"
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
  <!-- The roster (GDK-1355, GDK-1835). Its rows, its new-shell verb and the
       two chrome verbs live in TerminalRoster, because in the full-screen
       shape they are rendered in the app sidebar instead of here. Here it is
       the dock's left column; when this pane is the sheet, the sidebar is
       already on screen to its left and carries them. -->
  {#if !overlay}
    <TerminalRoster variant="dock" />
  {/if}
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <div
    class="relative min-h-0 min-w-0 flex-1 overflow-hidden"
    data-skeleton={connectingGrace.attr}
  >
    <div
      class="terminal-host h-full min-h-0 min-w-0 overflow-hidden"
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
  </div>
  {#if !overlay}
    <!--
      GDK-1815: the shell's one grip, driven by the dock's own store. It was
      a bare <button onpointerdown> here — the only drag affordance in the
      app you could not reach from a keyboard.
    -->
    <LayoutResizeHandle
      axis="terminal"
      grip={heightGrip}
      ondragging={(d) => (dragging = d)}
    />
  {/if}
</aside>
