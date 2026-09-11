<script lang="ts">
  /*
   * The sprint's burn-up, as one small chart (GDK-1710/1752).
   *
   * Where it lives is decided elsewhere — SprintStrip's own comment reserves
   * the line for it and names the bar as the element that yields the width.
   * This component is the drawing itself: hand-built SVG after
   * retro/Sparkline.svelte, not a charting dependency, because there is one
   * shape, no axes, no interaction, and every mark has to stay inside the
   * strip's 24px-tall visual language.
   *
   * Two lines, and the colours say what they are rather than decorating:
   *   scope     — text-secondary, the neutral ink of meta text. Scope is the
   *               container the work happens inside, so it is drawn as
   *               furniture: present, quieter, 1.5px.
   *   completed — status-done, the app's own "this went well" green (2px,
   *               with the end-dot), because the question the chart answers
   *               is "how much of it is done".
   * Both are @theme tokens; no hex enters the file, so light, dark and ink
   * keep their own contrasts (measured: done 7.06:1 / 8.08:1 and secondary
   * 6.06:1 / 6.79:1 on the strip's bg-elevated surface, both ≥3:1 for
   * graphical objects). The pair is an emphasis pair — hue + neutral — not
   * a categorical ramp, which this palette's low chroma does not carry.
   *
   * Two empty states, not one: an origin that keeps no changelog (Linear)
   * cannot answer the question at all, and a sprint with no placeable
   * window has not begun to. Both get the sentence the CLI prints — a flat
   * line would claim "a sprint where nothing happened", which is a
   * different and false claim (GDK-1679).
   *
   * Presentational: the fetch (api.getBurnup) belongs to the surface that
   * mounts this, which already knows the sprint id and when to ask.
   */
  import { t } from '../../lib/i18n'
  import { BURNUP_H, BURNUP_W, burnupGeometry } from './burnup'
  import type { BurnupResponse } from '../../lib/types'

  let { doc }: { doc: BurnupResponse } = $props()

  const geo = $derived(burnupGeometry(doc.burnup.days))

  /** The newest day — what the title reads out. The strip already prints
   *  `done / total` beside this chart; the title is the hover/AT reading of
   *  the same numbers plus the day they are true of. */
  const last = $derived(doc.burnup.days.at(-1) ?? null)

  const summary = $derived(
    last
      ? t('board.burnupTitle', { scope: last.scope, done: last.completed, date: last.date })
      : '',
  )
</script>

{#if !doc.has_history}
  <span
    class="text-micro text-text-muted"
    data-testid="burnup-empty"
    data-reason="no-history"
  >
    {t('board.burnupNoHistory')}
  </span>
{:else if !geo}
  <span
    class="text-micro text-text-muted"
    data-testid="burnup-empty"
    data-reason="no-window"
  >
    {t('board.burnupNoWindow')}
  </span>
{:else}
  <svg
    width={BURNUP_W}
    height={BURNUP_H}
    viewBox="0 0 {BURNUP_W} {BURNUP_H}"
    fill="none"
    role="img"
    aria-label={summary}
    class="flex-none overflow-visible"
    data-testid="burnup-spark"
  >
    <title>{summary}</title>

    <!-- Scope: the neutral container line. Dashed so the completed line stays
         the only solid statement in the box — two solid strokes of different
         colour is where a small chart starts needing a legend. -->
    {#if geo.scopePath}
      <path
        d={geo.scopePath}
        stroke="var(--color-text-secondary)"
        stroke-width="1.5"
        stroke-dasharray="3 2"
        stroke-linecap="round"
        stroke-linejoin="round"
        data-series="scope"
      />
    {/if}
    <!-- The scope line's terminator: the same hollow mark the one-day case
         uses, on the newest day, so the dashed stroke ends on purpose instead
         of mid-air beside the completed line's dot (vision pass 2026-09-11). -->
    {#if geo.scope.length}
      <circle
        cx={geo.scope[geo.scope.length - 1].x}
        cy={geo.scope[geo.scope.length - 1].y}
        r="2.5"
        fill="var(--color-bg-elevated)"
        stroke="var(--color-text-secondary)"
        stroke-width="1.5"
        data-series="scope"
      />
    {/if}

    <!-- Completed: the statement. -->
    {#if geo.donePath}
      <path
        d={geo.donePath}
        stroke="var(--color-status-done)"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        data-series="done"
      />
    {/if}
    {#if geo.endDot}
      <!-- "You are here": the newest day's completed value, with a 2px ring of
           the surface it sits on so the dot stays a dot where the two lines
           meet (scope often equals completed at the end of a closed sprint). -->
      <circle
        cx={geo.endDot.x}
        cy={geo.endDot.y}
        r="4"
        fill="var(--color-status-done)"
        stroke="var(--color-bg-elevated)"
        stroke-width="2"
        data-series="done-dot"
      />
    {/if}
  </svg>
{/if}
