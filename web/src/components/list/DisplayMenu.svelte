<script lang="ts">
  /*
   * 정렬 메뉴 ([explore]). 브레이크다운은 BreakdownBar 에서 직접 바꾼다.
   */
  import { filters } from '../../stores/filters.svelte'
  import type { SortKey } from '../../lib/view-config'
  const BASE_SORTS: { k: SortKey; l: string }[] = [
    { k: 'updated', l: '갱신순' },
    { k: 'created', l: '생성순' },
    { k: 'priority', l: '우선순위' },
    { k: 'reopen_count', l: '재오픈수' },
  ]
  const RELEVANCE = { k: 'relevance' as SortKey, l: '관련도' }

  // 검색 중(또는 관련도가 활성)일 때만 관련도 옵션을 노출. 자동 승격 상태가 셀렉트에 보이게.
  const sorts = $derived(
    filters.filters.q.trim() || filters.effectiveSort === 'relevance'
      ? [RELEVANCE, ...BASE_SORTS]
      : BASE_SORTS,
  )

  let open = $state(false)
  let rootEl = $state<HTMLDivElement | null>(null)

  function onDocClick(e: MouseEvent) {
    if (rootEl && !rootEl.contains(e.target as Node)) open = false
  }
</script>

<svelte:document onclick={onDocClick} />

<div bind:this={rootEl} class="relative">
  <button
    type="button"
    class="inline-flex items-center gap-1.5 rounded-md border border-border-strong/70 bg-bg-elevated px-2.5 py-1.5 text-[12px] text-text-secondary transition-colors hover:border-border-strong hover:text-text-primary"
    onclick={() => (open = !open)}
    title="정렬 옵션"
  >
    <span>정렬</span>
    <span class="text-text-muted">
      {[RELEVANCE, ...BASE_SORTS].find((s) => s.k === filters.effectiveSort)?.l}
    </span>
  </button>

  {#if open}
    <div
      class="anim-enter absolute right-0 top-full z-30 mt-1 w-56 rounded-lg border border-border-strong bg-bg-elevated p-2 shadow-xl shadow-black/40"
    >
      <div class="mb-1 text-[11px] font-medium text-text-muted">정렬</div>
      <div class="flex flex-wrap gap-1">
        {#each sorts as s (s.k)}
          <button
            type="button"
            class="rounded px-2 py-0.5 text-[12px] transition-colors {filters.effectiveSort === s.k
              ? 'bg-accent text-white'
              : 'bg-bg-base text-text-secondary hover:bg-bg-hover'}"
            onclick={() => filters.setSort(s.k)}
          >
            {s.l}
          </button>
        {/each}
        <button
          type="button"
          class="ml-auto rounded px-2 py-0.5 text-[12px] text-text-secondary transition-colors hover:bg-bg-hover"
          onclick={() => filters.toggleDir()}
          title="정렬 방향"
        >
          {filters.display.dir === 'desc' ? '↓ 내림' : '↑ 오름'}
        </button>
      </div>
    </div>
  {/if}
</div>
