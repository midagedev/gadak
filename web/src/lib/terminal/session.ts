/*
 * Terminal HTTP + WebSocket client (GDK-864).
 *
 * Framing is the serve contract (internal/server/terminal.go):
 *   binary either way = PTY bytes
 *   text client→server = {"t":"resize","cols":N,"rows":N}
 *   text server→client = {"t":"exit","code":N} | {"t":"dropped","reason":"…"}
 *
 * Loopback needs no Bearer. Same-origin WebSocket passes the Origin guard.
 *
 * Inside Gadak.app there is no TCP port for a ws:// URL to reach, so
 * openSessionSocket picks a wails GoStream there instead (./wails-stream,
 * GDK-892). Both transports carry the same bytes and end in the same
 * callbacks; everything below this seam — REST, grace, the kept session id —
 * is shared.
 *
 * Page unload never closes a session (navigator.sendBeacon is a POST, and
 * unload is not a decision anyway): the 60 s reconnect grace is what reaps a
 * session nobody reattached to, and a reopen inside the grace reattaches and
 * replays the ring (the selected id, ./sessions.svelte.ts). That contract
 * stands (GDK-922). What the client does send is the explicit gesture — a
 * tab's × (GDK-1200) — through deleteSession below, the same REST DELETE the
 * server has always kept for its tests and e2e.
 */

import { config, isDesktop } from '../config'
import { classifyUnavailable, coerceDroppedReason } from './protocol'
import type { SocketHandle, SocketHandlers, UnavailableCause } from './protocol'
import { openWailsSessionSocket } from './wails-stream'

// The wire vocabulary lives in ./protocol so the two transports do not import
// each other; re-exported here because this is where the pane already looks.
export { coerceDroppedReason }
export {
  classifyUnavailable,
  droppedAllowsRestart,
  unavailableAllowsRestart,
  UNAVAILABLE_KEYS,
} from './protocol'
export type { DroppedReason, SocketHandle, SocketHandlers, UnavailableCause } from './protocol'

export const TERMINAL_GRACE_MS = 60_000
export const TERMINAL_RECONNECT_BACKOFF_MS = [500, 1000, 2000, 4000] as const
/** Bound for "WS that never opens" (desktop / wails:// has no TCP socket). */
export const TERMINAL_WS_OPEN_MS = 8_000

/** Sibling of dashboardsBase(): apiBase ends in issues/, this swaps the suffix
 *  so /w/<name>/ mounts keep working. */
export function terminalBase(): string {
  return config().apiBase.replace(/issues\/$/, 'terminal/')
}

export function terminalWsUrl(id: string): string {
  const path = `${terminalBase()}sessions/${encodeURIComponent(id)}/ws/`
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  if (path.startsWith('/')) return `${scheme}//${location.host}${path}`
  const u = new URL(path, location.href)
  u.protocol = scheme
  return u.toString()
}

export class TerminalHttpError extends Error {
  constructor(
    readonly status: number,
    readonly code: string | null,
    /** `message` from failMsg; fail() bodies have none. */
    readonly serverMessage: string | null = null,
  ) {
    super(serverMessage || code || `terminal HTTP ${status}`)
    this.name = 'TerminalHttpError'
  }
}

/** The browser's adapter onto the shared classifier: unwrap this surface's
 *  error class, and carry the host's own words for a `failed`. */
export function classifyCreateFail(err: unknown): {
  cause: UnavailableCause
  detail: string | null
} {
  if (!(err instanceof TerminalHttpError)) return { cause: 'network', detail: null }
  const cause = classifyUnavailable(err.status, err.code)
  if (cause !== 'failed') return { cause, detail: null }
  return { cause, detail: err.serverMessage?.trim() || err.message }
}

/** First-connect attach that never opened retries once, using the same
 *  backoff the live reconnect path already uses. null = do not retry. */
export function firstAttachRetryDelayMs(attempt: number): number | null {
  if (attempt !== 0) return null
  return TERMINAL_RECONNECT_BACKOFF_MS[0]
}

/**
 * Terminal behavior the create response carries (GDK-896 R2). The server's
 * EffectiveTerminal is the single owner of the values; these constants are
 * only the fallback for a response that predates the fields (an old serve
 * behind a new bundle, or an old desktop bundle's backend) and match the
 * server's own defaults, so both sides answer 5000/false when unset.
 */
export const TERMINAL_SCROLLBACK_FALLBACK = 5000
export const TERMINAL_CURSOR_BLINK_FALLBACK = false

