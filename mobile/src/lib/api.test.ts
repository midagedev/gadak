import { describe, it, expect } from 'vitest'
import {
  apiUrl,
  absoluteApiUrl,
  apiHeaders,
  request,
  setApiActorName,
  ApiError,
  errorMessage,
  isPairingDead,
  type FetchLike,
} from './api'

function fakeFetch(status: number, body: unknown, headers: Record<string, string> = {}): {
  fn: FetchLike
  calls: { url: string; init: RequestInit }[]
} {
  const calls: { url: string; init: RequestInit }[] = []
  const fn: FetchLike = async (url, init) => {
    calls.push({ url, init })
    return new Response(body === null ? null : JSON.stringify(body), { status, headers })
  }
  return { fn, calls }
}

describe('apiUrl', () => {
  it('dev rides the same-origin proxy regardless of endpoint', () => {
    expect(apiUrl('https://home.example.ts.net', 'issues/bootstrap/', true)).toBe('/api/v1/issues/bootstrap/')
  })
  it('packaged joins the endpoint, trimming trailing slashes', () => {
    expect(apiUrl('https://home.example.ts.net/', 'auth/me/', false)).toBe(
      'https://home.example.ts.net/api/v1/auth/me/',
    )
  })
})

describe('absoluteApiUrl', () => {
  // GDK-1503: a file chip's tap copies this string. It must resolve where it
  // is pasted, which a bare path does not.
  it('dev resolves the proxy path against the page origin', () => {
    expect(
      absoluteApiUrl('issues/STD-1/attachments/9/content/', {
        endpoint: 'https://home.example.ts.net',
        dev: true,
        origin: 'http://127.0.0.1:5182',
      }),
    ).toBe('http://127.0.0.1:5182/api/v1/issues/STD-1/attachments/9/content/')
  })
  it('packaged keeps the paired endpoint apiUrl joined', () => {
    expect(
      absoluteApiUrl('issues/STD-1/attachments/9/content/', {
        endpoint: 'https://home.example.ts.net/',
        dev: false,
        origin: 'http://127.0.0.1:5182',
      }),
    ).toBe('https://home.example.ts.net/api/v1/issues/STD-1/attachments/9/content/')
  })
  it('never emits a doubled slash when the origin carries one', () => {
    expect(
      absoluteApiUrl('auth/me/', { endpoint: '', dev: true, origin: 'http://127.0.0.1:5182/' }),
    ).toBe('http://127.0.0.1:5182/api/v1/auth/me/')
  })
})

describe('apiHeaders', () => {
  it('sends Bearer only when a token exists', () => {
    expect(apiHeaders(null, false)).toEqual({})
    expect(apiHeaders('tok', false)).toEqual({ Authorization: 'Bearer tok' })
  })
  it('marks JSON only when a body rides along', () => {
    expect(apiHeaders(null, true)['Content-Type']).toBe('application/json')
  })
  // GDK-1973: the declared name rides every dial once set, percent-encoded
  // (header values are bytes, the name is text), and absent when unset —
  // the server's workspace-default attribution stands.
  it('carries the declared name, encoded, only when non-empty', () => {
    expect(apiHeaders(null, false, 'Dana')['X-Gadak-Actor-Name']).toBe('Dana')
    expect(apiHeaders(null, false, 'Kim Cheolsu')['X-Gadak-Actor-Name']).toBe('Kim%20Cheolsu')
    expect(apiHeaders(null, false, '김철수')['X-Gadak-Actor-Name']).toBe(
      encodeURIComponent('김철수'),
    )
  })
  it('sends no declared-name header for null, empty, and whitespace', () => {
    expect(apiHeaders(null, false, null)).toEqual({})
    expect(apiHeaders(null, false, '')).toEqual({})
    expect(apiHeaders(null, false, '   ')).toEqual({})
  })
})

