<script module lang="ts">
  import type { IssueLite } from '../../lib/types'

  /*
   * The row a column header is drawn from (GDK-1087). The header is this same
   * component in `header` mode, so that alignment cannot drift: the slot
   * widths, the container-query folds and the shrink behaviour are the row's
   * own markup, not a second copy of it. Nothing here is displayed — every
   * value branch is replaced by the column's label — so the object exists only
   * to keep the deriveds total. Frozen: one instance for the app's lifetime,
   * and an accidental write is a throw instead of a header that drifts.
   */
  export const HEADER_ROW_ISSUE: IssueLite = Object.freeze({
    issue_key: '',
    summary: '',
    status: '',
    status_category: 'new',
    issue_type: '',
    priority: null,
    priority_rank: null,
    severity: null,
    assignee: null,
    assignee_email: null,
    reporter: null,
    reporter_email: null,
    labels: [],
    fix_versions: [],
    components: [],
    team_group: null,
    epic_key: null,
    parent_key: null,
    source_project: null,
    created_at: null,
    updated_at: null,
    resolved_at: null,
    status_changed_at: null,
    reopen_count: 0,
    reopened_at: null,
    reopen_reason: null,
    comment_count: 0,
    dev_project_number: null,
    related_project_number: null,
    environment: null,
    browser: null,
    found_version: null,
    occurrence: null,
    solution: null,
    critical_phenomenon: null,
    development_area: null,
    cs: null,
    development_test_assignee: null,
    development_test_assignee_email: null,
    development_test_result: null,
    qa_impact_state: '',
    qa_impact_label: '',
    qa_runs: [],
    qa_suites: [],
  })
</script>

