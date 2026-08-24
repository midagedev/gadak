/*
 * GDK-850: VirtualRows' offset/window math joins row-metrics — the helpers
 * every virtualized surface already rides — instead of keeping a private
 * copy. The unit project cannot import .svelte components (node
 * environment, no svelte plugin — the IssueList.test.ts precedent), so the
 * delegation is pinned by source scan, and the "behavior must not change"
 * half is a numerical oracle: the exact formulas this component spelled
 * before the replacement, re-expressed here, must agree with
 * rowOffsets/rowWindow on every height mix, boundary landing, and
 * overscan. row-metrics was extracted from this component, so equivalence
 * is exact by construction — the oracle exists to keep it that way.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

import { rowOffsets, rowWindow } from '../../lib/row-metrics'

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(join(HERE, 'VirtualRows.svelte'), 'utf8')

describe('VirtualRows delegates its offset math to row-metrics (GDK-850)', () => {
  test('the prefix-sum sheet and the window come from row-metrics', () => {
    expect(SRC).toContain("from '../../lib/row-metrics'")
    expect(SRC).toContain('rowOffsets(rows.map(height))')
    expect(SRC).toContain('rowWindow(offsets, scrollTop, viewportH, overscan)')
  })

  test('no private copy of the math remains', () => {
    expect(SRC, 'the inline prefix sum was the duplicate owner').not.toContain('new Float64Array')
    expect(
      SRC,
      'indexAt was the private binary search; rowIndexAt owns it now',
    ).not.toContain('function indexAt')
    expect(SRC).not.toContain('Math.max(0, indexAt')
    expect(SRC).not.toContain('Math.min(rows.length, indexAt')
  })
})

/*
 * The oracle: VirtualRows' pre-GDK-850 formulas, verbatim. Same prefix
 * sum, same binary search, same clamps — including the boundary case c34
 * documented for the uniform form: where the viewport bottom lands exactly
 * on a row edge, the window still renders the row whose top is that edge
 * (end is a superset, never a gap).
 */
function oldOffsets(heights: readonly number[]): Float64Array {
  const out = new Float64Array(heights.length + 1)
  for (let i = 0; i < heights.length; i++) out[i + 1] = out[i] + heights[i]
  return out
}

function oldIndexAt(offsets: ArrayLike<number>, y: number): number {
  let lo = 0
  let hi = offsets.length - 1
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (offsets[mid + 1] <= y) lo = mid + 1
    else hi = mid
  }
  return lo
}

function oldWindow(
  offsets: ArrayLike<number>,
  scrollTop: number,
  viewportH: number,
  overscan: number,
): { start: number; end: number } {
  const n = offsets.length - 1
  const start = Math.max(0, oldIndexAt(offsets, scrollTop) - overscan)
  const end = Math.min(n, oldIndexAt(offsets, scrollTop + viewportH) + 1 + overscan)
  return { start, end }
}

describe('row-metrics is equivalent to the formulas VirtualRows shipped (GDK-850)', () => {
  const cases: readonly [name: string, heights: number[]][] = [
    ['uniform 42px (the dense row)', Array<number>(60).fill(42)],
    ['mixed 42/59 (excerpt rows)', [42, 59, 42, 59, 59, 42, 42, 59, 42, 42, 59, 42]],
    ['control-height rows (the 32px tree)', Array<number>(7).fill(32)],
    ['single row', [59]],
  ]

  for (const [name, heights] of cases) {
    test(`offsets and window agree everywhere: ${name}`, () => {
      const offsets = rowOffsets(heights)
      const want = oldOffsets(heights)
      expect(offsets.length, 'sheet is heights+1 long').toBe(want.length)
      for (let i = 0; i < want.length; i++)
        expect(offsets[i], `offsets[${i}]`).toBe(want[i])

      const total = offsets[heights.length]
      for (const overscan of [0, 6, 8]) {
        for (const vh of [0, 1, 100, 420]) {
          // Stepped sweep over the whole scroll range…
          for (let s = 0; s <= total + 100; s += 13) {
            expect(rowWindow(offsets, s, vh, overscan), `s=${s} vh=${vh} o=${overscan}`).toEqual(
              oldWindow(want, s, vh, overscan),
            )
          }
          // …plus every exact boundary landing, where the +1-row superset
          // semantics live (viewport bottom on a row edge).
          for (const s of [...want]) {
            expect(rowWindow(offsets, s, vh, overscan), `edge s=${s} vh=${vh} o=${overscan}`).toEqual(
              oldWindow(want, s, vh, overscan),
            )
          }
        }
      }
    })
  }

  test('empty sheet: one zero offset, empty window', () => {
    const offsets = rowOffsets([])
    const want = oldOffsets([])
    expect(offsets.length).toBe(1)
    expect(offsets[0]).toBe(0)
    expect(want[0]).toBe(0)
    expect(rowWindow(offsets, 0, 400, 8)).toEqual({ start: 0, end: 0 })
    expect(oldWindow(want, 0, 400, 8)).toEqual({ start: 0, end: 0 })
  })
})
