<script lang="ts">
  /*
   * Center main-column frame ([foundation], flex-1).
   * Top toolbar (search/filter/display) + issue list are filled by [explore].
   */
  import type { Snippet } from 'svelte'

  let { children }: { children?: Snippet } = $props()
</script>

<main class="issue-main-column flex min-w-0 flex-1 flex-col bg-bg-base">
  {@render children?.()}
</main>
