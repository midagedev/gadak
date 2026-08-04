/*
 * Issue Navigator — 실시간 프레즌스 스토어 (Google Drive식 "지금 누가 보고 있나")
 *
 * 백엔드와 합의한 WS 프로토콜:
 *  - 티켓 발급: GET presence-ticket/ → { ticket }
 *  - 접속: wss://<host><base>ws/issues/?ticket=<ticket>
 *  - 클라→서버: {"t":"view","issue":KEY|null}(상세 열림/닫힘), {"t":"hb"}(20초)
 *  - 서버→클라: {"t":"presence","viewers":{KEY:[{email,name}]},"online":[{email,name}]} — 항상 전체 스냅샷
 *
 * 설계 원칙(콕핏): 실패는 조용히(warn 1회, UI 무영향), 표시는 미묘하게.
 *  끊기면 지수 백오프(1s→…→30s)로 티켓을 다시 발급받아 재접속한다.
 *
 * ⚠️ 반응성: viewers 는 SvelteMap(개별 키 세분 반응) — 뷰어 없는 행은 O(1) get 만 하고 오버헤드 없음.
 */

import { SvelteMap } from 'svelte/reactivity'
import * as api from '../lib/api'
import { basePath } from '../lib/config'
import { issues } from './issues.svelte'
import { me } from './me.svelte'

export interface Viewer {
  email: string
  name: string
}

const HB_INTERVAL_MS = 20_000
const MAX_BACKOFF_MS = 30_000
const HIDDEN_HB_STOP_MS = 20_000

/** 뷰어 없는 행이 매번 새 배열을 만들지 않도록 공유하는 빈 배열(참조 안정). */
const EMPTY: readonly Viewer[] = Object.freeze([])

class PresenceStore {
  /** issueKey → 그 이슈를 보고 있는 사람들(본인 포함, 전체 스냅샷). */
  viewers = new SvelteMap<string, Viewer[]>()
  /** 현재 접속(온라인)한 사람들. */
  online = $state<Viewer[]>([])

  // ── 내부 상태(비반응형) ──
  #ws: WebSocket | null = null
  #initialized = false
  /** 현재 열려 있는 상세 이슈 키(재접속/복귀 시 재전송용). */
  #currentIssue: string | null = null
  #backoff = 1000
  #reconnectTimer: ReturnType<typeof setTimeout> | null = null
  #hbTimer: ReturnType<typeof setInterval> | null = null
  #hiddenTimer: ReturnType<typeof setTimeout> | null = null
  #warned = false

