/*
 * Issue Navigator — identity / personalization store ([personal], contract §2)
 *
 * Role:
 *  - Identity (email/name/department) from GET auth/me/, which reads the stored
 *    Jira credential. There is no gadak account and no session token.
 *  - Personal feed / read state, feed toggle, recent issues (localStorage
 *    `gadak:recent`, max 30).
 *  - Derived group (part) for smart default views.
 *
 * Watches, web push, and favorites live in their own stores
 * (`watches.svelte`, `push.svelte`, `favorites.svelte`).
 *
 * Read-only features work with no credential. Personalization and writes need
 * a configured credential (`identified` === email !== null).
 *
 * Reactivity: use svelte/reactivity where needed for collection updates.
 */

import { t, type MessageKey } from '../lib/i18n'
import * as api from '../lib/api'
import { config, feature } from '../lib/config'
import { STORAGE_KEYS } from '../lib/storage'
import { issues } from './issues.svelte'
import { watches } from './watches.svelte'
import { push } from './push.svelte'
import { favorites } from './favorites.svelte'
import type {
  FeedFocus,
  FeedItem,
  FeedUnreadCounts,
} from '../lib/types'

export type { FeedFocus } from '../lib/types'

/* Auth API lives outside the issues API base. Read at call time so a
 * loadConfig() override (hosted demo api/auth base under /gadak/) is honoured —
 * a module-level capture would freeze DEFAULTS before config.json loads. */

const RECENT_KEY = STORAGE_KEYS.recent
const RECENT_MAX = 30

/** What a recent entry points at. Absent in anything stored before documents
 *  joined the list, so a missing kind reads as 'issue'. */
export type RecentKind = 'issue' | 'doc'

export interface RecentVisit {
  key: string
  viewed_at: string | null
  kind: RecentKind
}

/** Issue keys and page keys come from different namespaces, so identity here is
 *  the pair, not the key alone. */
function visitId(kind: RecentKind, key: string): string {
  return `${kind}:${key}`
}

function loadRecent(): RecentVisit[] {
  try {
    const raw = localStorage.getItem(RECENT_KEY)
    if (!raw) return []
    const values = JSON.parse(raw) as unknown
    if (!Array.isArray(values)) return []

    const seen = new Set<string>()
    const visits: RecentVisit[] = []
    for (const value of values) {
      // Legacy string[] keeps order; fill exact timestamps on next view.
      const key = typeof value === 'string' ? value : isRecentVisit(value) ? value.key : ''
      if (!key) continue
      const kind: RecentKind =
        typeof value !== 'string' && (value as Record<string, unknown>).kind === 'doc'
          ? 'doc'
          : 'issue'
      const id = visitId(kind, key)
      if (seen.has(id)) continue
      seen.add(id)
      visits.push({
        key,
        viewed_at: typeof value === 'string' ? null : value.viewed_at,
        kind,
      })
      if (visits.length === RECENT_MAX) break
    }
    return visits
  } catch {
    return []
  }
}

function isRecentVisit(value: unknown): value is RecentVisit {
  if (!value || typeof value !== 'object') return false
  const visit = value as Record<string, unknown>
  return (
    typeof visit.key === 'string' &&
    (visit.viewed_at === null || typeof visit.viewed_at === 'string')
  )
}

function saveRecent(visits: RecentVisit[]): void {
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(visits))
  } catch (e) {
    console.warn(`[me] ${RECENT_KEY} 저장 실패`, e)
  }
}

const EMPTY_UNREAD: FeedUnreadCounts = { all: 0, assignee: 0, reporter: 0, mention: 0 }

const EVENT_NOTIFY_KIND: Record<FeedItem['event_type'], MessageKey> = {
  created: 'feed.notifyCreated',
  status_changed: 'feed.notifyStatus',
  reopened: 'feed.notifyReopened',
  assigned: 'feed.notifyAssigned',
  comment_added: 'feed.notifyComment',
  attachment_added: 'feed.notifyAttachment',
  fields_changed: 'feed.notifyFields',
}

class MeStore {
  /* ── Identity (from stored credential via auth/me/) ── */
  email = $state<string | null>(null)
  accountId = $state<string | null>(null)
  name = $state<string | null>(null)
  department = $state<string | null>(null)
  /** init() finished — personalization UI can branch without a flash. */
  authChecked = $state(false)

  /* ── Server personal feed / read state ── */
  feedItems = $state<FeedItem[]>([])
  feedUnread = $state<FeedUnreadCounts>({ ...EMPTY_UNREAD })
  feedLoaded = $state(false)
  feedLoading = $state(false)

  /* ── Local personalization (works without credential) ── */
  /** Recently opened issues *and* documents, newest first. */
  recent = $state<RecentVisit[]>([])
  /** The issue slice — for anything that resolves a key against the issue pool. */
  recentIssues = $derived(this.recent.filter((visit) => visit.kind === 'issue'))

