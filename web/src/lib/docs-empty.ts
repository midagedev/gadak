/*
 * Empty-docs decision table (sidebar CTA + Documents view).
 *
 * An empty DOCS section has six causes, and "go connect a source" is right
 * for exactly one of them. The $derived that used to live in SidebarNav is
 * this table — DocsView's empty branch is the same class of combination.
 */

import type { MessageKey } from './i18n'

export type DocsEmptyState = 'off' | 'syncing' | 'failed' | 'never' | 'empty' | 'unavailable'

export type DocsEmptyRun = { error?: string }

export function docsEmptyState(input: {
  hasDocsServer: boolean
  confluenceEnabled: boolean
  fetchingDocuments: boolean
  confluenceRuns: DocsEmptyRun[] | null
}): DocsEmptyState {
  // First question, before "is it configured": does this deployment have a
  // docs server to configure at all. A static snapshot is not "off" — there
  // is no Settings screen that could switch it on.
  if (!input.hasDocsServer) return 'unavailable'
  if (!input.confluenceEnabled) return 'off'
  // Only while the mirror is fetching *documents*. An issue pass is the sync
  // row's business, not this section's — that split is what made one mirror
  // look like two.
  if (input.fetchingDocuments) return 'syncing'
  if (input.confluenceRuns === null) return 'never' // not asked yet — never claim failure
  if (input.confluenceRuns[0]?.error) return 'failed'
  if (input.confluenceRuns.length === 0) return 'never'
  return 'empty'
}

export type DocsEmptyCopy = {
  titleKey: MessageKey
  hintKey: MessageKey | null
  /** Syncing titles the busy sentence when the chip has one; else titleKey. */
  titlePrefersBusy: boolean
}

export function docsEmptyCopy(state: DocsEmptyState): DocsEmptyCopy {
  switch (state) {
    case 'unavailable':
      return { titleKey: 'sidebar.docsUnavailable', hintKey: null, titlePrefersBusy: false }
    case 'syncing':
      return { titleKey: 'sidebar.docsSyncing', hintKey: null, titlePrefersBusy: true }
    case 'failed':
      return {
        titleKey: 'sidebar.docsFetchFailed',
        hintKey: 'sidebar.docsFetchFailedHint',
        titlePrefersBusy: false,
      }
    case 'never':
      return {
        titleKey: 'sidebar.docsNotFetched',
        hintKey: 'sidebar.docsNotFetchedHint',
        titlePrefersBusy: false,
      }
    case 'empty':
      return {
        titleKey: 'sidebar.docsEmptySpaces',
        hintKey: 'sidebar.docsEmptySpacesHint',
        titlePrefersBusy: false,
      }
    default:
      return {
        titleKey: 'sidebar.docsNoneTitle',
        hintKey: 'sidebar.docsNoneHint',
        titlePrefersBusy: false,
      }
  }
}

/** 'never' is the one state the user can act on without leaving the sidebar. */
export type DocsEmptyClick = 'none' | 'sync' | 'settings'

export function docsEmptyClickAction(state: DocsEmptyState): DocsEmptyClick {
  if (state === 'unavailable' || state === 'syncing') return 'none'
  if (state === 'never') return 'sync'
  return 'settings'
}

export function docsEmptyGlyph(
  state: DocsEmptyState,
): 'warning' | 'refresh' | 'search-x' | 'settings' {
  if (state === 'failed') return 'warning'
  if (state === 'syncing' || state === 'never') return 'refresh'
  if (state === 'empty' || state === 'unavailable') return 'search-x'
  return 'settings'
}

export type DocsListEmptyKind = 'filter-text' | 'filter-label' | 'viewed' | 'recent'

/** Documents view empty branch. Null when the list has rows. */
export function docsListEmptyKind(input: {
  empty: boolean
  filtering: boolean
  hasNeedle: boolean
  tab: 'viewed' | 'updated' | 'author'
}): DocsListEmptyKind | null {
  if (!input.empty) return null
  if (input.filtering) return input.hasNeedle ? 'filter-text' : 'filter-label'
  if (input.tab === 'viewed') return 'viewed'
  return 'recent'
}
