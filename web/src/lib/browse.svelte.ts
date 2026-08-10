/*
 * Desktop in-app browse sessions.
 *
 * When a Jira/Confluence URL opens in a native in-app tab (POST /desktop/browse),
 * we remember the tab id → {kind, key}. Closing the tab (or regaining main-window
 * focus) triggers a targeted resync so the open detail panel refreshes without a
 * full mirror pull.
 *
 * Installed only in desktop mode; browser `scry serve` never wires this up.
 */

import * as api from './api'
import * as db from './db'
import { isDesktop } from './config'
import { invalidate } from './detail-cache.svelte'
import { issues } from '../stores/issues.svelte'
import { pages } from '../stores/pages.svelte'
import { write } from '../stores/write.svelte'
import type { IssueLite } from './types'

/** Matches classifyAtlassianLink kind — kept local to avoid a cycle with desktop-links. */
export type BrowseKind = 'issue' | 'page' | 'other'

interface BrowseSession {
  kind: BrowseKind
  key: string | null
}

const POLL_MS = 2000
const FOCUS_THROTTLE_MS = 15_000

/** id → session metadata. Non-reactive: no UI binds this map. */
const sessions = new Map<string, BrowseSession>()

/** item throttle key (`issue:NMB-1` / `page:123`) → last successful resync attempt ms. */
const lastResyncAt = new Map<string, number>()

let pollTimer: ReturnType<typeof setInterval> | null = null

function throttleKey(sess: BrowseSession): string | null {
  if (!sess.key || sess.kind === 'other') return null
  return `${sess.kind}:${sess.key}`
}

function startPoll(): void {
  if (pollTimer !== null) return
  pollTimer = setInterval(() => {
    void pollOpenTabs()
  }, POLL_MS)
}

function stopPoll(): void {
  if (pollTimer === null) return
  clearInterval(pollTimer)
  pollTimer = null
}

/**
 * Remember a tab opened via POST /desktop/browse. Starts state polling while
 * at least one session is open.
 */
export function trackBrowseSession(
  id: string,
  kind: BrowseKind,
  key: string | null,
): void {
  if (!isDesktop()) return
  sessions.set(id, { kind, key })
  startPoll()
}

/**
 * Apply a write-shaped issue resync the same way write.svelte does after a
 * successful comment/transition: pool + IndexedDB, then detail cache miss +
 * detailNonce so an open DetailPanel reloads.
 */
function applyIssueWriteResponse(issue: IssueLite | null | undefined): void {
  if (!issue || !issue.issue_key) return
  issues.pool.set(issue.issue_key, issue)
  void db.putIssues([issue]).catch(() => {})
  invalidate(issue.issue_key)
  write.bumpDetail()
}

async function resyncSession(sess: BrowseSession, opts?: { throttle?: boolean }): Promise<void> {
  if (sess.kind === 'other' || !sess.key) return

  const tKey = throttleKey(sess)
  if (opts?.throttle && tKey) {
    const last = lastResyncAt.get(tKey) ?? 0
    if (Date.now() - last < FOCUS_THROTTLE_MS) return
  }

  try {
    if (sess.kind === 'issue') {
      const res = await api.resyncIssue(sess.key)
      applyIssueWriteResponse(res.issue)
    } else if (sess.kind === 'page') {
      await api.resyncPage(sess.key)
      pages.invalidateDetail(sess.key)
    }
    if (tKey) lastResyncAt.set(tKey, Date.now())
  } catch (e) {
    console.warn('[browse] resync failed', sess.kind, sess.key, e)
  }
}

async function pollOpenTabs(): Promise<void> {
  if (sessions.size === 0) {
    stopPoll()
    return
  }
  try {
    const res = await fetch('/desktop/browse/state')
    if (!res.ok) {
      console.warn('[browse] state poll →', res.status)
      return
    }
    const body = (await res.json()) as { open?: string[] }
    const open = new Set(body.open ?? [])
    const closed: BrowseSession[] = []
    for (const [id, sess] of sessions) {
      if (!open.has(id)) {
        sessions.delete(id)
        closed.push(sess)
      }
    }
    for (const sess of closed) {
      void resyncSession(sess)
    }
    if (sessions.size === 0) stopPoll()
  } catch (e) {
    console.warn('[browse] state poll failed', e)
  }
}

function onWindowFocus(): void {
  if (sessions.size === 0) return
  // One resync per unique item (multiple tabs on the same issue share throttle).
  const seen = new Set<string>()
  for (const sess of sessions.values()) {
    const tKey = throttleKey(sess)
    if (tKey) {
      if (seen.has(tKey)) continue
      seen.add(tKey)
    }
    void resyncSession(sess, { throttle: true })
  }
}

/** Install focus listener + (when sessions exist) poller. Noop off desktop. */
export function installBrowseSessions(): () => void {
  if (!isDesktop()) return () => {}
  window.addEventListener('focus', onWindowFocus)
  // Polling starts lazily on first trackBrowseSession.
  return () => {
    window.removeEventListener('focus', onWindowFocus)
    stopPoll()
    sessions.clear()
    lastResyncAt.clear()
  }
}
