/*
 * UI-side delivery of a server-search outcome, and the one owner of "hand
 * this text to the list's server search" (GDK-727).
 *
 * The filter engine returns issue keys and match reasons; it does not toast
 * and does not write the pages store. Surfaces that start a search apply
 * this: page hits go to `pages`, a failed search leaves the list EmptyState
 * (retry) rather than also toasting the same `list.searchFailed` sentence
 * (GDK-549), and a deployment with no server FTS toasts what that snapshot
 * actually searches (`list.searchBodyUnavailable`, info tone).
 *
 * The `default: never` arm is the recurrence lock: a new outcome status must
 * decide its toast here or the build breaks, so a fourth surface cannot
 * inherit the previous one's "check the connection" lie.
 *
 * `widenToServerSearch` is the widen sequence: setQuery then runServerSearch
 * then apply the outcome. Which screen to leave is the caller's — documents
 * close docs, history closes history — passed as `leave` so an empty query
 * does not drop the person off the screen they are on.
 */

import { filters, type ServerSearchOutcome } from '../stores/filters.svelte'
import { pages } from '../stores/pages.svelte'
import { write } from '../stores/write.svelte'
import { t } from './i18n'

/**
 * Hand `query` to the list's server search and go where those results live.
 * Empty input is a no-op, including `leave`.
 */
export function widenToServerSearch(query: string, leave?: () => void): void {
  const q = query.trim()
  if (!q) return
  leave?.()
  filters.setQuery(q)
  void filters.runServerSearch().then(applyServerSearchOutcome)
}

export function applyServerSearchOutcome(outcome: ServerSearchOutcome): void {
  switch (outcome.status) {
    case 'empty':
      return
    case 'ok':
      pages.setSearchHits(outcome.pages)
      return
    case 'error':
      pages.clearSearchHits()
      return
    case 'unavailable':
      // Not an error: the network is fine, the deployment just has no server
      // FTS to ask. Client-side title/key matching still ran.
      pages.clearSearchHits()
      write.toast(t('list.searchBodyUnavailable'), 'info')
      return
    default: {
      const _exhaustive: never = outcome
      return _exhaustive
    }
  }
}
