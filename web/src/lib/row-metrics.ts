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
 */

function tokenPx(name: string, fallback: number): number {
  if (typeof document === 'undefined' || typeof getComputedStyle !== 'function') return fallback
  const raw = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  const n = Number.parseFloat(raw)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

let cache: { row: number; rowExcerpt: number; control: number } | null = null

/** Cached because every scroll frame asks, and the tokens never change at runtime. */
export function rowMetrics(): { row: number; rowExcerpt: number; control: number } {
  if (!cache) {
    cache = {
      row: tokenPx('--spacing-row', 42),
      rowExcerpt: tokenPx('--spacing-row-excerpt', 59),
      control: tokenPx('--spacing-control', 32),
    }
  }
  return cache
}

/** Test seam: forget the cached read (nothing in the app changes these). */
export function resetRowMetrics(): void {
  cache = null
}
