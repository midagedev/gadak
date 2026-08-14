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
import type { FieldSpec, IssueLite, Member, SyncHealth, SyncSourceHealth } from '../lib/types'
import * as api from '../lib/api'
import * as db from '../lib/db'
import { applyCacheScopeDebug, isHostedDemo } from '../lib/config'
import { invalidate, invalidateAll } from '../lib/detail-cache.svelte'
import type { CacheMeta } from '../lib/types'

const POLL_MS = 15_000
/** Sync-progress cadence: tight while the mirror moves, slow while it does not. */
const ACTIVITY_BUSY_MS = 2_000
const ACTIVITY_IDLE_MS = 15_000
/**
 * How long a pass has to run before the UI mentions it. The watch loop's
 * incremental passes finish in a second or two, every minute — narrating those
 * would put a status that blinks on and off in front of someone all day, which
 * is noise, not information. What needed saying was the six-minute backfill.
 */
const ACTIVITY_MIN_VISIBLE_MS = 4_000

/**
 * How old the mirror may be when a backgrounded tab comes back before we pull
 * it from Jira ourselves. The server's watch loop runs every 60s, so this is
 * "slept through two ticks" — past that, polling the server faster only fetches
 * a fresher copy of an old mirror.
 */
const FOCUS_PULL_MAX_AGE_MS = 120_000

/** What lib/sync-now hands this store so the mirror can be pulled from here. */
export type MirrorPuller = (mode: 'full' | 'incremental', quiet: boolean) => Promise<void>

class IssuesStore {
  /** issue_key → IssueLite. Delta replaces individual Map entries → only those rows re-render. */
  pool = new SvelteMap<string, IssueLite>()
  /** email or account id → Member (avatar/part). Email-less people key on account id. */
  members = new SvelteMap<string, Member>()
  /** normalized email → Member, for case-insensitive legacy filter tokens. */
  #membersByNormalizedEmail = new SvelteMap<string, Member>()
  /** jira_account_id → Member. O(1) on the filter path. */
  #membersByAccountId = new SvelteMap<string, Member>()

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
  /** Newer published release, when the server's daily check found one. */
  latestVersion = $state('')
  releaseUrl = $state('')
  /** Boot failure (usually auth). Blocks UI only when there is no cache (render-before-auth). */
  error = $state<string | null>(null)
  /**
   * True when a poll/bootstrap failed after the pool was already ready —
   * UI shows an "Offline — showing cached data" strip without blocking the list.
   */
  offline = $state(false)
  /**
   * True while a mirror pull (sync-now) runs — the server↔Jira leg, not the
   * 15s delta poll. Drives the freshness chip's spinner wherever the pull was
   * started from (chip click, focus auto-pull).
   */
  mirrorSyncing = $state(false)

  /**
   * What the mirror is fetching right now, whoever started it.
   *
   * mirrorSyncing above only knows about pulls this tab started. The background
   * watch loop does the long ones — a first Confluence backfill ran for six and
   * a half minutes with the screen saying nothing, because no surface here
   * could see it. The server reports every pass through sync/progress/'s
   * `activity`, and this is where it lands so one status line can speak for the
   * whole mirror instead of each surface guessing.
   *
   * `source` names the connector mid-pass ('issues' | 'documents'); `fetched`
   * is that pass's running total, reset when the source changes — carrying
   * Jira's count into the Confluence phase would report documents the mirror
   * does not have.
   */
  mirrorActivity = $state<{ running: boolean; source: string; fetched: number }>({
    running: false,
    source: '',
    fetched: 0,
  })

  /** True when anything is fetching: this tab's pull, or the background loop. */
  get mirrorBusy(): boolean {
    return this.mirrorSyncing || this.mirrorActivity.running
  }

  /**
   * Health of the issue mirror's own source. `synced_at` here is the last sync
   * run that finished without an error (server: store.SyncState.SyncedAt), so
   * it is the one field that means "the mirror is fresh" — the delta cursor
   * (lastSync) only measures the browser↔server leg. Falls back to the first
   * reported source so a mirror without a `jira` row still has an age.
   */
  get mirrorHealth(): SyncSourceHealth | null {
    const sources = this.syncHealth?.sources
    if (!sources || sources.length === 0) return null
    return sources.find((s) => s.key === 'jira') ?? sources[0]
  }

  /**
   * Mirror age in ms, or null when it has never synced. Reads the wall clock,
   * so it is a point-in-time answer, not something to hang a $derived on —
   * callers that render it drive their own tick.
   */
  get mirrorAgeMs(): number | null {
    const at = this.mirrorHealth?.synced_at
    if (!at) return null
    const ts = Date.parse(at)
    return Number.isNaN(ts) ? null : Date.now() - ts
  }

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
    applyCacheScopeDebug()

