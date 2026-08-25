import { afterEach, describe, expect, it } from 'vitest'
import {
  applyServerTextFrame,
  assertAllowedShellEndpoint,
  assertPairedWsUrl,
  nativeBinaryToBytes,
  openShellSocket,
  shellWsUrl,
  type BrowserSocket,
  type NativeConnect,
  type NativeWs,
  type NativeWsMessage,
  type ShellSocketOpts,
} from './transport'
import type { SocketHandlers } from '../../../../web/src/lib/terminal/protocol'

const ENDPOINT = 'https://home.example.ts.net'
const TOKEN = '<terminal-token>'

const KNOWN_DROPPED = [
  'slow_client',
  'token_revoked',
  'idle_timeout',
  'server_shutdown',
  'closed',
] as const

function recorder() {
  return {
    opens: 0,
    bytes: [] as Uint8Array[],
    exits: [] as number[],
    dropped: [] as string[],
    closes: [] as boolean[],
    handlers(): SocketHandlers {
      return {
        onOpen: () => {
          this.opens += 1
        },
        onBytes: (data: Uint8Array) => {
          this.bytes.push(data)
        },
        onExit: (code: number) => {
          this.exits.push(code)
        },
        onDropped: (reason) => {
          this.dropped.push(reason)
        },
        onClose: (neverOpened: boolean) => {
          this.closes.push(neverOpened)
        },
      }
    },
  }
}

class FakeWS implements BrowserSocket {
  static instances: FakeWS[] = []
  readyState = 0
  binaryType = ''
  readonly url: string
  readonly sent: (string | ArrayBufferLike | Blob | ArrayBufferView)[] = []
  private readonly listeners = new Map<string, Array<(ev: { data?: unknown }) => void>>()

  constructor(url: string) {
    this.url = url
    FakeWS.instances.push(this)
  }

  addEventListener(type: string, fn: (ev: { data?: unknown }) => void): void {
    const list = this.listeners.get(type) ?? []
    list.push(fn)
    this.listeners.set(type, list)
  }

  send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void {
    this.sent.push(data)
  }

  close(): void {
    this.readyState = 3
    this.emit('close')
  }

  open(): void {
    this.readyState = 1
    this.emit('open')
  }

  emit(type: string, ev: { data?: unknown } = {}): void {
    for (const fn of this.listeners.get(type) ?? []) fn(ev)
  }
}

class FakeNative implements NativeWs {
  readonly sent: Array<string | number[] | NativeWsMessage> = []
  disconnected = false
  private readonly listeners: Array<(msg: NativeWsMessage) => void> = []

  addListener(cb: (msg: NativeWsMessage) => void): () => void {
    this.listeners.push(cb)
    return () => {}
  }

  async send(message: string | number[] | NativeWsMessage): Promise<void> {
    this.sent.push(message)
  }

  async disconnect(): Promise<void> {
    this.disconnected = true
    this.deliver({ type: 'Close', data: { code: 1000, reason: '' } })
  }

