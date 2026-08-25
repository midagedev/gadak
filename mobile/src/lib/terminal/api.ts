// Terminal REST (GDK-865). Every call takes an explicit session built from
// the terminal token — never the configured module session in lib/api.ts,
// which holds the serve token. A serve-scope Bearer is 403 scope_rejected
// on this surface (internal/server/terminal.go terminalGate).
//
// The probe is GET terminal/sessions/: the cheapest call the gate answers
// truthfully for every failure mode. A phone that scanned a serve QR into
// the terminal slot must learn that here, not from a shell that never opens.

import { request, ApiError, type ApiSession, type FetchLike } from '../api'

export interface SessionDoc {
  id: string
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
  const env = await request<SessionDoc>(pathFor(), {
    session,
    method: 'POST',
    body: { cols, rows },
    fetchFn,
  })
  if (!env.body) throw new ApiError('bad_response', env.status)
  return env.body
}

export async function listShellSessions(
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<SessionDoc[]> {
  const env = await request<{ sessions?: SessionDoc[] }>(pathFor(), { session, fetchFn })
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
