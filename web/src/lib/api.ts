/*
 * Issue Navigator — REST client (contract §1)
 *
 * base comes from runtime config(); dev proxies via Vite to the local server.
 * No session token: identity is the Jira credential stored by the server.
 * Reads work without a credential on loopback; writes need one configured.
 */

import { config } from './config'
import type { GadakFeatures } from './config'
import type {
  AttachmentUploadResponse,
  BootstrapResponse,
  CommentMention,
  CommentsByAuthorResponse,
  CommentWriteResponse,
  CreateIssuePayload,
  CreateFieldsResponse,
  CreateMetaResponse,
  DeltaResponse,
  DetailResponse,
  EditMetaResponse,
  FeedFocus,
  FeedResponse,
  FeedUnreadCounts,
  IssueWriteResponse,
  JiraCredential,
  PrioritiesResponse,
  PageDetail,
  PagesResponse,
  SavedView,
  HistoryPage,
  HistoryVisitKind,
  SearchEvent,
  SearchResponse,
  TransitionsResponse,
  VisitEvent,
  UsersResponse,
  ViewsResponse,
  WatchesResponse,
  WriteMeta,
  AdfNode,
} from './types'

// Runtime config (built artifacts stay tenant-neutral).

/**
 * Lets callers distinguish credential-missing (409) and similar failures.
 * `code` is the server body `error` string (e.g. 'credential_required');
 * `jiraErrors` carries raw Jira field errors when present.
 */
export class ApiError extends Error {
  status: number
  code: string | null
  jiraErrors: Record<string, unknown> | null
  constructor(
    status: number,
    message: string,
    code: string | null = null,
    jiraErrors: Record<string, unknown> | null = null,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.jiraErrors = jiraErrors
  }
}

/** 409 standalone_data_present from PUT onboarding/connect/. */
export class LocalOriginDataPresentError extends ApiError {
  issues: number
  persist: string
  constructor(status: number, issues: number, persist: string) {
    super(status, 'standalone_data_present', 'standalone_data_present')
    this.name = 'LocalOriginDataPresentError'
    this.issues = issues
    this.persist = persist
  }
}

/**
 * GDK-477: a network throw (server gone) is the down signal. A caller abort
 * is a client timeout — Jira being slow is not "gadak serve is unreachable".
 *
 * GDK-1054: a RESPONSE with status >= 500 is the same signal. The GDK-1025
 * audit measured a fully-503 server rendering as total silence — the server
 * answered, but nothing marked "the server is failing". The single owner is
 * lib/reachability (its state publishes on <html data-server-reachability>,
 * so one DOM inspection names it on the next silent-failure hunt). 4xx stays
 * per-surface: those are answers, not outages. Recovery is unchanged and
 * pool-owned — a successful sync marks up (stores/issues #sync), the signal
 * that always took the strip down.
 */
let onNetworkDown: (() => void) | null = null

export function setNetworkDownHandler(fn: () => void): void {
  onNetworkDown = fn
}

function isAbortError(err: unknown): boolean {
  // AbortSignal.timeout() rejects as TimeoutError; caller abort() as AbortError.
  // Neither is "gadak serve is gone".
  const name = err instanceof Error ? err.name : ''
  return name === 'AbortError' || name === 'TimeoutError'
}

function noteNetworkFailure(err: unknown): void {
  if (isAbortError(err)) return
  onNetworkDown?.()
}

/** An answered 5xx joins the network throw as a down signal (GDK-1054). */
function noteServerFailure(status: number): void {
  if (status >= 500) onNetworkDown?.()
}

/** Shared fetch — path is relative to the API base. */
async function raw(path: string, init?: RequestInit): Promise<Response> {
  try {
    const res = await fetch(config().apiBase + path, {
      credentials: 'same-origin',
      ...init,
    })
    noteServerFailure(res.status)
    return res
  } catch (err) {
    noteNetworkFailure(err)
    throw err
  }
}

/** Parse a JSON response; throw ApiError on 4xx/5xx. */
async function json<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await raw(path, init)
  if (!res.ok) {
    throw new ApiError(res.status, `${init?.method ?? 'GET'} ${path} → ${res.status}`)
  }
  return (await res.json()) as T
}

/* ── bootstrap (ETag / 304) ── */

export type BootstrapResult =
  | { status: 'ok'; data: BootstrapResponse; etag: string | null }
  | { status: 'not_modified' }

/**
 * Load all issues + members. Pass prior `etag` (If-None-Match) to get `not_modified` on 304.
 * ETag shape is `"in-<sync_version>"` (server). Echo it on the next call.
 */
export async function getBootstrap(etag?: string | null): Promise<BootstrapResult> {
  const headers = new Headers()
  if (etag) headers.set('If-None-Match', etag)
  const res = await raw('bootstrap/', { headers })
  if (res.status === 304) return { status: 'not_modified' }
  if (!res.ok) throw new ApiError(res.status, `GET bootstrap/ → ${res.status}`)
  return {
    status: 'ok',
    data: (await res.json()) as BootstrapResponse,
    etag: res.headers.get('ETag'),
  }
}

/* ── delta ── */

/**
 * Changes since `since`. Pass prior `membersVersion` so the server can omit members
 * when the hash matches (payload diet). Without it, server always includes members.
 */
export function getDelta(since: string, membersVersion?: string): Promise<DeltaResponse> {
  let path = `delta/?since=${encodeURIComponent(since)}`
  if (membersVersion) path += `&mv=${encodeURIComponent(membersVersion)}`
  return json<DeltaResponse>(path)
}

/* ── detail (on demand) ── */

