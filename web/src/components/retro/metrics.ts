/*
 * The retro's metric vocabulary, in one place (GDK-1712).
 *
 * The table, the summary strip and the sparklines are three readings of the
 * same eight rows, so the unit ladder and the "which way is better" rule
 * live here rather than three times over. The formatting ladder is the CLI's
 * (`retro.FormatDays`, mirrored in RetroView since GDK-1683) — the two
 * surfaces print the same number.
 */

import type { RetroBucket } from '../../lib/types'

export type Unit = 'count' | 'seconds' | 'days'

/**
 * Which way is better, for the metrics where that is defined.
 *
 * Deliberately not defined for sessions, resume, in progress and mismatch.
 * "More sessions" is not progress, and a team that reads its own retro as a
 * scoreboard is the failure mode the coaching grammar exists to avoid
 * (THEORY.md G9: progress, not score). Colour is reserved for the four
 * numbers whose direction a team has actually agreed on.
 */
export type Direction = 'up-good' | 'down-good' | 'neutral'

/** Tone for a delta chip; 'none' means print it without colour. */
export type Tone = 'good' | 'bad' | 'none'

/** The CLI's own ladder for a day-valued number (GDK-1683). */
export function formatDays(v: number): string {
  if (v >= 1) return `${v.toFixed(1)}d`
  const h = v * 24
  if (h >= 1) return `${h.toFixed(1)}h`
  return `${Math.round(h * 60)}m`
}

export function formatSeconds(v: number): string {
  if (v < 60) return `${Math.round(v)}s`
  if (v < 3600) return `${Math.round(v / 60)}m`
  return `${(v / 3600).toFixed(1)}h`
}

/** A cell's text. `—` is the empty cell the CLI prints. */
export function formatValue(v: number | null | undefined, unit: Unit): string {
  if (v == null) return '—'
  if (unit === 'count') return String(v)
  if (unit === 'seconds') return formatSeconds(v)
  return formatDays(v)
}

/** The signed difference, in the metric's own unit. */
export function formatDelta(diff: number, unit: Unit): string {
  // U+2212 MINUS SIGN, not a hyphen: it is the same width as the plus beside
  // it, so a column of deltas does not jitter.
  const sign = diff > 0 ? '+' : '−'
  const mag = Math.abs(diff)
  if (unit === 'count') return `${sign}${Math.round(mag)}`
  if (unit === 'seconds') return `${sign}${formatSeconds(mag)}`
  return `${sign}${formatDays(mag)}`
}

export interface Delta {
  text: string
  tone: Tone
  /** ↑ or ↓ — the glyph, so the direction survives a greyscale print. */
  glyph: string
}

/**
 * Bucket-over-bucket change, or null when there is nothing to compare.
 *
 * `partial` suppresses colour: the running bucket's number is not a final
 * one, and a green "closed +4" three days into a sprint says the sprint went
 * well when what happened is that it is not over. The figure is still shown
 * — it is real — but it is not scored.
 */
export function deltaOf(
  cur: number | null | undefined,
  prev: number | null | undefined,
  unit: Unit,
  direction: Direction,
  partial: boolean,
): Delta | null {
  if (cur == null || prev == null) return null
  const diff = cur - prev
  if (diff === 0) return null
  const good = direction === 'neutral' ? null : direction === 'up-good' ? diff > 0 : diff < 0
  return {
    text: formatDelta(diff, unit),
    tone: partial || good === null ? 'none' : good ? 'good' : 'bad',
    glyph: diff > 0 ? '↑' : '↓',
  }
}

/** The row set, in the order the report prints it. */
export interface MetricSpec {
  key: keyof RetroBucket
  unit: Unit
  direction: Direction
  /** Which key array opens the cell, if any. */
  keys?: 'closed' | 'in progress' | 'mismatch' | 'cycle'
}

export const METRIC_SPECS: MetricSpec[] = [
  { key: 'sessions', unit: 'count', direction: 'neutral' },
  { key: 'resume (median)', unit: 'seconds', direction: 'neutral' },
  { key: 'closed', unit: 'count', direction: 'up-good', keys: 'closed' },
  { key: 'cycle p50', unit: 'days', direction: 'down-good', keys: 'cycle' },
  { key: 'cycle p85', unit: 'days', direction: 'down-good', keys: 'cycle' },
  { key: 'in progress', unit: 'count', direction: 'neutral', keys: 'in progress' },
  { key: 'wip age max', unit: 'days', direction: 'down-good' },
  { key: 'mismatch', unit: 'count', direction: 'neutral', keys: 'mismatch' },
]

/** The four the summary strip carries, in reading order. */
export const SUMMARY_KEYS = ['closed', 'cycle p85', 'in progress', 'wip age max'] as const

/** Tailwind classes for a tone. Existing status tokens, no new ones: done is
 *  the app's "this went well" green and stale its amber. Red is deliberately
 *  absent (GDK-1336) — a retro has no failures to flag in red. */
export const TONE_CLASS: Record<Tone, string> = {
  good: 'text-status-done',
  bad: 'text-status-stale',
  none: 'text-text-muted',
}
