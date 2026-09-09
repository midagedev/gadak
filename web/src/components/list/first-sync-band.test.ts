import { describe, expect, test } from 'vitest'
import { firstSyncLine } from './first-sync-band'

/*
 * The first-sync band's wording contract (GDK-1677): phase × denominator
 * picks the sentence, formatNumber picks the digits, wiki_pending picks the
 * tail. The node environment has no localStorage/navigator, so the catalog
 * resolves to en here; the ko line is asserted verbatim on the rendered
 * page by e2e/first-sync-band.spec.ts (forceLocale), which is the only
 * place a locale switch is honest.
 */

describe('firstSyncLine', () => {
  test('issues phase with a total counts toward it and promises the wiki', () => {
    expect(firstSyncLine({ phase: 'issues', fetched: 1200, total: 3514, wiki_pending: true })).toBe(
      'Recent issues first · 1,200 / 3,514 · wiki next',
    )
  })

  test('issues phase without a total says so far, not a fraction', () => {
    expect(firstSyncLine({ phase: 'issues', fetched: 1200, wiki_pending: true })).toBe(
      'Recent issues first · 1,200 so far · wiki next',
    )
  })

  test('no wiki pending drops the tail', () => {
    expect(firstSyncLine({ phase: 'issues', fetched: 1200, total: 3514, wiki_pending: false })).toBe(
      'Recent issues first · 1,200 / 3,514',
    )
  })

  test('documents phase names the wiki and never promises a next', () => {
    expect(firstSyncLine({ phase: 'documents', fetched: 120, total: 462, wiki_pending: true })).toBe(
      'Issues done · wiki 120 / 462',
    )
    expect(firstSyncLine({ phase: 'documents', fetched: 120 })).toBe('Issues done · wiki 120 so far')
  })

  test('digits go through the thousands separator', () => {
    // A raw 1200/3514 in the line would mean the numbers bypassed
    // formatNumber — the same helper list-count uses.
    expect(firstSyncLine({ phase: 'issues', fetched: 1200, total: 3514, wiki_pending: true })).toContain(
      '1,200 / 3,514',
    )
  })
})
