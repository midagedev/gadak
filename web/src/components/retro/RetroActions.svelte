<script lang="ts">
  /*
   * Decided last time (GDK-1453).
   *
   * A retro that never reads its previous one is a meeting rather than a
   * loop, and until now nothing on this screen closed that loop: the
   * decisions lived in whatever the team wrote down, and the numbers they
   * were about lived here. The label `retro-action` is the join — an issue
   * carrying it is a decision, and a first line reading `metric: wip age
   * max` names the row it was meant to move.
   *
   * Then and now, side by side, and nothing else. No verdict, no arrow
   * coloured green: whether a number that did not move means the decision
   * was wrong or that nobody did it is the conversation, and a view that
   * scores it has ended the conversation instead of starting it (THEORY.md
   * G9: progress, not score).
   */
  import { t } from '../../lib/i18n'
  import { METRIC_SPECS, formatValue } from './metrics'
  import type { RetroAction } from '../../lib/types'

  let {
    actions,
    labels,
    onOpen,
  }: {
    actions: RetroAction[]
    /** The table's own row labels, so a decision names its metric the way
     *  the row above it does. */
    labels: Record<string, string>
    onOpen: (keys: string[]) => void
  } = $props()

  /** The metric's unit, so "then 12.4 → now 9.1" prints in days or counts
   *  rather than as bare floats. Unknown names fall back to a count. */
  function unitOf(metric: string | undefined) {
    return METRIC_SPECS.find((m) => m.key === metric)?.unit ?? 'count'
  }
  function thenNow(a: RetroAction): string | null {
    if (!a.metric || a.then == null || a.now == null) return null
    const u = unitOf(a.metric)
    return t('retro.actions.thenNow', { then: formatValue(a.then, u), now: formatValue(a.now, u) })
  }
</script>

<ul class="max-w-[720px]" data-testid="retro-actions">
  {#each actions as a (a.key)}
    {@const step = thenNow(a)}
    <li class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 py-0.5 text-micro leading-snug" data-testid="retro-action" data-key={a.key}>
      <button
        type="button"
        class="rounded px-0.5 tabular-nums text-text-secondary underline decoration-border-strong decoration-dotted underline-offset-4 hover:bg-bg-hover hover:decoration-accent"
        onclick={() => onOpen([a.key])}>{a.key}</button
      >
      <span class="text-text-primary">{a.summary}</span>
      {#if a.metric}
        <span class="text-text-muted">{labels[a.metric] ?? a.metric}</span>
      {/if}
      {#if step}
        <span class="tabular-nums text-text-muted" data-testid="retro-action-step">{step}</span>
      {/if}
      <!--
        Only the finished ones say so. The three status categories do not
        have three words on this screen and inventing them would put a badge
        column on a list of sentences; a decision that is still open is the
        default state of a decision and needs no label.
      -->
      {#if a.status_category === 'done'}
        <span class="text-status-done" data-testid="retro-action-done">{t('retro.closed')}</span>
      {/if}
    </li>
  {/each}
</ul>
