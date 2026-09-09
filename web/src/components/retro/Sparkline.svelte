<script lang="ts">
  /*
   * The metric row's own shape (GDK-1712).
   *
   * A retro table answers "what is the number" per bucket and leaves "which
   * way is it going" to the reader's eye scanning left to right. Twelve
   * columns is more than an eye scans reliably, so the row carries its own
   * line: the same numbers, drawn once, small enough to sit inside the label
   * column without changing the row's height.
   *
   * Hand-drawn SVG rather than a chart library on purpose. There is one
   * shape, no axes, no legend, no interaction — everything a charting
   * dependency exists to provide is something this must not have. It also
   * keeps the mark in `currentColor`, so the line takes the tone of the text
   * beside it and needs no color of its own in either theme.
   *
   * Nulls break the line instead of interpolating across them: a bucket with
   * no value is not a value between its neighbours, and a report whose whole
   * point is "this cell is empty and here is why" (GDK-1679) must not draw
   * over the gap.
   */
  let {
    values,
    metric = '',
    width = 64,
    height = 16,
  }: {
    /** One per bucket, oldest first. `null` is a gap, not a zero. */
    values: (number | null)[]
    /** Which row this is, on the mark itself — so a test or an inspection
     *  can name the line it is looking at rather than count anonymous ones. */
    metric?: string
    width?: number
    height?: number
  } = $props()

  // Inset by the stroke's own half-width plus the last dot's radius, so
  // neither is clipped at the box edge.
  const PAD = 2.5

  type Point = { x: number; y: number }

  const points = $derived.by<(Point | null)[]>(() => {
    const nums = values.filter((v): v is number => v != null)
    if (nums.length === 0) return []
    let lo = Math.min(...nums)
    let hi = Math.max(...nums)
    // A flat series has no range to scale into; draw it down the middle
    // rather than dividing by zero or pinning it to an edge.
    if (hi === lo) {
      hi = lo + 1
      lo = lo - 1
    }
    const stepX = values.length > 1 ? (width - PAD * 2) / (values.length - 1) : 0
    return values.map((v, i) =>
      v == null
        ? null
        : {
            x: PAD + i * stepX,
            y: PAD + (1 - (v - lo) / (hi - lo)) * (height - PAD * 2),
          },
    )
  })

  /** Sub-paths, one per unbroken run — a gap ends the run it was in. */
  const segments = $derived.by<string[]>(() => {
    const out: string[] = []
    let run: Point[] = []
    const flush = (): void => {
      if (run.length > 1) out.push('M' + run.map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join('L'))
      run = []
    }
    for (const p of points) {
      if (p) run.push(p)
      else flush()
    }
    flush()
    return out
  })

  /** The newest bucket that has a value — the dot says "you are here". */
  const last = $derived.by<Point | null>(() => {
    for (let i = points.length - 1; i >= 0; i--) if (points[i]) return points[i]
    return null
  })

  // Two points are a line; one is a dot with no direction in it, and a
  // single dot floating in the label column reads as a bullet, not a trend.
  const enough = $derived(points.filter(Boolean).length >= 2)
</script>

<!-- A row with fewer than two values keeps the slot: the label column's
     width is the widest label plus this box, and a row that gave the box up
     (the vision pass caught Resume, whose values were all null) moved every
     label's right edge. -->
{#if !enough}
  <span class="inline-block flex-none" style:width="{width}px" style:height="{height}px" aria-hidden="true" data-testid="retro-sparkline-empty" data-metric={metric}></span>
{:else}
  <svg
    {width}
    {height}
    viewBox="0 0 {width} {height}"
    fill="none"
    aria-hidden="true"
    class="flex-none overflow-visible text-text-muted"
    data-testid="retro-sparkline"
    data-metric={metric}
  >
    {#each segments as d (d)}
      <path {d} stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
    {/each}
    {#if last}
      <circle cx={last.x} cy={last.y} r="1.75" fill="currentColor" class="text-text-secondary" />
    {/if}
  </svg>
{/if}
