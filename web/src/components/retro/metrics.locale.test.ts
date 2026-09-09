/*
 * GDK-1728: the retro's durations read in the reader's language.
 *
 * `formatDays` and `formatSeconds` printed the unit as a literal — `9.0d`,
 * `23.0h`, `14m` — so a Korean or Japanese capture of the retro carried Latin
 * suffixes in the middle of its own sentences. They go through the catalog's
 * duration keys now. The suffix is the only thing that moved: the rung of the
 * ladder and the digits are the CLI's, which is the contract GDK-1683 set
 * ('the two surfaces print the same number').
 *
 * The default-locale suite cannot see this at all — en's `time.day` is
 * `{n}d`, so every existing assertion is byte-identical, which is exactly why
 * this file exists and pins the other two locales.
 *
 * Contract ↔ assertion table:
 *   L1 the unit comes from the catalog, per locale, on every rung
 *      → 'L1 the duration ladder reads in each locale'
 *   L2 en does not move — the fix is ko/ja, not a restyled en
 *      → 'L2 the en rendering is byte-identical to the old literals'
 *   L3 the number is the CLI's: same rung boundaries, same digits
 *      → 'L3 the rung and the digits are unchanged in every locale'
 *   L4 a delta keeps its U+2212 sign in front of a translated unit
 *      → 'L4 a delta signs a translated unit'
 */
import { afterEach, describe, expect, it } from 'vitest'
import { en, ja, ko } from '../../lib/i18n/catalog'
import { initLocale } from '../../lib/i18n'
import { formatDays, formatDelta, formatSeconds } from './metrics'

/*
 * The unit project runs in node, where there is no localStorage — and
 * detectLocale's read of it is inside a try/catch, so an absent one is not an
 * error, it is a silent fall through to 'en'. A test that only set a value
 * would therefore pass in every locale by testing none of them. So the store
 * is provided, minimally, and the locale is pinned the way the app pins it.
 */
const store = new Map<string, string>()
const shim = {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => void store.set(k, v),
  removeItem: (k: string) => void store.delete(k),
}
Object.defineProperty(globalThis, 'localStorage', {
  value: shim,
  configurable: true,
})

/** Pin the active locale the way the app itself picks one, then re-detect. */
function useLocale(loc: string): void {
  store.set('gadak_locale', loc)
  expect(initLocale()).toBe(loc)
}

afterEach(() => {
  store.clear()
  initLocale()
})

describe('GDK-1728 retro duration units', () => {
  it('L1 the duration ladder reads in each locale', () => {
    for (const [loc, cat] of [
      ['en', en],
      ['ko', ko],
      ['ja', ja],
    ] as const) {
      useLocale(loc)
      // 9 days, 23 hours, 14 minutes, 45 seconds — one value per rung.
      expect(formatDays(9), loc).toBe(cat['time.day'].replace('{n}', '9.0'))
      expect(formatDays(23 / 24), loc).toBe(
        cat['time.hour'].replace('{n}', '23.0'),
      )
      expect(formatDays(14 / (24 * 60)), loc).toBe(
        cat['time.minute'].replace('{n}', '14'),
      )
      expect(formatSeconds(45), loc).toBe(
        cat['time.second'].replace('{n}', '45'),
      )
      expect(formatSeconds(120), loc).toBe(
        cat['time.minute'].replace('{n}', '2'),
      )
      expect(formatSeconds(7200), loc).toBe(
        cat['time.hour'].replace('{n}', '2.0'),
      )
    }
  })

  it('L2 the en rendering is byte-identical to the old literals', () => {
    useLocale('en')
    expect(formatDays(9)).toBe('9.0d')
    expect(formatDays(23 / 24)).toBe('23.0h')
    expect(formatDays(14 / (24 * 60))).toBe('14m')
    expect(formatSeconds(45)).toBe('45s')
    expect(formatSeconds(120)).toBe('2m')
    expect(formatSeconds(7200)).toBe('2.0h')
  })

  it('L3 the rung and the digits are unchanged in every locale', () => {
    // The boundaries are the CLI's: a day or more prints days, an hour or
    // more prints hours, below that minutes. A translated unit must not move
    // any of those, so the digits are read back out of the rendered string.
    for (const loc of ['en', 'ko', 'ja'] as const) {
      useLocale(loc)
      expect(formatDays(1), loc).toContain('1.0')
      expect(formatDays(0.9999), loc).toContain('24.0')
      expect(formatDays(1 / 24), loc).toContain('1.0')
      expect(formatDays(0.9 / 24), loc).toContain('54')
    }
  })

  it('L4 a delta signs a translated unit', () => {
    useLocale('ko')
    const d = formatDelta(1.25, 'days')
    expect(d.charCodeAt(0)).not.toBe(0x2212); // +1.3…
    expect(d).toBe('+' + ko['time.day'].replace('{n}', '1.3'))
    expect(formatDelta(-1.25, 'days')).toBe(
      '−' + ko['time.day'].replace('{n}', '1.3'),
    )
    expect(formatDelta(-1.25, 'days').charCodeAt(0)).toBe(0x2212)
  })
})
