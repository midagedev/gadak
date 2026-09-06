/*
 * The feed's day sections (feed-days.ts) — the layer above GDK-1058's
 * groups. A pure function, so this file pins it directly; the unit
 * project has no svelte plugin (PersonalFeed.test.ts documents the same
 * limit) and the component's $derived only forwards.
 *
 * Contract ↔ assertion table — each clause of feed-days.ts and the test
 * that kills a mutation of it. Beyond all of these: against the
 * pre-round tree feed-days.ts does not exist, so this whole file fails
 * to import — that is the file-level FAIL-first.
 *
 *   C1 sections are runs per local day, in the feed's own order
 *      → 'sections follow the feed's days in order' (two issues on one
 *        day share one section; days appear in feed order) and 'a day
 *        split by another day stays two sections' (a global regroup, or
 *        any re-sort of groups, reorders keys and dies).
 *   C2 today / yesterday come from dateGroup — History's midnight
 *      → 'local midnight is the boundary' (23:59 → yesterday, 00:01 →
 *        today; a boundary one minute off kills it) and the labels are
 *        the today/yesterday kinds, never a week/older bucket.
 *   C3 2..6 days ago is a weekday, otherwise a date; key is local
 *      YYYY-MM-DD
 *      → 'the 2..6-day window' (both edges 2 and 6 weekday, 7 a date)
 *        and the key form ('2026-09-01', local, zero-padded).
 *   C4 timestamp-less items fall where they are
 *      → 'timestamp-less items stay where they fall' (today, unknown,
 *        yesterday — hoisting or sinking the run reorders keys and
 *        dies) and adjacent ones share one unknown section.
 *   C5 total counts items, unread counts read_at == null
 *      → 'total counts items and unread counts read_at == null'
 *        (total 3 with two groups — counting groups gives 2 and dies;
 *        a read item must not count as unread).
 *   C6 label text: catalog keys for today/yesterday/unknown, Intl for
 *      weekday/date, year only when the year differs
 *      → 'today/yesterday/unknown labels are the history catalog
 *        strings' (en/ko/ja — hand-written copy dies on ko) and
 *        'weekday and date labels come from Intl' (a known Monday in
 *        three locales) and 'a date label carries the year only when
 *        the year differs'.
 *   C7 input defense: malformed occurred_at, a group whose first item
 *      has no timestamp, an empty array
 *      → the three 'C7' tests.
 */
import { describe, expect, test } from 'vitest'
import type { FeedItem } from '../../lib/types'
import { en, ja, ko, type MessageKey } from '../../lib/i18n/catalog'
import { groupFeedItems, type FeedGroup } from './feed-groups'
import { feedDaySections, feedDayLabelText, type FeedDayMessageKey } from './feed-days'

// A fixed now for the structure tests: local noon, so nothing here
// depends on the runner's timezone (every fixture below is built with
// the local Date constructor and converted with toISOString, which
// round-trips to the same local wall clock in any zone).
// Month argument is 0-based: 8 = September.
const NOW = new Date(2026, 8, 7, 12, 0)

/** ISO string for `daysAgo` local days back at h:min local wall clock. */
function daysAgo(days: number, h = 10, min = 0): string {
  return new Date(2026, 8, 7 - days, h, min).toISOString()
}

let nextId = 1

function item(over: Partial<FeedItem> & { issue_key: string }): FeedItem {
  return {
    id: nextId++,
    event_id: `evt-${nextId}`,
    summary: 'summary',
    current_status: 'To Do',
    event_type: 'comment_added',
    occurred_at: daysAgo(1),
    actor_name: 'Alex',
    payload: {},
    reasons: ['assignee'],
    read_at: null,
    ...over,
  }
}

/** The layer as the component builds it: items → groups → sections. */
function sectionsOf(items: FeedItem[]): ReturnType<typeof feedDaySections> {
  return feedDaySections(groupFeedItems(items), NOW)
}

