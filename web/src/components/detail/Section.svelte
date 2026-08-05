<script lang="ts">
  /*
   * Detail panel section wrapper ([detail]).
   * Separates with spacing + section title (optional count), not borders (contract §3).
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