export function getDetail(issueKey: string): Promise<DetailResponse> {
  return json<DetailResponse>(`${encodeURIComponent(issueKey)}/detail/`)
}

/* ── Full-text search (server) ── */

export function search(q: string, limit = 200): Promise<SearchResponse> {
  return json<SearchResponse>(`search/?q=${encodeURIComponent(q)}&limit=${limit}`)
}

/* ── Personal history (local.db; POST is a side effect, never blocks the UI) ── */

export function postVisit(kind: HistoryVisitKind, key: string): Promise<VisitEvent> {
  return json<VisitEvent>('history/visits/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ kind, key }),
  })
}

export function postSearch(body: {
  query: string
  result_count: number
  opened_kind?: string
  opened_key?: string
}): Promise<SearchEvent> {
  return json<SearchEvent>('history/searches/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export function patchSearch(
  id: number,
  openedKind: HistoryVisitKind,
  openedKey: string,
): Promise<SearchEvent> {
  return json<SearchEvent>(`history/searches/${id}/`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ opened_kind: openedKind, opened_key: openedKey }),
  })
}

/** One row per key at its newest visit — the set a list marks opened rows
 *  from (GDK-1344). Older servers and the hosted demo answer 404 → empty. */
export async function getVisited(
  kind: HistoryVisitKind,
): Promise<{ key: string; viewed_at: string }[]> {
  try {
    const doc = await json<{ items?: { key: string; viewed_at: string }[] }>(
      `history/visited/?kind=${kind}`,
    )
    return doc.items ?? []
  } catch {
    return []
  }
}

export function getHistory(opts?: {
  kind?: string
  limit?: number
  cursor?: string
}): Promise<HistoryPage> {
  const q = new URLSearchParams()
  if (opts?.kind) q.set('kind', opts.kind)
  if (opts?.limit != null) q.set('limit', String(opts.limit))
  if (opts?.cursor) q.set('cursor', opts.cursor)
  const qs = q.toString()
  return json<HistoryPage>(qs ? `history/?${qs}` : 'history/')
}

/** JQL / Jira-URL → ViewFilters. Unsupported clauses are listed, never dropped. */
export interface JqlParseResult {
  input?: string
  jql: string
  filters: import('./view-config').ViewFilters
  display: { sort?: string; dir?: string }
  applied: string[]
  unsupported: string[]
  omitted?: string[]
  error?: string
  message?: string
}

export function parseJql(input: string, email?: string | null): Promise<JqlParseResult> {
  return json<JqlParseResult>('jql/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ input, email: email ?? '' }),
  })
}

/** What the 500ms ui-focus poll carries back (GDK-791). */
export interface UIFocusPoll {
  /** View hash left by `gadak views open`. null when nothing is still fresh. */
  hash: string | null
  /**
   * Write timestamp of the focus file (RFC3339). Empty when hash is null.
   * Each client applies a given `at` once (GDK-960).
   */
  at: string
  /**
   * Disk identity of config.json — always present on current servers, empty
   * on older ones (204 era) and on fetch failure. When it moves, settings
   * were written elsewhere (CLI `config set`, another tab) and the app
   * refetches config.json instead of reloading.
   */
  configVersion: string
  /**
   * Disk identity of the mirror (GDK-1170) — same trick as configVersion, one
   * level down. Empty on an older server, on a server with no mirror, and on
   * fetch failure; empty means "no signal", never "nothing changed", so the
   * issue store's 15s backstop poll stays in charge. When it moves the mirror
   * was written — by `gadak claim` in a terminal, another tab, or the watch
   * loop — and the board pulls a delta on this tick instead of waiting out
   * that backstop.
   */
  mirrorVersion: string
}

const emptyUIFocus = (): UIFocusPoll => ({
  hash: null,
  at: '',
  configVersion: '',
  mirrorVersion: '',
})

export async function pollUIFocus(): Promise<UIFocusPoll> {
  try {
    const res = await raw('ui-focus/')
    // 404 = serve without the endpoint. 204 = an older server with nothing
    // pending (no body to decode); either way only the focus half is dead.
    if (res.status === 404 || res.status === 204) return emptyUIFocus()
    if (!res.ok) return emptyUIFocus()
    const body = (await res.json()) as {
      hash?: string
      at?: string
      configVersion?: string
      mirrorVersion?: string
    }
    return {
      hash: body.hash?.trim() ? body.hash : null,
      at: typeof body.at === 'string' ? body.at : '',
      configVersion: typeof body.configVersion === 'string' ? body.configVersion : '',
      mirrorVersion: typeof body.mirrorVersion === 'string' ? body.mirrorVersion : '',
    }
  } catch {
    return emptyUIFocus()
  }
}

export function emitJql(
  filters: import('./view-config').ViewFilters,
  display: import('./view-config').ViewDisplay,
  email?: string | null,
): Promise<{ jql: string; omitted: string[] }> {
  return json('jql/emit/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ filters, display, email: email ?? '' }),
  })
}

/* ── Mirrored wiki pages (docs) ── */

/** Every mirrored page, without bodies. Small enough to hold in memory. */
export function getPages(): Promise<PagesResponse> {
  return json<PagesResponse>('pages/')
}

/** One page with its body ADF and comments. */
export function getPageDetail(key: string): Promise<PageDetail> {
  return json<PageDetail>(`pages/${encodeURIComponent(key)}/`)
}

/**
 * Re-fetch one issue from the upstream tracker into the mirror.
 * Response shape matches other write endpoints (refreshed IssueLite).
 */
