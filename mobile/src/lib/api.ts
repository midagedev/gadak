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
  constructor(
    public code: string,
    public status: number,
  ) {
    super(code)
  }
}

/** Joins the API base with a relative path — exported for tests. */
export function apiUrl(endpoint: string, path: string, dev: boolean = IS_DEV): string {
  const base = dev ? '' : endpoint.replace(/\/+$/, '')
  return `${base}/api/v1/${path}`
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
    })
  } catch {
    throw new ApiError('network', 0)
  }
  if (res.status === 304) return { status: 304, etag: res.headers.get('ETag'), body: null }
  if (!res.ok) {
    let code = 'internal_error'
    try {
      const doc = (await res.json()) as { error?: unknown }
      if (typeof doc.error === 'string' && doc.error !== '') code = doc.error
    } catch {
      // Non-JSON error body (a proxy page, an empty reply): keep the generic code.
    }
    throw new ApiError(code, res.status)
  }
  let body: T
  try {
    body = (await res.json()) as T
  } catch {
    throw new ApiError('bad_response', res.status)
  }
  return { status: res.status, etag: res.headers.get('ETag'), body }
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
