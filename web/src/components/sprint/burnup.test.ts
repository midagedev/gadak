/*
 * The burn-up geometry's numeric contract (GDK-1752) — the spec's numbers,
 * each with the failure it exists to catch.
 *
 * The module is pure on purpose (see burnup.ts): everything below reads
 * coordinates, not rendered output, so a geometry regression fails here in
 * milliseconds instead of in a screenshot a person has to squint at.
 */
import { describe, expect, test } from 'vitest'
import { BURNUP_H, BURNUP_PAD, BURNUP_W, burnupGeometry } from './burnup'
import type { BurnupDay } from '../../lib/types'

/** The end-dot's full extent: radius 4 + 2px surface ring + half the 2px
 *  stroke it terminates — 7. Written as a literal, not BURNUP_PAD, so the
 *  assertion keeps measuring the mark even if someone retunes the inset and
 *  forgets the mark it was inset for. */
const END_DOT_EXTENT = 7

const day = (date: string, scope: number, completed: number): BurnupDay => ({
  date,
  scope,
  started: completed,
  completed,
})

describe('burnupGeometry (GDK-1752)', () => {
  test('the box is the contract: 200×36', () => {
    expect(BURNUP_W).toBe(200)
    expect(BURNUP_H).toBe(36)
  })

  test('no days is the caller’s empty state, not an empty chart', () => {
    expect(burnupGeometry([])).toBeNull()
  })

  test('two lines share one ceiling: completed ≤ scope reads as below-or-equal on screen', () => {
    const days = [
      day('2026-08-03', 5, 0),
      day('2026-08-04', 5, 2),
      day('2026-08-06', 6, 4),
      day('2026-08-07', 8, 8),
    ]
    const geo = burnupGeometry(days)
    expect(geo).not.toBeNull()
    // The ceiling is the largest scope, not each line's own maximum: a
    // per-line scale would make 6-of-8 look like "caught up".
    expect(geo!.max).toBe(8)
    for (let i = 0; i < days.length; i++) {
      expect(geo!.done[i].y).toBeGreaterThanOrEqual(geo!.scope[i].y)
    }
    // Rising values move up the shared axis (smaller y), monotonically here.
    expect(geo!.done[3].y).toBeLessThan(geo!.done[0].y)
  })

  test('every mark sits where the end-dot’s ring cannot clip', () => {
    const days = [
      day('2026-08-03', 9, 0),
      day('2026-08-04', 9, 3),
      day('2026-08-05', 9, 9),
    ]
    const geo = burnupGeometry(days)!
    for (const p of [...geo.scope, ...geo.done]) {
      expect(p.x).toBeGreaterThanOrEqual(END_DOT_EXTENT)
      expect(p.x).toBeLessThanOrEqual(BURNUP_W - END_DOT_EXTENT)
      expect(p.y).toBeGreaterThanOrEqual(END_DOT_EXTENT)
      expect(p.y).toBeLessThanOrEqual(BURNUP_H - END_DOT_EXTENT)
    }
    // x advances strictly with the day index — a redrawn series that
    // backtracks would zig-zag across days.
    for (let i = 1; i < geo.scope.length; i++) {
      expect(geo.scope[i].x).toBeGreaterThan(geo.scope[i - 1].x)
    }
    // The extremes land exactly on the inset, not past it.
    expect(geo.scope[0].x).toBe(BURNUP_PAD)
    expect(geo.scope.at(-1)!.x).toBe(BURNUP_W - BURNUP_PAD)
    expect(geo.scope[0].y).toBe(BURNUP_PAD) // scope 9 = the ceiling, top of the plot
    expect(geo.done[0].y).toBe(BURNUP_H - BURNUP_PAD) // completed 0 = baseline
  })

  test('an all-zero series is a flat baseline, not NaN', () => {
    const geo = burnupGeometry([day('2026-08-03', 0, 0), day('2026-08-04', 0, 0)])!
    expect(geo.max).toBe(1)
    for (const p of [...geo.scope, ...geo.done]) {
      expect(Number.isFinite(p.x)).toBe(true)
      expect(Number.isFinite(p.y)).toBe(true)
    }
    expect(geo.done[0].y).toBe(BURNUP_H - BURNUP_PAD)
  })

  test('one day is dots, not a line — both paths empty, one centred x', () => {
    const geo = burnupGeometry([day('2026-08-03', 4, 1)])!
    expect(geo.scopePath).toBe('')
    expect(geo.donePath).toBe('')
    expect(geo.scope[0].x).toBe(BURNUP_W / 2)
    expect(geo.endDot).toEqual({ x: BURNUP_W / 2, y: geo.done[0].y })
  })

  test('paths are M-prefixed polyline strings over every day', () => {
    const days = [day('2026-08-03', 3, 0), day('2026-08-04', 3, 1), day('2026-08-05', 4, 4)]
    const geo = burnupGeometry(days)!
    for (const d of [geo.scopePath, geo.donePath]) {
      expect(d.startsWith('M')).toBe(true)
      // 3 points → M + 2 L joins; one coordinate pair per point.
      expect(d.match(/L/g)?.length).toBe(days.length - 1)
      expect(d.match(/-?\d+(?:\.\d+)?,-?\d+(?:\.\d+)?/g)?.length).toBe(days.length)
    }
  })
})
