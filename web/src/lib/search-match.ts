/*
 * What a server-search row can show as its reason for being there.
 *
 * A row in a search group has to answer "why is this here?" on its own. It has
 * two ways to: the query highlighted inside the title, or a snippet line from
 * the field that actually matched. A row with neither reads as noise — the list
 * claims a match and shows nothing that supports it (vision verdict 2026-08-06).
 *
 * The two are not interchangeable. Repeating a title the row already shows
 * highlighted costs a line and says nothing, so a title hit renders no snippet
 * — but only while the highlight is really there. It often is not: the server
 * matches each query token separately (FTS, prefix), while the client
 * highlights the query as one literal string, so "webhook replay" hits the
 * title "Write a runbook for replaying failed webhook deliveries" on the server
 * and highlights nothing on the client. That row is exactly the unsupported one.
 */
import { highlightSegments } from './format'
import type { SearchMatch } from './types'

/** The title carries the reason by itself when the query is visible in it. */
export function titleShowsQuery(title: string, q: string): boolean {
  if (!q.trim()) return false
  return highlightSegments(title, q).some((seg) => seg.hit)
}

/**
 * The evidence a row should render:
 *   SearchMatch — draw this snippet line
 *   'title'     — the highlighted title already says it; draw no line
 *   null        — the row cannot say why it matched, so it does not belong
 *
 * `q` must be the query the title is highlighted against, or the row would be
 * judged on a highlight it does not display.
 */
export function matchEvidence(
  match: SearchMatch | null | undefined,
  title: string,
  q: string,
): SearchMatch | 'title' | null {
  if (titleShowsQuery(title, q)) return 'title'
  if (match && match.snippet) return match
  return null
}