    // ① Hydration
    try {
      const [cached, meta] = await Promise.all([db.getAllIssues(), db.getMeta()])
      if (meta) {
        for (const m of meta.members) this.#setMember(m)
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
    this.#startActivityPolling()
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
    this.#clearMembers()
    for (const m of data.members) this.#setMember(m)
    invalidateAll()

    this.lastSync = data.server_time
    this.#syncVersion = data.sync_version
    this.#membersVersion = data.members_version ?? ''
    this.#etag = etag ?? `"in-${data.sync_version}"`
    this.syncHealth = data.sync_health
    this.fieldSpecs = data.field_specs ?? []
    this.fieldUsage = data.field_usage ?? {}
    this.latestVersion = data.latest_version ?? ''
    this.releaseUrl = data.release_url ?? ''
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
    for (const it of upserted) {
      this.pool.set(it.issue_key, it)
      invalidate(it.issue_key)
    }
    for (const key of deletedKeys) {
      this.pool.delete(key)
      invalidate(key)
    }
    // members arrive only when changed (omit → keep existing). Refresh hash when present.
    if (members) {
      this.#clearMembers()
      for (const member of members) this.#setMember(member)
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
      if (document.visibilityState !== 'visible') return
      void this.#sync()
      // Coming back to a tab that slept past the watch loop's cadence, a delta
      // only buys a fresh copy of an old mirror — pull the mirror too. Quiet:
      // the user did not ask for it, and the 15s poll covers a failure.
      const age = this.mirrorAgeMs
      if (age !== null && age > FOCUS_PULL_MAX_AGE_MS) void this.pullMirror('incremental', true)
    })
  }

  /**
   * Registered by lib/sync-now at import time. Injected rather than imported:
   * sync-now already imports this store to refresh the pool, and importing it
   * back would close a module cycle for one call.
   */
  #mirrorPuller: MirrorPuller | null = null

  setMirrorPuller(fn: MirrorPuller): void {
    this.#mirrorPuller = fn
  }

  /**
   * Called while a document pass is committing batches, and once when any pass
   * ends. Injected for the same reason as the puller: the page store must not
   * be imported here, and this store must not know what a page is.
   */
  #mirrorBatch: (() => void) | null = null

  setMirrorBatchHandler(fn: () => void): void {
    this.#mirrorBatch = fn
  }

  /**
   * Poll what the mirror is doing. Tight while it moves, slow while it does
   * not: a count that updates every two seconds is the difference between a
   * long sync and a hung one, and outside a sync there is nothing to watch.
   */
  #activityTimer: ReturnType<typeof setTimeout> | null = null

  #startActivityPolling(): void {
    if (this.#activityTimer || typeof window === 'undefined' || isHostedDemo()) return
    const tick = async () => {
      let busy = false
      if (document.visibilityState === 'visible') {
        try {
          const a = (await api.getSyncProgress()).activity
          if (a) {
            const ended = this.mirrorActivity.running && !a.running
            const startedMs = Date.parse(a.started_at)
            const old = Number.isFinite(startedMs)
              ? Date.now() - startedMs >= ACTIVITY_MIN_VISIBLE_MS
              : true
            this.mirrorActivity = {
              running: a.running && old,
              source: a.source ?? '',
              fetched: a.fetched ?? 0,
            }
            // Cadence follows the real pass, not the reported one: a young pass
            // has to be watched closely enough to catch it crossing over.
            busy = a.running
            // Pages are committed in batches, so they can be shown as they
            // arrive rather than all at once at the end — which is when
            // someone who just configured the source is watching hardest.
            if (a.source === 'documents' || ended) this.#mirrorBatch?.()
          }
        } catch {
          /* progress is advisory: a failure here must not surface anywhere */
        }
      }
      this.#activityTimer = setTimeout(tick, busy ? ACTIVITY_BUSY_MS : ACTIVITY_IDLE_MS)
    }
    this.#activityTimer = setTimeout(tick, 0)
  }

  /**
   * Pull the mirror itself from Jira, then let the pull's own refresh bring the
   * rows in. Single-flight so the chip, the palette and the focus handler
   * cannot stack runs.
   */
  async pullMirror(mode: 'full' | 'incremental' = 'incremental', quiet = false): Promise<void> {
    // Nothing to pull on the hosted demo — a snapshot service worker answers
    // every write with 501.
    if (this.mirrorSyncing || isHostedDemo() || !this.#mirrorPuller) return
    this.mirrorSyncing = true
    try {
      await this.#mirrorPuller(mode, quiet)
    } catch (e) {
      console.warn('[issues] 미러 동기화 실패', e)
    } finally {
      this.mirrorSyncing = false
    }
  }

  /** Manual refresh (tab focus / user trigger). */
  async refresh(): Promise<void> {
    await this.#sync()
  }

  /** Convenience lookup. */
  get(issueKey: string): IssueLite | undefined {
    return this.pool.get(issueKey)
  }

  #clearMembers(): void {
    this.members.clear()
    this.#membersByNormalizedEmail.clear()
    this.#membersByAccountId.clear()
  }

  #setMember(member: Member): void {
    const email = member.email?.trim() ?? ''
    const accountId = member.jira_account_id?.trim() ?? ''
    if (email) {
      this.members.set(email, member)
      this.#membersByNormalizedEmail.set(email.toLowerCase(), member)
    } else if (accountId) {
      this.members.set(accountId, member)
    }
    if (accountId) this.#membersByAccountId.set(accountId, member)
  }

  memberOf(email: string | null | undefined): Member | undefined {
    if (!email) return undefined
    return (
      this.members.get(email) ??
      this.#membersByNormalizedEmail.get(email.toLowerCase()) ??
      this.#membersByAccountId.get(email)
    )
  }

  memberOfAccountId(accountId: string | null | undefined): Member | undefined {
    if (!accountId) return undefined
    return this.#membersByAccountId.get(accountId) ?? this.members.get(accountId)
  }
}

/** App-wide singleton. Import anywhere: `import { issues } from '../stores/issues.svelte'`. */
export const issues = new IssuesStore()
