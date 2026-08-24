/*
 * serve REST client — the transport owner for every screen.
 *
 * Endpoints (flow-report Q3, measured against a live serve; the same REST
 * web/src/lib/api.ts consumes):
 *
 *   GET  /api/v1/auth/me/
 *   GET  /api/v1/issues/bootstrap/         — If-None-Match → 304
 *   GET  /api/v1/issues/delta/?since=&mv=
 *   GET  /api/v1/issues/{key}/detail/
 *   POST /api/v1/issues/{key}/comment/            {"text": …}
 *   GET  /api/v1/issues/search/?q=&limit=
 *   GET  /api/v1/issues/{key}/transitions/
 *   POST /api/v1/issues/{key}/transition/         {"transition_id": …}
 *   GET  /api/v1/issues/feed/?focus=&limit=
 *   POST /api/v1/issues/feed/read/                {event_ids|issue_keys|all}
 *
 * Transport: the packaged app rides @tauri-apps/plugin-http (Rust reqwest —
 * no Origin header, no CORS preflight; docs/decisions/0003). The serve gate
 * answers a webview fetch with forbidden_origin and serves no CORS headers,
 * so direct fetch stays a DEV/vitest-only path through the vite /api proxy.
 * The http capability is URL-scoped in src-tauri/capabilities/default.json.
 *
 * List logic keys on ids/categories only — status_category and priority_rank,
 * never display names (CLAUDE.md schema rules). priority_rank 0 is unset, not
 * "most urgent" (specs/001 L1 trap), so unset rows sort last.
 */

import { fetch as tauriFetch } from '@tauri-apps/plugin-http'
import { inTauriApp } from './settings'

/** The subset of store.IssueLite the queue renders. */
export interface QueueRow {
  issue_key: string
  summary: string
  status: string
  status_category: string
  priority: string | null
  priority_rank: number
  assignee: string | null
  updated_at: string | null
}

export interface Bootstrap {
  issues: QueueRow[]
  sync_version: number
  server_time: string
}

/** Where requests go, and the pairing token ('' when unpaired). */
export interface ApiContext {
  endpoint: string
  token: string
}

/**
 * Server failures are `{"error": "<code>"}` (internal/server `fail()`).
 * The code is the branch axis for the UI — forbidden_host teaches
 * "wrong network", pairing_rejected teaches "re-pair", credential_required
 * teaches "fix the home's Jira login" — so it rides on the error object
 * instead of a message string. Messages stay log-safe: the token never
 * appears in any of them.
 */
export class ApiError extends Error {
  /** HTTP status when a response arrived; undefined for network/shape. */
  readonly status: number | undefined
  /** Server error code when the body carried `{"error": code}`. */
  readonly code: string | undefined

  constructor(message: 'network' | 'shape' | string, opts?: { status?: number; code?: string }) {
    super(message)
    this.name = 'ApiError'
    this.status = opts?.status
    this.code = opts?.code
  }
}

/**
 * The one transport decision in the app: packaged builds run inside the
 * Tauri webview (`_TAURI_INTERNALS_` injected) and use plugin-http; DEV
 * (vite proxy) and vitest keep window fetch. Read per call, not at module
 * load, so tests can exercise both branches.
 */
function useNativeHttp(): boolean {
  return !import.meta.env.DEV && inTauriApp()
}

async function request(ctx: ApiContext, path: string, init?: RequestInit): Promise<Response> {
  const headers = new Headers(init?.headers)
  if (ctx.token !== '') headers.set('Authorization', `Bearer ${ctx.token}`)
  // DEV rides the vite /api proxy (same-origin — serve sends no CORS
  // headers); the packaged app talks to the paired endpoint directly.
  const base = import.meta.env.DEV ? '' : ctx.endpoint.replace(/\/+$/, '')
  const url = `${base}/api/v1/${path}`
  const transport = useNativeHttp() ? tauriFetch : fetch
  try {
    return await transport(url, { ...init, headers })
  } catch (err) {
    if (init?.signal?.aborted) throw err
    throw new ApiError('network')
  }
}

