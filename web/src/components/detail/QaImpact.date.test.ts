/*
 * GDK-1704: QaImpact's formatTime hardcoded a Korean region tag, so EN and
 * JA readers got Korean-formatted QA case timestamps — a reverse-direction
 * leak (Korean date shapes on an English screen). The default vitest
 * project has no svelte plugin (vitest.config.ts; PersonalFeed.test.ts
 * documents the same limit), so this file pins what a mount would render:
 * the exact Intl option set the component now formats with, per locale,
 * plus the wiring — the component source must format through the i18n
 * runtime's locale() and must not name a fixed region tag.
 *
 * Contract ↔ assertion table:
 *   C1 the component formats through the runtime locale, not a region
 *      constant → 'C1 formatTime formats through the i18n runtime locale'
 *   C2 the stamp itself reads in the reader's own locale
 *      → 'C2 the case timestamp reads in the reader's locale (en/ko/ja)'
 *   C3 the runtime default is one of the three catalog locales
 *      → 'C3 the runtime default resolves to a supported locale'
 */
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { locale } from '../../lib/i18n'

const SRC = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'QaImpact.svelte'), 'utf8')

/** The exact option set QaImpact.formatTime formats with. */
function qaStamp(value: Date, tag: string): string {
  return new Intl.DateTimeFormat(tag, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(value)
}

describe('GDK-1704 QaImpact case timestamps follow the active locale', () => {
  test('C1 formatTime formats through the i18n runtime locale', () => {
    // Wiring, not spelling: the call names the runtime helper and the
    // source carries no fixed region tag anywhere (comment included).
    expect(SRC).toContain('Intl.DateTimeFormat(locale()')
    expect(SRC).not.toContain('ko-KR')
  })

  test("C2 the case timestamp reads in the reader's locale (en/ko/ja)", () => {
    // Local-constructor fixture (14:30 on the local Aug 12): the wall
    // clock — and therefore the output — is timezone-independent.
    //
    // The assertion is the date *shape* per locale, never ICU's rendered
    // spelling: CI's Node carries a different CLDR than a dev machine's
    // (measured 2026-09-09 — the same Node 20 line printed '오후 02:30'
    // locally and 'PM 02:30' on the runner), so a pinned Korean day-period
    // string is a flake, not a contract. What the round must guarantee is
    // that the reader's locale reaches Intl at all.
    const at = new Date(2026, 7, 12, 14, 30)
    expect(qaStamp(at, 'en')).toBe('8/12, 02:30 PM')
    // ko separates the numbers with periods; ja does not, and neither
    // renders the en form.
    expect(qaStamp(at, 'ko')).toMatch(/^8\. 12\./)
    expect(qaStamp(at, 'ja')).toBe('8/12 14:30')
    expect(new Set([qaStamp(at, 'en'), qaStamp(at, 'ko'), qaStamp(at, 'ja')]).size).toBe(3)
  })

  test('C3 the runtime default resolves to a supported locale', () => {
    expect(['en', 'ko', 'ja']).toContain(locale())
  })
})
