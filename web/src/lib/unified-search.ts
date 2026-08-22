/*
 * Palette unified search — debounce, stale-response drop, and key dedupe.
 *
 * The list SearchBox still narrows the in-memory pool (and AND-s the active
 * chips). This module is the other path: GET search/?q= over items_fts, which
 * already walks issue and document titles, bodies, and comments together
 * (internal/store/read.go Search). The palette owns assembly; this file owns
 * the race rules so a slow response cannot paint a query the user has left.
 *
 * fetch is injected so this module does not import api.ts — the opener
 * binding is the only other thing living here, so SearchBox can ask App to
 * open the palette without a store.
 */

import type { PageLite, SearchMatch, SearchResponse } from './types'

/** Matches createUserSearch's default; spec band is 250–300ms. */
const UNIFIED_DEBOUNCE_MS = 250

/** Palette preview caps — the list is where the full set is read. */
const UNIFIED_ISSUE_LIMIT = 8
const UNIFIED_PAGE_LIMIT = 6

/** Server Search() uses 50 when the caller sends limit <= 0. */
export const UNIFIED_FETCH_LIMIT = 50

export type UnifiedStatus = 'idle' | 'pending' | 'loading' | 'ready' | 'error'

export interface UnifiedHit {
  kind: 'issue' | 'page'
  key: string
  title: string
  match?: SearchMatch
  page?: PageLite
}

export interface UnifiedProjection {
  issues: UnifiedHit[]
  pages: UnifiedHit[]
  total: number
  /** True when the server returned more than the palette preview will show. */
  truncated: boolean
}

export type UnifiedView =
  | { status: 'idle'; query: string }
  | { status: 'pending' | 'loading'; query: string }
  | { status: 'ready'; query: string; response: SearchResponse }
  | { status: 'error'; query: string; error: string }

export function emptyUnifiedView(query = ''): UnifiedView {
  return { status: 'idle', query }
}

/**
 * A response is stale when its request id is no longer the live session id —
 * a newer query started, or cancel() ran (palette closed / input cleared).
 */
export function isStale(requestId: number, currentId: number): boolean {
  return requestId !== currentId
}

export function excludeLocalKeys(keys: string[], local: ReadonlySet<string>): string[] {
  if (local.size === 0) return keys.slice()
  return keys.filter((k) => !local.has(k))
}

export function projectUnifiedHits(
  res: SearchResponse,
  localIssueKeys: ReadonlySet<string> = new Set(),
  localPageKeys: ReadonlySet<string> = new Set(),
): UnifiedProjection {
  const matches = res.matches ?? {}
  const issueKeys = excludeLocalKeys(res.keys ?? [], localIssueKeys)
  const pages = (res.pages ?? []).filter((p) => !localPageKeys.has(p.key))
  const shownIssues = issueKeys.slice(0, UNIFIED_ISSUE_LIMIT)
  const shownPages = pages.slice(0, UNIFIED_PAGE_LIMIT)
  return {
    issues: shownIssues.map((key) => ({
      kind: 'issue',
      key,
      title: '',
      match: matches[key],
    })),
    pages: shownPages.map((page) => ({
      kind: 'page',
      key: page.key,
      title: page.title,
      match: matches[page.key],
      page,
    })),
    total: res.total ?? 0,
    truncated: issueKeys.length > UNIFIED_ISSUE_LIMIT || pages.length > UNIFIED_PAGE_LIMIT,
  }
}

export interface UnifiedSearchHandle {
  request(query: string): void
  cancel(): void
}

export function createUnifiedSearch(opts: {
  fetch: (q: string) => Promise<SearchResponse>
  onView: (view: UnifiedView) => void
  debounceMs?: number
}): UnifiedSearchHandle {
  const debounceMs = opts.debounceMs ?? UNIFIED_DEBOUNCE_MS
  let seq = 0
  let timer: ReturnType<typeof setTimeout> | null = null

  function cancel(): void {
    if (timer !== null) {
      clearTimeout(timer)
      timer = null
    }
    seq += 1
  }

  function request(query: string): void {
    const q = query.trim()
    cancel()
    if (!q) {
      opts.onView(emptyUnifiedView())
      return
    }
    const my = seq
    opts.onView({ status: 'pending', query: q })
    timer = setTimeout(() => {
      timer = null
      if (isStale(my, seq)) return
      opts.onView({ status: 'loading', query: q })
      void opts.fetch(q).then(
        (response) => {
          if (isStale(my, seq)) return
          opts.onView({ status: 'ready', query: q, response })
        },
        (err: unknown) => {
          if (isStale(my, seq)) return
          opts.onView({
            status: 'error',
            query: q,
            error: err instanceof Error ? err.message : 'error',
          })
        },
      )
    }, debounceMs)
  }

  return { request, cancel }
}

/* ── Palette opener (SearchBox → App) ── */

let paletteOpener: (() => void) | null = null

export function bindPaletteOpener(fn: () => void): () => void {
  paletteOpener = fn
  return () => {
    if (paletteOpener === fn) paletteOpener = null
  }
}

export function requestOpenPalette(): void {
  paletteOpener?.()
}

let shortcutsOpener: (() => void) | null = null

export function bindShortcutsOpener(fn: () => void): () => void {
  shortcutsOpener = fn
  return () => {
    if (shortcutsOpener === fn) shortcutsOpener = null
  }
}

export function requestOpenShortcuts(): void {
  shortcutsOpener?.()
}

/** Same platform test ShortcutsDialog uses for ⌘ vs Ctrl. */
export function modifierSymbol(): '⌘' | 'Ctrl' {
  if (typeof navigator === 'undefined') return 'Ctrl'
  return /Mac|iP(hone|ad)/.test(navigator.platform) ? '⌘' : 'Ctrl'
}

export function paletteShortcutLabel(): string {
  return `${modifierSymbol()}K`
}
