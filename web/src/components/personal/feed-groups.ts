/*
 * GDK-1058: consecutive events of one issue collapse into one row group.
 *
 * The feed renders as a flat list, and a busy issue arrives as a run of
 * adjacent rows that all repeat the same header (key + summary). Grouping
 * is render-only: me.feedItems keeps its order and read state, and this
 * module never mutates, filters, or reorders what the store shipped —
 * markEventRead / markEventsRead / markAllFeedRead are untouched.
 *
 * A group is a run of adjacent items with the same issue_key on the same
 * day — adjacency in the feed's own sort order, not a global regroup: a
 * different issue's event between two of yours splits the run. The day
 * boundary is inherited from the pre-GDK-1058 same-kind collapse ("New
 * comment ×2" never spanned midnight either). Items without a timestamp
 * never merge (`solo-${id}`), the pre-existing key, kept as-is.
 */
import type { FeedItem } from '../../lib/types'

export interface FeedGroup {
  id: number // group representative (first item) id — {#each} key + expand state key
  groupKey: string // adjacent-merge key
  items: FeedItem[]
}

/** Collapse adjacent same-issue, same-day feed items into row groups. */
export function groupFeedItems(items: FeedItem[]): FeedGroup[] {
  const out: FeedGroup[] = []
  for (const item of items) {
    const day = item.occurred_at ? new Date(item.occurred_at).toDateString() : `solo-${item.id}`
    // GDK-1058: the key is the issue, not the event type — a run of
    // comment/status/assignee on one issue is one row, and the old
    // same-kind collapse ("New comment ×2") is the single-kind case of it.
    const groupKey = `${item.issue_key}::${day}`
    const last = out[out.length - 1]
    if (last && last.groupKey === groupKey) last.items.push(item)
    else out.push({ id: item.id, groupKey, items: [item] })
  }
  return out
}

/** Distinct event types in a group, first-seen order, with per-kind counts —
 * the collapsed row's summary line. A single-kind group keeps the exact
 * pre-GDK-1058 shape (one label, one ×N badge). */
export function groupKindCounts(
  items: FeedItem[],
): { type: FeedItem['event_type']; count: number }[] {
  const out: { type: FeedItem['event_type']; count: number }[] = []
  for (const item of items) {
    const seen = out.find((k) => k.type === item.event_type)
    if (seen) seen.count++
    else out.push({ type: item.event_type, count: 1 })
  }
  return out
}
