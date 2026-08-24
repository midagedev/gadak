/*
 * Transport tests for the api client. These pin the three contracts the
 * screens lean on: which fetch implementation runs where (window fetch in
 * DEV/vitest, plugin-http in the packaged webview), the Bearer attachment,
 * and the server error shape ({"error": code}) surfacing as ApiError.code
 * so a screen can teach the next move instead of showing a bare status.
 *
 * The offer golden vectors (offer.test.ts) are untouched by this file.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { tauriFetch } = vi.hoisted(() => ({ tauriFetch: vi.fn() }))
vi.mock('@tauri-apps/plugin-http', () => ({ fetch: tauriFetch }))

const { invoke } = vi.hoisted(() => ({ invoke: vi.fn() }))
vi.mock('@tauri-apps/api/core', () => ({ invoke }))

import {
  ApiError,
  bootstrap,
  delta,
  feed,
  markFeedRead,
  me,
  postComment,
  postTransition,
  search,
  transitions,
  type ApiContext,
} from './api'

const CTX: ApiContext = { endpoint: 'https://home.example.ts.net/', token: 'tok-1' }
const NO_TOKEN: ApiContext = { endpoint: 'https://home.example.ts.net', token: '' }

const jsonResponse = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })

// A Response body is single-use: hand each test a fresh one.
const okBootstrap = (): Response =>
  jsonResponse({
    issues: [],
    sync_version: 1,
    server_time: '2026-08-25T00:00:00Z',
  })

let windowFetch: ReturnType<typeof vi.fn>

beforeEach(() => {
  windowFetch = vi.fn()
  vi.stubGlobal('fetch', windowFetch)
  tauriFetch.mockReset()
  invoke.mockReset()
  // vitest runs with DEV=true (dev transform); the packaged-app branches
  // below need the production flag. Assignable in the test runtime.
  ;(import.meta.env as Record<string, unknown>).DEV = false
})

afterEach(() => {
  delete (globalThis as Record<string, unknown>)._TAURI_INTERNALS_
  vi.unstubAllGlobals()
})

describe('transport pick', () => {
  it('uses window fetch outside the Tauri webview', async () => {
    windowFetch.mockResolvedValue(okBootstrap())
    await bootstrap(CTX)
    expect(windowFetch).toHaveBeenCalledOnce()
    expect(tauriFetch).not.toHaveBeenCalled()
  })

  it('uses plugin-http inside the Tauri webview', async () => {
    ;(globalThis as Record<string, unknown>)._TAURI_INTERNALS_ = {}
    tauriFetch.mockResolvedValue(okBootstrap())
    await bootstrap(CTX)
    expect(tauriFetch).toHaveBeenCalledOnce()
    expect(windowFetch).not.toHaveBeenCalled()
  })

  it('builds the URL from the endpoint (trailing slash tolerated)', async () => {
    windowFetch.mockResolvedValue(okBootstrap())
    await bootstrap(CTX)
    const [url] = windowFetch.mock.calls[0] as [string]
    expect(url).toBe('https://home.example.ts.net/api/v1/issues/bootstrap/')
  })

  it('DEV (vite proxy) keeps the relative URL even inside a Tauri dev window', async () => {
    ;(import.meta.env as Record<string, unknown>).DEV = true
    ;(globalThis as Record<string, unknown>)._TAURI_INTERNALS_ = {}
    windowFetch.mockResolvedValue(okBootstrap())
    await bootstrap(CTX)
    const [url] = windowFetch.mock.calls[0] as [string]
    expect(url).toBe('/api/v1/issues/bootstrap/')
    expect(windowFetch).toHaveBeenCalledOnce()
    expect(tauriFetch).not.toHaveBeenCalled()
  })
})

describe('Bearer attachment', () => {
  it('attaches Authorization when a token exists', async () => {
    windowFetch.mockResolvedValue(okBootstrap())
    await bootstrap(CTX)
    const init = windowFetch.mock.calls[0][1] as RequestInit
    expect(new Headers(init.headers).get('Authorization')).toBe('Bearer tok-1')
  })

  it('sends no Authorization header when unpaired', async () => {
    windowFetch.mockResolvedValue(okBootstrap())
    await bootstrap(NO_TOKEN)
    const init = windowFetch.mock.calls[0][1] as RequestInit
    expect(new Headers(init.headers).get('Authorization')).toBeNull()
  })
})

describe('error shape', () => {
  it('exposes the server error code and status on ApiError', async () => {
    windowFetch.mockResolvedValue(jsonResponse({ error: 'forbidden_host' }, 403))
    const err = await bootstrap(CTX).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    const apiErr = err as ApiError
    expect(apiErr.code).toBe('forbidden_host')
    expect(apiErr.status).toBe(403)
    expect(apiErr.message).toBe('http 403 forbidden_host')
  })

  it('keeps pairing_rejected distinguishable from forbidden_host', async () => {
    windowFetch.mockResolvedValue(jsonResponse({ error: 'pairing_rejected' }, 401))
    const err = (await bootstrap(CTX).catch((e: unknown) => e)) as ApiError
    expect(err.code).toBe('pairing_rejected')
    expect(err.status).toBe(401)
  })

  it('falls back to status-only when the body is not {"error": …}', async () => {
    windowFetch.mockResolvedValue(new Response('gateway junk', { status: 502 }))
    const err = (await bootstrap(CTX).catch((e: unknown) => e)) as ApiError
    expect(err.code).toBeUndefined()
    expect(err.status).toBe(502)
  })

  it('maps a failed connection to network', async () => {
    windowFetch.mockRejectedValue(new TypeError('fetch failed'))
    const err = (await bootstrap(CTX).catch((e: unknown) => e)) as ApiError
    expect(err.message).toBe('network')
    expect(err.status).toBeUndefined()
  })

  it('rethrows the abort error instead of masking it as network', async () => {
    const abort = new DOMException('aborted', 'AbortError')
    windowFetch.mockRejectedValue(abort)
    const err = await bootstrap(CTX, undefined, signalOf(abort)).catch((e: unknown) => e)
    expect(err).toBe(abort)
  })

  it('rejects a 200 whose body misses the expected field (shape)', async () => {
    windowFetch.mockResolvedValue(jsonResponse({ nope: true }))
    const err = (await bootstrap(CTX).catch((e: unknown) => e)) as ApiError
    expect(err.message).toBe('shape')
  })

  it('rejects a 200 whose body is not JSON (shape)', async () => {
    windowFetch.mockResolvedValue(new Response('<html>', { status: 200 }))
    const err = (await bootstrap(CTX).catch((e: unknown) => e)) as ApiError
    expect(err.message).toBe('shape')
  })
})

describe('bootstrap etag', () => {
  it('returns not_modified on 304 without parsing a body', async () => {
    windowFetch.mockResolvedValue(new Response(null, { status: 304 }))
    const res = await bootstrap(CTX, '"sv-1"')
    expect(res).toEqual({ status: 'not_modified' })
  })

  it('echoes the request etag as If-None-Match and returns the response etag', async () => {
    windowFetch.mockResolvedValue(
      new Response(JSON.stringify({ issues: [], sync_version: 2, server_time: 't' }), {
        status: 200,
        headers: { ETag: '"sv-2"' },
      }),
    )
    const res = await bootstrap(CTX, '"sv-1"')
    const init = windowFetch.mock.calls[0][1] as RequestInit
    expect(new Headers(init.headers).get('If-None-Match')).toBe('"sv-1"')
    expect(res.status).toBe('ok')
    if (res.status === 'ok') expect(res.etag).toBe('"sv-2"')
  })
})

describe('endpoint wrappers', () => {
  it('me() hits auth/me/', async () => {
    windowFetch.mockResolvedValue(jsonResponse({ account_id: null, email: '', name: '' }))
    await me(CTX)
    expect(windowFetch.mock.calls[0][0]).toBe('https://home.example.ts.net/api/v1/auth/me/')
  })

  it('delta() encodes since and mv', async () => {
    windowFetch.mockResolvedValue(
      jsonResponse({ server_time: 't', upserted: [], deleted_keys: [] }),
    )
    await delta(CTX, '2026-08-25T00:00:00Z', 'mv-7')
    expect(windowFetch.mock.calls[0][0]).toBe(
      'https://home.example.ts.net/api/v1/issues/delta/?since=2026-08-25T00%3A00%3A00Z&mv=mv-7',
    )
  })

  it('postComment() posts {text} as JSON', async () => {
    windowFetch.mockResolvedValue(
      jsonResponse({ issue: {}, comment: { comment_id: '1', author: null, body: 'b', created_at: null } }),
    )
    await postComment(CTX, 'STD-1', 'hello')
    const [url, init] = windowFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('https://home.example.ts.net/api/v1/issues/STD-1/comment/')
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json')
    expect(init.body).toBe(JSON.stringify({ text: 'hello' }))
  })

  it('search() encodes q and limit', async () => {
    windowFetch.mockResolvedValue(jsonResponse({ keys: [], total: 0 }))
    await search(CTX, 'flow probe', 20)
    expect(windowFetch.mock.calls[0][0]).toBe(
      'https://home.example.ts.net/api/v1/issues/search/?q=flow%20probe&limit=20',
    )
  })

  it('transitions() hits the transitions route', async () => {
    windowFetch.mockResolvedValue(jsonResponse({ transitions: [] }))
    await transitions(CTX, 'STD-1')
    expect(windowFetch.mock.calls[0][0]).toBe(
      'https://home.example.ts.net/api/v1/issues/STD-1/transitions/',
    )
  })

  it('postTransition() posts {transition_id}, never the display name', async () => {
    windowFetch.mockResolvedValue(jsonResponse({ issue: {} }))
    await postTransition(CTX, 'STD-1', '10003')
    const init = windowFetch.mock.calls[0][1] as RequestInit
    expect(init.body).toBe(JSON.stringify({ transition_id: '10003' }))
  })

  it('feed() sends focus and limit', async () => {
    windowFetch.mockResolvedValue(
      jsonResponse({ items: [], unread_counts: { all: 0, assignee: 0, reporter: 0, mention: 0 } }),
    )
    await feed(CTX, 'mention', 20)
    expect(windowFetch.mock.calls[0][0]).toBe(
      'https://home.example.ts.net/api/v1/issues/feed/?focus=mention&limit=20',
    )
  })

  it('markFeedRead() posts the web-parity payload', async () => {
    windowFetch.mockResolvedValue(
      jsonResponse({ updated: 2, unread_counts: { all: 0, assignee: 0, reporter: 0, mention: 0 } }),
    )
    await markFeedRead(CTX, { event_ids: ['e1', 'e2'] })
    const [url, init] = windowFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('https://home.example.ts.net/api/v1/issues/feed/read/')
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ event_ids: ['e1', 'e2'] }))
  })
})

/** An already-aborted-style signal: aborted flag true so request() rethrows. */
function signalOf(err: DOMException): AbortSignal {
  const c = new AbortController()
  c.abort(err)
  return c.signal
}
