<script lang="ts">
  /*
   * Freshness chip — the server↔Jira leg, the one nothing else on screen shows.
   *  The 15s delta poll already keeps browser↔server current, so a list that
   *  looks live can still be a live view of a mirror that stopped an hour ago.
   *  Source: sync_health's jira source, whose synced_at is the last sync run
   *  that finished without an error, and whose status carries the server's own
   *  staleness rule (10 missed watch ticks) — the chip and the sidebar badge
   *  read the same field so they cannot disagree.
   *  Click runs the same sync-now the palette does.
   */
  import { onMount } from 'svelte'
  import { issues } from '../../stores/issues.svelte'
  import { t, relativeTime, absTime } from '../../lib/i18n'
  import { isHostedDemo } from '../../lib/config'

  /** Relabel cadence. relativeTime is minute-granular below the hour, so a 1s
      tick would re-render 59 times to print the same string. */
  const TICK_MS = 10_000

  let tick = $state(0)
  onMount(() => {
    const id = setInterval(() => (tick += 1), TICK_MS)
    return () => clearInterval(id)
  })

  const health = $derived(issues.mirrorHealth)
  const syncedAt = $derived(health?.synced_at ?? null)

  type Level = 'fresh' | 'stale' | 'failed' | 'never'
  const level = $derived.by<Level>(() => {
    const status = health?.status
    if (status === 'failed') return 'failed'
    if (status === 'missing' || !syncedAt) return 'never'
    if (status === 'stale') return 'stale'
    return 'fresh'
  })

  const label = $derived.by(() => {
    void tick // re-read the wall clock every tick
    if (issues.mirrorSyncing) return t('freshness.syncing')
    if (level === 'failed') return t('freshness.failed')
    if (level === 'never') return t('freshness.never')
    return t('freshness.synced', { when: relativeTime(syncedAt, 'long') })
  })

  const title = $derived.by(() => {
    void tick
    if (level === 'failed') return t('freshness.titleFailed', { message: health?.message ?? '' })
    if (level === 'never') return t('freshness.titleNever')
    const key = level === 'stale' ? 'freshness.titleStale' : 'freshness.titleFresh'
    const abs = absTime(syncedAt)
    return `${t(key, { when: relativeTime(syncedAt, 'long') })}${abs ? `\n${abs}` : ''}`
  })

  // Same semantic tones as the sidebar sync badge — no new colors. While a pull
  // runs the old state has stopped being the message, so it drops back to muted.
  const tone = $derived(
    issues.mirrorSyncing
      ? 'text-text-muted'
      : level === 'failed'
        ? 'text-status-reopen'
        : level === 'stale' || level === 'never'
          ? 'text-status-stale'
          : 'text-text-muted',
  )
  const dot = $derived(
    level === 'failed'
      ? 'bg-status-reopen'
      : level === 'stale' || level === 'never'
        ? 'bg-status-stale'
        : 'bg-status-done',
  )

  // Hidden where there is nothing to sync: the hosted demo is a static snapshot,
  // and a server that reports no source has no mirror age to show.
  const visible = $derived(!isHostedDemo() && health !== null)
</script>

{#if visible}
  <button
    type="button"
    class="inline-flex h-control-sm flex-none items-center gap-1.5 rounded-md px-1.5 text-[12px] {tone} transition-colors {issues.mirrorSyncing
      ? 'cursor-progress'
      : 'hover:bg-bg-hover hover:text-text-primary'}"
    data-testid="freshness-chip"
    data-state={issues.mirrorSyncing ? 'syncing' : level}
    disabled={issues.mirrorSyncing}
    aria-label={t('freshness.label')}
    {title}
    onclick={() => void issues.pullMirror()}
  >
    {#if issues.mirrorSyncing}
      <svg
        class="h-3 w-3 flex-none animate-spin"
        viewBox="0 0 12 12"
        fill="none"
        aria-hidden="true"
      >
        <circle cx="6" cy="6" r="4.5" stroke="currentColor" stroke-width="1.5" opacity="0.3" />
        <path
          d="M10.5 6A4.5 4.5 0 0 0 6 1.5"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
        />
      </svg>
    {:else}
      <span class="h-1.5 w-1.5 flex-none rounded-full {dot}" aria-hidden="true"></span>
    {/if}
    {label}
  </button>
{/if}
