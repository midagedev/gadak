<script lang="ts">
  /*
   * Cold-start (no cache) loading shell. Visually continues index.html's inline
   * boot shell so skeleton → real UI has no white flash.
   */
  import { isDesktop } from '../../lib/config'

  const navWidths = ['70%', '55%', '62%', '48%']
  const rows = Array.from({ length: 8 })
  /** Desktop app: clear the window controls, which start at y=0 here too. */
  const wordmarkInset = isDesktop() ? '78px' : null // 90px, less the aside's p-3
</script>

<div class="issue-shell">
  <div class="issue-layout">
    <aside class="issue-sidebar flex-none border-r border-border-subtle bg-bg-panel p-3">
      <div
        class="mb-5 h-4 w-20 animate-pulse rounded bg-bg-elevated"
        style:margin-left={wordmarkInset}
      ></div>
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
