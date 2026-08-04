/*
 * Issue Navigator — 로그인/개인화 상태 스토어 ([personal], 계약 §2)
 *
 * 역할:
 *  - 로그인 상태(email/name/department/토큰). 토큰은 localStorage(TOKEN_KEY)에 저장 →
 *    api.ts 가 매 요청에 `Authorization: Token` 을 자동으로 붙인다.
 *  - 워치 셋(SvelteSet, 옵티미스틱 토글 + 실패 롤백), 멘션 목록.
 *  - 즐겨찾기(localStorage `issue-nav:favorites`) / 최근 본 이슈(localStorage `issue-nav:recent`, 최대 30).
 *  - 소속 파트(group) 파생 — 스마트 기본값(파트 프리셋)에 쓰인다.
 *
 * 읽기 전용 기능은 비로그인(익명)으로 완전히 동작한다. 개인화 섹션만 로그인을 요구한다.
 *
 * ⚠️ 반응성: Set 은 순정 Set 대신 svelte/reactivity 의 SvelteSet 을 써야 add/delete 가 반응성을 트리거한다.
 */

import { t } from '../lib/i18n'
import { SvelteSet } from 'svelte/reactivity'
import * as api from '../lib/api'
import { TOKEN_KEY, getToken } from '../lib/api'
import { basePath, config, feature } from '../lib/config'
import { issues } from './issues.svelte'
import type {
  FeedFocus,
  FeedItem,
  FeedUnreadCounts,
  NotificationConfig,
  NotificationPreferences,
} from '../lib/types'

export type { FeedFocus } from '../lib/types'

/* 인증 API 는 issues API base 밖이라 여기서 직접 fetch 한다. */
const AUTH_BASE = config().authBase

const FAVORITES_KEY = 'issue-nav:favorites'
const RECENT_KEY = 'issue-nav:recent'
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
      // 기존 string[] 기록은 순서를 보존하고, 다음 조회부터 정확한 시각을 채운다.
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

export interface LoginResult {
  ok: boolean
  error?: string
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

class MeStore {
  /* ── 로그인 상태 ── */
  email = $state<string | null>(null)
  name = $state<string | null>(null)
  department = $state<string | null>(null)
  /** init()(토큰 검증)이 끝났는지 — 개인화 UI 가 깜빡임 없이 분기하도록. */
  authChecked = $state(false)

  /* ── 워치 ── */
  watches = new SvelteSet<string>()

  /* ── 서버 개인 피드 / 읽음 ── */
  feedItems = $state<FeedItem[]>([])
  feedUnread = $state<FeedUnreadCounts>({ ...EMPTY_UNREAD })
  feedLoaded = $state(false)
  feedLoading = $state(false)

  /* ── Web Push ── */
  notificationConfig = $state<NotificationConfig | null>(null)
  pushState = $state<PushState>('default')
  pushError = $state<string | null>(null)

  /* ── 로컬 개인화(비로그인도 동작) ── */
  favorites = new SvelteSet<string>()
  recent = $state<RecentIssueVisit[]>([])

  /** 개인 피드(메인 영역 토글 뷰)가 열려 있는지. */
  feedOpen = $state(false)
  /** 피드 초점 탭(전체/담당/보고/멘션). */
  feedFocus = $state<FeedFocus>('all')
  /** 로그인 다이얼로그 표시 여부(어디서든 promptLogin 으로 연다). */
  loginOpen = $state(false)

  get authed(): boolean {
    return this.email !== null
  }

  /** 내 소속 파트(멤버 목록에서 email 매칭 → group). 스마트 기본값에 사용. */
  get group(): string | null {
    if (!this.email) return null
    return issues.members.get(this.email)?.group ?? null
  }

  #initialized = false
  #feedPollTimer: ReturnType<typeof setInterval> | null = null

