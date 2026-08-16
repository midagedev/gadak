import { describe, expect, test } from 'vitest'
import { aggregateHistory, dateGroup, issueKeysOf } from './history'

describe('history aggregation', () => {
  test('two visit events of one key collapse to count 2', () => {
    const rows = aggregateHistory([
      { type: 'visit', id: 3, kind: 'issue', key: 'NMB-1', at: '2026-08-14T12:00:00.000Z' },
      { type: 'search', id: 1, query: 'upload', result_count: 4, at: '2026-08-14T11:00:00.000Z' },
      { type: 'visit', id: 2, kind: 'issue', key: 'NMB-1', at: '2026-08-14T10:00:00.000Z' },
      { type: 'visit', id: 1, kind: 'page', key: '622723', at: '2026-08-14T09:00:00.000Z' },
    ])
    expect(rows).toEqual([
      { type: 'visit', kind: 'issue', key: 'NMB-1', count: 2, at: '2026-08-14T12:00:00.000Z' },
      {
        type: 'search',
        id: 1,
        query: 'upload',
        resultCount: 4,
        openedKind: null,
        openedKey: null,
        at: '2026-08-14T11:00:00.000Z',
      },
      { type: 'visit', kind: 'page', key: '622723', count: 1, at: '2026-08-14T09:00:00.000Z' },
    ])
    expect(issueKeysOf(rows)).toEqual(['NMB-1'])
  })

  test('date groups are today / yesterday / this week / older', () => {
    const now = new Date(2026, 7, 14, 15, 0, 0) // Friday
    const iso = (y: number, m: number, d: number) => new Date(y, m, d, 10, 0, 0).toISOString()
    expect(dateGroup(iso(2026, 7, 14), now)).toBe('today')
    expect(dateGroup(iso(2026, 7, 13), now)).toBe('yesterday')
    expect(dateGroup(iso(2026, 7, 12), now)).toBe('week')
    expect(dateGroup(iso(2026, 7, 10), now)).toBe('week') // Monday
    expect(dateGroup(iso(2026, 7, 9), now)).toBe('older')
  })
})
