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
  'slow_client' | 'token_revoked' | 'idle_timeout' | 'server_shutdown' | 'closed'

// Exported so a lock test can pin this set against the Go Reason* constants
// (internal/term/session.go) it mirrors — the comment above promises neither
// side re-spells the other, and GDK-932 makes that a test, not a promise.
export const DROPPED_REASONS: ReadonlySet<string> = new Set([
  'slow_client',
  'token_revoked',
  'idle_timeout',
  'server_shutdown',
  'closed',
])

/** An unknown reason is reported as a plain close: the shell is gone either
 *  way, and inventing a cause would be worse than naming none. */
export function coerceDroppedReason(raw: unknown): DroppedReason {
  return typeof raw === 'string' && DROPPED_REASONS.has(raw) ? (raw as DroppedReason) : 'closed'
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

/*
 * The chrome CSS variables both renderers read (GDK-1109). The *names* have
 * one owner here because the phone imports this module directly: before
 * this, each renderer carried its own copy of the list, so a web rename left
 * the phone's chrome silently on its fallbacks — no gate fired. The
 * *fallback values* stay with each renderer (web pins paper-theme hexes;
 * the phone passes '' and lets xterm's own default stand until its
 * stylesheet loads). protocol.test.ts pins this list against app.css, where
 * the tokens are declared.
 */

/** xterm theme slot → the app token that paints it. Chrome only — the ANSI
 * palette stays the library default so a light paper theme does not invert
 * black/white. */
export const TERMINAL_CHROME_VARS = {
  background: '--color-bg-base',
  foreground: '--color-text-primary',
  cursor: '--color-accent',
  cursorAccent: '--color-bg-base',
  selectionBackground: '--color-bg-active',
} as const

/*
 * When those variables move, and who notices (GDK-1156).
 *
 * The names having one owner was not enough: both renderers read them once,
 * at construction, so a live theme change left the pane painted in the
 * palette it was born in. Measured on the phone during the 0.19 hero shoot —
 * iOS flipped to dark mid-session and the command output that had just
 * landed became dark ink on a dark pane, which reads as an empty terminal.
 *
 * There are three live paths, and counting them was the trap: the picker
 * (data-theme), the OS (prefers-color-scheme, the phone's only path), and
 * `gadak config set ui.tokens`, which swaps a <style> element with no reload
 * because retinting an open tab is the whole point of that feature. A
 * subscriber wired to any one of them is a fix for one path and a bug for
 * the next one someone adds.
 *
 * So this watches the *outcome* instead of the causes: anything that could
 * repaint the document re-reads the tokens, and the callback fires only when
 * a value actually changed. A fourth path costs nothing.
 */
export function watchChromeVars(read: () => string, onChange: () => void): { stop(): void } {
  if (typeof document === 'undefined' || typeof window === 'undefined') {
    return { stop() {} }
  }
  let last = read()
  let queued = false
  const check = () => {
    queued = false
    const now = read()
    if (now === last) return
    last = now
    onChange()
  }
  // Coalesced to a frame: a token write touches the stylesheet, the layout
  // dims and the row-metric cache in one call, and a picker change lands an
  // attribute plus a media flip. One re-read per frame covers all of it.
  const schedule = () => {
    if (queued) return
    queued = true
    requestAnimationFrame(check)
  }
  // <html>: data-theme (picker) and inline custom properties (layout dims,
  // and anything that later writes a token straight onto the element).
  const root = new MutationObserver(schedule)
  root.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme', 'style'],
  })
  // <head>: the ui.tokens override sheet is installed once and then has its
  // textContent replaced, so childList alone would miss every write after
  // the first — characterData under a subtree is what sees the swap.
  const head = new MutationObserver(schedule)
  head.observe(document.head, { childList: true, subtree: true, characterData: true })
  const mq = window.matchMedia?.('(prefers-color-scheme: dark)')
  mq?.addEventListener?.('change', schedule)
  return {
    stop() {
      root.disconnect()
      head.disconnect()
      mq?.removeEventListener?.('change', schedule)
    },
  }
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
