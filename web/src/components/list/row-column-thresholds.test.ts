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
import {
  alwaysOnFoldCss,
  alwaysOnFoldLadder,
  rowFoldCss,
  trailBreakCss,
  trailBreakLadder,
  TITLE_COMFORT_PX,
  TRAIL_BREAK_PRIORITY,
} from './row-column-thresholds'

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

/*
 * GDK-1791 contract: the always-on strip yields before the title does.
 *
 * The numbers below are the model's, and the model is checked against the
 * browser by e2e/list-title-floor.spec.ts — this block pins the arithmetic
 * so a rung cannot move silently, and the ordering property that makes the
 * fold a fold rather than a squeeze.
 *
 * FAIL-first for the whole change is in e2e/list-title-floor.spec.ts: with
 * the static 620/480/400 rungs this file replaced, the 1000px window
 * measured a 222px title (floor 292.5px, six of six rows truncated).
 */
describe('alwaysOnFoldLadder (GDK-1791)', () => {
  /** The demo fixture's default option set: epic, and nothing else. */
  const DEFAULT_SET = ['epic']

  const hiddenAt = (set: readonly string[], w: number): string[] =>
    alwaysOnFoldLadder(set)
      .filter((f) => f.bands.some((b) => w >= b.lo && w <= b.hi))
      .map((f) => f.cssClass)

  test('the three widths this issue was reported at', () => {
    // Row widths measured in the browser, demo fixture, same round:
    // a 1440px window is a 1168px row, 1000 → 728, 800 → 592.
    expect(hiddenAt(DEFAULT_SET, 1168), 'a 1440px window keeps the whole strip').toEqual([])
    expect(hiddenAt(DEFAULT_SET, 728), 'a 1000px window drops assignee+updated only').toEqual([
      'trail-fold-1',
    ])
    expect(hiddenAt(DEFAULT_SET, 592), 'an 800px window keeps stale and the title').toEqual([
      'trail-fold-1',
      'trail-fold-2',
      'trail-fold-3',
    ])
  })

  test('a hidden level implies every level folded before it', () => {
    const order = ['trail-fold-1', 'trail-fold-2', 'trail-fold-3']
    for (const set of [[], DEFAULT_SET, ['epic', 'status', 'due'], TRAIL_BREAK_PRIORITY]) {
      for (let w = 300; w <= 1400; w += 1) {
        const hidden = hiddenAt(set, w)
        expect(hidden, `[${set.join(',')}] at ${w}px hides out of order`).toEqual(
          order.slice(0, hidden.length),
        )
      }
    }
  })

  test('the bands see the enabled set — epic opens a hole labels falls into', () => {
    // epic paints from 750 and costs 64px + a gap, so with it enabled the
    // floor is unmet in 750-792 even with assignee+updated gone. `labels`
    // folds there and comes back above it; with no option column on, that
    // band does not exist at all.
    const labels = (set: readonly string[]) =>
      alwaysOnFoldLadder(set).find((f) => f.cssClass === 'trail-fold-2')!.bands
    expect(labels([])).toEqual([{ lo: 0, hi: 718 }])
    expect(labels(DEFAULT_SET)).toEqual([
      { lo: 0, hi: 718 },
      { lo: 750, hi: 792 },
    ])
  })

  test('a narrow row folds the whole strip whether or not that reaches the floor', () => {
    // The rule is "closer to the floor is better", not "only when it wins".
    // A draft that folded only when the fold reached 293px stopped folding
    // under 549px — the narrowest rows the app makes — and left the title on
    // its 13ch hard floor with the whole strip beside it.
    expect(hiddenAt(TRAIL_BREAK_PRIORITY, 400)).toEqual([
      'trail-fold-1',
      'trail-fold-2',
      'trail-fold-3',
    ])
    expect(hiddenAt([], 400)).toEqual(['trail-fold-1', 'trail-fold-2', 'trail-fold-3'])
  })

  test('css hides inside the band, in @layer utilities', () => {
    const css = alwaysOnFoldCss(DEFAULT_SET)
    expect(css).toContain('@layer utilities {')
    expect(css).toContain('@container issuerow (max-width: 718px)')
    expect(css).toContain('@container issuerow (min-width: 750px) and (max-width: 792px)')
    expect(css).toContain('.trail-fold-2')
    expect(css).toContain(String(TITLE_COMFORT_PX))
  })

  test('rowFoldCss carries both ladders', () => {
    const css = rowFoldCss(DEFAULT_SET)
    expect(css).toContain(trailBreakCss(DEFAULT_SET))
    expect(css).toContain(alwaysOnFoldCss(DEFAULT_SET))
  })
})
