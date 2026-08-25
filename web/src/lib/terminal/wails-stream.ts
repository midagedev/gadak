/*
 * The terminal's transport inside Gadak.app (GDK-892).
 *
 * The app mounts gadak's http.Handler behind the wails asset server — a
 * WKURLSchemeHandler on macOS — and opens no TCP port, by design. The pane's
 * REST calls therefore work here, but its WebSocket cannot: a browser's
 * network stack owns ws:// and never consults a custom-scheme handler. So in
 * the app the pane used to end at `terminal.unavailable`.
 *
 * wails v3 ships a transport for exactly this position: GoStream, a held poll
 * plus a POST that wails dispatches before any user handler, so gadak's own
 * asset handler never sees it. `Stream(name)` hands back a WailsSocket with
 * the WebSocket shape, and desktop/terminal_stream.go is the Go end.
 *
 * Framing, symmetrical both ways, because a WailsSocket delivers every
 * message as an ArrayBuffer and has no text-vs-binary distinction to carry
 * the control channel on (desktop/terminal_stream.go states the same table):
 *
 *   byte 0 = 0x00 → the rest of the frame is raw PTY bytes
 *   byte 0 = 0x01 → the rest of the frame is UTF-8 JSON control
 *
 * The control vocabulary is the one the WebSocket already speaks, so the
 * pane's status rendering needs no new case.
 */

// The callback shape and the dropped-reason vocabulary have one owner, and it
// is the transport-neutral half of ./session. This is a cycle only in the
// module graph: nothing here runs at import time.
import { coerceDroppedReason, type SocketHandle, type SocketHandlers } from './protocol'

/** Frame tags. Mirrors desktop/terminal_stream.go. */
export const TERM_FRAME_DATA = 0x00
export const TERM_FRAME_CTRL = 0x01

/** The stream name desktop/terminal_stream.go registers. */
export const TERMINAL_STREAM_NAME = 'terminal'

/**
 * wails serves this itself, ahead of gadak's asset handler
 * (pkg/application/application.go:138-146). It exists only inside the app.
 */
export const WAILS_RUNTIME_URL = '/wails/runtime.js'

/** WailsSocket.OPEN. Spelled out because the class is loaded at runtime. */
const SOCKET_OPEN = 1
/** WailsSocket.CLOSED. */
const SOCKET_CLOSED = 3

/**
 * The part of wails' WailsSocket this transport uses. Structural on purpose:
 * /wails/runtime.js is served by the app at runtime, so there is no package
 * to import a type from, and a wider surface would only be a guess.
 */
export interface WailsSocketLike {
  binaryType: string
  readyState: number
  send(data: ArrayBufferView | ArrayBufferLike | string): void
  close(): void
  onopen: ((ev: unknown) => void) | null
  onmessage: ((ev: { data: unknown }) => void) | null
  onclose: ((ev: unknown) => void) | null
  onerror: ((ev: unknown) => void) | null
}

export type WailsStreamFactory = (name: string) => WailsSocketLike

/**
 * Loads wails' own runtime module. The @vite-ignore is what keeps the browser
 * build free of it: /wails/runtime.js exists only inside the app, served by
 * wails itself, and Vite must not try to resolve it at build time.
 */
export async function loadWailsStreamFactory(): Promise<WailsStreamFactory> {
  // Widened to `string` deliberately: as a literal, TypeScript tries to
  // resolve the module and fails ("cannot find module"), because the file is
  // produced by the running app rather than by this repository.
  const url: string = WAILS_RUNTIME_URL
  const mod = (await import(/* @vite-ignore */ url)) as {
    Stream?: unknown
  }
  if (typeof mod.Stream !== 'function') {
    throw new Error('wails runtime: no Stream export')
  }
  return mod.Stream as WailsStreamFactory
}

function encodeFrame(tag: number, body: Uint8Array): Uint8Array {
  const frame = new Uint8Array(body.length + 1)
  frame[0] = tag
  frame.set(body, 1)
  return frame
}

function controlFrame(msg: object): Uint8Array {
  return encodeFrame(TERM_FRAME_CTRL, new TextEncoder().encode(JSON.stringify(msg)))
}

