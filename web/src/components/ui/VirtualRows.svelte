<script lang="ts" generics="T">
  /*
   * Windowed list over rows of known height.
   *
   * The issue list has always done this inline, with its own sticky-header and
   * cursor logic wound through it. The document lists had nothing: they rendered
   * every row, so opening the view built one DOM subtree per document. Measured
   * on a 10,000-page fixture, that was 90,013 nodes and a 4.4s freeze in the
   * desktop app's WebKit (1.5s in Chromium) — paid again on every tab switch,
   * because the list is rebuilt from scratch each time.
   *
   * Heights are per row rather than uniform: a document row is 42px, or 59px
   * when it carries a body line, and whether it does depends on that document
   * having a body at all. So offsets are a prefix sum and the window is found by
   * binary search — O(log n) per scroll, O(n) arithmetic when the list changes,
   * which is the part that was never the problem.
   */
  import type { Snippet } from 'svelte'
  import { rowOffsets, rowWindow } from '../../lib/row-metrics'

  let {
    rows,
    height,
    key,
    overscan = 6,
    scrollerClass = 'min-h-0 flex-1 overflow-y-auto',
    innerClass = '',
    testid,
    row,
  }: {
    rows: T[]
    /** Rendered height of one row, in px. Must match what `row` actually paints. */
    height: (item: T) => number
    /** Stable identity, so scrolling moves rows instead of rebuilding them. */
    key: (item: T, index: number) => string
    overscan?: number
    scrollerClass?: string
    /** Applied to the translated slice wrapper (padding a caller wants inside). */
    innerClass?: string
    testid?: string
    row: Snippet<[T, number]>
  } = $props()

  let scroller = $state<HTMLDivElement | null>(null)
  let scrollTop = $state(0)
  let viewportH = $state(0)

  // Offset sheet: offsets[i] is row i's top, offsets[n] the total height.
  // The formulas live in row-metrics (GDK-842 extracted them from this
  // component; GDK-850 makes this the consumer, not a second copy) so every
  // virtualized surface agrees by construction. The height prop's reads are
  // tracked signals — a caller that snapshots rowMetrics() in $state
  // re-runs these deriveds when user dimension overrides land at runtime.
  const offsets = $derived(rowOffsets(rows.map(height)))
  const total = $derived(rows.length ? offsets[rows.length] : 0)

  const win = $derived(rowWindow(offsets, scrollTop, viewportH, overscan))
  const slice = $derived(rows.slice(win.start, win.end))
  const offsetTop = $derived(offsets[win.start] ?? 0)

  /**
   * Put a row at the top of the viewport. Replaces scrollIntoView, which needs
   * the element to exist — with a window, the row being scrolled to is usually
   * the one row that has not been rendered yet.
   */
  export function scrollToIndex(index: number): void {
    if (!scroller || index < 0 || index >= rows.length) return
    scroller.scrollTop = offsets[index]
  }
</script>

<div
  bind:this={scroller}
  bind:clientHeight={viewportH}
  class={scrollerClass}
  data-testid={testid}
  onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}
>
  <!-- Full height so the scrollbar reflects the whole list, not the window. -->
  <div style="height: {total}px" class="relative">
    <div style="transform: translateY({offsetTop}px)" class={innerClass}>
      {#each slice as item, i (key(item, win.start + i))}
        {@render row(item, win.start + i)}
      {/each}
    </div>
  </div>
</div>
