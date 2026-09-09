/*
 * The retro materials' arithmetic (GDK-1721..1725).
 *
 * FAIL-first: every case here was written against the geometry the sections
 * needed and failed on an empty module. What they pin is the part a
 * screenshot cannot check — that the p85 line lands where the bars say it
 * does, that an empty Tuesday is still a column, that a nine-month outlier
 * is clamped and counted rather than allowed to flatten the sample, and that
 * a translated sentence keeps its own word order.
 */
import { describe, expect, it } from 'vitest'
import {
  agingChart,
  cycleScatter,
  densityStrip,
  hasMaterials,
  percentile,
  setCount,
  splitTemplate,
} from './materials'
import type { RetroBucket } from '../../lib/types'

describe('percentile', () => {
  it('is nearest-rank, the ladder the report itself uses', () => {
    const xs = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
    expect(percentile(xs, 0.5)).toBe(5)
    expect(percentile(xs, 0.85)).toBe(9)
    expect(percentile(xs, 1)).toBe(10)
  })
  it('has no value for an empty sample', () => {
    expect(percentile([], 0.5)).toBeNull()
  })
  it('sorts its input rather than trusting the caller', () => {
    expect(percentile([9, 1, 5], 0.5)).toBe(5)
  })
})

describe('agingChart', () => {
  const items = [
    { key: 'A-1', days: 10 },
    { key: 'A-2', days: 40 },
    { key: 'A-3', days: 20 },
  ]

  it('sorts by age and scales to the widest bar, not to p85', () => {
    const c = agingChart(items, 30)
    expect(c.bars.map((b) => b.key)).toEqual(['A-2', 'A-3', 'A-1'])
    expect(c.bars[0].width).toBe(1)
    expect(c.bars[2].width).toBeCloseTo(0.25)
    // The line sits inside the box because the scale is the maximum.
    expect(c.p85At).toBeCloseTo(0.75)
  })

  it('marks only the bars past the line', () => {
    const c = agingChart(items, 30)
    expect(c.bars.map((b) => b.over)).toEqual([true, false, false])
  })

  it('cuts to the top N and says how many are left', () => {
    const many = Array.from({ length: 45 }, (_, i) => ({ key: `A-${i}`, days: i + 1 }))
    const c = agingChart(many, null, 30)
    expect(c.bars).toHaveLength(30)
    expect(c.more).toBe(15)
    // No p85 means no line, and the bars still have widths.
    expect(c.p85At).toBeNull()
    expect(c.bars[0].width).toBe(1)
  })

  it('is empty without dividing by zero when nothing is in progress', () => {
    const c = agingChart([], null)
    expect(c.bars).toEqual([])
    expect(c.more).toBe(0)
    expect(c.p85At).toBeNull()
  })
})

describe('densityStrip', () => {
  const now = new Date('2026-03-10T12:00:00Z')

  it('keeps a column for every day, including the empty ones', () => {
    const s = densityStrip(
      [
        { at: '2026-03-02T09:00:00Z', key: 'A-1', kind: 'started' },
        { at: '2026-03-04T09:00:00Z', key: 'A-2', kind: 'resolved' },
        { at: '2026-03-04T11:00:00Z', key: 'A-3', kind: 'comment' },
      ],
      '2026-03-02T00:00:00Z',
      '2026-03-09T00:00:00Z',
      now,
    )
    expect(s).toHaveLength(7)
    expect(s.map((d) => d.total)).toEqual([1, 0, 2, 0, 0, 0, 0])
    expect(s[0].started).toBe(1)
    expect(s[2].resolved).toBe(1)
    expect(s[2].other).toBe(1)
    // Heights are relative to the busiest day, so the shape survives a
    // window where nothing much happened.
    expect(s[2].height).toBe(1)
    expect(s[0].height).toBeCloseTo(0.5)
  })

  it('stops at today rather than drawing a running bucket into the future', () => {
    const s = densityStrip([], '2026-03-09T00:00:00Z', '2026-03-16T00:00:00Z', now)
    expect(s.map((d) => d.day)).toEqual(['2026-03-09', '2026-03-10'])
  })

  it('drops an event that falls outside the window instead of inventing a column', () => {
    const s = densityStrip(
      [{ at: '2026-02-01T09:00:00Z', key: 'A-1', kind: 'started' }],
      '2026-03-02T00:00:00Z',
      '2026-03-05T00:00:00Z',
      now,
    )
    expect(s).toHaveLength(3)
    expect(s.every((d) => d.total === 0)).toBe(true)
  })

  it('has nothing to draw for an inverted window', () => {
    expect(densityStrip([], '2026-03-09T00:00:00Z', '2026-03-02T00:00:00Z', now)).toEqual([])
  })
})