  deliver(msg: NativeWsMessage): void {
    for (const l of this.listeners) l(msg)
  }
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

afterEach(() => {
  FakeWS.instances = []
})

describe('shellWsUrl', () => {
  it('maps http to ws and strips a trailing slash on the endpoint', () => {
    expect(shellWsUrl('http://home.example.ts.net/', 'sess-1', false)).toBe(
      'ws://home.example.ts.net/api/v1/terminal/sessions/sess-1/ws/',
    )
  })

  it('maps https to wss', () => {
    expect(shellWsUrl('https://home.example.ts.net', 'sess-1', false)).toBe(
      'wss://home.example.ts.net/api/v1/terminal/sessions/sess-1/ws/',
    )
  })

  it('percent-encodes an id that is not a path segment', () => {
    expect(shellWsUrl(ENDPOINT, 'sess/a b', false)).toBe(
      'wss://home.example.ts.net/api/v1/terminal/sessions/sess%2Fa%20b/ws/',
    )
  })

  it('keeps an explicit port', () => {
    expect(shellWsUrl('http://127.0.0.1:7899', 's', false)).toBe(
      'ws://127.0.0.1:7899/api/v1/terminal/sessions/s/ws/',
    )
  })
})

describe('assertPairedWsUrl', () => {
  const paired = 'wss://home.example.ts.net/api/v1/terminal/sessions/sess-1/ws/'

  it('accepts the same origin after http→ws mapping', () => {
    expect(() => assertPairedWsUrl(ENDPOINT, paired)).not.toThrow()
    expect(() =>
      assertPairedWsUrl(
        'http://home.example.ts.net',
        'ws://home.example.ts.net/api/v1/terminal/sessions/x/ws/',
      ),
    ).not.toThrow()
  })

  it('accepts a default https port written on only one side', () => {
    expect(() => assertPairedWsUrl('https://home.example.ts.net:443', paired)).not.toThrow()
  })

  it('rejects a different host', () => {
    expect(() =>
      assertPairedWsUrl(ENDPOINT, 'wss://other.example.ts.net/api/v1/terminal/sessions/s/ws/'),
    ).toThrow('shell websocket is not the paired origin')
  })

  it('rejects a different port', () => {
    expect(() =>
      assertPairedWsUrl(
        'https://home.example.ts.net:8443',
        'wss://home.example.ts.net/api/v1/terminal/sessions/s/ws/',
      ),
    ).toThrow('shell websocket is not the paired origin')
  })

  it('rejects a downgraded scheme', () => {
    expect(() =>
      assertPairedWsUrl(ENDPOINT, 'ws://home.example.ts.net/api/v1/terminal/sessions/s/ws/'),
    ).toThrow('shell websocket is not the paired origin')
  })

  it('rejects a host that merely starts with the endpoint host', () => {
    expect(() =>
      assertPairedWsUrl(
        'https://home.ts.net',
        'wss://home.ts.net.evil.example/api/v1/terminal/sessions/s/ws/',
      ),
    ).toThrow('shell websocket is not the paired origin')
  })

  it('does not put the token in the error', () => {
    try {
      assertPairedWsUrl(ENDPOINT, 'wss://evil.example/')
      throw new Error('expected throw')
    } catch (err) {
      expect((err as Error).message).not.toContain(TOKEN)
    }
  })
})

describe('applyServerTextFrame', () => {
  it('delivers exit with a code, defaulting a missing code to 0', () => {
    const rec = recorder()
    applyServerTextFrame('{"t":"exit","code":7}', rec.handlers())
    applyServerTextFrame('{"t":"exit"}', rec.handlers())
    expect(rec.exits).toEqual([7, 0])
  })

  it('delivers each known dropped reason', () => {
    const rec = recorder()
    const h = rec.handlers()
    for (const reason of KNOWN_DROPPED) {
      applyServerTextFrame(JSON.stringify({ t: 'dropped', reason }), h)
    }
    expect(rec.dropped).toEqual([...KNOWN_DROPPED])
  })

  it('coerces an unknown dropped reason to closed', () => {
    const rec = recorder()
    applyServerTextFrame('{"t":"dropped","reason":"not-a-reason"}', rec.handlers())
    expect(rec.dropped).toEqual(['closed'])
  })

  it('ignores malformed JSON instead of throwing', () => {
    const rec = recorder()
    expect(() => applyServerTextFrame('not-json', rec.handlers())).not.toThrow()
    expect(() => applyServerTextFrame('{', rec.handlers())).not.toThrow()
    expect(rec.exits).toEqual([])
    expect(rec.dropped).toEqual([])
    expect(rec.closes).toEqual([])
  })
})

describe('nativeBinaryToBytes', () => {
  it('converts a number array once, in one place', () => {
    const bytes = nativeBinaryToBytes([0x67, 0x64, 0x6b, 0xff])
    expect(bytes).toBeInstanceOf(Uint8Array)
    expect(Array.from(bytes!)).toEqual([0x67, 0x64, 0x6b, 0xff])
  })
})

describe('openShellSocket dev branch', () => {
  function openDev(id = 'sess-1') {
    const rec = recorder()
    const handle = openShellSocket(id, rec.handlers(), {
      endpoint: ENDPOINT,
      token: TOKEN,
      dev: true,
      webSocket: FakeWS,
    })
    const sock = FakeWS.instances[0]
    return { rec, handle, sock }
  }

  it('opens a browser WebSocket on the vite origin, not the paired host', () => {
    const { sock } = openDev()
    expect(sock.url).toBe('ws://localhost:5180/api/v1/terminal/sessions/sess-1/ws/')
    expect(sock.url).not.toContain(TOKEN)
  })

  it('reports onClose(true) when the socket dies before opening', () => {
    const { rec, sock } = openDev()
    sock.close()
    expect(rec.opens).toBe(0)
    expect(rec.closes).toEqual([true])
  })

  it('reports onClose(false) after it has opened', () => {
    const { rec, sock } = openDev()
    sock.open()
    sock.close()
    expect(rec.opens).toBe(1)
    expect(rec.closes).toEqual([false])
  })

  it('decodes text frames the same way as the web transport', () => {
    const { rec, sock } = openDev()
    sock.open()
    sock.emit('message', { data: '{"t":"exit","code":3}' })
    sock.emit('message', { data: '{"t":"dropped","reason":"idle_timeout"}' })
    sock.emit('message', { data: 'nope' })
    const payload = new Uint8Array([0x61, 0x0a])
    sock.emit('message', { data: payload.buffer })
    expect(rec.exits).toEqual([3])
    expect(rec.dropped).toEqual(['idle_timeout'])
    expect(Array.from(rec.bytes[0])).toEqual([0x61, 0x0a])
  })

  it('sends bytes and resize only while open', () => {
    const { handle, sock } = openDev()
    handle.send(new Uint8Array([0x61]))
    handle.resize(80, 24)
    expect(sock.sent).toHaveLength(0)
    sock.open()
    handle.send(new Uint8Array([0x61]))
    handle.resize(132, 43)
    expect(sock.sent[0]).toEqual(new Uint8Array([0x61]))
    expect(sock.sent[1]).toBe('{"t":"resize","cols":132,"rows":43}')
  })
})

describe('openShellSocket packaged branch', () => {
  function packagedOpts(connectNative: NativeConnect): ShellSocketOpts {
    return {
      endpoint: ENDPOINT,
      token: TOKEN,
      dev: false,
      webSocket: FakeWS,
      connectNative,
    }
  }

  it('dials the paired origin with a Bearer header and no token in the URL', async () => {
    const fake = new FakeNative()
    const calls: { url: string; headers?: [string, string][] }[] = []
    const rec = recorder()
    openShellSocket(
      'sess-1',
      rec.handlers(),
      packagedOpts(async (url, config) => {
        calls.push({ url, headers: config?.headers })
        return fake
      }),
    )
    await settle()
    expect(FakeWS.instances).toHaveLength(0)
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe(
      'wss://home.example.ts.net/api/v1/terminal/sessions/sess-1/ws/',
    )
    expect(calls[0].url).not.toContain(TOKEN)
    expect(calls[0].headers).toEqual([['Authorization', `Bearer ${TOKEN}`]])
    expect(rec.opens).toBe(1)
  })

  /*
   * 2026-08-25 — GDK-865 (lead review). Two guards run before connect, in
   * this order, and the messages differ because the questions differ: "may
   * this app dial that host at all" comes first, "is that host the one this
   * phone paired with" second. This asserted only the second message, on an
   * endpoint that fails the first — so it pinned the error text of a check
   * that would no longer be the one reached.
   */
  it('throws before connecting on an endpoint outside the dialling scope', () => {
    const rec = recorder()
    expect(() =>
      openShellSocket('sess-1', rec.handlers(), {
        endpoint: 'not a url',
        token: TOKEN,
        dev: false,
        connectNative: async () => new FakeNative(),
      }),
    ).toThrow('shell endpoint is outside the app dialling scope')
    expect(rec.opens).toBe(0)
    expect(rec.closes).toEqual([])
  })

  it('reports onClose(true) when connect fails before opening', async () => {
    const rec = recorder()
    openShellSocket(
      'sess-1',
      rec.handlers(),
      packagedOpts(async () => {
        throw new Error('refused')
      }),
    )
    await settle()
    expect(rec.opens).toBe(0)
    expect(rec.closes).toEqual([true])
  })

  it('reports onClose(true) when closed before connect resolves', async () => {
    const rec = recorder()
    let release: (ws: NativeWs) => void = () => {}
    const pending = new Promise<NativeWs>((resolve) => {
      release = resolve
    })
    const handle = openShellSocket(
      'sess-1',
      rec.handlers(),
      packagedOpts(async () => pending),
    )
    handle.close()
    expect(rec.closes).toEqual([true])
    const fake = new FakeNative()
    release(fake)
    await settle()
    expect(rec.opens).toBe(0)
    expect(rec.closes).toEqual([true])
    expect(fake.disconnected).toBe(true)
  })

  it('reports onClose(false) after a native socket has opened', async () => {
    const fake = new FakeNative()
    const rec = recorder()
    const handle = openShellSocket(
      'sess-1',
      rec.handlers(),
      packagedOpts(async () => fake),
    )
    await settle()
    handle.close()
    await settle()
    expect(rec.opens).toBe(1)
    expect(rec.closes).toEqual([false])
  })

  it('converts native binary arrays and decodes text frames', async () => {
    const fake = new FakeNative()
    const rec = recorder()
    const handle = openShellSocket(
      'sess-1',
      rec.handlers(),
      packagedOpts(async () => fake),
    )
    await settle()
    fake.deliver({ type: 'Binary', data: [0x61, 0x0a] })
    fake.deliver({ type: 'Text', data: '{"t":"exit","code":9}' })
    fake.deliver({ type: 'Text', data: '{"t":"dropped","reason":"token_revoked"}' })
    fake.deliver({ type: 'Text', data: 'nope' })
    handle.send(new Uint8Array([0x62]))
    handle.resize(40, 12)
    expect(Array.from(rec.bytes[0])).toEqual([0x61, 0x0a])
    expect(rec.exits).toEqual([9])
    expect(rec.dropped).toEqual(['token_revoked'])
    expect(fake.sent[0]).toEqual([0x62])
    expect(fake.sent[1]).toBe('{"t":"resize","cols":40,"rows":12}')
  })

  it('builds a URL that assertPairedWsUrl accepts', () => {
    const url = shellWsUrl(ENDPOINT, 'sess-1', false)
    expect(() => assertPairedWsUrl(ENDPOINT, url)).not.toThrow()
  })
})

describe('assertAllowedShellEndpoint', () => {
  /*
   * 2026-08-25 — GDK-865 (lead review). Track A shipped assertPairedWsUrl
   * called on a URL shellWsUrl had just derived from that same endpoint, so
   * on the real path it could not fail, and the capability description
   * pointed at it as the replacement for the missing websocket allowlist.
   * This is the check with content: the endpoint must be inside the list the
   * platform already enforces for http (capabilities/default.json).
   *
   * FAIL-first is not available for the guard itself — it did not exist —
   * but the row it exists for did: before this, openShellSocket would have
   * dialled `https://anything.example` without complaint.
   */
  it('admits the two shapes the http permission admits', () => {
    expect(() => assertAllowedShellEndpoint('https://home.example.ts.net')).not.toThrow()
    expect(() => assertAllowedShellEndpoint('https://home.example.ts.net:8443')).not.toThrow()
    expect(() => assertAllowedShellEndpoint('http://127.0.0.1:7899')).not.toThrow()
    expect(() => assertAllowedShellEndpoint('http://localhost:7899')).not.toThrow()
  })

  it('refuses everything else, including the near misses', () => {
    for (const bad of [
      'https://example.com',
      'https://ts.net.evil.example', // suffix trick
      'https://notts.net.example',
      'http://home.example.ts.net', // plaintext to a tailnet name
      'ws://home.example.ts.net', // already a ws URL, not an endpoint
      'file:///etc/passwd',
      'not a url',
      '',
    ]) {
      expect(() => assertAllowedShellEndpoint(bad), bad).toThrow()
    }
  })
})
