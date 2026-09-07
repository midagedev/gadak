// The one transport module (DESIGN.md §5). Two branches, split here and
// nowhere else:
//   dev      — the vite dev server proxies /api to the serve on
//              127.0.0.1:7899 (same origin, browser fetch). The endpoint in
//              the pairing meta is ignored; loopback needs no bearer but we
//              attach one when present, which the gate tolerates.
//   packaged — @tauri-apps/plugin-http (native reqwest: no Origin header,
//              no CORS) against the paired endpoint with the Keychain token.
//
// Error bodies are `{"error": code}`. The code is mapped to copy in
// errorMessage(); the raw body is never echoed into the UI, and the token
// never appears in an error, a log line, or a URL.

import { demoRequest, isDemoSession } from './demo'
import { inDialScope } from './dial-scope'
import { t } from './i18n'

const IS_DEV = import.meta.env.DEV

export interface ApiSession {
  endpoint: string
  token: string | null
}

let session: ApiSession = { endpoint: '', token: null }

export function configureApi(s: ApiSession): void {
  session = s
}

export class ApiError extends Error {
  /**
   * The `message` field of a `{"error": code, "message": text}` body, when
   * the server sent one. Only the placeholder refusal (409) carries text a
   * person should read; errorMessage() never echoes it, and the one caller
   * that shows it (the description editor) marks it as the server's words.
   */
  constructor(
    public code: string,
    public status: number,
    public serverMessage: string | null = null,
  ) {
    super(code)
  }
}

/**
 * The versioned prefix every request path hangs off — the same one the
 * server builds its attachment content_urls on (internal/server). Exported
 * so AdfBody can turn a rendered attachment URL back into a requestBlob
 * path without a second literal.
 */
export const API_V1 = '/api/v1/'

/** Joins the API base with a relative path — exported for tests. */
export function apiUrl(endpoint: string, path: string, dev: boolean = IS_DEV): string {
  const base = dev ? '' : endpoint.replace(/\/+$/, '')
  return `${base}${API_V1}${path}`
}

/**
 * The same join, resolved so the result still means something outside this
 * WebView (GDK-1503). apiUrl() answers "what do I dial", and in dev that is
 * a bare path because the vite proxy is same-origin — but a *copied* link is
 * pasted somewhere else, where a path resolves against the wrong host or
 * nothing at all. Dev resolves it against the page origin (the proxy is the
 * serve), packaged against the paired endpoint apiUrl already joined.
 *
 * The session is module-private, which is why this lives here and not in the
 * one caller; endpoint/dev/origin are test seams, same as apiUrl's `dev`.
 */
export function absoluteApiUrl(
  path: string,
  opts: { endpoint?: string; dev?: boolean; origin?: string } = {},
): string {
  const dev = opts.dev ?? IS_DEV
  const url = apiUrl(opts.endpoint ?? session.endpoint, path, dev)
  if (!url.startsWith('/')) return url
  const origin = opts.origin ?? (typeof location === 'undefined' ? '' : location.origin)
  return `${origin.replace(/\/+$/, '')}${url}`
}

/** Builds request headers — exported for tests. Bearer only when a token exists. */
export function apiHeaders(token: string | null, hasBody: boolean): Record<string, string> {
  const h: Record<string, string> = {}
  if (token) h['Authorization'] = `Bearer ${token}`
  if (hasBody) h['Content-Type'] = 'application/json'
  return h
}

export type FetchLike = (url: string, init: RequestInit) => Promise<Response>

async function pickFetch(): Promise<FetchLike> {
  if (IS_DEV) return (url, init) => window.fetch(url, init)
  const mod = await import('@tauri-apps/plugin-http')
  return mod.fetch as FetchLike
}

export interface Envelope<T> {
  status: number
  etag: string | null
  body: T | null
}

interface RequestOpts {
  method?: string
  body?: unknown
  etag?: string | null
  /** Overrides the configured session — the pairing probe uses this. */
  session?: ApiSession
  /** Test seam: same role as apiUrl's `dev` parameter. */
  dev?: boolean
  /** Test seam. */
  fetchFn?: FetchLike
  /** Caller-initiated abort (the assignee search's debounced keystrokes). */
  signal?: AbortSignal
}

/**
 * Shared transport core: URL, scope refusal, headers, dial, error mapping.
 * Everything except the body parse, which differs by kind — request() reads
 * JSON, requestBlob() reads bytes. Throwing is the same in both: see request.
 */
