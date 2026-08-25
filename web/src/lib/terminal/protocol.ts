/*
 * The terminal wire vocabulary, owned in one place because two transports
 * speak it (GDK-892): the browser's WebSocket (./session) and Gadak.app's
 * wails GoStream (./wails-stream).
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
