<script lang="ts">
  /*
   * DashboardView (GDK-782/793) — the host half of an agent dashboard.
   *
   * The authored HTML runs in a sandboxed iframe (`allow-scripts`, never
   * `allow-same-origin`: with both, the frame would share this origin and the
   * sandbox would be decoration). The parent owns every fetch: it reads the
   * dashboard's datasource map from the row, executes each through the
   * server's data routes, and pushes results in with postMessage. The frame's
   * only way back is the one-verb whitelist in lib/dashboard-protocol
   * (`refresh`); there is no open, no navigate, no URL verb to grant.
   *
   * Live updates (GDK-793):
   *  - authoring — the store bumps `renderGen` when the saved document
   *    changed; keying the iframe on it replaces the frame wholesale (no
   *    state-preservation duty) and re-runs the first push on load.
   *  - data — the 15s delta poll advances issues.lastSync; the effect below
   *    re-runs the datasources and re-pushes (≤2s contract).
   */
  import { onMount } from 'svelte'
  import { dashboardsBase, getDashboardData } from '../../lib/api'
  import {
    createRefreshThrottle,
    dataMessageFromError,
    parseFrameMessage,
    type DataMessage,
  } from '../../lib/dashboard-protocol'
  import { dashboards } from '../../stores/dashboards.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { t } from '../../lib/i18n'
  import Icon from '../ui/Icon.svelte'

  let frame = $state<HTMLIFrameElement | null>(null)
  /** Set once the frame's load event fired — pushing before it silently drops. */
  let frameReady = $state(false)

  const id = $derived(dashboards.openId)
  const row = $derived(dashboards.row)
  /** Full document swap on authoring change (GDK-793): the key re-creates the frame. */
  const renderGen = $derived(dashboards.renderGen)
  const src = $derived(id ? `${dashboardsBase()}${encodeURIComponent(id)}/render/` : '')
  /** What identifies "the frame has everything it needs for a fresh push":
   *  which dashboard + which generation of it (a save swaps the document),
   *  the mirror's sync stamp, and the datasource set (the row can land after
   *  the frame does — open() loads them in parallel). */
  const frameKey = $derived(`${id ?? ''}:${renderGen}`)
  const dsKey = $derived(
    Object.keys(dashboards.row?.config?.datasources ?? {})
      .sort()
      .join(','),
  )
  let pushedStamp = ''

  // `refresh` flood control: a hostile or looping dashboard cannot turn the
  // host into a query pump. 2s floor, trailing coalesce so a burst still ends
  // in one run.
  const throttle = createRefreshThrottle(2000, window)

  /** Push one datasource result (or its failure) into the frame. */
  async function pushOne(dashId: string, name: string): Promise<void> {
    let msg: DataMessage
    try {
      const res = await getDashboardData(dashId, name)
      msg = { type: 'data', name, ...res }
    } catch (e) {
      msg = dataMessageFromError(name, e instanceof Error ? e.message : String(e))
    }
    // targetOrigin '*' because the sandboxed frame is opaque-origin — it
    // cannot be named, and the message it receives carries no secrets (the
    // data routes already answered this caller's fetch).
    frame?.contentWindow?.postMessage(msg, '*')
  }

  /** Execute every datasource (stable name order) and push each as it lands. */
  function pushAll(): void {
    const dashId = dashboards.openId
    const config = dashboards.row?.config
    if (!dashId || !config || !frameReady) return
    for (const name of Object.keys(config.datasources).sort()) {
      void pushOne(dashId, name)
    }
  }

  function onFrameLoad(): void {
    frameReady = true
  }

  function onMessage(ev: MessageEvent): void {
    // Only our frame, never some other window's message.
    if (!frame || ev.source !== frame.contentWindow) return
    const msg = parseFrameMessage(ev.data)
    if (!msg) {
      // Unknown types are inert by design: logged for the author debugging a
      // dashboard, granted nothing.
      console.log('[dashboards] unhandled frame message', ev.data)
      return
    }
    if (msg.type === 'refresh') throttle.run(pushAll)
  }

  onMount(() => {
    window.addEventListener('message', onMessage)
    return () => {
      window.removeEventListener('message', onMessage)
      throttle.flush()
    }
  })

  // A replaced frame (key change) starts with no listeners — drop ready so
  // nothing pushes into the void between the swap and the new load event.
  $effect(() => {
    void frameKey
    frameReady = false
  })

  // The one push trigger: run when the frame is loaded AND anything the push
  // depends on moved — first load (stamp unset), a save (renderGen), a
  // dashboard switch (id), the row arriving after the frame (dsKey), or the
  // 15s delta poll advancing the mirror (lastSync, GDK-793 ≤2s contract).
  // `refresh` from the frame bypasses this via the throttle; that is the
  // frame asking, this is the host deciding.
  $effect(() => {
    const sync = issues.lastSync
    if (!sync || !frameReady || !dsKey) return
    const stamp = `${frameKey}|${sync}|${dsKey}`
    if (stamp === pushedStamp) return
    pushedStamp = stamp
    pushAll()
  })
</script>

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="dashboard-view">
  <header
    class="flex flex-none flex-wrap items-center gap-2 border-b border-border-subtle px-4 py-2"
  >
    <Icon name="layout-dashboard" size={15} class="flex-none text-text-muted" />
    <h2 class="min-w-0 flex-1 truncate text-body font-semibold text-text-primary" title={row?.name}>
      {row?.name ?? '…'}
    </h2>
    <button
      type="button"
      class="flex h-control-sm w-control-sm flex-none items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary"
      onclick={() => dashboards.close()}
      title={t('feed.backToList')}
      aria-label={t('feed.backToList')}
      data-testid="dashboard-close"
    >
      <Icon name="arrow-left" size={15} />
    </button>
  </header>

  {#if dashboards.error === 'not_found'}
    <div class="flex flex-1 items-center justify-center px-6" data-testid="dashboard-not-found">
      <p class="max-w-sm text-center text-body text-text-secondary">{t('dash.notFound')}</p>
    </div>
  {:else if dashboards.error}
    <div class="flex flex-1 items-center justify-center px-6" data-testid="dashboard-load-error">
      <p class="max-w-sm text-center text-body text-text-secondary">{t('dash.loadError')}</p>
    </div>
  {:else if row}
    <!--
      sandbox="allow-scripts" and nothing else: no allow-same-origin (the frame
      must not read this origin), no allow-forms/popups/top-navigation. The
      {#key} wrapper is the full-document swap: a new key destroys and
      recreates the element (a src change alone would navigate the same
      frame). data-render-gen is how e2e (and a human with devtools) sees a
      swap actually happened.
    -->
    {#key frameKey}
      <iframe
        bind:this={frame}
        {src}
        title={row.name}
        sandbox="allow-scripts"
        class="min-h-0 w-full flex-1 border-0 bg-white"
        style="color-scheme: normal"
        data-testid="dashboard-frame"
        data-render-gen={renderGen}
        onload={onFrameLoad}
      ></iframe>
    {/key}
  {:else}
    <div class="flex flex-1 items-center justify-center" data-testid="dashboard-loading">
      <p class="text-body text-text-muted">…</p>
    </div>
  {/if}
</section>
