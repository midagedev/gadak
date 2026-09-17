<script lang="ts">
  import type { Snippet } from 'svelte'

  // The one screen frame: owns the safe-top inset and the scroll region.
  // Screens author content into an already-inset frame, so a touch target
  // under the status bar cannot be written (DESIGN.md §4.1).
  //
  // `scroller` is the scroll region, lent out (GDK-902): the list has to
  // save and restore its position across a palette open-and-cancel, and the
  // element that scrolls is this component's, not the caller's. Bindable
  // and defaulted, so every screen that does not care is unchanged.
  let {
    header,
    children,
    footer,
    scroller = $bindable(null),
  }: {
    header?: Snippet
    children: Snippet
    footer?: Snippet
    scroller?: HTMLElement | null
  } = $props()
</script>

<div class="screen">
  {#if header}
    <header class="safe-top">
      {@render header()}
    </header>
  {/if}
  <main bind:this={scroller}>
    {@render children()}
  </main>
  {#if footer}
    {@render footer()}
  {/if}
</div>

<style>
  .screen {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
    background: var(--color-bg-base);
  }
  header {
    flex: none;
    padding-left: 16px;
    padding-right: 16px;
  }
  main {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    -webkit-overflow-scrolling: touch;
    /* GDK-1971: the one scroller every screen uses pays the keyboard band
       as scrollable bottom padding — with the keys up, the band (bottom
       ~300px of the viewport) is not part of the page, so without this the
       last rows of any list are unreachable: the palette's tail, the last
       comments. scroll-padding-bottom makes a scrollIntoView target land
       above the band too. 0px with the keys down: no change (the rule
       carried no bottom padding before this). */
    padding-bottom: var(--keyboard-inset);
    scroll-padding-bottom: var(--keyboard-inset);
  }
</style>
