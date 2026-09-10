/*
 * GDK-1704 date round: the phone's calendar labels were a hardcoded
 * English MONTHS array, the <60s relTime step a hardcoded 'now', and the
 * pairing tab's offer expiry a hardcoded en-US date. All three now follow
 * the active locale. The formatting functions take an explicit localeTag
 * (defaulting to the runtime's), so this file pins them per locale the way
 * feed-days.test.ts does on the web.
 *
 * Contract ↔ assertion table:
 *   C1 the month-day label is the locale's own shape (folioDate and
 *      relTime's 7-day step share it)
 *      → 'C1 the month-day label reads in the locale's own shape'
 *   C2 en does not move — the fix is ko/ja, not a restyled en
 *      → 'C2 the en rendering is byte-identical to the old hardcoded output'
 *   C3 the <60s step is the catalog's time.justNow (the web's compact
 *      relative time reads the same key), never a hardcoded word
 *      → 'C3 the just-now step reads time.justNow'
 *   C4 offerExpiry (PairingTab) formats the locale's short date with year
 *      and answers '' for missing/junk input → 'C4 offerExpiry…'
 *   C5 junk input stays empty regardless of the asked locale
 *      → 'C5 junk input answers empty in every locale'
 *
 * GDK-1765 extends the file: the m/h/d steps between justNow and the
 * calendar step used to hardcode latin unit glyphs, so ko/ja phones read
 * '5m' where the desk reads '5분'.
 *   C6 those steps read time.minute/hour/day from the catalog
 *      → 'C6 the m/h/d steps read time.minute/hour/day in every locale'
 *   C7 the class, not the three deltas: no non-en locale can answer a
 *      latin unit suffix at any age → 'C7 sweep: a non-en locale never
 *      answers a latin unit suffix'
 */
import { describe, expect, it, vi } from 'vitest'
import { en, ja, ko } from '../../../web/src/lib/i18n/catalog'
import { folioDate, offerExpiry, relTime } from './domain'
import { initLocale, t } from './i18n'

// Local-constructor fixtures converted with toISOString, which round-trips
// to the same local wall clock in any runner timezone (feed-days.test.ts's
// trick) — the formatted output is then timezone-independent.
const AUG12 = new Date(2026, 7, 12, 9, 0, 0).toISOString()
const JUL1 = new Date(2026, 6, 1, 9, 0, 0).toISOString()
const SEP9 = new Date(2026, 8, 9, 12, 0, 0).toISOString()
const NOW = new Date(2026, 7, 12, 9, 0, 30) // 30s after AUG12

describe('GDK-1704 locale date labels', () => {
  it('C1 the month-day label reads in the locale’s own shape', () => {
    expect(folioDate(AUG12, 'en')).toBe('Aug 12')
    expect(folioDate(AUG12, 'ko')).toBe('8월 12일')
    expect(folioDate(AUG12, 'ja')).toBe('8月12日')
    // Beyond the 7-day window, relTime's step is the same label —
    // measured against NOW so the branch is deterministic.
    expect(relTime(JUL1, NOW, 'en')).toBe('Jul 1')
    expect(relTime(JUL1, NOW, 'ko')).toBe('7월 1일')
    expect(relTime(JUL1, NOW, 'ja')).toBe('7月1日')
  })

  it('C2 the en rendering is byte-identical to the old hardcoded output', () => {
    // MONTHS spelled 'Aug 12' and the expiry said 'Sep 9, 2026'. If these
    // move, the round changed en copy, which was never the intent.
    expect(folioDate(AUG12, 'en')).toBe('Aug 12')
    expect(offerExpiry(SEP9, 'en')).toBe('Sep 9, 2026')
  })

  it('C3 the just-now step reads time.justNow', () => {
    // All three catalogs carry the key translated…
    expect(en['time.justNow']).toBe('just now')
    expect(ko['time.justNow']).toBe('방금')
    expect(ja['time.justNow']).toBe('たった今')
    // …and the runtime reads it in the default locale (node: en — no
    // localStorage, no navigator).
    expect(relTime(AUG12, NOW)).toBe(en['time.justNow'])
  })

  it('C4 offerExpiry formats the locale’s short date and guards junk', () => {
    // Year-month-day in the locale's own order and marks. Numerals are
    // pinned; the marks are matched loosely because CI's CLDR is not the
    // dev machine's (see QaImpact.date.test.ts C2).
    expect(offerExpiry(SEP9, 'ko')).toMatch(/^2026\D+9\D+9\D*$/)
    expect(offerExpiry(SEP9, 'ja')).toMatch(/^2026\D+9\D+9\D*$/)
    expect(offerExpiry(SEP9, 'ko')).not.toBe(offerExpiry(SEP9, 'en'))
    expect(offerExpiry('')).toBe('')
    expect(offerExpiry('not-a-date')).toBe('')
  })

  it('C5 junk input answers empty in every locale', () => {
    expect(folioDate(null, 'ko')).toBe('')
    expect(folioDate('not-a-date', 'ja')).toBe('')
    expect(relTime(null, NOW, 'ko')).toBe('')
    expect(relTime('not-a-date', NOW, 'ja')).toBe('')
  })
})

