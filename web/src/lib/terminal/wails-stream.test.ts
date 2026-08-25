import { afterEach, describe, expect, test, vi } from 'vitest'
import {
  openWailsSessionSocket,
  TERM_FRAME_CTRL,
  TERM_FRAME_DATA,
  TERMINAL_STREAM_NAME,
  type WailsSocketLike,
} from './wails-stream'

/**
 * The desktop terminal transport (GDK-892). The Go end of this protocol is
 * pinned in desktop/terminal_stream_test.go against a real PTY; this file
 * pins the frames the browser half puts on the wire and reads off it, since
 * a wrong tag byte here is a shell that silently does nothing.
 *
 * The fake is a WailsSocket, not a WebSocket: what /wails/runtime.js hands
 * back is an object with that shape, and the loader is injectable precisely
 * so this suite needs no webview.
 */

class FakeWailsSocket implements WailsSocketLike {
  binaryType = 'blob'
  readyState = 0
  readonly sent: Uint8Array[] = []
  closeCalls = 0
  onopen: ((ev: unknown) => void) | null = null
  onmessage: ((ev: { data: unknown }) => void) | null = null
  onclose: ((ev: unknown) => void) | null = null
  onerror: ((ev: unknown) => void) | null = null

  send(data: ArrayBufferView | ArrayBufferLike | string): void {
    if (typeof data === 'string') throw new Error('the transport must never send text')
    this.sent.push(
      ArrayBuffer.isView(data)
        ? new Uint8Array(data.buffer, data.byteOffset, data.byteLength).slice()
        : new Uint8Array(data as ArrayBuffer).slice(),
    )
  }

  close(): void {
    this.closeCalls += 1
    if (this.readyState === 3) return
    this.readyState = 3
    this.onclose?.({})
  }

  /** The runtime's open ack: readyState moves, then the event fires. */
  open(): void {
    this.readyState = 1
    this.onopen?.({})
  }

  /** One inbound frame, delivered the way binaryType 'arraybuffer' does. */
  deliver(frame: Uint8Array): void {
    const buf = frame.slice().buffer
    this.onmessage?.({ data: buf })
  }

  deliverControl(msg: unknown): void {
    const body = new TextEncoder().encode(JSON.stringify(msg))
    const frame = new Uint8Array(body.length + 1)
    frame[0] = TERM_FRAME_CTRL
    frame.set(body, 1)
    this.deliver(frame)
  }
}

function recorder() {
  return {
    opens: 0,
    bytes: [] as Uint8Array[],
    exits: [] as number[],
    dropped: [] as string[],
    closes: [] as boolean[],
    handlers() {
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
        onDropped: (reason: string) => {
          this.dropped.push(reason)
        },
        onClose: (neverOpened: boolean) => {
          this.closes.push(neverOpened)
        },
      }
    },
  }
}

/** Flushes the microtasks the injected loader's promise chain runs on. */
const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

function decodeControl(frame: Uint8Array): Record<string, unknown> {
  expect(frame[0]).toBe(TERM_FRAME_CTRL)
  return JSON.parse(new TextDecoder().decode(frame.slice(1))) as Record<string, unknown>
}

/** Opens the transport against a fake socket and returns both ends. */
async function connect(id = 'sess-1') {
  const sock = new FakeWailsSocket()
  const names: string[] = []
  const rec = recorder()
  const handle = openWailsSessionSocket(id, rec.handlers(), async () => (name: string) => {
    names.push(name)
    return sock
  })
  await settle()
  return { sock, handle, rec, names }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.resetModules()
  vi.doUnmock('../config')
  vi.doUnmock('./wails-stream')
})