<script lang="ts">
  /*
   * Issue row ([explore]). Fixed 42px (virtual scroll assumption).
   *  Layout: priority icon · status dot · key (mono) · title · trailing scan
   *  columns (fixed-width slots). Off columns are omitted — no hole. An on
   *  column keeps its width when the row has no value, so the same field
   *  shares an x. A hide drops the whole slot, never a squeeze: every
   *  optional column (qa_impact, epic and the twelve catalog options
   *  alike) breaks on the ROW's own width (@container issuerow, GDK-1046
   *  and GDK-1049) — a detail panel narrows the row for reasons the
   *  viewport cannot see, and the width that has to fit is the one that
   *  decides. Each slot's threshold lives in one table in app.css (the
   *  trail-break-* rungs). Title takes leftover and truncates.
   *  Chip/dot/avatar click = add that value as a filter (stopPropagation vs row select).
   *  Tint and cursor ring are separate: checking a row must not hide where the
   *  keyboard is, or the next x lands blind.
   *
   *  A `match` grows the row by exactly one line — 59px, the same height a
   *  document row takes with its excerpt, so the two lists stay one density.
   *  Only the server-search section passes it; the virtual list never does, so
   *  its uniform 42px assumption still holds. The line is dropped only when the
   *  highlighted summary already says why the row is here — see matchEvidence.
   */
  import { onMount } from 'svelte'
  import { t, columnLabel, type ColumnLabelKey } from '../../lib/i18n'
  import type { SearchMatch } from '../../lib/types'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { bulk } from '../../stores/bulk.svelte'
  import { watches } from '../../stores/watches.svelte'
  import { favorites } from '../../stores/favorites.svelte'
  import { categoryOf, categoryMetaOf, relativeTime, absTime, highlightSegments } from '../../lib/format'
  import { calendarDay } from '../../lib/calendar'
  import Marks from '../ui/Marks.svelte'
  import { matchEvidence } from '../../lib/search-match'
  import { config } from '../../lib/config'
  import { labelChipTint, typeChipTint } from '../../stores/ui-tokens.svelte'
  import { isStale, statusAgeHours } from '../../lib/view-config'
  import PriorityIcon from './PriorityIcon.svelte'
  import Avatar from './Avatar.svelte'
  import Icon from '../ui/Icon.svelte'
  import MatchLine from './MatchLine.svelte'
  import { prefetchDetail } from '../../lib/detail-cache.svelte'

  let {
    issue,
    active = false,
    cursor = false,
    /** Why the server search returned this row (body/comment hit). Absent in the
     *  main list, where the highlighted title already says why. */
    match = null,
    /** Draw this row as the list's column header (GDK-1087): the same slots,
     *  the same folds, the column's label instead of the row's value. */
    header = false,
  }: {
    issue: IssueLite
    active?: boolean
    cursor?: boolean
    match?: SearchMatch | null
    header?: boolean
  } = $props()

  const cat = $derived(categoryOf(issue))
  const catMeta = $derived(categoryMetaOf(cat))
  const isFavorite = $derived(favorites.keys.has(issue.issue_key))
  const isWatching = $derived(watches.keys.has(issue.issue_key))
  // Stale (time in current status). Badge is day-based — floor at 1 so sub-day reads "day 1".
  // Weight follows magnitude (multiples of the configured threshold). A single
  // maximum-emphasis chip on every stale row warns about nothing.
  const stale = $derived(isStale(issue))
  const staleDays = $derived(Math.max(1, Math.round(statusAgeHours(issue) / 24)))
  const staleBand = $derived.by((): 'quiet' | 'mid' | 'loud' | null => {
    if (!stale) return null
    const threshold = config().staleThresholdHours
    if (!(threshold > 0)) return 'loud'
    const ratio = statusAgeHours(issue) / threshold
    if (ratio <= 2) return 'quiet'
    if (ratio <= 4) return 'mid'
    return 'loud'
  })
  // Fill is ground-neutral (bg-elevated), not stale-amber — an amber wash
  // mixed to olive on dark/ink and read as a stain. Amber stays on the
  // glyph, the day count, and a hairline. Band weight is fill/border
  // opacity, not a louder wash.
  const staleBandClass = $derived.by(() => {
    switch (staleBand) {
      case 'quiet':
        return 'text-text-muted'
      case 'mid':
        return 'border border-status-stale/20 bg-bg-elevated/50 text-status-stale/80'
      case 'loud':
        return 'border border-status-stale/40 bg-bg-elevated font-medium text-status-stale'
      default:
        return ''
    }
  })
  const shownLabels = $derived(issue.labels.slice(0, 3))
  const extraLabels = $derived(Math.max(0, issue.labels.length - 3))
  // A counter renders below iff either markup guard is true (n-extra:
  // extraLabels; n-rest: length > 1 — n-all is display:none everywhere).
  // .chipfold-counted lets the CSS reserve the counter's width only on
  // rows that actually have one: a one-label row has nothing to make
  // room for, and its chip keeps the full slot (GDK-1050).
  const hasCountedLabels = $derived(extraLabels > 0 || issue.labels.length > 1)
  // Every count in the strip points at the same list, so a folded chip is still
  // one hover away from naming itself.
  const allLabelsTitle = $derived(
    t('list.fieldValue', { field: t('common.labels'), value: issue.labels.join(', ') }),
  )
  // Visible column set (view settings). O(1) check per trailing field.
  const cols = $derived(filters.columnSet)
  // Query highlight (title·key). Empty q → single segment, same render cost.
  const summarySegs = $derived(highlightSegments(issue.summary, filters.filters.q))
  const keySegs = $derived(highlightSegments(issue.issue_key, filters.filters.q))
  // The snippet line to draw, if any. 'title' means the summary above already
  // carries the highlight, so a second line would only repeat it.
  const evidence = $derived(matchEvidence(match, issue.summary, filters.filters.q))
  const matchLine = $derived(evidence === 'title' || evidence === null ? null : evidence)
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
        return { label: t('deploy.qa'), cls: 'bg-[#2dd4bf]/15 text-[#5eead4]', dot: true }
      case 'qa_preview':
        return { label: t('list.qaPending'), cls: 'bg-[#2dd4bf]/8 text-[#2dd4bf]/70', dot: false }
      case 'dev':
        return { label: t('deploy.dev'), cls: 'bg-bg-active text-text-muted', dot: false }
      case 'prod':
        return { label: t('deploy.prod'), cls: 'bg-accent-subtle/50 text-accent-text', dot: false }
      default:
        return null
    }
  })
  // Done but not in any release (stuck at merged) → subtle warning tone.
  const deployStale = $derived(cat === 'done' && deployState === 'merged')

  // Own-issue highlight. Off when the view is already scoped to "my issues".
  const mine = $derived(filters.isMine(issue) && !filters.scopedToMe)

  // Epic chip. Redundant while the list is already sectioned by epic, so it is
  // dropped there rather than repeating the group header on every row.
  const epicKey = $derived(filters.display.group_by === 'epic' ? null : issue.epic_key)
  const epicSummary = $derived(epicKey ? issues.get(epicKey)?.summary : undefined)

  // Recency: updates within 24h pull the time label up to accent. Date.now()
  // is not a dependency a $derived can see, so without a tick the accent
  // freezes at mount — a list left open overnight keeps yesterday's "fresh".
  // The question is already answered repo-wide (FreshnessChip): a tick state
  // the derivation re-reads. Cadence: this value can only flip once per 24h,
  // so a minute is already tighter than the chip's 10s-on-minute-text
  // standard, and the virtual list mounts only its slice, so ticks scale
  // with what is on screen, not with the rows behind the window.
  let tick = $state(0)
  onMount(() => {
    const id = setInterval(() => (tick += 1), 60_000)
    return () => clearInterval(id)
  })
  const isFresh = $derived.by(() => {
    void tick // re-read the wall clock every tick
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

{#snippet head(col: ColumnLabelKey | 'epic')}
  <!-- A header cell. The slot is the row's, so the label lives inside the same
       width the value does — narrow icon columns therefore truncate, and the
       full name is the tooltip. A label that overflowed its slot would be the
       one thing this row must never do. -->
  {@const label = col === 'epic' ? t('common.epic') : columnLabel(col)}
  <span class="block min-w-0 flex-1 truncate text-left text-micro font-medium text-text-muted" title={label}>{label}</span>
{/snippet}

<div
  class="chipfold-host group flex select-none flex-col justify-center gap-0.5 border-b text-body
    {header
      ? 'h-8 border-border-subtle'
      : 'cursor-pointer border-border-subtle/70 transition-colors'}
    {header ? '' : matchLine ? 'h-row-excerpt' : 'h-row'}
    {header
      ? ''
      : selected
        ? 'bg-accent-subtle/30'
        : active
          ? 'bg-accent-subtle/20'
          : cursor
            ? 'bg-bg-hover'
            : mine
              ? 'bg-accent-subtle/10 hover:bg-bg-hover'
              : 'hover:bg-bg-hover'}
    {header
      ? ''
      : cursor
        ? 'shadow-[inset_0_0_0_1px_var(--color-accent)]'
        : active
          ? 'shadow-[inset_3px_0_0_var(--color-accent)]'
          : ''}"
  role={header ? undefined : 'button'}
  tabindex="-1"
  aria-current={active ? 'true' : undefined}
  data-issue-key={header ? undefined : issue.issue_key}
  data-testid={header ? 'issue-column-header' : undefined}
  data-cursor={cursor ? 'true' : undefined}
  onclick={header ? undefined : onRowClick}
  onmouseenter={header ? undefined : () => prefetchDetail(issue.issue_key)}
  onkeydown={header
    ? undefined
    : (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          selection.toggle(issue.issue_key)
        }
      }}
>
  <div class="flex w-full min-w-0 items-center gap-2.5 px-4">
  <!-- Leading strip + title. In header mode these are the row's own nodes made
       invisible rather than replaced by spacers: `display:contents` generates
       no box, so the flex line is character-for-character the row's, and
       `visibility:hidden` inherits through it. A spacer would be a second
       owner of 196px — and of the lead-fold-1 that takes 34 of them away at
       ≤480 (GDK-1087). -->
  <div class="contents" class:invisible={header}>
  <!-- Multi-select checkbox (hit target only; shift=range).
       lead-fold-1: the leading strip folds too on a very narrow row
       (GDK-1089). Bulk-selecting rows you cannot read is not a workflow,
       and this is the cheapest 26px on the row. -->
  <button
    type="button"
    class="lead-fold-1 flex h-4 w-4 flex-none items-center justify-center rounded border transition-all {boxOpacity}
      {selected ? 'border-accent bg-accent text-white' : 'border-border-strong'}"
    onclick={onCheckClick}
    aria-pressed={selected}
    aria-label={selected ? t('common.deselect') : t('list.select')}
    title={t('list.select')}
  >
    {#if selected}<Icon name="check" size={11} />{/if}
  </button>

  <!-- Priority -->
  <PriorityIcon priority={issue.priority} rank={issue.priority_rank} />

  <!-- Status dot (click = category filter). Folds with the checkbox: the
       category is a filter shortcut, and the same eight pixels of title
       carry more at that width (GDK-1089). -->
  <button
    type="button"
    class="lead-fold-1 h-2 w-2 flex-none rounded-full transition-transform hover:scale-125"
    style:background={catMeta.color}
    title={t('list.categoryTitle', { label: catMeta.label, status: issue.status })}
    onclick={stop(() => filters.addValue('status_category', cat))}
    aria-label={t('list.categoryFilter', { label: catMeta.label })}
  ></button>

  <!-- Key (accent tone marks own issues) -->
  <span class="w-[88px] flex-none truncate font-mono text-micro {mine ? 'text-accent-text' : 'text-text-secondary'}">
    <Marks segs={keySegs} />
  </span>

  <!-- Personal markers (favorite/watch) — quiet, before title -->
  {#if isFavorite || isWatching}
    <span class="flex flex-none items-center gap-1" aria-hidden="true">
      {#if isFavorite}
        <Icon name="star" size={12} filled class="text-status-stale" title={t('common.favorite')} />
      {/if}
      {#if isWatching}
        <Icon name="eye" size={12} class="text-accent-text" title={t('common.watching')} />
      {/if}
    </span>
  {/if}

  <!-- Title.
       `row-title` is the row's one non-negotiable: the title takes whatever
       the trailing strip leaves, and with the detail panel open that came to a
       character and an ellipsis while the label chips beside it kept every pixel
       they asked for — the widest thing on the row surrendering to the narrowest
       (vision verdict 2026-08-07). The floor is a CSS minimum, never a measured
       one: this row is the 10k-row hot path and must not read layout. It is
       13ch, and a share of the row below 620px — app.css owns both, because
       row-column-thresholds.ts models the same number. -->
  <span class="row-title flex-1 truncate font-normal text-text-primary" title={issue.summary}>
    <Marks segs={summarySegs} />
    {#if filters.filters.reopened && issue.reopen_count > 0 && issue.reopen_reason}
      <!-- Inline reason only on reopen view — elsewhere the badge + its tooltip is enough -->
      <span class="ml-1 text-micro text-status-reopen/80" title={issue.reopen_reason}>
        · {issue.reopen_reason}
      </span>
    {/if}
  </span>
  </div>

  <!-- Trailing scan columns. Fixed-width flex slots, not inline flow: an on
       column occupies its width even when this row has no value, so the field
       shares an x. An off column is not rendered (no hole). Break classes
       live on the slot so a hide drops the column instead of leaving a gap.
       Container-query folds (trail-fold-*) hide whole slots in designed order
       when the row cannot fit them, and every optional column's break
       (trail-break-*) is row-width-keyed for the same reason: the row, not
       the viewport, is what runs out of room (GDK-1046; GDK-1049 moved the
       twelve catalog options here too, thresholds in one app.css table).
       overflow:hidden on the list column is not a fold (GDK-766). Keep
       last: stale, then reopen, then deploy. -->
  <div class="flex min-w-0 shrink items-center gap-2.5" data-testid="issue-row-trail">
  <!-- Badges: reopen / stale -->
  {#if cols.has('reopen')}
    <div class="trail-fold-3 flex w-9 flex-none items-center justify-start overflow-hidden" data-col="reopen">
      {#if header}{@render head('reopen')}{:else if issue.reopen_count > 0}
        <button
          type="button"
          class="flex flex-none items-center gap-1 rounded bg-status-reopen/15 px-1.5 py-0.5 text-micro font-medium text-status-reopen transition-colors hover:bg-status-reopen/25"
          title={issue.reopen_reason ? t('list.reopenCountReason', { n: issue.reopen_count, reason: issue.reopen_reason }) : t('list.reopenCount', { n: issue.reopen_count })}
          onclick={stop(() => filters.toggleFlag('reopened'))}
        >
          <!-- currentColor keeps the glyph inside the badge's red: reopen count is a
               semantic signal, and the icon must not read cooler than the number. -->
          <Icon name="rotate-ccw" size={11} />
          {issue.reopen_count}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('stale')}
    <!-- 56px with the glyph (English 48, Korean ~51). At issuerow ≤1100 the
         glyph hides and the slot is 44px — enough for three-digit "35일"
         (~38px without the glyph). Do not put a Tailwind display utility on
         .stale-glyph: components-layer display:none lost to `flex` (GDK-766). -->
    <div class="stale-slot flex flex-none items-center justify-start overflow-hidden" data-col="stale">
      {#if header}{@render head('stale')}{:else if stale}
        <span
          class="flex flex-none items-center gap-1 rounded px-1.5 py-0.5 text-micro {staleBandClass}"
          data-stale-band={staleBand}
          title={t('list.staleDays', { n: staleDays })}
        >
          <!-- The mark the string used to carry as an emoji. Same treatment as the
               reopen badge beside it: currentColor, so it stays inside the badge's
               amber instead of arriving in whatever colors the platform's ⏳ ships.
               Parent is already flex; this wrapper must not be. -->
          <span class="stale-glyph"><Icon name="hourglass" size={11} /></span>
          {t('list.staleDaysShort', { n: staleDays })}
        </span>
      {/if}
    </div>
  {/if}

  {#if cols.has('qa_impact')}
    <div class="trail-break-qa flex w-24 flex-none items-center overflow-hidden" data-col="qa_impact">
      {#if header}{@render head('qa_impact')}{:else if qaImpactMeta}
        <button
          type="button"
          class="inline-flex max-w-full truncate rounded px-1.5 py-0.5 text-micro font-medium transition-opacity hover:opacity-80 {qaImpactMeta.cls}"
          title={issue.qa_runs?.map((run) => run.label).join(', ') || issue.qa_impact_label}
          onclick={stop(() => filters.addValue('qa_impact', issue.qa_impact_state))}
        >
          {qaImpactMeta.label}
        </button>
      {/if}
    </div>
  {/if}

  <!-- Deploy-stage badge (qa=teal emphasis / others muted) -->
  {#if cols.has('deploy')}
    <div class="trail-fold-3 flex w-10 flex-none items-center overflow-hidden" data-col="deploy">
      {#if header}{@render head('deploy')}{:else if deployMeta}
        <button
          type="button"
          class="flex min-w-0 items-center gap-1 truncate rounded px-1.5 py-0.5 text-micro font-medium transition-opacity hover:opacity-80 {deployMeta.cls}"
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
      {:else if deployStale}
        <span
          class="max-w-full truncate rounded bg-status-stale/12 px-1.5 py-0.5 text-micro font-medium text-status-stale/80"
          title={t('deploy.resolvedNoRelease')}
        >
          {t('deploy.notDeployed')}
        </span>
      {/if}
    </div>
  {/if}

  <!-- Optional columns (view settings). Slot is reserved when the column is
       on; empty rows keep the width so later fields do not shift. -->
  {#if cols.has('severity')}
    <div class="trail-break-severity flex w-20 flex-none items-center overflow-hidden" data-col="severity">
      {#if header}{@render head('severity')}{:else if issue.severity}
        <button
          type="button"
          class="max-w-full truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          title={t('list.fieldValue', { field: t('common.severity'), value: issue.severity })}
          onclick={stop(() => filters.addValue('severity', issue.severity!))}
        >
          {issue.severity}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('issue_type')}
    <div class="trail-break-issue-type flex w-20 flex-none items-center overflow-hidden" data-col="issue_type">
      {#if header}{@render head('issue_type')}{:else if issue.issue_type}
        <button
          type="button"
          class="max-w-full truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          style:background={typeChipTint(issue.issue_type_id)}
          title={t('list.fieldValue', { field: t('common.type'), value: issue.issue_type })}
          onclick={stop(() => filters.addValue('issue_type', issue.issue_type_id || issue.issue_type))}
        >
          {issue.issue_type}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('status')}
    <div class="trail-break-status flex w-20 flex-none items-center overflow-hidden" data-col="status">
      {#if header}{@render head('status')}{:else if issue.status}
        <button
          type="button"
          class="max-w-full truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          title={t('list.fieldValue', { field: t('common.status'), value: issue.status })}
          onclick={stop(() => filters.addValue('status', issue.status_id || issue.status))}
        >
          {issue.status}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('dev_test_result')}
    <div class="trail-break-dev-test-result flex w-24 flex-none items-center overflow-hidden" data-col="dev_test_result">
      {#if header}{@render head('dev_test_result')}{:else if issue.development_test_result}
        <button
          type="button"
          class="max-w-full truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          title={t('list.fieldValue', { field: t('column.dev_test_result'), value: issue.development_test_result })}
          onclick={stop(() => filters.addFieldValue('development_test_result', issue.development_test_result!))}
        >
          {issue.development_test_result}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('environment')}
    <div class="trail-break-environment flex w-20 flex-none items-center overflow-hidden" data-col="environment">
      {#if header}{@render head('environment')}{:else if issue.environment}
        <button
          type="button"
          class="max-w-full truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          title={t('list.fieldValue', { field: t('column.environment'), value: issue.environment })}
          onclick={stop(() => filters.addFieldValue('environment', issue.environment!))}
        >
          {issue.environment}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('team_group')}
    <div class="trail-break-team-group flex w-20 flex-none items-center overflow-hidden" data-col="team_group">
      {#if header}{@render head('team_group')}{:else if issue.team_group}
        <button
          type="button"
          class="max-w-full truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          title={t('list.fieldValue', { field: t('column.team_group'), value: issue.team_group })}
          onclick={stop(() => filters.addValue('team_group', issue.team_group!))}
        >
          {issue.team_group}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('reporter')}
    <div class="trail-break-reporter flex w-[90px] flex-none items-center overflow-hidden" data-col="reporter">
      {#if header}{@render head('reporter')}{:else if issue.reporter}
        <button
          type="button"
          class="max-w-full truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
          title={t('list.fieldValue', { field: t('common.reporter'), value: issue.reporter })}
          onclick={issue.reporter_id || issue.reporter_email
            ? stop(() => filters.addValue('reporter_email', issue.reporter_id ?? issue.reporter_email!))
            : undefined}
        >
          {issue.reporter}
        </button>
      {/if}
    </div>
  {/if}
  {#if cols.has('comment_count')}
    <div class="trail-break-comment-count flex w-10 flex-none items-center overflow-hidden" data-col="comment_count">
      {#if header}{@render head('comment_count')}{:else if issue.comment_count > 0}
        <span class="flex items-center gap-1 text-micro text-text-muted" title={t('list.commentCount', { n: issue.comment_count })}>
          <Icon name="message-square" size={11} />
          {issue.comment_count}
        </span>
      {/if}
    </div>
  {/if}
  {#if cols.has('fix_versions')}
    <div class="trail-break-fix-versions flex w-[110px] flex-none items-center overflow-hidden" data-col="fix_versions">
      {#if header}{@render head('fix_versions')}{:else if issue.fix_versions.length}
        <span class="flex min-w-0 items-center gap-1">
          <button
            type="button"
            class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
            title={`${t('column.fix_versions')}: ${issue.fix_versions.join(', ')}`}
            onclick={stop(() => filters.addValue('fix_versions', issue.fix_versions[0]))}
          >
            {issue.fix_versions[0]}
          </button>
          {#if issue.fix_versions.length > 1}
            <span class="text-micro text-text-muted">+{issue.fix_versions.length - 1}</span>
          {/if}
        </span>
      {/if}
    </div>
  {/if}
  {#if cols.has('components')}
    <div class="trail-break-components flex w-[110px] flex-none items-center overflow-hidden" data-col="components">
      {#if header}{@render head('components')}{:else if issue.components.length}
        <span class="flex min-w-0 items-center gap-1">
          <button
            type="button"
            class="max-w-[110px] truncate rounded px-1.5 py-0.5 text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-text-secondary"
            title={t('list.fieldValue', { field: t('field.components'), value: issue.components.join(', ') })}
            onclick={stop(() => filters.addValue('components', issue.components[0]))}
          >
            {issue.components[0]}
          </button>
          {#if issue.components.length > 1}
            <span class="text-micro text-text-muted">+{issue.components.length - 1}</span>
          {/if}
        </span>
      {/if}
    </div>
  {/if}
  {#if cols.has('created')}
    <div class="trail-break-created flex w-10 flex-none items-center overflow-hidden" data-col="created">
      {#if header}{@render head('created')}{:else if issue.created_at}
        <span class="w-10 text-right text-micro text-text-muted" title={t('list.createdAt', { time: absTime(issue.created_at) })}>
          {relativeTime(issue.created_at)}
        </span>
      {/if}
    </div>
  {/if}
  {#if cols.has('due')}
    <div class="trail-break-due flex w-20 flex-none items-center overflow-hidden" data-col="due">
      {#if header}{@render head('due')}{:else if issue.duedate}
        <span
          class="w-20 text-right text-micro text-text-muted"
          title={t('list.dueAt', { date: absTime(issue.duedate) })}
          data-calendar-kind="date"
          data-calendar-day={calendarDay(issue.duedate, 'date')}
        >
          {calendarDay(issue.duedate, 'date')}
        </span>
      {/if}
    </div>
  {/if}

  <!-- Epic chip. It lives in the trailing strip, not beside the key: an optional
       element before the title would move every summary's left edge by its width,
       and the title column is what the eye scans down. Slot is reserved whenever
       the list is not already sectioned by epic, so later columns stay put. -->
  {#if filters.display.group_by !== 'epic'}
    <div class="trail-break-epic flex w-16 flex-none items-center overflow-hidden" data-col="epic">
      {#if header}{@render head('epic')}{:else if epicKey}
        <button
          type="button"
          class="max-w-[84px] truncate rounded px-1.5 py-0.5 font-mono text-micro text-text-muted transition-colors hover:bg-bg-elevated hover:text-accent-text"
          data-testid="epic-chip"
          title={t('list.fieldValue', { field: t('common.epic'), value: epicSummary ?? epicKey })}
          onclick={stop(() => selection.select(epicKey))}
        >
          {epicKey}
        </button>
      {/if}
    </div>
  {/if}

  <!-- Label chips. Still the one shrinkable slot: a fixed basis so the column
       shares an x, and shrink so the title's 13ch floor can claim space back.
       Chips fold via .chipfold-* (row + slot containers); they are never ground
       down. `min-w-[3rem]` is the last line of that rule — except where a +N
       counter shares the slot: there the chip's floor and max step down to
       reserve the counter's width, because a clipped count is unreadable
       while a truncated label is still a route (GDK-1050). -->
  {#if cols.has('labels')}
    <span
      class="trail-fold-2 chipfold-labels {hasCountedLabels ? 'chipfold-counted' : ''} flex shrink items-center gap-1 overflow-hidden"
      data-col="labels"
    >
      {#if header}{@render head('labels')}{:else if shownLabels.length}
        {#each shownLabels as label, i (label)}
          <button
            type="button"
            class="min-w-[3rem] max-w-[110px] truncate rounded bg-bg-elevated px-1.5 py-0.5 text-micro text-text-secondary transition-colors hover:bg-bg-hover hover:text-text-primary
              {i === 0 ? 'chipfold-first' : 'chipfold-rest'}"
            style:background={labelChipTint(label)}
            onclick={stop(() => filters.addValue('labels', label))}
            title={t('list.fieldValue', { field: t('common.labels'), value: label })}
          >
            {label}
          </button>
        {/each}
        <!-- The counters wear a faint chip tint (weaker than a hovered label, +4px
             width in total) because at the narrowest step no real chip is left to
             anchor them: a bare "+1" beside the epic key reads as the key column's
             continuation, not the label column's residue (vision verdict
             2026-08-07). -->
        {#if extraLabels}
          <span class="chipfold-n-extra flex-none rounded bg-bg-elevated/70 px-0.5 text-micro text-text-muted" title={allLabelsTitle}>+{extraLabels}</span>
        {/if}
        {#if issue.labels.length > 1}
          <span class="chipfold-n-rest flex-none rounded bg-bg-elevated/70 px-0.5 text-micro text-text-muted" title={allLabelsTitle}>+{issue.labels.length - 1}</span>
        {/if}
        <span class="chipfold-n-all flex-none rounded bg-bg-elevated/70 px-0.5 text-micro text-text-muted" title={allLabelsTitle}>+{issue.labels.length}</span>
      {/if}
    </span>
  {/if}

  <!-- Assignee -->
  {#if cols.has('assignee')}
    <div class="trail-fold-1 flex h-5 w-5 flex-none items-center justify-center overflow-hidden" data-col="assignee">
      {#if header}{@render head('assignee')}{:else}
      <Avatar
        email={issue.assignee_email}
        accountId={issue.assignee_id}
        name={issue.assignee}
        onclick={issue.assignee_id || issue.assignee_email
          ? stop(() => filters.addValue('assignee_email', issue.assignee_id ?? issue.assignee_email!))
          : undefined}
      />
      {/if}
    </div>
  {/if}

  <!-- Relative updated time. Freshness is weight, not accent: on a list sorted
       by recency almost every visible row is fresh, so accent here painted the
       whole column indigo (28 of 29 accent nodes on the default screen,
       measured 2026-08-06) and said nothing. Accent is reserved for what is
       yours or actionable — your issues, watches, links. -->
  {#if cols.has('updated')}
    <span
      class="trail-fold-1 w-10 flex-none overflow-hidden text-right text-micro {isFresh
        ? 'font-medium text-text-secondary'
        : 'text-text-muted'}"
      data-col="updated"
      title={header ? undefined : absTime(issue.updated_at)}
    >
      {#if header}{@render head('updated')}{:else}{relativeTime(issue.updated_at)}{/if}
    </span>
  {/if}
  </div>
  </div>

  <!-- The matched text, when the row came back from a server search for a
       reason the title does not show. -->
  {#if matchLine}
    <div class="w-full min-w-0 px-4">
      <MatchLine match={matchLine} q={filters.serverMatchQuery} />
    </div>
  {/if}
</div>