describe('request', () => {
  const session = { endpoint: '', token: 'STD-token' }

  it('threads the pushed declared name onto the dial, and drops it when cleared', async () => {
    // GDK-1973: the store pushes the name through setApiActorName (it cannot
    // ride the session — the store owns persistence), and dial is what
    // carries it onto every request. Clearing with '' must remove it again.
    const { fn, calls } = fakeFetch(200, {})
    setApiActorName('Dana Kim')
    try {
      await request('auth/me/', { session, fetchFn: fn })
      expect((calls[0]!.init.headers as Record<string, string>)['X-Gadak-Actor-Name']).toBe(
        'Dana%20Kim',
      )
      await request('auth/me/', { session, fetchFn: fn })
      expect(calls).toHaveLength(2)
    } finally {
      setApiActorName('')
    }
    await request('auth/me/', { session, fetchFn: fn })
    expect((calls[2]!.init.headers as Record<string, string>)['X-Gadak-Actor-Name']).toBeUndefined()
  })

  it('returns the envelope with the ETag on 200', async () => {
    const { fn } = fakeFetch(200, { issues: [] }, { ETag: '"sv-7"' })
    const res = await request<{ issues: unknown[] }>('issues/bootstrap/', { session, fetchFn: fn })
    expect(res.status).toBe(200)
    expect(res.etag).toBe('"sv-7"')
    expect(res.body).toEqual({ issues: [] })
  })

  it('sends If-None-Match and passes 304 through with a null body', async () => {
    const { fn, calls } = fakeFetch(304, null)
    const res = await request('issues/bootstrap/', { session, etag: '"sv-7"', fetchFn: fn })
    expect((calls[0].init.headers as Record<string, string>)['If-None-Match']).toBe('"sv-7"')
    expect(res.status).toBe(304)
    expect(res.body).toBeNull()
  })

  it('maps {"error": code} bodies to ApiError(code)', async () => {
    const { fn } = fakeFetch(401, { error: 'pairing_rejected' })
    await expect(request('auth/me/', { session, fetchFn: fn })).rejects.toMatchObject({
      code: 'pairing_rejected',
      status: 401,
    })
  })

  it('keeps a generic code for non-JSON error bodies (never echoes them)', async () => {
    const calls: { url: string; init: RequestInit }[] = []
    const fn: FetchLike = async (url, init) => {
      calls.push({ url, init })
      return new Response('<html>proxy said no</html>', { status: 502 })
    }
    await expect(request('auth/me/', { session, fetchFn: fn })).rejects.toMatchObject({
      code: 'internal_error',
    })
  })

  it('wraps transport failure as ApiError(network)', async () => {
    const fn: FetchLike = async () => {
      throw new TypeError('Load failed')
    }
    await expect(request('auth/me/', { session, fetchFn: fn })).rejects.toMatchObject({ code: 'network' })
  })

  it('serializes POST bodies and bearer together', async () => {
    const { fn, calls } = fakeFetch(200, { changed: true })
    await request('issues/STD-1/transition/', {
      session,
      method: 'POST',
      body: { transition_id: '31' },
      fetchFn: fn,
    })
    const h = calls[0].init.headers as Record<string, string>
    expect(h['Authorization']).toBe('Bearer STD-token')
    expect(h['Content-Type']).toBe('application/json')
    expect(calls[0].init.body).toBe('{"transition_id":"31"}')
  })

  // GDK-1872 — the multipart branch. One dial, one refusal grammar: the only
  // thing a form body changes is the body and the absent Content-Type.
  it('hands a FormData body to the runtime untouched, with no Content-Type', async () => {
    const { fn, calls } = fakeFetch(200, { attachments: [], origin: 'built-in' })
    const form = new FormData()
    form.append('file', new File(['bytes'], 'field.jpg', { type: 'image/jpeg' }))
    await request('issues/NMB-1/attachments/', { session, method: 'POST', form, fetchFn: fn })
    const h = calls[0].init.headers as Record<string, string>
    // The boundary is the runtime's to generate (plugin-http index.js:66-75
    // copies the one its `new Request` stamped, but only if we declared none).
    expect(h['Content-Type']).toBeUndefined()
    expect(h['Authorization']).toBe('Bearer STD-token')
    expect(calls[0].init.body).toBe(form)
  })

  it('refuses a body and a form together before dialing', async () => {
    const { fn, calls } = fakeFetch(200, {})
    await expect(
      request('issues/NMB-1/attachments/', {
        session,
        method: 'POST',
        body: { text: 'x' },
        form: new FormData(),
        fetchFn: fn,
      }),
    ).rejects.toMatchObject({ code: 'bad_request', status: 0 })
    expect(calls).toHaveLength(0)
  })

  it('decodes an upload refusal by the code the server sent, not by status', async () => {
    const { fn } = fakeFetch(413, { error: 'too_large' })
    await expect(
      request('issues/NMB-1/attachments/', { session, method: 'POST', form: new FormData(), fetchFn: fn }),
    ).rejects.toMatchObject({ code: 'too_large', status: 413 })
  })

  it('carries the oversize answer this serve actually sends (400 file_required)', async () => {
    // write.go:693 caps the whole request body with MaxBytesReader, so an
    // overrun trips r.FormFile and answers 400 file_required (write.go:696) —
    // there is no 413 on this route. Pinned so part 2 writes copy for the
    // code that arrives, not the one the status suggests.
    const { fn } = fakeFetch(400, { error: 'file_required' })
    await expect(
      request('issues/NMB-1/attachments/', { session, method: 'POST', form: new FormData(), fetchFn: fn }),
    ).rejects.toMatchObject({ code: 'file_required', status: 400 })
  })

  it('surfaces write_applied_mirror_stale, the 502 that means the upload landed', async () => {
    const { fn } = fakeFetch(502, { error: 'write_applied_mirror_stale' })
    await expect(
      request('issues/NMB-1/attachments/', { session, method: 'POST', form: new FormData(), fetchFn: fn }),
    ).rejects.toMatchObject({ code: 'write_applied_mirror_stale', status: 502 })
  })

  it('carries the message field on 409 placeholder, and errorMessage never echoes it', async () => {
    // write.go:894 failMsg(409, "placeholder", …) — the one error body whose
    // text is meant for the person (the description editor shows it). It
    // rides ApiError.serverMessage, not the generic copy.
    const { fn } = fakeFetch(409, { error: 'placeholder', message: 'the table marker cannot return' })
    const err: unknown = await request('issues/STD-1/description/', {
      session,
      method: 'PUT',
      body: { description: 'x' },
      fetchFn: fn,
    }).then(
      () => undefined,
      (e) => e,
    )
    expect(err).toMatchObject({
      code: 'placeholder',
      status: 409,
      serverMessage: 'the table marker cannot return',
    })
    expect(errorMessage(err)).not.toContain('the table marker cannot return')
  })
})

describe('error copy', () => {
  it('never contains the server body or a token', () => {
    for (const code of ['pairing_rejected', 'forbidden_host', 'not_found', 'weird_new_code']) {
      const msg = errorMessage(new ApiError(code, 400))
      expect(msg).not.toContain('STD-token')
      expect(msg.length).toBeGreaterThan(10)
    }
  })
  it('classifies dead pairings', () => {
    expect(isPairingDead(new ApiError('pairing_rejected', 401))).toBe(true)
    expect(isPairingDead(new ApiError('network', 0))).toBe(false)
  })
})
