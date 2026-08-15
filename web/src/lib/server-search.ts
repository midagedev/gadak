/*
 * UI-side delivery of a server-search outcome.
 *
 * The filter engine returns issue keys and match reasons; it does not toast
 * and does not write the pages store. Surfaces that start a search apply
 * this: page hits go to `pages`, a failed search toasts with the existing
 * `list.searchFailed` string.
 */

import { pages } from '../stores/pages.svelte'
import { write } from '../stores/write.svelte'
import { t } from './i18n'
import type { ServerSearchOutcome } from '../stores/filters.svelte'

export function applyServerSearchOutcome(outcome: ServerSearchOutcome): void {
  if (outcome.status === 'ok') {
    pages.setSearchHits(outcome.pages)
    return
  }
  if (outcome.status === 'error') {
    pages.clearSearchHits()
    write.toast(t('list.searchFailed'), 'error')
  }
}
