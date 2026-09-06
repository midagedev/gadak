/*
 * The feed's day sections — a layer above feed-groups (GDK-1058), never
 * inside it. feed-groups collapses adjacent same-issue same-day events
 * into row groups; this buckets those groups into *runs* per local
 * calendar day, so the feed reads as days the way History already reads
 * as days for visits.
 *
 * One vocabulary for "when" across the app: today / yesterday come from
 * dateGroup() (lib/history.ts) — the same function the History screen
 * buckets by — so the two screens can never disagree about midnight.
 * Beyond yesterday the feed diverges from History on purpose: a feed is
 * read for *what happened on which day*, so the header names the day
 * (weekday for the last six days, then a date), not a coarse
 * "this week / older" bucket.
 *
 * Sections never re-sort: a section is a run of adjacent groups on the
 * same local day, in the feed's own order. me.feedItems order is the
 * store's, and this module must not mutate, filter, or reorder what the
 * store shipped — the same discipline feed-groups keeps.
 */
import { dateGroup, startOfLocalDay } from '../../lib/history'
import type { FeedGroup } from './feed-groups'

export type FeedDayLabel =
  | { kind: 'today' }
  | { kind: 'yesterday' }
  /** 2..6 days ago: the weekday name. */
  | { kind: 'weekday'; date: Date }
  /** Older: month + day, year when not this year. */
  | { kind: 'date'; date: Date }
  /** No usable occurred_at. */
  | { kind: 'unknown' }

export interface FeedDaySection {
  /** 'today' | 'yesterday' | 'YYYY-MM-DD' | 'unknown'. */
  key: string
  label: FeedDayLabel
  /** feed-groups' output, in order, never re-sorted. */
  groups: FeedGroup[]
  /** Events (items) in the section. */
  total: number
  /** Items with read_at == null. */
  unread: number
}

/** The catalog keys this module reads. Narrow on purpose: the i18n `t`
 *  (whose parameter is the MessageKey literal union) is assignable to
 *  this and NOT to a `(k: string) => string` — so the owner is passed
 *  through directly, with no cast and no second string table. */
export type FeedDayMessageKey = 'history.groupToday' | 'history.groupYesterday' | 'history.groupOlder'

/** Local calendar day as YYYY-MM-DD — the section key past yesterday,
 *  and what the two screens would compare to agree on a day. */
function localDateKey(d: Date): string {
  const month = `${d.getMonth() + 1}`.padStart(2, '0')
  const day = `${d.getDate()}`.padStart(2, '0')
  return `${d.getFullYear()}-${month}-${day}`
}

function sectionOf(iso: string | null, now: Date): { key: string; label: FeedDayLabel } {
  if (!iso) return { key: 'unknown', label: { kind: 'unknown' } }
  const parsed = new Date(iso)
  if (Number.isNaN(parsed.getTime())) return { key: 'unknown', label: { kind: 'unknown' } }
  const group = dateGroup(iso, now)
  if (group === 'today') return { key: 'today', label: { kind: 'today' } }
  if (group === 'yesterday') return { key: 'yesterday', label: { kind: 'yesterday' } }
  // Past yesterday the day count decides: 2..6 days ago is a weekday
  // name, older is a date. Day starts + rounded quotient — the DST-safe
  // arithmetic dateGroup itself uses, so a 23-hour transit day still
  // counts as one day.
  const diffDays = Math.round(
    (startOfLocalDay(now).getTime() - startOfLocalDay(parsed).getTime()) / 86_400_000,
  )
  if (diffDays >= 2 && diffDays <= 6) {
    return { key: localDateKey(parsed), label: { kind: 'weekday', date: parsed } }
  }
  return { key: localDateKey(parsed), label: { kind: 'date', date: parsed } }
}

/** Bucket feed-groups' output into runs of adjacent groups per local
 *  calendar day. Never re-sorts: two runs of the same day with another
 *  day between them stay two sections. */
export function feedDaySections(groups: FeedGroup[], now: Date = new Date()): FeedDaySection[] {
  const out: FeedDaySection[] = []
  for (const group of groups) {
    // A group's day is its first item's occurred_at. feed-groups keys a
    // group `${issue_key}::${toDateString()}`, so every item in a group
    // is already on one local calendar day (and a timestamp-less item is
    // a `solo-${id}` group of one) — the first item answers for all.
    const { key, label } = sectionOf(group.items[0]?.occurred_at ?? null, now)
    const last = out[out.length - 1]
    if (last && last.key === key) last.groups.push(group)
    else out.push({ key, label, groups: [group], total: 0, unread: 0 })
  }
  for (const section of out) {
    section.total = section.groups.reduce((n, g) => n + g.items.length, 0)
    section.unread = section.groups.reduce(
      (n, g) => n + g.items.filter((item) => item.read_at === null).length,
      0,
    )
  }
  return out
}

/**
 * The header text for a section label. today / yesterday / unknown come
 * from the history.* catalog keys History already uses (zero new copy);
 * weekday and date come from Intl in the active locale, which is what
 * keeps them zero-copy across en/ko/ja.
 *
 * The year clause compares against the wall clock at render time: the
 * label carries only the date, and "this year" is a property of now,
 * not of the section (which was built against the feed's own `now`).
 */
export function feedDayLabelText(
  label: FeedDayLabel,
  t: (key: FeedDayMessageKey) => string,
  locale: string,
): string {
  switch (label.kind) {
    case 'today':
      return t('history.groupToday')
    case 'yesterday':
      return t('history.groupYesterday')
    case 'weekday':
      return new Intl.DateTimeFormat(locale, { weekday: 'long' }).format(label.date)
    case 'date': {
      const withYear = label.date.getFullYear() !== new Date().getFullYear()
      return new Intl.DateTimeFormat(locale, {
        month: 'short',
        day: 'numeric',
        ...(withYear ? { year: 'numeric' } : {}),
      }).format(label.date)
    }
    default:
      return t('history.groupOlder')
  }
}
