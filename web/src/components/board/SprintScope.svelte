<script lang="ts">
  /*
   * The board's sprint scope (GDK-1656): Active sprint · Backlog · All.
   *
   * It is a filter, not board state — each segment sets `sprint_state` on
   * the view (`active`, `none`, or nothing), so the URL carries it, the
   * back button undoes it, a saved view keeps it, and the CLI's
   * `views open --jql 'sprint in openSprints()'` opens the same scope. The
   * control only appears when the workspace has a sprint at all; a kanban
   * team never sees it.
   *
   * Same segmented shape as the history view's kind filter (no new tokens).
   * The active segment names the sprint when there is exactly one active,
   * with its end date on hover — the one fact a scrum team looks for.
   */
  import { t, absTime } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { sprints } from '../../stores/sprints.svelte'

  type Scope = 'active' | 'backlog' | 'all'

  const current = $derived.by<Scope>(() => {
    const st = filters.filters.sprint_state
    if (st.length === 1 && st[0] === 'active') return 'active'
    if (st.length === 1 && st[0] === 'none') return 'backlog'
    return 'all'
  })

  const activeLabel = $derived.by(() => {
    const a = sprints.active
    return a.length === 1 ? a[0].name : t('board.scopeActive')
  })
  const activeTitle = $derived.by(() => {
    const a = sprints.active
    if (a.length !== 1) return t('board.scopeActive')
    const s = a[0]
    // Name, then when it ends, then what it is for. The goal has been in the
    // mirror since sprints became rows and no surface read it (GDK-1695);
    // this is the one place a person is already hovering to ask about the
    // sprint. Omitted when the sprint has none, rather than an empty line.
    const parts = [s.name]
    if (s.end_at) parts.push(t('board.scopeEnds', { date: absTime(s.end_at) }))
    if (s.goal) parts.push(s.goal)
    return parts.join(' · ')
  })

  function set(scope: Scope) {
    const want = scope === 'active' ? ['active'] : scope === 'backlog' ? ['none'] : []
    // One axis, replaced wholesale: the segments are exclusive by design.
    for (const v of [...filters.filters.sprint_state]) filters.removeValue('sprint_state', v)
    for (const v of want) filters.addValue('sprint_state', v)
  }

  const SEGMENTS: { key: Scope; label: () => string; title: () => string }[] = [
    { key: 'active', label: () => activeLabel, title: () => activeTitle },
    { key: 'backlog', label: () => t('board.scopeBacklog'), title: () => t('board.scopeBacklogHint') },
    { key: 'all', label: () => t('board.scopeAll'), title: () => t('board.scopeAll') },
  ]
</script>

{#if sprints.any}
  <div
    class="inline-flex h-control-sm items-center gap-0.5 rounded-md border border-border-subtle p-0.5 pl-2"
    role="group"
    aria-label={t('board.scopeLabel')}
    data-testid="sprint-scope"
  >
    <!--
      The axis, said out loud (GDK-1682). Every other control on this toolbar
      names its own — "Breakdown Progress", "+ Filter", the sort — and this one
      shipped as three bare words. It read as a sprint control only because the
      fixture's active sprint happens to be called "Sprint 42"; on a Linear
      origin the same row is "Cycle 1  Backlog  All", where nothing says sprint
      at all. `board.scopeLabel` existed already but reached the aria-label only.
      The word is "Scope", not "Sprint": these are three slices of the board and
      only one of them is a sprint, and an axis word that repeats the active
      sprint's name reads as a stutter ("Sprint Sprint 42" on the demo mirror).
    -->
    <span class="mr-1 select-none text-micro font-medium text-text-muted" aria-hidden="true"
      >{t('board.scopeAxis')}</span
    >
    {#each SEGMENTS as seg (seg.key)}
      <button
        type="button"
        class="flex h-full items-center rounded px-2 text-micro font-medium transition-colors {current ===
        seg.key
          ? 'bg-bg-active text-text-primary'
          : 'text-text-muted hover:text-text-secondary'}"
        aria-pressed={current === seg.key}
        data-testid="sprint-scope-{seg.key}"
        title={seg.title()}
        onclick={() => set(seg.key)}
      >
        <span class="max-w-[120px] truncate">{seg.label()}</span>
      </button>
    {/each}
  </div>
{/if}
