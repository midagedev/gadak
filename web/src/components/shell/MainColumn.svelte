<script lang="ts">
  /*
   * Center main-column frame ([foundation], flex-1).
   * Top toolbar (search/filter/display) + issue list are filled by [explore].
   */
  import type { Snippet } from 'svelte'

  let { children, inert = false }: { children?: Snippet; inert?: boolean } = $props()
</script>

<!-- `inert` while the detail panel covers the list as an overlay (GDK-201/
     GDK-1585): a prop from App, which owns the overlay verdict — not a
     DOM walk from outside. -->
<main
  class="issue-main-column flex min-w-0 flex-1 flex-col bg-bg-base"
  data-testid="main-column"
  {inert}
>
  {@render children?.()}
</main>
