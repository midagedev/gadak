/*
 * What the main column can show — the vocabulary, without the store.
 *
 * It lived in stores/column.svelte.ts, which is the right home for the state
 * but the wrong one for the words: a `.svelte.ts` module cannot be imported
 * outside the Svelte compiler, so nothing rune-free could read the set of
 * destinations. The palette-coverage gate (GDK-137) needs exactly that, and a
 * gate that had to re-type the list would be measuring its own copy.
 *
 * The store re-exports these, so its readers did not have to learn a new path.
 */

import type { FeedFocus } from './types'

/** Everything the main column can show. `list` is the resting state. */
export type ColumnView =
  | { view: 'list' }
  | { view: 'docs' }
  | { view: 'space'; key: string }
  | { view: 'history' }
  | { view: 'retro' }
  | { view: 'dashboard'; id: string }
  | { view: 'feed'; focus: FeedFocus }

/** The discriminant alone — what `is()`/`close()` key on. */
export type ColumnKind = ColumnView['view']

/**
 * The same set at runtime, so a test can iterate the column's destinations
 * instead of re-listing them. A destination added to ColumnView above and
 * forgotten here is a compile error (both directions are asserted below), not
 * a silently unmeasured screen.
 */
export const COLUMN_KINDS = [
  'list',
  'docs',
  'space',
  'history',
  'retro',
  'dashboard',
  'feed',
] as const satisfies readonly ColumnKind[]

// `satisfies` above proves the list holds no strangers. This is the other
// direction: a kind in the union that the list does not name.
type KindMissingFromList = Exclude<ColumnKind, (typeof COLUMN_KINDS)[number]>
const _columnKindsComplete: [KindMissingFromList] extends [never]
  ? true
  : KindMissingFromList = true
void _columnKindsComplete
