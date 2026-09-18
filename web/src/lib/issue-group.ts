/*
 * The list grouper — one owner for "what does group_by=<axis> mean"
 * (GDK-1993), lifted out of `stores/filters.svelte.ts` the way the
 * comparator was one round earlier (GDK-1992, `issue-sort.ts`).
 *
 * Same reason as that round, and the same shape of defect behind it. The
 * store is a rune store wired to the desk's URL, its member catalog, its
 * issue pool and its runtime config, so the phone cannot import it — and so
 * the phone had a grouper of its own that bucketed by priority everywhere
 * but the sprint scope and ignored every view's `display.group_by`. Two
 * surfaces disagreeing about what a view *is* is worse than either answer
 * alone.
 *
 * What moved here is the part that is a pure function of the rows plus three
 * named lookups. Those three are what tied the grouper to the store — an
 * epic's summary, an actor's display name, and the org's group→product map —
 * and they are passed in rather than reached for, so the desk supplies its
 * store and the phone supplies what it has (a pool of its own for epics, and
 * nothing for the two axes it does not offer).
 *
 * Group *counts* did not move: they are the desk's own reading (per-category
 * and per-severity tallies under each header), the phone's sections have no
 * such line, and they are an accumulation over items rather than a property
 * of the axis.
 */
import { categoryLabel, t } from './i18n'
import { prioritySortRank, type GroupBy } from './view-config'

/**
 * The row shape the grouper reads — stated structurally, like
 * `SortableIssue`, so the phone's narrower row is assignable without
 * widening the phone's own type. Everything the phone's row does not carry
 * is optional, and an axis that reads only optional fields is simply an axis
 * the phone does not offer (`GROUPABLE_ON_LITE` below).
 */
export interface GroupableIssue {
  issue_key: string
  status: string | null
  status_id?: string | null
  status_category: string | null
  assignee: string | null
  assignee_id?: string | null
  assignee_email?: string | null
  priority: string | null
  priority_rank: number | null
  issue_type: string | null
  issue_type_id?: string | null
  epic_key?: string | null
  severity?: string | null
  team_group?: string | null
  development_test_result?: string | null
  qa_impact_state?: string | null
  qa_impact_label?: string
  source_project?: string | null
  actor_ids?: string[] | null
}

/**
 * The three reaches that used to be store reads. Every one is optional: a
 * caller that cannot answer gets the honest fallback (the epic's key instead
 * of its summary, the account id instead of a name, "no product"), never a
 * wrong bucket.
 */
export interface GroupLookups {
  epicSummary?: (key: string) => string | undefined
  actorName?: (accountId: string) => string | undefined
  productByGroup?: Record<string, { key: string; label: string } | undefined>
}

/** One bucket: its stable key, what to call it, and what fell into it. */
export interface IssueGrouping<T> {
  key: string
  label: string
  /** Monospace identifier rendered before the label (the epic's key). */
  prefix?: string
  items: T[]
}

/**
 * The axes a row with no desk-only columns can answer — the grouping twin of
 * `LIST_SORT_KEYS`. `severity`, `team_group`, `product`,
 * `development_test_result`, `qa_impact` and `actor` read fields the phone's
 * snapshot does not carry, so asking for one there is answered by the
 * catalog default rather than by a screen full of "(none)".
 */
export const GROUPABLE_ON_LITE = [
  'none',
  'status_category',
  'status',
  'assignee',
  'priority',
  'issue_type',
  'source_project',
  'epic',
] as const
export type LiteGroupBy = (typeof GROUPABLE_ON_LITE)[number]

export function isLiteGroupBy(v: unknown): v is LiteGroupBy {
  return typeof v === 'string' && (GROUPABLE_ON_LITE as readonly string[]).includes(v)
}

/** Status category buckets read in work order: doing, then queued, then done. */
const IN_RANK: Record<string, number> = { inprogress: 0, new: 1, done: 2 }

const CATEGORY_ALIASES: Record<string, 'new' | 'inprogress' | 'done'> = {
  new: 'new',
  indeterminate: 'inprogress',
  inprogress: 'inprogress',
  'in progress': 'inprogress',
  done: 'done',
  complete: 'done',
}

/** The three-bucket axis, tolerant of the aliases origins actually send. */
export function groupCategory(issue: GroupableIssue): 'new' | 'inprogress' | 'done' {
  return CATEGORY_ALIASES[(issue.status_category ?? '').toLowerCase()] ?? 'inprogress'
}

function personIdentity(issue: GroupableIssue): string | null {
  return issue.assignee_id || issue.assignee_email || null
}

