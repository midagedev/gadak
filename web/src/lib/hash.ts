/*
 * The hash grammar, as a pure module: `#/path?a=b` ⇄ { path, params }.
 *
 * Split out of router.svelte.ts (GDK-164): the boot-time querystring
 * promotion must read and rebuild the hash with the same grammar the router
 * lives by, but router.svelte.ts uses runes and cannot be imported from the
 * rune-free vitest `unit` project. One grammar, zero reactivity here; the
 * router re-exports these so its importers see one surface.
 */

export interface Route {
  /** Path part of the hash (leading `#` stripped, before `?`). Default `/`. e.g. `/board` */
  path: string
  /** Parsed query string after the path. */
  params: URLSearchParams
}

export function parseHash(hash: string): Route {
  // location.hash is "#..." or ""
  const raw = hash.startsWith('#') ? hash.slice(1) : hash
  const qIndex = raw.indexOf('?')
  const path = (qIndex === -1 ? raw : raw.slice(0, qIndex)) || '/'
  const query = qIndex === -1 ? '' : raw.slice(qIndex + 1)
  return { path: path.startsWith('/') ? path : '/' + path, params: new URLSearchParams(query) }
}

/** Route → hash string (`#/path?a=b`). No trailing `?` when the query is empty. */
export function serialize(route: Route): string {
  const qs = route.params.toString()
  return '#' + route.path + (qs ? '?' + qs : '')
}
