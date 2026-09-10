// App state (Svelte 5 runes). One store module: pairing lifecycle, the
// issue snapshot, and navigation. Screens mutate state only through the
// exported functions.

import { configureApi, request, isPairingDead, ApiError } from './api'
import { t } from './i18n'
import { VISIT_DEBOUNCE_MS } from '../../../web/src/lib/history'
import { setDemoSession } from './demo'
import {
  SCOPE_MY_WORK,
  feedAfterRead,
  migrateScopeId,
  relatchBoundary,
  sessionDelta,
  setFlow,
  type FeedResponse,
  type MarkFeedReadResponse,
  type SessionDelta,
} from './domain'
import {
  getActiveHostId,
  hasHostsDoc,
  hostIdForEndpoint,
  removeHost,
  setActiveHostId,
  touchHost,
  upsertHostFromPairing,
} from './hosts'
import { tokenGet, tokenSet, tokenDel } from './secure'
import {
  CACHE_KEY,
  HOST_SCOPED_KEYS,
  META_KEY,
  PAGES_KEY,
  SCOPE_KEY,
  TERM_META_KEY,
  VIEWS_KEY,
  hostKey,
  migrateHostKeys,
} from './host-keys'
import { probeShellPairing } from './terminal/api'
import { serveTokenOf, terminalTokenOf, OfferScopeError, type OfferToken } from './offer'
import type {
  BootstrapResponse,
  FlowSummary,
  IssueLite,
  Me,
  PageLite,
  PagesResponse,
  PairMeta,
  SavedViewDoc,
  SourceViewDoc,
  ViewsResponse,
} from './types'

// The six host-scoped session keys (META/TERM_META/CACHE/VIEWS/PAGES/SCOPE)
// are imported from host-keys.ts (GDK-1097 B2): the module that namespaces
// them owns their names. UNPAIRED_KEY is a device-wide verdict and
// RECENTS_KEY a device-wide history — neither belongs to a host.
const UNPAIRED_KEY = 'gadak.pairing.unpaired'
const RECENTS_KEY = 'gadak.search.recents'

export type Tab = 'issues' | 'search' | 'pairing' | 'shell'
export type Phase = 'boot' | 'unpaired' | 'paired'
export type DetailRef = { kind: 'issue'; key: string } | { kind: 'page'; key: string }

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
  /**
   * True while the bundled demo workspace is showing (GDK-1051): a
   * pairing-free, read-only session over /demo/. Never persisted — a
   * relaunch always starts at the gate — and every persistence write and
   * Keychain call below is gated on it being false.
   */
  demo: false,

  issues: [] as IssueLite[],
  me: null as Me | null,
  /** Views the developer saved at the desk (GET issues/views/ → `views`). */
  views: [] as SavedViewDoc[],
  /** Jira filters imported at the desk (same response → `source`). */
  sources: [] as SourceViewDoc[],
  /** Mirrored wiki pages (GET issues/pages/). Empty → no Documents section. */
  pages: [] as PageLite[],
  /**
   * Personal feed (GET issues/feed/), the glance strip's data (GDK-871).
   * Null = no cycle has answered yet, or unpaired — the strip is absent,
   * never an error chrome (optional surface, DESIGN.md §1).
   */
  feed: null as FeedResponse | null,
  /**
   * Last scope the user picked; the heading is this scope's name. The first
   * run wants My issues — the desk's first-run rule (GDK-1542). It is a
   * *want*, evaluated at module load before auth/me has answered, so the
   * identity condition lives in `resolveScope`/`defaultScopeId`: without an
   * identity the row is not offered and All open is what paints.
   */
  scopeId: SCOPE_MY_WORK as string,
  /**
   * True once the first bootstrap attempt has settled — cache restore,
   * successful sync, or a failed sync that is not pairing-dead. Distinct
   * from "a snapshot exists": that is `showOfflineBanner` / `issuesBootKind`.
   */
  loaded: false,
  syncing: false,
  offline: false,
  lastSyncAt: null as Date | null,

  tab: 'issues' as Tab,
  detail: null as DetailRef | null,
  /** Terminal pairing metadata — never the token. Null → no Shell tab. */
  terminal: null as PairMeta | null,
  /** Ticks every 30s so relative times stay honest while the app is open. */
  now: new Date(),

  /**
   * The learned stale threshold the serve sent with the last bootstrap
   * (GDK-1495 ②). Null → the row age falls back to the shared default. Read
   * through the seam registered below, never by screens.
   */
  flow: null as FlowSummary | null,

  /**
   * The session strip's latch (GDK-1495 ①). `boundary` is where the previous
   * session of person reads ended — the serve's on the first bootstrap, and
   * the moment the app went away on a re-latch. `delta` is a *snapshot*
   * frozen the moment the boundary and a pool first meet: a value that grew
   * with every later sync would make the strip an interruption instead of a
   * session-start reading. `computed` is what makes it a snapshot; the whole
   * latch is reset only by a real re-latch.
   */
  session: {
    boundary: null as string | null,
    delta: null as SessionDelta | null,
    computed: false,
    dismissed: false,
  },
})