export function resyncIssue(issueKey: string): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/resync/`, {
    method: 'POST',
  })
}

/**
 * Re-fetch one mirrored wiki page. 204 with no body on success.
 */
export async function resyncPage(pageId: string): Promise<void> {
  const res = await raw(`pages/${encodeURIComponent(pageId)}/resync/`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(res.status, `POST pages/${pageId}/resync/ → ${res.status}`)
  }
}

/**
 * Post a top-level comment on a wiki page through the owning origin
 * (GDK-381). Returns the refreshed PageDetail.
 */
export function commentOnPage(pageId: string, text: string): Promise<{ page: PageDetail }> {
  return jsonW<{ page: PageDetail }>(`pages/${encodeURIComponent(pageId)}/comment/`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  })
}

/* ── People axis ── */

/**
 * One person's comments, newest first. `authorId` is the source account id
 * (`comments.author_id`), which is what `Member.jira_account_id` carries.
 * Unlike the rest of the read path this is a request: comment bodies are not
 * in the client pool, and holding every comment in memory to answer one panel
 * would cost the boot budget the pool is spent on.
 */
export function getCommentsByAuthor(
  authorId: string,
  limit = 50,
): Promise<CommentsByAuthorResponse> {
  return json<CommentsByAuthorResponse>(
    `people/${encodeURIComponent(authorId)}/comments/?limit=${limit}`,
  )
}

/* ── Personal feed / read state ── */

export function getFeed(focus: FeedFocus = 'all', limit = 80): Promise<FeedResponse> {
  return json<FeedResponse>(`feed/?focus=${encodeURIComponent(focus)}&limit=${limit}`)
}

export function markFeedRead(payload: {
  event_ids?: string[]
  issue_keys?: string[]
  all?: boolean
}): Promise<{ updated: number; unread_counts: FeedUnreadCounts }> {
  return json('feed/read/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

/* ── Saved views ── */

export function getViews(): Promise<ViewsResponse> {
  return json<ViewsResponse>('views/')
}

export function createView(
  name: string,
  config: import('./view-config').ViewConfig,
): Promise<SavedView> {
  return json<SavedView>('views/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, config }),
  })
}

/**
 * One-shot (GDK-437): move this browser's localStorage views onto the server.
 * Structural type on purpose — importing the store's PersonalView here would
 * invert the api←store dependency. Returns the merged views document.
 */
export function absorbViews(
  views: { id: string; name: string; config: unknown; created_at?: string }[],
): Promise<ViewsResponse> {
  return json<ViewsResponse>('views/absorb/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ views }),
  })
}

export async function deleteView(id: string): Promise<void> {
  const res = await raw(`views/${encodeURIComponent(id)}/`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) {
    throw new ApiError(res.status, `DELETE views/${id}/ → ${res.status}`)
  }
}

/* ── Agent dashboards (GDK-781) — served next to issues under /api/v1/dashboards/ ── */

/**
 * Dashboards live under /api/v1/dashboards/, a sibling of the issues API base
 * (/api/v1/issues/). Deriving it from apiBase keeps workspace mounts
 * (/w/<name>/) working: their apiBase is /w/<name>/api/v1/issues/, so that
 * workspace's dashboards follow automatically.
 */
export function dashboardsBase(): string {
  return config().apiBase.replace(/issues\/$/, 'dashboards/')
}

/** GET/parse on the dashboards base (same contract as `json`, different prefix). */
async function dashJSON<T>(path: string): Promise<T> {
  const url = dashboardsBase() + path
  let res: Response
  try {
    res = await fetch(url, { credentials: 'same-origin' })
  } catch (err) {
    noteNetworkFailure(err)
    throw err
  }
  noteServerFailure(res.status)
  if (!res.ok) {
    let code: string | null = null
    let message = `GET ${url} → ${res.status}`
    try {
      const body = (await res.json()) as { error?: string; message?: string }
      if (body.error) {
        code = body.error
        message = body.message ? `${body.error}: ${body.message}` : body.error
      }
    } catch {
      /* No body / non-JSON — keep default message */
    }
    throw new ApiError(res.status, message, code)
  }
  return (await res.json()) as T
}

/** GET dashboards/ — rows without configs, plus the change counter. */
export function getDashboards(): Promise<import('./types').DashboardsResponse> {
  return dashJSON<import('./types').DashboardsResponse>('')
}

/** GET dashboards/{id}/ — the row with its config (the host's datasource map). */
export function getDashboard(id: string): Promise<import('./types').DashboardRow> {
  return dashJSON<import('./types').DashboardRow>(`${encodeURIComponent(id)}/`)
}

/** GET dashboards/{id}/data/{name}/ — one datasource execution (host-side). */
export function getDashboardData(
  id: string,
  name: string,
): Promise<import('./types').DashboardDataDoc> {
  return dashJSON<import('./types').DashboardDataDoc>(
    `${encodeURIComponent(id)}/data/${encodeURIComponent(name)}/`,
  )
}

/* ── Watches ── */

export function getWatches(): Promise<WatchesResponse> {
  return json<WatchesResponse>('watches/')
}

export async function addWatch(issueKey: string): Promise<void> {
  const res = await raw(`watches/${encodeURIComponent(issueKey)}/`, { method: 'PUT' })
  if (!res.ok) throw new ApiError(res.status, `PUT watches/${issueKey}/ → ${res.status}`)
}

export async function removeWatch(issueKey: string): Promise<void> {
  const res = await raw(`watches/${encodeURIComponent(issueKey)}/`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) {
    throw new ApiError(res.status, `DELETE watches/${issueKey}/ → ${res.status}`)
  }
}

/* ── Favorites (response shape matches watches) ── */

export function getFavorites(): Promise<WatchesResponse> {
  return json<WatchesResponse>('favorites/')
}

export async function addFavorite(issueKey: string): Promise<void> {
  const res = await raw(`favorites/${encodeURIComponent(issueKey)}/`, { method: 'PUT' })
  if (!res.ok) throw new ApiError(res.status, `PUT favorites/${issueKey}/ → ${res.status}`)
}

export async function removeFavorite(issueKey: string): Promise<void> {
  const res = await raw(`favorites/${encodeURIComponent(issueKey)}/`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) {
    throw new ApiError(res.status, `DELETE favorites/${issueKey}/ → ${res.status}`)
  }
}

/* ── Write — with error-body parsing ──
 * Write endpoints return { error, jira_errors? } on failure. 409 credential_required is
 *  exposed as ApiError.code so callers can open the credential dialog.
 */

/** Write-response parser — on !ok, throw ApiError carrying body error/jira_errors. */
async function jsonW<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await raw(path, init)
  if (!res.ok) {
    let code: string | null = null
    let jiraErrors: Record<string, unknown> | null = null
    let message = `${init?.method ?? 'GET'} ${path} → ${res.status}`
    try {
      const body = (await res.json()) as {
        error?: string
        message?: string
        jira_errors?: Record<string, unknown>
      }
      if (body.error) {
        code = body.error
        // failMsg's reason (placeholder refusals, GDK-1396): the server's own
        // wording, shown as it is.
        message = body.message || body.error
      }
      if (body.jira_errors) jiraErrors = body.jira_errors
    } catch {
      /* No body / non-JSON — keep default message */
    }
    throw new ApiError(res.status, message, code, jiraErrors)
  }
  return (await res.json()) as T
}

const JSON_HEADERS = { 'Content-Type': 'application/json' }

/* ── Credentials (personal Jira API token) ── */

export function getCredential(): Promise<JiraCredential> {
  return jsonW<JiraCredential>('credential/')
}

export function saveCredential(
  jiraEmail: string,
  apiToken: string,
  tokenExpiresAt?: string,
): Promise<JiraCredential> {
  const body: Record<string, string> = { jira_email: jiraEmail, api_token: apiToken }
  const expires = tokenExpiresAt?.trim()
  if (expires) body.token_expires_at = expires
  return jsonW<JiraCredential>('credential/', {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  })
}

export function deleteCredential(): Promise<JiraCredential> {
  return jsonW<JiraCredential>('credential/', { method: 'DELETE' })
}

/* ── First-run onboarding (loopback only) ──
 * Unlike `credential/`, this also takes site: first run has no stored site, and
 * `PUT credential/` rejects without a site because there is nothing to verify against.
 */

export interface AvailableProject {
  key: string
  name: string
  projectTypeKey: string
}

/** Shared response of POST sync/ · GET sync/progress/. Never carries credential data. */
export interface SyncProgress {
  running: boolean
  /** idle | syncing | done | error */
  phase: string
  fetched: number
  changed: number
  deleted: number
  done: boolean
  error: string
  /**
   * Machine-readable classification of `error`, present only when the failure
   * means the stored credential was rejected ("credential_rejected" — the same
   * code the write path's 409 uses). Absent on older servers and on every
   * other failure; key recovery affordances on this, never on `error` prose.
   */
  error_code?: string
  started_at: string
  finished_at: string
  /**
   * Any pass running in the server process, including the background watch
   * loop — the fields above describe only the one-shot job this client can
   * start. Absent on servers older than this field; treat that as "unknown",
   * never as "idle".
   */
  activity?: {
    running: boolean
    /** issues | documents, '' when nothing is in flight. */
    source: string
    /** Running total for the current source's pass, reset when it changes. */
    fetched: number
    changed: number
    started_at: string
  }
}

/** Verify site+email+token via /myself, then store. Failures distinguished by ApiError.code. */
export async function connectJira(
  site: string,
  jiraEmail: string,
  apiToken: string,
  tokenExpiresAt?: string,
  opts?: { replaceLocalOrigin?: boolean },
): Promise<JiraCredential> {
  const body: Record<string, unknown> = { site, jira_email: jiraEmail, api_token: apiToken }
  const expires = tokenExpiresAt?.trim()
  if (expires) body.token_expires_at = expires
  if (opts?.replaceLocalOrigin) body.replace_standalone = true
  const res = await raw('onboarding/connect/', {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    let code: string | null = null
    let issues = 0
    let persist = ''
    let message = `PUT onboarding/connect/ → ${res.status}`
    try {
      const doc = (await res.json()) as {
        error?: string
        issues?: number
        persist?: string
      }
      if (doc.error) {
        code = doc.error
        message = doc.error
      }
      if (typeof doc.issues === 'number') issues = doc.issues
      if (typeof doc.persist === 'string') persist = doc.persist
    } catch {
      /* No body / non-JSON — keep default message */
    }
    if (res.status === 409 && code === 'standalone_data_present') {
      throw new LocalOriginDataPresentError(res.status, issues, persist)
    }
    throw new ApiError(res.status, message, code)
  }
  return (await res.json()) as JiraCredential
}

/** What POST onboarding/standalone answers: the seeded workspace facts. */
export type LocalOriginInit = {
  workspace_kind: 'standalone'
  default_project: string
}

/**
 * Seed a local-origin workspace — the GUI twin of `gadak init --local`
 * (GDK-377). Body is `{}`: the server seeds the default STD project and the
 * LOC wiki space. 409 workspace_connected when this workspace already has an
 * origin; a workspace that is already local-origin answers 200 idempotently.
 */
export function createLocalOriginWorkspace(): Promise<LocalOriginInit> {
  return jsonW<LocalOriginInit>('onboarding/standalone/', {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({}),
  })
}

/** Real project list for the site. `truncated` means the list was capped at 500. */
export function getAvailableProjects(init?: RequestInit): Promise<{
  projects: AvailableProject[]
  truncated: boolean
}> {
  return jsonW<{ projects: AvailableProject[]; truncated: boolean }>('projects/available/', init)
}

/**
 * Start a background sync. Default mode is full (onboarding first run).
 * Daily "Sync now" uses `incremental`. 409 sync_in_progress when already running.
 */
export function startSync(mode: 'full' | 'incremental' = 'full'): Promise<SyncProgress> {
  return jsonW<SyncProgress>('sync/', {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ mode }),
  })
}

export function getSyncProgress(): Promise<SyncProgress> {
  return jsonW<SyncProgress>('sync/progress/')
}

/* ── Status transitions ── */

export function getTransitions(issueKey: string): Promise<TransitionsResponse> {
  return jsonW<TransitionsResponse>(`${encodeURIComponent(issueKey)}/transitions/`)
}

export function doTransition(
  issueKey: string,
  transitionId: string,
  fields?: Record<string, unknown>,
): Promise<IssueWriteResponse> {
  const body: Record<string, unknown> = { transition_id: transitionId }
  if (fields && Object.keys(fields).length > 0) body.fields = fields
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/transition/`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify(body),
  })
}

