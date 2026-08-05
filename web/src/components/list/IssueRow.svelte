<script lang="ts">
  /*
   * Issue row ([explore]). Fixed 42px (virtual scroll assumption).
   *  Layout: priority icon · status dot · key (mono) · title · label chips (≤3+n)
   *  · reopen/stale badges · assignee · relative time.
   *  Chip/dot/avatar click = add that value as a filter (stopPropagation vs row select).
   */
  import { t } from '../../lib/i18n'
  import type { IssueLite } from '../../lib/types'
  import { filters } from '../../stores/filters.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { bulk } from '../../stores/bulk.svelte'
  import { me } from '../../stores/me.svelte'
  import { categoryOf, CATEGORY_META, relativeTime, absTime, highlightSegments } from '../../lib/format'
  import { isStale, statusAgeHours } from '../../lib/view-config'
  import PriorityIcon from './PriorityIcon.svelte'
  import Avatar from './Avatar.svelte'
  import { prefetchDetail } from '../detail/cache.svelte'

  let {
    issue,
    active = false,
    cursor = false,
  }: { issue: IssueLite; active?: boolean; cursor?: boolean } = $props()

  const cat = $derived(categoryOf(issue))
  const catMeta = $derived(CATEGORY_META[cat])
  const isFavorite = $derived(me.favorites.has(issue.issue_key))
  const isWatching = $derived(me.watches.has(issue.issue_key))
  // Stale (time in current status). Badge is day-based — floor at 1 so sub-day reads "day 1".
  const stale = $derived(isStale(issue))
  const staleDays = $derived(Math.max(1, Math.round(statusAgeHours(issue) / 24)))
  const shownLabels = $derived(issue.labels.slice(0, 3))
  const extraLabels = $derived(Math.max(0, issue.labels.length - 3))
  // Visible column set (view settings). O(1) check per trailing field.
  const cols = $derived(new Set(filters.display.columns))
  // Query highlight (title·key). Empty q → single segment, same render cost.
  const summarySegs = $derived(highlightSegments(issue.summary, filters.filters.q))
  const keySegs = $derived(highlightSegments(issue.issue_key, filters.filters.q))
  const qaImpactMeta = $derived.by(() => {
    switch (issue.qa_impact_state) {
      case 'blocking':
        return { label: t('list.qaBlock'), cls: 'bg-status-reopen/15 text-status-reopen' }
      case 'retest':
        return { label: t('list.qaRetest'), cls: 'bg-status-stale/15 text-status-stale' }
      case 'verified':
        return { label: t('list.qaDone'), cls: 'bg-status-done/15 text-status-done' }
      case 'linked':
        return { label: t('list.qaRun'), cls: 'bg-accent-subtle/60 text-accent-text' }
      default:
        return null
    }
  })
  // Deploy-stage badge. QA scan target is qa (swap done) → strong teal; waiting/dev/prod
  //  stay muted. none/merged = no badge (noise control).
  const deployState = $derived(issue.deploy_status?.state ?? 'none')
  const deployMeta = $derived.by(() => {
    switch (deployState) {
      case 'qa':
        // Swap done = QA can verify — teal dot + label so it pops in the list
        return { label: 'QA', cls: 'bg-[#2dd4bf]/15 text-[#5eead4]', dot: true }
      case 'qa_preview':
        return { label: t('list.qaPending'), cls: 'bg-[#2dd4bf]/8 text-[#2dd4bf]/70', dot: false }
      case 'dev':
        return { label: 'dev', cls: 'bg-bg-active text-text-muted', dot: false }
      case 'prod':
        return { label: 'prod', cls: 'bg-accent-subtle/50 text-accent-text', dot: false }
      default:
        return null
    }
  })
  // Done but not in any release (stuck at merged) → subtle warning tone.
  const deployStale = $derived(cat === 'done' && deployState === 'merged')

  // Own-issue highlight. Off when the view is already scoped to "my issues".
  const mine = $derived(filters.isMine(issue) && !filters.scopedToMe)

  // Recency: updates within 24h pull the time label up to accent.
  const isFresh = $derived.by(() => {
    if (!issue.updated_at) return false
    const t = Date.parse(issue.updated_at)
    return Number.isFinite(t) && Date.now() - t < 24 * 60 * 60 * 1000
  })

  function stop(fn: () => void) {
    return (e: MouseEvent) => {
      e.stopPropagation()
      fn()
    }
  }

  // ── Multi-select ──
  const selected = $derived(bulk.has(issue.issue_key))
  // Checkbox: dim by default (clear on hover); always clear when selected or in select mode.
  const boxOpacity = $derived(
    selected || bulk.count > 0 ? 'opacity-100' : 'opacity-40 group-hover:opacity-100',
  )

  // Checkbox hit = toggle select (separate from opening the row). Shift = range.
  function onCheckClick(e: MouseEvent) {
    e.stopPropagation()
    if (e.shiftKey) bulk.selectRange(issue.issue_key)
    else bulk.toggle(issue.issue_key)
  }

  // Row click: shift = range, select mode (≥1) = toggle, else open detail.
  function onRowClick(e: MouseEvent) {
    if (e.shiftKey) {
      e.preventDefault()
      bulk.selectRange(issue.issue_key)
      return
    }
    if (bulk.count > 0) {
      bulk.toggle(issue.issue_key)
      return
    }
    selection.toggle(issue.issue_key)
  }
