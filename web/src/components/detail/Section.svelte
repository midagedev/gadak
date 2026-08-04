<script lang="ts">
  /*
   * 상세 패널 섹션 래퍼 ([detail]).
   * 보더 대신 여백 + 섹션 제목(옵션 카운트)로 구분한다(계약 §3).
   */
  import type { Snippet } from 'svelte'

  let {
    title,
    count = undefined,
    children,
  }: {
    title: string
    count?: number | undefined
    children?: Snippet
  } = $props()
</script>

<section class="px-5 py-4">
  <h3 class="mb-3 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wider text-text-muted">
    <span class="flex-none">{title}</span>
    {#if count !== undefined}
      <span class="flex-none text-[11px] tabular-nums text-text-muted">{count}</span>
    {/if}
    <span class="h-px flex-1 bg-border-subtle"></span>
  </h3>
  {@render children?.()}
</section>