/* ── Issue links (GDK-85) — origin write, then mirror refresh ── */

export type IssueLinkType = {
  id: string
  name: string
  inward: string
  outward: string
}

/** GET <key>/linktypes/ — site catalog for this issue's origin. */
export function getIssueLinkTypes(issueKey: string): Promise<{ link_types: IssueLinkType[] }> {
  return jsonW<{ link_types: IssueLinkType[] }>(`${encodeURIComponent(issueKey)}/linktypes/`)
}

/** POST <key>/link/ — `type` is name, inward/outward description, or id. */
export function createIssueLink(
  issueKey: string,
  type: string,
  otherKey: string,
): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/link/`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ type, key: otherKey }),
  })
}

/* ── Comments ── */

export function postComment(
  issueKey: string,
  text: string,
  mentions: CommentMention[] = [],
  attachmentIds: string[] = [],
): Promise<CommentWriteResponse> {
  return jsonW<CommentWriteResponse>(`${encodeURIComponent(issueKey)}/comment/`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ text, mentions, attachment_ids: attachmentIds }),
  })
}

/**
 * Upload a comment attachment (multipart). Do not set Content-Type yourself so the
 * browser can add the boundary (skip JSON_HEADERS). Returns uploaded attachment meta.
 */
export function uploadCommentAttachment(
  issueKey: string,
  file: File,
): Promise<AttachmentUploadResponse> {
  const fd = new FormData()
  fd.append('file', file)
  return jsonW<AttachmentUploadResponse>(`${encodeURIComponent(issueKey)}/attachments/`, {
    method: 'POST',
    body: fd,
  })
}

/* ── Assignee ── */

export function setAssignee(
  issueKey: string,
  accountId: string | null,
): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/assignee/`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ account_id: accountId }),
  })
}

