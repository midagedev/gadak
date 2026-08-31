/*
 * The strip's arithmetic (GDK-1153 / GDK-1163): what one row is called, and
 * what state it is in. Pure on purpose — the strip is the first surface in
 * this app whose whole content is derived from a list the server hands over,
 * so the derivation is testable without a DOM, a socket or a shell.
 *
 * Wire shape is internal/term.Info (manager.go). Only the fields a row
 * actually reads are declared: a client mirror that copies every field is a
 * second schema to keep in step.
 */

/** One row of GET /api/v1/terminal/sessions/ — the fields a row reads. */
export interface TerminalSessionInfo {
  id: string
  /** The issue this session was claimed for; absent when none (GDK-1158). */
  issue_key?: string
  /** How many clients are watching. The pane is one of them. */
  attached?: number
  /** RFC3339; zero-valued for a session that has never printed. */
  last_output_at?: string
  /** Every process on the session's controlling terminal, the shell
   *  included. Absent where the enumerator cannot see a tty. */
  pids?: number[]
  /** When the session last lost its final attachment; zero while attached. */
  detached_at?: string
  /** A BEL went through the ring and nobody has attached since. */
  needs_attention?: boolean
}

/**
 * The four states a row can be in.
 *
 * - `needs` — the session rang for a person (BEL). The one state that is a
 *   request, not a description.
 * - `running` — it printed within RUNNING_WINDOW_MS.
 * - `quiet` — nothing lately, but something is still there: a watcher, or a
 *   process besides the shell on the tty.
 * - `ghost` — nobody watching, nothing running. This is the row the
 *   reconnect grace is about to reap.
 *
 * `pids` is read for the quiet/ghost split only. It must never be read for
 * the `needs` decision: "a process on the tty" answers "may I reap you",
 * and an agent parked at a prompt with a child process is busy to that
 * question and blocked on you to this one (internal/term/session.go,
 * idleForReap).
 */
export type TerminalSessionState = 'needs' | 'running' | 'quiet' | 'ghost'

/** How recently a session must have printed to read as running. One breath:
 *  long enough that a build's quiet stretch between lines does not flicker,
 *  short enough that "running" means now. */
export const RUNNING_WINDOW_MS = 6_000

/** How much of a session id stands in for a name when no issue does. */
export const SHORT_ID_CHARS = 8

/** Go's zero time on the wire. A session that never printed carries it. */
function instantMs(iso: string | undefined): number | null {
  if (!iso) return null
  const ms = Date.parse(iso)
  if (Number.isNaN(ms)) return null
  // time.Time{} marshals to year 1; treat it as "never", not "long ago".
  return ms <= 0 && iso.startsWith('0001-') ? null : ms
}

/**
 * What the row is called. The issue binding is the whole point of the strip
 * — "this terminal is this ticket's" — so it wins whenever it exists; the
 * id prefix is the fallback that at least tells two unnamed shells apart.
 */
export function sessionLabel(info: TerminalSessionInfo): string {
  const key = info.issue_key?.trim()
  if (key) return key
  return info.id.length > SHORT_ID_CHARS ? `${info.id.slice(0, SHORT_ID_CHARS)}…` : info.id
}

/** True when this row has a claimed issue rather than a fallback id. */
export function sessionNamedByIssue(info: TerminalSessionInfo): boolean {
  return !!info.issue_key?.trim()
}

export function sessionState(info: TerminalSessionInfo, nowMs: number): TerminalSessionState {
  if (info.needs_attention) return 'needs'
  const last = instantMs(info.last_output_at)
  if (last !== null && nowMs - last < RUNNING_WINDOW_MS) return 'running'
  if ((info.attached ?? 0) > 0) return 'quiet'
  // More than the shell itself on the tty means work is still there.
  if ((info.pids?.length ?? 0) > 1) return 'quiet'
  return 'ghost'
}

/** One rendered row: everything the component paints, decided here. */
export interface StripRow {
  id: string
  label: string
  namedByIssue: boolean
  state: TerminalSessionState
  /** ISO instant the row's elapsed reads from, null when it never printed. */
  since: string | null
  selected: boolean
}

/**
 * The rows, oldest first — the order the server already returns, which is
 * creation order, so a row never moves under the pointer.
 *
 * `selected` is the pane's own attachment, not the server's `attached`: two
 * clients can watch one session, and the strip's job is to say which one
 * *this* pane is showing.
 */
export function stripRows(
  sessions: readonly TerminalSessionInfo[],
  selectedId: string | null,
  nowMs: number,
): StripRow[] {
  return sessions.map((info) => ({
    id: info.id,
    label: sessionLabel(info),
    namedByIssue: sessionNamedByIssue(info),
    state: sessionState(info, nowMs),
    since: instantMs(info.last_output_at) === null ? null : (info.last_output_at ?? null),
    selected: info.id === selectedId,
  }))
}

/**
 * Which session the pane should hold once `killedId` is gone (GDK-1200).
 *
 * A kill that was not aimed at the shown session moves nothing. Killing the
 * shown one hands the pane to the right-hand neighbour first — the tab the
 * eye falls on when the killed one collapses — then the left, and null when
 * the roster is about to be empty (the pane's own exit path takes it from
 * there).
 */
export function nextSelectedAfterKill(
  ids: readonly string[],
  killedId: string,
  selectedId: string | null,
): string | null {
  if (selectedId !== killedId) return selectedId
  const i = ids.indexOf(killedId)
  if (i === -1) return null
  return ids[i + 1] ?? ids[i - 1] ?? null
}