describe('wails stream transport', () => {
  test('the tag bytes are the wire values desktop/terminal_stream.go names', () => {
    // The other speaker of this protocol is Go and cannot import these, so
    // the numbers themselves are the contract, not the identifiers.
    expect(TERM_FRAME_DATA).toBe(0x00)
    expect(TERM_FRAME_CTRL).toBe(0x01)
    expect(TERMINAL_STREAM_NAME).toBe('terminal')
  })

  test('the attach frame is the first thing sent, on the stream Go registers', async () => {
    const { sock, names } = await connect('sess-abc')
    expect(names).toEqual([TERMINAL_STREAM_NAME])
    // The runtime's default, restated: the framing depends on it.
    expect(sock.binaryType).toBe('arraybuffer')
    // send() throws while CONNECTING, so nothing may go out before open.
    expect(sock.sent).toHaveLength(0)

    sock.open()
    expect(sock.sent).toHaveLength(1)
    expect(decodeControl(sock.sent[0])).toEqual({ t: 'attach', id: 'sess-abc' })
  })

  test('onOpen means attached to a shell, not the socket coming up', async () => {
    const { sock, rec } = await connect()
    sock.open()
    expect(rec.opens).toBe(0)

    sock.deliverControl({ t: 'attached' })
    expect(rec.opens).toBe(1)
  })

  test('a 0x00 frame becomes onBytes with exactly those bytes', async () => {
    const { sock, rec } = await connect()
    sock.open()
    sock.deliverControl({ t: 'attached' })

    const payload = new Uint8Array([0x67, 0x64, 0x6b, 0x00, 0xff, 0x0a])
    const frame = new Uint8Array(payload.length + 1)
    frame[0] = TERM_FRAME_DATA
    frame.set(payload, 1)
    sock.deliver(frame)

    expect(rec.bytes).toHaveLength(1)
    expect(Array.from(rec.bytes[0])).toEqual(Array.from(payload))
  })

  test('keystrokes and resizes go out tagged', async () => {
    const { sock, handle } = await connect()
    sock.open()
    sock.deliverControl({ t: 'attached' })
    sock.sent.length = 0

    handle.send(new Uint8Array([0x6c, 0x73, 0x0d]))
    handle.resize(132, 43)

    expect(Array.from(sock.sent[0])).toEqual([TERM_FRAME_DATA, 0x6c, 0x73, 0x0d])
    expect(decodeControl(sock.sent[1])).toEqual({ t: 'resize', cols: 132, rows: 43 })
  })

  test('exit carries the code; dropped carries the reason', async () => {
    const { sock, rec } = await connect()
    sock.open()
    sock.deliverControl({ t: 'attached' })

    sock.deliverControl({ t: 'exit', code: 7 })
    expect(rec.exits).toEqual([7])

    sock.deliverControl({ t: 'dropped', reason: 'slow_client' })
    sock.deliverControl({ t: 'dropped', reason: 'not-a-reason' })
    expect(rec.dropped).toEqual(['slow_client', 'closed'])
  })

  test('an unknown control type is ignored, not thrown', async () => {
    const { sock, rec } = await connect()
    sock.open()
    sock.deliverControl({ t: 'attached' })

    expect(() => sock.deliverControl({ t: 'someday' })).not.toThrow()
    expect(() => sock.deliver(new Uint8Array([TERM_FRAME_CTRL, 0x7b]))).not.toThrow()
    expect(() => sock.deliver(new Uint8Array([0x09, 0x01]))).not.toThrow()
    expect(() => sock.deliver(new Uint8Array(0))).not.toThrow()
    // None of that may be mistaken for output, an exit, or a close.
    expect(rec.bytes).toHaveLength(0)
    expect(rec.exits).toEqual([])
    expect(rec.closes).toEqual([])
  })

  test('an error frame closes as never-opened, the 404-reattach path', async () => {
    const { sock, rec } = await connect()
    sock.open()
    sock.deliverControl({ t: 'error', code: 'not_found' })

    expect(sock.closeCalls).toBe(1)
    expect(rec.opens).toBe(0)
    expect(rec.closes).toEqual([true])
  })

  test('a closed socket after attaching reports opened, so the pane reconnects', async () => {
    const { sock, rec } = await connect()
    sock.open()
    sock.deliverControl({ t: 'attached' })
    sock.close()
    expect(rec.closes).toEqual([false])
  })

  test('no wails runtime is the honest unavailable answer', async () => {
    const rec = recorder()
    openWailsSessionSocket('sess-1', rec.handlers(), async () => {
      throw new Error('Cannot find module /wails/runtime.js')
    })
    await settle()
    expect(rec.closes).toEqual([true])
  })
})

describe('the transport picker', () => {
  /**
   * Both halves are observed: which opener ran, and — the part that matters
   * on the desktop — that no WebSocket was constructed at all. There is no
   * TCP port for one to reach inside Gadak.app, so a picker that fell through
   * to the socket would leave the pane on `unavailable`, which is the state
   * this whole round exists to end.
   */
  async function pick(desktop: boolean) {
    vi.resetModules()
    vi.doMock('../config', () => ({
      config: () => ({ apiBase: '/api/v1/issues/' }),
      isDesktop: () => desktop,
    }))
    const wails = vi.fn(() => ({ send() {}, resize() {}, close() {} }))
    vi.doMock('./wails-stream', () => ({ openWailsSessionSocket: wails }))
    const urls: string[] = []
    class SpyWebSocket {
      static readonly OPEN = 1
      readyState = 0
      binaryType = ''
      constructor(url: string) {
        urls.push(url)
      }
      addEventListener(): void {}
      close(): void {}
    }
    vi.stubGlobal('WebSocket', SpyWebSocket)
    vi.stubGlobal('location', {
      protocol: 'http:',
      host: '127.0.0.1:7877',
      href: 'http://127.0.0.1:7877/',
    })
    const mod = await import('./session')
    const handlers = recorder().handlers()
    const handle = mod.openSessionSocket('sess-1', handlers)
    return { urls, wails, handle, handlers }
  }

  test('desktop takes the wails stream and opens no WebSocket', async () => {
    const { urls, wails, handlers } = await pick(true)
    expect(urls).toEqual([])
    expect(wails).toHaveBeenCalledTimes(1)
    // The same handlers, unwrapped: the pane's callbacks are the contract
    // both transports meet.
    expect(wails.mock.calls[0]).toEqual(['sess-1', handlers])
  })

  test('a browser opens the session WebSocket and never loads wails', async () => {
    const { urls, wails } = await pick(false)
    expect(urls).toEqual(['ws://127.0.0.1:7877/api/v1/terminal/sessions/sess-1/ws/'])
    expect(wails).not.toHaveBeenCalled()
  })
})