describe('cycleScatter', () => {
  const from = '2026-03-02T00:00:00Z'
  const to = '2026-03-09T00:00:00Z'

  it('carries each point its title, so the dot can name itself (GDK-1737)', () => {
    const s = cycleScatter(
      [
        { key: 'A-1', summary: 'Search relevance regressed on empty queries', resolved_at: from, days: 1 },
        { key: 'A-2', resolved_at: to, days: 2 },
      ],
      from,
      to,
    )
    expect(s.points[0].summary).toBe('Search relevance regressed on empty queries')
    // An older server sends no title, and an empty string is the honest
    // stand-in rather than the word "undefined" inside a tooltip.
    expect(s.points[1].summary).toBe('')
  })

  it('places a point by when it resolved and how long it took', () => {
    const s = cycleScatter(
      [
        { key: 'A-1', resolved_at: '2026-03-02T00:00:00Z', days: 1 },
        { key: 'A-2', resolved_at: '2026-03-09T00:00:00Z', days: 2 },
      ],
      from,
      to,
    )
    expect(s.points[0].x).toBe(0)
    expect(s.points[1].x).toBe(1)
    expect(s.p50).toBe(1)
    expect(s.p85).toBe(2)
  })

  it('clamps an outlier to the top and counts it rather than flattening the sample', () => {
    const days = [1, 1, 2, 2, 3, 300]
    const s = cycleScatter(
      days.map((d, i) => ({ key: `A-${i}`, resolved_at: from, days: d })),
      from,
      to,
    )
    // p85 of six values is the 6th by nearest rank — the outlier itself — so
    // the axis top is min(max, p85 * 1.5) and nothing is clipped here.
    expect(s.yMax).toBe(300)
    expect(s.clipped).toBe(0)
    // With the outlier out of the p85 band the axis stops well below it.
    const s2 = cycleScatter(
      [1, 1, 1, 1, 1, 1, 1, 1, 2, 300].map((d, i) => ({
        key: `B-${i}`,
        resolved_at: from,
        days: d,
      })),
      from,
      to,
    )
    expect(s2.p85).toBe(2)
    expect(s2.yMax).toBe(3)
    expect(s2.clipped).toBe(1)
    expect(s2.points[9].y).toBe(1)
  })

  it('is drawable with one point and no span', () => {
    const s = cycleScatter([{ key: 'A-1', resolved_at: from, days: 0 }], from, from)
    expect(s.yMax).toBe(1)
    expect(s.points[0].y).toBe(0)
  })
})

describe('hasMaterials', () => {
  const base = { from: '', to: '', partial: false } as unknown as RetroBucket

  it('is false for the bucket an older server sends', () => {
    expect(hasMaterials(base)).toBe(false)
    expect(hasMaterials(undefined)).toBe(false)
  })

  it('is true for an empty-but-present field — nothing happened is an answer', () => {
    expect(hasMaterials({ ...base, events: [] })).toBe(true)
    expect(hasMaterials({ ...base, unplanned: { keys: [] } })).toBe(true)
  })
})

describe('setCount', () => {
  it('prefers the server count and falls back to the keys it sent', () => {
    expect(setCount({ count: 7, keys: ['A-1'] })).toBe(7)
    expect(setCount({ keys: ['A-1', 'A-2'] })).toBe(2)
    expect(setCount(undefined)).toBe(0)
  })
})

describe('splitTemplate', () => {
  const slots = ['closed', 'unplanned', 'reopened', 'age']

  it('keeps the translation’s own word order', () => {
    const en = splitTemplate('Closed {closed}, reopened {reopened}.', slots)
    expect(en).toEqual([
      { text: 'Closed ' },
      { slot: 'closed' },
      { text: ', reopened ' },
      { slot: 'reopened' },
      { text: '.' },
    ])
    const ko = splitTemplate('되돌아온 것 {reopened}, 닫힌 것 {closed}.', slots)
    expect(ko.filter((p) => 'slot' in p)).toEqual([{ slot: 'reopened' }, { slot: 'closed' }])
  })

  it('leaves an unknown placeholder visible instead of deleting the clause', () => {
    expect(splitTemplate('a {nope} b', slots)).toEqual([{ text: 'a {nope} b' }])
  })
})
