/*
 * Single owner of "which calendar day is this?".
 *
 * Two stored shapes:
 *  - instant — created_at / updated_at / resolved_at / status_changed_at /
 *    reopened_at / assignee_changed_at. UTC timestamps. Calendar day is the
 *    day in the given IANA zone (the viewer's local zone in the UI).
 *  - date — duedate. YYYY-MM-DD stored as written. Never Date.parse it
 *    (UTC midnight would print the previous day in the Americas).
 *
 * Must stay aligned with internal/calendar (same cases in both test files).
 */

export type DateKind = 'instant' | 'date'

export interface CalendarZone {
  id: string
}

export function localZone(): CalendarZone {
  try {
    return { id: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' }
  } catch {
    return { id: 'UTC' }
  }
}

export function utcZone(): CalendarZone {
  return { id: 'UTC' }
}

export function zoneNamed(id: string): CalendarZone {
  return { id }
}

const YMD = /^(\d{4}-\d{2}-\d{2})/

export interface CalendarDecision {
  raw: string
  kind: DateKind
  zone: string
  calendar_day: string
  ok: boolean
}

/** Report how calendarDay classified raw. */
export function explain(
  raw: string | null | undefined,
  kind: DateKind,
  zone: CalendarZone = localZone(),
): CalendarDecision {
  const day = calendarDay(raw, kind, zone)
  return {
    raw: raw ?? '',
    kind,
    zone: zone.id,
    calendar_day: day ?? '',
    ok: day != null,
  }
}

/** YYYY-MM-DD for a stored value, or null when unreadable. */
export function calendarDay(
  raw: string | null | undefined,
  kind: DateKind,
  zone: CalendarZone = localZone(),
): string | null {
  if (raw == null) return null
  const s = raw.trim()
  if (!s) return null
  const ymd = YMD.exec(s)?.[1] ?? null
  if (kind === 'date') return ymd
  if (ymd && s.length === 10) return ymd
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return ymd
  return formatYmd(d, zone.id)
}

/**
 * Inclusive YYYY-MM-DD compare after calendarDay.
 * Empty raw fails when any bound is set; with no bounds it still passes.
 */
export function inRange(
  raw: string | null | undefined,
  kind: DateKind,
  from: string | null | undefined,
  to: string | null | undefined,
  zone: CalendarZone = localZone(),
): boolean {
  if (!raw) return !from && !to
  const day = calendarDay(raw, kind, zone)
  if (!day) return false
  if (from && day < from) return false
  if (to && day > to) return false
  return true
}

/** ISO date of this week's Monday in zone. */
export function startOfWeekMonday(now: Date = new Date(), zone: CalendarZone = localZone()): string {
  const weekday = new Intl.DateTimeFormat('en-US', { timeZone: zone.id, weekday: 'short' }).format(now)
  const offset = { Mon: 0, Tue: 1, Wed: 2, Thu: 3, Fri: 4, Sat: 5, Sun: 6 }[weekday]
  if (offset == null) return calendarDay(now.toISOString(), 'instant', zone) ?? ''
  const today = calendarDay(now.toISOString(), 'instant', zone)
  if (!today) return ''
  const [y, m, d] = today.split('-').map(Number)
  const mon = new Date(Date.UTC(y, m - 1, d - offset))
  const yy = mon.getUTCFullYear()
  const mm = String(mon.getUTCMonth() + 1).padStart(2, '0')
  const dd = String(mon.getUTCDate()).padStart(2, '0')
  return `${yy}-${mm}-${dd}`
}

/**
 * Absolute display. Date-only strings stay on their calendar day (no UTC
 * midnight). Instants use the given zone.
 */
export function formatAbs(
  raw: string | null | undefined,
  kind: DateKind,
  zone: CalendarZone = localZone(),
  locale = 'en-US',
): string {
  if (!raw) return ''
  const s = raw.trim()
  if (kind === 'date' || (s.length === 10 && YMD.test(s))) {
    const day = calendarDay(s, 'date', zone)
    if (!day) return raw
    const [y, m, d] = day.split('-').map(Number)
    return new Date(y, m - 1, d).toLocaleDateString(locale, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
  }
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return raw
  return d.toLocaleString(locale, {
    timeZone: zone.id,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatYmd(d: Date, timeZone: string): string {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(d)
  const year = parts.find((p) => p.type === 'year')?.value
  const month = parts.find((p) => p.type === 'month')?.value
  const day = parts.find((p) => p.type === 'day')?.value
  if (!year || !month || !day) return ''
  return `${year}-${month}-${day}`
}
