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

/*
 * What a write does to history — the three rules every caller lives by:
 *
 *   1. a place (which issue, which filters, which screen) pushes, so back is
 *      the previous place;
 *   2. a dialog opening pushes one entry; closing it is `history.back()`
 *      (the binding's job, see url-sync — the marker below is how it knows
 *      the entry it is closing is the one it opened);
 *   3. continuous input — typing, a dialog's tabs, scrolling — replaces.
 *
 * And one entry per user action: the stores that mirror the URL write from
 * effects, which flush in a microtask, so opening an issue over a document
 * (`doc=` leaves, `issue=` arrives) or applying a view that also releases the
 * column reaches here as two pushes in one task. The second becomes a
 * replace — back then walks actions, not the writes that made them up.
 * A setTimeout(0) ends the task; a click, a key, a poll tick is the next one.
 * The next input also ends it outright: Chromium runs input ahead of timers,
 * so with the list still re-rendering after a typed query, the click that
 * opened an issue arrived before the timer and its push fell to a replace
 * (measured flaky in e2e/history-nav.spec.ts — back skipped the query).
 */
let pushedThisTask = false

if (typeof window !== 'undefined') {
  for (const type of ['pointerdown', 'keydown', 'input'] as const) {
    window.addEventListener(
      type,
      () => {
        pushedThisTask = false
      },
      { capture: true },
    )
  }
}

function commit(hash: string, replace: boolean, entry?: string): void {
  if (replace || pushedThisTask) {
    // A replace keeps the entry's marker: a dialog's tab switches must not
    // erase the sign that its open pushed this entry.
    history.replaceState(entry ? { entry } : history.state, '', hash)
  } else if (hash !== location.hash) {
    history.pushState(entry ? { entry } : null, '', hash)
    pushedThisTask = true
    setTimeout(() => {
      pushedThisTask = false
    }, 0)
  }
  // Neither fires hashchange — sync now, so a second write in the same flush
  // builds on this one instead of on the hash before it (GDK-1292).
  current = parseHash(hash)
}

/** Replace the whole hash (path + params). Pushes a new history entry. */
export function navigate(path: string, params?: URLSearchParams | Record<string, string>): void {
  const sp =
    params instanceof URLSearchParams
      ? params
      : new URLSearchParams((params ?? {}) as Record<string, string>)
  commit(serialize({ path, params: sp }), false)
}

/** Push a hash string as given — for a hash another process composed
 *  (`views open`), whose spelling (`ks=A,B` unescaped) is kept verbatim. */
export function pushHash(hash: string): void {
  commit(hash, false)
}

/**
 * Merge-update several query params — preserve the rest.
 * `null` removes that key. Path stays. `replace=true` rewrites the current
 * history entry instead of pushing (continuous input — rule 3 above).
 * `entry` marks a pushed entry so its owner can recognise it (rule 2).
 */
export function setParams(
  next: Record<string, string | null>,
  replace = false,
  entry?: string,
): void {
  const sp = new URLSearchParams(current.params)
  for (const [k, v] of Object.entries(next)) {
    if (v === null) sp.delete(k)
    else sp.set(k, v)
  }
  commit(serialize({ path: current.path, params: sp }), replace, entry)
}

/** Is the current history entry one pushed under this marker? */
export function atEntry(entry: string): boolean {
  const s = history.state as { entry?: unknown } | null
  return s !== null && typeof s === 'object' && s.entry === entry
}
