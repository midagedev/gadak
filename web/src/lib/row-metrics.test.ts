/*
 * GDK-842 chunk 3: row geometry has one owner — the CSS tokens via
 * rowMetrics() — and the virtual window derives from per-row heights, not a
 * uniform constant. Two contracts:
 *
 *   1. invalidateRowMetrics() re-reads the tokens (applyUserTokens calls it
 *      after installing override styles). The old "tokens never change at
 *      runtime" assumption is gone.
 *   2. The offset math mirrors VirtualRows.svelte's (prefix sums + the same
 *      indexAt binary search) so every virtualized surface agrees by
 *      construction — including the 42px window vs 59px excerpt-row paint
 *      drift the census flagged (Q4 #1).
 *
 * Node environment, no DOM: computed styles arrive via stubbed globals.
 */
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  invalidateRowMetrics,
  onRowMetricsInvalidated,
  rowIndexAt,
  rowMetrics,
  rowOffsets,
  rowWindow,
} from './row-metrics'

describe('rowMetrics reads the tokens it documents', () => {
  afterEach(() => {
    invalidateRowMetrics()
    vi.unstubAllGlobals()
  })

  it('falls back to the shipped defaults without a DOM', () => {
    expect(rowMetrics()).toEqual({ row: 42, rowExcerpt: 59, control: 32 })
  })

  it('caches between reads and re-reads after invalidateRowMetrics', () => {
    const computed: Record<string, string> = { '--spacing-row': '42px' }
    vi.stubGlobal('document', { documentElement: {} })
    vi.stubGlobal('getComputedStyle', () => ({
      getPropertyValue: (n: string) => computed[n] ?? '',
    }))

    expect(rowMetrics().row).toBe(42)
    computed['--spacing-row'] = '48px'
    expect(rowMetrics().row).toBe(42) // cached — every scroll frame asks

    invalidateRowMetrics()
    expect(rowMetrics().row).toBe(48)
    expect(rowMetrics().rowExcerpt).toBe(59) // unstubbed token keeps its default
  })

  it('notifies subscribers on invalidation and honors unsubscribe', () => {
    const calls: string[] = []
    const off = onRowMetricsInvalidated(() => calls.push('a'))
    onRowMetricsInvalidated(() => calls.push('b'))
    invalidateRowMetrics()
    expect(calls).toEqual(['a', 'b'])

    off()
    invalidateRowMetrics()
    expect(calls).toEqual(['a', 'b', 'b'])
  })
})

describe('rowOffsets/rowWindow mirror the VirtualRows math', () => {
  it('produces an empty, zero-height sheet for no rows', () => {
    const offsets = rowOffsets([])
    expect(offsets.length).toBe(1)
    expect(offsets[0]).toBe(0)
    expect(rowWindow(offsets, 0, 400, 8)).toEqual({ start: 0, end: 0 })
  })

  it('matches the old uniform closed form — start always, end off exact boundaries', () => {
    const H = 42
    const N = 100
    const O = 8
    const offsets = rowOffsets(Array<number>(N).fill(H))
    // The old ROW_H closed form: start = max(0, floor(s/H) - O),
    // end = min(N, ceil((s+vh)/H) + O). Where the viewport bottom lands
    // exactly on a row boundary the offset form renders one more row —
    // the row whose top IS the boundary. VirtualRows.svelte has always
    // done this; it is a superset of the old window, never a gap.
    for (const vh of [400, 420]) {
      for (let s = 0; s <= 3 * H; s++) {
        const w = rowWindow(offsets, s, vh, O)
        const oldEnd = Math.min(N, Math.ceil((s + vh) / H) + O)
        expect(w.start, `vh=${vh} s=${s}`).toBe(Math.max(0, Math.floor(s / H) - O))
        expect(w.end, `vh=${vh} s=${s}`).toBeGreaterThanOrEqual(oldEnd)
        expect(w.end, `vh=${vh} s=${s}`).toBeLessThanOrEqual(oldEnd + 1)
      }
    }
  })

  it('keeps scroll geometry aligned when excerpt rows (59px) mix with plain rows (42px)', () => {
    const heights = [42, 59, 42, 59, 59, 42, 42, 59, 42, 42]
    const offsets = rowOffsets(heights)

    // Prefix sums: row i paints at [offsets[i], offsets[i+1]) — no gaps,
    // no overlaps, whatever the mix. This is the exact-alignment contract
    // that a uniform window silently broke for excerpt rows.
    let top = 0
    heights.forEach((h, i) => {
      expect(offsets[i]).toBe(top)
      top += h
    })
    expect(offsets[heights.length]).toBe(6 * 42 + 4 * 59)

    // Coverage: every row intersecting the viewport is inside the window.
    for (let s = 0; s <= 488 - 42; s += 7) {
      const w = rowWindow(offsets, s, 100, 2)
      for (let i = 0; i < heights.length; i++) {
        const visible = offsets[i + 1] > s && offsets[i] < s + 100
        if (visible) {
          expect(i, `scrollTop=${s}`).toBeGreaterThanOrEqual(w.start)
          expect(i, `scrollTop=${s}`).toBeLessThan(w.end)
        }
      }
    }

    // The sticky-header anchor is the row the scroller top sits in — the
    // binary-search "first row whose bottom is past y".
    const anchors = [
      [0, 0],
      [42, 1],
      [101, 2],
      [143, 3],
      [487, 9],
      [488, 10], // past the end: the N sentinel, same as VirtualRows
    ] as const
    for (const [y, want] of anchors) {
      expect(rowIndexAt(offsets, y)).toBe(want)
    }
  })

  it('clamps the window to the row count on both ends', () => {
    const offsets = rowOffsets([42, 42, 42])
    expect(rowWindow(offsets, 0, 400, 8)).toEqual({ start: 0, end: 3 })
    expect(rowWindow(offsets, 10_000, 400, 8)).toEqual({ start: 0, end: 3 })
  })
})
