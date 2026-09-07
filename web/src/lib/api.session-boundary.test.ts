/*
 * GDK-1537: the session strip's boundary must survive a conditional GET.
 *
 * It rode the bootstrap body, and the body is exactly what a 304 does not
 * have. A tab holding an unchanged issue set sent If-None-Match, got a
 * bodiless 304, and learned nothing — so the strip went missing on the one
 * morning nothing had changed overnight. A warm tab never even reaches that
 * path: it hydrates from IndexedDB and syncs by delta from then on, and the
 * delta body carries no boundary by design (a field there would move the
 * boundary under a long-lived tab).
 *
 * The closure is one response header on both endpoints and both status codes.
 * This file owns the api layer's half — that getBootstrap surfaces it on 200
 * and on 304, and that getDelta surfaces it at all. The store's claim-once
 * rule is asserted in e2e/session-strip.spec.ts case (f), against the real
 * server.
 *
 * Node env, fetch stubbed per case (api.reachability.test.ts's shape);
 * config() falls back to DEFAULTS so nothing is dialed.
 */
import { afterEach, describe, expect, test, vi } from 'vitest'
import { getBootstrap, getDelta, SESSION_BOUNDARY_HEADER } from './api'

const BOUNDARY = '2026-09-06T22:00:00.000Z'

/** A bootstrap-shaped 200 with whatever headers the case is about. */
function ok(headers: Record<string, string>): () => Promise<Response> {
  return vi.fn(async () =>
    new Response('{"issues":[],"members":[],"server_time":"2026-09-07T00:00:00Z","sync_version":1}', {
      status: 200,
      headers: { 'Content-Type': 'application/json', ...headers },
    }),
  )
}

/** A 304 — no body at all, which is the whole point. */
function notModified(headers: Record<string, string>): () => Promise<Response> {
  return vi.fn(async () => new Response(null, { status: 304, headers }))
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('the boundary rides a header, so a 304 still carries it', () => {
  test('a 304 surfaces the boundary even though it has no body', async () => {
    vi.stubGlobal('fetch', notModified({ [SESSION_BOUNDARY_HEADER]: BOUNDARY }))
    const res = await getBootstrap('"sv-1"')
    expect(res.status).toBe('not_modified')
    expect(res.sessionBoundary).toBe(BOUNDARY)
  })

  test('a 200 surfaces the same header — one seat, not two', async () => {
    vi.stubGlobal('fetch', ok({ ETag: '"sv-2"', [SESSION_BOUNDARY_HEADER]: BOUNDARY }))
    const res = await getBootstrap()
    expect(res.status).toBe('ok')
    if (res.status !== 'ok') return
    expect(res.sessionBoundary).toBe(BOUNDARY)
    expect(res.etag).toBe('"sv-2"')
  })

  test('no header → null, on both codes: a server older than this client', async () => {
    vi.stubGlobal('fetch', notModified({}))
    expect((await getBootstrap('"sv-1"')).sessionBoundary).toBeNull()
    vi.stubGlobal('fetch', ok({ ETag: '"sv-2"' }))
    expect((await getBootstrap()).sessionBoundary).toBeNull()
  })

  test('the delta carries it too — a warm tab never asks bootstrap again', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response('{"upserted":[],"deleted_keys":[],"server_time":"2026-09-07T00:00:00Z"}', {
          status: 200,
          headers: { 'Content-Type': 'application/json', [SESSION_BOUNDARY_HEADER]: BOUNDARY },
        }),
      ),
    )
    const res = await getDelta('2026-09-06T00:00:00Z')
    expect(res.sessionBoundary).toBe(BOUNDARY)
    expect(res.data.server_time).toBe('2026-09-07T00:00:00Z')
  })

  test('a delta that fails still throws — the header did not soften the errors', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 500 })))
    await expect(getDelta('2026-09-06T00:00:00Z')).rejects.toMatchObject({ status: 500 })
  })
})