async function dial(path: string, opts: RequestOpts): Promise<Response> {
  const s = opts.session ?? session
  const dev = opts.dev ?? IS_DEV
  const url = apiUrl(s.endpoint, path, dev)
  if (!dev && !inDialScope(url)) {
    throw new ApiError('endpoint_out_of_scope', 0)
  }
  const doFetch = opts.fetchFn ?? (await pickFetch())
  const headers = apiHeaders(s.token, opts.body !== undefined)
  if (opts.etag) headers['If-None-Match'] = opts.etag
  let res: Response
  try {
    res = await doFetch(url, {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    })
  } catch (err) {
    // A caller-initiated abort is not a network verdict; the caller already
    // knows (it aborted) and drops the result by sequence, not by error.
    if (opts.signal?.aborted) throw err
    throw new ApiError('network', 0)
  }
  if (res.status !== 304 && !res.ok) {
    let code = 'internal_error'
    let serverMessage: string | null = null
    try {
      const doc = (await res.json()) as { error?: unknown; message?: unknown }
      if (typeof doc.error === 'string' && doc.error !== '') code = doc.error
      if (typeof doc.message === 'string' && doc.message !== '') serverMessage = doc.message
    } catch {
      // Non-JSON error body (a proxy page, an empty reply): keep the generic code.
    }
    throw new ApiError(code, res.status, serverMessage)
  }
  return res
}

/**
 * Core request. Throws ApiError('endpoint_out_of_scope') before dialing
 * when the packaged session endpoint sits outside the `http:default`
 * capability scope — the refusal the plugin would make anyway, named
 * instead of swallowed (GDK-1048: this used to surface as 'network', with
 * zero requests in the serve log to tell the two apart). ApiError('network')
 * when the server is unreachable, ApiError(code) for `{"error": code}`
 * bodies; returns the envelope otherwise (304 comes back with body null).
 */
export async function request<T>(path: string, opts: RequestOpts = {}): Promise<Envelope<T>> {
  // The demo session's whole transport branch (GDK-1051): demo.ts owns it.
  if (isDemoSession()) return demoRequest<T>(path, opts)
  const res = await dial(path, opts)
  if (res.status === 304) return { status: 304, etag: res.headers.get('ETag'), body: null }
  let body: T
  try {
    body = (await res.json()) as T
  } catch {
    throw new ApiError('bad_response', res.status)
  }
  return { status: res.status, etag: res.headers.get('ETag'), body }
}

/**
 * Attachment bytes through the same road as JSON (GDK-1497): one dial, so
 * scope refusals and error codes are identical for both kinds. The demo
 * session has no attachment bytes — its bundle is JSON only — so it refuses
 * rather than pretending.
 */
export async function requestBlob(path: string, opts: RequestOpts = {}): Promise<Blob> {
  if (isDemoSession()) throw new ApiError('not_found', 404)
  const res = await dial(path, opts)
  try {
    return await res.blob()
  } catch {
    throw new ApiError('network', 0)
  }
}

/** Server codes → copy. Never includes server text or the token. */
export function errorMessage(err: unknown): string {
  const code = err instanceof ApiError ? err.code : 'network'
  switch (code) {
    case 'network':
      return 'Cannot reach the server.'
    case 'endpoint_out_of_scope':
      // The only localized sentence so far: it names a cause the generic
      // 'network' line cannot ("the server is down" vs "this app never
      // sent the request"), so it rides the shared catalog (GDK-1048).
      return t('app.endpointScope')
    case 'pairing_rejected':
      return 'Pairing was refused. Mint a new offer on the desktop and pair again.'
    case 'forbidden_host':
    case 'scope_rejected':
      return 'This pairing cannot read the mirror. Pair again with a serve-scope offer.'
    case 'not_found':
      return 'Not found on the server.'
    case 'credential_required':
      return 'This serve has no origin credential, so writes are off. Add one on the desktop.'
    case 'bad_response':
      return 'The server sent an unreadable reply.'
    default:
      return 'The server refused this request.'
  }
}

/** True when the failure means the pairing itself is dead (re-pair needed). */
export function isPairingDead(err: unknown): boolean {
  return (
    err instanceof ApiError &&
    (err.code === 'pairing_rejected' || err.code === 'scope_rejected' || err.code === 'forbidden_host')
  )
}