  /** Personal feed main-area toggle. */
  feedOpen = $state(false)
  /** Feed focus tab (all / assignee / reporter / mention). */
  feedFocus = $state<FeedFocus>('all')
  /** Browser Notification.permission snapshot for the settings toggle. */
  browserNotifyPermission = $state<NotificationPermission | 'unsupported'>(
    typeof Notification === 'undefined' ? 'unsupported' : Notification.permission,
  )

  /**
   * True when GET auth/me/ returned an email — synonymous with
   * "a Jira credential is configured on the server".
   */
  get identified(): boolean {
    return this.email !== null
  }

  /** My group (member directory email match). Used for smart defaults. */
  get group(): string | null {
    if (!this.email) return null
    return issues.members.get(this.email)?.group ?? null
  }

  #initialized = false
  #feedPollTimer: ReturnType<typeof setInterval> | null = null
  /** Last seen feedUnread.all — used to fire at most one browser Notification when it grows. */
  #prevUnreadAll = 0
  /** Whether the first successful feed load has set #prevUnreadAll (avoid notify on boot). */
  #feedBaselineReady = false

  /**
   * Boot:
   *  - Load favorites from the mirror (or localStorage when the server is
   *    absent — hosted demo); restore recent from localStorage.
   *  - Always probe GET auth/me/ — configured credential → email (identity);
   *    otherwise 200 {email:null} and stay anonymous (render-before-auth).
   */
  async init(): Promise<void> {
    if (this.#initialized) return
    this.#initialized = true

    this.recent = loadRecent()
    await favorites.load()

    try {
      await this.#fetchIdentity({ loadPersonal: true })
    } catch (e) {
      console.warn('[me] identity probe failed (continuing anonymous)', e)
    } finally {
      this.authChecked = true
    }
  }

  /**
   * Re-read identity after credential save/delete. Loads personal surfaces when
   * identity appears; clears them when it disappears.
   */
  async refreshIdentity(): Promise<void> {
    try {
      await this.#fetchIdentity({ loadPersonal: true })
    } catch (e) {
      console.warn('[me] identity refresh failed', e)
    }
  }

  async #fetchIdentity(opts: { loadPersonal: boolean }): Promise<void> {
    const res = await fetch(`${config().authBase}me/`, { credentials: 'same-origin' })
    if (!res.ok) return
    const data = (await res.json()) as {
      email: string | null
      account_id?: string | null
      name?: string
      department?: string
    }
    if (data.email) {
      const wasIdentified = this.email !== null
      this.#setUser(data.email, data.account_id ?? null, data.name ?? null, data.department ?? null)
      if (opts.loadPersonal && !wasIdentified) {
        await Promise.all([watches.load(), this.loadFeed(), push.load()])
        this.#startFeedPolling()
      }
    } else if (this.email !== null) {
      this.#clearIdentity()
    }
  }