  /**
   * 부팅: visibilitychange 리스너 등록 + 최초 접속 시도.
   * 실패해도 조용히 재시도 루프로 넘어가므로 호출부는 결과를 신경 쓸 필요 없다.
   */
  init(): void {
    if (this.#initialized || typeof window === 'undefined') return
    this.#initialized = true
    document.addEventListener('visibilitychange', this.#onVisibility)
    void this.#connect()
  }

  /**
   * 상세 열림/닫힘 반영. 연결 안 됐으면 큐잉 없이 무시하고,
   * 재접속(onopen) 시 #currentIssue 로 자동 재전송된다.
   */
  setViewing(issueKey: string | null): void {
    this.#currentIssue = issueKey
    this.#sendView()
  }

  /** 본인 제외한 특정 이슈의 뷰어. 없으면 참조 안정한 빈 배열. */
  viewersOf(key: string): readonly Viewer[] {
    const list = this.viewers.get(key)
    if (!list || list.length === 0) return EMPTY
    const myEmail = me.email
    const filtered = myEmail ? list.filter((v) => v.email !== myEmail) : list
    return filtered.length ? filtered : EMPTY
  }

  // ── 접속/재접속 ──

  async #connect(): Promise<void> {
    if (typeof window === 'undefined' || this.#ws) return

    let ticket: string
    try {
      ticket = await api.getPresenceTicket()
    } catch (e) {
      this.#warnOnce('티켓 발급 실패', e)
      this.#scheduleReconnect()
      return
    }

    try {
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
      const url = `${proto}//${location.host}${basePath()}ws/issues/?ticket=${encodeURIComponent(ticket)}`
      const ws = new WebSocket(url)
      this.#ws = ws

      ws.onopen = () => {
        this.#backoff = 1000 // 성공 시 백오프 리셋
        this.#startHeartbeat()
        this.#sendView() // 현재 선택 재전송
      }
      ws.onmessage = (ev) => this.#onMessage(ev)
      ws.onerror = () => this.#warnOnce('WS 오류', null) // 뒤이어 onclose 가 재접속 처리
      ws.onclose = () => {
        if (this.#ws === ws) this.#ws = null
        this.#stopHeartbeat()
        // 끊긴 동안 스테일 표시를 남기지 않는다(재접속 시 스냅샷으로 복원됨).
        this.viewers.clear()
        this.online = []
        this.#scheduleReconnect()
      }
    } catch (e) {
      this.#warnOnce('WS 접속 실패', e)
      this.#ws = null
      this.#scheduleReconnect()
    }
  }

  #scheduleReconnect(): void {
    if (this.#reconnectTimer) return
    const delay = this.#backoff
    this.#reconnectTimer = setTimeout(() => {
      this.#reconnectTimer = null
      void this.#connect()
    }, delay)
    this.#backoff = Math.min(this.#backoff * 2, MAX_BACKOFF_MS)
  }

  // ── 메시지 ──

  #onMessage(ev: MessageEvent): void {
    let msg: { t?: string; viewers?: Record<string, Viewer[]>; online?: Viewer[] }
    try {
      msg = JSON.parse(ev.data as string)
    } catch {
      return
    }
    if (msg.t === 'sync') {
      // 서버 데이터 변경 푸시 — 15초 폴링을 기다리지 않고 즉시 delta 동기화.
      // 전원이 동시에 받으므로 0~1.5초 지터로 분산 (delta 는 mv 다이어트로 ~0.6KB).
      setTimeout(() => void issues.refresh(), Math.random() * 1500)
      return
    }
    if (msg.t !== 'presence') return

    // 전체 스냅샷 reconcile — 뷰어 있는 키만 유지(populated 키는 소수라 저렴).
    const next = msg.viewers ?? {}
    const seen = new Set<string>()
    for (const [key, list] of Object.entries(next)) {
      if (Array.isArray(list) && list.length > 0) {
        seen.add(key)
        this.viewers.set(key, list)
      }
    }
    for (const key of [...this.viewers.keys()]) {
      if (!seen.has(key)) this.viewers.delete(key)
    }
    this.online = msg.online ?? []
  }

  #sendView(): void {
    this.#send({ t: 'view', issue: this.#currentIssue })
  }

  #send(msg: Record<string, unknown>): void {
    if (this.#ws && this.#ws.readyState === WebSocket.OPEN) {
      try {
        this.#ws.send(JSON.stringify(msg))
      } catch {
        /* 전송 실패는 곧 onclose→재접속으로 수렴 */
      }
    }
  }

  // ── 하트비트 ──

  #startHeartbeat(): void {
    this.#stopHeartbeat()
    this.#hbTimer = setInterval(() => this.#send({ t: 'hb' }), HB_INTERVAL_MS)
  }

  #stopHeartbeat(): void {
    if (this.#hbTimer) {
      clearInterval(this.#hbTimer)
      this.#hbTimer = null
    }
  }

  // ── 탭 가시성: hidden 20초 후 hb 중단(서버가 스테일 정리), visible 복귀 시 즉시 재개 ──

  #onVisibility = (): void => {
    if (document.visibilityState === 'hidden') {
      if (this.#hiddenTimer) return
      this.#hiddenTimer = setTimeout(() => {
        this.#hiddenTimer = null
        this.#stopHeartbeat()
      }, HIDDEN_HB_STOP_MS)
      return
    }
    // visible 복귀
    if (this.#hiddenTimer) {
      clearTimeout(this.#hiddenTimer)
      this.#hiddenTimer = null
    }
    if (this.#ws && this.#ws.readyState === WebSocket.OPEN) {
      this.#startHeartbeat()
      this.#send({ t: 'hb' })
      this.#sendView()
    } else {
      // 끊긴 채로 돌아왔으면 즉시 재접속(대기 백오프 취소).
      this.#backoff = 1000
      if (this.#reconnectTimer) {
        clearTimeout(this.#reconnectTimer)
        this.#reconnectTimer = null
      }
      void this.#connect()
    }
  }

  #warnOnce(label: string, e: unknown): void {
    if (this.#warned) return
    this.#warned = true
    console.warn(`[presence] ${label} — 프레즌스 비활성(재시도 계속)`, e ?? '')
  }
}

/** 앱 전역 싱글턴. */
export const presence = new PresenceStore()
