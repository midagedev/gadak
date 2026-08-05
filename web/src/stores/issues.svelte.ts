/*
 * Issue Navigator — issue pool store (contract §2)
 *
 * "Browser as DB": this store is the sole source of truth (in-memory pool);
 *  IndexedDB is the durable cache, and the server is a background sync source.
 *  List/filter/search/grouping all derive from this pool via $derived.
 *
 * ⚠️ Svelte 5 reactivity traps:
 *  - Map reactivity needs `svelte/reactivity` SvelteMap, not a plain Map
 *    (plain Map set/delete does not trigger reactivity).
 *  - `$state`/`$derived` class fields compile to getters and stay reactive
 *    across module boundaries — export a singleton and read `issues.allIssues`
 *    anywhere.
 */

import { SvelteMap } from 'svelte/reactivity'
import type { FieldSpec, IssueLite, Member, SyncHealth } from '../lib/types'
import * as api from '../lib/api'
import * as db from '../lib/db'
import type { CacheMeta } from '../lib/types'

const POLL_MS = 15_000

class IssuesStore {
  /** issue_key → IssueLite. Delta replaces individual Map entries → only those rows re-render. */
  pool = new SvelteMap<string, IssueLite>()
  /** email → Member (avatar/part). */
  members = new SvelteMap<string, Member>()

  /** True once cache hydration or first bootstrap finishes and the UI can run. */
  ready = $state(false)
  /** Last server sync cursor (server_time). Used as delta's since. */
  lastSync = $state('')
  /** Per-source (Jira/Qase/members) server health. Cached so first paint can show it. */
  syncHealth = $state<SyncHealth | null>(null)
  /** Discovered custom fields (bootstrap field_specs). Drives detail rows and filter axes. */
  fieldSpecs = $state<FieldSpec[]>([])
  /** project → alias → filled count. Which fields a board actually uses. */
  fieldUsage = $state<Record<string, Record<string, number>>>({})
  /** Boot failure (usually auth). Blocks UI only when there is no cache (render-before-auth). */
  error = $state<string | null>(null)
  /**
   * True when a poll/bootstrap failed after the pool was already ready —
   * UI shows an "Offline — showing cached data" strip without blocking the list.
   */
  offline = $state(false)

  /** Full list sorted by updated_at desc. Canonical collection for filter/grouping (contract). */
  allIssues = $derived.by(() => {
    const arr = [...this.pool.values()]
    arr.sort((a, b) => {
      const av = a.updated_at ?? ''
      const bv = b.updated_at ?? ''
      return av < bv ? 1 : av > bv ? -1 : 0
    })
    return arr
  })

  // ── Internal state (non-reactive) ──
  #etag: string | null = null
  #syncVersion = 0
  /** Stable members-set hash. Sent as delta mv so unchanged members are omitted. */
  #membersVersion = ''
  #initialized = false
  #syncing = false
  #pollTimer: ReturnType<typeof setInterval> | null = null

  /**
   * Boot sequence:
   *  ① IndexedDB → memory hydration (ready immediately when cache hits)
   *  ② Background bootstrap (ETag 304) or delta (since=stored server_time)
   *  ③ Start 15s delta polling (one immediate sync on tab focus)
   */
  async init(): Promise<void> {
    if (this.#initialized) return
    this.#initialized = true

    // ① Hydration
    try {
      const [cached, meta] = await Promise.all([db.getAllIssues(), db.getMeta()])
      if (meta) {
        for (const m of meta.members) this.members.set(m.email, m)
        this.lastSync = meta.server_time
        this.#syncVersion = meta.sync_version
        this.#membersVersion = meta.members_version ?? ''
        this.#etag = `"in-${meta.sync_version}"`
        this.syncHealth = meta.sync_health ?? null
        this.fieldSpecs = meta.field_specs ?? []
        this.fieldUsage = meta.field_usage ?? {}
      }
      if (cached.length > 0) {
        for (const it of cached) this.pool.set(it.issue_key, it)
        this.ready = true // usable immediately from cache
      }
    } catch (e) {
      console.warn('[issues] hydration 실패', e)
    }

    // ② Background sync
    await this.#sync()

    // ③ Polling
    this.#startPolling()
  }

  /** Pick delta vs bootstrap based on whether a cache exists. */
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
        // With cache: stay quiet (render-before-auth). Without: block the UI.
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

    // Full replace (drop stale tombstone leftovers)
    this.pool.clear()
    for (const it of data.issues) this.pool.set(it.issue_key, it)
    this.members.clear()
    for (const m of data.members) this.members.set(m.email, m)

    this.lastSync = data.server_time
    this.#syncVersion = data.sync_version
    this.#membersVersion = data.members_version ?? ''
    this.#etag = etag ?? `"in-${data.sync_version}"`
    this.syncHealth = data.sync_health
    this.fieldSpecs = data.field_specs ?? []
    this.fieldUsage = data.field_usage ?? {}
    this.ready = true

    // Persist (memory pool is already current even if this fails — works without IndexedDB)
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
    // Discovery may run server-side long after this tab bootstrapped.
    if (delta.field_specs) this.fieldSpecs = delta.field_specs
    if (delta.field_usage) this.fieldUsage = delta.field_usage
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
   * Apply a delta — replace changed issues, drop deleted keys, persist to IndexedDB,
   * advance the cursor. Reusable by later code that wants to inject a delta directly.
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
    // members arrive only when changed (omit → keep existing). Refresh hash when present.
    if (members) {
      this.members.clear()
      for (const member of members) this.members.set(member.email, member)
    }
    if (membersVersion !== undefined) this.#membersVersion = membersVersion
    if (syncHealth) this.syncHealth = syncHealth
    this.lastSync = serverTime

    // Persist (memory pool is already current even if this fails)
    try {
      await db.putIssues(upserted)
      await db.deleteIssues(deletedKeys)
      await this.#persistMeta()
    } catch (e) {
      console.warn('[issues] delta 영속화 실패', e)
    }
  }

  async #persistMeta(): Promise<void> {
    // ⚠️ members·sync_health are $state/Svelte proxies — IndexedDB.put on them
    // fails structuredClone with DataCloneError. Flatten via $state.snapshot.
    await db.putMeta(
      $state.snapshot({
        key: 'sync',
        server_time: this.lastSync,
        sync_version: this.#syncVersion,
        members: [...this.members.values()],
        members_version: this.#membersVersion,
        sync_health: this.syncHealth ?? undefined,
        field_specs: this.fieldSpecs,
        field_usage: this.fieldUsage,
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

  /** Manual refresh (tab focus / user trigger). */
  async refresh(): Promise<void> {
    await this.#sync()
  }

  /** Convenience lookup. */
  get(issueKey: string): IssueLite | undefined {
    return this.pool.get(issueKey)
  }

  memberOf(email: string | null | undefined): Member | undefined {
    return email ? this.members.get(email) : undefined
  }
}

/** App-wide singleton. Import anywhere: `import { issues } from '../stores/issues.svelte'`. */
export const issues = new IssuesStore()