  #setUser(
    email: string,
    accountId: string | null,
    name: string | null,
    department: string | null,
  ): void {
    this.email = email
    this.accountId = accountId
    this.name = name
    this.department = department
  }

  /** Drop identity and personal server state; keep favorites/recent. */
  #clearIdentity(): void {
    this.email = null
    this.accountId = null
    this.name = null
    this.department = null
    watches.clear()
    this.feedItems = []
    this.feedUnread = { ...EMPTY_UNREAD }
    this.feedLoaded = false
    this.#feedBaselineReady = false
    this.#prevUnreadAll = 0
    push.clear()
    this.#syncAppBadge()
  }

  /* ── Personal feed / read ── */

  /** No-op when feed feature is off or no identity. */
  async loadFeed(focus: FeedFocus = this.feedFocus): Promise<void> {
    if (!feature('feed') || !this.identified) return
    this.feedLoading = true
    try {
      const response = await api.getFeed(focus)
      if (focus === this.feedFocus) this.feedItems = response.items
      const prevAll = this.feedUnread.all
      this.feedUnread = response.unread_counts
      this.feedLoaded = true
      this.#syncAppBadge()
      this.#maybeNotifyNewUnread(prevAll, response.items, response.unread_counts.all)
    } catch (e) {
      console.warn('[me] 피드 로드 실패', e)
    } finally {
      this.feedLoading = false
    }
  }

  /**
   * When unread_counts.all grows and the user has granted Notification permission,
   * fire a single in-tab Notification summarizing the newest unread item.
   * No VAPID / service-worker push — that stays behind features.push.
   */
  #maybeNotifyNewUnread(prevAll: number, items: FeedItem[], nextAll: number): void {
    if (!this.#feedBaselineReady) {
      this.#feedBaselineReady = true
      this.#prevUnreadAll = nextAll
      return
    }
    const grew = nextAll > this.#prevUnreadAll || nextAll > prevAll
    this.#prevUnreadAll = nextAll
    if (!grew || nextAll === 0) return
    if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return
    const newest = items.find((item) => !item.read_at) ?? items[0]
    if (!newest) return
    const kindKey = EVENT_NOTIFY_KIND[newest.event_type]
    const kind = kindKey ? t(kindKey) : newest.event_type
    const title = newest.actor_name
      ? t('feed.notifyTitle', { key: newest.issue_key, kind, actor: newest.actor_name })
      : t('feed.notifyTitleNoActor', { key: newest.issue_key, kind })
    try {
      new Notification(title, {
        body: newest.summary || undefined,
        tag: `gadak-feed-${newest.event_id}`,
      })
    } catch {
      /* some browsers throw when the page is not visible / permission races */
    }
  }

  /** Request browser Notification permission (settings toggle). Returns final permission. */
  async requestBrowserNotificationPermission(): Promise<NotificationPermission | 'unsupported'> {
    if (typeof Notification === 'undefined') {
      this.browserNotifyPermission = 'unsupported'
      return 'unsupported'
    }
    try {
      if (Notification.permission === 'default') {
        await Notification.requestPermission()
      }
      this.browserNotifyPermission = Notification.permission
      return this.browserNotifyPermission
    } catch {
      this.browserNotifyPermission = Notification.permission
      return this.browserNotifyPermission
    }
  }

  async markEventRead(eventId: string): Promise<void> {
    if (!this.identified) return
    const unread = this.feedItems.filter((item) => item.event_id === eventId && !item.read_at)
    if (!unread.length) return
    const now = new Date().toISOString()
    this.feedItems = this.feedItems.map((item) =>
      item.event_id === eventId ? { ...item, read_at: item.read_at ?? now } : item,
    )
    try {
      const response = await api.markFeedRead({ event_ids: [eventId] })
      this.feedUnread = response.unread_counts
      this.#syncAppBadge()
    } catch (e) {
      console.warn('[me] 피드 읽음 처리 실패', e)
      await this.loadFeed()
    }
  }

  async markEventsRead(eventIds: string[]): Promise<void> {
    if (!this.identified || !eventIds.length) return
    const ids = new Set(eventIds)
    const unread = this.feedItems.filter((item) => ids.has(item.event_id) && !item.read_at)
    if (!unread.length) return
    const now = new Date().toISOString()
    this.feedItems = this.feedItems.map((item) =>
      ids.has(item.event_id) ? { ...item, read_at: item.read_at ?? now } : item,
    )
    try {
      const response = await api.markFeedRead({ event_ids: [...ids] })
      this.feedUnread = response.unread_counts
      this.#syncAppBadge()
    } catch (e) {
      console.warn('[me] 피드 그룹 읽음 처리 실패', e)
      await this.loadFeed()
    }
  }

  async markIssueRead(issueKey: string): Promise<void> {
    if (!this.identified || !issueKey) return
    if (!this.feedItems.some((item) => item.issue_key === issueKey && !item.read_at)) return
    const now = new Date().toISOString()
    this.feedItems = this.feedItems.map((item) =>
      item.issue_key === issueKey ? { ...item, read_at: item.read_at ?? now } : item,
    )
    try {
      const response = await api.markFeedRead({ issue_keys: [issueKey] })
      this.feedUnread = response.unread_counts
      this.#syncAppBadge()
    } catch (e) {
      console.warn('[me] 이슈 읽음 처리 실패', e)
      await this.loadFeed()
    }
  }

  async markAllFeedRead(): Promise<void> {
    if (!this.identified || this.feedUnread.all === 0) return
    const previous = this.feedItems
    const now = new Date().toISOString()
    this.feedItems = this.feedItems.map((item) => ({ ...item, read_at: item.read_at ?? now }))
    try {
      const response = await api.markFeedRead({ all: true })
      this.feedUnread = response.unread_counts
      this.#syncAppBadge()
    } catch (e) {
      this.feedItems = previous
      console.warn('[me] 피드 전체 읽음 실패', e)
    }
  }

  #startFeedPolling(): void {
    if (!feature('feed')) return
    if (this.#feedPollTimer || typeof window === 'undefined') return
    this.#feedPollTimer = setInterval(() => {
      if (this.identified) void this.loadFeed()
    }, 15_000)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible' && this.identified) void this.loadFeed()
    })
  }

  #syncAppBadge(): void {
    const badge = navigator as Navigator & {
      setAppBadge?: (count?: number) => Promise<void>
      clearAppBadge?: () => Promise<void>
    }
    if (this.feedUnread.all > 0) void badge.setAppBadge?.(this.feedUnread.all)
    else void badge.clearAppBadge?.()
  }

  /* ── Personal feed toggle ── */

  openFeed(focus: FeedFocus = 'all'): void {
    if (!feature('feed')) return
    this.feedFocus = focus
    this.feedOpen = true
    void this.loadFeed(focus)
  }

  closeFeed(): void {
    this.feedOpen = false
  }

  toggleFeed(): void {
    this.feedOpen = !this.feedOpen
  }

  /* ── Recent issues and documents (local) ── */

  recordRecent(key: string, kind: RecentKind = 'issue'): void {
    if (!key) return
    const id = visitId(kind, key)
    const next = [
      { key, viewed_at: new Date().toISOString(), kind },
      ...this.recent.filter((visit) => visitId(visit.kind, visit.key) !== id),
    ].slice(0, RECENT_MAX)
    this.recent = next
    saveRecent(next)
  }
}

/** App-wide singleton. */
export const me = new MeStore()
