/*
 * Issue Navigator — lightweight hash router
 *
 * ⚠️ Filename note: contract §2 says `router.ts`, but exposing the hash via `$state`
 *  (runes) requires a `.svelte.ts` file so the compiler processes runes (plain `.ts`
 *  cannot). So this is `router.svelte.ts`; imports use `./lib/router.svelte`.
 *
 * Responsibility (foundation scope): expose the current hash reactively + parse/serialize
 *  `{ path, params }` + generic query-param get/set. **View state (filter/display)
 *  serialization is [explore]'s job** — we do not interpret what any key means.
 */

// The grammar itself lives in hash.ts (pure, importable without runes —
// GDK-164 needed it from the vitest unit project). Re-exported so this file
// stays the one surface router consumers import.
import { parseHash, serialize } from './hash'
export { parseHash, serialize } from './hash'
export type { Route } from './hash'
import type { Route } from './hash'

// Current hash state (reactive). Kept in sync via hashchange.
let current = $state<Route>(parseHash(typeof location !== 'undefined' ? location.hash : ''))

if (typeof window !== 'undefined') {
  window.addEventListener('hashchange', () => {
    current = parseHash(location.hash)
  })
}

/**
 * Reactive access to the current route. Reading `router.path` / `router.params`
 * inside a component or `$derived` auto-updates on hash change.
 */
export const router = {
  get path(): string {
    return current.path
  },
  get params(): URLSearchParams {
    return current.params
  },
}

/** Replace the whole hash (path + params). Pushes a new history entry. */
export function navigate(path: string, params?: URLSearchParams | Record<string, string>): void {
  const sp =
    params instanceof URLSearchParams
      ? params
      : new URLSearchParams((params ?? {}) as Record<string, string>)
  location.hash = serialize({ path, params: sp })
}

/**
 * Merge-update several query params — preserve the rest.
 * `null` removes that key. Path stays. `replace=true` rewrites the current
 * history entry instead of pushing (useful while scrolling/typing).
 */
export function setParams(next: Record<string, string | null>, replace = false): void {
  const sp = new URLSearchParams(current.params)
  for (const [k, v] of Object.entries(next)) {
    if (v === null) sp.delete(k)
    else sp.set(k, v)
  }
  const hash = serialize({ path: current.path, params: sp })
  if (replace) {
    history.replaceState(null, '', hash)
    // replaceState does not fire hashchange — sync manually
    current = parseHash(hash)
  } else {
    location.hash = hash
  }
}

/** Set one query param (sugar). */
export function setParam(key: string, value: string | null, replace = false): void {
  setParams({ [key]: value }, replace)
}
