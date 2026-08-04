<script lang="ts">
  /*
   * 후속 웨이브가 채울 자리를 명시하는 개발용 플레이스홀더.
   * [explore]/[detail]/[personal] 이 실제 컴포넌트로 교체한다.
   */
  let { label, hint = '' }: { label: string; hint?: string } = $props()
</script>

<div
  class="flex h-full min-h-24 flex-col items-center justify-center gap-1 rounded-md border border-dashed border-border-subtle p-6 text-center"
>
  <span class="text-[12px] font-medium text-text-secondary">{label}</span>
  {#if hint}
    <span class="text-[11px] text-text-muted">{hint}</span>
  {/if}
</div>
