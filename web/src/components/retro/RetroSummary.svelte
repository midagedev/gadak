<script lang="ts">
  /*
   * The report's first sentence (GDK-1712).
   *
   * The table below is complete and therefore slow: eight rows against up to
   * twelve columns is a grid you read by scanning, and the question a person
   * opens a retro with — how is the bucket we are in going — was answered
   * only by finding four numbers in the rightmost column. This strip is those
   * four numbers, named, with the step from the bucket before them.
   *
   * It is one elevated surface divided into four, not four cards: four cards
   * would be four objects competing with the table for the eye, and the
   * numbers are set at the table's own text size for the same reason. A retro
   * is not a dashboard — the strip is a summary of what is below it, not a
   * headline above it (THEORY.md G9: progress, not score).
   */
  import { t } from '../../lib/i18n'
  import type { RetroBucket } from '../../lib/types'
  import { METRIC_SPECS, SUMMARY_KEYS, TONE_CLASS, deltaOf, formatValue } from './metrics'

  let {
    bucket,
    previous,
    title,
    labels,
  }: {
    /** The bucket being summarised — the running one when there is one. */
    bucket: RetroBucket
    /** The one before it, for the step. Absent on the first bucket. */
    previous?: RetroBucket
    /** The bucket's own name, already carrying its running marker. */
    title: string
    /** Row label per metric key — the table's, so the two agree word for word. */
    labels: Record<string, string>
  } = $props()

  const cells = $derived(
    SUMMARY_KEYS.map((key) => {
      const spec = METRIC_SPECS.find((m) => m.key === key)!
      return {
        key,
        label: labels[key] ?? key,
        value: formatValue(bucket[spec.key] as number | null, spec.unit),
        delta: deltaOf(
          bucket[spec.key] as number | null,
          previous ? (previous[spec.key] as number | null) : null,
          spec.unit,
          spec.direction,
          bucket.partial,
        ),
      }
    }),
  )
</script>

<div class="mb-4 w-max min-w-[320px] max-w-full rounded-md bg-bg-elevated" data-testid="retro-summary">
  <div class="px-3 pt-2 text-micro font-medium text-text-secondary" data-testid="retro-summary-title">
    {title}
  </div>
  <div class="flex flex-wrap items-stretch px-1 pb-2 pt-1">
    {#each cells as c, i (c.key)}
      <div
        class="min-w-[9rem] flex-1 px-2 py-1 {i > 0 ? 'border-l border-border-subtle' : ''}"
        data-testid="retro-summary-cell"
        data-metric={c.key}
      >
        <div class="flex items-baseline gap-1.5">
          <span class="text-body tabular-nums text-text-primary">{c.value}</span>
          {#if c.delta}
            <span
              class="text-micro tabular-nums {TONE_CLASS[c.delta.tone]}"
              data-testid="retro-summary-delta"
              data-tone={c.delta.tone}
              title={t('retro.vsPrevious')}
            >
              {c.delta.glyph}{c.delta.text}
            </span>
          {/if}
        </div>
        <div class="mt-0.5 whitespace-nowrap text-micro text-text-muted">{c.label}</div>
      </div>
    {/each}
  </div>
</div>
