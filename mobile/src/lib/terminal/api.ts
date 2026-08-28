// Terminal REST (GDK-865). Every call takes an explicit session built from
// the terminal token — never the configured module session in lib/api.ts,
// which holds the serve token. A serve-scope Bearer is 403 scope_rejected
// on this surface (internal/server/terminal.go terminalGate).
//
// The probe is GET terminal/sessions/: the cheapest call the gate answers
// truthfully for every failure mode. A phone that scanned a serve QR into
// the terminal slot must learn that here, not from a shell that never opens.

import { request, ApiError, type ApiSession, type FetchLike } from '../api'

/**
 * Terminal behavior the create response carries (GDK-896 R3; the web
 * sibling lives in web/src/lib/terminal/session.ts). The server's
 * EffectiveTerminal is the single owner of the values; these constants
 * are only the fallback for a response that predates the fields (an old
 * serve behind a new app build) and match the server's own defaults, so
 * both sides answer 5000/false when unset.
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

/** The wire shape before normalization: the behavior fields are optional
 *  because an older serve never sent them. */
type RawSessionDoc = Omit<SessionDoc, 'scrollback' | 'cursorBlink'> &
  Partial<Pick<SessionDoc, 'scrollback' | 'cursorBlink'>>

/** Fills the behavior fields an older serve never sent with the fallback
 *  defaults. id/cols/rows stay trusted as before — every server version
 *  that can answer at all has answered with those. */
export function normalizeSessionDoc(raw: RawSessionDoc): SessionDoc {
  return {
    ...raw,
    scrollback:
      typeof raw.scrollback === 'number' && Number.isFinite(raw.scrollback) && raw.scrollback > 0
        ? Math.floor(raw.scrollback)
        : TERMINAL_SCROLLBACK_FALLBACK,
    cursorBlink: typeof raw.cursorBlink === 'boolean' ? raw.cursorBlink : TERMINAL_CURSOR_BLINK_FALLBACK,
  }
}

/** A listed session row — the server's term.Info: identity, geometry, and
 *  process facts. Behavior never travels on this shape (GDK-896 R3: it
 *  rides the create response only — internal/server/terminal.go answers
 *  list with term.Info), so no behavior field is claimed here. */
export interface ListedSessionDoc {
  id: string
  pid: number
  cols: number
  rows: number
}

function pathFor(id?: string): string {
  return id === undefined
    ? 'terminal/sessions/'
    : `terminal/sessions/${encodeURIComponent(id)}/`
}

export async function createShellSession(
  cols: number,
  rows: number,
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<SessionDoc> {
  const env = await request<RawSessionDoc>(pathFor(), {
    session,
    method: 'POST',
    body: { cols, rows },
    fetchFn,
  })
  if (!env.body) throw new ApiError('bad_response', env.status)
  return normalizeSessionDoc(env.body)
}

export async function listShellSessions(
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<ListedSessionDoc[]> {
  const env = await request<{ sessions?: ListedSessionDoc[] }>(pathFor(), { session, fetchFn })
  return env.body?.sessions ?? []
}

export async function deleteShellSession(
  id: string,
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<void> {
  // request() parses JSON on every 2xx. DELETE is 204 No Content, so the
  // parse fails with ApiError('bad_response', 204). That status is success.
  try {
    await request(pathFor(id), { session, method: 'DELETE', fetchFn })
  } catch (err) {
    if (err instanceof ApiError && err.code === 'bad_response' && err.status === 204) return
    throw err
  }
}

/** Resolves when this token may open a shell on this endpoint; throws
 *  ApiError('scope_rejected') when it is the wrong kind of token. */
export async function probeShellPairing(
  endpoint: string,
  token: string,
  fetchFn?: FetchLike,
): Promise<void> {
  await listShellSessions({ endpoint, token }, fetchFn)
}
