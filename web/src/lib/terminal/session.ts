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
 * Page unload deliberately does nothing: navigator.sendBeacon is a POST, and
 * DELETE is the only close verb. The 60 s reconnect grace reaps a session
 * that nobody reattached to.
 */

import { config, isDesktop } from '../config'
import { coerceDroppedReason } from './protocol'
import type { DroppedReason, SocketHandle, SocketHandlers } from './protocol'
import { openWailsSessionSocket } from './wails-stream'

// The wire vocabulary lives in ./protocol so the two transports do not import
// each other; re-exported here because this is where the pane already looks.
export { coerceDroppedReason }
export type { DroppedReason, SocketHandle, SocketHandlers } from './protocol'

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

/** Why create/attach ended in unavailable — keyed on the server's error
 *  *code* (and HTTP status as fallback), never on a localized string. */
export type UnavailableCause = 'unsupported' | 'forbidden' | 'failed' | 'network'

const FORBIDDEN_CODES: ReadonlySet<string> = new Set([
  'scope_rejected',
  'forbidden_host',
  'pairing_rejected',
])

export function classifyCreateFail(err: unknown): {
  cause: UnavailableCause
  detail: string | null
} {
  if (err instanceof TerminalHttpError) {
    if (err.code === 'terminal_unsupported' || err.status === 501) {
      return { cause: 'unsupported', detail: null }
    }
    if (
      (err.code !== null && FORBIDDEN_CODES.has(err.code)) ||
      err.status === 401 ||
      err.status === 403
    ) {
      return { cause: 'forbidden', detail: null }
    }
    if (err.code === 'terminal_failed' || err.status === 500) {
      const detail = err.serverMessage?.trim() || err.message
      return { cause: 'failed', detail }
    }
    if (err.status === 0 || err.code === 'unreachable') {
      return { cause: 'network', detail: null }
    }
  }
  return { cause: 'network', detail: null }
}

/** First-connect attach that never opened retries once, using the same
 *  backoff the live reconnect path already uses. null = do not retry. */
export function firstAttachRetryDelayMs(attempt: number): number | null {
  if (attempt !== 0) return null
  return TERMINAL_RECONNECT_BACKOFF_MS[0]
}

export function unavailableAllowsRestart(cause: UnavailableCause): boolean {
  return cause !== 'unsupported'
}

export function droppedAllowsRestart(reason: DroppedReason): boolean {
  return reason !== 'token_revoked'
}

export interface SessionDoc {
  id: string
  cols: number
  rows: number
}

export async function createSession(cols: number, rows: number): Promise<SessionDoc> {
  const url = `${terminalBase()}sessions/`
  let res: Response
  try {
    res = await fetch(url, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ cols, rows }),
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
  return (await res.json()) as SessionDoc
}

export async function listSessions(): Promise<SessionDoc[]> {
  const url = `${terminalBase()}sessions/`
  const res = await fetch(url, { credentials: 'same-origin' })
  if (!res.ok) return []
  const body = (await res.json()) as { sessions?: SessionDoc[] }
  return body.sessions ?? []
}

export async function deleteSession(id: string): Promise<void> {
  await fetch(`${terminalBase()}sessions/${encodeURIComponent(id)}/`, {
    method: 'DELETE',
    credentials: 'same-origin',
  })
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

/** Module-level session id: closing the pane keeps this so a reopen inside
 *  the grace reattaches and the ring replay brings scrollback back. */
let keptSessionId: string | null = null

export function peekSessionId(): string | null {
  return keptSessionId
}

export function rememberSessionId(id: string | null): void {
  keptSessionId = id
}