/** Reads an error body's `{"error": code}` without ever echoing the body. */
async function toApiError(res: Response): Promise<ApiError> {
  let code: string | undefined
  try {
    const body: unknown = JSON.parse(await res.text())
    const c = (body as { error?: unknown } | null)?.error
    if (typeof c === 'string' && c.length > 0 && c.length <= 64) code = c
  } catch {
    /* non-JSON error body — status only */
  }
  const message = code === undefined ? `http ${res.status}` : `http ${res.status} ${code}`
  return new ApiError(message, { status: res.status, code })
}

function expectField(body: unknown, field: string): unknown {
  if (typeof body !== 'object' || body === null) throw new ApiError('shape')
  return (body as Record<string, unknown>)[field]
}

function expectArray(body: unknown, field: string): unknown[] {
  const v = expectField(body, field)
  if (!Array.isArray(v)) throw new ApiError('shape')
  return v
}

async function parseJson<T>(res: Response, validate?: (body: unknown) => void): Promise<T> {
  let body: unknown
  try {
    body = JSON.parse(await res.text())
  } catch {
    throw new ApiError('shape')
  }
  validate?.(body)
  return body as T
}

/* ── wrappers: each names one endpoint, validates the one field it needs ── */

/** GET auth/me/ — the account the home mirror syncs as "me". */
export async function me(ctx: ApiContext, signal?: AbortSignal): Promise<Me> {
  const res = await request(ctx, 'auth/me/', { signal })
  if (!res.ok) throw await toApiError(res)
  return parseJson<Me>(res, (b) => {
    expectField(b, 'account_id')
  })
}

export interface Me {
  /** null on a standalone home — only a connected home has an account. */
  account_id: string | null
  email: string
  name: string
}

export type BootstrapResult =
  | { status: 'ok'; data: Bootstrap; etag: string | null }
  | { status: 'not_modified' }

/**
 * GET issues/bootstrap/ — the full lite-rows snapshot (the only list). With
 * a prior `etag` the server may answer 304. The etag is opaque — echo what
 * the server sent, never parse it (serve uses "sv-<n>", hosted "in-<n>").
 */
export async function bootstrap(
  ctx: ApiContext,
  etag?: string | null,
  signal?: AbortSignal,
): Promise<BootstrapResult> {
  const headers = new Headers()
  if (etag) headers.set('If-None-Match', etag)
  const res = await request(ctx, 'issues/bootstrap/', { headers, signal })
  if (res.status === 304) return { status: 'not_modified' }
  if (!res.ok) throw await toApiError(res)
  const data = await parseJson<Bootstrap>(res, (b) => {
    expectArray(b, 'issues')
  })
  return { status: 'ok', data, etag: res.headers.get('ETag') }
}

/** The delta fields this app consumes — members/wiki stay web-side. */
export interface Delta {
  server_time: string
  upserted: QueueRow[]
  deleted_keys: string[]
}

/** GET issues/delta/ — rows changed since `since` (the bootstrap cursor). */
export async function delta(
  ctx: ApiContext,
  since: string,
  mv?: string,
  signal?: AbortSignal,
): Promise<Delta> {
  let path = `issues/delta/?since=${encodeURIComponent(since)}`
  if (mv !== undefined && mv !== '') path += `&mv=${encodeURIComponent(mv)}`
  const res = await request(ctx, path, { signal })
  if (!res.ok) throw await toApiError(res)
  return parseJson<Delta>(res, (b) => {
    expectArray(b, 'upserted')
  })
}

export interface DetailComment {
  comment_id: string
  author_account_id: string | null
  body: string
  /** Raw ADF — kept for later rendering, opaque here. */
  raw_body: unknown
  created_at: string | null
}

