/*
 * Feed polling tests. The contracts under pin:
 *
 *  - The promotion table: only the event_type ∩ reasons intersections
 *    banner (assigned∩assignee, comment_added∩mention|assignee, reopened),
 *    only while unread — branching keys on ids/categories, never display
 *    names (current_status is copy material, not logic).
 *  - The client-side watermark: the server has no cursor and recomputes the
 *    whole window per request, so "new" is computed from a persisted
 *    occurred_at watermark plus a seen-id set; a re-received event_id
 *    promotes nothing, and the first poll after pairing is a silent
 *    baseline (no 30-day banner storm).
 *  - The poll engine: 15s floor (the server recomputes per request — never
 *    faster), visibilitychange return polls once immediately, hidden stops
 *    the timer, and banners only go out while the document is hidden.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { isPermissionGranted, onAction, requestPermission, sendNotification } = vi.hoisted(() => ({
  isPermissionGranted: vi.fn(),
  onAction: vi.fn(),
  requestPermission: vi.fn(),
  sendNotification: vi.fn(),
}))
vi.mock('@tauri-apps/plugin-notification', () => ({
  isPermissionGranted,
  onAction,
  requestPermission,
  sendNotification,
}))

import type { ApiContext, FeedItem } from './api'
import {
  bannerFor,
  commitFeedState,
  computeNewItems,
  MIN_POLL_MS,
  promoteItem,
  resolvePollIntervalMs,
  startFeedPolling,
} from './feed'

const CTX: ApiContext = { endpoint: 'https://home.example.ts.net', token: 'tok-1' }

/** Minimal localStorage for the node test environment (settings.test.ts pattern). */
class MemStorage {
  private m = new Map<string, string>()
  getItem(k: string): string | null {
    return this.m.has(k) ? (this.m.get(k) as string) : null
  }
  setItem(k: string, v: string): void {
    this.m.set(k, v)
  }
  removeItem(k: string): void {
    this.m.delete(k)
  }
}

/** Minimal document for the node test environment. */
class FakeDoc {
  visibilityState: 'visible' | 'hidden' = 'visible'
  private listeners = new Set<() => void>()
  addEventListener(_type: string, cb: () => void): void {
    this.listeners.add(cb)
  }
  removeEventListener(_type: string, cb: () => void): void {
    this.listeners.delete(cb)
  }
  dispatch(): void {
    for (const cb of [...this.listeners]) cb()
  }
  hide(): void {
    this.visibilityState = 'hidden'
    this.dispatch()
  }
  show(): void {
    this.visibilityState = 'visible'
    this.dispatch()
  }
}

let storage: MemStorage
let doc: FakeDoc
let windowFetch: ReturnType<typeof vi.fn>

const UNREAD = { all: 0, assignee: 0, reporter: 0, mention: 0 }

