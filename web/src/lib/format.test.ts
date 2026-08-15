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

describe('priorityMeta (rank, not name)', () => {
  test.each([
    { rank: 0, name: null, level: 0, label: 'None' },
    { rank: 1, name: 'Highest', level: 5, label: 'Highest' },
    { rank: 2, name: 'High', level: 4, label: 'High' },
    { rank: 3, name: 'Medium', level: 3, label: 'Medium' },
    { rank: 4, name: 'Low', level: 2, label: 'Low' },
    { rank: 5, name: 'Lowest', level: 1, label: 'Lowest' },
    { rank: null, name: null, level: 0, label: 'None' },
    { rank: undefined, name: null, level: 0, label: 'None' },
  ] as const)('rank $rank / $name → level $level, label untouched', ({ rank, name, level, label }) => {
    const meta = priorityMeta(rank, name)
    expect(meta.level).toBe(level)
    expect(meta.label).toBe(label)
  })

  test('localized name does not change the level: French/German rank 1 == Highest rank 1', () => {
    const en = priorityMeta(1, 'Highest')
    const fr = priorityMeta(1, 'La plus haute')
    const de = priorityMeta(1, 'Höchste')
    expect(fr.level).toBe(en.level)
    expect(de.level).toBe(en.level)
    expect(en.level).toBe(5)
    expect(fr.label).toBe('La plus haute')
    expect(de.label).toBe('Höchste')
  })

  test('Korean name is a label only: rank 2 + 높음 is the same level as High', () => {
    const meta = priorityMeta(2, '높음')
    expect(meta.level).toBe(priorityMeta(2, 'High').level)
    expect(meta.label).toBe('높음')
  })

  test('missing rank keeps the row: unset styling, label untouched', () => {
    const meta = priorityMeta(undefined, 'Highest')
    expect(meta.level).toBe(0)
    expect(meta.label).toBe('Highest')
    expect(meta.color).toBe('var(--color-border-strong)')
  })

  test('rank 0 with a leftover name is unset, not name-matched', () => {
    const meta = priorityMeta(0, 'Highest')
    expect(meta.level).toBe(0)
    expect(meta.label).toBe('Highest')
  })

  test('colors are paper tokens; rank 6 clamps to the coolest step', () => {
    expect(priorityMeta(1, 'x').color).toBe('var(--color-status-reopen)')
    expect(priorityMeta(2, 'x').color).toBe('var(--color-status-inprogress)')
    expect(priorityMeta(3, 'x').color).toBe('var(--color-status-new)')
    expect(priorityMeta(4, 'x').color).toBe('var(--color-text-secondary)')
    expect(priorityMeta(5, 'x').color).toBe('var(--color-text-muted)')
    expect(priorityMeta(6, 'x').color).toBe('var(--color-text-muted)')
    expect(priorityMeta(6, 'x').level).toBe(1)
    expect(priorityMeta(0, null).color).toBe('var(--color-border-strong)')
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