export interface DetailHistoryEntry {
  at: string | null
  field: string
  /** Category axis of a status change — from/to names are display-only. */
  from_category: string | null
  to_category: string | null
}

/**
 * Narrow slice of GET {key}/detail/ — body, comments, history. The issue's
 * own status_category/priority_rank are NOT in this payload (measured,
 * flow-report Q3 ②): the caller keeps the list row for those.
 */
export interface Detail {
  issue_key: string
  description_adf: unknown
  description_text?: string
  comments: DetailComment[]
  history: DetailHistoryEntry[]
}

export async function detail(ctx: ApiContext, key: string, signal?: AbortSignal): Promise<Detail> {
  const res = await request(ctx, `issues/${encodeURIComponent(key)}/detail/`, { signal })
  if (!res.ok) throw await toApiError(res)
  return parseJson<Detail>(res, (b) => {
    expectArray(b, 'comments')
  })
}

export interface CommentWrite {
  issue: QueueRow
  comment: {
    comment_id: string
    author: string | null
    body: string
    created_at: string | null
  }
}

/** POST {key}/comment/ — origin write through the home; returns the fresh row. */
export async function postComment(
  ctx: ApiContext,
  key: string,
  text: string,
  signal?: AbortSignal,
): Promise<CommentWrite> {
  const res = await request(ctx, `issues/${encodeURIComponent(key)}/comment/`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
    signal,
  })
  if (!res.ok) throw await toApiError(res)
  return parseJson<CommentWrite>(res, (b) => {
    expectField(b, 'comment')
  })
}

export interface SearchMatch {
  field: string
  snippet: string
}

/**
 * GET search/ — keys only; search does not return IssueLite (measured).
 * The caller joins keys against the bootstrap pool.
 */
export interface SearchResult {
  keys: string[]
  total: number
  matches?: Record<string, SearchMatch>
}

export async function search(
  ctx: ApiContext,
  q: string,
  limit = 50,
  signal?: AbortSignal,
): Promise<SearchResult> {
  const path = `issues/search/?q=${encodeURIComponent(q)}&limit=${limit}`
  const res = await request(ctx, path, { signal })
  if (!res.ok) throw await toApiError(res)
  return parseJson<SearchResult>(res, (b) => {
    expectArray(b, 'keys')
  })
}

export interface Transition {
  id: string
  /** Display names — pass-through for labels only, never a branch key. */
  name: string
  to_status: string
  /** new|inprogress|done — the axis the UI keys on. */
  to_category: string
}

export interface Transitions {
  transitions: Transition[]
}

/** GET {key}/transitions/ — the live origin's options for this issue. */
export async function transitions(
  ctx: ApiContext,
  key: string,
  signal?: AbortSignal,
): Promise<Transitions> {
  const res = await request(ctx, `issues/${encodeURIComponent(key)}/transitions/`, { signal })
  if (!res.ok) throw await toApiError(res)
  return parseJson<Transitions>(res, (b) => {
    expectArray(b, 'transitions')
  })
}

export interface IssueWrite {
  issue: QueueRow
}

/**
 * POST {key}/transition/ — `transitionId` is the id or the to_category
 * value (write.go accepts both); never the display name.
 */
export async function postTransition(
  ctx: ApiContext,
  key: string,
  transitionId: string,
  signal?: AbortSignal,
): Promise<IssueWrite> {
  const res = await request(ctx, `issues/${encodeURIComponent(key)}/transition/`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ transition_id: transitionId }),
    signal,
  })
  if (!res.ok) throw await toApiError(res)
  return parseJson<IssueWrite>(res, (b) => {
    expectField(b, 'issue')
  })
}

export type FeedEventType =
  | 'created'
  | 'status_changed'
  | 'reopened'
  | 'assigned'
  | 'comment_added'
  | 'attachment_added'
  | 'fields_changed'