/* ── Labels ── */

/** PUT <key>/labels/ — full replace. Send `[]` to clear. */
export function setLabels(issueKey: string, labels: string[]): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/labels/`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ labels }),
  })
}

/* ── Priority ── */

export function getPriorities(): Promise<PrioritiesResponse> {
  return jsonW<PrioritiesResponse>('priorities/')
}

/** Per-key catalog: Linear rows answer 0-4, Jira rows match GET priorities/. */
export function getPrioritiesFor(issueKey: string): Promise<PrioritiesResponse> {
  return jsonW<PrioritiesResponse>(`${encodeURIComponent(issueKey)}/priorities/`)
}

/** PUT <key>/priority/ — `null` clears. Send the site id, not the display name. */
export function setPriority(issueKey: string, priorityId: string | null): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/priority/`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ priority_id: priorityId }),
  })
}

/** PUT <key>/summary/ — trim; empty is refused. */
export function setSummary(issueKey: string, summary: string): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/summary/`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ summary }),
  })
}

/**
 * POST preview/ — render a markdown draft the way a save would store it.
 * The server owns the one converter (GDK-1385); the web keeps no parser.
 */
export function previewMarkdown(
  text: string,
  base: AdfNode | null = null,
): Promise<{ adf: AdfNode | null }> {
  // base: the body the draft was opened from, so its placeholders resolve
  // the way the save will (GDK-1396). 409 placeholder names one that cannot.
  return jsonW<{ adf: AdfNode | null }>('preview/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(base ? { text, base } : { text }),
  })
}

/**
 * PUT <key>/description/ — markdown; `null` or whitespace clears. Server
 * builds the ADF and puts the body's preserved nodes back where the text's
 * placeholders stand (GDK-1396). 409 format_loss: the text has no
 * placeholder and the body has preserved nodes — `force` replaces anyway.
 * 409 placeholder: a marker the current body cannot honour (message says).
 */
export function setDescription(
  issueKey: string,
  description: string | null,
  force = false,
): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/description/`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify(force ? { description, force } : { description }),
  })
}

/** PUT <key>/duedate/ — `null` or `""` clears. Send YYYY-MM-DD, not a timestamp. */
export function setDuedate(issueKey: string, duedate: string | null): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/duedate/`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ duedate }),
  })
}

/* ── QA field inline edit ── */

/** PATCH <key>/fields/ — change an allowed QA field (option id / accountId / version id array). */
export function setIssueField(
  issueKey: string,
  field: string,
  value: string | string[] | null,
): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/fields/`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify({ field, value }),
  })
}

