<script lang="ts">
  /*
   * Right detail panel frame ([foundation]).
   * Occupies width only when `open` (issue selected). Wide screens use a grid
   * track; narrow screens use an overlay.
   *
   * Selection contract: open state comes from [explore] selection store
   * (`selectedKey`) via App.svelte's `open` prop — this component only displays.
   */
  import type { Snippet } from 'svelte'

  let { open = false, children }: { open?: boolean; children?: Snippet } = $props()
</script>

<aside
  class="issue-detail-panel h-full overflow-hidden bg-bg-panel"
  class:is-open={open}
  class:border-l={open}
  class:border-border-subtle={open}
  aria-hidden={!open}
  data-testid="issue-detail-panel"
>
  <div class="h-full w-full min-w-0 overflow-y-auto">
    {@render children?.()}
  </div>
</aside>
