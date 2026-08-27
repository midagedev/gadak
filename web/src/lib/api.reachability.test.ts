/*
 * GDK-1054: an ANSWERED 5xx is a down signal.
 *
 * GDK-477 wired reachability to network throws only, so a server answering
 * 503 to everything rendered as silence (empty copies, no strip). The branch
 * under test lives in raw()/dashJSON: status >= 500 fires the same handler a
 * connection failure does. 4xx never does — those are answers, not outages.
 *
 * Node env, fetch stubbed per case; config() falls back to DEFAULTS so the
 * URL is never dialed.
 */
import { afterEach, describe, expect, test, vi } from 'vitest'
import { getPages, setNetworkDownHandler } from './api'

const respond = (status: number) =>
  vi.fn(async () =>
    new Response('{"error":"service unavailable"}', {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GDK-1054: the api layer reports answered 5xx as down', () => {
  test('a 503 response fires the network-down handler once', async () => {
    const down = vi.fn()
    setNetworkDownHandler(down)
    vi.stubGlobal('fetch', respond(503))
    await expect(getPages()).rejects.toMatchObject({ status: 503 })
    expect(down).toHaveBeenCalledTimes(1)
  })

  test('a 502 fires it too — the class is >= 500, not one status', async () => {
    const down = vi.fn()
    setNetworkDownHandler(down)
    vi.stubGlobal('fetch', respond(502))
    await expect(getPages()).rejects.toMatchObject({ status: 502 })
    expect(down).toHaveBeenCalledTimes(1)
  })

  test('a 404 does not: 4xx is an answer, not an outage', async () => {
    const down = vi.fn()
    setNetworkDownHandler(down)
    vi.stubGlobal('fetch', respond(404))
    await expect(getPages()).rejects.toMatchObject({ status: 404 })
    expect(down).not.toHaveBeenCalled()
  })

  test('a 200 does not', async () => {
    const down = vi.fn()
    setNetworkDownHandler(down)
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('{"pages":[]}', { headers: { 'Content-Type': 'application/json' } })),
    )
    await getPages()
    expect(down).not.toHaveBeenCalled()
  })

  test('a connection throw still fires it (GDK-477 pin)', async () => {
    const down = vi.fn()
    setNetworkDownHandler(down)
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('fetch failed')
      }),
    )
    await expect(getPages()).rejects.toThrow('fetch failed')
    expect(down).toHaveBeenCalledTimes(1)
  })
})
