/*
 * Personal history helpers — visit/search timeline aggregation and date groups.
 *
 * Shapes match GET history/ (internal/server/history.go). Counts are derived
 * from the loaded event rows the same way reopen_count is derived from
 * changelog: the server stores append-only events, the view collapses them.
 */

import type { HistoryItem, HistoryVisitKind } from './types'

/** Skip a *consecutive* server visit for the same (kind, key) inside this window.
 *  2s: spec 002-history said start at opening and consider 1–2s to absorb
 *  remount / $effect re-runs. A different key in between is a new visit. */
export const VISIT_DEBOUNCE_MS = 2000

/** Same query posted twice inside this window (palette flush + list Enter
 *  from "see more") is one search row. */
export const SEARCH_DEDUPE_MS = 3000

export type HistoryKindFilter = '' | 'issue' | 'page' | 'search'

export type DateGroup = 'today' | 'yesterday' | 'week' | 'older'

export interface AggregatedVisit {
  type: 'visit'
  kind: HistoryVisitKind
  key: string
  count: number
  at: string
}

export interface HistorySearchRow {
  type: 'search'
  id: number
  query: string
  resultCount: number | null
  openedKind: string | null
  openedKey: string | null
  at: string
}

export type TimelineEntry = AggregatedVisit | HistorySearchRow

export function visitId(kind: HistoryVisitKind, key: string): string {
  return `${kind}:${key}`
}

// Exported for feed-days.ts (the feed's day sections): it needs the same
// day-start arithmetic for its 2..6-day weekday window, and a second copy
// of the concept beside dateGroup is how the two drift.
export function startOfLocalDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

/** Calendar buckets the History view groups by. Monday starts the week. */
export function dateGroup(iso: string, now: Date = new Date()): DateGroup {
  const parsed = new Date(iso)
  if (Number.isNaN(parsed.getTime())) return 'older'
  const today = startOfLocalDay(now)
  const target = startOfLocalDay(parsed)
  const diffDays = Math.round((today.getTime() - target.getTime()) / 86_400_000)
  if (diffDays <= 0) return 'today'
  if (diffDays === 1) return 'yesterday'
  const dow = today.getDay()
  const mondayOffset = dow === 0 ? 6 : dow - 1
  const weekStart = new Date(today)
  weekStart.setDate(today.getDate() - mondayOffset)
  if (target >= weekStart) return 'week'
  return 'older'
}

/**
 * Collapse visit events of the same (kind, key) into one row with a count;
 * leave each search as its own row. Input is newest-first (server order).
 */
export function aggregateHistory(items: readonly HistoryItem[]): TimelineEntry[] {
  const visitCounts = new Map<string, number>()
  for (const it of items) {
    if (it.type !== 'visit' || !it.kind || !it.key) continue
    if (it.kind !== 'issue' && it.kind !== 'page') continue
    const id = visitId(it.kind, it.key)
    visitCounts.set(id, (visitCounts.get(id) ?? 0) + 1)
  }
  const seen = new Set<string>()
  const out: TimelineEntry[] = []
  for (const it of items) {
    if (it.type === 'search') {
      out.push({
        type: 'search',
        id: it.id,
        query: it.query ?? '',
        resultCount: it.result_count ?? null,
        openedKind: it.opened_kind ?? null,
        openedKey: it.opened_key ?? null,
        at: it.at,
      })
      continue
    }
    if (it.type !== 'visit' || !it.kind || !it.key) continue
    if (it.kind !== 'issue' && it.kind !== 'page') continue
    const id = visitId(it.kind, it.key)
    if (seen.has(id)) continue
    seen.add(id)
    out.push({
      type: 'visit',
      kind: it.kind,
      key: it.key,
      count: visitCounts.get(id) ?? 1,
      at: it.at,
    })
  }
  return out
}

export function issueKeysOf(entries: readonly TimelineEntry[]): string[] {
  const keys: string[] = []
  const seen = new Set<string>()
  for (const e of entries) {
    if (e.type !== 'visit' || e.kind !== 'issue') continue
    if (seen.has(e.key)) continue
    seen.add(e.key)
    keys.push(e.key)
  }
  return keys
}