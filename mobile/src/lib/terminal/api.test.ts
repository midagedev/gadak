import { describe, expect, it } from 'vitest'
import { ApiError, type FetchLike } from '../api'
import {
  createShellSession,
  deleteShellSession,
  listShellSessions,
  probeShellPairing,
  TERMINAL_CURSOR_BLINK_FALLBACK,
  TERMINAL_SCROLLBACK_FALLBACK,
} from './api'

const TOKEN = '<terminal-token>'
const session = { endpoint: 'https://home.example.ts.net', token: TOKEN }

function fakeFetch(
  status: number,
  body: unknown,
): { fn: FetchLike; calls: { url: string; init: RequestInit }[] } {
  const calls: { url: string; init: RequestInit }[] = []
  const fn: FetchLike = async (url, init) => {
    calls.push({ url, init })
    if (status === 204) return new Response(null, { status })
    return new Response(body === null ? null : JSON.stringify(body), { status })
  }
  return { fn, calls }
}

function bearer(init: RequestInit): string | undefined {
  return (init.headers as Record<string, string> | undefined)?.Authorization
}

describe('createShellSession', () => {
  it('POSTs geometry with the terminal Bearer, never the token in the URL', async () => {
    const { fn, calls } = fakeFetch(200, { id: 'sess-1', cols: 80, rows: 24 })
    const doc = await createShellSession(80, 24, session, fn)
    // No behavior fields in this response, so normalize fills the
    // fallbacks (GDK-896 R3) — the deeper cases live in session-doc.test.ts.
    expect(doc).toEqual({
      id: 'sess-1',
      cols: 80,
      rows: 24,
      scrollback: TERMINAL_SCROLLBACK_FALLBACK,
      cursorBlink: TERMINAL_CURSOR_BLINK_FALLBACK,
    })
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/')
    expect(calls[0].url).not.toContain(TOKEN)
    expect(calls[0].init.method).toBe('POST')
    expect(calls[0].init.body).toBe('{"cols":80,"rows":24}')
    expect(bearer(calls[0].init)).toBe(`Bearer ${TOKEN}`)
  })
})

describe('listShellSessions', () => {
  it('returns the sessions array and treats a missing field as empty', async () => {
    const listed = fakeFetch(200, {
      sessions: [{ id: 'sess-1', cols: 80, rows: 24, pid: 9 }],
    })
    expect(await listShellSessions(session, listed.fn)).toEqual([
      { id: 'sess-1', cols: 80, rows: 24, pid: 9 },
    ])
    const empty = fakeFetch(200, {})
    expect(await listShellSessions(session, empty.fn)).toEqual([])
  })

  it('percent-encodes the id on delete', async () => {
    const { fn, calls } = fakeFetch(204, null)
    await deleteShellSession('sess/a b', session, fn)
    expect(calls[0].init.method).toBe('DELETE')
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/sess%2Fa%20b/')
    expect(calls[0].url).not.toContain(TOKEN)
  })
})

describe('deleteShellSession', () => {
  it('treats 204 No Content as success', async () => {
    const { fn } = fakeFetch(204, null)
    await expect(deleteShellSession('sess-1', session, fn)).resolves.toBeUndefined()
  })

  it('still surfaces a real error body', async () => {
    const { fn } = fakeFetch(404, { error: 'not_found' })
    await expect(deleteShellSession('sess-1', session, fn)).rejects.toMatchObject({
      code: 'not_found',
      status: 404,
    })
  })
})

describe('probeShellPairing', () => {
  it('resolves when the gate admits the token', async () => {
    const { fn, calls } = fakeFetch(200, { sessions: [] })
    await expect(probeShellPairing(session.endpoint, TOKEN, fn)).resolves.toBeUndefined()
    expect(calls[0].url).toContain('/api/v1/terminal/sessions/')
    expect(calls[0].url).not.toContain(TOKEN)
    expect(bearer(calls[0].init)).toBe(`Bearer ${TOKEN}`)
  })

  it('maps each server error body to its ApiError code', async () => {
    const cases: { status: number; error: string }[] = [
      { status: 403, error: 'scope_rejected' },
      { status: 401, error: 'pairing_rejected' },
      { status: 403, error: 'forbidden_host' },
    ]
    for (const c of cases) {
      const { fn } = fakeFetch(c.status, { error: c.error })
      await expect(probeShellPairing(session.endpoint, TOKEN, fn)).rejects.toMatchObject({
        code: c.error,
        status: c.status,
      })
    }
  })

  it('wraps a transport failure as ApiError(network)', async () => {
    const fn: FetchLike = async () => {
      throw new TypeError('Load failed')
    }
    await expect(probeShellPairing(session.endpoint, TOKEN, fn)).rejects.toMatchObject({
      code: 'network',
      status: 0,
    })
    await expect(probeShellPairing(session.endpoint, TOKEN, fn)).rejects.toBeInstanceOf(ApiError)
  })

  it('never echoes the token in an error', async () => {
    const { fn } = fakeFetch(403, { error: 'scope_rejected' })
    try {
      await probeShellPairing(session.endpoint, TOKEN, fn)
      throw new Error('expected throw')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as Error).message).not.toContain(TOKEN)
    }
  })
})
