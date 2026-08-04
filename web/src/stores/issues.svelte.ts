/*
 * Issue Navigator — 이슈 풀 스토어 (계약 §2)
 *
 * "브라우저를 DB로": 이 스토어가 유일한 진실원(메모리 풀)이고, IndexedDB 는 영속 캐시,
 *  서버는 백그라운드 동기화 소스다. 리스트/필터/검색/그룹핑은 전부 이 풀 위 $derived 로 계산된다.
 *
 * ⚠️ Svelte 5 반응성 주의:
 *  - Map 반응성은 순정 Map 이 아니라 `svelte/reactivity` 의 SvelteMap 을 써야 한다
 *    (순정 Map 은 set/delete 가 반응성을 트리거하지 않는다).
 *  - `$state`/`$derived` 클래스 필드는 컴파일 시 getter 로 바뀌어 모듈 경계 너머에서도
 *    반응형으로 읽힌다. 그래서 싱글턴 인스턴스를 export 해 어디서든 `issues.allIssues` 를 읽으면 된다.
 */

import { SvelteMap } from 'svelte/reactivity'
import type { IssueLite, Member, SyncHealth } from '../lib/types'
import * as api from '../lib/api'
import * as db from '../lib/db'
import type { CacheMeta } from '../lib/types'

const POLL_MS = 15_000

class IssuesStore {
  /** issue_key → IssueLite. delta 는 이 Map 의 개별 항목만 교체 → 해당 행만 리렌더. */
  pool = new SvelteMap<string, IssueLite>()
  /** email → Member (아바타/파트). */
  members = new SvelteMap<string, Member>()

  /** 캐시 하이드레이션 또는 최초 bootstrap 이 끝나 UI 가 동작 가능한 상태. */
  ready = $state(false)
  /** 마지막으로 서버와 동기화된 커서(server_time). 다음 delta 의 since. */
  lastSync = $state('')
  /** Jira/Qase/멤버 소스별 서버 판정. 캐시에도 저장해 첫 렌더부터 표시한다. */
  syncHealth = $state<SyncHealth | null>(null)
  /** 부팅 실패(주로 인증). 캐시가 없을 때만 UI 를 막는다(render-before-auth). */
  error = $state<string | null>(null)
  /**
   * True when a poll/bootstrap failed after the pool was already ready —
   * UI shows an "Offline — showing cached data" strip without blocking the list.
   */
  offline = $state(false)

  /** updated_at 내림차순 전체 리스트. 필터/그룹핑의 기준 컬렉션(계약). */
  allIssues = $derived.by(() => {
    const arr = [...this.pool.values()]
    arr.sort((a, b) => {
      const av = a.updated_at ?? ''
      const bv = b.updated_at ?? ''
      return av < bv ? 1 : av > bv ? -1 : 0
    })
    return arr
  })

  // ── 내부 상태(비반응형) ──
  #etag: string | null = null
  #syncVersion = 0
  /** 멤버 셋 안정 해시. delta 의 mv 로 보내 변경 없을 때 members 재전송을 생략시킨다. */
  #membersVersion = ''
  #initialized = false
  #syncing = false
  #pollTimer: ReturnType<typeof setInterval> | null = null

