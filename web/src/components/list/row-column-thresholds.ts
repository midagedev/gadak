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

/* ────────────────────────────────────────────────────────────────────────
 * GDK-1791: the always-on strip yields to the title's comfort floor.
 *
 * Everything above this line prices the OPTIONAL columns against the title's
 * hard floor (13ch, LEAD_AND_TITLE) — a column the user switched on may
 * squeeze the title down to the point where it stops being a title. That is
 * a choice the user made. The always-on strip is not: reopen, stale, deploy,
 * labels, assignee and updated are the row's furniture, and they were taking
 * their fixed widths first and leaving the title the remainder.
 *
 * Measured on this tree before the change (demo fixture, en, Chromium,
 * scratch harness title-width.mjs — top six rows, `.row-title`
 * getBoundingClientRect().width and scrollWidth > clientWidth):
 *
 *   viewport   row    title   truncated   trailing strip
 *   1440      1168      564       0/6     reopen stale deploy epic labels assignee updated
 *   1000       728      222       6/6     reopen stale deploy labels assignee updated
 *    800       592      166       6/6     reopen stale deploy labels
 *
 * The window narrows 30% and the title loses 60%: the strip's widths are
 * fixed, so every pixel the row loses comes off the one flexible element.
 *
 * The rule this module already had — "a hide drops the whole slot, never a
 * squeeze" — is the fix; it only needed a wider scope. Below the rungs
 * generated here the always-on slots drop, in the fold order the row already
 * uses (assignee+updated, then labels, then reopen+deploy; `stale` never
 * folds — "is this stuck" is the one fact the narrow row keeps), until the
 * title can hold TITLE_COMFORT_PX.
 *
 * Why a second floor rather than raising the 13ch one: the 13ch floor is a
 * CSS min-width, and two floors cannot share one row (GDK-1089 measured a
 * proportional floor pushing the label chips 52px past the scroller, because
 * chipfold-labels has a min-width of its own). This floor is not a min-width
 * at all — it never binds on the element. It is the number the fold rungs
 * are priced against, so the strip is gone before the title gets near it.
 *
 * Measured constants, same round, same harness. ROW_OVERHEAD is
 * row − title − trailing-strip, identical at all three widths above
 * (1168−564−392 = 728−222−294 = 592−166−214 = 212): the leading strip to the
 * title's left edge (186), the gap before the trailing strip (10) and the
 * row's own horizontal padding (16).
 */

/** row width − title width − trailing-strip width. Measured 2026-09-11. */
const ROW_OVERHEAD = 212

/**
 * The width at which a title is a title. 36ch in the shipped 11px face
 * (1ch = 8.125px measured) = 293px.
 *
 * Derived against the demo fixture in the title's own font, same round
 * (scratch harness floor-derive.mjs, n=45 summaries): the shortest prefix
 * that is unique among the list's summaries is 15.8ch at the 95th
 * percentile, and the 25th-percentile summary renders whole in 325px. So
 * 36ch clears identification with better than 2x margin and renders a
 * quarter of the rows complete, where the 13ch hard floor (106px) is the
 * "about twelve characters and an ellipsis" GDK-1089 measured.
 *
 * px, not ch, because this is rung arithmetic: the whole module is a
 * constant-math model of the row and reads no layout. A narrower ch (CI's
 * Linux face) makes the real title wider than this model assumes, which is
 * the safe direction.
 */
export const TITLE_COMFORT_PX = 293

/** Always-on slots that never fold: the row's last fact. */
const STALE_WIDTH_BY_BAND = { narrow: 44, wide: 56 } as const
/** Labels slot, by the same bands app.css steps it on (≤1100 / ≤1299 / ≥1300). */
const LABELS_WIDTH_BY_BAND = { narrow: 64, mid: 76, wide: 140 } as const

/** One always-on fold level and the row widths at which it is hidden.
 *  `bands` are inclusive [lo, hi] row widths; `lo === 0` means "and narrower". */
export interface AlwaysOnFold {
  /** The class the slots wear in IssueRow.svelte. */
  cssClass: string
  /** What the level holds, for the generated comment. */
  slots: readonly string[]
  bands: readonly { lo: number; hi: number }[]
}

/** Fold order — the level that goes FIRST is first. Same order the row's
 *  information order names (what to keep last): stale → reopen → deploy →
 *  labels → assignee → updated. */
const ALWAYS_ON_LEVELS: readonly { cssClass: string; slots: readonly string[]; width: number; count: number }[] = [
  // assignee (w-5, 20) + updated (w-10, 40). The carryover glyph rides this
  // class too and is not modeled: it is on some rows only and folds with this
  // level, so leaving it out keeps the model conservative.
  { cssClass: 'trail-fold-1', slots: ['assignee', 'updated'], width: 60, count: 2 },
  // labels — band-dependent width, resolved in alwaysOnStrip.
  { cssClass: 'trail-fold-2', slots: ['labels'], width: 0, count: 1 },
  // reopen (w-9, 36) + deploy (w-10, 40).
  { cssClass: 'trail-fold-3', slots: ['reopen', 'deploy'], width: 76, count: 2 },
]

function bandOf(rowWidth: number): 'narrow' | 'mid' | 'wide' {
  if (rowWidth >= 1300) return 'wide'
  if (rowWidth >= 1101) return 'mid'
  return 'narrow'
}

/** Width + slot count of the always-on strip at `rowWidth` with the first
 *  `hiddenLevels` levels folded away. `stale` is always in it. */
