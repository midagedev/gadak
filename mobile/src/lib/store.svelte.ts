// App state (Svelte 5 runes). One store module: pairing lifecycle, the
// issue snapshot, and navigation. Screens mutate state only through the
// exported functions.

import { configureApi, request, isPairingDead, ApiError } from './api'
import { SCOPE_ME } from './domain'
import { tokenGet, tokenSet, tokenDel } from './secure'
import type {
  BootstrapResponse,
  IssueLite,
  Me,
  PairMeta,
  SavedViewDoc,
  SourceViewDoc,
  ViewsResponse,
} from './types'

const META_KEY = 'gadak.pairing.meta'
const UNPAIRED_KEY = 'gadak.pairing.unpaired'
const CACHE_KEY = 'gadak.snapshot'
const VIEWS_KEY = 'gadak.views'
const SCOPE_KEY = 'gadak.issues.scope'
const RECENTS_KEY = 'gadak.search.recents'

export type Tab = 'issues' | 'search' | 'pairing'
export type Phase = 'boot' | 'unpaired' | 'paired'

interface Snapshot {
  etag: string | null
  issues: IssueLite[]
  serverTime: string
}

interface ViewsCache {
  views: SavedViewDoc[]
  sources: SourceViewDoc[]
}

export const app = $state({
  phase: 'boot' as Phase,
  meta: null as PairMeta | null,
  /** Token verdict from the last sync: true → PairGate with explanation. */
  rejected: false,

  issues: [] as IssueLite[],
  me: null as Me | null,
  /** Views the developer saved at the desk (GET issues/views/ → `views`). */
  views: [] as SavedViewDoc[],
  /** Jira filters imported at the desk (same response → `source`). */
  sources: [] as SourceViewDoc[],
  /** Last scope the user picked; the heading is this scope's name. */
  scopeId: SCOPE_ME as string,
  /** True once any snapshot (cache or network) has painted. */
  loaded: false,
  syncing: false,
  offline: false,
  lastSyncAt: null as Date | null,

  tab: 'issues' as Tab,
  detailKey: null as string | null,
  /** Ticks every 30s so relative times stay honest while the app is open. */
  now: new Date(),
})

let etag: string | null = null
let syncTimer: ReturnType<typeof setInterval> | null = null

/* ── storage helpers (all guarded: storage can be unavailable or full) ── */

function readJSON<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(key)
    return raw ? (JSON.parse(raw) as T) : null
  } catch {
    return null
  }
}

function writeJSON(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Over-quota or unavailable: the cache is a convenience, never a requirement.
  }
}

function drop(key: string): void {
  try {
    localStorage.removeItem(key)
  } catch {
    /* ignore */
  }
}

/* ── boot ── */

export async function boot(): Promise<void> {
  const meta = readJSON<PairMeta>(META_KEY)
  const token = await tokenGet()
  if (meta && (token !== null || import.meta.env.DEV)) {
    // Dev sessions may hold a tokenless meta (proxy adoption below).
    await enterPaired(meta, token ?? '')
    return
  }
  if (meta && token === null) {
    // Meta without a token (fresh reinstall wiped the web storage's twin, or
    // the Keychain entry was removed outside the app): honest re-pair.
    drop(META_KEY)
    app.phase = 'unpaired'
    return
  }
  // No meta. In dev, adopt the shell's existing pairing once — unless the
  // user explicitly unpaired, which must stick.
  const unpaired = (() => {
    try {
      return localStorage.getItem(UNPAIRED_KEY) === '1'
    } catch {
      return false
    }
  })()
  if (import.meta.env.DEV && !unpaired) {
    // Dev trust boundary is the vite proxy itself: it only exists because
    // the developer pointed it at their own loopback serve, which decision
    // 0003 keeps unauthenticated. If the proxy answers, adopt it — this is
    // the "pairs instantly in dev" behavior the shell expects. A Keychain
    // token, when present, rides along for parity with packaged builds.
    try {
      await request<Me>('auth/me/', { session: { endpoint: '', token } })
      const adopted: PairMeta = { endpoint: '', label: 'This Mac (dev)', expires_at: '' }
      writeJSON(META_KEY, adopted)
      await enterPaired(adopted, token ?? '')
      return
    } catch {
      // Proxy dead or serve down — fall through to the gate.
    }
  }
  app.phase = 'unpaired'
}

async function enterPaired(meta: PairMeta, token: string): Promise<void> {
  app.meta = meta
  app.rejected = false
  configureApi({ endpoint: meta.endpoint, token })
  const cached = readJSON<Snapshot>(CACHE_KEY)
  if (cached) {
    app.issues = cached.issues
    etag = cached.etag
    app.loaded = true
  }
  // Views are cached too, so a restored scope paints its own name on the
  // first frame instead of flashing the default until sync answers.
  const cachedViews = readJSON<ViewsCache>(VIEWS_KEY)
  if (cachedViews) {
    app.views = cachedViews.views ?? []
    app.sources = cachedViews.sources ?? []
  }
  const scope = readJSON<string>(SCOPE_KEY)
  if (typeof scope === 'string' && scope !== '') app.scopeId = scope
  app.phase = 'paired'
  void sync()
  if (syncTimer) clearInterval(syncTimer)
  syncTimer = setInterval(() => void sync(), 60_000)
}

