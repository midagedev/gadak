/*
 * Row heights, read from the CSS tokens that actually set them.
 *
 * A virtualized list has to know a row's height in JavaScript before the row
 * exists, which normally means the number is written down twice — once in the
 * token and once in the measuring code — and the two drift the next time
 * someone tunes the list's density. These read the token instead, so there is
 * still one source. The fallbacks only matter where getComputedStyle does not
 * exist (SSR, tests without a document); a wrong height there costs scroll
 * geometry, not correctness.
 *
 * GDK-842 (dim wave): the tokens DO change at runtime now — applyUserTokens
 * installs user overrides after boot — so the cache gained an invalidation
 * hook, and the offset math every virtualized surface needs (prefix sums,
 * the window, the sticky-header anchor) lives here instead of being
 * re-spelled per component. VirtualRows.svelte's math is the original; the
 * issue list rides the same formulas through these helpers.
 */

export type RowMode = 'row' | 'row-excerpt'

export interface RowMetrics {
  row: number
  rowExcerpt: number
  control: number
}

function tokenPx(name: string, fallback: number): number {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return fallback
  const raw = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  const n = Number.parseFloat(raw)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

let cache: RowMetrics | null = null
const invalidated: Set<() => void> = new Set()

/**
 * Cached because every scroll frame asks — but the tokens can change at
 * runtime now (GDK-842 user overrides); applyUserTokens calls this after
 * installing the override style, and subscribers (IssueList's metrics
 * snapshot) re-read on the next frame.
 */
export function rowMetrics(): RowMetrics {
  if (!cache) {
    cache = {
      row: tokenPx('--spacing-row', 36),
      rowExcerpt: tokenPx('--spacing-row-excerpt', 59),
      control: tokenPx('--spacing-control', 32),
    }
  }
  return cache
}

/** Drop the cache and notify subscribers. Safe to call with none. */
export function invalidateRowMetrics(): void {
  cache = null
  for (const fn of [...invalidated]) fn()
}

/**
 * Subscribe to token-driven metric changes. Returns the unsubscribe; the
 * issue list uses it to re-snapshot heights without remounting.
 */
export function onRowMetricsInvalidated(fn: () => void): () => void {
  invalidated.add(fn)
  return () => invalidated.delete(fn)
}

/**
 * Prefix-sum offsets for per-row heights: offsets[i] is row i's top,
 * offsets[n] the sheet's total height. Float64Array to match
 * VirtualRows.svelte (same memory shape, same arithmetic).
 */
export function rowOffsets(heights: readonly number[]): Float64Array {
  const offsets = new Float64Array(heights.length + 1)
  for (let i = 0; i < heights.length; i++) offsets[i + 1] = offsets[i] + heights[i]
  return offsets
}

/**
 * The row the scroller top sits in: the first row whose bottom is past y
 * (VirtualRows.svelte's indexAt). y at or past the sheet's end returns n —
 * the past-the-end sentinel callers clamp against.
 */
export function rowIndexAt(offsets: ArrayLike<number>, y: number): number {
  const n = offsets.length - 1
  let lo = 0
  let hi = n
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (offsets[mid + 1] > y) hi = mid
    else lo = mid + 1
  }
  return lo
}

/** The render window [start, end) covering the viewport plus overscan rows. */
export function rowWindow(
  offsets: ArrayLike<number>,
  scrollTop: number,
  viewportH: number,
  overscan: number,
): { start: number; end: number } {
  const n = offsets.length - 1
  const start = Math.max(0, rowIndexAt(offsets, scrollTop) - overscan)
  const end = Math.min(n, rowIndexAt(offsets, scrollTop + viewportH) + 1 + overscan)
  return { start, end }
}
