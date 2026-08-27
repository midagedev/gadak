// Phone shell transport (GDK-865). Same split as lib/api.ts, for the same
// reason: a webview WebSocket cannot set Authorization and its origin is
// not the serve's. Dev rides the vite /api proxy (loopback, no bearer).
// Packaged uses tauri-plugin-websocket with a Bearer the terminal gate
// accepts, after assertPairedWsUrl has refused any other origin.
//
// Wire vocabulary is web/src/lib/terminal/protocol.ts — not re-spelled.
// The token never appears in a log, an error, or a URL.

import { coerceDroppedReason } from '../../../../web/src/lib/terminal/protocol'
import type { SocketHandle, SocketHandlers } from '../../../../web/src/lib/terminal/protocol'
import { inDialScope } from '../dial-scope'

const IS_DEV = import.meta.env.DEV

const WS_CONNECTING = 0
const WS_OPEN = 1

const ORIGIN_MISMATCH = 'shell websocket is not the paired origin'

export interface NativeWsMessage {
  type: string
  data?: unknown
}

export interface NativeWs {
  addListener(cb: (msg: NativeWsMessage) => void): () => void
  send(message: string | number[] | NativeWsMessage): Promise<void>
  disconnect(): Promise<void>
}

export type NativeConnect = (
  url: string,
  config?: { headers?: [string, string][] },
) => Promise<NativeWs>

export interface BrowserSocket {
  readyState: number
  binaryType: string
  send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void
  close(): void
  addEventListener(type: string, fn: (ev: { data?: unknown }) => void): void
}

export interface ShellSocketOpts {
  endpoint: string
  token: string | null
  /** Test seam. Defaults to import.meta.env.DEV. */
  dev?: boolean
  /** Test seam: browser WebSocket constructor. */
  webSocket?: new (url: string) => BrowserSocket
  /** Test seam: plugin WebSocket.connect. */
  connectNative?: NativeConnect
}

function effectivePort(u: URL): string {
  if (u.port !== '') return u.port
  if (u.protocol === 'https:' || u.protocol === 'wss:') return '443'
  if (u.protocol === 'http:' || u.protocol === 'ws:') return '80'
  return ''
}

function wsProtocolFor(httpProtocol: string): string | null {
  if (httpProtocol === 'https:') return 'wss:'
  if (httpProtocol === 'http:') return 'ws:'
  return null
}

const ENDPOINT_OUT_OF_SCOPE = 'shell endpoint is outside the app dialling scope'

/**
 * The endpoint is inside the same scope the platform enforces for HTTP.
 *
 * `http:default` in capabilities/default.json is URL-scoped to
 * `https://*.ts.net:*` and loopback, so a fetch outside that scope is refused
 * by Tauri itself. The websocket permission carries no such list, which
 * would otherwise let the shell socket reach a host the mirror's own
 * transport cannot. The list itself lives in lib/dial-scope.ts — one owner,
 * ports included (GDK-1048), shared with the fetch path in lib/api.ts — so
 * the two transports agree on where this app may dial.
 *
 * Read it for what it is: a **correctness** guard, not a security boundary.
 * The grant is process-wide — anything running in this webview can call the
 * plugin directly and skip this function entirely. The boundary version is a
 * Rust command that validates against the stored pairing before connecting;
 * that is filed, not built (GDK-897).
 */
export function assertAllowedShellEndpoint(endpoint: string): void {
  if (!inDialScope(endpoint)) throw new Error(ENDPOINT_OUT_OF_SCOPE)
}

/**
 * Throws unless `url` addresses the paired endpoint's own origin.
 *
 * Defence in depth beside assertAllowedShellEndpoint: that one says "a host
 * this app may dial at all", this one says "the host this phone actually
 * paired with". Neither implies the other — the tailnet has many `.ts.net`
 * names and only one of them is yours.
 */
export function assertPairedWsUrl(endpoint: string, url: string): void {
  let expected: URL
  let actual: URL
  try {
    expected = new URL(endpoint)
    actual = new URL(url)
  } catch {
    throw new Error(ORIGIN_MISMATCH)
  }
  const want = wsProtocolFor(expected.protocol)
  if (want === null || actual.protocol !== want) {
    throw new Error(ORIGIN_MISMATCH)
  }
  if (actual.hostname !== expected.hostname) {
    throw new Error(ORIGIN_MISMATCH)
  }
  if (effectivePort(actual) !== effectivePort(expected)) {
    throw new Error(ORIGIN_MISMATCH)
  }
}

/** `<endpoint>/api/v1/terminal/sessions/<id>/ws/` with http→ws, https→wss. */
export function shellWsUrl(endpoint: string, id: string, dev: boolean = IS_DEV): string {
  const path = `/api/v1/terminal/sessions/${encodeURIComponent(id)}/ws/`
  if (dev) {
    const host =
      typeof location !== 'undefined' && location.host ? location.host : 'localhost:5180'
    const scheme =
      typeof location !== 'undefined' && location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${scheme}//${host}${path}`
  }
  let http: URL
  try {
    http = new URL(endpoint)
  } catch {
    throw new Error(ORIGIN_MISMATCH)
  }
  const mapped = wsProtocolFor(http.protocol)
  if (mapped === null) throw new Error(ORIGIN_MISMATCH)
  // host is hostname[:port]; do not slice origin on ':' — that hits the
  // scheme colon and produces wss:://.
  return `${mapped}//${http.host}${path}`
}