</script>

<div
  class="group flex h-row cursor-pointer select-none items-center gap-2.5 border-b border-border-subtle/70 px-4 text-body transition-colors
    {selected
      ? 'bg-accent-subtle/30'
      : active
        ? 'bg-accent-subtle/20 shadow-[inset_3px_0_0_var(--color-accent)]'
        : cursor
          ? 'bg-bg-hover shadow-[inset_0_0_0_1px_var(--color-accent)]'
          : mine
            ? 'bg-accent-subtle/10 hover:bg-bg-hover'
            : 'hover:bg-bg-hover'}"
  role="button"
  tabindex="-1"
  aria-current={active ? 'true' : undefined}
  onclick={onRowClick}
  onmouseenter={() => prefetchDetail(issue.issue_key)}
  onkeydown={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      selection.toggle(issue.issue_key)
    }
  }}
>
  <!-- Multi-select checkbox (hit target only; shift=range) -->
  <button
    type="button"
    class="flex h-4 w-4 flex-none items-center justify-center rounded border transition-all {boxOpacity}
      {selected ? 'border-accent bg-accent text-white' : 'border-border-strong'}"
    onclick={onCheckClick}
    aria-pressed={selected}
    aria-label={selected ? t('list.deselect') : t('list.select')}
    title={t('list.select')}
  >
    {#if selected}<span class="text-[9px]">✓</span>{/if}
  </button>

  <!-- Priority -->
  <PriorityIcon priority={issue.priority} />

  <!-- Status dot (click = category filter) -->
  <button
    type="button"
    class="h-2 w-2 flex-none rounded-full transition-transform hover:scale-125"
    style:background={catMeta.color}
    title={t('list.categoryTitle', { label: catMeta.label, status: issue.status })}
    onclick={stop(() => filters.addValue('status_category', cat))}
    aria-label={t('list.categoryFilter', { label: catMeta.label })}
  ></button>

  <!-- Key (accent tone marks own issues) -->
  <span class="w-[88px] flex-none truncate font-mono text-[12px] {mine ? 'text-accent-text' : 'text-text-secondary'}">
    {#each keySegs as seg, i (i)}{#if seg.hit}<mark class="rounded-[2px] bg-status-stale/30 text-inherit">{seg.text}</mark>{:else}{seg.text}{/if}{/each}
  </span>

  <!-- Personal markers (favorite/watch) — quiet, before title -->
  {#if isFavorite || isWatching}
    <span class="flex flex-none items-center gap-0.5 text-[10px]" aria-hidden="true">
      {#if isFavorite}<span class="text-status-stale" title={t('common.favorite')}>★</span>{/if}
      {#if isWatching}<span class="text-accent-text" title={t('common.watching')}>👁</span>{/if}
    </span>
  {/if}

  <!-- Title -->
  <span class="min-w-0 flex-1 truncate font-medium text-text-primary" title={issue.summary}>
    {#each summarySegs as seg, i (i)}{#if seg.hit}<mark class="rounded-[2px] bg-status-stale/30 text-inherit">{seg.text}</mark>{:else}{seg.text}{/if}{/each}
    {#if filters.filters.reopened && issue.reopen_count > 0 && issue.reopen_reason}
      <!-- Inline reason only on reopen view — elsewhere 🔁 badge+tooltip is enough -->
      <span class="ml-1 text-[11px] text-status-reopen/80" title={issue.reopen_reason}>
        · {issue.reopen_reason}
      </span>
    {/if}
  </span>

  <!-- Badges: reopen / stale -->
  {#if cols.has('reopen') && issue.reopen_count > 0}
    <button
      type="button"
      class="flex-none rounded bg-status-reopen/15 px-1.5 py-0.5 text-[10px] font-medium text-status-reopen transition-colors hover:bg-status-reopen/25"
      title={issue.reopen_reason ? t('list.reopenCountReason', { n: issue.reopen_count, reason: issue.reopen_reason }) : t('list.reopenCount', { n: issue.reopen_count })}
      onclick={stop(() => filters.toggleFlag('reopened'))}
    >
      🔁 {issue.reopen_count}
    </button>
  {/if}
  {#if cols.has('stale') && stale}
    <span
      class="flex-none rounded bg-status-stale/15 px-1.5 py-0.5 text-[10px] font-medium text-status-stale"
      title={t('list.staleDays', { n: staleDays })}
    >
      {t('list.staleDaysShort', { n: staleDays })}
    </span>
  {/if}

  {#if cols.has('qa_impact') && qaImpactMeta}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] font-medium transition-opacity hover:opacity-80 xl:inline-flex {qaImpactMeta.cls}"
      title={issue.qa_runs?.map((run) => run.label).join(', ') || issue.qa_impact_label}
      onclick={stop(() => filters.addValue('qa_impact', issue.qa_impact_state))}
    >
      {qaImpactMeta.label}
    </button>
  {/if}

  <!-- Deploy-stage badge (qa=teal emphasis / others muted) -->
  {#if cols.has('deploy') && deployMeta}
    <button
      type="button"
      class="flex flex-none items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium transition-opacity hover:opacity-80 {deployMeta.cls}"
      title={deployState === 'qa'
        ? t('deploy.qaSwapDone')
        : t('deploy.stageTitle', { label: deployMeta.label })}
      onclick={stop(() => filters.addValue('deploy_state', deployState))}
    >
      {#if deployMeta.dot}
        <span class="h-1.5 w-1.5 flex-none rounded-full bg-[#2dd4bf]"></span>
      {/if}
      {deployMeta.label}
    </button>
  {:else if cols.has('deploy') && deployStale}
    <span
      class="flex-none rounded bg-status-stale/12 px-1.5 py-0.5 text-[10px] font-medium text-status-stale/80"
      title={t('deploy.resolvedNoRelease')}
    >
      {t('deploy.notDeployed')}
    </span>
  {/if}

  <!-- Optional columns (view settings) — only when valued; most add a filter on click -->
  {#if cols.has('severity') && issue.severity}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary sm:inline-flex"
      title={t('list.fieldValue', { field: t('common.severity'), value: issue.severity })}
      onclick={stop(() => filters.addValue('severity', issue.severity!))}
    >
      {issue.severity}
    </button>
  {/if}
  {#if cols.has('issue_type') && issue.issue_type}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary sm:inline-flex"
      title={t('list.fieldValue', { field: t('common.type'), value: issue.issue_type })}
      onclick={stop(() => filters.addValue('issue_type', issue.issue_type))}
    >
      {issue.issue_type}
    </button>
  {/if}
  {#if cols.has('status') && issue.status}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary sm:inline-flex"
      title={t('list.fieldValue', { field: t('common.status'), value: issue.status })}
      onclick={stop(() => filters.addValue('status', issue.status))}
    >
      {issue.status}
    </button>
  {/if}
  {#if cols.has('dev_test_result') && issue.development_test_result}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('column.dev_test_result'), value: issue.development_test_result })}
      onclick={stop(() => filters.addValue('development_test_result', issue.development_test_result!))}
    >
      {issue.development_test_result}
    </button>
  {/if}
  {#if cols.has('environment') && issue.environment}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('column.environment'), value: issue.environment })}
      onclick={stop(() => filters.addValue('environment', issue.environment!))}
    >
      {issue.environment}
    </button>
  {/if}
  {#if cols.has('team_group') && issue.team_group}
    <button
      type="button"
      class="hidden flex-none rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('column.team_group'), value: issue.team_group })}
      onclick={stop(() => filters.addValue('team_group', issue.team_group!))}
    >
      {issue.team_group}
    </button>
  {/if}
  {#if cols.has('reporter') && issue.reporter}
    <button
      type="button"
      class="hidden max-w-[90px] flex-none truncate rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary md:inline-flex"
      title={t('list.fieldValue', { field: t('common.reporter'), value: issue.reporter })}
      onclick={issue.reporter_email
        ? stop(() => filters.addValue('reporter_email', issue.reporter_email!))
        : undefined}
    >
      {issue.reporter}
    </button>
  {/if}
  {#if cols.has('comment_count') && issue.comment_count > 0}
    <span class="hidden flex-none text-[10px] text-text-muted sm:inline" title={t('list.commentCount', { n: issue.comment_count })}>
      💬 {issue.comment_count}
    </span>
  {/if}
  {#if cols.has('fix_versions') && issue.fix_versions.length}
    <span class="hidden flex-none items-center gap-1 lg:flex">
      <button
        type="button"
        class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
        title={`Fix Version: ${issue.fix_versions.join(', ')}`}
        onclick={stop(() => filters.addValue('fix_versions', issue.fix_versions[0]))}
      >
        {issue.fix_versions[0]}
      </button>
      {#if issue.fix_versions.length > 1}
        <span class="text-[10px] text-text-muted">+{issue.fix_versions.length - 1}</span>
      {/if}
    </span>
  {/if}
  {#if cols.has('components') && issue.components.length}
    <span class="hidden flex-none items-center gap-1 lg:flex">
      <button
        type="button"
        class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-[10px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
        title={t('list.fieldValue', { field: t('field.components'), value: issue.components.join(', ') })}
        onclick={stop(() => filters.addValue('components', issue.components[0]))}
      >
        {issue.components[0]}
      </button>
      {#if issue.components.length > 1}
        <span class="text-[10px] text-text-muted">+{issue.components.length - 1}</span>
      {/if}
    </span>
  {/if}
  {#if cols.has('created') && issue.created_at}
    <span class="hidden w-10 flex-none text-right text-[11px] text-text-muted sm:inline" title={t('list.createdAt', { time: absTime(issue.created_at) })}>
      {relativeTime(issue.created_at)}
    </span>
  {/if}

  <!-- Label chips -->
  {#if cols.has('labels') && shownLabels.length}
    <span class="hidden flex-none items-center gap-1 md:flex">
      {#each shownLabels as label (label)}
        <button
          type="button"
          class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-[11px] text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          onclick={stop(() => filters.addValue('labels', label))}
          title={t('list.fieldValue', { field: t('common.labels'), value: label })}
        >
          {label}
        </button>
      {/each}
      {#if extraLabels}
        <span class="text-[11px] text-text-muted">+{extraLabels}</span>
      {/if}
    </span>
  {/if}

  <!-- Assignee -->
  {#if cols.has('assignee')}
    <Avatar
      email={issue.assignee_email}
      name={issue.assignee}
      onclick={issue.assignee_email
        ? stop(() => filters.addValue('assignee_email', issue.assignee_email!))
        : undefined}
    />
  {/if}

  <!-- Relative updated time. Accent within 24h for recency. -->
  {#if cols.has('updated')}
    <span
      class="w-10 flex-none text-right text-[11px] {isFresh
        ? 'font-medium text-accent-text'
        : 'text-text-muted'}"
      title={absTime(issue.updated_at)}
    >
      {relativeTime(issue.updated_at)}
    </span>
  {/if}
</div>
