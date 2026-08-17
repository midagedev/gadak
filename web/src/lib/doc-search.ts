/*
 * Local page matching — the one rule every document surface narrows by.
 *
 * The palette (jump to a page) and the document screens' filter (narrow the
 * list in front of you) ask the same question of the same in-memory index, so
 * they ask it here rather than each growing its own idea of what "matches"
 * means. Everything is a string compare over the loaded index: this runs on a
 * keystroke and must never reach the network.
 */

import type { PageLite } from './types'

/** Rank buckets, best first. Exported so callers can sort without restating them. */
export const RANK_TITLE_PREFIX = 0
export const RANK_TITLE = 1
export const RANK_SPACE = 3
export const RANK_AUTHOR = 4
/** No match. */
export const RANK_NONE = -1

export interface PageMatchOptions {
  /** Include the author name in the haystack. The document screens filter by it
   *  (the By-author tab makes it a visible axis); the palette does not, where a
   *  name means a person, not a page. */
  author?: boolean
}

/**
 * Which bucket a page falls in for `needle`, or `RANK_NONE`.
 *
 * `needle` must already be lowercased and trimmed — it is loop-invariant and
 * computing it per page is what makes a filter feel slow.
 */
export function pageRank(
  page: PageLite,
  needle: string,
  spaceLabel: string,
  opts: PageMatchOptions = {},
): number {
  if (!needle) return RANK_TITLE_PREFIX
  const title = page.title.toLowerCase()
  if (title.startsWith(needle)) return RANK_TITLE_PREFIX
  if (title.includes(needle)) return RANK_TITLE
  if (spaceLabel.toLowerCase().includes(needle)) return RANK_SPACE
  if (opts.author && (page.author ?? '').toLowerCase().includes(needle)) return RANK_AUTHOR
  return RANK_NONE
}

export function pageMatches(
  page: PageLite,
  needle: string,
  spaceLabel: string,
  opts: PageMatchOptions = {},
): boolean {
  return pageRank(page, needle, spaceLabel, opts) !== RANK_NONE
}

/**
 * Every match, best bucket first and newest edit inside a bucket — the palette's
 * order. A screen that already has an order of its own (recency, author groups)
 * filters with `pageMatches` instead, so narrowing never reshuffles the list
 * someone is reading.
 */
export function rankPages(
  list: PageLite[],
  needle: string,
  spaceLabel: (spaceKey: string) => string,
  opts: PageMatchOptions = {},
): PageLite[] {
  const scored: { page: PageLite; rank: number }[] = []
  for (const page of list) {
    const rank = pageRank(page, needle, spaceLabel(page.space_key), opts)
    if (rank !== RANK_NONE) scored.push({ page, rank })
  }
  scored.sort(
    (a, b) => a.rank - b.rank || (b.page.updated_at ?? '').localeCompare(a.page.updated_at ?? ''),
  )
  return scored.map((s) => s.page)
}
