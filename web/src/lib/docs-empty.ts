/*
 * Empty-docs decision table (sidebar CTA + Documents view).
 *
 * An empty DOCS section has seven causes, and "go connect a source" is right
 * for exactly one of them. The $derived that used to live in SidebarNav is
 * this table — DocsView's empty branch is the same class of combination.
 */

import type { MessageKey } from './i18n'

export type DocsEmptyState =
  | 'off'
  | 'syncing'
  | 'failed'
  | 'loadfailed'
  | 'never'
  | 'empty'
  | 'unavailable'
  /** A built-in (gadak-origin) tracker whose wiki has no pages yet. Nothing
   *  to configure and nothing to fetch — the page you write is the first one. */
  | 'local-empty'

export type DocsEmptyRun = { error?: string }

export function docsEmptyState(input: {
  hasDocsServer: boolean
  confluenceEnabled: boolean
  fetchingDocuments: boolean
  /** The page-index request itself failed (pages store's loadFailed). */
  indexLoadFailed: boolean
  confluenceRuns: DocsEmptyRun[] | null
  /** The workspace's origin is gadak's own tracker (GDK-1342). */
  localOrigin?: boolean
}): DocsEmptyState {
  // First question, before "is it configured": does this deployment have a
  // docs server to configure at all. A static snapshot is not "off" — there
  // is no Settings screen that could switch it on.
  if (!input.hasDocsServer) return 'unavailable'
  // A built-in tracker has a wiki of its own; "turn on Confluence" and
  // "change the space selection" are both the wrong errand for it.
  if (input.localOrigin) return 'local-empty'
  if (!input.confluenceEnabled) return 'off'
  // Only while the mirror is fetching *documents*. An issue pass is the sync
  // row's business, not this section's — that split is what made one mirror
  // look like two.
  if (input.fetchingDocuments) return 'syncing'
  // An errored run outranks a failed index request (GDK-1067): it is usually
  // the *cause* of that failure, and it carries the evidence — the run's
  // error string is the row tooltip, and its hint names the recovery.
  if (input.confluenceRuns?.[0]?.error) return 'failed'
  // The index request failed on its own. It must outrank the run history
  // below: when the same outage kills the runs fetch, `confluenceRuns`
  // stays null and "not asked yet" would stand in for an unanswered
  // request — the exact masquerade this axis exists to end.
  if (input.indexLoadFailed) return 'loadfailed'
  if (input.confluenceRuns === null) return 'never' // not asked yet — never claim failure
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
    case 'local-empty':
      return { titleKey: 'sidebar.docsLocalEmpty', hintKey: null, titlePrefersBusy: false }
    case 'syncing':
      return { titleKey: 'sidebar.docsSyncing', hintKey: null, titlePrefersBusy: true }
    case 'failed':
      return {
        titleKey: 'sidebar.docsFetchFailed',
        hintKey: 'sidebar.docsFetchFailedHint',
        titlePrefersBusy: false,
      }
    // GDK-1067: DocsView already says this sentence for the same failure
    // (GDK-1054) — one failure, one string. No hint on purpose: 'common.retry'
    // as a hint line would give the CTA an accessible name containing
    // "Retry" and collide with e2e lookups of the *other* Retry button.
    case 'loadfailed':
      return { titleKey: 'docs.loadFailed', hintKey: null, titlePrefersBusy: false }
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

/** 'never' and 'loadfailed' are the states the user can act on without
 *  leaving the sidebar (sync, retry). */
export type DocsEmptyClick = 'none' | 'sync' | 'settings' | 'retry'

export function docsEmptyClickAction(state: DocsEmptyState): DocsEmptyClick {
  if (state === 'unavailable' || state === 'syncing' || state === 'local-empty') return 'none'
  if (state === 'never') return 'sync'
  if (state === 'loadfailed') return 'retry'
  return 'settings'
}

export function docsEmptyGlyph(
  state: DocsEmptyState,
): 'warning' | 'refresh' | 'search-x' | 'settings' | 'file' {
  if (state === 'failed' || state === 'loadfailed') return 'warning'
  if (state === 'syncing' || state === 'never') return 'refresh'
  if (state === 'empty' || state === 'unavailable') return 'search-x'
  if (state === 'local-empty') return 'file'
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
