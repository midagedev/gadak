/*
 * The retro materials, as pure functions (GDK-1721..1725).
 *
 * The report used to be one table, and a table answers "what is the number"
 * while a retro is a conversation about what happened. The research behind
 * this round named four inputs a person actually retrospects on — a
 * timeline, a delta, a surprise, and what was decided last time — and the
 * table was only the second. The sections that carry the other three are
 * drawings, so the arithmetic that turns rows into marks lives here rather
 * than inside six components: a bar's width, a day column's height, a
 * scatter point's place, and the percentile lines both of them need.
 *
 * Every function takes plain data and returns plain data. Nothing here
 * reaches for a store, a locale or the DOM, so the unit tests are the FAIL
 * -first for the geometry and the browser only has to confirm it rendered.
 */

import type { RetroAgingItem, RetroBucket, RetroCyclePoint, RetroEvent } from '../../lib/types'

/** Nearest-rank percentile, the ladder the report itself uses. `p` is 0..1. */
export function percentile(values: number[], p: number): number | null {
  const xs = values.filter((v) => Number.isFinite(v)).sort((a, b) => a - b)
  if (!xs.length) return null
  const rank = Math.ceil(p * xs.length)
  return xs[Math.min(Math.max(rank, 1), xs.length) - 1]
}

/** One bar on the aging chart. `width` and `p85At` are fractions of the box. */
export interface AgingBar {
  key: string
  days: number
  summary: string
  /** 0..1 of the widest bar. */
  width: number
  /** Past the p85 line — the ones next week starts with. */
  over: boolean
}

export interface AgingChart {
  bars: AgingBar[]
  /** How many items the top-N cut left behind. */
  more: number
  /** Where the p85 line sits, 0..1, or null when there is no p85. */
  p85At: number | null
  p85Days: number | null
}

/**
 * The aging chart's geometry.
 *
 * Sorted by days desc and cut to `limit`, because thirty bars is a shape and
 * three hundred is a wall. The scale is the widest *item*, not the p85, so
 * the line stays inside the box and the longest bar always reaches the edge.
 */
export function agingChart(
  items: readonly RetroAgingItem[],
  p85Days: number | null | undefined,
  limit = 30,
): AgingChart {
  const sorted = [...items].sort((a, b) => b.days - a.days)
  const max = sorted.length ? Math.max(...sorted.map((i) => i.days)) : 0
  const shown = sorted.slice(0, limit)
  const p85 = p85Days ?? null
  return {
    bars: shown.map((i) => ({
      key: i.key,
      days: i.days,
      summary: i.summary ?? '',
      width: max > 0 ? i.days / max : 0,
      over: p85 != null && i.days > p85,
    })),
    more: Math.max(sorted.length - shown.length, 0),
    p85At: p85 != null && max > 0 ? Math.min(p85 / max, 1) : null,
    p85Days: p85,
  }
}

/** One day column of the density strip. */
export interface DensityDay {
  /** The day's midnight in UTC, as `YYYY-MM-DD` — the column's identity. */
  day: string
  started: number
  resolved: number
  other: number
  total: number
  /** 0..1 of the busiest day in the strip. */
  height: number
}

function dayKey(at: string): string {
  return at.slice(0, 10)
}

/**
 * The bucket's events as one column per day.
 *
 * Every day between `from` and `to` gets a column, including the empty ones:
 * the gaps are the point. A strip drawn only over the days that had events
 * says "steady" about a week where everything landed on the Friday.
 */
export function densityStrip(
  events: readonly RetroEvent[],
  from: string,
  to: string,
  now = new Date(),
): DensityDay[] {
  const start = Date.parse(from.slice(0, 10) + 'T00:00:00Z')
  // `to` is exclusive, and a running bucket's `to` is in the future — the
  // strip would then be mostly a stretch of days that have not happened.
  // Midnight after today, in the same UTC day grid the columns are cut on —
  // `now + 24h` would let tomorrow's half-column in whenever the report is
  // read in the afternoon.
  const tomorrow = Date.parse(now.toISOString().slice(0, 10) + 'T00:00:00Z') + 86_400_000
  const end = Math.min(Date.parse(to.slice(0, 10) + 'T00:00:00Z'), tomorrow)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return []
  const days: DensityDay[] = []
  const index = new Map<string, DensityDay>()
  for (let t = start; t < end; t += 86_400_000) {
    const d: DensityDay = {
      day: new Date(t).toISOString().slice(0, 10),
      started: 0,
      resolved: 0,
      other: 0,
      total: 0,
      height: 0,
    }
    days.push(d)
    index.set(d.day, d)
    // A window wider than a year is not a retro; stop rather than spin.
    if (days.length > 400) break
  }
  for (const e of events) {
    const d = index.get(dayKey(e.at))
    if (!d) continue
    if (e.kind === 'started') d.started++
    else if (e.kind === 'resolved') d.resolved++
    else d.other++
    d.total++
  }
  const max = Math.max(...days.map((d) => d.total), 0)
  if (max > 0) for (const d of days) d.height = d.total / max
  return days
}

