<script lang="ts">
  /*
   * What closed (GDK-1723).
   *
   * The table says a number; this says what the number was made of. Two
   * small cuts — by issue type and by epic — because "we closed eleven" and
   * "we closed eleven, nine of them bugs" are different retros, and the
   * second one is the one with a conversation in it.
   *
   * Then the scatter. p50 and p85 are two numbers standing in for a shape,
   * and the shape is what says whether the tail is one stuck issue or the
   * way this team works. A dot per resolved issue costs nothing to draw and
   * answers that directly; the two dashed lines are the same percentiles the
   * table above prints, so the reader can see which points made them.
   *
   * Everything with a count behind it is a door, the way every cell in this
   * report has been since GDK-1660.
   */
  import { t } from '../../lib/i18n'
  import { formatDays } from './metrics'
  import { cycleScatter, setCount } from './materials'
  import type { RetroBucket, RetroClosedGroup } from '../../lib/types'

  let {
    bucket,
    onOpen,
  }: {
    bucket: RetroBucket
    onOpen: (keys: string[]) => void
  } = $props()

  const byType = $derived(bucket.closed_by_type ?? [])
  const byEpic = $derived(bucket.closed_by_epic ?? [])
  const unplanned = $derived(setCount(bucket.unplanned))
  const points = $derived(bucket.cycle_points ?? [])
  const scatter = $derived(cycleScatter(points, bucket.from, bucket.to))
  const anything = $derived(byType.length || byEpic.length || points.length)

  /** The scatter's height in px — tall enough for a shape, short enough
   *  that the section stays a paragraph rather than a dashboard panel. */
  const H = 96

  function groupLabel(g: RetroClosedGroup): string {
    if (g.epic_key !== undefined) return g.title || g.epic_key || t('retro.closed.noEpic')
    return g.issue_type || g.issue_type_id || ''
  }
  /** The whole label, for the tooltip: the title the row truncates, and the
   *  epic key it belongs to when the two are different things. */
  function groupTitle(g: RetroClosedGroup): string {
    const label = groupLabel(g)
    return g.epic_key && g.title ? `${label} · ${g.epic_key}` : label
  }
  function maxOf(rows: RetroClosedGroup[]): number {
    return rows.length ? Math.max(...rows.map((r) => r.count)) : 0
  }
</script>

