<script lang="ts">
  /*
   * One group of server-search hits above the local list (documents, body
   * matches). Both groups share this frame so the whole search region reads as
   * one surface: one left edge, one header rhythm, one scroll rule.
   *
   * The cap lands on a row boundary. A fixed max-height cuts whichever row
   * happens to straddle it — a line of text sliced through the middle, which
   * reads as a rendering fault rather than as "there is more below" (vision
   * verdict 2026-08-06: a row was cut at 27px while 300px of viewport sat
   * unused). Rows here are not one height (42px without a snippet line, 59px
   * with one, and document rows differ again), so the boundary cannot be
   * arithmetic on a constant — it is measured from the rows that rendered.
   */
  import type { Snippet } from 'svelte'

  let {
    label,
    testid,
    /** Row count — the cap is remeasured whenever the group's contents change. */
    count,
    children,
  }: {
    label: string
    testid?: string
    count: number
    children: Snippet
  } = $props()

  /** Share of the space below the group's own top that it may occupy — just
   *  under half, so the list underneath always keeps the larger share; it holds
   *  the title matches, which are usually the better hits. A second group sits
   *  lower, so its own budget shrinks without any bookkeeping between the two. */
  const MAX_FRACTION = 0.45

  let scroller = $state<HTMLElement | null>(null)
  let content = $state<HTMLElement | null>(null)
  let capPx = $state<number | null>(null)

  function snap() {
    const el = scroller
    if (!el) return
    const budget = (window.innerHeight - el.getBoundingClientRect().top) * MAX_FRACTION
    let acc = 0
    for (const row of Array.from(content?.children ?? [])) {
      const h = row.getBoundingClientRect().height
      // The first row always counts: a group that shows nothing is worse than
      // one that overruns its budget by a single line.
      if (acc > 0 && acc + h > budget) break
      acc += h
    }
    capPx = acc > 0 ? Math.round(acc) : null
  }

  // Measure after the rows are laid out, and again whenever their number or the
  // window changes. Observing the content (not the scroller) keeps the cap we
  // write from feeding back in as a resize.
  $effect(() => {
    void count
    const raf = requestAnimationFrame(snap)
    return () => cancelAnimationFrame(raf)
  })

  $effect(() => {
    if (!content) return
    const ro = new ResizeObserver(() => snap())
    ro.observe(content)
    window.addEventListener('resize', snap)
    return () => {
      ro.disconnect()
      window.removeEventListener('resize', snap)
    }
  })
</script>

<div class="flex-none border-b border-border-subtle bg-bg-panel/40" data-testid={testid}>
  <!-- Breathing room above the label: the group's border and the row above it
       must not close on the text. -->
  <div class="px-4 pb-1 pt-2 text-micro font-medium text-text-secondary">{label}</div>
  <div
    bind:this={scroller}
    class="overflow-y-auto"
    style:max-height={capPx === null ? undefined : `${capPx}px`}
    data-testid="search-section-rows"
  >
    <div bind:this={content}>
      {@render children()}
    </div>
  </div>
</div>