/** One dot on the cycle-time scatter. `x` and `y` are 0..1 of the box. */
export interface ScatterPoint {
  key: string
  /** The issue's title, so the dot can name itself (GDK-1737). */
  summary: string
  days: number
  x: number
  y: number
}

export interface Scatter {
  points: ScatterPoint[]
  /** The y axis top, in days. */
  yMax: number
  p50: number | null
  p85: number | null
  /** Where the two lines sit, 0..1 from the bottom. */
  p50At: number | null
  p85At: number | null
  /** How many points sit above the axis top and are drawn clamped to it. */
  clipped: number
}

/**
 * The scatter's geometry.
 *
 * The axis stops at p85 × 1.5 rather than the maximum, so one nine-month
 * straggler cannot squash the whole sample into the bottom pixel row. The
 * points above it are drawn on the top edge and counted, never dropped —
 * "three above the top" is itself a thing to talk about.
 */
export function cycleScatter(
  points: readonly RetroCyclePoint[],
  from: string,
  to: string,
): Scatter {
  const days = points.map((p) => p.days)
  const p50 = percentile(days, 0.5)
  const p85 = percentile(days, 0.85)
  const max = days.length ? Math.max(...days) : 0
  const top = p85 != null && p85 > 0 ? Math.min(max, p85 * 1.5) : max
  const yMax = top > 0 ? top : 1
  const t0 = Date.parse(from)
  const t1 = Date.parse(to)
  const span = t1 > t0 ? t1 - t0 : 1
  let clipped = 0
  const out = points.map((p) => {
    if (p.days > yMax) clipped++
    const t = Date.parse(p.resolved_at)
    const x = Number.isFinite(t) ? Math.min(Math.max((t - t0) / span, 0), 1) : 0
    return { key: p.key, summary: p.summary ?? '', days: p.days, x, y: Math.min(p.days / yMax, 1) }
  })
  return {
    points: out,
    yMax,
    p50,
    p85,
    p50At: p50 != null ? Math.min(p50 / yMax, 1) : null,
    p85At: p85 != null ? Math.min(p85 / yMax, 1) : null,
    clipped,
  }
}

/**
 * Whether a bucket carries anything the new sections can draw.
 *
 * One predicate rather than six `#if`s: an older server sends none of these
 * fields, and the whole materials half of the screen is then absent rather
 * than present-and-empty.
 */
export function hasMaterials(b: RetroBucket | undefined): boolean {
  if (!b) return false
  return Boolean(
    b.events ??
      b.surprises ??
      b.closed_by_type ??
      b.closed_by_epic ??
      b.unplanned ??
      b.cycle_points ??
      b.seen_not_moved ??
      b.moved_not_seen,
  )
}

/** A key set's count — the server's own when it sent one, else the keys. */
export function setCount(s: { count?: number; keys?: string[] } | undefined): number {
  if (!s) return 0
  return s.count ?? s.keys?.length ?? 0
}

/** A piece of a filled sentence: literal text, or a named slot. */
export type SentencePiece = { text: string } | { slot: string }

/**
 * A catalog template cut at its `{name}` placeholders.
 *
 * The summary sentence (GDK-1724) is prose whose numbers are doors, so it
 * cannot be a `t()` call with the values substituted in — the value has to
 * survive as its own element. Splitting the translated string keeps word
 * order in the translator's hands: Korean and Japanese put the same four
 * numbers in different places, and neither is the English order.
 *
 * Unknown placeholders are left as literal text rather than dropped, so a
 * typo in a catalog entry shows itself instead of silently deleting a clause.
 */
export function splitTemplate(template: string, slots: readonly string[]): SentencePiece[] {
  const out: SentencePiece[] = []
  const re = /\{([a-zA-Z0-9_]+)\}/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(template))) {
    if (!slots.includes(m[1])) continue
    if (m.index > last) out.push({ text: template.slice(last, m.index) })
    out.push({ slot: m[1] })
    last = m.index + m[0].length
  }
  if (last < template.length) out.push({ text: template.slice(last) })
  return out
}
