<script lang="ts">
  /*
   * The queue — screen 1 (market-report: the quiet queue is the product).
   * 1차 scope: unresolved rows by priority_rank. Offline/failure keeps the
   * last synced list plus a banner — an empty screen is the #5049 failure
   * mode, never ours. Returning to the foreground revalidates immediately
   * (orca #5049: a session that only looks alive).
   */
  import { bootstrap, categoryInk, priorityInk, queueRows, type ApiContext, type QueueRow } from '../lib/api'
  import { readPairing, readQueueCache, readToken, writeQueueCache } from '../lib/settings'
  import { categoryLabel, t } from '../lib/i18n'

  let { onpair } = $props<{ onpair?: () => void }>()

  let rows: QueueRow[] = $state([])
  let openCount = $state(0)
  let loading = $state(true)
  // offline is set when the latest fetch failed; rows may still hold the
  // last good data (from memory or the localStorage cache).
  let offline = $state(false)
  let syncedAt = $state<string | null>(null)
  let everLoaded = $state(false)

  function timeLabel(iso: string): string {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return ''
    return d.toLocaleTimeString()
  }

  async function load(): Promise<void> {
    loading = true
    const pairing = readPairing()
    // Dev default: the vite proxy targets the dev serve port, so an
    // unpaired dev install still sees data. The packaged app requires a
    // pairing (endpoint) first.
    const endpoint = pairing?.endpoint ?? (import.meta.env.DEV ? 'http://127.0.0.1:7899' : '')
    if (endpoint === '') {
      loading = false
      offline = false
      return
    }
    const ctx: ApiContext = { endpoint, token: await readToken() }
    try {
      const res = await bootstrap(ctx)
      // Without a prior etag the server never answers 304; the branch
      // exists so the caching round can land without touching this screen.
      if (res.status === 'not_modified') {
        offline = false
        return
      }
      const boot = res.data
      const all = queueRows(boot.issues)
      openCount = boot.issues.filter((i) => i.status_category !== 'done').length
      rows = all
      offline = false
      everLoaded = true
      syncedAt = new Date().toISOString()
      writeQueueCache(all)
    } catch {
      offline = true
      if (!everLoaded) {
        const cache = readQueueCache()
        if (cache) {
          rows = cache.rows
          openCount = cache.rows.length
          syncedAt = cache.syncedAt
        }
      }
    } finally {
      loading = false
    }
  }

  $effect(() => {
    void load()
    const onVisible = (): void => {
      if (document.visibilityState === 'visible') void load()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  })
</script>

<section class="m-main scroll-region" data-testid="queue-screen">
  {#if offline}
    <div
      class="m-banner"
      role="status"
      data-testid="offline-banner"
      data-ever-loaded={everLoaded ? '1' : '0'}
    >
      <p>{everLoaded || rows.length > 0 ? t('queue.banner.offline') : t('queue.banner.neverLoaded')}</p>
      {#if syncedAt}
        <span class="m-banner-time">{t('queue.banner.lastSync', { time: timeLabel(syncedAt) })}</span>
      {/if}
    </div>
  {/if}

  {#if rows.length > 0}
    <header class="m-queue-header">
      <h1 class="type-subject m-queue-title">{t('queue.title')}</h1>
      <span class="m-count" data-testid="queue-count">{t('queue.count', { n: openCount })}</span>
      <button
        class="m-refresh"
        type="button"
        onclick={() => void load()}
        disabled={loading}
        aria-label={t('queue.refresh')}
        data-testid="refresh"
      >
        {loading ? t('queue.loading') : t('queue.refresh')}
      </button>
    </header>
    <ul class="m-rows" data-testid="queue-rows">
      {#each rows as row (row.issue_key)}
        <li class="m-row" data-testid="queue-row" data-key={row.issue_key}>
          <span class="m-row-priority" style:color={priorityInk(row.priority_rank)} aria-hidden="true"></span>
          <span class="m-row-key">{row.issue_key}</span>
          <span class="m-row-summary">{row.summary}</span>
          <span class="m-row-status" style:color={categoryInk(row.status_category)}>
            {t(categoryLabel(row.status_category))}
          </span>
          <span class="m-row-prio-label">{row.priority ?? t('priority.none')}</span>
        </li>
      {/each}
    </ul>
  {:else if loading}
    <p class="m-empty" data-testid="loading">{t('queue.loading')}</p>
  {:else if everLoaded}
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