const feedResponse = (items: FeedItem[]): Response =>
  new Response(JSON.stringify({ items, unread_counts: UNREAD }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

const item = (over: Partial<FeedItem> = {}): FeedItem => ({
  event_id: 'cm:1',
  issue_key: 'GDK-1',
  summary: 'Fix the thing',
  current_status: 'In Progress',
  event_type: 'comment_added',
  occurred_at: '2026-08-25T09:00:00Z',
  actor_name: 'Robin',
  payload: {},
  reasons: ['assignee'],
  read_at: null,
  ...over,
})

beforeEach(() => {
  vi.useFakeTimers()
  storage = new MemStorage()
  doc = new FakeDoc()
  windowFetch = vi.fn()
  vi.stubGlobal('localStorage', storage)
  vi.stubGlobal('document', doc)
  vi.stubGlobal('fetch', windowFetch)
  sendNotification.mockReset()
  onAction.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('promotion table (golden)', () => {
  type Row = [string, FeedItem['event_type'], string[], string | null, string | null]
  const READ = '2026-08-24T00:00:00Z'
  // [label, event_type, reasons, read_at, expected kind]
  const rows: Row[] = [
    ['created × assignee', 'created', ['assignee'], null, null],
    ['created × watched', 'created', ['watched'], null, null],
    ['status_changed × assignee', 'status_changed', ['assignee'], null, null],
    ['status_changed × mention', 'status_changed', ['mention'], null, null],
    ['fields_changed × assignee', 'fields_changed', ['assignee'], null, null],
    ['fields_changed × watched', 'fields_changed', ['watched'], null, null],
    ['attachment_added × assignee', 'attachment_added', ['assignee'], null, null],
    ['reopened × assignee', 'reopened', ['assignee'], null, 'reopened'],
    ['reopened × watched', 'reopened', ['watched'], null, 'reopened'],
    ['reopened × no reasons', 'reopened', [], null, 'reopened'],
    ['assigned × assignee', 'assigned', ['assignee'], null, 'assigned'],
    ['assigned × assignee+watched', 'assigned', ['assignee', 'watched'], null, 'assigned'],
    ['assigned × watched', 'assigned', ['watched'], null, null],
    ['assigned × reporter', 'assigned', ['reporter'], null, null],
    ['comment × mention', 'comment_added', ['mention'], null, 'mention'],
    ['comment × mention+assignee', 'comment_added', ['mention', 'assignee'], null, 'mention'],
    ['comment × assignee', 'comment_added', ['assignee'], null, 'comment'],
    ['comment × assignee+watched', 'comment_added', ['assignee', 'watched'], null, 'comment'],
    ['comment × watched', 'comment_added', ['watched'], null, null],
    ['comment × reporter', 'comment_added', ['reporter'], null, null],
    ['assigned × assignee but read', 'assigned', ['assignee'], READ, null],
    ['comment × mention but read', 'comment_added', ['mention'], READ, null],
    ['reopened × assignee but read', 'reopened', ['assignee'], READ, null],
  ]
  it('keys only on event_type ∩ reasons and skips already-read events', () => {
    for (const [label, event_type, reasons, read_at, want] of rows) {
      const got = promoteItem(item({ event_type, reasons, read_at }))
      expect(got === null ? null : got.kind, label).toBe(want)
    }
  })
})

describe('banner copy', () => {
  it('mentions carry the comment excerpt as the body', () => {
    const b = bannerFor({
      item: item({ payload: { excerpt: 'take a look at line 40' } }),
      kind: 'mention',
    })
    expect(b.title).toBe('Robin mentioned you on GDK-1')
    expect(b.body).toBe('take a look at line 40')
    expect(b.issueKey).toBe('GDK-1')
  })

  it('falls back to the summary when a comment has no excerpt', () => {
    const b = bannerFor({ item: item({ payload: {} }), kind: 'comment' })
    expect(b.title).toBe('Robin commented on GDK-1')
    expect(b.body).toBe('Fix the thing')
  })

  it('assignee and reopened banners carry the summary', () => {
    expect(bannerFor({ item: item({ event_type: 'assigned' }), kind: 'assigned' }).title).toBe(
      'Robin assigned GDK-1 to you',
    )
    expect(bannerFor({ item: item({ event_type: 'reopened' }), kind: 'reopened' }).title).toBe(
      'Robin reopened GDK-1',
    )
  })
})

describe('watermark and duplicate suppression', () => {
  const a = item({ event_id: 'cm:1', occurred_at: '2026-08-25T09:00:00Z' })
  const b = item({ event_id: 'cm:2', occurred_at: '2026-08-25T09:00:10Z' })

  it('the first poll is a silent baseline (no stored state → nothing is new)', () => {
    expect(computeNewItems(null, [a, b])).toEqual([])
  })

  it('commits the baseline: every id seen, watermark at the newest occurred_at', () => {
    const s = commitFeedState(null, CTX.endpoint, [a, b])
    expect(s.endpoint).toBe(CTX.endpoint)
    expect(s.watermark).toBe('2026-08-25T09:00:10Z')
    expect(s.seen).toEqual(['cm:1', 'cm:2'])
  })

  it('an unseen newer event_id is new; a re-received one is not', () => {
    const s = commitFeedState(null, CTX.endpoint, [a, b])
    const c = item({ event_id: 'cm:3', occurred_at: '2026-08-25T09:00:20Z' })
    expect(computeNewItems(s, [a, b, c])).toEqual([c])
    expect(computeNewItems(s, [a, b, c])).toEqual([c]) // state not committed yet → still new
    const s2 = commitFeedState(s, CTX.endpoint, [a, b, c])
    expect(computeNewItems(s2, [a, b, c])).toEqual([]) // duplicate suppression
  })

  it('a same-second unseen event is new (seen-set, not just the watermark)', () => {
    const s = commitFeedState(null, CTX.endpoint, [a, b])
    const sameSecond = item({ event_id: 'cl:9', occurred_at: '2026-08-25T09:00:10Z' })
    expect(computeNewItems(s, [b, sameSecond])).toEqual([sameSecond])
  })

  it('an old unseen event (below the watermark) is not new — evictions cannot re-promote', () => {
    const s = commitFeedState(null, CTX.endpoint, [a, b])
    const old = item({ event_id: 'cm:0', occurred_at: '2026-08-24T08:00:00Z' })
    expect(computeNewItems(s, [old, b])).toEqual([])
  })

  it('an undated event is never promoted (conservative — no banner storms)', () => {
    const s = commitFeedState(null, CTX.endpoint, [a, b])
    const undated = item({ event_id: 'cm:4', occurred_at: null })
    expect(computeNewItems(s, [b, undated])).toEqual([])
  })

  it('a different endpoint is a different event space — the baseline restarts', () => {
    const s = commitFeedState(null, 'https://old.example.ts.net', [a, b])
    expect(computeNewItems(s, [a, b, item({ event_id: 'cm:3' })])).toEqual([])
    const s2 = commitFeedState(s, 'https://new.example.ts.net', [a])
    expect(s2.endpoint).toBe('https://new.example.ts.net')
    expect(s2.seen).toEqual(['cm:1'])
  })
})

describe('poll interval floor', () => {
  it('clamps everything below 15s up to the floor', () => {
    expect(MIN_POLL_MS).toBe(15_000)
    expect(resolvePollIntervalMs()).toBe(15_000)
    expect(resolvePollIntervalMs(1_000)).toBe(15_000)
    expect(resolvePollIntervalMs(14_999)).toBe(15_000)
    expect(resolvePollIntervalMs(15_000)).toBe(15_000)
    expect(resolvePollIntervalMs(60_000)).toBe(60_000)
  })
})

describe('poll engine', () => {
  it('polls once on start, then every interval; the floor holds even when a faster one is requested', async () => {
    windowFetch.mockResolvedValue(feedResponse([]))
    const h = startFeedPolling(CTX, { intervalMs: 1_000 })
    expect(h.intervalMs).toBe(15_000)
    await vi.advanceTimersByTimeAsync(0)
    expect(windowFetch).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(14_000)
    expect(windowFetch).toHaveBeenCalledTimes(1) // 15s not elapsed on the clamped floor
    await vi.advanceTimersByTimeAsync(1_000)
    expect(windowFetch).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(30_000)
    expect(windowFetch).toHaveBeenCalledTimes(4)
    h.stop()
  })

  it('hides stop the timer; returning to visible polls once immediately', async () => {
    windowFetch.mockResolvedValue(feedResponse([]))
    const h = startFeedPolling(CTX)
    await vi.advanceTimersByTimeAsync(0)
    expect(windowFetch).toHaveBeenCalledTimes(1)
    doc.hide()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(windowFetch).toHaveBeenCalledTimes(1) // hidden → timer stopped
    doc.show()
    await vi.advanceTimersByTimeAsync(0)
    expect(windowFetch).toHaveBeenCalledTimes(2) // return → immediate poll
    await vi.advanceTimersByTimeAsync(15_000)
    expect(windowFetch).toHaveBeenCalledTimes(3) // and the timer resumed
    h.stop()
  })

  it('stop() ends polling and detaches the visibility listener', async () => {
    windowFetch.mockResolvedValue(feedResponse([]))
    const h = startFeedPolling(CTX)
    await vi.advanceTimersByTimeAsync(0)
    h.stop()
    await vi.advanceTimersByTimeAsync(60_000)
    doc.show()
    await vi.advanceTimersByTimeAsync(0)
    expect(windowFetch).toHaveBeenCalledTimes(1)
  })

  it('a slow in-flight poll is not stacked by the next tick', async () => {
    let release: ((r: Response) => void) | undefined
    windowFetch.mockImplementation(
      () => new Promise<Response>((res) => (release = res)),
    )
    const h = startFeedPolling(CTX)
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(windowFetch).toHaveBeenCalledTimes(1) // still in flight → tick skipped
    release?.(feedResponse([]))
    await vi.advanceTimersByTimeAsync(0)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(windowFetch).toHaveBeenCalledTimes(2)
    h.stop()
  })

  it('transport errors surface on onerror and polling continues', async () => {
    windowFetch.mockRejectedValue(new TypeError('tailnet down'))
    const onerror = vi.fn()
    const h = startFeedPolling(CTX, { events: { onerror } })
    await vi.advanceTimersByTimeAsync(0)
    expect(onerror).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(15_000)
    expect(onerror).toHaveBeenCalledTimes(2) // the timer survived the failure
    h.stop()
  })

  it('persistence across restarts: the same items re-received promote nothing', async () => {
    const a = item({ event_id: 'cm:1' })
    windowFetch.mockResolvedValue(feedResponse([a]))
    const first = startFeedPolling(CTX)
    await vi.advanceTimersByTimeAsync(0)
    first.stop()
    const onpromoted = vi.fn()
    const second = startFeedPolling(CTX, { events: { onpromoted } })
    await vi.advanceTimersByTimeAsync(0)
    expect(onpromoted).not.toHaveBeenCalled()
    second.stop()
  })
})

describe('promotion end-to-end through the engine', () => {
  it('the first poll baselines silently; new unread events promote and banner only while hidden', async () => {
    const eligible = (n: string): FeedItem =>
      item({ event_id: `cm:${n}`, occurred_at: `2026-08-25T09:00:0${n}Z` })
    let current = [eligible('1'), eligible('2')]
    windowFetch.mockImplementation(async () => feedResponse(current))
    const onpromoted = vi.fn()
    const h = startFeedPolling(CTX, { events: { onpromoted } })
    await vi.advanceTimersByTimeAsync(0)
    expect(onpromoted).not.toHaveBeenCalled() // baseline: 30-day backlog stays quiet
    expect(sendNotification).not.toHaveBeenCalled()

    // New event while visible: the queue refreshes in place, no banner.
    current = [...current, eligible('3')]
    await vi.advanceTimersByTimeAsync(15_000)
    expect(onpromoted).toHaveBeenCalledOnce()
    expect(onpromoted.mock.calls[0][0]).toHaveLength(1)
    expect(onpromoted.mock.calls[0][0][0].item.event_id).toBe('cm:3')
    expect(sendNotification).not.toHaveBeenCalled()

    // New event while hidden: a poll completing off-screen (here pollNow —
    // the timer itself is stopped while hidden) banners.
    doc.hide()
    current = [...current, eligible('4')]
    await h.pollNow()
    expect(onpromoted).toHaveBeenCalledTimes(2)
    expect(sendNotification).toHaveBeenCalledOnce()
    expect(sendNotification).toHaveBeenCalledWith({
      title: 'Robin commented on GDK-1',
      body: 'Fix the thing',
      group: 'GDK-1',
      extra: { issue_key: 'GDK-1' },
    })
    h.stop()
  })

  it('new but already-read events (read on another surface) stay quiet', async () => {
    const a = item({ event_id: 'cm:1' })
    windowFetch.mockResolvedValue(feedResponse([a]))
    const h = startFeedPolling(CTX)
    await vi.advanceTimersByTimeAsync(0)
    doc.hide()
    const read = item({ event_id: 'cm:2', occurred_at: '2026-08-25T09:00:05Z', read_at: '2026-08-25T09:00:06Z' })
    windowFetch.mockResolvedValue(feedResponse([read, a]))
    await h.pollNow()
    expect(sendNotification).not.toHaveBeenCalled()
    h.stop()
  })
})
