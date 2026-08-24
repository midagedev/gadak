/*
 * Foreground feed polling + local notification promotion — GDK-802, the
 * A-feed chunk. The screen IS the quiet queue: this module renders
 * nothing, and a feed row is promoted to a local banner only when it is
 * new, unread, and the app is off-screen (notify.ts owns that gate).
 *
 * Server premises (flow-report Q4, measured against a live serve):
 * GET issues/feed/ has NO cursor — every request recomputes the whole
 * 30-day window (all issue metadata + changelog + comments + attachments)
 * and POST feed/read/ only records receipts; the next GET recomputes
 * again. Two consequences:
 *
 *  - "New" is computed client-side. event_ids are not globally ordered
 *    (`cr:<key>`, `cl:<changelog id>`, `fl:<item>:<at>`, `cm:<comment id>`,
 *    `at:<attach id>` — internal/store/feed.go), so a max-event_id
 *    watermark cannot work. The watermark is instead the newest
 *    occurred_at consumed — the server's own sort axis (feed.go sorts
 *    FeedItems by comparing OccurredAt strings) — plus a seen-id set for
 *    same-second and duplicate events: new = unseen ∧ occurred_at ≥
 *    watermark. The first poll after pairing is a silent baseline: the
 *    30-day backlog becomes "seen" and nothing banners.
 *  - The poll period never goes below 15s — the web client's cadence —
 *    because every poll makes the server recompute (534-issue mirror ≈10ms
 *    per request; a faster phone poll is server load for no freshness).
 *
 * Identity gap, by design: a standalone home has no account
 * (me.account_id null), so the server's reasons come out empty and feed
 * items are []. Zero notifications on a standalone home is the expected
 * steady state, not a hidden failure — there is no "you" to match
 * mentions/assignments against. Nothing client-side compensates, because
 * the client must never compute "me" from display names.
 *
 * APNs does not exist here (product premise): polling runs only while
 * the document is visible, the timer stops when hidden, and a
 * visibilitychange return polls once immediately — the gap is covered by
 * the full recompute on that poll. Events that arrived while hidden and
 * are still unread surface through onpromoted for the queue's unread
 * counts; whether they re-banner after the fact is A-nav's call.
 */
import { feed as fetchFeed, type ApiContext, type Feed, type FeedItem } from './api'
import { t, type MessageKey } from './i18n'
import { appIsVisible, showBanner, type Banner } from './notify'

/** The floor for the poll period — never faster (server recomputes per request). */
export const MIN_POLL_MS = 15_000

const FEED_STATE_KEY = 'gadak-mobile.feed'
/** Consumed event_ids kept for duplicate suppression; the oldest drop off. */
const SEEN_CAP = 512

export type PromotionKind = 'assigned' | 'mention' | 'comment' | 'reopened'

export interface PromotedItem {
  item: FeedItem
  kind: PromotionKind
}

/**
 * The promotion rule (ux-report Q5 + flow-report Q4): only the
 * event_type ∩ reasons intersections banner — assigned∩assignee,
 * comment_added∩mention (mention wins when both) or ∩assignee, reopened.
 * created / status_changed / fields_changed / attachment_added never
 * promote (fields_changed is measured noise), and neither does an event
 * already read on another surface (banner the unread increment only).
 * Branching keys on event_type/reasons — current_status is a display
 * name and never enters logic.
 */
export function promoteItem(item: FeedItem): PromotedItem | null {
  if (item.read_at !== null) return null
  switch (item.event_type) {
    case 'assigned':
      return item.reasons.includes('assignee') ? { item, kind: 'assigned' } : null
    case 'comment_added':
      if (item.reasons.includes('mention')) return { item, kind: 'mention' }
      if (item.reasons.includes('assignee')) return { item, kind: 'comment' }
      return null
    case 'reopened':
      return { item, kind: 'reopened' }
    default:
      return null
  }
}

const TITLE_KEY: Record<PromotionKind, MessageKey> = {
  assigned: 'feed.banner.assigned.title',
  comment: 'feed.banner.comment.title',
  mention: 'feed.banner.mention.title',
  reopened: 'feed.banner.reopened.title',
}

/**
 * Banner copy from the promoted item. The body prefers the server's
 * comment excerpt (≤120 runes, already truncated server-side) and falls
 * back to the issue summary; current_status stays out of it — copy may
 * show display names, but none of them adds signal to a one-line banner.
 */
export function bannerFor(p: PromotedItem): Banner {
  let body = p.item.summary
  if (p.kind === 'mention' || p.kind === 'comment') {
    const excerpt = p.item.payload?.excerpt
    if (typeof excerpt === 'string' && excerpt !== '') body = excerpt
  }
  return {
    title: t(TITLE_KEY[p.kind], { actor: p.item.actor_name, key: p.item.issue_key }),
    body,
    issueKey: p.item.issue_key,
  }
}

/**
 * The persisted consumption state. `endpoint` scopes it: the workspace is
 * bound to one origin, so pairing a different home is a different event
 * space and restarts the baseline instead of suppressing everything.
 */
export interface FeedState {
  endpoint: string
  /** Newest occurred_at consumed so far (RFC3339 string, server's sort axis). */
  watermark: string
  /** Consumed event_ids, oldest-consumed first, capped at SEEN_CAP. */
  seen: string[]
}

/**
 * Which of `items` are new given the consumed state. No state (first
 * poll / new endpoint / unreadable storage) → nothing: the caller
 * baselines. An item is new when it is unseen, dated, and at or past the
 * watermark — "at" matters for same-second arrivals (the seen-set, not
 * the watermark, decides those), and "past" is what makes seen-cap
 * evictions unable to re-promote old events.
 */
