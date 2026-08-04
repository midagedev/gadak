<script lang="ts">
  /*
   * 그룹 헤더 ([explore]). 고정 높이 42px. 라벨 + 총건수 + 상태분류 미니 집계.
   *  리스트가 곧 요약 대시보드(plan §5.1) — 헤더에 집계를 내장한다.
   */
  import { t } from '../../lib/i18n'
  import type { IssueGroup } from '../../stores/filters.svelte'
  import { CATEGORY_META } from '../../lib/format'
  import type { StatusCategory } from '../../lib/view-config'

  let {
    group,
    floating = false,
    showCategoryCounts = true,
  }: { group: IssueGroup; floating?: boolean; showCategoryCounts?: boolean } = $props()

  const order: StatusCategory[] = ['new', 'inprogress', 'done']
</script>

<!--
  섹션 리듬: 헤더는 채워진 카드가 아니라 "라벨 줄"로 물러난다 — 마이크로캡 라벨 + 얇은 룰.
  가상 스크롤이 슬롯을 42px 로 고정하므로 높이는 유지하되, 라벨은 하단 정렬해 그룹 위 여백을 만든다.
  (스티키로 떠 있을 때만 배경/블러로 아래 행을 가린다.)
-->
<div
  class="flex h-row items-end gap-2 px-4 pb-1.5 text-[12px]
    {floating
      ? 'bg-bg-base/95 backdrop-blur border-b border-border-subtle'
      : ''}"
>
  <span class="truncate text-[11px] font-semibold uppercase tracking-wider text-text-muted">
    {group.label || t('common.all')}
  </span>
  <span class="flex-none text-[11px] tabular-nums text-text-muted">
    {group.counts.total}
  </span>
  <span class="h-px flex-1 self-center bg-border-subtle"></span>

  <!-- 상태분류 미니 집계 -->
  {#if showCategoryCounts}
    <span class="flex flex-none items-center gap-2">
      {#each order as c (c)}
        {#if group.counts.category[c] > 0}
          <span class="flex items-center gap-1 text-[11px] text-text-muted" title={CATEGORY_META[c].label}>
            <span class="h-1.5 w-1.5 rounded-full" style:background={CATEGORY_META[c].color}></span>
            {group.counts.category[c]}
          </span>
        {/if}
      {/each}
    </span>
  {/if}
</div>
