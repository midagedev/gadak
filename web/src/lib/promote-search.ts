/*
 * GDK-164: one-shot promotion of querystring-shaped deep links into the hash.
 *
 * The app is a hash router. External tools (Raycast, Slack, a pasted doc) emit
 * `/?issue=KEY` with no `#/`. Leaving that in location.search is a silent
 * no-op — the worst outcome named in internal/deeplink's package comment.
 * At boot, any search param the app gives meaning to (isPlaceParam /
 * isViewParam, including `f.<alias>`) moves into the hash; the hash wins on
 * key conflict; unknown search params stay put.
 */

import { isPlaceParam } from './url-state'
import { isViewParam } from './view-config'
import { parseHash, serialize } from './router.svelte'

export interface PromotedUrl {
  hash: string
  search: string
}

function isAppParam(key: string): boolean {
  return isPlaceParam(key) || isViewParam(key)
}

/** Pure merge: `(search, hash) → { hash, search }`. No DOM. */
export function promoteSearchToHash(search: string, hash: string): PromotedUrl {
  const rawSearch = search.startsWith('?') ? search.slice(1) : search
  if (!rawSearch) return { hash, search }

  const searchParams = new URLSearchParams(rawSearch)
  const appKeys: string[] = []
  const seen = new Set<string>()
  for (const key of searchParams.keys()) {
    if (seen.has(key)) continue
    seen.add(key)
    if (isAppParam(key)) appKeys.push(key)
  }
  if (appKeys.length === 0) return { hash, search }

  const { path, params: hashParams } = parseHash(hash)
  for (const key of appKeys) {
    if (hashParams.has(key)) continue
    for (const value of searchParams.getAll(key)) hashParams.append(key, value)
  }

  const kept = new URLSearchParams()
  for (const [key, value] of searchParams) {
    if (!isAppParam(key)) kept.append(key, value)
  }
  const keptQs = kept.toString()
  return {
    hash: serialize({ path, params: hashParams }),
    search: keptQs ? '?' + keptQs : '',
  }
}

/**
 * Apply the promotion to the live location. history.replaceState does not
 * fire hashchange; router.svelte.ts initializes `current` from location.hash
 * at import time, so a hash that actually moved needs a hashchange for the
 * reactive state to match before App mounts.
 */
export function applySearchPromotion(): void {
  if (typeof location === 'undefined' || typeof history === 'undefined') return
  const prevHash = location.hash
  const next = promoteSearchToHash(location.search, prevHash)
  if (next.search === location.search && next.hash === prevHash) return
  history.replaceState(null, '', location.pathname + next.search + next.hash)
  if (typeof window !== 'undefined' && next.hash !== prevHash) {
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  }
}
