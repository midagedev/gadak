<script lang="ts">
  /*
   * Weekly retro (GDK-1660) — the document `gadak retro` prints, as a
   * main-column view. One column per ISO week, one row per metric; the row
   * label carries the metric's own definition, so a number never stands
   * without the sentence that says what it counts. A cell that has issue
   * keys behind it is a door (THEORY.md G8): clicking puts those issues on
   * the list, the same move `retro --open` makes. Visual language follows
   * HistoryView — header, segmented range, no new tokens.
   */
  import ColumnHeader from '../ui/ColumnHeader.svelte'
  import LoadingState from '../ui/LoadingState.svelte'
  import EmptyState from '../list/EmptyState.svelte'
  import { t, locale } from '../../lib/i18n'
  import { getRetro } from '../../lib/api'
  import type { RetroBucket } from '../../lib/types'
  import { emptyConfig } from '../../lib/view-config'
  import { showIssueList } from '../../lib/show-issue-list'
  import { pages } from '../../stores/pages.svelte'
  import { createSkeletonGrace } from '../../lib/skeleton-grace.svelte'
  import { createResource } from '../../lib/resource.svelte'

  const RANGES = ['4w', '8w', '12w'] as const
  type Range = (typeof RANGES)[number]
  const RANGE_LABEL: Record<Range, string> = {
    '4w': t('retro.range4w'),
    '8w': t('retro.range8w'),
    '12w': t('retro.range12w'),
  }

  let since = $state<Range>('4w')
  // The shared resource rune: range change → reload, stale answers dropped.
  const res = createResource(
    () => since,
    (k) => getRetro(k),
  )
  const doc = $derived(res.data)
  const loading = $derived(res.loading)
  const failed = $derived(res.errorKind !== null)

  const skeleton = createSkeletonGrace(() => loading && !doc)

  type Metric = {
    /** JSON row name (also the definitions key). */
    key: keyof RetroBucket
    label: string
    unit: 'count' | 'seconds' | 'days'
    /** Which key array opens the cell, if any. */
    keys?: 'closed' | 'in progress' | 'mismatch' | 'cycle'
  }
  const METRICS: Metric[] = [
    { key: 'sessions', label: t('retro.sessions'), unit: 'count' },
    { key: 'resume (median)', label: t('retro.resume'), unit: 'seconds' },
    { key: 'closed', label: t('retro.closed'), unit: 'count', keys: 'closed' },
    { key: 'cycle p50', label: t('retro.cycleP50'), unit: 'days', keys: 'cycle' },
    { key: 'cycle p85', label: t('retro.cycleP85'), unit: 'days', keys: 'cycle' },
    { key: 'in progress', label: t('retro.inProgress'), unit: 'count', keys: 'in progress' },
    { key: 'wip age max', label: t('retro.wipAge'), unit: 'days' },
    { key: 'mismatch', label: t('retro.mismatch'), unit: 'count', keys: 'mismatch' },
  ]

  const buckets = $derived(doc?.buckets ?? [])
  const empty = $derived(buckets.every((b) => b.sessions === 0 && !b.closed && !b['in progress']))

  function weekLabel(b: RetroBucket): string {
    const from = new Date(b.from)
    // `to` is exclusive; the header names the last day the week holds.
    const last = new Date(new Date(b.to).getTime() - 86_400_000)
    const f = new Intl.DateTimeFormat(locale(), { month: 'short', day: 'numeric' })
    const sameMonth = from.getMonth() === last.getMonth()
    const end = sameMonth ? new Intl.DateTimeFormat(locale(), { day: 'numeric' }).format(last) : f.format(last)
    return `${f.format(from)} – ${end}`
  }

  function cell(b: RetroBucket, m: Metric): string {
    const v = b[m.key] as number | null | undefined
    if (v == null) return '—'
    if (m.unit === 'count') return String(v)
    if (m.unit === 'seconds') {
      if (v < 60) return `${Math.round(v)}s`
      if (v < 3600) return `${Math.round(v / 60)}m`
      return `${(v / 3600).toFixed(1)}h`
    }
    // days: the JSON is already rounded the way the CLI table prints it.
    return `${v.toFixed(1)}d`
  }

  function keysOf(b: RetroBucket, m: Metric): string[] {
    return m.keys ? b.keys[m.keys] : []
  }

  function open(keys: string[]): void {
    if (!keys.length) return
    const c = emptyConfig()
    c.filters.keys = keys
    c.display.group_by = 'none'
    showIssueList(c)
  }
