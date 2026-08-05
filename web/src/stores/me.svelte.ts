/*
 * Issue Navigator — identity / personalization store ([personal], contract §2)
 *
 * Role:
 *  - Identity (email/name/department) from GET auth/me/, which reads the stored
 *    Jira credential. There is no scry account and no session token.
 *  - Watch set (SvelteSet, optimistic toggle + rollback), feed, push prefs.
 *  - Favorites (localStorage `scry:favorites`) / recent issues
 *    (localStorage `scry:recent`, max 30).
 *  - Derived group (part) for smart default views.
 *
 * Read-only features work with no credential. Personalization and writes need
 * a configured credential (`identified` === email !== null).
 *
 * Reactivity: use svelte/reactivity SvelteSet so add/delete trigger updates.
 */

import { t, type MessageKey } from '../lib/i18n'
import { SvelteSet } from 'svelte/reactivity'
import * as api from '../lib/api'
import { basePath, config, feature } from '../lib/config'
import { STORAGE_KEYS } from '../lib/storage'
import { issues } from './issues.svelte'
import type {
  FeedFocus,
  FeedItem,
  FeedUnreadCounts,
  NotificationConfig,
  NotificationPreferences,
} from '../lib/types'

export type { FeedFocus } from '../lib/types'

/* Auth API lives outside the issues API base. Read at call time so a
 * loadConfig() override (hosted demo api/auth base under /scry/) is honoured —
 * a module-level capture would freeze DEFAULTS before config.json loads. */

const FAVORITES_KEY = STORAGE_KEYS.favorites
const RECENT_KEY = STORAGE_KEYS.recent
const RECENT_MAX = 30

function loadArray(key: string): string[] {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    const arr = JSON.parse(raw) as unknown
    return Array.isArray(arr) ? (arr.filter((v) => typeof v === 'string') as string[]) : []
  } catch {
    return []
  }
}

function saveArray(key: string, arr: string[]): void {
  try {
    localStorage.setItem(key, JSON.stringify(arr))
  } catch (e) {
    console.warn(`[me] ${key} 저장 실패`, e)
  }
}

function urlBase64ToUint8Array(value: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (value.length % 4)) % 4)
  const raw = atob((value + padding).replace(/-/g, '+').replace(/_/g, '/'))
  const bytes = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i += 1) bytes[i] = raw.charCodeAt(i)
  return bytes
}

export interface RecentIssueVisit {
  key: string
  viewed_at: string | null
}

function loadRecent(): RecentIssueVisit[] {
  try {
    const raw = localStorage.getItem(RECENT_KEY)
    if (!raw) return []
    const values = JSON.parse(raw) as unknown
    if (!Array.isArray(values)) return []

    const seen = new Set<string>()
    const visits: RecentIssueVisit[] = []
    for (const value of values) {
      // Legacy string[] keeps order; fill exact timestamps on next view.
      const key = typeof value === 'string' ? value : isRecentVisit(value) ? value.key : ''
      if (!key || seen.has(key)) continue
      seen.add(key)
      visits.push({
        key,
        viewed_at: typeof value === 'string' ? null : value.viewed_at,
      })
      if (visits.length === RECENT_MAX) break
    }
    return visits
  } catch {
    return []
  }
}

function isRecentVisit(value: unknown): value is RecentIssueVisit {
  if (!value || typeof value !== 'object') return false
  const visit = value as Record<string, unknown>
  return (
    typeof visit.key === 'string' &&
    (visit.viewed_at === null || typeof visit.viewed_at === 'string')
  )
}

function saveRecent(visits: RecentIssueVisit[]): void {
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(visits))
  } catch (e) {
    console.warn(`[me] ${RECENT_KEY} 저장 실패`, e)
  }
}