/** GET <key>/editmeta/ — allowed edit values (options/versions). Non-editable fields omitted. */
export function getEditMeta(issueKey: string): Promise<EditMetaResponse> {
  return jsonW<EditMetaResponse>(`${encodeURIComponent(issueKey)}/editmeta/`)
}

/* ── Issue create ── */

export function createIssue(payload: CreateIssuePayload): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>('create/', {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify(payload),
  })
}

export function getCreateMeta(): Promise<CreateMetaResponse> {
  return jsonW<CreateMetaResponse>('create-meta/')
}

/** GET create-meta/fields/?project=&issue_type= — issue_type is the id, never a localized name. */
export function getCreateFields(
  project: string,
  issueTypeId: string,
  init?: RequestInit,
): Promise<CreateFieldsResponse> {
  const q =
    `project=${encodeURIComponent(project)}` + `&issue_type=${encodeURIComponent(issueTypeId)}`
  return jsonW<CreateFieldsResponse>(`create-meta/fields/?${q}`, init)
}

/* ── User search (for assignee picker) ── */

export function searchUsers(q: string): Promise<UsersResponse> {
  return jsonW<UsersResponse>(`users/?q=${encodeURIComponent(q)}`)
}

/** Per-key user search: Linear rows query Linear members, Jira rows match GET users/. */
export function searchUsersFor(issueKey: string, q: string): Promise<UsersResponse> {
  return jsonW<UsersResponse>(`${encodeURIComponent(issueKey)}/users/?q=${encodeURIComponent(q)}`)
}

/* ── Server settings (loopback only) ──
 * Editable slice of `~/.gadak/config.json`. Credentials are not included (credential/ owns those).
 * Any field may be absent in the response, so everything is optional.
 */

export interface SettingsMember {
  email: string
  name?: string
  display_name?: string
  group?: string
  department?: string
  job_role?: string
  jira_account_id?: string
  avatar_url?: string
}

/** Group-assignment rule. First match wins top-to-bottom; conditions AND, list values OR; empty condition always true. */
export interface SettingsGroupRule {
  group: string
  projects?: string[]
  labels?: string[]
  components?: string[]
}

/** Read-only instance facts from GET settings/. Never carries secrets. */
export interface SettingsRuntime {
  profile: string
  dbPath: string
  dbSizeBytes: number
  dbSizeHuman: string
  dbModifiedAt?: string | null
  configPath: string
  issueCount: number
  commentCount: number
  schemaVersion: number
  watermark?: string
  syncVersion: number
  lastFullSyncAt?: string | null
  lastError?: string | null
  gadakVersion: string
  /** Placeholder when syncIntervalSec is 0. */
  defaultSyncIntervalSec: number
  /** Placeholder when reconcileIntervalSec is 0. */
  defaultReconcileIntervalSec: number
  /**
   * Whether this process can fire a real OS desktop notification.
   * Always sent on current servers (false is Windows / other no-ops).
   * Omitted on older servers — treat as true so macOS/Linux keep hiding
   * the in-tab browser toggle.
   */
  osNotifySupported?: boolean
  /** Our own outbound Jira volume, not Jira's remaining budget. */
  apiUsage?: ApiUsageSummary
}

export interface ApiUsageDay {
  day: string
  requests: number
  throttled: number
  server_errors: number
  retries: number
  wait_ms: number
  last_throttled_at?: string | null
}

export interface ApiUsageSummary {
  today: ApiUsageDay
  last_7_days: ApiUsageDay
}

/** One discovered/configured custom field as settings carries it (full shape, ids included). */
export interface SettingsFieldSpec {
  alias: string
  label: string
  ids: string[]
  role: string
  kind?: string
  /** true = discovery-owned (regenerated by `gadak fields --apply`); false = user-pinned. */
  auto?: boolean
}

/** Confluence slice of settings. Present in the GET response only while the
 *  source is configured — absence is how the UI knows to hide the scope picker,
 *  and PUTting the key with the source off is a 400. */
export interface SettingsConfluence {
  /**
   * Turn the source on or off. Absent on the way in (the server omits the whole
   * block while off); sent on the way out, because a bare `spaces` is rejected
   * unless the source is already on — which is what used to make this screen
   * unable to enable Confluence at all.
   */
  enabled?: boolean
  /** Mirrored space keys. Empty = every global space, but only while enabled. */
  spaces: string[]
}

