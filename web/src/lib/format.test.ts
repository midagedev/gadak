import { afterEach, beforeAll, describe, expect, test, vi } from 'vitest'
import {
  colorIndex,
  highlightSegments,
  initials,
  mergeAdjacentHits,
  priorityMeta,
  relativeTime,
} from './format'
import { initLocale, locale } from './i18n'

beforeAll(() => {
  const mem = new Map<string, string>([['gadak_locale', 'en']])
  vi.stubGlobal('localStorage', {
    getItem: (k: string) => mem.get(k) ?? null,
    setItem: (k: string, v: string) => {
      mem.set(k, v)
    },
    removeItem: (k: string) => {
      mem.delete(k)
    },
    clear: () => {
      mem.clear()
    },
    key: (i: number) => [...mem.keys()][i] ?? null,
    get length() {
      return mem.size
    },
  })
  initLocale()
  expect(locale()).toBe('en')
})

describe('highlightSegments / mergeAdjacentHits', () => {
  test('empty query is a single miss span', () => {
    expect(highlightSegments('replaying failed webhook deliveries', '')).toEqual([
      { text: 'replaying failed webhook deliveries', hit: false },
    ])
  })

  test('each query word is marked on its own (not the whole phrase)', () => {
    expect(highlightSegments('replaying failed webhook deliveries', 'webhook replay')).toEqual([
      { text: 'replay', hit: true },
      { text: 'ing failed ', hit: false },
      { text: 'webhook', hit: true },
      { text: ' deliveries', hit: false },
    ])
  })

  test('at a tie the longer word wins', () => {
    expect(highlightSegments('runbook', 'run runbook')).toEqual([{ text: 'runbook', hit: true }])
  })

  test('mergeAdjacentHits joins hits that only whitespace separates', () => {
    const segs = highlightSegments('rate limit storm', 'rate limit storm')
    expect(mergeAdjacentHits(segs)).toEqual([{ text: 'rate limit storm', hit: true }])
  })

  test('mergeAdjacentHits does not swallow a hyphen between hits', () => {
    const segs = [
      { text: 'rate', hit: true },
      { text: '-', hit: false },
      { text: 'limit', hit: true },
    ]
    expect(mergeAdjacentHits(segs)).toEqual(segs)
  })
})

describe('initials / colorIndex', () => {
  test('initials: empty, email local-part, Hangul given name, Latin tokens', () => {
    expect(initials(null)).toBe('?')
    expect(initials('')).toBe('?')
    expect(initials(null, 'marco@x.io')).toBe('MA')
    expect(initials('김철수')).toBe('철수')
    expect(initials('Jane Doe')).toBe('JD')
    expect(initials('Jane')).toBe('JA')
  })

  test('colorIndex is stable for a seed', () => {
    expect(colorIndex('alex@example.com')).toBe(colorIndex('alex@example.com'))
    expect(colorIndex('alex@example.com', 8)).toBeGreaterThanOrEqual(0)
    expect(colorIndex('alex@example.com', 8)).toBeLessThan(8)
    expect(colorIndex(null)).toBe(0)
  })
})

describe('priorityMeta (EN + KO names from format.ts:137–144)', () => {
  test('maps both language names onto the same level', () => {
    expect(priorityMeta('Highest').level).toBe(5)
    expect(priorityMeta('긴급').level).toBe(5)
    expect(priorityMeta('High').level).toBe(4)
    expect(priorityMeta('높음').level).toBe(4)
    expect(priorityMeta('Medium').level).toBe(3)
    expect(priorityMeta('보통').level).toBe(3)
    expect(priorityMeta('Low').level).toBe(2)
    expect(priorityMeta('낮음').level).toBe(2)
    expect(priorityMeta('Lowest').level).toBe(1)
    expect(priorityMeta('가장 낮음').level).toBe(1)
    expect(priorityMeta(null).level).toBe(0)
    expect(priorityMeta(null).label).toBe('None')
  })
})

describe('relativeTime (en catalog, pinned clock)', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  test('compact buckets against a fixed now', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-15T12:00:00.000Z'))
    expect(relativeTime(null)).toBe('')
    expect(relativeTime('2026-08-15T11:59:30.000Z')).toBe('just now')
    expect(relativeTime('2026-08-15T11:57:00.000Z')).toBe('3m')
    expect(relativeTime('2026-08-15T10:00:00.000Z')).toBe('2h')
    expect(relativeTime('2026-08-13T12:00:00.000Z')).toBe('2d')
  })
})