/**
 * Issues list plate. `loaded` used to mean both "stop the skeleton" and
 * "a snapshot exists", so a failed first sync painted an infinite skeleton
 * under a banner that claimed cached data. One owner, three values.
 */
export type IssuesBootKind = 'skeleton' | 'failed' | 'ready'

export function showOfflineBanner(s: {
  offline: boolean
  issueCount: number
  pageCount: number
  lastSyncAt: Date | null
}): boolean {
  if (!s.offline) return false
  return s.issueCount > 0 || s.pageCount > 0 || s.lastSyncAt !== null
}

export function issuesBootKind(s: {
  loaded: boolean
  offline: boolean
  issueCount: number
  pageCount: number
  lastSyncAt: Date | null
}): IssuesBootKind {
  if (!s.loaded) return 'skeleton'
  if (s.offline && !showOfflineBanner(s)) return 'failed'
  return 'ready'
}

/* ── session strip latch (GDK-1495 ①) ── */

/**
 * Freezes the strip's answer, once per session. Called after every bootstrap
 * lands, because three things must be in hand at the same moment and they
 * arrive in no fixed order: the boundary (bootstrap), the pool (bootstrap),
 * and the identity (auth/me, which the mine count needs). The `computed`
 * guard is what makes the result a snapshot rather than a running total.
 */
function latchSession(): void {
  if (app.session.computed) return
  if (!app.session.boundary || app.issues.length === 0) return
  app.session.computed = true
  app.session.delta = sessionDelta(app.issues, app.session.boundary, app.me)
}

/**
 * Claims the session boundary, once (GDK-1537). Only when nothing has claimed
 * it yet: a re-latch is a *newer* session than anything the serve can know
 * about, so it must not be overwritten by the refresh the return itself
 * triggers — and the serve's own answer moves too, once this phone sits idle
 * past the session gap. `null` is not a claim (no previous session on record,
 * a demo bundle, or a serve older than 0.21).
 */
function claimSessionBoundary(at: string | null): void {
  if (!at || app.session.boundary) return
  app.session.boundary = at
}

/**
 * A new session begins (GDK-1495 ①): the app was away longer than the
 * session gap, so the previous session ended when it went away. Clears the
 * latch so the strip speaks once more — with the pool the return's own sync
 * is about to refresh, not the one from before the app was hidden.
 */
function relatchSession(hiddenAtMs: number | null): void {
  const since = relatchBoundary(hiddenAtMs, Date.now())
  if (!since) return
  app.session = { boundary: since, delta: null, computed: false, dismissed: false }
}

/** The strip is a reading, and reading it is done with (tap = dismiss). */
export function dismissSessionStrip(): void {
  app.session.dismissed = true
}

/**
 * Search body plate. Empty, in-flight, and failed must not be the same
 * picture (DESIGN.md §3.5 / §4.6 / §5). Hits always win so local-only
 * degradation stays visible.
 */
export type SearchPaint = 'idle' | 'searching' | 'failed' | 'empty' | 'results'

export function searchPaint(s: {
  query: string
  resultCount: number
  pageCount: number
  searching: boolean
  failed: boolean
}): SearchPaint {
  if (s.query.trim() === '') return 'idle'
  if (s.resultCount > 0 || s.pageCount > 0) return 'results'
  if (s.searching) return 'searching'
  if (s.failed) return 'failed'
  return 'empty'
}

let etag: string | null = null
let syncTimer: ReturnType<typeof setInterval> | null = null
/** Terminal Bearer. Lives next to the serve session token, not on `app`. */
let terminalToken: string | null = null

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

/**
 * The active host's address for a host-scoped key (GDK-1097 B2). No active
 * host → the bare key, which is what every pre-B2 phone's data lives at.
 */
function scopedKey(base: string): string {
  return hostKey(base, getActiveHostId())
}

/* ── boot ── */

