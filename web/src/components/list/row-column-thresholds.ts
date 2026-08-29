/*
 * Trail-break rung constants + set-aware generator (GDK-1077).
 *
 * Every optional column's row-width hide (@container issuerow) had one
 * static threshold table in app.css since GDK-1049 — cumulative over the
 * FULL catalog: a column's rung carried every higher-priority slot's width
 * whether the user enabled them or not. CSS container queries know the
 * row's width, not the enabled set's sum, so six rungs (dev_test_result …
 * components) sat above the 1360px row cap and could not paint under any
 * width, even enabled alone. This module is the single owner of the
 * arithmetic now: the ladder is recomputed for the enabled set (pure
 * constant math — no layout reads, no ResizeObserver) and injected as one
 * dynamic <style id="trail-break-rungs"> by IssueList.svelte. app.css keeps
 * only a pointer comment where the static table used to be.
 *
 * Derivation, moved verbatim from the GDK-1049 app.css comment (measured
 * against the demo fixture, 2026-08-28):
 *   rung = the narrowest row where the column plus everything that outranks
 *   it fits —
 *     303 (leading 196 + the title's min-w-[13ch] floor 106.4, measured)
 *     + Σ slot widths + 10px per gap (gap-2.5) ≤ row width,
 *   re-checked at every event that grows the visible set: the fold unfires
 *   (401 / 481 / 621 — only 621 is modeled; no option rung can land below
 *   it, the narrowest possible solo fit is 303+303+40+10 = 656), the ≤1100
 *   step (stale 44→56, labels 64→76), labels 140 at ≥1300, and the
 *   epic/qa rungs joining. Priority is the order the old viewport groups
 *   encoded (sm > md > lg, DOM order inside a group).
 *
 * The fold base occupies, by row-width band: 303 up to 1100, 327 in
 * 1101–1299 (the +24 stale/labels step), 391 from 1300 (labels 140). The
 * base only grows at band edges, so a set that first fits just below a
 * step can un-fit just above it (due at the 1300 step: fits at 1270, hole
 * until 1334 — GDK-1049's due rung is 1340, not 1270, for exactly this
 * reason). The generator takes the continuous-fit threshold: the smallest
 * width from which the set fits at that width and every wider one.
 *
 * The two GDK-1046 rungs keep their MEASURED values (epic 750, qa_impact
 * 1000): the model's base is an approximation and those two were measured
 * against the real layout, so they are anchors, not derivations. A derived
 * rung never drops below an anchored one above it — the rung is "this
 * column plus everything that outranks it fits", and an anchored column
 * that cannot fit carries that floor down the priority order (a no-op for
 * the full catalog, where the table is already monotone).
 *
 * Known edge, carried from GDK-1046/1049: a row with a favorite/watch
 * marker needs up to 38px more than the title floor leaves, so a marked
 * row sitting within ~38px above a rung can paint that slot slightly past
 * the scroller.
 *
 * The generated CSS wraps in @layer utilities for the GDK-766 reason: the
 * slots keep their `flex` Tailwind utility, so the display:none must
 * outrank it — in @layer components it would lose. Written as max-width so
 * a browser without container-query support lands on the columns-visible
 * step (the chipfold direction).
 */

/** Priority order — a rung is "this column plus everything above it fits".
 *  Same order the GDK-1049 table encoded; DOM order in IssueRow.svelte
 *  matches for the rendered slots. */
export const TRAIL_BREAK_PRIORITY = [
  'epic',
  'severity',
  'issue_type',
  'qa_impact',
  'status',
  'comment_count',
  'created',
  'due',
  'dev_test_result',
  'environment',
  'team_group',
  'reporter',
  'fix_versions',
  'components',
] as const

export type TrailBreakColumn = (typeof TRAIL_BREAK_PRIORITY)[number]

/** Slot width per column, px (GDK-1049 table · the w-* utilities on the
 *  slots in IssueRow.svelte). */
const SLOT_WIDTH: Record<TrailBreakColumn, number> = {
  epic: 64, // w-16
  severity: 80, // w-20
  issue_type: 80, // w-20
  qa_impact: 96, // w-24
  status: 80, // w-20
  comment_count: 40, // w-10
  created: 40, // w-10
  due: 80, // w-20
  dev_test_result: 96, // w-24
  environment: 80, // w-20
  team_group: 80, // w-20
  reporter: 90, // w-[90px]
  fix_versions: 110, // w-[110px]
  components: 110, // w-[110px]
}

/** Measured rungs (GDK-1046) — anchors, not derived from the model. */
const MEASURED_RUNGS: Partial<Record<TrailBreakColumn, number>> = {
  epic: 750,
  qa_impact: 1000,
}

/** The class each slot wears in IssueRow.svelte (qa_impact's is historical:
 *  `trail-break-qa`, from GDK-1046 — not the id with underscores mapped). */
const BREAK_CLASS: Record<TrailBreakColumn, string> = {
  epic: 'trail-break-epic',
  severity: 'trail-break-severity',
  issue_type: 'trail-break-issue-type',
  qa_impact: 'trail-break-qa',
  status: 'trail-break-status',
  comment_count: 'trail-break-comment-count',
  created: 'trail-break-created',
  due: 'trail-break-due',
  dev_test_result: 'trail-break-dev-test-result',
  environment: 'trail-break-environment',
  team_group: 'trail-break-team-group',
  reporter: 'trail-break-reporter',
  fix_versions: 'trail-break-fix-versions',
  components: 'trail-break-components',
}