  /**
   * 부팅:
   *  - 로컬 개인화(favorites/recent)는 항상 복원.
   *  - 항상 me 확인(GET auth/me/) — 서버 credential 이 설정돼 있으면 email 이 오고
   *    로그인 상태(쓰기/워치 활성). email:null 이면 조용히 익명(render-before-auth).
   */
  async init(): Promise<void> {
    if (this.#initialized) return
    this.#initialized = true

    for (const key of loadArray(FAVORITES_KEY)) this.favorites.add(key)
    this.recent = loadRecent()

    // 서버 credential 이 곧 identity — 토큰 유무와 무관하게 항상 확인한다.
    // (레거시 토큰이 있으면 같이 보내고, 무효 판명 시 정리)
    const token = getToken()
    try {
      const res = await fetch(`${AUTH_BASE}me/`, {
        credentials: 'same-origin',
        ...(token ? { headers: { Authorization: `Token ${token}` } } : {}),
      })
      if (res.ok) {
        const data = (await res.json()) as { email: string | null; name?: string; department?: string }
        if (data.email) {
          this.#setUser(data.email, data.name ?? null, data.department ?? null)
          await Promise.all([this.loadWatches(), this.loadFeed(), this.loadNotificationConfig()])
          this.#startFeedPolling()
        }
      } else if (token) {
        // 토큰 만료/무효 → 정리
        this.#clearToken()
      }
    } catch (e) {
      console.warn('[me] 인증 확인 실패(익명 진행)', e)
    } finally {
      this.authChecked = true
    }
  }

  /** 이메일/비밀번호 로그인. 성공 시 토큰 저장 + 워치/멘션 로드. */
  async login(email: string, password: string): Promise<LoginResult> {
    try {
      const res = await fetch(`${AUTH_BASE}login/`, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: email.trim(), password }),
      })
      const data = (await res.json().catch(() => ({}))) as {
        token?: string
        name?: string
        email?: string
        department?: string
        error?: string
      }
      if (!res.ok || !data.token) {
        return { ok: false, error: data.error ?? t('login.failed') }
      }
      try {
        localStorage.setItem(TOKEN_KEY, data.token)
      } catch (e) {
        console.warn('[me] 토큰 저장 실패', e)
      }
      this.#setUser(data.email ?? email.trim(), data.name ?? null, data.department ?? null)
      await Promise.all([this.loadWatches(), this.loadFeed(), this.loadNotificationConfig()])
      this.#startFeedPolling()
      return { ok: true }
    } catch (e) {
      console.warn('[me] 로그인 요청 실패', e)
      return { ok: false, error: t('login.networkFailed') }
    }
  }

  /** 로그아웃 — 토큰/개인화 상태 정리(로컬 favorites/recent 는 유지). */
  async logout(): Promise<void> {
    if (this.pushState === 'subscribed') {
      await this.disablePush().catch(() => undefined)
    }
    const token = getToken()
    if (token) {
      try {
        await fetch(`${AUTH_BASE}logout/`, {
          method: 'POST',
          credentials: 'same-origin',
          headers: { Authorization: `Token ${token}` },
        })
      } catch {
        // 세션 로그아웃 실패는 무시 — 로컬 토큰만 지우면 충분
      }
    }
    this.#clearToken()
    this.watches.clear()
    this.feedItems = []
    this.feedUnread = { ...EMPTY_UNREAD }
    this.feedLoaded = false
    this.notificationConfig = null
    this.pushState = 'default'
    this.#syncAppBadge()
  }

  #setUser(email: string, name: string | null, department: string | null): void {
    this.email = email
    this.name = name
    this.department = department
  }

  #clearToken(): void {
    try {
      localStorage.removeItem(TOKEN_KEY)
    } catch {
      /* noop */
    }
    this.email = null
    this.name = null
    this.department = null
  }

  /* ── 워치 ── */

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

  /** 워치 토글 — 옵티미스틱 반영 후 서버 반영, 실패 시 롤백. 비로그인은 false 반환(호출부가 유도). */
  async toggleWatch(key: string): Promise<boolean> {
    if (!this.authed) return false
    const wasWatching = this.watches.has(key)
    // 옵티미스틱
    if (wasWatching) this.watches.delete(key)
    else this.watches.add(key)
    try {
      if (wasWatching) await api.removeWatch(key)
      else await api.addWatch(key)
      return true
    } catch (e) {
      console.warn('[me] 워치 토글 실패(롤백)', e)
      // 롤백
      if (wasWatching) this.watches.add(key)
      else this.watches.delete(key)
      return false
    }
  }

  /* ── 개인 피드 / 읽음 ── */

  /** 서버가 피드를 제공하지 않으면(feed=false) 아무 요청도 하지 않는다. 읽음 처리 계열도
   *  전부 feedItems 가 비어 있어 자연히 no-op 이 된다. */
  async loadFeed(focus: FeedFocus = this.feedFocus): Promise<void> {
    if (!feature('feed') || !this.authed) return
    this.feedLoading = true
    try {
      const response = await api.getFeed(focus)
      if (focus === this.feedFocus) this.feedItems = response.items
      this.feedUnread = response.unread_counts
      this.feedLoaded = true
      this.#syncAppBadge()
    } catch (e) {
      console.warn('[me] 피드 로드 실패', e)
    } finally {
      this.feedLoading = false
    }
  }

  async markEventRead(eventId: string): Promise<void> {
    if (!this.authed) return
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
    if (!this.authed || !eventIds.length) return
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
    if (!this.authed || !issueKey) return
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
    if (!this.authed || this.feedUnread.all === 0) return
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
      if (this.authed) void this.loadFeed()
    }, 15_000)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible' && this.authed) void this.loadFeed()
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

  /** 웹 푸시가 꺼져 있으면 설정 조회도, 서비스워커 등록도 하지 않는다. */
  async loadNotificationConfig(): Promise<void> {
    if (!feature('push') || !this.authed) return
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

  /* ── 즐겨찾기 (로컬) ── */

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

  /* ── 개인 피드 토글 ── */

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

  /* ── 로그인 다이얼로그 ── */

  promptLogin(): void {
    this.loginOpen = true
  }

  closeLogin(): void {
    this.loginOpen = false
  }

  /* ── 최근 본 이슈 (로컬) ── */

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

/** 앱 전역 싱글턴. */
export const me = new MeStore()
