/*
 * Detail response cache + prefetch ([detail]).
 *
 * Detail API is on-demand, so every select is a round-trip. Reopening the same
 * issue or prefetching on list hover can hide perceived latency.
 *
 * - Module-level Map cache (max 50 LRU via Map insertion order → drop oldest).
 * - Inflight promises are cached too so concurrent same-key fetches coalesce.
 * - explore/detail call invalidate(key) when issue updated_at changes.
 */

import type { DetailComment, DetailResponse } from './types'
import { getDetail } from './api'

const MAX = 50

/** key(issue_key) → completed response. Map insertion order = LRU order. */
const cache = new Map<string, DetailResponse>()
/** key → in-flight Promise (coalesce duplicate requests). */
const inflight = new Map<string, Promise<DetailResponse>>()

/** LRU: move a recently used key to the end. */
function touch(key: string, value: DetailResponse): void {
  cache.delete(key)
  cache.set(key, value)
  // Over capacity → drop oldest (front of Map)
  while (cache.size > MAX) {
    const oldest = cache.keys().next().value
    if (oldest === undefined) break
    cache.delete(oldest)
  }
}

/**
 * Cache-first detail fetch. Hit → resolved Promise; miss → fetch then cache.
 * Join an existing inflight Promise for the same key when present.
 */
export function getDetailCached(key: string): Promise<DetailResponse> {
  const hit = cache.get(key)
  if (hit) {
    touch(key, hit) // LRU refresh
    return Promise.resolve(hit)
  }
  const pending = inflight.get(key)
  if (pending) return pending

  const p = getDetail(key)
    .then((data) => {
      touch(key, data)
      return data
    })
    .finally(() => {
      inflight.delete(key)
    })
  inflight.set(key, p)
  return p
}

/**
 * Fire-and-forget prefetch — warm cache, don't await.
 * (Called on hover etc.; failures are silent.)
 */
export function prefetchDetail(key: string): void {
  if (cache.has(key) || inflight.has(key)) return
  void getDetailCached(key).catch(() => {})
}

/** Invalidate one key (issue updated_at changed). */
export function invalidate(key: string): void {
  cache.delete(key)
  inflight.delete(key)
}

/** Invalidate all (tests / re-login etc.). */
export function invalidateAll(): void {
  cache.clear()
  inflight.clear()
}

/**
 * Optimistically append a comment to a cached detail (after comment success).
 * Noop if not loaded — next load will include the server comment.
 */
export function appendComment(key: string, comment: DetailComment): void {
  const hit = cache.get(key)
  if (!hit) return
  cache.set(key, { ...hit, comments: [...hit.comments, comment] })
}