describe('GDK-1765 relTime unit steps read the catalog', () => {
  const ago = (sec: number) => new Date(NOW.getTime() - sec * 1000).toISOString()

  // Switching the runtime locale the way locale-detect.test.ts /
  // format.test.ts do: stub storage, re-run initLocale. Restored in the
  // finally so later suites never observe a switched catalog.
  function boot(l: 'en' | 'ko' | 'ja'): Map<string, string> {
    const mem = new Map<string, string>([['gadak_locale', l]])
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => mem.get(k) ?? null,
      setItem: (k: string, v: string) => {
        mem.set(k, v)
      },
      removeItem: (k: string) => {
        mem.delete(k)
      },
      clear: () => mem.clear(),
      key: (i: number) => [...mem.keys()][i] ?? null,
      get length() {
        return mem.size
      },
    })
    initLocale()
    return mem
  }

  it('C6 the m/h/d steps read time.minute/hour/day in every locale', () => {
    const mem = boot('en')
    try {
      for (const l of ['en', 'ko', 'ja'] as const) {
        mem.set('gadak_locale', l)
        initLocale()
        expect(relTime(ago(5 * 60), NOW, l), `${l} minute`).toBe(t('time.minute', { n: 5 }))
        expect(relTime(ago(9 * 3600), NOW, l), `${l} hour`).toBe(t('time.hour', { n: 9 }))
        expect(relTime(ago(3 * 86400), NOW, l), `${l} day`).toBe(t('time.day', { n: 3 }))
      }
      // The audit's finding pinned in glyphs: a ko/ja phone read latin
      // 'm'/'h'/'d' chips here before the catalog round.
      mem.set('gadak_locale', 'ko')
      initLocale()
      expect(relTime(ago(5 * 60), NOW, 'ko')).toBe('5분')
      expect(relTime(ago(9 * 3600), NOW, 'ko')).toBe('9시간')
      expect(relTime(ago(3 * 86400), NOW, 'ko')).toBe('3일')
      mem.set('gadak_locale', 'ja')
      initLocale()
      expect(relTime(ago(5 * 60), NOW, 'ja')).toBe('5分')
      expect(relTime(ago(9 * 3600), NOW, 'ja')).toBe('9時間')
      expect(relTime(ago(3 * 86400), NOW, 'ja')).toBe('3日')
    } finally {
      mem.set('gadak_locale', 'en')
      initLocale()
      vi.unstubAllGlobals()
    }
  })

  it('C7 sweep: a non-en locale never answers a latin unit suffix', () => {
    const mem = boot('ko')
    try {
      for (const l of ['ko', 'ja'] as const) {
        mem.set('gadak_locale', l)
        initLocale()
        // Coprime stride crosses every unit window many times over, so a
        // future branch that hardcodes a latin glyph again is caught even
        // if C6's three deltas get updated alongside it.
        for (let sec = 60; sec < 7 * 86400; sec += 4117) {
          expect(relTime(ago(sec), NOW, l), `${l} at ${sec}s`).not.toMatch(/\d[mhd]$/)
        }
      }
    } finally {
      mem.set('gadak_locale', 'en')
      initLocale()
      vi.unstubAllGlobals()
    }
  })
})
