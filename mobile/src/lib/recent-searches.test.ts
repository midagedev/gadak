/*
 * recent-searches.ts contract: newest first, at most 10, deduped
 * case-insensitively (the newest spelling wins), recorded at execution —
 * and storage corruption or a throwing localStorage degrades to "no
 * recents", never a crash (settings.ts idiom).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  MAX_RECENT_SEARCHES,
  readRecentSearches,
  recordRecentSearch,
} from './recent-searches'

/** Minimal localStorage for the node test environment (settings.test.ts idiom). */
class MemStorage {
  private m = new Map<string, string>()
  getItem(k: string): string | null {
    return this.m.has(k) ? (this.m.get(k) as string) : null
  }
  setItem(k: string, v: string): void {
    this.m.set(k, v)
  }
  removeItem(k: string): void {
    this.m.delete(k)
  }
}

let storage: MemStorage

beforeEach(() => {
  storage = new MemStorage()
  vi.stubGlobal('localStorage', storage)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('read', () => {
  it('returns [] for empty storage', () => {
    expect(readRecentSearches()).toEqual([])
  })

  it('returns [] for corrupt JSON', () => {
    storage.setItem('gadak-mobile.recent-searches', '{not json')
    expect(readRecentSearches()).toEqual([])
  })

  it('returns [] when the stored value is not an array', () => {
    storage.setItem('gadak-mobile.recent-searches', JSON.stringify({ 0: 'x' }))
    expect(readRecentSearches()).toEqual([])
  })

  it('drops non-string and blank entries, trims the rest', () => {
    storage.setItem(
      'gadak-mobile.recent-searches',
      JSON.stringify(['  camera  ', 7, null, '   ', 'gdk-801']),
    )
    expect(readRecentSearches()).toEqual(['camera', 'gdk-801'])
  })

  it('caps an oversized stored list at the limit', () => {
    const stored = Array.from({ length: 40 }, (_, i) => `q${i}`)
    storage.setItem('gadak-mobile.recent-searches', JSON.stringify(stored))
    expect(readRecentSearches()).toEqual(stored.slice(0, MAX_RECENT_SEARCHES))
  })
})

describe('record', () => {
  it('stores a first search and returns the new list', () => {
    expect(recordRecentSearch('camera')).toEqual(['camera'])
    expect(readRecentSearches()).toEqual(['camera'])
  })

  it('trims before storing; blank input is a no-op', () => {
    recordRecentSearch('  camera  ')
    expect(readRecentSearches()).toEqual(['camera'])
    expect(recordRecentSearch('   ')).toEqual(['camera'])
  })

  it('prepends: newest first', () => {
    recordRecentSearch('a')
    recordRecentSearch('b')
    recordRecentSearch('c')
    expect(readRecentSearches()).toEqual(['c', 'b', 'a'])
  })

  it('dedupes exactly-repeated searches to one entry at the front', () => {
    recordRecentSearch('a')
    recordRecentSearch('b')
    recordRecentSearch('a')
    expect(readRecentSearches()).toEqual(['a', 'b'])
  })

  it('dedupes case-insensitively; the newest spelling wins', () => {
    recordRecentSearch('FTS')
    recordRecentSearch('fts')
    expect(readRecentSearches()).toEqual(['fts'])
  })

  it('moves a re-searched older entry back to the front', () => {
    recordRecentSearch('a')
    recordRecentSearch('b')
    recordRecentSearch('c')
    recordRecentSearch('b')
    expect(readRecentSearches()).toEqual(['b', 'c', 'a'])
  })

  it(`keeps at most ${MAX_RECENT_SEARCHES} searches, dropping the oldest`, () => {
    for (let i = 1; i <= MAX_RECENT_SEARCHES + 1; i++) recordRecentSearch(`q${i}`)
    const got = readRecentSearches()
    expect(got).toHaveLength(MAX_RECENT_SEARCHES)
    expect(got[0]).toBe(`q${MAX_RECENT_SEARCHES + 1}`)
    expect(got).not.toContain('q1')
  })

  it('survives a throwing localStorage (private mode / quota)', () => {
    const boom = {
      getItem: (): string | null => null,
      setItem: (): void => {
        throw new Error('QuotaExceededError')
      },
      removeItem: (): void => {},
    }
    vi.stubGlobal('localStorage', boom)
    expect(recordRecentSearch('camera')).toEqual(['camera'])
  })
})
