/*
 * GDK-1058: consecutive events of one issue collapse into one row group.
 * The grouping is a pure function (feed-groups.ts) precisely so this file
 * can pin it — the unit project has no svelte plugin (PersonalFeed.test.ts
 * documents the same limit), and the component's $derived only forwards.
 *
 * What is pinned here:
 *  - adjacency, not global regroup: a different issue's event between two
 *    of yours splits the run;
 *  - the pre-GDK-1058 same-kind collapse ("New comment ×2") survives as
 *    the single-kind case;
 *  - the day boundary survives (a run never spans midnight);
 *  - timestamp-less items never merge;
 *  - the collapsed row's kind summary: distinct kinds, first-seen order,
 *    per-kind counts — one entry for a single-kind group.
 */
import { describe, expect, test } from 'vitest'
import type { FeedItem, FeedEventType } from '../../lib/types'
import { groupFeedItems, groupKindCounts } from './feed-groups'

let nextId = 1

function item(over: Partial<FeedItem> & { issue_key: string }): FeedItem {
  return {
    id: nextId++,
    event_id: `evt-${nextId}`,
    summary: 'summary',
    current_status: 'To Do',
    event_type: 'comment_added',
    occurred_at: '2026-08-27T10:00:00.000Z',
    actor_name: 'Alex',
    payload: {},
    reasons: ['assignee'],
    read_at: null,
    ...over,
  }
}

describe('GDK-1058 groupFeedItems', () => {
  test('adjacent same-issue events of different kinds form one group', () => {
    const items = [
      item({ issue_key: 'STD-1', event_type: 'assigned' }),
      item({ issue_key: 'STD-1', event_type: 'comment_added' }),
      item({ issue_key: 'STD-1', event_type: 'status_changed' }),
      item({ issue_key: 'STD-1', event_type: 'created' }),
    ]
    const groups = groupFeedItems(items)
    expect(groups).toHaveLength(1)
    expect(groups[0].items).toHaveLength(4)
    // Representative is the first item — the {#each} key and expand state.
    expect(groups[0].id).toBe(items[0].id)
  })

  test('a different issue intervening splits the run — adjacency, not regroup', () => {
    const groups = groupFeedItems([
      item({ issue_key: 'STD-1', event_type: 'comment_added' }),
      item({ issue_key: 'STD-2', event_type: 'status_changed' }),
      item({ issue_key: 'STD-1', event_type: 'assigned' }),
    ])
    expect(groups.map((g) => g.items[0].issue_key)).toEqual(['STD-1', 'STD-2', 'STD-1'])
    expect(groups.every((g) => g.items.length === 1)).toBe(true)
  })

  test('the same-kind collapse survives as the single-kind case', () => {
    const groups = groupFeedItems([
      item({ issue_key: 'STD-1', event_type: 'comment_added' }),
      item({ issue_key: 'STD-1', event_type: 'comment_added' }),
      item({ issue_key: 'STD-1', event_type: 'comment_added' }),
    ])
    expect(groups).toHaveLength(1)
    expect(groups[0].items).toHaveLength(3)
  })

  test('a run does not span days — the day boundary survives', () => {
    // The key's day is toDateString(): a *local* calendar day. Noon UTC a
    // day apart is on different local dates in every timezone, so the
    // fixture cannot accidentally share a day with the runner's clock.
    const groups = groupFeedItems([
      item({ issue_key: 'STD-1', occurred_at: '2026-08-27T12:00:00.000Z' }),
      item({ issue_key: 'STD-1', occurred_at: '2026-08-28T12:00:00.000Z' }),
    ])
    expect(groups).toHaveLength(2)
  })

  test('timestamp-less items never merge', () => {
    const groups = groupFeedItems([
      item({ issue_key: 'STD-1', occurred_at: null }),
      item({ issue_key: 'STD-1', occurred_at: null }),
    ])
    expect(groups).toHaveLength(2)
  })
})

describe('GDK-1058 groupKindCounts', () => {
  function kindsOf(types: FeedEventType[]): { type: FeedEventType; count: number }[] {
    return groupKindCounts(types.map((event_type) => item({ issue_key: 'STD-1', event_type })))
  }

  test('single-kind group keeps one entry with the full count', () => {
    expect(kindsOf(['comment_added', 'comment_added'])).toEqual([
      { type: 'comment_added', count: 2 },
    ])
  })

  test('mixed run lists distinct kinds in first-seen order with per-kind counts', () => {
    expect(kindsOf(['assigned', 'comment_added', 'status_changed', 'comment_added'])).toEqual([
      { type: 'assigned', count: 1 },
      { type: 'comment_added', count: 2 },
      { type: 'status_changed', count: 1 },
    ])
  })
})
