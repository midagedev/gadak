/*
 * Which URL param owns which dimension of the screen (GDK-880).
 *
 * Place params (url-state PLACE_PARAM_KEYS) already split "where you are"
 * from "what you filter". This module is the next axis: among those place
 * params, which ones own the main column, which own the right panel, and
 * which are neither (a dialog, a modifier of another param).
 *
 * The dashboard `open` handler and App.svelte's restore-before-bind boot
 * both read from PLACE_DIMENSION. A new PlaceParamKey without a row here
 * is a type error; a new column/panel/other key without a matching identity
 * map entry is a type error (`satisfies`); a boot restore that does not
 * read the identity map is caught by place-dimension.test.ts reading
 * App.svelte's source. classifyOpenParams iterates the table, so a param
 * classified here cannot be treated as the other dimension there.
 */

import { PLACE_PARAM_KEYS, type PlaceParamKey } from './url-state'

/** Dimension of the screen a registered place param owns. */
export type PlaceDimension = 'column' | 'panel' | 'other'

/**
 * Exhaustive over PlaceParamKey: adding a param to PLACE_PARAM_KEYS without
 * a row here does not compile. Column / panel / other identity maps below
 * are `satisfies`-keyed off this table, so a dimension change without an
 * identity-map update also does not compile.
 */
export const PLACE_DIMENSION: { readonly [K in PlaceParamKey]: PlaceDimension } = {
  dash: 'column',
  docs: 'column',
  hist: 'column',
  space: 'column',
  feed: 'column',
  issue: 'panel',
  doc: 'panel',
  person: 'panel',
  // Modifier of `space`; does not own the column by itself.
  dview: 'other',
  // Settings dialog — overlay, not a column or the right panel.
  settings: 'other',
}

export type ColumnParamKey = {
  [K in PlaceParamKey]: (typeof PLACE_DIMENSION)[K] extends 'column' ? K : never
}[PlaceParamKey]

export type PanelParamKey = {
  [K in PlaceParamKey]: (typeof PLACE_DIMENSION)[K] extends 'panel' ? K : never
}[PlaceParamKey]

export type OtherParamKey = {
  [K in PlaceParamKey]: (typeof PLACE_DIMENSION)[K] extends 'other' ? K : never
}[PlaceParamKey]

export const COLUMN_PARAM_KEYS: readonly ColumnParamKey[] = PLACE_PARAM_KEYS.filter(
  (k): k is ColumnParamKey => PLACE_DIMENSION[k] === 'column',
)
export const PANEL_PARAM_KEYS: readonly PanelParamKey[] = PLACE_PARAM_KEYS.filter(
  (k): k is PanelParamKey => PLACE_DIMENSION[k] === 'panel',
)
export const OTHER_PARAM_KEYS: readonly OtherParamKey[] = PLACE_PARAM_KEYS.filter(
  (k): k is OtherParamKey => PLACE_DIMENSION[k] === 'other',
)

/** Identity map so boot reads `COLUMN_PARAM.dash` rather than the string `'dash'`. */
export const COLUMN_PARAM = {
  dash: 'dash',
  docs: 'docs',
  hist: 'hist',
  space: 'space',
  feed: 'feed',
} as const satisfies { readonly [K in ColumnParamKey]: K }

/** Identity map so boot reads `PANEL_PARAM.issue` rather than the string `'issue'`. */
export const PANEL_PARAM = {
  issue: 'issue',
  doc: 'doc',
  person: 'person',
} as const satisfies { readonly [K in PanelParamKey]: K }

/** Identity map for place params that own neither column nor panel. */
export const OTHER_PARAM = {
  dview: 'dview',
  settings: 'settings',
} as const satisfies { readonly [K in OtherParamKey]: K }

/** Dimension of one param key. Unregistered keys (view params, unknown) are `other`. */
export function placeDimension(key: string): PlaceDimension {
  if ((PLACE_PARAM_KEYS as readonly string[]).includes(key)) {
    return PLACE_DIMENSION[key as PlaceParamKey]
  }
  return 'other'
}

/**
 * What an `open` hash does to the main column.
 *
 *  1. `column` — names a column param (`dash`, `docs`, `hist`, `space`,
 *     `feed`) → that view takes the column.
 *  2. `panel` — names *only* panel params (`issue`, `doc`, `person`) → the
 *     column is untouched; only the panel opens.
 *  3. `list` — anything else (filter/view params, unknown params, mixed
 *     panel+filter, no params) → the column goes to the list.
 *
 * Rule 2 is strict: `#/?issue=K&sc=new` is `list`, not `panel`.
 */
export type OpenRule = 'column' | 'panel' | 'list'

export function classifyOpenParams(params: URLSearchParams): OpenRule {
  let column = false
  let panel = false
  let other = false
  let any = false
  for (const key of params.keys()) {
    any = true
    const dim = placeDimension(key)
    if (dim === 'column') column = true
    else if (dim === 'panel') panel = true
    else other = true
  }
  if (!any) return 'list'
  if (column) return 'column'
  if (panel && !other) return 'panel'
  return 'list'
}

/**
 * Keep the current column (and anything else that is not a panel param),
 * replace the panel identity with what the incoming hash named.
 */
export function mergePanelParams(
  current: URLSearchParams,
  incoming: URLSearchParams,
): URLSearchParams {
  const next = new URLSearchParams(current)
  for (const key of PANEL_PARAM_KEYS) next.delete(key)
  for (const [key, value] of incoming) next.set(key, value)
  return next
}

export interface OpenResolution {
  rule: OpenRule
  params: URLSearchParams
}

/**
 * Decide the next hash for a dashboard `open`. Panel-only hashes merge into
 * the current params (so `dash` survives); every other rule replaces the
 * hash wholesale, which is what drops `dash` and hands the column over.
 */
export function resolveOpen(
  current: URLSearchParams,
  incoming: URLSearchParams,
): OpenResolution {
  const rule = classifyOpenParams(incoming)
  if (rule === 'panel') {
    return { rule, params: mergePanelParams(current, incoming) }
  }
  return { rule, params: new URLSearchParams(incoming) }
}
