/*
 * GDK-842 chunk 3: the issue list's virtualization must not own a row-height
 * constant. vitest's unit project cannot import .svelte components (node
 * environment, no svelte plugin — the Onboarding test precedent), so this
 * pins the source the way DialogShell.test.ts does: the height owner
 * (rowMetrics), the invalidation subscription, and every scroll-geometry
 * path deriving from per-row offsets. The numeric contracts those helpers
 * must satisfy live in row-metrics.test.ts.
 *
 * The defect class this closes: ROW_H = 42 baked the window math while
 * IssueRow paints h-row / h-row-excerpt from the tokens — change only the
 * token (user override, GDK-842) or add a match-line row mode, and the
 * window, sticky header, and cursor follow all silently misalign (census
 * Q4 #1: the 42-window vs 59-paint drift).
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(join(HERE, 'IssueList.svelte'), 'utf8')

test('no row-height constant: rowMetrics owns the heights', () => {
  expect(SRC, 'ROW_H must be gone — a second height owner is the defect').not.toContain('ROW_H')
  expect(SRC).toContain('rowMetrics()')
})

test('re-reads the metrics when the tokens change at runtime', () => {
  expect(SRC).toContain('onRowMetricsInvalidated(')
})

test('the window derives from per-row offsets, not a uniform multiple', () => {
  expect(SRC).toContain('rowOffsets(heights)')
  expect(SRC).toContain('rowWindow(offsets, scrollTop, viewportH, OVERSCAN)')
  expect(SRC).toContain('rowIndexAt(offsets, scrollTop)')
  expect(SRC).toContain('style:top="{offsets[idx]}px"')
  expect(SRC).toContain('style:height="{heights[idx]}px"')
})

test('issue rows carry a paint mode so an excerpt row rides the offsets', () => {
  // Today this list never passes a match line to IssueRow, so every row is
  // 'row' and the paint is byte-identical to the uniform era. The mode
  // field is what makes rowHeightOf the single place a taller row mode
  // would plug in — without it the window cannot see a 59px paint.
  expect(SRC).toContain("mode: 'row'")
  expect(SRC).toContain("'row-excerpt'")
})

test('cursor follow, group reveal, and the sticky push ride the same offsets', () => {
  // scrollToRow clamps against the row's own top/bottom…
  expect(SRC).toContain('const top = offsets[rowIndex]')
  // …the reveal effect scrolls to the header's offset…
  expect(SRC).toContain('scroller.scrollTop = offsets[idx]')
  // …and the floating header pushes by the next header's measured top.
  expect(SRC).toContain('const nextTop = offsets[i]')
})
