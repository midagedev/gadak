/*
 * Resume card helpers (spec w1-resume) — "what changed since this issue was
 * last opened" (THEORY.md T2: resumption is the designed half of
 * interruption; T3: change is the information).
 *
 * Pure functions only: no fetch, no store reads, one pass over already-loaded
 * detail (UX §1's 50ms budget — the card rides the detail response). The
 * component (components/detail/ResumeCard.svelte) owns rendering and the
 * scroll; these three decide whether there is anything to say at all. The
 * absence of a card is the design (G3/G5): no previous visit or no delta →
 * nothing rendered, no empty state.
 */

import type { MessageKey, MessageParams } from './i18n'
import type { DetailResponse } from './types'

/** A visit whose newest read is this close to "now" is this very open, not a
 *  previous one — the detail GET and the visit POST race, so both orders must
 *  land on the same boundary (see pickSince). */
export const SELF_VISIT_MS = 120_000

/** The per-part counts the card shows. All zero → no card (resumeDelta null). */
export interface ResumeDelta {
  statusChanges: number
  comments: number
  assigneeChanged: boolean
  other: number
}

/** i18n `t`, passed in so the helpers stay free of UI imports. */
export type TranslateFn = (key: MessageKey, params?: MessageParams) => string

function parse(iso: string | null | undefined): number | null {
  if (!iso) return null
  const ms = Date.parse(iso)
  return Number.isNaN(ms) ? null : ms
}

/**
 * The delta boundary: the previous person read of this issue, or null when
 * there is none to diff against.
 *
 * The server returns the two newest `ui` visits. The newest may be *this*
 * open — the client POSTs a visit when an issue opens (me.recordRecent),
 * racing the detail GET it is part of:
 *  - POST lands first → last_visited_at is seconds old → this open → the
 *    boundary is previous_visit_at;
 *  - GET lands first → last_visited_at is the previous real visit → it is
 *    already the boundary.
 * Both orders produce the same diff, which is why the freshness window
 * exists at all. A last_visited_at older than the window is never this open.
 */
export function pickSince(
  lastVisitedAt: string | null | undefined,
  previousVisitAt: string | null | undefined,
  now: number = Date.now(),
): string | null {
  const last = parse(lastVisitedAt)
  if (last === null) return null
  if (Math.abs(now - last) <= SELF_VISIT_MS) return previousVisitAt ?? null
  return lastVisitedAt ?? null
}

/**
 * Count what happened after `since`: history entries and comments whose
 * parsed time is strictly newer (a change at exactly the boundary was seen
 * on that visit — C7). History fields are counted, not named: status →
 * statusChanges, assignee → assigneeChanged, anything else → other.
 * null when nothing changed (or `since`/entries cannot be parsed): the
 * caller renders no card, never an empty one.
 */
export function resumeDelta(
  detail: Pick<DetailResponse, 'history' | 'comments'>,
  since: string | null | undefined,
): ResumeDelta | null {
  const sinceMs = parse(since)
  if (sinceMs === null) return null
  const delta: ResumeDelta = { statusChanges: 0, comments: 0, assigneeChanged: false, other: 0 }
  for (const entry of detail.history ?? []) {
    const at = parse(entry.at)
    if (at === null || at <= sinceMs) continue
    if (entry.field === 'status') delta.statusChanges += 1
    else if (entry.field === 'assignee') delta.assigneeChanged = true
    else delta.other += 1
  }
  for (const comment of detail.comments ?? []) {
    const at = parse(comment.created_at)
    if (at !== null && at > sinceMs) delta.comments += 1
  }
  const empty =
    delta.statusChanges === 0 &&
    delta.comments === 0 &&
    !delta.assigneeChanged &&
    delta.other === 0
  return empty ? null : delta
}

/**
 * The one line: "Since last opened 3d ago · 2 status changes · 1 new comment
 * · assignee changed". Zero parts are omitted, not rendered as 0 (G5: quiet
 * until there is a reason); `other` is last as the catch-all. Singular forms
 * are their own keys — the i18n runtime has no plural infrastructure.
 */
export function resumeLabel(delta: ResumeDelta, sinceAgo: string, t: TranslateFn): string {
  const parts = [t('detail.resume.sinceOpened', { ago: sinceAgo })]
  if (delta.statusChanges > 0) {
    parts.push(
      t(delta.statusChanges === 1 ? 'detail.resume.statusChangeOne' : 'detail.resume.statusChanges', {
        n: delta.statusChanges,
      }),
    )
  }
  if (delta.comments > 0) {
    parts.push(
      t(delta.comments === 1 ? 'detail.resume.newCommentOne' : 'detail.resume.newComments', {
        n: delta.comments,
      }),
    )
  }
  if (delta.assigneeChanged) parts.push(t('detail.resume.assigneeChanged'))
  if (delta.other > 0) {
    parts.push(
      t(delta.other === 1 ? 'detail.resume.otherChangeOne' : 'detail.resume.otherChanges', {
        n: delta.other,
      }),
    )
  }
  return parts.join(' · ')
}
