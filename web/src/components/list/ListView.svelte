<script lang="ts">
  /*
   * 중앙 리스트 화면 조립 ([explore]) — SearchBox + FilterBar + DisplayMenu + 리스트.
   *  본문 검색(서버) 결과는 로컬 리스트 위에 "본문 매칭 N건" 섹션으로 합쳐 보여준다(plan §5.2).
   */
  import { t, formatNumber } from '../../lib/i18n'
  import { untrack } from 'svelte'
  import { filters } from '../../stores/filters.svelte'
  import { issues } from '../../stores/issues.svelte'
  import { selection } from '../../stores/selection.svelte'
  import { bulk } from '../../stores/bulk.svelte'
  import SearchBox from './SearchBox.svelte'
  import FilterBar from './FilterBar.svelte'
  import DisplayMenu from './DisplayMenu.svelte'
  import ColumnsMenu from './ColumnsMenu.svelte'
  import BulkBar from './BulkBar.svelte'
  import BreakdownBar from './BreakdownBar.svelte'
  import IssueList from './IssueList.svelte'
  import IssueRow from './IssueRow.svelte'
  import EmptyState from './EmptyState.svelte'

  const visibleCount = $derived(filters.visibleIssues.length)
  const extra = $derived(filters.serverExtraIssues)

  // 뷰(필터/정렬/그룹)가 바뀌면 가시 리스트에서 빠진 선택은 정리한다.
  //  viewKey 에만 반응(데이터 delta 마다 재계산하지 않도록 visibleIssues 는 untrack).
  $effect(() => {
    void filters.viewKey
    if (!bulk.active) return
    const keys = untrack(() => filters.visibleIssues.map((i) => i.issue_key))
    bulk.retain(keys)
  })
</script>

<div class="flex h-full flex-col">
  <!-- 도구줄 -->
  <div class="flex-none border-b border-border-strong/70 bg-bg-panel/35 px-4 py-3">
    <div class="mb-2.5 flex items-center gap-2.5">
      <div class="min-w-0 flex-1"><SearchBox /></div>
      <ColumnsMenu />
      <DisplayMenu />
    </div>
    <div class="flex items-center gap-2.5">
      <div class="min-w-0 flex-1"><FilterBar /></div>
      <span class="flex-none text-[12px] text-text-muted">
        {t('list.countIssues', { n: formatNumber(visibleCount) })}
      </span>
    </div>
  </div>

  <!-- 일괄 작업 바(선택 시에만 표시) -->
  <BulkBar />

  <BreakdownBar />

  <!-- 본문 검색 결과(로컬에 없던 매칭) -->
  {#if filters.serverMatchQuery && extra.length}
    <div class="flex-none border-b border-border-subtle bg-bg-panel/40">
      <div class="px-3 py-1 text-[11px] font-medium text-accent-text">
        {t('list.bodyMatchCount', { n: formatNumber(extra.length), q: filters.serverMatchQuery })}
      </div>
      <div class="max-h-48 overflow-y-auto">
        {#each extra.slice(0, 50) as issue (issue.issue_key)}
          <IssueRow {issue} active={selection.selectedKey === issue.issue_key} />
        {/each}
      </div>
    </div>
  {/if}

  <!-- 리스트 / 빈 상태 -->
  <div class="min-h-0 flex-1">
    {#if visibleCount === 0}
      {#if issues.pool.size === 0}
        <EmptyState icon="📭" title={t('list.emptyTitle')} hint={t('list.emptyHint')} />
      {:else if filters.serverMatchQuery && extra.length}
        <EmptyState
          icon="📄"
          title={t('list.bodyOnlyTitle')}
          hint={t('list.bodyOnlyHint')}
        />
      {:else}
        <EmptyState
          icon="🔍"
          title={t('list.noMatchTitle')}
          hint={t('list.noMatchHint')}
          actionLabel={filters.hasFilters ? t('list.clearFilters') : ''}
          onAction={() => filters.clearAll()}
        />
      {/if}
    {:else}
      <IssueList />
    {/if}
  </div>
</div>
