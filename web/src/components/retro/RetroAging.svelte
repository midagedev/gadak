<script lang="ts">
  /*
   * Aging work in progress (GDK-1721).
   *
   * The one figure on this report a person can still act on. Cycle time is
   * what finished work already cost; age is what unfinished work is costing
   * now, and the retro's usual answer to "what should we do this week" is a
   * bar past the p85 line.
   *
   * Drawn by hand, in SVG, for the same reason Sparkline is: one shape, no
   * axes, no legend. The chart is split in two though — the keys are HTML in
   * a left gutter and only the geometry is SVG. The bars scale to the
   * container's width, which means a non-uniform viewBox, and text inside
   * one of those is stretched horizontally by whatever the column happens to
   * be. Keeping the words out of the drawing is the whole trick; the dashes
   * on the p85 line run down the unscaled axis, so they survive it.
   *
   * Colour is the amber the rest of the app spends on "this has sat too
   * long", and only past the line. Everything under it is muted: a chart
   * where thirty bars are all coloured has told the reader nothing.
   */
  import { t } from '../../lib/i18n'
  import { formatDays } from './metrics'
  import { agingChart } from './materials'
  import type { RetroAging } from '../../lib/types'

  let {
    aging,
    onOpen,
  }: {
    aging: RetroAging
    /** Put these issues on the list — the door every number here is. */
    onOpen: (keys: string[]) => void
  } = $props()

  const ROW = 14
  const BAR = 8
  /** User units across. Only X is scaled, so this is an arbitrary grid. */
  const W = 1000

  const chart = $derived(agingChart(aging.items, aging.p85_days))
  const height = $derived(Math.max(chart.bars.length * ROW, ROW))
</script>

{#if !chart.bars.length}
  <p class="text-micro text-text-muted" data-testid="retro-aging-empty">{t('retro.aging.empty')}</p>
{:else}
  <div class="flex max-w-[720px] items-start gap-3" data-testid="retro-aging">
    <!-- The gutter: key, title and age, one line per bar, on the chart's own
         rhythm. The gutter is 27rem — 22rem left the title about 165px, which cut
         every title before its distinguishing word (vision FIX, 2026-09-10);
         at 27rem the title keeps about 245px and the bars keep 240px. It takes
         whatever the key and the age leave and the bars still keep a shape
         (GDK-1737: the key alone left the reader with a column of
         identifiers and the title only in a tooltip). -->
    <div class="w-[27rem] flex-none">
      {#each chart.bars as b (b.key)}
        <button
          type="button"
          class="flex h-[14px] w-full items-center gap-2 rounded text-left text-micro leading-none hover:bg-bg-hover"
          data-testid="retro-aging-row"
          data-key={b.key}
          data-over={b.over ? '1' : '0'}
          title={b.summary}
          onclick={() => onOpen([b.key])}
        >
          <span class="w-[7.5rem] flex-none truncate {b.over ? 'text-text-primary' : 'text-text-secondary'}">{b.key}</span>
          <span class="min-w-0 flex-1 truncate text-text-secondary" data-testid="retro-aging-title">{b.summary}</span>
          <span class="w-[3.25rem] flex-none text-right tabular-nums {b.over ? 'text-status-stale' : 'text-text-muted'}"
            >{formatDays(b.days)}</span
          >
        </button>
      {/each}
    </div>
    <div class="min-w-[240px] flex-1">
      <svg
        width="100%"
        {height}
        viewBox="0 0 {W} {height}"
        preserveAspectRatio="none"
        fill="none"
        aria-hidden="true"
        data-testid="retro-aging-chart"
      >
        {#each chart.bars as b, i (b.key)}
          <rect
            x="0"
            y={i * ROW + (ROW - BAR) / 2}
            width={Math.max(b.width * W, 1)}
            height={BAR}
            fill="currentColor"
            class={b.over ? 'text-status-stale' : 'text-text-muted opacity-45'}
          />
        {/each}
        {#if chart.p85At != null}
          <!-- The line the eye actually reads the chart against. Two strokes:
               a ground-coloured halo first, then the dots in the primary ink,
               so the line survives crossing an amber bar (vision round
               2026-09-09: on a real bucket with many bars past p85 the dotted
               line vanished inside the fill). Both are kept at device pixels
               through the horizontal scale. -->
          <line
            x1={chart.p85At * W}
            x2={chart.p85At * W}
            y1="0"
            y2={height}
            stroke="currentColor"
            class="text-bg-base"
            stroke-width="3"
            vector-effect="non-scaling-stroke"
          />
          <line
            x1={chart.p85At * W}
            x2={chart.p85At * W}
            y1="0"
            y2={height}
            stroke="currentColor"
            class="text-text-primary"
            stroke-width="1"
            stroke-dasharray="2 2"
            vector-effect="non-scaling-stroke"
            data-testid="retro-aging-p85"
          />
        {/if}
      </svg>
      {#if chart.p85Days != null}
        <!-- "p85" is the same three characters in every locale — the same
             reason the scatter's own two labels are literals and not catalog
             keys (a ko value byte-equal to en is a catalog failure, and
             rightly). -->
        <div class="mt-1 text-micro tabular-nums text-text-muted" data-testid="retro-aging-p85-label">
          p85 {formatDays(chart.p85Days)}
        </div>
      {/if}
    </div>
  </div>
  {#if chart.more}
    <button
      type="button"
      class="mt-1 rounded px-1 text-micro text-text-muted underline decoration-border-strong decoration-dotted underline-offset-4 hover:bg-bg-hover hover:decoration-accent"
      data-testid="retro-aging-more"
      onclick={() => onOpen(aging.items.map((i) => i.key))}
    >
      {t('retro.aging.more', { n: chart.more })}
    </button>
  {/if}
{/if}