export interface GadakSettings {
  projects?: string[]
  confluence?: SettingsConfluence
  fieldMap?: Record<string, string>
  bodyFields?: string[]
  editableFields?: Record<string, string>
  members?: SettingsMember[]
  groupRules?: SettingsGroupRule[]
  /** Optional SELECT/WITH (key, group). Omitted on PUT leaves the stored query. */
  groupQuery?: string
  groupLabels?: Record<string, string>
  groupColors?: Record<string, string>
  productByGroup?: Record<string, { key: string; label: string }>
  features?: Partial<GadakFeatures>
  qaDashboardUrl?: string
  staleThresholdHours?: number
  /** Incremental sync period in seconds. 0 = server default. Min 15 when set. */
  syncIntervalSec?: number
  /** Reconcile (deletion) period in seconds. 0 = server default. Min 300 when set. */
  reconcileIntervalSec?: number
  /** Field-spec edits. Only sent when the user touched the section — absence preserves discovery output. */
  fields?: SettingsFieldSpec[]
  /** Connection panel (read-only). */
  site?: string
  hasCredential?: boolean
  /** Instance facts (read-only; ignored on PUT). */
  runtime?: SettingsRuntime
  /** Discovered field specs (read-only; edit via `fields`). */
  fieldSpecs?: SettingsFieldSpec[]
  /** project → alias → filled (read-only). */
  fieldUsage?: Record<string, Record<string, number>>
  /**
   * UI look. GET always sends it (empty stored → "system"). Omit on PUT
   * to keep the stored value — the document is otherwise a full replace.
   */
  appearance?: { theme?: string; terminal?: string }
  /**
   * Terminal display behavior (GDK-1357): scrollback lines (0 = default)
   * and cursor blink. GET always sends it; omit on PUT to keep the stored
   * values. Shell and workingDir are deliberately not in this document
   * (GDK-1069) — `gadak config set terminal.shell` is their only road.
   */
  terminal?: { scrollback: number; cursorBlink: boolean }
  /**
   * User color overrides (GDK-786). Same omit-to-preserve rule: GET always
   * sends it, an older PUT that omits the key keeps the stored overrides.
   * Refusals (locked token, bad key kind) answer 400 with the reason.
   */
  ui?: {
    tokens?: UITokens
    tokensByTheme?: Record<string, UITokens>
    dataColors?: Record<string, Record<string, string>>
  }
}

/** One token block of `ui` (internal/config/uitokens.go UITokens): colors
 *  are hex, the dimension axes are CSS lengths, fonts are family stacks.
 *  `type.terminal` and `fonts.mono-terminal` are the two the Terminal
 *  settings tab edits. */
export interface UITokens {
  colors?: Record<string, string>
  spacing?: Record<string, string>
  layout?: Record<string, string>
  type?: Record<string, string>
  fonts?: Record<string, string>
}

/** One recorded sync pass (meaningful runs only: changed something, full, or failed). */
export interface SyncRun {
  kind: string // full | incremental (+reconcile)
  started_at: string
  finished_at: string
  fetched: number
  changed: number
  deleted: number
  error?: string
}

/** GET sync/runs/ document. last_checked_at is sources.synced_at for `source`. */
export interface SyncRunsDoc {
  runs: SyncRun[]
  source?: string
  /** Same origin as sync_health.sources[].synced_at. Absent on older servers. */
  last_checked_at?: string | null
}

/**
 * GET sync/runs/ — newest first. Older servers 404 → treat as empty.
 *
 * `source` selects which connector's history to read; omitted means Jira, the
 * only one the endpoint used to serve. A server that predates the parameter
 * ignores it and answers with Jira's runs, so callers asking for Confluence
 * must not read a non-empty answer as proof Confluence ever ran — check
 * `source` on the response.
 *
 * last_checked_at is omitted on a 404 or a server that does not send it —
 * callers hide the "Last checked" line rather than inventing a timestamp.
 */
export async function getSyncRuns(source?: 'jira' | 'confluence'): Promise<SyncRunsDoc> {
  try {
    const path = source ? `sync/runs/?source=${source}` : 'sync/runs/'
    const res = await jsonW<SyncRunsDoc>(path)
    if (source && res.source !== source) return { runs: [] }
    return {
      runs: res.runs ?? [],
      source: res.source,
      last_checked_at: res.last_checked_at,
    }
  } catch {
    return { runs: [] }
  }
}

/* ── workspaces (one process, several profile mirrors) ── */

/** One profile the running serve can mount. Never carries credentials. */
export interface WorkspaceInfo {
  name: string
  site?: string
  projects?: string[]
  /** True for the profile serve was started with (its API is at the root). */
  active?: boolean
  error?: string
}

/** Where the switcher and the palette send a click: the active one is the root mount. */
export function workspaceHref(w: WorkspaceInfo): string {
  return w.active ? '/' : `/w/${w.name}/`
}

/** The site's host, or the raw site when it is not a URL; empty for a built-in origin. */
export function workspaceHost(w: WorkspaceInfo): string {
  if (!w.site) return ''
  try {
    return new URL(w.site).host
  } catch {
    return w.site
  }
}

/**
 * List mountable workspaces. Deliberately fetched at the origin root, not the
 * API base: the list is one process-wide fact, the same from every mount.
 * Older servers and the hosted demo answer 404 → empty list, section hidden.
 */
export async function getWorkspaces(): Promise<WorkspaceInfo[]> {
  try {
    const res = await fetch('/api/v1/workspaces', { credentials: 'same-origin' })
    if (!res.ok) return []
    const doc = (await res.json()) as { workspaces?: WorkspaceInfo[] }
    return doc.workspaces ?? []
  } catch {
    return []
  }
}

/**
 * A refused workspace-management call, kept as data rather than a message:
 * `error` is the server's code (self_delete, exists, forbidden_host, …) and
 * `detail` is the CLI's own refusal wording, which the UI shows verbatim —
 * the server stays the single owner of what a refusal means.
 */
export class WorkspaceManageError extends Error {
  status: number
  error: string | null
  detail: string | null
  constructor(status: number, error: string | null, detail: string | null) {
    super(error ?? `workspaces → ${status}`)
    this.name = 'WorkspaceManageError'
    this.status = status
    this.error = error
    this.detail = detail
  }
}

/** POST /api/v1/workspaces 201 document (internal/workspace/manage.go). */
export interface CreatedWorkspace {
  name: string
  kind: string
  /** Absolute path of the new local-origin persist — informational. */
  persist: string
}

/** DELETE /api/v1/workspaces/{name} 200 document. `advisories` is server
 *  wording, rendered as-is (pairing hint, stored-default cleanup, …). */
