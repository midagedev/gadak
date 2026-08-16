/*
 * The URL's second job: naming where you are, not what you filter.
 *
 * Every param key this app reads belongs to exactly one of three categories:
 *
 *   1. View state — the filters and display that describe the list. Owned by
 *      VIEW_PARAM_KEYS / isViewParam in view-config; saved views serialize
 *      exactly those, so nothing below may ever leak into one.
 *   2. Place state — which panel and which screen you are on. That is this
 *      registry. In the URL, deliberately never in a saved view: a place is
 *      where one person is standing, not part of the view they would share.
 *   3. Neither — state a link must not impose on whoever opens it (below).
 *
 * This registry is the only door to category 2: bindParam / bindParams
 * (lib/url-sync) accept no key that is not listed here, so a URL param cannot
 * quietly start existing — adding one means appearing in a list a reviewer
 * reads, next to the exclusions below. That list is where the exclusions are
 * actually enforced; a comment in App cannot refuse a future binding.
 *
 * Why the categories matter: a gadak:// deep link is this hash carried over a
 * URL scheme (internal/deeplink), and the Go side passes the query through
 * opaquely — a param becomes linkable the moment it reaches the URL, with no
 * further work anywhere. `gadak://view/w/oss?person=dev@example.com` needs no
 * Go change, which is exactly why the door has to be on this side.
 *
 * Deliberately NOT in the URL — never add these:
 *
 *   - Compose and write flows (`write.newIssueOpen`, `write.settingsOpen`,
 *     `triage.commentKey`): a link that prefills a form someone is about to
 *     submit is a phishing surface, and it is the same rule that keeps verbs
 *     out of the gadak:// scheme (internal/deeplink's package comment: a link
 *     says *where to go*, never what to do). Note the split inside "settings":
 *     `settings` below is the server settings document — a place you can point
 *     someone at — while `write.settingsOpen` is the personal Jira-token form,
 *     a credential entry, and stays unaddressable.
 *   - Transient affordances (`paletteOpen`, `shortcutsOpen` in App): keystroke
 *     UI, not a place; a shared link should not open someone's palette.
 *   - Per-browser return paths (the docs space tab): already reasoned at the
 *     doc-restore comment in App — a path this browser remembers (localStorage)
 *     is not something a link should impose on the person opening it.
 */

/** Params describing which panel/screen you are on. In the URL; never in a
 *  saved view. */
export const PLACE_PARAM_KEYS = [
  'issue',
  'doc',
  'person',
  'space',
  'docs',
  'dview',
  'hist',
  'feed',
  'settings',
] as const
export type PlaceParamKey = (typeof PLACE_PARAM_KEYS)[number]

/**
 * The static view-param aliases, mirrored from view-config's VIEW_PARAM_KEYS —
 * which is typed `string[]` and so cannot carry its literals into a union.
 * url-state.test.ts asserts the two lists hold the same keys, so a new alias
 * in view-config fails the suite until it is registered here too.
 */
export const VIEW_ALIAS_KEYS = [
  'q',
  'sc',
  'st',
  'as',
  'rp',
  'gr',
  'lb',
  'pr',
  'sv',
  'ty',
  'co',
  'fx',
  'qr',
  'qs',
  'qi',
  'ds',
  'pj',
  'spj',
  'ks',
  'cf',
  'ct',
  'uf',
  'ut',
  'fl',
  'g',
  's',
  'd',
  'cl',
] as const
export type ViewParamKey = (typeof VIEW_ALIAS_KEYS)[number]

/**
 * Every key the app gives meaning to: a place param, a view param, or a
 * discovered `f.<alias>` axis. The last is an open set — field discovery adds
 * axes at runtime — so `isViewParam` (view-config) stays the runtime
 * authority for that half; this union spells only the closed parts.
 */
export type UrlParamKey = PlaceParamKey | ViewParamKey | `f.${string}`

const PLACE_KEYS: ReadonlySet<string> = new Set(PLACE_PARAM_KEYS)

/** Place half of the two categories; `isViewParam` (view-config) is the other. */
export function isPlaceParam(key: string): boolean {
  return PLACE_KEYS.has(key)
}