const tt = (table: Record<MessageKey, string>) => (k: FeedDayMessageKey) => table[k]

describe('feed-day sections', () => {
  test("C1 sections follow the feed's days in order", () => {
    const s = sectionsOf([
      item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 9) }),
      item({ issue_key: 'STD-2', occurred_at: daysAgo(0, 8) }),
      item({ issue_key: 'STD-3', occurred_at: daysAgo(1, 15) }),
      item({ issue_key: 'STD-4', occurred_at: daysAgo(3, 11) }),
    ])
    expect(s.map((x) => x.key)).toEqual(['today', 'yesterday', '2026-09-04'])
    // Two issues on one day are two groups under one header — the day
    // layer never merges groups, only buckets them.
    expect(s[0].groups.map((g) => g.items[0].issue_key)).toEqual(['STD-1', 'STD-2'])
  })

  test('C1 a day split by another day stays two sections — runs, not a regroup', () => {
    const s = sectionsOf([
      item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 9) }),
      item({ issue_key: 'STD-2', occurred_at: daysAgo(1, 15) }),
      item({ issue_key: 'STD-3', occurred_at: daysAgo(0, 8) }),
    ])
    expect(s.map((x) => x.key)).toEqual(['today', 'yesterday', 'today'])
  })

  test('C2 local midnight is the boundary — 23:59 vs 00:01', () => {
    const s = sectionsOf([
      item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 0, 1) }),
      item({ issue_key: 'STD-2', occurred_at: daysAgo(1, 23, 59) }),
    ])
    expect(s.map((x) => x.key)).toEqual(['today', 'yesterday'])
    expect(s[0].label).toEqual({ kind: 'today' })
    expect(s[1].label).toEqual({ kind: 'yesterday' })
  })

  test('C3 the 2..6-day window is weekday; 7 days is a date', () => {
    const s = sectionsOf([
      item({ issue_key: 'STD-6', occurred_at: daysAgo(6, 9) }),
      item({ issue_key: 'STD-7', occurred_at: daysAgo(7, 9) }),
      item({ issue_key: 'STD-2', occurred_at: daysAgo(2, 9) }),
    ])
    expect(s.map((x) => [x.key, x.label.kind])).toEqual([
      ['2026-09-01', 'weekday'], // 6 days back — the far edge of the window
      ['2026-08-31', 'date'], // 7 days back — one past it
      ['2026-09-05', 'weekday'], // 2 days back — the near edge
    ])
  })

  test('C4 timestamp-less items stay where they fall — not hoisted', () => {
    const s = sectionsOf([
      item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 9) }),
      item({ issue_key: 'STD-9', occurred_at: null }),
      item({ issue_key: 'STD-2', occurred_at: daysAgo(1, 15) }),
    ])
    expect(s.map((x) => x.key)).toEqual(['today', 'unknown', 'yesterday'])
    expect(s[1].label).toEqual({ kind: 'unknown' })
    // Adjacent timestamp-less items are two solo groups under one
    // unknown section — run semantics, same as any other day.
    const s2 = sectionsOf([
      item({ issue_key: 'STD-9', occurred_at: null }),
      item({ issue_key: 'STD-8', occurred_at: null }),
    ])
    expect(s2).toHaveLength(1)
    expect(s2[0].groups).toHaveLength(2)
  })

  test('C5 total counts items and unread counts read_at == null', () => {
    const s = sectionsOf([
      item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 9), read_at: null }),
      item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 8), read_at: daysAgo(0, 11) }),
      item({ issue_key: 'STD-2', occurred_at: daysAgo(0, 7), read_at: null }),
    ])
    expect(s).toHaveLength(1)
    expect(s[0].groups).toHaveLength(2) // STD-1 collapses to one group
    expect(s[0].total).toBe(3) // items, not groups — counting groups gives 2
    expect(s[0].unread).toBe(2)
  })

  test('C6 today/yesterday/unknown labels are the history catalog strings', () => {
    const tables: [string, Record<MessageKey, string>][] = [
      ['en-US', en],
      ['ko-KR', ko],
      ['ja-JP', ja],
    ]
    for (const [tag, table] of tables) {
      expect(feedDayLabelText({ kind: 'today' }, tt(table), tag)).toBe(table['history.groupToday'])
      expect(feedDayLabelText({ kind: 'yesterday' }, tt(table), tag)).toBe(
        table['history.groupYesterday'],
      )
      expect(feedDayLabelText({ kind: 'unknown' }, tt(table), tag)).toBe(table['history.groupOlder'])
    }
    // The shared strings themselves, pinned per locale so a rename is a
    // visible decision on both screens at once, not drift.
    expect(en['history.groupToday']).toBe('Today')
    expect(ko['history.groupToday']).toBe('오늘')
    expect(ja['history.groupToday']).toBe('今日')
  })

  test('C6 weekday and date labels come from Intl in the active locale', () => {
    // 2026-09-07 is a Monday in every timezone (locally constructed).
    const monday = new Date(2026, 8, 7, 9, 0)
    expect(feedDayLabelText({ kind: 'weekday', date: monday }, tt(en), 'en-US')).toBe('Monday')
    expect(feedDayLabelText({ kind: 'weekday', date: monday }, tt(ko), 'ko-KR')).toBe('월요일')
    expect(feedDayLabelText({ kind: 'weekday', date: monday }, tt(ja), 'ja-JP')).toBe('月曜日')
  })

  test('C6 a date label carries the year only when the year differs', () => {
    // feedDayLabelText compares the label's year with the wall clock, so
    // the fixtures are built from the real now — a hard-coded 2026 date
    // would silently gain a year when this file runs in 2027.
    const wall = new Date()
    const thisYear = new Date(wall.getFullYear(), 2, 5, 9, 0) // March 5
    const lastYear = new Date(wall.getFullYear() - 1, 2, 5, 9, 0)
    expect(feedDayLabelText({ kind: 'date', date: thisYear }, tt(en), 'en-US')).toBe(
      new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' }).format(thisYear),
    )
    const oldLabel = feedDayLabelText({ kind: 'date', date: lastYear }, tt(en), 'en-US')
    expect(oldLabel).toBe(
      new Intl.DateTimeFormat('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
      }).format(lastYear),
    )
    expect(oldLabel).toContain(String(lastYear.getFullYear()))
  })

  test('C7 malformed occurred_at strings land in unknown, where they fall', () => {
    const s = sectionsOf([
      item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 9) }),
      item({ issue_key: 'STD-2', occurred_at: 'not-a-timestamp' }),
      item({ issue_key: 'STD-3', occurred_at: '' }),
      item({ issue_key: 'STD-4', occurred_at: daysAgo(1, 15) }),
    ])
    expect(s.map((x) => x.key)).toEqual(['today', 'unknown', 'yesterday'])
    expect(s[1].total).toBe(2)
  })

  test('C7 a group speaks with its first item — a null first item makes the whole group unknown', () => {
    // Cannot come out of groupFeedItems: a timestamp-less item is keyed
    // `solo-${id}` and never merges with a timestamped one. Pinned anyway
    // because the contract ("a group's day is its first item's
    // occurred_at") must not quietly become "the first *usable* one".
    const group: FeedGroup = {
      id: 1,
      groupKey: 'STD-1::hand-built',
      items: [
        item({ issue_key: 'STD-1', occurred_at: null }),
        item({ issue_key: 'STD-1', occurred_at: daysAgo(0, 9) }),
      ],
    }
    const s = feedDaySections([group], NOW)
    expect(s).toHaveLength(1)
    expect(s[0].key).toBe('unknown')
    expect(s[0].total).toBe(2)
  })

  test('C7 an empty feed stays empty', () => {
    expect(feedDaySections([], NOW)).toEqual([])
  })
})