/**
 * One-time legacy→roster migration (GDK-1097 B1). The no-unpair
 * invariant is structural, not rolled back: everything risky happens
 * BEFORE anything observable changes. The token is copied to its host
 * slot and read back; only a verified copy earns the roster write, and
 * only a verified roster (read back too — localStorage fails as quietly
 * as a Keychain can) earns retiring the legacy address. Any failure
 * leaves the phone exactly as it was — token at the legacy address, no
 * gadak.hosts.v1 — so the next boot retries and the boot read below
 * falls back to the legacy slot. No failure path unpairs.
 */
async function migrateLegacyPairing(): Promise<void> {
  if (hasHostsDoc()) return
  const meta = readJSON<PairMeta>(META_KEY)
  if (!meta) return
  let serveToken: string | null
  try {
    serveToken = await tokenGet('serve')
  } catch {
    return
  }
  if (serveToken === null) return
  try {
    const hostId = await hostIdForEndpoint(meta.endpoint)
    await tokenSet(serveToken, 'serve', hostId)
    if ((await tokenGet('serve', hostId)) !== serveToken) return
    // The terminal token is keyed by its own meta's endpoint — the same
    // id loadTerminal() derives — which is the serve's endpoint in the
    // usual one-host case.
    const termMeta = readJSON<PairMeta>(TERM_META_KEY)
    const termToken = await tokenGet('terminal')
    if (termToken !== null) {
      const termHostId = await hostIdForEndpoint(termMeta?.endpoint ?? meta.endpoint)
      await tokenSet(termToken, 'terminal', termHostId)
      if ((await tokenGet('terminal', termHostId)) !== termToken) return
    }
    await upsertHostFromPairing({ endpoint: meta.endpoint, label: meta.label })
    setActiveHostId(hostId)
    // Read-back gate #2: the roster doc and the active pointer must be
    // observable before the legacy address may die — a silently failed
    // write here must not strand boot with no token address at all.
    if (!hasHostsDoc() || getActiveHostId() !== hostId) return
    await tokenDel('serve')
    if (termToken !== null) await tokenDel('terminal')
  } catch {
    // Partial copies may remain in host slots (harmless duplicates of a
    // value that still lives at the legacy address) — never a partial unpair.
    return
  }
}

