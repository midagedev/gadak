<script lang="ts">
  /*
   * The queue — screen 1 (market-report: the quiet queue is the product).
   *
   * GDK-801 queue half. Loading is stale-while-revalidate (ux-report Q5):
   * the localStorage cache (or the in-memory last good fetch) paints
   * immediately and revalidation only moves the header dot; the skeleton
   * appears only when there is nothing to paint. Failure keeps the last
   * synced rows plus the banner — an empty screen is the #5049 failure
   * mode, never ours. Foreground return and pull-to-refresh both
   * revalidate.
   *
   * Row grammar is the web IssueRow semantics on a phone (ux-report Q1):
   * priority tick · key · 1-line summary · status NAME (the category is
   * the dot's color only) · compact age. The mine/all filter keys on
   * assignee_id — account ids, never display names (CLAUDE.md).
   */
  import { bootstrap, me, type ApiContext } from '../lib/api'
  import { readPairing, readQueueCache, readToken, writeQueueCache } from '../lib/settings'
  import { t } from '../lib/i18n'
  import {
    defaultMode,
    rowView,
    syncAgeLabel,
    visibleRows,
    type QueueMode,
    type QueueRowFull,
  } from '../lib/queue-rows'

  let {
    onpair,
    onopen,
  }: { onpair?: () => void; onopen?: (issueKey: string) => void } = $props()

  // The open pool straight from bootstrap — rows derive from it so the
  // mine/all toggle recomputes without a refetch.
  let pool = $state<QueueRowFull[]>([])
  let accountId = $state('')
  // null = follow me(): a connected home defaults to mine, standalone to all.
  let mode = $state<QueueMode | null>(null)
  // offline is set when the latest fetch failed; the pool may still hold
  // the last good data (from memory or the localStorage cache).
  let offline = $state(false)
  let revalidating = $state(false)
  let syncedAt = $state<string | null>(null)
  let everLoaded = $state(false)
  // A cold-start cache draw happened (possibly an empty one — "all done"
  // was a real sync result, not "nothing here yet").
  let cacheDrawn = $state(false)

  const effectiveMode = $derived(mode ?? defaultMode(accountId))
  const rows = $derived(visibleRows(pool, effectiveMode, accountId))
  // Skeleton only when there is nothing to paint (ux-report Q5 ①).
  const firstLoading = $derived(revalidating && !everLoaded && !cacheDrawn)
  // '' when never synced (or a legacy cache with no stamp) — the line then
  // shows the pair label alone, never a dangling separator.
  const freshAge = $derived(syncAgeLabel(syncedAt))
  const pairLabel = readPairing()?.label ?? ''

  function timeLabel(iso: string): string {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return ''
    return d.toLocaleTimeString()
  }

  async function load(): Promise<void> {
    if (revalidating) return
    const pairing = readPairing()
    // Dev default: the vite proxy targets the dev serve port, so an
    // unpaired dev install still sees data. The packaged app requires a
    // pairing (endpoint) first.
    const endpoint = pairing?.endpoint ?? (import.meta.env.DEV ? 'http://127.0.0.1:7899' : '')
    if (endpoint === '') return
    revalidating = true
    const ctx: ApiContext = { endpoint, token: await readToken() }
    try {
      // me() failing must not fail the queue: the filter falls back to all
      // and keeps the last known account id.
      const [res, meRes] = await Promise.all([bootstrap(ctx), me(ctx).catch(() => null)])
      if (res.status === 'ok') {
        const open = res.data.issues.filter((i) => i.status_category !== 'done')
        pool = open
        if (meRes?.account_id) accountId = meRes.account_id
        writeQueueCache(open)
      }
      // not_modified: nothing changed — the validation itself succeeded, so
      // the freshness clock and flags move too (no cache rewrite needed).
      offline = false
      everLoaded = true
      syncedAt = new Date().toISOString()
    } catch {
      offline = true
    } finally {
      revalidating = false
    }
  }

  function toggleMode(): void {
    mode = effectiveMode === 'mine' ? 'all' : 'mine'
  }

  /* ── pull-to-refresh ──
     The scroller owns the gesture when it sits at the top; a drag down
     past the slop reveals the spinner, release past the fire line
     revalidates. Listeners stay passive — native vertical panning is kept
     (touch-action: pan-y), and the scroller's own rubber-band is off
     (overscroll-behavior-y: none, app.css) so the indicator is the only
     thing the pull moves. */
  const PTR_ARM = 8 // slop px before a drag counts as a pull
  const PTR_REVEAL = 56 // max revealed px
  const PTR_FIRE = 30 // release at/after this revalidates

  let scroller: HTMLElement | undefined = $state()
  let pullStart: number | null = null
  let pull = $state(0)

  function onTouchStart(e: TouchEvent): void {
    pullStart = (scroller?.scrollTop ?? 1) <= 0 ? e.touches[0].clientY : null
  }

  function onTouchMove(e: TouchEvent): void {
    if (pullStart === null || !scroller) return
    if (scroller.scrollTop > 0) {
      // Became a scroll, not a pull — disarm until the next touch.
      pullStart = null
      pull = 0
      return
    }
    const drag = e.touches[0].clientY - pullStart
    if (drag <= 0) {
      pull = 0
      return
    }
    pull = Math.min(Math.max(drag - PTR_ARM, 0) * 0.5, PTR_REVEAL)
  }

  function onTouchEnd(): void {
    if (pull >= PTR_FIRE) void load()
    pullStart = null
    pull = 0
  }

  $effect(() => {
    // Cold start: paint the cache before the first fetch resolves.
    const cache = readQueueCache()
    if (cache) {
      pool = cache.rows
      syncedAt = cache.syncedAt
      cacheDrawn = true
    }
    void load()
    const onVisible = (): void => {
      if (document.visibilityState === 'visible') void load()
    }
    document.addEventListener('visibilitychange', onVisible)

    const el = scroller
    const tap = { passive: true } as AddEventListenerOptions
    el?.addEventListener('touchstart', onTouchStart, tap)
    el?.addEventListener('touchmove', onTouchMove, tap)
    el?.addEventListener('touchend', onTouchEnd, tap)
    el?.addEventListener('touchcancel', onTouchEnd, tap)
    return () => {
      document.removeEventListener('visibilitychange', onVisible)
      el?.removeEventListener('touchstart', onTouchStart)
      el?.removeEventListener('touchmove', onTouchMove)
      el?.removeEventListener('touchend', onTouchEnd)
      el?.removeEventListener('touchcancel', onTouchEnd)
    }
  })
