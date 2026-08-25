<script lang="ts">
  /*
   * Central list screen ([explore]) — SearchBox + FilterBar + DisplayMenu + list.
   *  Server body-search hits merge above the local list as a "N body matches"
   *  section (plan §5.2).
   */
  import { t, formatNumber, relativeTime, absTime } from '../../lib/i18n'
  import { highlightSegments } from '../../lib/format'
  import Marks from '../ui/Marks.svelte'
  import { matchEvidence } from '../../lib/search-match'
  import { untrack } from 'svelte'
  import { applyServerSearchOutcome } from '../../lib/server-search'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { pages } from '../../stores/pages.svelte'
  import { bulk } from '../../stores/bulk.svelte'
  import SearchBox from './SearchBox.svelte'
  import FilterBar from './FilterBar.svelte'
  import DisplayMenu from './DisplayMenu.svelte'
  import ColumnsMenu from './ColumnsMenu.svelte'
  import BulkBar from './BulkBar.svelte'
  import BreakdownBar from './BreakdownBar.svelte'
  import IssueList from './IssueList.svelte'
  import IssueRow from './IssueRow.svelte'
  import MatchLine from './MatchLine.svelte'
  import SearchSection from './SearchSection.svelte'
  import EmptyState from './EmptyState.svelte'
  import Onboarding from '../shell/Onboarding.svelte'
  import FreshnessChip from '../shell/FreshnessChip.svelte'
  import { isDesktop } from '../../lib/config'
  import { onboarding } from '../../stores/onboarding.svelte'
  import { runSyncNow } from '../../lib/sync-now'

  let { onOpenSettings }: { onOpenSettings?: () => void } = $props()
  /** Desktop app: this toolbar is the main-column window-drag strip. */
  const desktop = isDesktop()

  const visibleCount = $derived(filters.visibleIssues.length)
  /** Body/comment hits, minus any row that could not say why it is here. A
   *  group that lists a key and shows no reason is worse than a shorter group:
   *  it asks the reader to trust a claim it does not back. */
  const extra = $derived(
    filters.serverExtraIssues.filter(
      (i) => matchEvidence(filters.matchReason(i.issue_key), i.summary, filters.filters.q) !== null,
    ),
  )
  /** Wiki pages hit by the same server search — issues alone would miss them.
   *  Same rule: a page stays only while it can show its reason. */
  const docHits = $derived(
    pages.searchHits.filter(
      (p) => matchEvidence(filters.matchReason(p.key), p.title, filters.serverMatchQuery) !== null,
    ),
  )
  /** Same title highlight the issue rows use, against the query that was sent. */
  const titleSegs = $derived((title: string) =>
    highlightSegments(title, filters.serverMatchQuery),
  )

  const needsOnboarding = $derived(onboarding.needsOnboarding)

  // Drop selections that left the visible list when the view (filter/sort/group)
  // changes. React only to viewKey (untrack visibleIssues so data deltas don't rerun).
  $effect(() => {
    void filters.viewKey
    if (!bulk.active) return
    const keys = untrack(() => filters.visibleIssues.map((i) => i.issue_key))
    bulk.retain(keys)
  })

  function onListError(error: unknown): void {
    console.error('[list] render failed', error)
  }

  function retryListLoad(reset: () => void): void {
    void issues.refresh().finally(reset)
  }
</script>