export interface SessionDoc {
  id: string
  cols: number
  rows: number
  scrollback: number
  cursorBlink: boolean
}

/** Fills the behavior fields an older server never sent with the fallback
 *  defaults. id/cols/rows stay trusted as before — every server version
 *  that can answer at all has answered with those. */
export function normalizeSessionDoc(
  raw: Omit<SessionDoc, 'scrollback' | 'cursorBlink'> &
    Partial<Pick<SessionDoc, 'scrollback' | 'cursorBlink'>>,
): SessionDoc {
  return {
    ...raw,
    scrollback:
      typeof raw.scrollback === 'number' && Number.isFinite(raw.scrollback) && raw.scrollback > 0
        ? Math.floor(raw.scrollback)
        : TERMINAL_SCROLLBACK_FALLBACK,
    cursorBlink: typeof raw.cursorBlink === 'boolean' ? raw.cursorBlink : TERMINAL_CURSOR_BLINK_FALLBACK,
  }
}

/** What a create may carry besides its size (GDK-1388 / GDK-1195). */
export interface CreateSessionExtra {
  /** Bind the new shell to this issue from its first prompt. */
  issueKey?: string
  /** Label it at birth. */
  name?: string
}

export async function createSession(
  cols: number,
  rows: number,
  extra: CreateSessionExtra = {},
): Promise<SessionDoc> {
  const url = `${terminalBase()}sessions/`
  const body: Record<string, unknown> = { cols, rows }
  if (extra.issueKey) body.issue_key = extra.issueKey
  if (extra.name) body.name = extra.name
  let res: Response
  try {
    res = await fetch(url, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
  } catch {
    throw new TerminalHttpError(0, 'unreachable')
  }
  if (!res.ok) {
    let code: string | null = null
    let message: string | null = null
    try {
      const body = (await res.json()) as { error?: unknown; message?: unknown }
      code = typeof body.error === 'string' ? body.error : null
      message =
        typeof body.message === 'string' && body.message.trim() !== '' ? body.message : null
    } catch {
      /* non-JSON */
    }
    throw new TerminalHttpError(res.status, code, message)
  }
  return normalizeSessionDoc(await res.json())
}

/**
 * The explicit close verb (GDK-1200): a person aimed an × at this session.
 * Errors are swallowed on purpose — a 404 means it is already gone, and the
 * roster poll is the authority on what is left either way.
 */
export async function deleteSession(id: string): Promise<void> {
  try {
    await fetch(`${terminalBase()}sessions/${encodeURIComponent(id)}/`, {
      method: 'DELETE',
      credentials: 'same-origin',
    })
  } catch {
    /* the roster poll reconciles */
  }
}

/**
 * The transport picker (GDK-892). Gadak.app mounts the same handler behind
 * the wails asset server and opens no TCP port, so there is no `ws:` URL for
 * a browser network stack to reach — the desktop carries the identical bytes
 * over a wails GoStream instead. isDesktop() (lib/config.ts) is the existing
 * owner of "am I in the app"; this adds no second answer to that question.
 */
export function openSessionSocket(id: string, handlers: SocketHandlers): SocketHandle {
  return isDesktop()
    ? openWailsSessionSocket(id, handlers)
    : openWebSocketSession(id, handlers)
}

function openWebSocketSession(id: string, handlers: SocketHandlers): SocketHandle {
  const ws = new WebSocket(terminalWsUrl(id))
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
  ws.addEventListener('message', (ev: MessageEvent<ArrayBuffer | string>) => {
    if (typeof ev.data === 'string') {
      let msg: { t?: string; code?: number; reason?: string }
      try {
        msg = JSON.parse(ev.data) as { t?: string; code?: number; reason?: string }
      } catch {
        return
      }
      if (msg.t === 'exit') handlers.onExit(typeof msg.code === 'number' ? msg.code : 0)
      else if (msg.t === 'dropped') handlers.onDropped(coerceDroppedReason(msg.reason))
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
      if (ws.readyState === WebSocket.OPEN) ws.send(data)
    },
    resize(cols: number, rows: number) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ t: 'resize', cols, rows }))
      }
    },
    close() {
      if (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN) {
        ws.close()
      }
    },
  }
}

/*
 * The kept session id used to live here as a module-level `let`, and that
 * `let` was one of the two things making a fully multi-session server look
 * single-session to a person. It moved to ./sessions.svelte.ts (GDK-1153),
 * which is now the single owner of "which session is the pane on" — the
 * strip, a create, an exit and a reopen inside the grace all set it there.
 * This module keeps no second copy: it is the transport, not the selector.
 */