</script>

<section
  class="m-main scroll-region m-queue-scroll"
  bind:this={scroller}
  data-testid="queue-screen"
>
  <!-- Pull-to-refresh spinner: lives in the gutter above the content,
       translateY reveals it. Height/aria keep it out of reading order. -->
  <div
    class="m-ptr"
    data-testid="ptr"
    style:transform="translateY({pull - 44}px)"
    aria-hidden="true"
  >
    <span
      class="m-ptr-spinner"
      class:m-ptr-armed={pull >= PTR_FIRE || revalidating}
    ></span>
  </div>

  {#if offline}
    <div
      class="m-banner"
      role="status"
      data-testid="offline-banner"
      data-ever-loaded={everLoaded || cacheDrawn ? '1' : '0'}
    >
      <p>{everLoaded || rows.length > 0 ? t('queue.banner.offline') : t('queue.banner.neverLoaded')}</p>
      {#if syncedAt}
        <span class="m-banner-time">{t('queue.banner.lastSync', { time: timeLabel(syncedAt) })}</span>
      {/if}
    </div>
  {/if}

  {#if everLoaded || cacheDrawn}
    <header class="m-queue-header">
      <div class="m-queue-toprow">
        <h1 class="type-subject m-queue-title">{t('queue.title')}</h1>
        <span class="m-count" data-testid="queue-count">{t('queue.count', { n: rows.length })}</span>
        {#if accountId !== ''}
          <button
            class="m-header-btn m-filter"
            type="button"
            onclick={toggleMode}
            aria-pressed={effectiveMode === 'mine'}
            data-testid="queue-filter"
            data-mode={effectiveMode}
          >
            {effectiveMode === 'mine' ? t('queue.filter.mine') : t('queue.filter.all')}
          </button>
        {/if}
        <button
          class="m-header-btn m-refresh"
          type="button"
          onclick={() => void load()}
          disabled={revalidating}
          aria-label={t('queue.refresh')}
          data-testid="refresh"
        >
          {revalidating ? t('queue.loading') : t('queue.refresh')}
        </button>
      </div>
      <!-- Freshness line (ux-report Q5 ③): pair label + the phone↔home
           clock (NOT the home↔Jira mirror clock). While revalidating it
           says so; when the last attempt failed the slot becomes the
           offline chip — the banner below carries the details. -->
      <p class="m-queue-fresh" data-testid="queue-freshness" data-state={offline ? 'offline' : revalidating ? 'syncing' : 'synced'}>
        {#if offline}
          <span class="m-fresh-chip" data-testid="freshness-offline">{t('queue.freshness.offline')}</span>
        {:else if revalidating}
          <span class="m-fresh-dot" aria-hidden="true"></span>{t('queue.freshness.syncing')}
        {:else}
          {#if pairLabel && freshAge}{pairLabel} · {freshAge}{:else}{pairLabel || freshAge}{/if}
        {/if}
      </p>
    </header>
  {/if}

  {#if rows.length > 0}
    <ul class="m-rows" data-testid="queue-rows">
      {#each rows as row (row.issue_key)}
        {@const view = rowView(row)}
        <li data-testid="queue-row" data-key={row.issue_key}>
          <button class="m-row" type="button" onclick={() => onopen?.(row.issue_key)}>
            <span class="m-row-priority" style:color={view.priority_ink} aria-hidden="true"></span>
            <span class="m-row-key">{view.issue_key}</span>
            <span class="m-row-summary">{view.summary}</span>
            <span class="m-row-status">
              <span class="m-row-dot" style:color={view.status_ink} aria-hidden="true"></span>
              <span class="m-row-status-name">{view.status_text}</span>
            </span>
            <span class="m-row-age">{view.age}</span>
          </button>
        </li>
      {/each}
    </ul>
  {:else if firstLoading}
    <p class="m-empty" data-testid="loading">{t('queue.loading')}</p>
  {:else if everLoaded || cacheDrawn}
    <div class="m-empty" data-testid="empty-state" data-reason="allDone">
      <p>{t('queue.empty.allDone')}</p>
    </div>
  {:else}
    <div class="m-empty" data-testid="empty-state" data-reason="never">
      <p class="type-subject">{t('queue.empty.never.title')}</p>
      <p>{t('queue.empty.never.body')}</p>
      {#if onpair}
        <button class="m-button" type="button" onclick={onpair} data-testid="queue-pair-cta">{t('queue.empty.pair')}</button>
      {/if}
    </div>
  {/if}
</section>