/** ArrayBuffer | ArrayBufferView → bytes. The runtime sends the first. */
function frameBytes(data: unknown): Uint8Array | null {
  if (data instanceof ArrayBuffer) return new Uint8Array(data)
  if (ArrayBuffer.isView(data)) {
    const view = data as ArrayBufferView
    return new Uint8Array(view.buffer, view.byteOffset, view.byteLength)
  }
  return null
}

/**
 * Opens one attachment to session `id` over the wails stream, translating
 * frames to the same callbacks the WebSocket path uses.
 *
 * `load` is the runtime loader, injectable so the protocol can be tested
 * without a webview. Nothing in the app passes it.
 *
 * Returns synchronously, like `new WebSocket(...)` does: the runtime import
 * and the attach handshake both happen behind the handle. Until the Go side
 * answers `{"t":"attached"}` the handle drops sends, which is what the
 * WebSocket path does while it is CONNECTING.
 */
export function openWailsSessionSocket(
  id: string,
  handlers: SocketHandlers,
  load: () => Promise<WailsStreamFactory> = loadWailsStreamFactory,
): SocketHandle {
  let sock: WailsSocketLike | null = null
  let attached = false
  let closed = false
  let wantClose = false

  const finishClose = () => {
    if (closed) return
    closed = true
    // Not `!attached` at the socket's own `open`: for this pane onOpen means
    // "I am attached to a shell", and only the attached ack says that. An
    // error frame therefore reaches the pane as never-opened, which is the
    // path a 404 reattach already takes (drop the kept id, create a session).
    handlers.onClose(!attached)
  }

  const onControl = (body: Uint8Array) => {
    let msg: { t?: string; code?: unknown; reason?: unknown }
    try {
      msg = JSON.parse(new TextDecoder().decode(body)) as typeof msg
    } catch {
      return
    }
    switch (msg.t) {
      case 'attached':
        attached = true
        handlers.onOpen()
        return
      case 'exit':
        handlers.onExit(typeof msg.code === 'number' ? msg.code : 0)
        return
      case 'dropped':
        handlers.onDropped(coerceDroppedReason(msg.reason))
        return
      case 'error':
        // not_found / protocol / unsupported. The pane's answer to all three
        // is the same as to a socket that never opened, so say exactly that
        // rather than inventing a status it has no rendering for.
        sock?.close()
        finishClose()
        return
      default:
        // An unknown control type is a newer Go side talking, not a fault.
        return
    }
  }

  const onFrame = (data: unknown) => {
    const frame = frameBytes(data)
    if (!frame || frame.length === 0) return
    const body = frame.slice(1)
    if (frame[0] === TERM_FRAME_DATA) {
      handlers.onBytes(body)
      return
    }
    if (frame[0] === TERM_FRAME_CTRL) onControl(body)
  }

  void load()
    .then((factory) => {
      if (wantClose) {
        finishClose()
        return
      }
      const s = factory(TERMINAL_STREAM_NAME)
      // Already the runtime's default; set for the same reason the WebSocket
      // path sets it — the framing depends on it and the default is theirs.
      s.binaryType = 'arraybuffer'
      sock = s
      s.onopen = () => {
        // MUST be the first frame: the Go handler reads it before anything
        // else and answers a protocol error to anything that is not an
        // attach. send() throws while CONNECTING, so this is the earliest
        // point it can go out.
        s.send(controlFrame({ t: 'attach', id }))
      }
      s.onmessage = (ev) => onFrame(ev?.data)
      s.onclose = () => finishClose()
      s.onerror = () => {
        /* close follows */
      }
    })
    .catch(() => {
      // No wails runtime: this is not the app, or it is a build without the
      // stream transport. The pane's `unavailable` is the honest answer.
      finishClose()
    })

  return {
    send(data: Uint8Array) {
      if (sock?.readyState === SOCKET_OPEN) sock.send(encodeFrame(TERM_FRAME_DATA, data))
    },
    resize(cols: number, rows: number) {
      if (sock?.readyState === SOCKET_OPEN) sock.send(controlFrame({ t: 'resize', cols, rows }))
    },
    close() {
      wantClose = true
      if (sock && sock.readyState !== SOCKET_CLOSED) {
        sock.close()
        return
      }
      // Still loading the runtime, or already gone. The pane is owed its
      // onClose either way — the pane's own open timeout closes a handle
      // that never came up, and a WebSocket answers that with a close event.
      finishClose()
    },
  }
}
