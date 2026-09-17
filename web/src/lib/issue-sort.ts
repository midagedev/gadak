/*
 * The list comparator — one owner for "what does sort=<key> mean" (GDK-1992).
 *
 * It used to live inside `stores/filters.svelte.ts`, which the phone cannot
 * import: that file is a rune store wired to the desk's URL, its search
 * relevance context and its server order. So the phone had its own fixed
 * order and simply ignored every view's `display` block — and the desk's
 * built-in views carry four of them, which is how *Handed off* came out
 * newest-first on a phone when the view exists to read oldest-first.
 *
 * What moved here is the part that is a pure function of two rows: the seven
 * keys a stored `IssueLite` can answer. `relevance` (needs search scores) and
 * `keys` (needs a server order) stay in the store, because they are not
 * properties of the rows at all.
 *
 * The tie-break is `updated_at` newest-first everywhere, so the top of a list
 * stays the live work whichever axis put it there. A caller that needs a
 * total order adds its own final key (the phone does, for keyed-each
 * stability); the desk does not care and does not pay for it.
 */
import { prioritySortRank, type SortDir, type SortKey } from './view-config'

/**
 * The row shape the comparator reads — the intersection the desk's
 * `IssueLite` and the phone's `Pick` of it both satisfy. Stated structurally
 * rather than as `IssueLite` so a phone row, which carries 27 of the owner's
 * keys, is assignable without widening the phone's own type.
 */
export interface SortableIssue {
  issue_key: string
  created_at: string | null
  updated_at: string | null
  status_changed_at: string | null
  duedate?: string | null
  started_at?: string | null
  reopen_count: number
  priority_rank: number | null
}

/**
 * The keys this comparator answers: `SORT_KEY_VALUES` less `relevance` and
 * `keys`. Seven of the catalog's nine. A caller holding a stored view reads
 * `isListSortKey(display.sort)` before trusting it, the way the phone reads
 * `HONORED_AXES` before trusting a filter.
 */
export const LIST_SORT_KEYS = [
  'updated',
  'created',
  'status_changed',
  'started',
  'due',
  'priority',
  'reopen_count',
] as const
export type ListSortKey = (typeof LIST_SORT_KEYS)[number]

export function isListSortKey(value: unknown): value is ListSortKey {
  return typeof value === 'string' && (LIST_SORT_KEYS as readonly string[]).includes(value)
}

function cmpStr(a: string | null | undefined, b: string | null | undefined, dir: 1 | -1): number {
  const av = a ?? ''
  const bv = b ?? ''
  return av < bv ? -dir : av > bv ? dir : 0
}

/**
 * Orders two rows on one axis. `sort` is the whole catalog rather than
 * `ListSortKey` so the store can hand its own value through unchanged; a key
 * this comparator does not answer reads as `updated`, which is the default
 * the desk's switch already had.
 */
export function compareIssues(
  a: SortableIssue,
  b: SortableIssue,
  sort: SortKey,
  dir: SortDir,
): number {
  const d: 1 | -1 = dir === 'asc' ? 1 : -1
  switch (sort) {
    case 'created':
      return cmpStr(a.created_at, b.created_at, d)
    case 'status_changed':
    case 'started': {
      // Age (T4): asc = longest first. 'status_changed' is age in the
      // current status; 'started' is work item age — since started_at, the
      // flow canon's clock (2026-09-07), falling back to status_changed_at
      // for an issue that has not started or an origin with no history.
      // aging-in-progress reads 'started'. A missing/unparseable stamp is
      // "no evidence", not "oldest": last in both directions. Ties fall to
      // newest updated_at so the top of the list stays the live work.
      const stamp =
        sort === 'started'
          ? (it: SortableIssue): string => it.started_at ?? it.status_changed_at ?? ''
          : (it: SortableIssue): string => it.status_changed_at ?? ''
      const at = (it: SortableIssue): number => {
        const t = Date.parse(stamp(it))
        return Number.isFinite(t) ? t : Number.NaN
      }
      const av = at(a)
      const bv = at(b)
      const aMiss = !Number.isFinite(av)
      const bMiss = !Number.isFinite(bv)
      if (aMiss && bMiss) return cmpStr(a.updated_at, b.updated_at, -1)
      if (aMiss) return 1
      if (bMiss) return -1
      const diff = (av - bv) * d
      return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
    }
    case 'due':
      return cmpStr(a.duedate ?? '', b.duedate ?? '', d)
    case 'reopen_count': {
      const diff = (a.reopen_count - b.reopen_count) * d
      return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
    }
    case 'priority': {
      // priority_rank: lower = higher priority. Unset is 0 on the wire (not
      // null) and always sorts last, so untriaged never outranks Highest.
      const ar = prioritySortRank(a.priority_rank)
      const br = prioritySortRank(b.priority_rank)
      const aUnset = ar === Number.POSITIVE_INFINITY
      const bUnset = br === Number.POSITIVE_INFINITY
      if (aUnset && bUnset) return cmpStr(a.updated_at, b.updated_at, -1)
      if (aUnset) return 1
      if (bUnset) return -1
      const diff = (ar - br) * d
      return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
    }
    case 'updated':
    default:
      return cmpStr(a.updated_at, b.updated_at, d)
  }
}
