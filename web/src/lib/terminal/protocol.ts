/*
 * The terminal wire vocabulary, owned in one place because several hosts
 * speak it: the browser's WebSocket (./session), Gadak.app's wails GoStream
 * (./wails-stream, GDK-892), and the phone's native socket
 * (mobile/src/lib/terminal, GDK-865).
 *
 * It lives here rather than in ./session so the two transports do not import
 * each other. The picker in ./session has to reach the wails transport, and
 * the wails transport needs this vocabulary — with both in one file that is
 * a runtime import cycle. ESM tolerates one as long as nothing is read at
 * module-evaluation time, which is a property a future edit can silently
 * break; a third module cannot break it.
 *
 * The strings are the serve's own (internal/term/session.go), and neither
 * side re-spells them.
 */

export type DroppedReason =
  | 'slow_client'
  | 'token_revoked'
  | 'idle_timeout'
  | 'server_shutdown'
  | 'closed'

const DROPPED_REASONS: ReadonlySet<string> = new Set([
  'slow_client',
  'token_revoked',
  'idle_timeout',
  'server_shutdown',
  'closed',
])

/** An unknown reason is reported as a plain close: the shell is gone either
 *  way, and inventing a cause would be worse than naming none. */
export function coerceDroppedReason(raw: unknown): DroppedReason {
  return typeof raw === 'string' && DROPPED_REASONS.has(raw)
    ? (raw as DroppedReason)
    : 'closed'
}

/** One client's view of a live session, whatever carries the bytes. */
export interface SocketHandle {
  send(data: Uint8Array): void
  resize(cols: number, rows: number): void
  close(): void
}

/**
 * What a transport calls back into. `onOpen` means "attached to a shell",
 * not "the socket connected" — the wails transport has a handshake after
 * its socket opens, and the pane must not paint a terminal before a shell
 * is behind it.
 */
export interface SocketHandlers {
  onOpen: () => void
  onBytes: (data: Uint8Array) => void
  onExit: (code: number) => void
  onDropped: (reason: DroppedReason) => void
  onClose: (neverOpened: boolean) => void
}

/*
 * What a terminal renderer is, and the one decoder every host needs.
 *
 * These sit beside the wire vocabulary rather than in ./renderer for a
 * mechanical reason: ./renderer's createRenderer holds `import('ghostty-web')`,
 * and a bundler resolves that at build time. A host that ships only xterm —
 * the phone — cannot import anything from that module without dragging the
 * WASM renderer into its dependency graph. So the *contract* lives here,
 * where it costs nothing, and each host brings its own implementation.
 */

/** One renderer's surface, whichever library is behind it. */
export interface TerminalRenderer {
  open(host: HTMLElement): void
  write(data: Uint8Array | string): void
  onData(cb: (bytes: Uint8Array) => void): void
  onResize(cb: (cols: number, rows: number) => void): void
  fit(): void
  focus(): void
  dispose(): void
  readonly cols: number
  readonly rows: number
  readonly name: string
}

/** Streaming UTF-8 decoder so a 256 KiB ring replay that splits a character
 *  across two write() calls still renders one glyph. fatal:false matches
 *  "bytes from a PTY, never a contract that they are well-formed". */
export function createUtf8StreamDecoder(): {
  push(data: Uint8Array | string): string
} {
  const dec = new TextDecoder('utf-8', { fatal: false })
  return {
    push(data: Uint8Array | string): string {
      if (typeof data === 'string') return data
      return dec.decode(data, { stream: true })
    },
  }
}
