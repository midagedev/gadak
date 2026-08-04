<script lang="ts">
  /*
   * cold start(캐시 없음) 로딩 셸. index.html 의 인라인 부트 셸과 시각적으로 이어져
   *  흰 플래시 없이 스켈레톤 → 실제 UI 로 전환된다.
   */
  const navWidths = ['70%', '55%', '62%', '48%']
  const rows = Array.from({ length: 8 })
</script>

<div class="issue-shell">
  <div class="issue-layout">
    <aside class="issue-sidebar flex-none border-r border-border-subtle bg-bg-panel p-3">
      <div class="mb-5 h-4 w-20 animate-pulse rounded bg-bg-elevated"></div>
      {#each navWidths as w (w)}
        <div class="my-2.5 h-3 animate-pulse rounded bg-bg-elevated" style:width={w}></div>
      {/each}
    </aside>
    <main class="flex flex-1 flex-col gap-2 p-4">
      <div class="h-6 w-2/5 animate-pulse rounded bg-bg-elevated"></div>
      {#each rows as _, i (i)}
        <div class="h-row animate-pulse rounded bg-bg-elevated"></div>
      {/each}
    </main>
  </div>
</div>
