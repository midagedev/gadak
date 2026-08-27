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

/*
 * Why a shell could not be opened, and what a host may offer next.
 *
 * This sits with the wire vocabulary for the same reason the vocabulary
 * does: the browser pane and the phone are two hosts that must reach the
 * same verdict from the same server response. The first cut of this lived
 * in ./session, which the phone cannot import, so the phone kept its own
 * one-sentence version — and removing that sentence turned the phone's
 * typecheck red on a change that never touched it (2026-08-26). One owner,
 * two adapters: each host unwraps its own error class and hands over the
 * status and code.
 */

export type UnavailableCause = 'unsupported' | 'forbidden' | 'failed' | 'network'

/** 403 scope_rejected / forbidden_host, 401 pairing_rejected — the shapes
 *  internal/server/mirror_gate.go answers with. */
const FORBIDDEN_CODES: ReadonlySet<string> = new Set([
  'scope_rejected',
  'forbidden_host',
  'pairing_rejected',
])

/** Keyed on the server's error *code*, never on a localized string. The
 *  status is the fallback for a body that carried no code. */
export function classifyUnavailable(status: number, code: string | null): UnavailableCause {
  if (code === 'terminal_unsupported' || status === 501) return 'unsupported'
  if ((code !== null && FORBIDDEN_CODES.has(code)) || status === 401 || status === 403) {
    return 'forbidden'
  }
  if (code === 'terminal_failed' || status === 500) return 'failed'
  return 'network'
}

/** The i18n key for each cause. Both hosts render the same sentence. */
export const UNAVAILABLE_KEYS = {
  unsupported: 'terminal.unavailable.unsupported',
  forbidden: 'terminal.unavailable.forbidden',
  failed: 'terminal.unavailable.failed',
  network: 'terminal.unavailable.network',
} as const

/** Windows has no PTY, so no retry can succeed there; everything else is
 *  worth one more try, because the user asked for it. */
export function unavailableAllowsRestart(cause: UnavailableCause): boolean {
  return cause !== 'unsupported'
}

/** A revoked token cannot open another shell — offering "press Enter" there
 *  is advice that fails the moment it is followed. */
export function droppedAllowsRestart(reason: DroppedReason): boolean {
  return reason !== 'token_revoked'
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
 * mechanical reason: ./renderer is the web app's own implementation (DOM
 * hosts, cssVar, the xterm dynamic import). A host that brings its own
 * renderer — the phone, the desktop stream — cannot import anything from
 * that module without dragging the web renderer into its dependency graph.
 * So the *contract* lives here, where it costs nothing, and each host
 * brings its own implementation (GDK-865).
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
