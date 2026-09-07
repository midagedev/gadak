/*
 * The phone's terminal session surface (GDK-1497 A6).
 *
 * The serve has been fully multi-session since GDK-864, and by GDK-1195 /
 * GDK-1158 / GDK-1162 it also answered rename, issue-binding and
 * place-a-line. The phone reached none of them: Shell.svelte created one
 * session, held its id in a module `let`, and never asked what else was
 * running. This module is the missing half — every verb the roster needs,
 * in one file, so a route is spelled once and a body field once.
 *
 * What is deliberately NOT here: the socket (./transport), the create
 * response's behavior fields (./api), and the derivation of what a row is
 * called. That last one is the web's `strip.ts`, a pure module with no
 * imports at all — the phone shares it rather than growing a second answer
 * to "what is this shell named", the way it already shares `protocol` and
 * `resize` (CLAUDE.md). Sharing it is also what keeps the two surfaces
 * saying the same words about the same shell.
 *
 * Every call takes an explicit ApiSession built from the *terminal* token
 * (store.svelte.ts `terminalSession()`), never the configured module
 * session, which holds the serve token — that one is 403 scope_rejected on
 * this whole surface (internal/server/terminal.go terminalGate).
 */

import { ApiError, request, type ApiSession, type FetchLike } from '../api'
import { deleteShellSession, listShellSessions, sessionPath } from './api'
import type { TerminalSessionInfo } from '../../../../web/src/lib/terminal/strip'

export type { TerminalSessionInfo }
export {
  nextSelectedAfterKill,
  sessionLabel,
  sessionState,
  stripRows,
  type StripRow,
  type TerminalSessionState,
} from '../../../../web/src/lib/terminal/strip'

/**
 * How often the roster refreshes while the session sheet is open. The web
 * pane polls this cadence for as long as it is open (sessions.svelte.ts
 * ROSTER_POLL_MS); the phone polls only while the sheet is up, because
 * this list travels over a tailnet on a battery, and the one row the
 * closed sheet still shows — the attached session's own label — changes
 * only when this device changes it.
 */
export const ROSTER_POLL_MS = 2_000

/**
 * The server's bound on one placed line, in bytes of `text`
 * (internal/server/terminal.go terminalInputMax, 4 KiB). Mirrored so the
 * phone can refuse before dialling instead of collecting a 400; the
 * server keeps its own copy either way — this is a courtesy, not the
 * authority.
 */
export const TERMINAL_INPUT_MAX_BYTES = 4 << 10

const encoder = new TextEncoder()

/**
 * Why this line may not be placed, or null when it may.
 *
 * The three refusals are the server's own, in its own wire codes
 * (handleTerminalInput): empty is a client bug, a newline would make an
 * execute endpoint out of a place-a-line one, and the bound is counted in
 * *bytes* — `len(req.Text)` in Go, so a Hangul line is three bytes a
 * character. Counting characters here would pass a line the serve then
 * refuses, which is the worst of both: a round trip and an error.
 */
export function inputLineRefusal(text: string): 'invalid_body' | 'input_not_a_line' | 'input_too_long' | null {
  if (/[\n\r]/.test(text)) return 'input_not_a_line'
  if (text.trim() === '') return 'invalid_body'
  if (encoder.encode(text).length > TERMINAL_INPUT_MAX_BYTES) return 'input_too_long'
  return null
}

/**
 * The roster, as the server last described it. One GET, shared with the
 * pairing probe (./api listShellSessions) so the phone has exactly one
 * reader of this path.
 */
export async function listSessions(
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<TerminalSessionInfo[]> {
  return listShellSessions(session, fetchFn)
}

/**
 * Give a session the label a person chose (GDK-1195). An empty name
 * clears it and the row falls back to its issue key, then to `shell {n}`
 * — the web's `sessionLabel` decides that, here as there.
 */
export async function renameSession(
  id: string,
  name: string,
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<TerminalSessionInfo> {
  return post(sessionPath(id, 'name'), { name: name.trim() }, session, fetchFn)
}

/**
 * Bind a session to an issue (GDK-1158), or clear it with an empty key.
 *
 * The key is normalized *here*, before the wire: the server stores what it
 * is given and never checks it against the mirror or the origin, because
 * the truth of a key is origin's. So a thumb's ' gdk-1497 ' would become a
 * binding that no card-to-shell join can match and no eye can tell from a
 * good one.
 */
export async function bindSessionIssue(
  id: string,
  issueKey: string,
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<TerminalSessionInfo> {
  return post(sessionPath(id, 'issue'), { issue_key: normalizeIssueKey(issueKey) }, session, fetchFn)
}

/** Trim and upper-case; an empty string stays empty (that is the clear). */
export function normalizeIssueKey(raw: string): string {
  return raw.trim().toUpperCase()
}

/**
 * Place one line in a session's shell without running it (GDK-1162), and
 * without being that session's socket. That second half is why the phone
 * has a use for this route at all: it holds one socket at a time, so this
 * is the only way to reach a shell it is not currently showing. Enter is
 * still a keystroke a person makes in front of the line they can see.
 *
 * Returns the byte count the server placed.
 */
export async function placeSessionInput(
  id: string,
  text: string,
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<number> {
  const refusal = inputLineRefusal(text)
  if (refusal) throw new ApiError(refusal, 0)
  const body = await post<{ placed?: number }>(
    sessionPath(id, 'input'),
    { text },
    session,
    fetchFn,
  )
  return typeof body.placed === 'number' ? body.placed : 0
}

/**
 * End a session on purpose. Re-exported from ./api rather than re-written:
 * the 204-is-success quirk has one owner.
 */
export async function deleteSession(
  id: string,
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<void> {
  return deleteShellSession(id, session, fetchFn)
}

/** The three POSTs' shared shape: JSON in, the updated row (or ack) out. */
async function post<T>(
  path: string,
  body: unknown,
  session: ApiSession,
  fetchFn?: FetchLike,
): Promise<T> {
  const env = await request<T>(path, { session, method: 'POST', body, fetchFn })
  if (!env.body) throw new ApiError('bad_response', env.status)
  return env.body
}