/**
 * Server text frames. Malformed JSON is ignored, not thrown — the serve
 * may still be sending PTY bytes on the next frame.
 */
export function applyServerTextFrame(
  raw: string,
  handlers: Pick<SocketHandlers, 'onExit' | 'onDropped'>,
): void {
  let msg: { t?: unknown; code?: unknown; reason?: unknown }
  try {
    msg = JSON.parse(raw) as typeof msg
  } catch {
    return
  }
  if (msg.t === 'exit') {
    handlers.onExit(typeof msg.code === 'number' ? msg.code : 0)
    return
  }
  if (msg.t === 'dropped') handlers.onDropped(coerceDroppedReason(msg.reason))
}

/** Binary frames from the native plugin arrive as a number array. */
export function nativeBinaryToBytes(data: unknown): Uint8Array | null {
  if (data instanceof Uint8Array) return data
  if (data instanceof ArrayBuffer) return new Uint8Array(data)
  if (ArrayBuffer.isView(data)) {
    const view = data as ArrayBufferView
    return new Uint8Array(view.buffer, view.byteOffset, view.byteLength)
  }
  // One allocation, not two: this runs on every server frame, and a build
  // log is a lot of frames.
  if (Array.isArray(data)) return Uint8Array.from(data, (n) => Number(n) & 0xff)
  return null
}

function idleHandle(): SocketHandle {
  return {
    send() {},
    resize() {},
    close() {},
  }
}

export function openShellSocket(
  id: string,
  handlers: SocketHandlers,
  opts: ShellSocketOpts,
): SocketHandle {
  const dev = opts.dev ?? IS_DEV
  return dev ? openDevSocket(id, handlers, opts) : openPackagedSocket(id, handlers, opts)
}

function openDevSocket(id: string, handlers: SocketHandlers, opts: ShellSocketOpts): SocketHandle {
  const Ctor = opts.webSocket ?? (globalThis.WebSocket as unknown as new (url: string) => BrowserSocket)
  if (typeof Ctor !== 'function') {
    handlers.onClose(true)
    return idleHandle()
  }
  const ws = new Ctor(shellWsUrl(opts.endpoint, id, true))
  ws.binaryType = 'arraybuffer'
  let opened = false
  let closed = false
  const finishClose = () => {
    if (closed) return
    closed = true
    handlers.onClose(!opened)
  }
  ws.addEventListener('open', () => {
    opened = true
    handlers.onOpen()
  })
  ws.addEventListener('message', (ev) => {
    if (typeof ev.data === 'string') {
      applyServerTextFrame(ev.data, handlers)
      return
    }
    if (ev.data instanceof ArrayBuffer) {
      handlers.onBytes(new Uint8Array(ev.data))
    }
  })
  ws.addEventListener('error', () => {
    /* close follows */
  })
  ws.addEventListener('close', finishClose)
  return {
    send(data: Uint8Array) {
      if (ws.readyState === WS_OPEN) ws.send(data)
    },
    resize(cols: number, rows: number) {
      if (ws.readyState === WS_OPEN) {
        ws.send(JSON.stringify({ t: 'resize', cols, rows }))
      }
    },
    close() {
      if (ws.readyState === WS_CONNECTING || ws.readyState === WS_OPEN) {
        ws.close()
      }
    },
  }
}

function openPackagedSocket(
  id: string,
  handlers: SocketHandlers,
  opts: ShellSocketOpts,
): SocketHandle {
  assertAllowedShellEndpoint(opts.endpoint)
  const url = shellWsUrl(opts.endpoint, id, false)
  assertPairedWsUrl(opts.endpoint, url)

  let native: NativeWs | null = null
  let opened = false
  let closed = false
  let wantClose = false

  const finishClose = () => {
    if (closed) return
    closed = true
    handlers.onClose(!opened)
  }

  const onNativeMessage = (msg: NativeWsMessage) => {
    if (msg.type === 'Text' && typeof msg.data === 'string') {
      applyServerTextFrame(msg.data, handlers)
      return
    }
    if (msg.type === 'Binary') {
      const bytes = nativeBinaryToBytes(msg.data)
      if (bytes) handlers.onBytes(bytes)
      return
    }
    if (msg.type === 'Close') finishClose()
  }

  const headers: [string, string][] = []
  if (opts.token) headers.push(['Authorization', `Bearer ${opts.token}`])

  const connect: NativeConnect =
    opts.connectNative ??
    (async (connectUrl, config) => {
      const mod = await import('@tauri-apps/plugin-websocket')
      return mod.default.connect(connectUrl, config)
    })

  void connect(url, headers.length > 0 ? { headers } : undefined)
    .then((ws) => {
      if (wantClose || closed) {
        void ws.disconnect().catch(() => {})
        finishClose()
        return
      }
      native = ws
      ws.addListener(onNativeMessage)
      opened = true
      handlers.onOpen()
    })
    .catch(() => {
      finishClose()
    })

  return {
    send(data: Uint8Array) {
      if (!native) return
      void native.send(Array.from(data)).catch(() => {})
    },
    resize(cols: number, rows: number) {
      if (!native) return
      void native.send(JSON.stringify({ t: 'resize', cols, rows })).catch(() => {})
    },
    close() {
      wantClose = true
      if (native) {
        void native.disconnect().catch(() => {})
        return
      }
      finishClose()
    },
  }
}
