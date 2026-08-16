import { describe, expect, test } from 'vitest'
import {
  RANK_AUTHOR,
  RANK_CHOSUNG,
  RANK_NONE,
  RANK_SPACE,
  RANK_TITLE,
  RANK_TITLE_PREFIX,
  pageMatches,
  pageRank,
  rankPages,
} from './doc-search'
import { extractChosung, isChosungQuery } from './korean'
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
    expect(pageRank(billing, '', false, 'Engineering')).toBe(RANK_TITLE_PREFIX)
    expect(pageMatches(billing, '', false, 'Engineering')).toBe(true)
  })

  test('title prefix outranks a later title substring', () => {
    // Blocks: typing "bill" ranking "Quarterly billing notes" above
    // "Billing setup spec" — the page whose title starts with the query
    // buried under a weaker hit.
    expect(pageRank(billing, 'bill', false, 'Engineering')).toBe(RANK_TITLE_PREFIX)
    expect(pageRank(laterBilling, 'bill', false, 'Engineering')).toBe(RANK_TITLE)
  })

  test('chosung query matches the reduced title, not a Latin substring', () => {
    // Blocks: typing "ㅋㅌㅂ" failing to surface "커트백 가이드" — the
    // document that should match never appears.
    const needle = 'ㅋㅌㅂ'
    expect(isChosungQuery(needle)).toBe(true)
    expect(extractChosung(cutback.title)).toContain(needle)
    expect(pageRank(cutback, needle, true, 'Engineering')).toBe(RANK_CHOSUNG)
    expect(pageRank(cutback, needle, false, 'Engineering')).toBe(RANK_NONE)
    expect(pageRank(billing, needle, true, 'Engineering')).toBe(RANK_NONE)
  })

  test('space label is a match only when the title is not', () => {
    // Blocks: filtering the list to "oper" dropping every Operations page
    // whose title does not also contain the word.
    expect(pageRank(otherSpace, 'oper', false, 'Operations')).toBe(RANK_SPACE)
    expect(pageRank(billing, 'oper', false, 'Engineering')).toBe(RANK_NONE)
  })

  test('author is opted in; the palette path (no author) must not hit on a name', () => {
    // Blocks: the palette treating "dana" as a page (a name means a person
    // there) — or the By-author tab hiding Dana's pages when author is on.
    expect(pageRank(authored, 'dana', false, 'Engineering', { author: true })).toBe(RANK_AUTHOR)
    expect(pageRank(authored, 'dana', false, 'Engineering')).toBe(RANK_NONE)
    expect(pageMatches(authored, 'dana', false, 'Engineering', { author: true })).toBe(true)
    expect(pageMatches(authored, 'dana', false, 'Engineering')).toBe(false)
  })

  test('a rename invalidates the chosung cache so the old title stops matching', () => {
    // Blocks: a page renamed away from "커트백" still appearing for "ㅋㅌㅂ"
    // because the reduced string was cached under the old signature.
    const before = page({ key: 'cache', title: '커트백 가이드', updated_at: '2026-08-01T00:00:00.000Z' })
    expect(pageRank(before, 'ㅋㅌㅂ', true, 'Engineering')).toBe(RANK_CHOSUNG)
    const after = page({
      key: 'cache',
      title: 'Rollback guide',
      updated_at: '2026-08-02T00:00:00.000Z',
    })
    expect(pageRank(after, 'ㅋㅌㅂ', true, 'Engineering')).toBe(RANK_NONE)
  })
})

describe('rankPages', () => {
  const corpus = [billing, laterBilling, cutback, otherSpace, authored, unrelated]

  test('returns only matches, prefix first, newest edit inside a bucket', () => {
    // Blocks: a document that should match not surfacing, or the palette
    // putting an older prefix hit above a newer one in the same bucket.
    const billed = rankPages(corpus, 'bill', false, label)
    expect(billed.map((p) => p.key)).toEqual(['1', '2'])

    const sameBucket = [
      page({ key: 'old', title: 'Billing A', updated_at: '2026-08-01T00:00:00.000Z' }),
      page({ key: 'new', title: 'Billing B', updated_at: '2026-08-09T00:00:00.000Z' }),
    ]
    expect(rankPages(sameBucket, 'bill', false, label).map((p) => p.key)).toEqual(['new', 'old'])
  })

  test('chosung and space and author buckets stay distinct in the order', () => {
    // Blocks: a chosung hit (커트백) or a space hit (Operations) dropping
    // out of the palette because rankPages only kept title matches.
    const mixed = rankPages(corpus, 'ㅋㅌㅂ', true, label)
    expect(mixed.map((p) => p.key)).toEqual(['3'])

    const ops = rankPages(corpus, 'oper', false, label)
    expect(ops.map((p) => p.key)).toEqual(['4'])

    const byAuthor = rankPages(corpus, 'dana', false, label, { author: true })
    expect(byAuthor.map((p) => p.key)).toEqual(['5'])
    expect(rankPages(corpus, 'dana', false, label)).toEqual([])
  })

  test('unrelated titles are dropped, not ranked last', () => {
    // Blocks: the palette listing every document for a query that matches
    // none of them — "Release train" showing up under "bill".
    expect(rankPages(corpus, 'bill', false, label).some((p) => p.key === '6')).toBe(false)
    expect(pageRank(unrelated, 'bill', false, 'Engineering')).toBe(RANK_NONE)
  })
})
