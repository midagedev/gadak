import { describe, expect, test } from 'vitest'
import {
  RANK_AUTHOR,
  RANK_NONE,
  RANK_SPACE,
  RANK_TITLE,
  RANK_TITLE_PREFIX,
  pageMatches,
  pageRank,
  rankPages,
} from './doc-search'
import type { PageLite } from './types'

/*
 * Local document matching is the one rule the palette and the document
 * screens share. A page that should match and does not disappear from both.
 */

function page(over: Partial<PageLite> & Pick<PageLite, 'key' | 'title'>): PageLite {
  return {
    space_key: 'ENG',
    parent_id: null,
    author: null,
    updated_at: '2026-08-10T00:00:00.000Z',
    version: 1,
    url: '',
    ...over,
  }
}

const billing = page({ key: '1', title: 'Billing setup spec' })
const laterBilling = page({
  key: '2',
  title: 'Quarterly billing notes',
  updated_at: '2026-08-12T00:00:00.000Z',
})
const cutback = page({ key: '3', title: '커트백 가이드' })
const otherSpace = page({ key: '4', title: 'Runbook', space_key: 'OPS' })
const authored = page({ key: '5', title: 'On-call rota', author: 'Dana Kim' })
const unrelated = page({ key: '6', title: 'Release train' })

function label(spaceKey: string): string {
  return spaceKey === 'OPS' ? 'Operations' : 'Engineering'
}

describe('pageRank', () => {
  test('empty needle is a prefix hit so an empty filter keeps every page', () => {
    // Blocks: opening the docs filter (or the palette with no query) hiding
    // the whole index because "" was treated as "match nothing".
    expect(pageRank(billing, '', 'Engineering')).toBe(RANK_TITLE_PREFIX)
    expect(pageMatches(billing, '', 'Engineering')).toBe(true)
  })

  test('title prefix outranks a later title substring', () => {
    // Blocks: typing "bill" ranking "Quarterly billing notes" above
    // "Billing setup spec" — the page whose title starts with the query
    // buried under a weaker hit.
    expect(pageRank(billing, 'bill', 'Engineering')).toBe(RANK_TITLE_PREFIX)
    expect(pageRank(laterBilling, 'bill', 'Engineering')).toBe(RANK_TITLE)
  })

  test('space label is a match only when the title is not', () => {
    // Blocks: filtering the list to "oper" dropping every Operations page
    // whose title does not also contain the word.
    expect(pageRank(otherSpace, 'oper', 'Operations')).toBe(RANK_SPACE)
    expect(pageRank(billing, 'oper', 'Engineering')).toBe(RANK_NONE)
  })

  test('author is opted in; the palette path (no author) must not hit on a name', () => {
    // Blocks: the palette treating "dana" as a page (a name means a person
    // there) — or the By-author tab hiding Dana's pages when author is on.
    expect(pageRank(authored, 'dana', 'Engineering', { author: true })).toBe(RANK_AUTHOR)
    expect(pageRank(authored, 'dana', 'Engineering')).toBe(RANK_NONE)
    expect(pageMatches(authored, 'dana', 'Engineering', { author: true })).toBe(true)
    expect(pageMatches(authored, 'dana', 'Engineering')).toBe(false)
  })
})

describe('rankPages', () => {
  const corpus = [billing, laterBilling, cutback, otherSpace, authored, unrelated]

  test('returns only matches, prefix first, newest edit inside a bucket', () => {
    // Blocks: a document that should match not surfacing, or the palette
    // putting an older prefix hit above a newer one in the same bucket.
    const billed = rankPages(corpus, 'bill', label)
    expect(billed.map((p) => p.key)).toEqual(['1', '2'])

    const sameBucket = [
      page({ key: 'old', title: 'Billing A', updated_at: '2026-08-01T00:00:00.000Z' }),
      page({ key: 'new', title: 'Billing B', updated_at: '2026-08-09T00:00:00.000Z' }),
    ]
    expect(rankPages(sameBucket, 'bill', label).map((p) => p.key)).toEqual(['new', 'old'])
  })

  test('space and author buckets stay distinct in the order', () => {
    // Blocks: a space hit (Operations) dropping out of the palette because
    // rankPages only kept title matches.
    const ops = rankPages(corpus, 'oper', label)
    expect(ops.map((p) => p.key)).toEqual(['4'])

    const byAuthor = rankPages(corpus, 'dana', label, { author: true })
    expect(byAuthor.map((p) => p.key)).toEqual(['5'])
    expect(rankPages(corpus, 'dana', label)).toEqual([])
  })

  test('unrelated titles are dropped, not ranked last', () => {
    // Blocks: the palette listing every document for a query that matches
    // none of them — "Release train" showing up under "bill".
    expect(rankPages(corpus, 'bill', label).some((p) => p.key === '6')).toBe(false)
    expect(pageRank(unrelated, 'bill', 'Engineering')).toBe(RANK_NONE)
  })
})