export type PushState =
  | 'unsupported'
  | 'unavailable'
  | 'default'
  | 'denied'
  | 'subscribed'
  | 'unsubscribed'
  | 'loading'

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
  name = $state<string | null>(null)
  department = $state<string | null>(null)
  /** init() finished — personalization UI can branch without a flash. */
  authChecked = $state(false)

  /* ── Watches ── */
  watches = new SvelteSet<string>()

  /* ── Server personal feed / read state ── */
  feedItems = $state<FeedItem[]>([])
  feedUnread = $state<FeedUnreadCounts>({ ...EMPTY_UNREAD })
  feedLoaded = $state(false)
  feedLoading = $state(false)

  /* ── Web Push ── */
  notificationConfig = $state<NotificationConfig | null>(null)
  pushState = $state<PushState>('default')
  pushError = $state<string | null>(null)

  /* ── Local personalization (works without credential) ── */
  favorites = new SvelteSet<string>()
  recent = $state<RecentIssueVisit[]>([])

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
   *  - Always restore local favorites/recent.
   *  - Always probe GET auth/me/ — configured credential → email (identity);
   *    otherwise 200 {email:null} and stay anonymous (render-before-auth).
   */
  async init(): Promise<void> {
    if (this.#initialized) return
    this.#initialized = true

    for (const key of loadArray(FAVORITES_KEY)) this.favorites.add(key)
    this.recent = loadRecent()

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
      name?: string
      department?: string
    }
    if (data.email) {
      const wasIdentified = this.email !== null
      this.#setUser(data.email, data.name ?? null, data.department ?? null)
      if (opts.loadPersonal && !wasIdentified) {
        await Promise.all([this.loadWatches(), this.loadFeed(), this.loadNotificationConfig()])
        this.#startFeedPolling()
      }
    } else if (this.email !== null) {
      this.#clearIdentity()
    }
  }

  #setUser(email: string, name: string | null, department: string | null): void {
    this.email = email
    this.name = name
    this.department = department
  }

  /** Drop identity and personal server state; keep local favorites/recent. */
  #clearIdentity(): void {
    this.email = null
    this.name = null
    this.department = null
    this.watches.clear()
    this.feedItems = []
    this.feedUnread = { ...EMPTY_UNREAD }
    this.feedLoaded = false
    this.#feedBaselineReady = false
    this.#prevUnreadAll = 0
    this.notificationConfig = null
    this.pushState = 'default'
    this.#syncAppBadge()
  }

  /* ── Watches ── */

  async loadWatches(): Promise<void> {
    try {
      const res = await api.getWatches()
      this.watches.clear()
      for (const k of res.keys) this.watches.add(k)
    } catch (e) {
      console.warn('[me] 워치 로드 실패', e)
    }
  }

  isWatching(key: string): boolean {
    return this.watches.has(key)
  }

  /** Optimistic toggle; rolls back on failure. Returns false when not identified. */
  async toggleWatch(key: string): Promise<boolean> {
    if (!this.identified) return false
    const wasWatching = this.watches.has(key)
    if (wasWatching) this.watches.delete(key)
    else this.watches.add(key)
    try {
      if (wasWatching) await api.removeWatch(key)
      else await api.addWatch(key)
      return true
    } catch (e) {
      console.warn('[me] 워치 토글 실패(롤백)', e)
      if (wasWatching) this.watches.add(key)
      else this.watches.delete(key)
      return false
    }
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
        tag: `scry-feed-${newest.event_id}`,
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

  /* ── Web Push ── */

  get pushSupported(): boolean {
    return (
      typeof window !== 'undefined' &&
      'serviceWorker' in navigator &&
      'PushManager' in window &&
      'Notification' in window
    )
  }

  /** Skip config fetch and SW registration when push is off. */
  async loadNotificationConfig(): Promise<void> {
    if (!feature('push') || !this.identified) return
    try {
      this.notificationConfig = await api.getNotificationConfig()
      if (!this.pushSupported) {
        this.pushState = 'unsupported'
        return
      }
      if (!this.notificationConfig.enabled) {
        this.pushState = 'unavailable'
        return
      }
      if (Notification.permission === 'denied') {
        this.pushState = 'denied'
        return
      }
      const registration = await navigator.serviceWorker.register(`${basePath()}sw.js`, {
        scope: basePath(),
      })
      const subscription = await registration.pushManager.getSubscription()
      this.pushState = subscription ? 'subscribed' : 'unsubscribed'
    } catch (e) {
      console.warn('[me] 웹 알림 설정 로드 실패', e)
      this.pushState = 'unavailable'
    }
  }

  async enablePush(): Promise<boolean> {
    if (!this.pushSupported || !this.notificationConfig?.enabled) return false
    this.pushState = 'loading'
    this.pushError = null
    try {
      const permission = await Notification.requestPermission()
      if (permission !== 'granted') {
        this.pushState = permission === 'denied' ? 'denied' : 'unsubscribed'
        return false
      }
      const registration = await navigator.serviceWorker.register(`${basePath()}sw.js`, {
        scope: basePath(),
      })
      const existing = await registration.pushManager.getSubscription()
      const subscription =
        existing ??
        (await registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(
            this.notificationConfig.vapid_public_key,
          ),
        }))
      const serialized = subscription.toJSON()
      const endpoint = serialized.endpoint ?? subscription.endpoint
      const p256dh = serialized.keys?.p256dh
      const auth = serialized.keys?.auth
      if (!p256dh || !auth) throw new Error(t('me.noCryptoKey'))
      await api.savePushSubscription({ endpoint, keys: { p256dh, auth } })
      this.pushState = 'subscribed'
      return true
    } catch (e) {
      console.warn('[me] 웹 알림 활성화 실패', e)
      this.pushState = 'unsubscribed'
      this.pushError = t('me.enableNotifFailed')
      return false
    }
  }

  async disablePush(): Promise<void> {
    if (!this.pushSupported) return
    this.pushState = 'loading'
    this.pushError = null
    try {
      const registration = await navigator.serviceWorker.getRegistration(basePath())
      const subscription = await registration?.pushManager.getSubscription()
      if (subscription) {
        await api.deletePushSubscription(subscription.endpoint)
        await subscription.unsubscribe()
      }
      this.pushState = 'unsubscribed'
    } catch (e) {
      console.warn('[me] 웹 알림 해제 실패', e)
      this.pushState = 'subscribed'
      this.pushError = t('me.disableNotifFailed')
    }
  }

  async updateNotificationPreferences(
    patch: Partial<NotificationPreferences>,
  ): Promise<void> {
    if (!this.notificationConfig) return
    const previous = this.notificationConfig
    this.notificationConfig = {
      ...previous,
      preferences: { ...previous.preferences, ...patch },
    }
    try {
      this.notificationConfig = await api.updateNotificationPreferences(patch)
    } catch (e) {
      this.notificationConfig = previous
      console.warn('[me] 알림 선호 저장 실패', e)
    }
  }

  /* ── Favorites (local) ── */

  isFavorite(key: string): boolean {
    return this.favorites.has(key)
  }

  toggleFavorite(key: string): void {
    if (this.favorites.has(key)) this.favorites.delete(key)
    else this.favorites.add(key)
    saveArray(FAVORITES_KEY, [...this.favorites])
  }

  reorderFavorite(sourceKey: string, targetKey: string): void {
    if (sourceKey === targetKey) return
    const ordered = [...this.favorites]
    const sourceIndex = ordered.indexOf(sourceKey)
    const targetIndex = ordered.indexOf(targetKey)
    if (sourceIndex < 0 || targetIndex < 0) return

    const [moved] = ordered.splice(sourceIndex, 1)
    const insertAt = ordered.indexOf(targetKey) + (sourceIndex < targetIndex ? 1 : 0)
    ordered.splice(insertAt, 0, moved)
    this.favorites.clear()
    for (const key of ordered) this.favorites.add(key)
    saveArray(FAVORITES_KEY, ordered)
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

  /* ── Recent issues (local) ── */

  recordRecent(key: string): void {
    if (!key) return
    const next = [
      { key, viewed_at: new Date().toISOString() },
      ...this.recent.filter((visit) => visit.key !== key),
    ].slice(0, RECENT_MAX)
    this.recent = next
    saveRecent(next)
  }
}

/** App-wide singleton. */
export const me = new MeStore()
