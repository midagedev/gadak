<script lang="ts">
  /* Section the list by a chosen field and summarize top distribution in one line. */
  import { t } from '../../lib/i18n'
  import { filters } from '../../stores/filters.svelte'
  import { CATEGORY_META } from '../../lib/format'
  import { groupByEnabled, type GroupBy, type StatusCategory } from '../../lib/view-config'
  import Icon from '../ui/Icon.svelte'

  const ALL_OPTIONS: { key: GroupBy; label: string }[] = [
    { key: 'status_category', label: t('group.byStatusCategory') },
    { key: 'product', label: t('group.byProduct') },
    { key: 'team_group', label: t('group.byTeam') },
    { key: 'assignee', label: t('group.byAssignee') },
    { key: 'priority', label: t('group.byPriority') },
    { key: 'severity', label: t('group.bySeverity') },
    { key: 'issue_type', label: t('group.byType') },
    { key: 'development_test_result', label: t('group.byDevTestResult') },
    { key: 'qa_impact', label: t('group.byQaImpact') },
    { key: 'source_project', label: t('group.bySourceProject') },
    { key: 'epic', label: t('group.byEpic') },
    { key: 'status', label: t('group.byStatus') },
    { key: 'none', label: t('group.sectionNone') },
  ]
  // Disabled-feature axes (team/product/QA impact) are omitted from the options.
  const OPTIONS = ALL_OPTIONS.filter((o) => groupByEnabled(o.key))

  let open = $state(false)
  let rootEl = $state<HTMLDivElement | null>(null)

  const currentLabel = $derived(
    OPTIONS.find((option) => option.key === filters.display.group_by)?.label ?? t('group.byStatusCategory'),
  )
  const rankedGroups = $derived.by(() =>
    filters.display.group_by === 'none'
      ? []
      : filters.display.group_by === 'development_test_result' ||
          filters.display.group_by === 'qa_impact'
        ? filters.groups
        : [...filters.groups].sort((a, b) => b.counts.total - a.counts.total),
  )
  const shownGroups = $derived(rankedGroups.slice(0, 6))
  const hiddenGroupCount = $derived(Math.max(0, rankedGroups.length - shownGroups.length))

  function onDocClick(e: MouseEvent) {
    if (!rootEl) return
    if (!e.composedPath().includes(rootEl)) open = false
  }

  function select(key: GroupBy) {
    filters.setGroupBy(key)
    open = false
  }

  function groupColor(key: string, index: number): string {
    if (filters.display.group_by === 'status_category' && key in CATEGORY_META) {
      return CATEGORY_META[key as StatusCategory].color
    }
    if (filters.display.group_by === 'development_test_result') {
      if (key.toLowerCase() === 'fail') return 'var(--color-status-reopen)'
      if (key === 'none') return 'var(--color-status-stale)'
      if (key.toLowerCase() === 'pass') return 'var(--color-status-done)'
    }
    if (filters.display.group_by === 'qa_impact') {
      if (key === 'blocking') return 'var(--color-status-reopen)'
      if (key === 'retest') return 'var(--color-status-stale)'
      if (key === 'verified') return 'var(--color-status-done)'
      if (key === 'linked') return 'var(--color-accent)'
    }
    if (index === 0) return 'var(--color-accent)'
    if (index === 1) return 'var(--color-status-inprogress)'
    return 'var(--color-text-muted)'
  }
</script>

<svelte:document onclick={onDocClick} />

<div
  class="flex min-h-11 flex-none items-center gap-3 border-b border-border-subtle bg-bg-panel/55 px-4 py-1.5"
>
  <div bind:this={rootEl} class="relative flex-none">
    <button
      type="button"
      class="inline-flex h-control items-center gap-1.5 rounded-md border border-border-strong bg-bg-elevated px-2.5 text-[12px] font-medium text-text-secondary transition-colors hover:text-text-primary"
      onclick={() => (open = !open)}
      aria-expanded={open}
    >
      <span class="text-text-muted">{t('group.breakdown')}</span>
      <span class="text-text-primary">{currentLabel}</span>
      <Icon
        name="chevron-right"
        size={13}
        class="text-text-muted transition-transform duration-150 {open ? '-rotate-90' : 'rotate-90'}"
      />
    </button>

    {#if open}
      <div
        class="anim-enter absolute left-0 top-full z-30 mt-1 grid w-64 grid-cols-2 gap-1 rounded-lg border border-border-strong bg-bg-elevated p-1.5 shadow-overlay"
      >
        {#each OPTIONS as option (option.key)}
          <button
            type="button"
            class="flex min-h-control-sm items-center rounded px-2.5 py-1 text-left text-[12px] transition-colors {filters.display
              .group_by === option.key
              ? 'bg-accent text-white'
              : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
            onclick={() => select(option.key)}
          >
            {option.label}
          </button>
        {/each}
      </div>
    {/if}
  </div>

  {#if shownGroups.length > 0}
    <div class="h-5 w-px flex-none bg-border-strong/70"></div>
    <div class="min-w-0 flex-1 overflow-x-auto">
      <div class="flex w-max items-center gap-3 whitespace-nowrap text-[12px]">
        {#each shownGroups as group, i (group.key || `empty-${i}`)}
          <span class="inline-flex items-center gap-1.5 text-text-secondary">
            <span
              class="h-1.5 w-1.5 rounded-full"
              style:background={groupColor(group.key, i)}
            ></span>
            <span class="max-w-36 truncate">{group.label || t('common.all')}</span>
            <span class="font-mono text-[11px] text-text-muted">{group.counts.total}</span>
          </span>
        {/each}
        {#if hiddenGroupCount > 0}
          <span class="text-[11px] text-text-muted">{t('list.moreCount', { n: hiddenGroupCount })}</span>
        {/if}
      </div>
    </div>
  {/if}
</div>