export type FeedFocus = 'all' | 'assignee' | 'reporter' | 'mention'

export interface FeedItem {
  event_id: string
  issue_key: string
  summary: string
  /** Display name — banner copy only; branching uses event_type/reasons. */
  current_status: string
  event_type: FeedEventType
  occurred_at: string | null
  actor_name: string
  payload: Record<string, unknown>
  reasons: string[]
  read_at: string | null
}

export interface FeedUnreadCounts {
  all: number
  assignee: number
  reporter: number
  mention: number
}

export interface Feed {
  items: FeedItem[]
  unread_counts: FeedUnreadCounts
}

/** GET feed/ — full recompute, no server cursor (GDK-802 premise). */
export async function feed(
  ctx: ApiContext,
  focus: FeedFocus = 'all',
  limit?: number,
  signal?: AbortSignal,
): Promise<Feed> {
  let path = `issues/feed/?focus=${encodeURIComponent(focus)}`
  if (limit !== undefined) path += `&limit=${limit}`
  const res = await request(ctx, path, { signal })
  if (!res.ok) throw await toApiError(res)
  return parseJson<Feed>(res, (b) => {
    expectArray(b, 'items')
  })
}

export type MarkFeedReadPayload = {
  event_ids?: string[]
  issue_keys?: string[]
  all?: boolean
}

export interface FeedReadResult {
  updated: number
  unread_counts: FeedUnreadCounts
}

/** POST feed/read/ — same payload shape as the web client. */
export async function markFeedRead(
  ctx: ApiContext,
  payload: MarkFeedReadPayload,
  signal?: AbortSignal,
): Promise<FeedReadResult> {
  const res = await request(ctx, 'issues/feed/read/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    signal,
  })
  if (!res.ok) throw await toApiError(res)
  return parseJson<FeedReadResult>(res, (b) => {
    expectField(b, 'unread_counts')
  })
}

/**
 * The 1차 queue: unresolved rows, priority_rank ascending with unset (0)
 * last, updated_at descending as the tiebreak. `assignee` narrowing waits
 * for me() (needs the account id — display names are not a key).
 */
export function queueRows(issues: QueueRow[], limit = 50): QueueRow[] {
  const open = issues.filter((i) => i.status_category !== 'done')
  const rank = (i: QueueRow): number => (Number.isFinite(i.priority_rank) && i.priority_rank > 0 ? i.priority_rank : Number.MAX_SAFE_INTEGER)
  const updated = (i: QueueRow): number => {
    if (!i.updated_at) return 0
    const t = new Date(i.updated_at).getTime()
    return Number.isNaN(t) ? 0 : t
  }
  return open
    .map((i) => ({ i, r: rank(i), u: updated(i) }))
    .sort((a, b) => (a.r - b.r) || (b.u - a.u))
    .map(({ i }) => i)
    .slice(0, limit)
}

/*
 * Ink mapping — copied semantics, not new colors: web/src/lib/format.ts
 * categoryMetaOf (status) and PRIORITY_COLOR (priority). Keep in sync with
 * those two tables; they are the owners.
 */
export function categoryInk(cat: string): string {
  if (cat === 'new') return 'var(--color-status-new)'
  if (cat === 'inprogress') return 'var(--color-status-inprogress)'
  if (cat === 'done') return 'var(--color-status-done)'
  return 'var(--color-text-muted)'
}

const PRIORITY_INK = [
  'var(--color-border-strong)', // 0 unset / missing rank
  'var(--color-status-reopen)', // rank 1
  'var(--color-status-inprogress)', // rank 2
  'var(--color-status-new)', // rank 3
  'var(--color-text-secondary)', // rank 4
  'var(--color-text-muted)', // rank 5+
] as const

export function priorityInk(rank: number): string {
  if (!Number.isFinite(rank) || rank < 1) return PRIORITY_INK[0]
  return PRIORITY_INK[rank >= 5 ? 5 : rank]
}
