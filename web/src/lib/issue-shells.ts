/*
 * Which shell is on this issue, and which claim has none (GDK-1162/GDK-1164).
 *
 * A session carries `issue_key` since GDK-1158: `gadak claim` inside a pane
 * reflects the claim into the shell it ran in. That binding is the join this
 * file owns, in both directions —
 *
 *   forward  ▶ needs a shell to put a command in;
 *   backward an In Progress issue that no live shell is on.
 *
 * ── The thing this file must never become ──
 *
 * The backward direction detects; it does not act. No function here writes to
 * the origin, and none may grow one. "No shell is on this claim" is a fact
 * about *this serve right now*, and the truth of a claim is the origin's: a
 * laptop that slept for ten minutes is a serve that knows nothing, and a serve
 * that wrote its ignorance back would delete the claim of an agent still
 * running on another machine. That is the same defect wearing the opposite
 * sign. The server side of this rule is pinned in
 * internal/server/terminal_input_test.go.
 *
 * The wording of the mark carries the same honesty (i18n
 * detail.unattendedHint): bindings are runtime state that dies with the serve,
 * so "no shell here" is never "this work is dead".
 */

import type { IssueLite } from './types'
import { categoryFallbackSeen, effectiveCategory, type StatusCategory } from './view-config'
import { terminalBase } from './terminal/session'
import { sessionState, type TerminalSessionInfo, type TerminalSessionState } from './terminal/strip'

/**
 * The subset of `term.Info` this join reads (GET terminal/sessions/).
 *
 * It extends the strip's row shape rather than restating three fields,
 * because the board draws the strip's four states on its cards and a second
 * hand-written subset is how the two surfaces drift apart. `fetchShellSessions`
 * keeps whatever the wire sent, so the extra fields are really there.
 */
export interface ShellSession extends TerminalSessionInfo {
  /** A session whose shell has ended is still listed until it is reaped. */
  exited?: boolean
}

/** Sessions still able to take input. */
export function liveSessions(sessions: readonly ShellSession[]): ShellSession[] {
  return sessions.filter((s) => !s.exited)
}

/**
 * The live session bound to `key`, or null. First wins when several are: the
 * list is creation-ordered, so that is the oldest shell on the issue — the
 * one somebody has most likely been working in.
 */
export function shellForIssue(
  sessions: readonly ShellSession[],
  key: string | null | undefined,
): ShellSession | null {
  if (!key) return null
  return liveSessions(sessions).find((s) => s.issue_key === key) ?? null
}

/**
 * The category, only when the app's mapper actually recognized the string.
 *
 * effectiveCategory (view-config.ts) is the one owner of the fold, and its
 * fallback for an unknown key is 'inprogress' — the right default for
 * *filtering* (better to show an issue than hide it) and the wrong one for
 * *accusing*, because it would put the mark on every row whose status gadak
 * failed to mirror, and on every string that is not a category at all.
 *
 * Rather than copy the mapper's key list here — two lists that drift is how
 * "keyed by display name" comes back — this reads the fallback counter the
 * mapper already keeps. A value that moved the counter is one it did not
 * know, and this returns null for it.
 */
function recognizedCategory(raw: string): StatusCategory | null {
  const before = categoryFallbackSeen()
  const cat = effectiveCategory(raw)
  return categoryFallbackSeen() === before ? cat : null
}

/**
 * In progress here, with no live shell on it.
 *
 * Keyed on status_category, never on a display name: `status === 'In
 * Progress'` is silently false on a Korean or Japanese account, and this
 * judgment would then be "no issue is ever in progress" — a mark that never
 * appears, which nobody reports as a bug.
 *
 * A row with no category, or one carrying something that is not a category
 * (a display name is exactly that), is excluded rather than assumed.
 */
export function isUnattendedInProgress(
  issue: Pick<IssueLite, 'issue_key' | 'status_category'> | null | undefined,
  sessions: readonly ShellSession[],
): boolean {
  if (!issue) return false
  if (!issue.status_category || issue.status_category.trim() === '') return false
  if (recognizedCategory(issue.status_category) !== 'inprogress') return false
  return shellForIssue(sessions, issue.issue_key) === null
}

/**
 * Whether to *show* the mark — narrower than the judgment above, on purpose.
 *
 * With no live session at all, "nothing is bound to this key" is true of every
 * issue in the mirror and says nothing about any of them; a board where the
 * whole In Progress column is marked has been made useless rather than
 * honest. One shell running is what turns the statement into information:
 * some issue has a shell on it, and this one is not that issue.
 *
 * It also keeps the mark off surfaces that cannot see the whole table — a
 * serve with no terminal open, a hosted snapshot — without inventing a second
 * permission axis to ask about.
 */
export function shouldMarkUnattended(
  issue: Pick<IssueLite, 'issue_key' | 'status_category'> | null | undefined,
  sessions: readonly ShellSession[],
): boolean {
  if (liveSessions(sessions).length === 0) return false
  return isUnattendedInProgress(issue, sessions)
}

/**
 * The four states of the shell on `key`, or null when no live shell is on it.
 *
 * The whole join is free: a session is named by the issue it was claimed for
 * (GDK-1158), so a card knows its shell without asking for anything the
 * terminal strip was not already polling.
 */
export function shellStateForIssue(
  sessions: readonly ShellSession[],
  key: string | null | undefined,
  nowMs: number,
): TerminalSessionState | null {
  const s = shellForIssue(sessions, key)
  return s ? sessionState(s, nowMs) : null
}

/** Sibling of createSession()'s URL builder, for the two calls below. */
function sessionsUrl(): string {
  return `${terminalBase()}sessions/`
}

/**
 * Read the session table. Every failure answers [] — a gate refusal on a
 * paired host, a serve without the surface, an offline snapshot. The caller
 * treats "cannot see" and "nothing there" identically on purpose: both mean
 * this client has no evidence, and the surfaces above draw nothing without
 * evidence rather than guessing.
 */
export async function fetchShellSessions(): Promise<ShellSession[]> {
  try {
    const res = await fetch(sessionsUrl(), { credentials: 'same-origin' })
    if (!res.ok) return []
    const body = (await res.json()) as { sessions?: unknown }
    if (!Array.isArray(body.sessions)) return []
    return body.sessions.filter(
      (s): s is ShellSession => !!s && typeof (s as ShellSession).id === 'string',
    )
  } catch {
    return []
  }
}

/**
 * Put `text` at the prompt of `sessionId`. It is not run: the serve refuses a
 * payload carrying \n or \r, so this cannot become an execute call by adding
 * one — see internal/server/terminal.go's handleTerminalInput for why the
 * refusal lives there and not here.
 */
export async function placeInShell(sessionId: string, text: string): Promise<boolean> {
  try {
    const res = await fetch(`${sessionsUrl()}${encodeURIComponent(sessionId)}/input/`, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ text }),
    })
    return res.ok
  } catch {
    return false
  }
}