</script>

<section class="flex h-full min-h-0 flex-col bg-bg-base" data-testid="retro-view" data-skeleton={skeleton.attr}>
  <ColumnHeader title={t('retro.title')} closeTestid="retro-close" onClose={() => pages.closeRetro()}>
    <div class="ml-1 flex flex-none items-center gap-0.5 rounded-md bg-bg-elevated p-1">
      {#each RANGES as r (r)}
        <button
          type="button"
          class="flex h-control-sm items-center rounded px-2 text-micro font-medium {since === r
            ? 'bg-bg-active text-text-primary'
            : 'text-text-muted hover:text-text-secondary'}"
          aria-pressed={since === r}
          data-testid="retro-range"
          data-range={r}
          onclick={() => (since = r)}
        >
          {RANGE_LABEL[r]}
        </button>
      {/each}
    </div>
  </ColumnHeader>

  {#if failed}
    <EmptyState icon="warning" title={t('retro.loadFailed')} actionLabel={t('common.retry')} onAction={() => res.reload()} />
  {:else if loading && !doc}
    {#if skeleton.visible}
      <LoadingState />
    {/if}
  {:else if doc && empty}
    <EmptyState icon="" title={t('retro.empty')} />
  {:else if doc}
    <div class="min-h-0 flex-1 overflow-auto px-3 py-3">
      <table class="w-max min-w-full border-separate border-spacing-0 text-body" data-testid="retro-table">
        <thead>
          <tr>
            <th class="sticky left-0 z-10 bg-bg-base pb-2 pr-6 text-left text-micro font-medium text-text-muted"></th>
            {#each buckets as b (b.from)}
              <th class="whitespace-nowrap pb-2 pl-6 text-right text-micro font-medium {b.partial ? 'text-text-secondary' : 'text-text-muted'}" data-testid="retro-week">
                {weekLabel(b)}
                {#if b.partial}<span class="ml-1 font-normal text-text-muted">· {t('retro.thisWeek')}</span>{/if}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each METRICS as m (m.key)}
            <tr class="border-t border-border-subtle">
              <th
                class="sticky left-0 z-10 max-w-[260px] bg-bg-base py-2 pr-6 text-left align-top font-normal"
                title={doc.definitions[m.key] ?? ''}
              >
                <div class="text-body text-text-primary">{m.label}</div>
                <div class="mt-0.5 line-clamp-2 text-micro leading-snug text-text-muted">{doc.definitions[m.key] ?? ''}</div>
              </th>
              {#each buckets as b (b.from)}
                {@const keys = keysOf(b, m)}
                {@const text = cell(b, m)}
                <td class="py-2 pl-6 text-right align-top tabular-nums {text === '—' ? 'text-text-muted' : 'text-text-primary'}">
                  {#if keys.length}
                    <button
                      type="button"
                      class="-mr-2 rounded px-2 py-0.5 tabular-nums underline decoration-border-strong decoration-dotted underline-offset-4 transition-colors hover:bg-bg-hover hover:decoration-accent"
                      data-testid="retro-cell"
                      data-metric={m.key}
                      title={t('retro.openIssues')}
                      onclick={() => open(keys)}
                    >
                      {text}{#if b.keys.keys_truncated && keys.length >= 500}<span class="ml-0.5 text-micro text-text-muted">{t('retro.truncated')}</span>{/if}
                    </button>
                  {:else}
                    {text}
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>
