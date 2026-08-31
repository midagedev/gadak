/*
 * Which card moved, and whose hand moved it (GDK-1175) — the judgment half.
 *
 * Split from ./board-moves.svelte.ts for the same reason ./issue-shells.ts is
 * split from its runes sibling: the decision is a pure function of two rows
 * and belongs in plain vitest, while "who is asking" needs the reactive
 * graph.
 *
 * The board's signature is an asymmetry: a move this tab made is silent —
 * the card is already where you put it, so animating it there would be the
 * app re-enacting your own gesture — while a move that arrived from
 * somewhere else (a `gadak transition` in the next window, an agent, another
 * client) flies.
 *
 * The distinction needs no new bookkeeping, because the write path already
 * draws it. `write.#writeIssue` patches `issues.pool` optimistically and then
 * confirms with the server's row, so by the time the mirror tick carries that
 * same write back as a delta, the pool already holds it and the comparison
 * below is equal. Only a change this tab did not make arrives as a difference.
 *
 * Scope of "moved": a change of status identity. That is the board's default
 * axis, and it is the axis GDK-1175 is about.
 * ponytail: status only — widen `movedExternally` when a non-status axis
 * (assignee, actor) grows a board of its own.
 */

import type { IssueLite } from './types'

/**
 * How long a flagged move stays worth animating.
 *
 * A flag is a statement about the frame that is about to paint. If the board
 * is not mounted when it lands — a delta while the list is up, or while the
 * window is in another screen — the move is history by the time a board
 * exists, and history should render in place rather than fly in from a
 * position no one saw.
 */
export const MOVE_FRESH_MS = 1_000

/**
 * True when `next` puts the issue in a different status than `prev` held.
 *
 * Keyed on `status_id` when both rows carry one and on `status_category`
 * otherwise — never on the display name, which is a different string per
 * account language and would make this either always or never true
 * (CLAUDE.md's invariant). A row with no predecessor is an arrival, not a
 * move: it has no old position to fly from.
 */
export function movedExternally(prev: IssueLite | undefined, next: IssueLite): boolean {
  if (!prev) return false
  if (prev.status_id && next.status_id) return prev.status_id !== next.status_id
  return prev.status_category !== next.status_category
}

/**
 * How long this tab's own transition keeps its mirror echo silent (GDK-1176).
 *
 * The comparison above is blind to one ordering: the optimistic patch writes
 * the new status *name* but keeps the old `status_id` (a Transition does not
 * carry the target id), so a delta that lands before the origin confirm sees
 * old id → new id and reads this tab's own write as somebody else's move.
 * `write.transition` flags the key before it patches; a flagged key's next
 * echo is silent. Long enough for a slow origin write plus a tick; a genuine
 * outside move of the same issue inside this window loses only its animation.
 */
export const SELF_ECHO_MS = 10_000

/** True while a key flagged at `notedAt` is still this tab's own echo. */
export function selfEchoFresh(notedAt: number | undefined, nowMs: number): boolean {
  return notedAt !== undefined && nowMs - notedAt < SELF_ECHO_MS
}