<div class="flex h-full flex-col">
  <!-- Toolbar. In the desktop app this band is the main-column drag handle;
       each interactive cluster opts out so a click still reaches the control.
       Hidden while onboarding: search / chips / sort are query chrome that
       undercut the wizard (GDK-299 F6). -->
  {#if !needsOnboarding}
  <div
    class="flex-none border-b border-border-strong/70 bg-bg-panel/35 px-4 py-3"
    class:desktop-drag-region={desktop}
    data-testid="list-toolbar"
  >
    <!-- flex-wrap + a content floor on the search slot: with min-w-0 the row
         squeezed SearchBox below its fixed innards at the docked floor
         (GDK-201, 1120px) and glyphs painted over each other. Menus wrap
         under the field before anything overlaps. -->
    <div class="mb-2.5 flex flex-wrap items-center gap-2.5">
      {#if desktop}
        <div class="desktop-no-drag min-w-[260px] flex-1"><SearchBox /></div>
        <div class="desktop-no-drag"><ColumnsMenu /></div>
        <div class="desktop-no-drag"><DisplayMenu /></div>
      {:else}
        <div class="min-w-[260px] flex-1"><SearchBox /></div>
        <ColumnsMenu />
        <DisplayMenu />
      {/if}
    </div>
    <div class="flex items-center gap-2.5">
      {#if desktop}
        <div class="desktop-no-drag min-w-0 flex-1"><FilterBar /></div>
      {:else}
        <div class="min-w-0 flex-1"><FilterBar /></div>
      {/if}
      <FreshnessChip />
      <span data-testid="list-count" class="flex-none text-micro text-text-muted">
        {t('list.countIssues', { n: formatNumber(visibleCount) })}
      </span>
    </div>
  </div>
  {/if}

  <!-- Bulk bar (only when something is selected) -->
  <BulkBar />

  {#if !needsOnboarding}
    <BreakdownBar />
  {/if}

  <!-- Document hits from the same server search. Above the issue sections: a
       page is the answer people came for when the issue list comes back thin. -->
  {#if filters.serverMatchQuery && docHits.length}
    <SearchSection
      testid="search-docs"
      count={docHits.length}
      label={t('list.docMatchCount', {
        n: formatNumber(docHits.length),
        q: filters.serverMatchQuery,
      })}
    >
      {#each docHits as p (p.key)}
        {@const evidence = matchEvidence(
          filters.matchReason(p.key),
          p.title,
          filters.serverMatchQuery,
        )}
        <button
          type="button"
          class="flex min-h-9 w-full flex-col justify-center gap-0.5 px-4 py-1.5 text-left text-body transition-colors {pages.selectedKey ===
          p.key
            ? 'bg-bg-active text-text-primary'
            : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'}"
          data-testid="search-doc-row"
          onclick={() => pages.select(p.key)}
        >
          <span class="flex w-full min-w-0 items-center gap-2">
            <span
              class="flex-none rounded bg-accent-subtle/60 px-1.5 py-0.5 font-mono text-micro font-medium text-accent-text"
            >
              {p.space_key}
            </span>
            <span class="min-w-0 flex-1 truncate"
              ><Marks segs={titleSegs(p.title)} /></span
            >
            <span class="flex-none text-micro text-text-muted" title={absTime(p.updated_at)}>
              {relativeTime(p.updated_at, 'long')}
            </span>
          </span>
          <!-- A page can match on its body, and often does — the title alone
               leaves "why is this here?" unanswered. -->
          {#if evidence && evidence !== 'title'}
            <MatchLine match={evidence} q={filters.serverMatchQuery} />
          {/if}
        </button>
      {/each}
    </SearchSection>
  {/if}

  <!-- Body-search results (matches not already in the local list) -->
  {#if filters.serverMatchQuery && extra.length}
    <SearchSection
      count={extra.length}
      label={t('list.bodyMatchCount', {
        n: formatNumber(extra.length),
        q: filters.serverMatchQuery,
      })}
    >
      {#each extra.slice(0, 50) as issue (issue.issue_key)}
        <IssueRow
          {issue}
          active={selection.selectedKey === issue.issue_key}
          match={filters.matchReason(issue.issue_key)}
        />
      {/each}
    </SearchSection>
  {/if}

  <!-- List / empty state -->
  <div class="min-h-0 flex-1">
    {#if visibleCount === 0 || needsOnboarding}
      {#if needsOnboarding}
        <Onboarding onOpenSettings={() => onOpenSettings?.()} />
      {:else if issues.pool.size === 0}
        <EmptyState
          icon="inbox"
          title={t('list.emptyTitle')}
          hint={`${t('list.emptyHint')} ${t('list.emptySyncHint')}`}
          actionLabel={t('list.emptyRunSync')}
          onAction={() => void runSyncNow('full')}
        />
      {:else if filters.searchQuery}
        <EmptyState
          icon="warning"
          title={t('list.searchFailed')}
          actionLabel={t('list.searchRetry')}
          onAction={() => void filters.runServerSearch().then(applyServerSearchOutcome)}
        />
      {:else if filters.serverMatchQuery && extra.length}
        <EmptyState
          icon="file"
          title={t('list.bodyOnlyTitle')}
          hint={t('list.bodyOnlyHint')}
        />
      {:else if filters.serverMatchQuery && docHits.length}
        <!-- The query only lives in the wiki (docs group above). Saying so beats
             "no issues match", which reads as "nothing found". -->
        <EmptyState icon="file" title={t('list.docOnlyTitle')} hint={t('list.docOnlyHint')} />
      {:else}
        {@const q = filters.filters.q.trim()}
        {@const bodyEmpty = Boolean(filters.serverMatchQuery) && !filters.searching}
        <EmptyState
          icon="search-x"
          title={t('list.noMatchTitle')}
          hint={q
            ? bodyEmpty
              ? t('list.noMatchBodyHint')
              : t('list.noMatchQueryHint')
            : t('list.noMatchHint')}
          actionLabel={q ? t('list.clearSearch') : filters.hasUserChips ? t('filter.clear') : ''}
          onAction={() => {
            if (q) {
              filters.setQuery('')
              filters.clearServerSearch()
            } else {
              filters.clearUserFilters()
            }
          }}
        />
      {/if}
    {:else}
      <svelte:boundary onerror={onListError}>
        {#snippet failed(_error, reset)}
          <EmptyState
            icon="warning"
            title={t('list.renderFailedTitle')}
            actionLabel={t('list.renderFailedRetry')}
            onAction={() => retryListLoad(reset)}
          />
        {/snippet}
        <IssueList />
      </svelte:boundary>
    {/if}
  </div>
</div>
