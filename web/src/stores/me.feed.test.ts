/*
 * GDK-1066: a failed feed request is a failure signal, not "no new
 * activity". loadFeed's catch used to swallow the error (console.warn
 * only), so an answered 503 rendered as the feed.empty copy. The contract
 * here mirrors pages/history (GDK-1054): the flag records the failure,
 * stale rows survive a failed reload, and the next success clears it.
 *
 * This file joins the pages-store project (vitest.config.ts): me.svelte.ts
 * uses runes and pulls the store graph, which the no-svelte-plugin unit
 * project cannot compile — the same reason pages.test.ts lives there.
 *
 * State is set through the modules' own public surfaces: the feature flag
 * via loadConfig() with fetch stubbed (config.ts deliberately has no
 * setter — the seam config.test.ts drives), identity via the public
 * `email` state, and the feed API through the api mock, getFeed being the
 * only call loadFeed makes.
 */
import { afterEach, beforeAll, describe, expect, test, vi } from 'vitest'
import type { FeedItem, FeedUnreadCounts } from '../lib/types'

const api = vi.hoisted(() => ({
  getFeed: vi.fn(),
  // reachability.svelte.ts registers its down-handler at module load.
  setNetworkDownHandler: vi.fn(),
}))

vi.mock('../lib/api', () => api)

import { loadConfig } from '../lib/config'
import { me } from './me.svelte'

const COUNTS: FeedUnreadCounts = { all: 0, assignee: 0, reporter: 0, mention: 0 }

function feedItem(id: number): FeedItem {
  return {
    id,
    event_id: `evt-${id}`,
    issue_key: 'STD-1',
    summary: `item ${id}`,
    current_status: 'To Do',
    event_type: 'status_changed',
    occurred_at: '2026-08-01T00:00:00.000Z',
    actor_name: 'Alex',
    payload: {},
    reasons: ['assignee'],
    read_at: null,
  }
}

beforeAll(async () => {
  // runtimeBase() reads window.location.pathname; this project runs in node.
  vi.stubGlobal('window', { location: { pathname: '/' } })
  vi.stubGlobal(
    'fetch',
    async () => new Response(JSON.stringify({ features: { feed: true } }), { status: 200 }),
  )
  await loadConfig()
  // loadFeed is a no-op without identity; the public email state is enough.
  me.email = 'dev@example.net'
})

afterEach(() => {
  vi.clearAllMocks()
  // The me store is a singleton; leave it as the next test expects to find it.
  me.email = 'dev@example.net'
  me.feedItems = []
  me.feedUnread = { ...COUNTS }
  me.feedLoaded = false
  me.feedLoading = false
  me.feedLoadFailed = false
})

describe('GDK-1066 feed load failure is recorded, not disguised as empty', () => {
  test('a rejected getFeed marks feedLoadFailed and keeps the stale rows', async () => {
    const stale = [feedItem(1), feedItem(2)]
    me.feedItems = stale
    api.getFeed.mockRejectedValueOnce(new Error('503 service unavailable'))

    await me.loadFeed()

    expect(me.feedLoadFailed).toBe(true)
    // Stale survival is the GDK-1054 contract: a failed reload never wipes
    // what a successful load had put on screen.
    expect(me.feedItems).toBe(stale)
    expect(me.feedLoaded).toBe(false)
    expect(me.feedLoading).toBe(false)
  })

  test('a later success clears the flag and swaps the rows in', async () => {
    me.feedItems = [feedItem(1)]
    api.getFeed.mockRejectedValueOnce(new Error('503 service unavailable'))
    await me.loadFeed()
    expect(me.feedLoadFailed).toBe(true)

    const fresh = [feedItem(9)]
    api.getFeed.mockResolvedValueOnce({ items: fresh, unread_counts: { ...COUNTS } })
    await me.loadFeed()

    expect(me.feedLoadFailed).toBe(false)
    expect(me.feedItems).toEqual(fresh)
    expect(me.feedLoaded).toBe(true)
  })
})
