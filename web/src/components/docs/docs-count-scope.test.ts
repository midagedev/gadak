/*
 * GDK-1092 B-5 (+ the A-7 class): a count beside a screen's name says which
 * scope it counts.
 *
 * The Documents header read "Documents 0" on an account whose wiki held 71
 * pages. The number was true — the Viewed tab's own length — but the badge
 * sits beside the word "Documents", and nothing on the screen said the tab
 * had narrowed it. Worse, the denominator meant two different things per tab:
 * `tab === 'viewed' ? pages.recentlyViewed.length : pages.index.length`, so
 * the fraction the filter drew on Updated was "of the library" and on Viewed
 * was "of the pages you happen to have opened".
 *
 * The rule this pins (UX_PRINCIPLES §"Two totals"): the denominator beside a
 * screen's name is the LIBRARY total, one owner for every tab, and a scoped
 * number is written as a fraction of it. "0" becomes "0 / 71" — the tab has
 * narrowed, and the badge shows the narrowing instead of hiding it.
 *
 * FAIL-first output on the source before this round is in the round report:
 * the `total` derived branched on `tab`, and `headerCount` did not exist.
 */
import { readFileSync } from 'node:fs'
import { describe, expect, test } from 'vitest'

import { headerCount } from './docs-count'

const SRC = readFileSync(new URL('./DocsView.svelte', import.meta.url), 'utf8')

describe('GDK-1092 B-5: the Documents denominator is the library, on every tab', () => {
  test('DocsView does not compute a per-tab total of its own', () => {
    // The whole defect was a denominator that changed meaning with the tab.
    // The declaration, balanced-paren-free: everything from `const total =`
    // to the line that closes it (`)` at the declaration's own indent, or the
    // same line when it fits on one).
    const totalDecl = /const total = \$derived\((?:[^\n]*\)|[\s\S]*?\n  \))/.exec(SRC)?.[0] ?? ''
    expect(
      /tab === '/.test(totalDecl) ? totalDecl : '(no per-tab branch)',
      'the header denominator must not branch on the active tab — see docs-count.ts',
    ).toBe('(no per-tab branch)')
  })

  test('the header count comes from the shared owner', () => {
    expect(SRC).toContain("from './docs-count'")
    expect(SRC).toMatch(/count=\{headerCount\(/)
  })

  test('a scoped number is a fraction of the library total', () => {
    // Viewed tab, nothing opened yet, 71 pages in the library.
    expect(headerCount(0, 71, (n) => String(n))).toBe('0 / 71')
    // A narrowed tab.
    expect(headerCount(3, 71, (n) => String(n))).toBe('3 / 71')
  })

  test('an unnarrowed screen shows one number, not "71 / 71"', () => {
    expect(headerCount(71, 71, (n) => String(n))).toBe('71')
  })

  test('an empty library says 0, not "0 / 0"', () => {
    expect(headerCount(0, 0, (n) => String(n))).toBe('0')
  })

  test('the formatter is the caller\'s (locale digits)', () => {
    expect(headerCount(0, 1234, (n) => n.toLocaleString('en-US'))).toBe('0 / 1,234')
  })
})