function alwaysOnStrip(rowWidth: number, hiddenLevels: number): { w: number; n: number } {
  const band = bandOf(rowWidth)
  let w = band === 'narrow' ? STALE_WIDTH_BY_BAND.narrow : STALE_WIDTH_BY_BAND.wide
  let n = 1
  for (let i = hiddenLevels; i < ALWAYS_ON_LEVELS.length; i++) {
    const level = ALWAYS_ON_LEVELS[i]
    n += level.count
    w +=
      level.cssClass === 'trail-fold-2'
        ? band === 'narrow'
          ? LABELS_WIDTH_BY_BAND.narrow
          : band === 'mid'
            ? LABELS_WIDTH_BY_BAND.mid
            : LABELS_WIDTH_BY_BAND.wide
        : level.width
  }
  return { w, n }
}

/** Title width the model predicts at `rowWidth` with `hiddenLevels` folded,
 *  given the option columns whose rung lets them paint at that width. */
function modeledTitle(
  rowWidth: number,
  hiddenLevels: number,
  optionLadder: readonly TrailBreakRung[],
): number {
  const strip = alwaysOnStrip(rowWidth, hiddenLevels)
  let w = strip.w
  let n = strip.n
  for (const rung of optionLadder) {
    if (rung.rung <= rowWidth) {
      w += SLOT_WIDTH[rung.col]
      n += 1
    }
  }
  return rowWidth - ROW_OVERHEAD - w - GAP * Math.max(0, n - 1)
}

/** Row widths the model is evaluated at. 260 is under the narrowest row the
 *  app makes (the 358px three-pane row GDK-1089 measured, the 410px docked
 *  one that replaced it); 1400 is above the 1360 layout cap. */
const SCAN_LO = 260
const SCAN_HI = 1400

/**
 * Which always-on levels are hidden at which row widths, for an enabled
 * option set.
 *
 * One rule, at every width: a level is hidden where the title, with the
 * levels before it already folded, would be under TITLE_COMFORT_PX.
 *
 * Deliberately not conditioned on the fold being ENOUGH. With the full
 * option catalog on a dense row there are widths where no amount of folding
 * buys a 293px title, and an earlier draft kept the strip there on the
 * grounds that folding furniture that cannot win is just a smaller row. It
 * is the wrong rule: it also stopped folding on the narrowest rows the app
 * makes (under 549px nothing could reach the floor, so nothing folded and
 * the title sat on its 13ch hard floor with the whole strip beside it) —
 * strictly worse than the static rungs this replaced. Closer to the floor
 * is better than further from it, at every width.
 *
 * The result is BANDS, not one max-width rung, and that is the difference
 * that made this worth generating. An option column joining the row at its
 * own rung can take the title back under the floor for a stretch — epic
 * paints from 750 and costs 74px, so with the default set the floor is unmet
 * in 750–792 even with `labels` folded. A single rung has to choose between
 * leaving that hole open (the floor is not a floor) and folding `labels` at
 * every width below 793 (it would be gone at a 1000px window, where the row
 * is 728 and the floor is met with room to spare). Bands say the true thing
 * at every width: `labels` is hidden below 719 AND in 750–792, and paints in
 * between and above.
 *
 * Nesting is automatic: if level k is hidden at a width then so is every
 * level before it (the title only gets narrower with fewer levels folded),
 * so the bands cannot describe a row that drops `labels` while keeping
 * `updated`.
 */
export function alwaysOnFoldLadder(enabled: readonly string[]): AlwaysOnFold[] {
  const optionLadder = trailBreakLadder(enabled)
  return ALWAYS_ON_LEVELS.map((level, k) => {
    const bands: { lo: number; hi: number }[] = []
    let open: { lo: number; hi: number } | null = null
    for (let w = SCAN_LO; w <= SCAN_HI; w++) {
      const hide = modeledTitle(w, k, optionLadder) < TITLE_COMFORT_PX
      if (hide) {
        if (open) open.hi = w
        else open = { lo: w === SCAN_LO ? 0 : w, hi: w }
      } else if (open) {
        bands.push(open)
        open = null
      }
    }
    if (open) bands.push(open)
    return { cssClass: level.cssClass, slots: level.slots, bands }
  })
}

/**
 * The <style> text for the always-on ladder — one @container issuerow rule
 * per band, in the same @layer utilities and for the same GDK-766 reason as
 * the option rungs (the display:none must outrank the slots' `flex`
 * utility). A band that starts at 0 is written max-width only, so a browser
 * without container-query support lands on the strip-visible step.
 */
export function alwaysOnFoldCss(enabled: readonly string[]): string {
  const folds = alwaysOnFoldLadder(enabled)
  const rules: string[] = []
  for (const fold of folds) {
    for (const band of fold.bands) {
      const query =
        band.lo === 0
          ? `(max-width: ${band.hi}px)`
          : `(min-width: ${band.lo}px) and (max-width: ${band.hi}px)`
      rules.push(
        `  /* ${fold.slots.join(' + ')} */\n  @container issuerow ${query} {\n    .${fold.cssClass} {\n      display: none;\n    }\n  }`,
      )
    }
  }
  if (rules.length === 0) return ''
  return [
    `@layer utilities {`,
    `  /* always-on fold bands, priced against the title's ${TITLE_COMFORT_PX}px comfort`,
    `     floor (GDK-1791) — generated, single owner:`,
    `     web/src/components/list/row-column-thresholds.ts */`,
    ...rules,
    `}`,
    ``,
  ].join('\n')
}

/** Both ladders, in one <style>: the option breaks and the always-on folds.
 *  IssueList.svelte injects this. */
export function rowFoldCss(enabled: readonly string[]): string {
  return trailBreakCss(enabled) + alwaysOnFoldCss(enabled)
}