export function computeNewItems(state: FeedState | null, items: FeedItem[]): FeedItem[] {
  if (state === null) return []
  return items.filter(
    (it) =>
      it.occurred_at !== null &&
      it.occurred_at >= state.watermark &&
      !state.seen.includes(it.event_id),
  )
}

/** Consumes `items`: every id becomes seen, the watermark only moves forward. */
export function commitFeedState(
  state: FeedState | null,
  endpoint: string,
  items: FeedItem[],
): FeedState {
  const base = state !== null && state.endpoint === endpoint ? state : null
  const has = new Set<string>(base?.seen ?? [])
  const seen: string[] = []
  for (const id of [...(base?.seen ?? []), ...items.map((i) => i.event_id)]) {
    if (has.has(id)) continue
    has.add(id)
    seen.push(id)
  }
  if (seen.length > SEEN_CAP) seen.splice(0, seen.length - SEEN_CAP)
  let watermark = base?.watermark ?? ''
  for (const it of items) {
    if (it.occurred_at !== null && it.occurred_at > watermark) watermark = it.occurred_at
  }
  return { endpoint, watermark, seen }
}

// localStorage accessors in the settings.ts idiom (guarded, never throw).
// settings.ts owns its own private helpers and this file may not edit it,
// so the guard is local — same shape, same quota/private-mode degradation:
// with no storage every poll re-baselines and nothing banners, which is
// quiet, never a crash.
function readFeedState(endpoint: string): FeedState | null {
  let v: unknown
  try {
    const raw = localStorage.getItem(FEED_STATE_KEY)
    if (raw === null) return null
    v = JSON.parse(raw)
  } catch {
    return null
  }
  if (typeof v !== 'object' || v === null) return null
  const o = v as Record<string, unknown>
  if (o.endpoint !== endpoint || typeof o.watermark !== 'string' || !Array.isArray(o.seen)) {
    return null
  }
  if (!o.seen.every((s) => typeof s === 'string')) return null
  return { endpoint, watermark: o.watermark, seen: o.seen as string[] }
}

function writeFeedState(state: FeedState): void {
  try {
    localStorage.setItem(FEED_STATE_KEY, JSON.stringify(state))
  } catch {
    /* quota / private mode — polls continue uncached (re-baseline each time) */
  }
}

/** Clamps a requested poll period up to the 15s floor. */
export function resolvePollIntervalMs(requested?: number): number {
  if (requested === undefined || !Number.isFinite(requested)) return MIN_POLL_MS
  return Math.max(MIN_POLL_MS, Math.floor(requested))
}

export interface FeedPollingEvents {
  /** Every successful poll — the queue refreshes its unread counts in place. */
  onfeed?: (f: Feed) => void
  /** New, unread, promotion-eligible items (feed order, newest first). */
  onpromoted?: (items: PromotedItem[]) => void
  /** Transport/server failure — screens keep their offline-banner logic. */
  onerror?: (err: unknown) => void
}

export interface FeedPollingOptions {
  intervalMs?: number
  events?: FeedPollingEvents
}

export interface FeedPollingHandle {
  stop(): void
  /** One poll now (pull-to-refresh, or an off-screen completion). */
  pollNow(): Promise<void>
  readonly intervalMs: number
}

/**
 * The polling owner. One handle per app: start when paired and on screen,
 * stop on unpair. focus stays 'all' — the reasons intersection is this
 * client's policy, not the server query's.
 */
export function startFeedPolling(ctx: ApiContext, opts: FeedPollingOptions = {}): FeedPollingHandle {
  const intervalMs = resolvePollIntervalMs(opts.intervalMs)
  const events = opts.events ?? {}
  let timer: ReturnType<typeof setInterval> | null = null
  let stopped = false
  let inFlight: Promise<void> | null = null
  let controller: AbortController | null = null

  const startTimer = (): void => {
    if (timer === null) timer = setInterval(() => void pollOnce(), intervalMs)
  }
  const stopTimer = (): void => {
    if (timer !== null) {
      clearInterval(timer)
      timer = null
    }
  }

  const pollOnce = async (): Promise<void> => {
    if (inFlight !== null) return inFlight // a slow tailnet must not stack requests
    const ctl = new AbortController()
    controller = ctl
    const run = (async () => {
      try {
        const f = await fetchFeed(ctx, 'all', undefined, ctl.signal)
        const state = readFeedState(ctx.endpoint)
        const fresh = computeNewItems(state, f.items)
        writeFeedState(commitFeedState(state, ctx.endpoint, f.items))
        events.onfeed?.(f)
        if (fresh.length > 0) {
          const promoted = fresh
            .map(promoteItem)
            .filter((p): p is PromotedItem => p !== null)
          for (const p of promoted) showBanner(bannerFor(p)) // no-op while visible
          if (promoted.length > 0) events.onpromoted?.(promoted)
        }
      } catch (err) {
        if (ctl.signal.aborted) return // stop() is intentional, not an error
        events.onerror?.(err)
      } finally {
        inFlight = null
        controller = null
      }
    })()
    inFlight = run
    return run
  }

  const onVisibility = (): void => {
    if (stopped) return
    if (appIsVisible()) {
      startTimer()
      void pollOnce()
    } else {
      stopTimer()
    }
  }

  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibility)
  }

  if (appIsVisible()) {
    void pollOnce()
    startTimer()
  }

  return {
    stop(): void {
      stopped = true
      stopTimer()
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', onVisibility)
      }
      controller?.abort()
    },
    pollNow(): Promise<void> {
      return pollOnce()
    },
    get intervalMs(): number {
      return intervalMs
    },
  }
}