/** Leading (checkbox/priority/status dot/key) + the title's 13ch floor. */
const LEAD_AND_TITLE = 303
/** gap-2.5 between slots. */
const GAP = 10

/** Fold-base width by row band: [lo, hi] inclusive, base px. The 621 lo is
 *  trail-fold-1's unfire (below it the fold model changes and no option
 *  rung may land — see the header).
 *
 *  GDK-1155 moved that unfire to 691 while the detail panel is open, and
 *  this table deliberately does not follow. It is the safe direction: in
 *  621–690 with the panel open the row has `assignee` + `updated` + their
 *  gaps (80px) MORE than these bands assume, so every rung stays
 *  conservative. Following it would mean a second, state-dependent ladder
 *  for a range where the measured cheapest rung (660) barely reaches. */
const BASE_BANDS: readonly { lo: number; hi: number; base: number }[] = [
  { lo: 621, hi: 1100, base: 303 },
  { lo: 1101, hi: 1299, base: 327 },
  { lo: 1300, hi: Number.POSITIVE_INFINITY, base: 391 },
]

const roundUp10 = (n: number): number => Math.ceil(n / 10) * 10

/**
 * Smallest row width from which `slotsW` of slots (with `count` gaps) fits
 * at that width AND at every wider width. The base grows at band edges, so
 * the plain first-fit can leave an un-fit hole above a step; each band's
 * unfitted tail contributes its top (+1) and the max over bands is the
 * continuous threshold. Never below the first band's lo: below 621 the
 * fold model differs and the caller's bands do not describe the row.
 */
function continuousFitWidth(slotsW: number, count: number): number {
  let threshold = 0
  for (const band of BASE_BANDS) {
    const fit = LEAD_AND_TITLE + band.base + slotsW + GAP * count
    if (fit - 1 >= band.lo) threshold = Math.max(threshold, Math.min(fit - 1, band.hi) + 1)
  }
  return Math.max(threshold, BASE_BANDS[0].lo)
}

/** The style element the ladder is injected into (IssueList.svelte owns its
 *  lifetime; one element for the whole document — only that list renders
 *  rows that wear these classes). */
export const TRAIL_BREAK_STYLE_ID = 'trail-break-rungs'

/** One rung of the set-aware ladder, in priority order. `rung` is the
 *  minimum row width (px) that paints the column. */
export interface TrailBreakRung {
  col: TrailBreakColumn
  rung: number
  /** The class the slot wears in IssueRow.svelte. */
  cssClass: string
}

/**
 * Cumulative ladder for the enabled set: for each enabled column (priority
 * order), the narrowest row where it plus the enabled columns that outrank
 * it fit. Unknown keys are ignored — the caller passes the view's full
 * column set (ColumnKey superset), of which only option columns ladder.
 */
export function trailBreakLadder(enabled: readonly string[]): TrailBreakRung[] {
  const present = TRAIL_BREAK_PRIORITY.filter((col) => enabled.includes(col))
  const rungs: TrailBreakRung[] = []
  let slotsW = 0
  let count = 0
  // Anchored rungs above carry down (set-fit semantics — see header).
  let floor = 0
  for (const col of present) {
    slotsW += SLOT_WIDTH[col]
    count += 1
    const rung = MEASURED_RUNGS[col] ?? roundUp10(continuousFitWidth(slotsW, count))
    floor = Math.max(floor, rung)
    rungs.push({ col, rung: floor, cssClass: BREAK_CLASS[col] })
  }
  return rungs
}

/**
 * The <style> text for an enabled set: one @container issuerow max-width
 * rule per rung (rung−1: the rule hides BELOW the rung), wrapped in
 * @layer utilities (GDK-766 — must outrank the slots' `flex` utility).
 * Empty set → empty string (no rules; every optional column paints).
 */
export function trailBreakCss(enabled: readonly string[]): string {
  const rungs = trailBreakLadder(enabled)
  if (rungs.length === 0) return ''
  const rules = rungs.map(
    (r) =>
      `  @container issuerow (max-width: ${r.rung - 1}px) {\n    .${r.cssClass} {\n      display: none;\n    }\n  }`,
  )
  return [
    `@layer utilities {`,
    `  /* trail-break rungs for the enabled set [${rungs.map((r) => r.col).join(', ')}] —`,
    `     generated, single owner: web/src/components/list/row-column-thresholds.ts (GDK-1077) */`,
    ...rules,
    `}`,
    ``,
  ].join('\n')
}

/**
 * Sync the document's ladder <style> to `css` (create if missing, remove
 * when empty). DOM-touching on purpose and only on call — the module stays
 * importable from node (the pure functions above are what unit tests pin).
 */
export function syncTrailBreakStyle(css: string): void {
  let el = document.getElementById(TRAIL_BREAK_STYLE_ID) as HTMLStyleElement | null
  if (!css) {
    el?.remove()
    return
  }
  if (!el) {
    el = document.createElement('style')
    el.id = TRAIL_BREAK_STYLE_ID
    document.head.append(el)
  }
  el.textContent = css
}
