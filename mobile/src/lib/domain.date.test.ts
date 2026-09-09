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
 */
import { describe, expect, it } from 'vitest'
import { en, ja, ko } from '../../../web/src/lib/i18n/catalog'
import { folioDate, offerExpiry, relTime } from './domain'

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