/* ── sync ── */

export async function sync(): Promise<void> {
  if (app.phase !== 'paired' || app.syncing) return
  app.syncing = true
  try {
    const res = await request<BootstrapResponse>('issues/bootstrap/', { etag })
    if (res.status !== 304 && res.body) {
      app.issues = res.body.issues
      etag = res.etag
      writeJSON(CACHE_KEY, {
        etag,
        issues: res.body.issues,
        serverTime: res.body.server_time,
      } satisfies Snapshot)
    }
    try {
      const me = await request<Me>('auth/me/')
      app.me = me.body
    } catch {
      // Identity is optional (standalone serves have none); the list falls back.
    }
    try {
      // The names the scope picker wears. Read-only: the phone consumes the
      // desk's view list and never POSTs one (DESIGN.md §5).
      const res = await request<ViewsResponse>('issues/views/')
      if (res.body) {
        app.views = res.body.views ?? []
        app.sources = res.body.source ?? []
        writeJSON(VIEWS_KEY, { views: app.views, sources: app.sources } satisfies ViewsCache)
      }
    } catch {
      // A serve that refuses the view list still lists issues under the two
      // hardcoded scopes — a picker with fewer names, not a dead screen.
    }
    app.loaded = true
    app.offline = false
    app.lastSyncAt = new Date()
  } catch (err) {
    if (isPairingDead(err)) {
      app.rejected = true
      app.phase = 'unpaired'
    } else {
      app.offline = true
    }
  } finally {
    app.syncing = false
  }
}

/* ── pairing lifecycle ── */

/**
 * Commits a decoded offer: probe the server first, store the token only on
 * success. Throws ApiError for the caller's error copy.
 */
export async function pair(offer: { endpoint: string; token: string; expires_at: string; label: string }): Promise<void> {
  // Probe with the offered credential before storing anything.
  await request<Me>('auth/me/', { session: { endpoint: offer.endpoint, token: offer.token } })
  await tokenSet(offer.token)
  const meta: PairMeta = { endpoint: offer.endpoint, label: offer.label, expires_at: offer.expires_at }
  writeJSON(META_KEY, meta)
  drop(UNPAIRED_KEY)
  await enterPaired(meta, offer.token)
}

export async function unpair(): Promise<void> {
  try {
    await tokenDel()
  } catch {
    // The meta is gone either way; a stale Keychain entry cannot re-pair by
    // itself because the unpaired flag blocks dev re-adoption.
  }
  drop(META_KEY)
  drop(CACHE_KEY)
  drop(VIEWS_KEY)
  drop(SCOPE_KEY)
  // Packaged: unpairing must stick across launches. Dev: the proxy is the
  // trust boundary, so the next launch may re-adopt it — the gate still
  // shows for this session, which is what unpair-state verification needs.
  if (!import.meta.env.DEV) {
    try {
      localStorage.setItem(UNPAIRED_KEY, '1')
    } catch {
      /* ignore */
    }
  }
  if (syncTimer) {
    clearInterval(syncTimer)
    syncTimer = null
  }
  app.meta = null
  app.issues = []
  app.me = null
  app.views = []
  app.sources = []
  app.scopeId = SCOPE_ME
  app.loaded = false
  app.rejected = false
  app.detailKey = null
  app.tab = 'issues'
  app.phase = 'unpaired'
}

/* ── navigation ── */

export function openIssue(key: string): void {
  app.detailKey = key
}

export function closeIssue(): void {
  app.detailKey = null
}

export function switchTab(tab: Tab): void {
  app.tab = tab
}

/** Picks a scope and remembers it — boot restores the last one used. */
export function setScope(id: string): void {
  app.scopeId = id
  writeJSON(SCOPE_KEY, id)
}

/* ── search recents ── */

export function recentSearches(): string[] {
  return readJSON<string[]>(RECENTS_KEY) ?? []
}

export function rememberSearch(q: string): void {
  const t = q.trim()
  if (t === '') return
  const list = [t, ...recentSearches().filter((x) => x !== t)].slice(0, 5)
  writeJSON(RECENTS_KEY, list)
}

/* ── clock ── */

export function startClock(): () => void {
  const id = setInterval(() => {
    app.now = new Date()
  }, 30_000)
  const onVisible = () => {
    if (document.visibilityState === 'visible') {
      app.now = new Date()
      void sync()
    }
  }
  document.addEventListener('visibilitychange', onVisible)
  return () => {
    clearInterval(id)
    document.removeEventListener('visibilitychange', onVisible)
  }
}

/** Re-export for screens that need to distinguish server refusals. */
export { ApiError }
