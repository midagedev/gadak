/*
 * The burn-up spark's geometry, pure (GDK-1752).
 *
 * The component owns no math of its own: every coordinate a browser would
 * have to agree on is decided here, where a unit test can read it. The
 * numeric contract is the spec's — a 200×36 viewBox (60 in the first cut;
 * the vision pass on 2026-09-11 read a 60px band as 2.5× the strip's other
 * rows with the ink in the middle third, and named height as the one axis), two lines (scope and
 * completed) sharing ONE y-axis, and geometry that never puts a mark where
 * the end-dot's surface ring would clip at the box edge.
 *
 * Scale decisions, each with a reason:
 *   - one shared y-max over both series (never one per line): a scope of 8
 *     beside a completed of 6 only reads as "6 of 8" if both are measured
 *     against the same ceiling. Dual scales are the first thing a chart
 *     reviewer strikes.
 *   - max clamped to ≥1: an all-zero series (a sprint scoped but nothing
 *     done) has no range, and `0/0` is NaN — the flat-baseline answer is
 *     y(0) at the plot's bottom, not a divide-by-zero.
 *   - x over day INDEX, not date arithmetic: the store already buckets to
 *     one row per calendar day, so index is date with the gaps pre-closed.
 *     Re-deriving it from the date strings would only re-answer a question
 *     the series has answered.
 *   - one day → dots, not a line: a single point has no direction in it, so
 *     both paths come back empty and the caller renders the two dots.
 */

import type { BurnupDay } from '../../lib/types'

/** The contract: 200×36. Named so the component, the test and any future
 *  consumer quote one constant instead of re-typing the pair. */
export const BURNUP_W = 200
export const BURNUP_H = 36

/** Inset on every side. The largest mark that must fit is the completed
 *  line's end-dot: radius 4 plus its 2px surface ring plus half the 2px
 *  stroke it sits on — 7 total. A smaller pad clips the ring at the edge;
 *  a larger one spends plot the series does not need. */
export const BURNUP_PAD = 7

export interface BurnupPoint {
  x: number
  y: number
}

export interface BurnupGeometry {
  /** Scope per day, oldest first — same length as the input. */
  scope: BurnupPoint[]
  /** Completed per day, oldest first. */
  done: BurnupPoint[]
  /** SVG path per series, `''` when there are fewer than two days. */
  scopePath: string
  donePath: string
  /** The completed line's newest point — where the end-dot goes. `null` on
   *  an empty series (the caller already refused empty input). */
  endDot: BurnupPoint | null
  /** The shared y-ceiling the points were scaled against (≥1). */
  max: number
}

/** `days` → coordinates in the 200×36 viewBox. `null` when `days` is empty —
 *  an empty series is the caller's empty state (a sentence naming which of
 *  the two reasons it is), not a chart with no marks. */
export function burnupGeometry(days: BurnupDay[]): BurnupGeometry | null {
  if (days.length === 0) return null

  // One ceiling for both lines. Scope is the ceiling by the store's own
  // invariant (completed ≤ scope), but completed is read explicitly rather
  // than trusted: this module measures what it was given.
  let max = 1
  for (const d of days) {
    if (d.scope > max) max = d.scope
    if (d.completed > max) max = d.completed
  }

  const innerW = BURNUP_W - BURNUP_PAD * 2
  const innerH = BURNUP_H - BURNUP_PAD * 2
  // One day: a single centred x; the paths stay empty and the dots carry it.
  const stepX = days.length > 1 ? innerW / (days.length - 1) : 0
  const x = (i: number): number => BURNUP_PAD + (days.length > 1 ? i * stepX : innerW / 2)
  const y = (v: number): number => BURNUP_PAD + (1 - v / max) * innerH

  const scope = days.map((d, i) => ({ x: x(i), y: y(d.scope) }))
  const done = days.map((d, i) => ({ x: x(i), y: y(d.completed) }))

  const path = (pts: BurnupPoint[]): string =>
    pts.length > 1
      ? 'M' + pts.map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join('L')
      : ''

  return {
    scope,
    done,
    scopePath: path(scope),
    donePath: path(done),
    endDot: done.length ? done[done.length - 1] : null,
    max,
  }
}
