<script lang="ts">
  /*
   * 인증 게이트 — 캐시가 없는데 401 이 난 경우에만 표시(render-before-auth).
   * 캐시가 있으면 앱은 그대로 뜨고 인증 확인은 백그라운드에서 조용히 처리된다.
   */
  let { onRetry }: { onRetry?: () => void } = $props()
</script>

<div class="flex h-screen flex-col items-center justify-center gap-4 bg-bg-base px-6 text-center">
  <div class="flex items-center gap-2">
    <span class="inline-block h-2.5 w-2.5 rounded-[3px] bg-accent"></span>
    <span class="text-[15px] font-semibold text-text-primary">Issues</span>
  </div>
  <p class="max-w-sm text-[13px] text-text-secondary">
    로컬 서버에 연결할 수 없습니다. <code class="text-text-primary">scry serve</code> 가 실행 중인지
    확인하세요.
  </p>
  <div class="flex gap-2">
    {#if onRetry}
      <button
        onclick={onRetry}
        class="rounded-md border border-border-strong px-3 py-1.5 text-[12px] font-medium text-text-secondary transition-colors hover:bg-bg-hover"
      >
        다시 시도
      </button>
    {/if}
  </div>
</div>