export async function boot(): Promise<void> {
  await migrateLegacyPairing()
  const activeHost = getActiveHostId()
  // B2: with a roster host active, the six session documents live at its
  // namespaced addresses. The rename is per-key verified and idempotent; a
  // null host (unmigrated phone) keeps the bare keys untouched.
  migrateHostKeys(activeHost)
  // Read fallback: a phone whose META rename could not be verified still
  // holds its pairing at the legacy address — missing both must not read
  // as "never paired".
  const meta =
    readJSON<PairMeta>(hostKey(META_KEY, activeHost)) ??
    (activeHost !== null ? readJSON<PairMeta>(META_KEY) : null)
  // Active host → its slot; no roster yet (migration refused) → the
  // legacy slot, which is where an unmigrated phone's token still lives.
  const token = await tokenGet('serve', activeHost ?? undefined)
  if (meta && (token !== null || import.meta.env.DEV)) {
    if (activeHost !== null && token !== null) touchHost(activeHost)
    // Dev sessions may hold a tokenless meta (proxy adoption below).
    await enterPaired(meta, token ?? '')
    return
  }
  if (meta && token === null) {
    // Meta without a token (fresh reinstall wiped the web storage's twin, or
    // the Keychain entry was removed outside the app): honest re-pair.
    drop(hostKey(META_KEY, activeHost))
    if (activeHost !== null) drop(META_KEY)
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
      // The roster learns the dev proxy too (id 'local'). No token copy:
      // dev reads fall back to the shell's legacy Keychain peek by design
      // (secure.ts dev rule), and the pointer only claims an unset slot.
      // The meta lands in the host's namespace (usually @local) once the
      // pointer can answer for it.
      const host = await upsertHostFromPairing({ endpoint: '', label: adopted.label })
      if (getActiveHostId() === null) setActiveHostId(host.id)
      writeJSON(hostKey(META_KEY, host.id), adopted)
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
  const cached = readJSON<Snapshot>(scopedKey(CACHE_KEY))
  if (cached) {
    app.issues = cached.issues
    etag = cached.etag
    app.loaded = true
  }
  // Views are cached too, so a restored scope paints its own name on the
  // first frame instead of flashing the default until sync answers.
  const cachedViews = readJSON<ViewsCache>(scopedKey(VIEWS_KEY))
  if (cachedViews) {
    app.views = cachedViews.views ?? []
    app.sources = cachedViews.sources ?? []
  }
  const cachedPages = readJSON<{ pages: PageLite[] }>(scopedKey(PAGES_KEY))
  if (cachedPages) {
    app.pages = cachedPages.pages ?? []
  }
  const scope = readJSON<string>(scopedKey(SCOPE_KEY))
  // A scope id an older build wrote may name a row that no longer exists
  // (GDK-1542): domain.ts owns that mapping, not this reader.
  if (typeof scope === 'string' && scope !== '') app.scopeId = migrateScopeId(scope)
  app.phase = 'paired'
  await loadTerminal()
  void sync()
  if (syncTimer) clearInterval(syncTimer)
  syncTimer = setInterval(() => void sync(), 60_000)
}

/* ── sync ── */

export async function sync(): Promise<void> {
  // The demo snapshot is frozen bytes: nothing to re-sync, and the real
  // sync path below must not run — it would overwrite CACHE/VIEWS/PAGES
  // with demo rows (the contamination contract demo.test.ts asserts).
  if (app.demo) return
  if (app.phase !== 'paired' || app.syncing) return
  app.syncing = true
  try {
    const res = await request<BootstrapResponse>('issues/bootstrap/', { etag })
    // Outside the 304 guard on purpose (GDK-1537): the boundary rides a
    // response header precisely so a cold start against an unchanged mirror
    // — 304, no body — still learns where the previous session ended. The
    // claim is once-only for the same reason the body's was: a re-latch is a
    // newer session than any the serve can know about.
    claimSessionBoundary(res.sessionBoundary ?? null)
    if (res.status !== 304 && res.body) {
      app.issues = res.body.issues
      // The learned threshold the age bands read (GDK-1495 ②). Absent means
      // "nothing to learn from" — the desktop's own default takes over.
      app.flow = res.body.flow ?? null
      setFlow(app.flow)
      // The body field is the fallback for a serve older than 0.21, which
      // sends no header. Kept for one release; 0.22 may drop it.
      claimSessionBoundary(res.body.last_session_ended_at ?? null)
      etag = res.etag
      writeJSON(scopedKey(CACHE_KEY), {
        etag,
        issues: res.body.issues,
        serverTime: res.body.server_time,
      } satisfies Snapshot)
    }
    try {
      const me = await request<Me>('auth/me/')
      app.me = me.body
    } catch {
      // Identity is optional (builtIn serves have none); the list falls back.
    }
    try {
      // The names the scope picker wears. Read-only: the phone consumes the
      // desk's view list and never POSTs one (DESIGN.md §5).
      const res = await request<ViewsResponse>('issues/views/')
      if (res.body) {
        app.views = res.body.views ?? []
        app.sources = res.body.source ?? []
        writeJSON(scopedKey(VIEWS_KEY), { views: app.views, sources: app.sources } satisfies ViewsCache)
      }
    } catch {
      // A serve that refuses the view list still lists issues under the two
      // hardcoded scopes — a picker with fewer names, not a dead screen.
    }
    try {
      // Same cache shape as views, no If-None-Match: the desktop's getPages()
      // does not send one either (web/src/lib/api.ts). An empty list or a
      // refusal hides the Documents section — no error chrome.
      const res = await request<PagesResponse>('issues/pages/')
      if (res.body) {
        app.pages = res.body.pages ?? []
        writeJSON(scopedKey(PAGES_KEY), { pages: app.pages })
      }
    } catch {
      // Keep whatever we already painted (offline). No pages → no section.
    }
    try {
      // The glance strip's data (GDK-871): what moved while the phone was
      // away. Rides sync()'s cycle — no timer of its own. A failure costs
      // the strip, never the boot; the next cycle retries.
      const res = await request<FeedResponse>('issues/feed/?focus=all&limit=20')
      if (res.body) app.feed = res.body
    } catch {
      // Keep whatever was painted; no feed answered → no strip this cycle.
    }
    // After identity, because the mine count needs it.
    latchSession()
    app.loaded = true
    app.offline = false
    app.lastSyncAt = new Date()
  } catch (err) {
    if (isPairingDead(err)) {
      app.rejected = true
      app.phase = 'unpaired'
    } else {
      app.offline = true
      // Settled: Issues must leave the skeleton. Do not write the snapshot —
      // a failed request never mutates cached issues (DESIGN.md §5).
      app.loaded = true
    }
  } finally {
    app.syncing = false
  }
}

/* ── glance strip read receipts (GDK-871) ── */

/**
 * Marks one issue's feed events read (POST issues/feed/read/). A read
 * receipt lives in feed_reads beside the mirror, not in origin — no
 * credential is needed, so a demo serve answers it too. Local state moves
 * only after the reply lands: nothing is removed optimistically, so a
 * refused receipt cannot eat a row. Throws ApiError for the caller's copy.
 */
export async function markGlanceIssueRead(key: string): Promise<void> {
  const res = await request<MarkFeedReadResponse>('issues/feed/read/', {
    method: 'POST',
    body: { issue_keys: [key] },
  })
  if (app.feed) app.feed = feedAfterRead(app.feed, [key], res.body?.unread_counts)
}

/** Marks everything read. Same receipt, same no-optimistic contract. */
export async function markGlanceAllRead(): Promise<void> {
  const res = await request<MarkFeedReadResponse>('issues/feed/read/', {
    method: 'POST',
    body: { all: true },
  })
  if (app.feed) app.feed = feedAfterRead(app.feed, null, res.body?.unread_counts)
}

/* ── pairing lifecycle ── */

/**
 * Commits a decoded offer: probe the server first, store the token only on
 * success. Throws ApiError for the caller's error copy.
 *
 * A v2 offer (GDK-1498) carries one token per scope from one mint, and one
 * scan pairs the whole device: the serve entry is this session's
 * credential, and a terminal entry lands in the terminal slot (the same
 * two writes pairTerminal would make) so the shell is armed without the
 * second scan. The terminal token is stored as-minted, no second probe —
 * the shell gate judges it on connect, and the offer already proved the
 * serve side. A v1 offer has one token whose scope the payload cannot
 * name; it goes to the serve slot exactly as before.
 */
export async function pair(offer: {
  endpoint: string
  token: string
  expires_at: string
  label: string
  tokens?: OfferToken[]
}): Promise<void> {
  const serveToken = serveTokenOf(offer)
  if (serveToken === null) {
    // Refuse before anything is written. A terminal-only offer is a real
    // thing (the desktop's second-step flow prints those) but it opens a
    // shell, not the mirror — this is the front door's field. The sentences
    // are catalog keys (GDK-1150); the catch sites render err.message.
    if (terminalTokenOf(offer) !== null) {
      throw new OfferScopeError(t('app.offerTerminalOnly'))
    }
    throw new OfferScopeError(t('app.offerNoMirrorToken'))
  }
  // Probe with the offered credential before storing anything.
  await request<Me>('auth/me/', { session: { endpoint: offer.endpoint, token: serveToken } })
  // Roster commit (GDK-1097 B1): host row, then the token in that host's
  // slot, then the active pointer — the pointer never leads the token,
  // so "active is set" always implies "its slot holds the token".
  const host = await upsertHostFromPairing({ endpoint: offer.endpoint, label: offer.label })
  await tokenSet(serveToken, 'serve', host.id)
  setActiveHostId(host.id)
  const meta: PairMeta = { endpoint: offer.endpoint, label: offer.label, expires_at: offer.expires_at }
  writeJSON(hostKey(META_KEY, host.id), meta)
  drop(UNPAIRED_KEY)
  // The terminal half of a v2 offer, if it carries one: same slot and
  // meta pairTerminal writes, so loadTerminal() arms the Shell tab on the
  // next boot — and PairingTab's second-step mint hint disappears because
  // app.terminal is set, not by a special case.
  const termToken = terminalTokenOf(offer)
  if (termToken !== null) {
    await tokenSet(termToken, 'terminal', host.id)
    writeJSON(scopedKey(TERM_META_KEY), meta)
    terminalToken = termToken
    app.terminal = meta
  }
  await enterPaired(meta, serveToken)
}

export async function unpair(): Promise<void> {
  const active = getActiveHostId()
  try {
    await tokenDel('serve', active ?? undefined)
  } catch {
    // The meta is gone either way; a stale Keychain entry cannot re-pair by
    // itself because the unpaired flag blocks dev re-adoption.
  }
  if (active !== null) {
    // Pre-migration phones keep their token at the legacy address —
    // forgetting the server forgets every address the token ever lived at.
    try {
      await tokenDel('serve')
    } catch {
      /* same stance as above */
    }
  }
  // The shell is a second token issued by the same serve, so forgetting the
  // server means forgetting both. Leaving it behind kept a live Bearer in the
  // Keychain and let loadTerminal() revive a Shell tab pointed at a server the
  // user had already unpaired from.
  await unpairTerminal()
  if (active !== null) removeHost(active) // also drops the active pointer
  setActiveHostId(null)
  // B2: forget the session documents at every address this host's data
  // lived at — the namespaced six, plus their bare-key residue from before
  // the rename (the same dual-address stance as the token deletes above).
  for (const base of HOST_SCOPED_KEYS) {
    drop(hostKey(base, active))
    if (active !== null) drop(base)
  }
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
  resetSessionState()
  app.phase = 'unpaired'
}

/* ── host switching (GDK-1097 B2) ── */

/**
 * Points the app at another roster host. Reads the target's meta and serve
 * token first; either missing → false with nothing moved (the roster row's
 * re-pair hint is the caller's copy — the phone cannot mint a token for a
 * host whose serve it cannot ask). A verified switch clears the sync timer
 * and the in-memory session, claims the pointer, and re-enters paired, so
 * caches, scope and terminal all reload from the new host's keys — the
 * previous host's documents stay byte-identical at their addresses.
 */
export async function switchHost(id: string): Promise<boolean> {
  const meta = readJSON<PairMeta>(hostKey(META_KEY, id))
  let token: string | null = null
  try {
    token = await tokenGet('serve', id)
  } catch {
    // A Keychain refusal counts as "no token here" for switching purposes.
  }
  if (!meta || token === null) return false
  if (syncTimer) {
    clearInterval(syncTimer)
    syncTimer = null
  }
  resetSessionState()
  terminalToken = null // loadTerminal() re-reads under the new namespace
  setActiveHostId(id)
  touchHost(id)
  await enterPaired(meta, token)
  return true
}

/**
 * Forgets an inactive roster row: its two token slots, its row, and its six
 * namespaced documents. The active host is unpair()'s case — delegate. An
 * inactive host's data only ever lived at its namespaced addresses (the
 * bare keys belong to the legacy/active phone), so no bare-key drops here.
 */
export async function removeRosterHost(id: string): Promise<void> {
  if (id === getActiveHostId()) {
    await unpair()
    return
  }
  try {
    await tokenDel('serve', id)
  } catch {
    /* the row is gone either way */
  }
  try {
    await tokenDel('terminal', id)
  } catch {
    /* same stance */
  }
  removeHost(id)
  for (const base of HOST_SCOPED_KEYS) drop(hostKey(base, id))
}

/* ── bundled demo workspace (GDK-1051) ── */

/** The in-memory reset unpair() performs — the shared leaving-list. */
function resetSessionState(): void {
  app.meta = null
  app.issues = []
  app.me = null
  app.views = []
  app.sources = []
  app.pages = []
  app.feed = null
  app.scopeId = SCOPE_MY_WORK
  app.loaded = false
  app.offline = false
  app.lastSyncAt = null
  app.rejected = false
  app.detail = null
  app.tab = 'issues'
  app.terminal = null
  // The boundary and the threshold belong to the host being left, not to the
  // next one (GDK-1495): a strip latched on one workspace must not speak on
  // another's pool.
  app.flow = null
  setFlow(null)
  app.session = { boundary: null, delta: null, computed: false, dismissed: false }
  etag = null
}

/**
 * Enters the bundled demo workspace from PairGate. Sets no meta, starts no
 * sync timer, touches no persistence and no Keychain slot — the demo is a
 * RAM-only session over the app's own /demo/ bundle (transport: demo.ts).
 * Failure to load the bundle settles honestly as the failed plate, the
 * same settlement a failed first sync gets.
 */
export async function enterDemo(): Promise<void> {
  setDemoSession(true)
  app.demo = true
  app.phase = 'paired'
  resetSessionState()
  if (syncTimer) {
    clearInterval(syncTimer)
    syncTimer = null
  }
  try {
    const res = await request<BootstrapResponse>('issues/bootstrap/')
    if (res.body) {
      app.issues = res.body.issues
      app.loaded = true
      app.lastSyncAt = new Date()
    } else {
      app.loaded = true
      app.offline = true
    }
  } catch {
    app.loaded = true
    app.offline = true
  }
}

/**
 * Exits the demo back to the gate. Same in-memory reset as unpair() but
 * deliberately narrower: no UNPAIRED_KEY write (a later real pairing must
 * not be blocked by demo debris — and in dev the proxy may re-adopt), no
 * token deletion (the demo never stored one), no localStorage drops (a
 * previously cached real snapshot survives untouched).
 */
export function exitDemo(): void {
  setDemoSession(false)
  app.demo = false
  resetSessionState()
  app.phase = 'unpaired'
}

/* ── terminal pairing (own token, own meta — DESIGN.md §10.1) ── */

/** The dev arm's own state, read once so the branches below read plainly. */
function devShellArmed(): boolean {
  return import.meta.env.DEV && import.meta.env.VITE_DEV_SHELL === '1'
}

async function loadTerminal(): Promise<void> {
  // The terminal meta lives in the active host's namespace (B2); the legacy
  // read covers a phone whose rename could not be verified.
  const meta = devShellArmed()
    ? null
    : (readJSON<PairMeta>(scopedKey(TERM_META_KEY)) ?? readJSON<PairMeta>(TERM_META_KEY))
  // The terminal token is keyed by the terminal pairing's own endpoint
  // (pairTerminal's rule) — the shell may point at a different host than
  // the serve session. Legacy fallback covers a phone whose migration
  // did not complete.
  let token: string | null = null
  if (meta) {
    const hostId = await hostIdForEndpoint(meta.endpoint)
    token = (await tokenGet('terminal', hostId)) ?? (await tokenGet('terminal'))
  }
  if (token && meta) {
    terminalToken = token
    app.terminal = meta
    return
  }
  terminalToken = null
  app.terminal = null
  // `VITE_DEV_SHELL=1 npm run dev`: adopt the proxy for the shell too, the
  // way boot() adopts it for the serve session. Same trust boundary (the
  // developer pointed the vite proxy at their own loopback serve, decision
  // 0003), and the server side already agrees: changeOrigin makes the Host
  // a loopback IP literal and the peer is loopback, so terminalGate's local
  // rule admits the request with no Bearer at all (internal/server/
  // terminal.go terminalLocal). Without it the Shell tab is unreachable in
  // dev without a QR dance — which is why the capture tour's own header
  // listed "a stored terminal pairing" as an environment precondition the
  // operator had to arrange by hand.
  //
  // Opt-in, unlike the serve session's adoption, because "no terminal
  // pairing" is a state the app must still be able to show and dev is where
  // it is shown: adopting on sight made the three-tab default unreachable
  // and the Pairing tab's offer field with it (mobile/e2e/shell.spec.ts
  // caught exactly that — six failures, and it was right).
  //
  // The probe is what decides even when armed: no serve, or a gate that
  // wants a token, leaves the tab absent. Unpairing is session-scoped, as
  // it already is for the serve session (unpair() writes its sticky flag
  // only outside DEV, above) — an armed relaunch re-adopts.
  // Armed, the arm wins over whatever this device happens to remember: a
  // rig has to put the app in a known state, and a simulator that scanned
  // an offer months ago would otherwise keep pointing the pane — and its
  // on-screen heading — at that old pairing. The label is the arm's too,
  // so a recording can name the machine the take is about
  // (VITE_DEV_SHELL_LABEL) instead of showing pairing debris.
  if (!devShellArmed()) return
  try {
    await probeShellPairing('', '')
    const label = import.meta.env.VITE_DEV_SHELL_LABEL || 'This Mac (dev)'
    app.terminal = { endpoint: '', label, expires_at: '' }
  } catch {
    /* proxy dead, serve down, or the gate wants a token — no Shell tab */
  }
}

/**
 * Probe the scanned offer against the terminal gate, then store. A serve
 * token is 403 scope_rejected here and must not land in the Keychain.
 * A v2 offer is read by its terminal entry (GDK-1498); a v1 offer's
 * single token is used as-is, exactly as before.
 */
export async function pairTerminal(offer: {
  endpoint: string
  token: string
  expires_at: string
  label: string
  tokens?: OfferToken[]
}): Promise<void> {
  const termToken = terminalTokenOf(offer) ?? offer.token
  await probeShellPairing(offer.endpoint, termToken)
  // Roster commit: host row, then the terminal token in that host's slot.
  // The active pointer belongs to the serve session (boot reads the serve
  // token from the active host's slot) — it is claimed only when nothing
  // else is active.
  const host = await upsertHostFromPairing({ endpoint: offer.endpoint, label: offer.label })
  await tokenSet(termToken, 'terminal', host.id)
  if (getActiveHostId() === null) setActiveHostId(host.id)
  const meta: PairMeta = {
    endpoint: offer.endpoint,
    label: offer.label,
    expires_at: offer.expires_at,
  }
  // Written under the namespace the pointer answers for — the active host's,
  // which is this host when the claim above fired (B2).
  writeJSON(scopedKey(TERM_META_KEY), meta)
  terminalToken = termToken
  app.terminal = meta
}

export async function unpairTerminal(): Promise<void> {
  // The terminal token's slot is derived from the terminal meta's own
  // endpoint (loadTerminal's rule) — read before the meta is dropped.
  const termMeta =
    readJSON<PairMeta>(scopedKey(TERM_META_KEY)) ?? readJSON<PairMeta>(TERM_META_KEY)
  const slotHost = termMeta ? await hostIdForEndpoint(termMeta.endpoint) : getActiveHostId() ?? undefined
  try {
    await tokenDel('terminal', slotHost)
  } catch {
    /* meta is gone either way */
  }
  if (slotHost !== undefined) {
    // Legacy residue (pre-migration phones).
    try {
      await tokenDel('terminal')
    } catch {
      /* same stance */
    }
  }
  // Both addresses, like the serve side: the active namespace and the bare
  // key a pre-B2 phone may still hold.
  drop(scopedKey(TERM_META_KEY))
  if (getActiveHostId() !== null) drop(TERM_META_KEY)
  terminalToken = null
  app.terminal = null
  if (app.tab === 'shell') app.tab = 'issues'
}

/** Session the shell REST calls use — terminal token, never the serve one. */
export function terminalSession(): { endpoint: string; token: string | null } {
  return { endpoint: app.terminal?.endpoint ?? '', token: terminalToken }
}

/* ── navigation ── */

/*
 * One owner for "the phone looked at this" (GDK-1538). Before this the
 * phone read details and posted nothing, so `last_visited_at` /
 * `previous_visit_at` on the detail response — which the serve computes
 * from recorded visits — stayed empty on a workspace nobody opens at a
 * desk, and the resume card (Detail.svelte ③) was permanently silent.
 *
 * The route is the desk's own (POST /api/v1/issues/history/visits/,
 * internal/server/server.go:234 — every serve route sits under the
 * `issues/` base, which the phone's API_V1 does not include, so the path
 * spelled here carries it) and the *server* decides the source: the POST body has no
 * field for it, so the phone is recorded as `ui` like every other client
 * and retro aggregation keeps one meaning per row. That is why no server
 * change rides with this — a phone-specific `source` would be a wire
 * contract change for a distinction nothing reads yet.
 *
 * Fire and forget: a visit is a side effect of opening, never a gate on
 * it. Offline, refused, or demo, the screen opens exactly the same.
 * The debounce window is the desk's constant, imported rather than
 * re-picked (web/src/lib/history.ts VISIT_DEBOUNCE_MS) — a remount inside
 * it is the same read; A → B → A is two visits of A.
 */
let lastVisitId: string | null = null
let lastVisitAt = 0

export function recordVisit(kind: 'issue' | 'page', key: string, now = Date.now()): void {
  if (key === '' || app.demo) return
  const id = `${kind}:${key}`
  if (lastVisitId === id && now - lastVisitAt < VISIT_DEBOUNCE_MS) return
  lastVisitId = id
  lastVisitAt = now
  void request('issues/history/visits/', {
    method: 'POST',
    body: { kind, key },
  }).catch(() => {
    // A visit that did not land is not a failure the reader must see: the
    // only thing it costs is one resume boundary. Never a toast.
  })
}

/** Test seam: forget the debounce so a spec can post the same key twice. */
export function resetVisitDebounce(): void {
  lastVisitId = null
  lastVisitAt = 0
}

export function openIssue(key: string): void {
  app.detail = { kind: 'issue', key }
  recordVisit('issue', key)
}

export function openPage(key: string): void {
  app.detail = { kind: 'page', key }
  recordVisit('page', key)
}

export function closeIssue(): void {
  app.detail = null
}

export function switchTab(tab: Tab): void {
  app.tab = tab
}

/** Picks a scope and remembers it — boot restores the last one used. Demo skips the write. */
export function setScope(id: string): void {
  app.scopeId = id
  if (!app.demo) writeJSON(scopedKey(SCOPE_KEY), id)
}

/* ── search recents ── */

export function recentSearches(): string[] {
  return readJSON<string[]>(RECENTS_KEY) ?? []
}

export function rememberSearch(q: string): void {
  const t = q.trim()
  if (t === '') return
  // Demo session: no recents row — the list would otherwise leak a demo
  // query into the paired session's history (Search only reaches this on a
  // server reply, which the demo transport never gives, but the guard
  // holds for any future caller).
  if (app.demo) return
  const list = [t, ...recentSearches().filter((x) => x !== t)].slice(0, 5)
  writeJSON(RECENTS_KEY, list)
}

/* ── clock ── */

export function startClock(): () => void {
  const id = setInterval(() => {
    app.now = new Date()
  }, 30_000)
  // The app's away-time, owned here because this is already the one place
  // that hears the app leave and come back (GDK-1495 ①).
  let hiddenAt: number | null = null
  const onVisible = () => {
    if (document.visibilityState === 'hidden') {
      hiddenAt = Date.now()
      return
    }
    if (document.visibilityState === 'visible') {
      relatchSession(hiddenAt)
      hiddenAt = null
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
