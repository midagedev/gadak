/*
 * GDK-1077 contract tests for the set-aware trail-break ladder.
 *
 * The load-bearing pin is the first block: with the FULL catalog enabled,
 * the generated ladder must equal the GDK-1049 static table exactly — the
 * static table's e2e matrix (list-row-overflow.spec.ts) stays green only
 * if dynamic == static for the set it was measured on. The other blocks
 * pin the property the static table could not have: rungs respond to the
 * enabled set (solo columns paint below the 1360 row cap), and the two
 * measured GDK-1046 anchors (epic/qa_impact) stay measured rather than
 * model-derived, carrying their floor down the priority order.
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { trailBreakCss, trailBreakLadder, TRAIL_BREAK_PRIORITY } from './row-column-thresholds'

const HERE = dirname(fileURLToPath(import.meta.url))

/** The GDK-1049 static table (app.css, removed by GDK-1077) — rung = minimum
 *  row width that paints the column. epic/qa_impact are the measured
 *  GDK-1046 values; the other twelve are the derivations the generator must
 *  reproduce, including due's hole-closed 1340 (not the first-fit 1270). */
const GDK_1049_FULL_CATALOG: [string, number][] = [
  ['epic', 750],
  ['severity', 770],
  ['issue_type', 860],
  ['qa_impact', 1000],
  ['status', 1060],
  ['comment_count', 1130],
  ['created', 1180],
  ['due', 1340],
  ['dev_test_result', 1440],
  ['environment', 1530],
  ['team_group', 1620],
  ['reporter', 1720],
  ['fix_versions', 1840],
  ['components', 1960],
]

describe('trailBreakLadder', () => {
  test('full catalog reproduces the GDK-1049 table exactly', () => {
    const ladder = trailBreakLadder(TRAIL_BREAK_PRIORITY)
    expect(ladder.map((r) => [r.col, r.rung])).toEqual(GDK_1049_FULL_CATALOG)
  })

  test('full-catalog rungs are monotone down the priority order', () => {
    const rungs = trailBreakLadder(TRAIL_BREAK_PRIORITY).map((r) => r.rung)
    const sorted = [...rungs].sort((a, b) => a - b)
    expect(rungs).toEqual(sorted)
  })

  test("due closes the 1300 label-step hole (1340, not the 1270 first-fit)", () => {
    // due fits at 1270 in the 1101–1299 band, but the labels-140 step at
    // 1300 raises the base and un-fits it until 1334 — the rung is the
    // continuous-fit threshold. Pinned alone so a regression reads as this
    // mechanism, not as "full table mismatch".
    const due = trailBreakLadder(TRAIL_BREAK_PRIORITY).find((r) => r.col === 'due')
    expect(due?.rung).toBe(1340)
  })

  test('a components-only set paints below the 1360 row cap', () => {
    const solo = trailBreakLadder(['components'])
    expect(solo).toHaveLength(1)
    expect(solo[0].col).toBe('components')
    expect(solo[0].rung).toBeLessThan(1360)
    // 303 + base 303 + slot 110 + one gap = 726 → 730.
    expect(solo[0].rung).toBe(730)
  })

  test("the app's solo-set shape (epic always present unless grouped by epic)", () => {
    // What IssueList injects for the e2e solo case (cl=components, default
    // grouping): epic rides along at its measured 750, components at 800.
    const ladder = trailBreakLadder(['epic', 'components'])
    expect(ladder.map((r) => [r.col, r.rung])).toEqual([
      ['epic', 750],
      ['components', 800],
    ])
  })

  test('measured anchors carry their floor down (set-fit semantics)', () => {
    // qa_impact's measured 1000 outranks status; status's model rung
    // (~810) is below it, so the set cannot fit before 1000.
    const ladder = trailBreakLadder(['qa_impact', 'status'])
    expect(ladder.map((r) => [r.col, r.rung])).toEqual([
      ['qa_impact', 1000],
      ['status', 1000],
    ])
  })

  test('empty set → no rules', () => {
    expect(trailBreakLadder([])).toEqual([])
    expect(trailBreakCss([])).toBe('')
  })

  test('unknown keys are ignored (caller passes the full column set)', () => {
    const ladder = trailBreakLadder(['assignee', 'updated', 'labels', 'components', 'nope'])
    expect(ladder.map((r) => r.col)).toEqual(['components'])
  })
})

describe('trailBreakCss', () => {
  const css = trailBreakCss(TRAIL_BREAK_PRIORITY)

  test('wraps in @layer utilities and hides below the rung (GDK-766, CQ fallback)', () => {
    expect(css.startsWith('@layer utilities {')).toBe(true)
    // max-width = rung−1: the rule hides below the rung, and a browser
    // without container-query support lands on columns-visible.
    expect(css).toContain('@container issuerow (max-width: 1959px) {\n    .trail-break-components {')
    expect(css).toContain('@container issuerow (max-width: 749px) {\n    .trail-break-epic {')
    expect(css).toContain('@container issuerow (max-width: 999px) {\n    .trail-break-qa {')
  })

  test('every trail-break class IssueRow.svelte wears is covered, and vice versa', () => {
    // The class names are the coupling to IssueRow's slot markup — a renamed
    // class there would silently stop hiding. Scan the source (the
    // established idiom: IssueRow.test.ts parses this file too) and pin
    // both directions against the full-catalog css.
    const row = readFileSync(join(HERE, 'IssueRow.svelte'), 'utf8')
    const worn = [...new Set(row.match(/trail-break-[a-z-]+/g) ?? [])]
    expect(worn.length).toBeGreaterThanOrEqual(14)
    for (const cls of worn) expect(css).toContain(`.${cls} {`)
    for (const { cssClass } of trailBreakLadder(TRAIL_BREAK_PRIORITY)) {
      expect(worn, `${cssClass} must be worn by a slot in IssueRow.svelte`).toContain(cssClass)
    }
  })
})
