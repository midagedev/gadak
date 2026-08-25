<script lang="ts">
  // First-sync loading state: paper rectangles in the row rhythm — the
  // shape of the ledger before the ink arrives. No spinners (DESIGN.md §3.5).
  let { rows = 8 }: { rows?: number } = $props()
</script>

{#each Array(rows) as _, i (i)}
  <div class="skeleton-row" aria-hidden="true">
    <span class="bar w1" style:--d="{i * 60}ms"></span>
    <span class="bar w2" style:--d="{i * 60}ms"></span>
  </div>
{/each}

<style>
  .skeleton-row {
    min-height: var(--spacing-row);
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 6px;
    padding: 8px 16px;
    border-bottom: 1px solid var(--color-border-subtle);
  }
  .bar {
    height: 12px;
    border-radius: 4px;
    background: var(--color-bg-elevated);
    animation: breathe 1.4s ease-in-out infinite;
    animation-delay: var(--d);
  }
  .w1 {
    width: 72%;
  }
  .w2 {
    width: 34%;
    height: 9px;
  }
  @keyframes breathe {
    50% {
      opacity: 0.55;
    }
  }
</style>