/** The bucket one row falls into on one axis. An empty key is "none of them". */
export function groupKeyOf(
  issue: GroupableIssue,
  by: GroupBy,
  look: GroupLookups = {},
): { key: string; label: string; prefix?: string } {
  switch (by) {
    case 'status_category': {
      const category = groupCategory(issue)
      return { key: category, label: categoryLabel(category) }
    }
    case 'status':
      return {
        key: issue.status_id || issue.status || '(none)',
        label: issue.status || t('group.noStatus'),
      }
    case 'assignee': {
      const id = personIdentity(issue)
      return id
        ? { key: id, label: issue.assignee || issue.assignee_email || id }
        : { key: '', label: t('common.unassigned') }
    }
    case 'priority':
      return { key: issue.priority || '', label: issue.priority || t('group.noPriority') }
    case 'severity':
      return { key: issue.severity || '', label: issue.severity || t('group.noSeverity') }
    case 'team_group':
      return issue.team_group
        ? { key: issue.team_group, label: issue.team_group }
        : { key: '', label: t('common.unclassified') }
    case 'product':
      return (
        look.productByGroup?.[issue.team_group ?? ''] ?? { key: '', label: t('group.noProduct') }
      )
    case 'issue_type':
      return {
        key: issue.issue_type_id || issue.issue_type || '',
        label: issue.issue_type || t('group.noType'),
      }
    case 'development_test_result': {
      const result = issue.development_test_result?.trim()
      return result ? { key: result, label: result } : { key: 'none', label: t('group.none') }
    }
    case 'qa_impact':
      return issue.qa_impact_state
        ? { key: issue.qa_impact_state, label: issue.qa_impact_label ?? issue.qa_impact_state }
        : { key: '', label: t('group.qaIrrelevant') }
    case 'source_project':
      return {
        key: issue.source_project || '',
        label: issue.source_project || t('group.noProject'),
      }
    case 'epic': {
      const epicKey = issue.epic_key
      if (!epicKey) return { key: '', label: t('group.noEpic') }
      // The epic is mirrored like any other issue, so its summary is usually
      // already in the caller's pool — key alone when it is not (partial
      // mirror, a narrowed project set, or a caller with no pool at all).
      const summary = look.epicSummary?.(epicKey)
      return summary ? { key: epicKey, label: summary, prefix: epicKey } : { key: epicKey, label: epicKey }
    }
    default:
      return { key: '', label: '' }
  }
}

/**
 * Actor group keys — the one multi-membership axis (GDK-590). An issue a bot
 * and a human both touched lands in both buckets: "throughput per bot this
 * sprint" must not silently pick a winner. Keys are account ids; labels come
 * from the caller's member catalog, with the id itself as the fallback so an
 * email-hidden actor still gets a readable (if blunt) header.
 */
function actorGroupKeys(
  issue: GroupableIssue,
  look: GroupLookups,
): { key: string; label: string; prefix?: string }[] {
  const ids = issue.actor_ids ?? []
  if (ids.length === 0) return [{ key: '', label: t('group.noActor') }]
  return ids.map((id) => ({ key: id, label: look.actorName?.(id) ?? id }))
}

/**
 * Header order. Empty keys last everywhere — an unassigned or un-epiced pile
 * is the leftover, not the lead — then each axis' own reading: work order for
 * the ranked ones, the key for epics (so one project's epics stay adjacent
 * and a rename does not move the page), the label otherwise.
 */
export function compareGroups<T extends GroupableIssue>(
  a: IssueGrouping<T>,
  b: IssueGrouping<T>,
  by: GroupBy,
): number {
  const ae = a.key === ''
  const be = b.key === ''
  if (ae !== be) return ae ? 1 : -1
  if (by === 'priority') {
    return prioritySortRank(a.items[0]?.priority_rank) - prioritySortRank(b.items[0]?.priority_rank)
  }
  if (by === 'severity') {
    const rank: Record<string, number> = { Critical: 0, Major: 1, Minor: 2, Trivial: 3 }
    return (rank[a.key] ?? 99) - (rank[b.key] ?? 99)
  }
  if (by === 'status_category') return (IN_RANK[a.key] ?? 99) - (IN_RANK[b.key] ?? 99)
  if (by === 'product') {
    const rank: Record<string, number> = { cloud: 0, crown: 1, batch: 2, backoffice: 3 }
    return (rank[a.key] ?? 99) - (rank[b.key] ?? 99)
  }
  if (by === 'development_test_result') {
    const rank: Record<string, number> = { fail: 0, none: 1, pass: 2 }
    return (rank[a.key.toLowerCase()] ?? 99) - (rank[b.key.toLowerCase()] ?? 99)
  }
  if (by === 'qa_impact') {
    const rank: Record<string, number> = { blocking: 0, retest: 1, linked: 2, verified: 3 }
    return (rank[a.key] ?? 99) - (rank[b.key] ?? 99)
  }
  if (by === 'status') {
    const ac = IN_RANK[groupCategory(a.items[0])] ?? 99
    const bc = IN_RANK[groupCategory(b.items[0])] ?? 99
    if (ac !== bc) return ac - bc
  }
  if (by === 'epic') return a.key < b.key ? -1 : a.key > b.key ? 1 : 0
  return a.label < b.label ? -1 : a.label > b.label ? 1 : 0
}

/**
 * The list cut into headed buckets, in header order. `none` is one unnamed
 * bucket holding everything, which is what makes an ungrouped list the same
 * code path as a grouped one.
 *
 * Insertion order inside a bucket is the caller's order, so the sort a view
 * asked for survives the cut.
 */
export function groupIssues<T extends GroupableIssue>(
  list: T[],
  by: GroupBy,
  look: GroupLookups = {},
): IssueGrouping<T>[] {
  if (by === 'none') return [{ key: '', label: '', items: list }]
  const map = new Map<string, IssueGrouping<T>>()
  for (const issue of list) {
    const entries = by === 'actor' ? actorGroupKeys(issue, look) : [groupKeyOf(issue, by, look)]
    for (const { key, label, prefix } of entries) {
      const hit = map.get(key)
      if (hit) hit.items.push(issue)
      else map.set(key, { key, label, prefix, items: [issue] })
    }
  }
  return [...map.values()].sort((a, b) => compareGroups(a, b, by))
}