  /**
   * 부팅 시퀀스:
   *  ① IndexedDB → 메모리 하이드레이션 (있으면 즉시 ready)
   *  ② 백그라운드 bootstrap(ETag 304) 또는 delta(since=저장된 server_time)
   *  ③ 15초 delta 폴링 시작 (탭 복귀 시 즉시 1회)
   */
  async init(): Promise<void> {
    if (this.#initialized) return
    this.#initialized = true

    // ① 하이드레이션
    try {
      const [cached, meta] = await Promise.all([db.getAllIssues(), db.getMeta()])
      if (meta) {
        for (const m of meta.members) this.members.set(m.email, m)
        this.lastSync = meta.server_time
        this.#syncVersion = meta.sync_version
        this.#membersVersion = meta.members_version ?? ''
        this.#etag = `"in-${meta.sync_version}"`
        this.syncHealth = meta.sync_health ?? null
      }
      if (cached.length > 0) {
        for (const it of cached) this.pool.set(it.issue_key, it)
        this.ready = true // 캐시로 즉시 사용 가능
      }
    } catch (e) {
      console.warn('[issues] hydration 실패', e)
    }

    // ② 백그라운드 동기화
    await this.#sync()

    // ③ 폴링
    this.#startPolling()
  }

  /** 캐시 유무에 따라 delta 또는 bootstrap 을 고른다. */
  async #sync(): Promise<void> {
    if (this.#syncing) return
    this.#syncing = true
    try {
      if (this.pool.size > 0 && this.lastSync) {
        await this.#deltaSync()
      } else {
        await this.#bootstrap()
      }
      this.error = null
      this.offline = false
    } catch (e) {
      const status = e instanceof api.ApiError ? e.status : 0
      if (status === 401) {
        // 캐시가 있으면 조용히 넘어가고(render-before-auth), 없으면 UI 를 막는다.
        if (!this.ready) this.error = 'auth'
        else this.offline = true
      } else {
        console.warn('[issues] sync 실패', e)
        if (!this.ready) this.error = 'network'
        else this.offline = true
      }
    } finally {
      this.#syncing = false
    }
  }

  async #bootstrap(): Promise<void> {
    const res = await api.getBootstrap(this.#etag)
    if (res.status === 'not_modified') {
      this.ready = true
      return
    }
    const { data, etag } = res

    // 전량 교체 (오래된 tombstone 잔재 제거)
    this.pool.clear()
    for (const it of data.issues) this.pool.set(it.issue_key, it)
    this.members.clear()
    for (const m of data.members) this.members.set(m.email, m)

    this.lastSync = data.server_time
    this.#syncVersion = data.sync_version
    this.#membersVersion = data.members_version ?? ''
    this.#etag = etag ?? `"in-${data.sync_version}"`
    this.syncHealth = data.sync_health
    this.ready = true

    // 영속화 (실패해도 메모리 풀은 이미 최신 — IndexedDB 없이도 동작)
    try {
      await db.replaceAllIssues(data.issues)
      await this.#persistMeta()
    } catch (e) {
      console.warn('[issues] bootstrap 영속화 실패', e)
    }
  }

  async #deltaSync(): Promise<void> {
    if (!this.lastSync) return this.#bootstrap()
    const delta = await api.getDelta(this.lastSync, this.#membersVersion)
    await this.applyDelta(
      delta.upserted,
      delta.deleted_keys,
      delta.server_time,
      delta.members,
      delta.sync_health,
      delta.members_version,
    )
  }

  /**
   * delta 적용 — 변경 이슈만 교체 + 삭제 키 제거 + IndexedDB 반영 + 커서 전진.
   * (직접 delta 를 받아 넣고 싶은 후속 코드도 이 메서드를 재사용)
   */
  async applyDelta(
    upserted: IssueLite[],
    deletedKeys: string[],
    serverTime: string,
    members?: Member[],
    syncHealth?: SyncHealth,
    membersVersion?: string,
  ): Promise<void> {
    for (const it of upserted) this.pool.set(it.issue_key, it)
    for (const key of deletedKeys) this.pool.delete(key)
    // members 는 변경 시에만 온다(생략 시 기존 멤버 유지). 해시는 오면 갱신.
    if (members) {
      this.members.clear()
      for (const member of members) this.members.set(member.email, member)
    }
    if (membersVersion !== undefined) this.#membersVersion = membersVersion
    if (syncHealth) this.syncHealth = syncHealth
    this.lastSync = serverTime

    // 영속화 (실패해도 메모리 풀은 이미 최신)
    try {
      await db.putIssues(upserted)
      await db.deleteIssues(deletedKeys)
      await this.#persistMeta()
    } catch (e) {
      console.warn('[issues] delta 영속화 실패', e)
    }
  }

  async #persistMeta(): Promise<void> {
    // ⚠️ members·sync_health 는 $state/Svelte 프록시라 그대로 IndexedDB.put 하면
    // structuredClone 이 DataCloneError 로 실패한다. $state.snapshot 으로 plain 화한다.
    await db.putMeta(
      $state.snapshot({
        key: 'sync',
        server_time: this.lastSync,
        sync_version: this.#syncVersion,
        members: [...this.members.values()],
        members_version: this.#membersVersion,
        sync_health: this.syncHealth ?? undefined,
      }) as CacheMeta,
    )
  }

  #startPolling(): void {
    if (this.#pollTimer || typeof window === 'undefined') return
    this.#pollTimer = setInterval(() => {
      void this.#sync()
    }, POLL_MS)
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible') void this.#sync()
    })
  }

  /** 수동 새로고침(탭 복귀/사용자 트리거). */
  async refresh(): Promise<void> {
    await this.#sync()
  }

  /** 편의 조회. */
  get(issueKey: string): IssueLite | undefined {
    return this.pool.get(issueKey)
  }

  memberOf(email: string | null | undefined): Member | undefined {
    return email ? this.members.get(email) : undefined
  }
}

/** 앱 전역 싱글턴. 어디서든 `import { issues } from '../stores/issues.svelte'`. */
export const issues = new IssuesStore()