export interface RemovedWorkspace {
  removed: string
  kind: string
  origin_destroyed: boolean
  advisories: string[]
}

/** Read a {error, detail?} refusal body off a non-OK response. Never throws:
 *  a body that is not JSON leaves code/detail null and the status says it. */
async function manageRefusal(res: Response): Promise<WorkspaceManageError> {
  let error: string | null = null
  let detail: string | null = null
  try {
    const doc = (await res.json()) as { error?: unknown; detail?: unknown }
    if (typeof doc.error === 'string') error = doc.error
    if (typeof doc.detail === 'string') detail = doc.detail
  } catch {
    /* not JSON — the status is the whole answer */
  }
  return new WorkspaceManageError(res.status, error, detail)
}

/**
 * List mountable workspaces, refusing where getWorkspaces collapses: the
 * management tab must tell "empty list" and "this browser may not manage
 * workspaces" (403 forbidden_host on a DNS-named Host) apart, and a plain
 * network failure apart from both. Same origin-root fetch as getWorkspaces —
 * the list is one process-wide fact, the same from every mount.
 */
export async function listWorkspaces(): Promise<WorkspaceInfo[]> {
  const res = await fetch('/api/v1/workspaces', { credentials: 'same-origin' })
  if (!res.ok) throw await manageRefusal(res)
  const doc = (await res.json()) as { workspaces?: WorkspaceInfo[] }
  return doc.workspaces ?? []
}

/**
 * Create a local-origin workspace. `projects` is a CSV the server parses;
 * empty mirrors every project. The paired flow is pairWorkspace's —
 * connected still needs the credential flow and stays off this API.
 */
export async function createWorkspace(name: string, projects = ''): Promise<CreatedWorkspace> {
  const body: Record<string, string> = { name, kind: 'standalone' }
  const csv = projects.trim()
  if (csv) body.projects = csv
  const res = await fetch('/api/v1/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify(body),
  })
  if (!res.ok) throw await manageRefusal(res)
  return (await res.json()) as CreatedWorkspace
}

/** POST /api/v1/workspaces kind:"paired" 201 document (createPaired). The
 *  new workspace reports kind connected — the same listing semantics the
 *  CLI pairing flow produces. */
export interface PairedWorkspace {
  name: string
  kind: string
  endpoint: string
  label: string
  account: string
}

/**
 * Register a remote gadak serve as a fresh workspace from its one-line
 * pairing offer (GDK-1099). The server verifies the offer against the
 * remote serve before writing anything (verify-before-save); its refusal
 * wording (invalid_offer, pairing_refused, serve_unreachable) is the CLI's
 * own, carried in `detail` for the UI to show verbatim. The offer is
 * credential-shaped: it goes into the request body and nowhere else.
 */
export async function pairWorkspace(name: string, offer: string): Promise<PairedWorkspace> {
  const res = await fetch('/api/v1/workspaces', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ name, kind: 'paired', offer }),
  })
  if (!res.ok) throw await manageRefusal(res)
  return (await res.json()) as PairedWorkspace
}

/**
 * Remove a workspace. With no options this is the *probe*: the server
 * answers 400 with the refusal whose detail explains what removal means for
 * this workspace (needs_destroy_origin carries the persist path) — that
 * wording is the confirmation dialog's body. `yes: true` commits, and
 * `destroyOrigin: true` additionally destroys a local-origin persist (the
 * only copy of that tracker).
 */
export async function removeWorkspace(
  name: string,
  opts: { yes?: boolean; destroyOrigin?: boolean } = {},
): Promise<RemovedWorkspace> {
  const q = new URLSearchParams()
  if (opts.yes) q.set('yes', '1')
  if (opts.destroyOrigin) q.set('destroy_origin', '1')
  const qs = q.toString()
  const suffix = qs ? `?${qs}` : ''
  const res = await fetch(`/api/v1/workspaces/${encodeURIComponent(name)}${suffix}`, {
    method: 'DELETE',
    credentials: 'same-origin',
  })
  if (!res.ok) throw await manageRefusal(res)
  return (await res.json()) as RemovedWorkspace
}

export function getSettings(): Promise<GadakSettings> {
  return jsonW<GadakSettings>('settings/')
}

/** One Confluence space offered by the scope picker. */
export interface SettingsSpace {
  key: string
  name: string
  /** global | personal. Personal spaces are noise for most mirrors. */
  type: string
  selected: boolean
}

/**
 * Live space list for the settings picker (global first). `all_global_when_empty`
 * says the stored scope is empty, which the sync reads as "every global space" —
 * so an empty selection here is a meaning, not a missing answer.
 * 400 confluence_not_configured when the source is off.
 */
export function getSettingsSpaces(init?: RequestInit): Promise<{
  spaces: SettingsSpace[]
  all_global_when_empty: boolean
}> {
  // `enabled` is the source's on/off state. The list itself is live discovery
  // and needs only a credential, so it answers while Confluence is off too —
  // which is what lets someone choose spaces before turning it on.
  return jsonW<{
    spaces: SettingsSpace[]
    all_global_when_empty: boolean
    enabled: boolean
  }>('settings/spaces/', init)
}

/** Full replace (PUT). Values are stored as sent — partial payloads wipe the rest. */
export function putSettings(settings: GadakSettings): Promise<GadakSettings> {
  return jsonW<GadakSettings>('settings/', {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify(settings),
  })
}

/* ── Write meta (anonymous read) — transition map + create-meta prefetched ── */

export function getWriteMeta(): Promise<WriteMeta> {
  return json<WriteMeta>('meta/write/')
}