{#if !anything}
  <p class="text-micro text-text-muted" data-testid="retro-closed-empty">{t('retro.closed.none')}</p>
{:else}
  <div class="flex max-w-[720px] flex-wrap gap-6" data-testid="retro-closed">
    {#each [{ id: 'type', title: t('retro.closed.byType'), rows: byType }, { id: 'epic', title: t('retro.closed.byEpic'), rows: byEpic }] as col (col.id)}
      {#if col.rows.length}
        {@const max = maxOf(col.rows)}
        <div class="min-w-[15rem] flex-1">
          <div class="text-micro text-text-muted">{col.title}</div>
          <div class="mt-1" data-testid="retro-closed-group" data-group={col.id}>
            {#each col.rows as g, i (`${col.id}:${g.epic_key ?? g.issue_type_id ?? i}`)}
              <button
                type="button"
                class="flex w-full items-center gap-2 rounded py-0.5 text-left text-micro hover:bg-bg-hover"
                data-testid="retro-closed-row"
                onclick={() => onOpen(g.keys)}
              >
                <!-- The label takes the row rather than a fixed 8rem
                     (GDK-1738): an epic title is as long as someone wrote it,
                     and in Korean the same width holds half the characters.
                     The key rides behind the title so a truncated row is
                     still identifiable, and the whole title is the tooltip. -->
                <span
                  class="flex min-w-[8rem] max-w-[50%] flex-1 items-baseline gap-1 truncate text-text-secondary"
                  data-testid="retro-closed-label"
                  title={groupTitle(g)}
                >
                  <span class="truncate">{groupLabel(g)}</span>
                  {#if g.epic_key && g.title}
                    <span class="flex-none tabular-nums text-text-muted" data-testid="retro-closed-epic-key">{g.epic_key}</span>
                  {/if}
                </span>
                <!-- The bar is the comparison; the number is the value. Both,
                     because a five-row chart is read either way. -->
                <span class="h-[6px] min-w-[6rem] flex-1">
                  <span
                    class="block h-[6px] rounded-sm bg-text-muted opacity-40"
                    style:width="{max > 0 ? (g.count / max) * 100 : 0}%"
                  ></span>
                </span>
                <span class="w-6 flex-none text-right tabular-nums text-text-primary">{g.count}</span>
              </button>
            {/each}
          </div>
        </div>
      {/if}
    {/each}
  </div>

  {#if bucket.unplanned}
    <button
      type="button"
      class="mt-2 rounded px-1 text-micro text-text-muted underline decoration-border-strong decoration-dotted underline-offset-4 hover:bg-bg-hover hover:decoration-accent"
      data-testid="retro-unplanned"
      onclick={() => onOpen(bucket.unplanned?.keys ?? [])}
    >
      {t('retro.closed.unplanned', { n: unplanned })}
    </button>
  {/if}

  {#if points.length}
    <div class="mt-3 max-w-[720px]">
      <div class="text-micro text-text-muted">{t('retro.closed.cycle')}</div>
      <!--
        Positioned in CSS rather than drawn in an SVG grid. A scatter needs a
        uniform scale to keep its dots round, and this box is wide and short
        — an SVG that fits a 1000-unit grid into it either letterboxes or
        stretches every circle into an ellipse. Percentages have neither
        problem, and the dashed percentile lines are then one border each.
      -->
      <div
        class="relative mt-1 border-b border-border-subtle"
        style:height="{H}px"
        data-testid="retro-cycle-scatter"
      >
        <!--
          Two lines, two weights (vision round 2026-09-09: identical dashes
          left a reader unable to tell p50 from p85). p50 is solid and p85
          dotted, and each carries its own value at its right end, so the
          line and its number are one thing rather than a footer to decode.
          The label sits at the LEFT end on a ground-coloured chip: the right
          end is where the newest closures cluster (the second vision pass
          found the dots sitting on the decimal point), and a bucket's first
          day rarely resolves anything.
        -->
        {#each [{ at: scatter.p50At, value: scatter.p50, id: 'p50', dash: false }, { at: scatter.p85At, value: scatter.p85, id: 'p85', dash: true }] as line (line.id)}
          {#if line.at != null}
            <div
              class="pointer-events-none absolute inset-x-0 border-t border-border-strong {line.dash ? 'border-dashed' : 'border-solid'}"
              style:bottom="{line.at * 100}%"
              data-testid="retro-cycle-line"
              data-line={line.id}
            ></div>
            {#if line.value != null}
              <span
                class="pointer-events-none absolute left-0 -translate-y-full rounded-sm bg-bg-base px-0.5 text-micro leading-none tabular-nums text-text-muted"
                style:bottom="{line.at * 100}%"
                data-testid="retro-cycle-line-label"
                data-line={line.id}>{line.id} {formatDays(line.value)}</span
              >
            {/if}
          {/if}
        {/each}
        {#each scatter.points as p, i (`${p.key}:${i}`)}
          <button
            type="button"
            class="absolute h-[7px] w-[7px] -translate-x-1/2 translate-y-1/2 rounded-full bg-text-muted opacity-55 hover:opacity-100"
            style:left="{2 + p.x * 96}%"
            style:bottom="{p.y * 100}%"
            data-testid="retro-cycle-point"
            data-key={p.key}
            aria-label={p.key}
            title={p.summary ? `${p.key} · ${p.summary} · ${formatDays(p.days)}` : `${p.key} · ${formatDays(p.days)}`}
            onclick={() => onOpen([p.key])}
          ></button>
        {/each}
      </div>
      <div class="mt-0.5 flex gap-3 text-micro text-text-muted">
        {#if scatter.clipped}<span data-testid="retro-cycle-clipped">{t('retro.closed.clipped', { n: scatter.clipped })}</span>{/if}
      </div>
    </div>
  {/if}
{/if}
