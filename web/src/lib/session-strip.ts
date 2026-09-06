/*
 * Session strip helpers (spec r2-session) — "what changed since the previous
 * session" as one line above the list (THEORY.md "Session start": the
 * session-start boundary; T2 the cost of returning is the cost of the work,
 * T3 change is the information).
 *
 * Pure functions only, the resume-card's shape: no fetch, no store reads, one
 * pass over the already-bootstrapped pool (UX §1's 50ms budget). The
 * component (components/list/SessionStrip.svelte) owns rendering and the
 * click; these decide whether there is anything to say at all. The absence of
 * the line is the design (G3/G5): no boundary, no changes → nothing rendered,
 * no empty state.
 *
 * The boundary itself (`last_session_ended_at` on bootstrap) is computed
 * server-side from local.db person reads — the same session rule `gadak
 * retro` splits by. The client never re-derives it: the count below is a
 * snapshot taken once when bootstrap lands, because a $derived over the pool
 * would grow it with every mid-session delta (G3 — the boundary is the
 * start).
 */

import type { TranslateFn } from './resume-card'
import { assignedTo, type PersonRef } from './person-match'
import type { IssueLite } from './types'
import { normalizeKeys } from './view-config'

/** The session gap, mirroring internal/retro SessionGap (30m) — one session
 *  rule on both sides. A tab hidden longer than this comes back to a new
 *  session (research F #24, 2026-09-07). */
export const SESSION_GAP_MS = 30 * 60 * 1000

/**
 * Re-latch (2026-09-07): a tab left open overnight never spoke again, while
 * `gadak retro` correctly counted two sessions. When the tab was hidden at
 * `hiddenAtMs` and is visible again at `nowMs`, the boundary of the new
 * session is the moment it went hidden — the last read of the previous one —
 * but only when the gap exceeded the session gap; otherwise null (the same
 * session, nothing to re-say). Still one utterance per real session: the
 * component recomputes its snapshot once on this boundary and latches again.
 */
export function relatchBoundary(
  hiddenAtMs: number | null,
  nowMs: number,
  gapMs = SESSION_GAP_MS,
): string | null {
  if (hiddenAtMs === null || !Number.isFinite(hiddenAtMs)) return null
  if (nowMs - hiddenAtMs <= gapMs) return null
  return new Date(hiddenAtMs).toISOString()
}

/** The strip's frozen answer: which issues changed and how many are mine. */
export interface SessionDelta {
  /** Changed issue keys in pool order. Uncapped — the label counts every
   *  change; only the keys handed to the view are capped (viewKeys). */
  keys: string[]
  /** Changed issues assigned to `me` (person-match's one rule). */
  mine: number
}

/**
 * Issues whose `updated_at` parses strictly after `since` — parsed dates, not
 * strings (a +09:00 spelling of the same instant is the same instant), and
 * equality is not "after": a change at exactly the boundary was seen before
 * it. Issues without a parseable `updated_at` are skipped, not counted.
 * null when nothing changed: the caller renders no strip, never an empty one.
 */
export function changedSince(
  issues: Iterable<IssueLite>,
  since: string,
  me: PersonRef | null,
): SessionDelta | null {
  const sinceMs = Date.parse(since)
  if (Number.isNaN(sinceMs)) return null
  const out: SessionDelta = { keys: [], mine: 0 }
  for (const it of issues) {
    const at = Date.parse(it.updated_at ?? '')
    if (Number.isNaN(at) || at <= sinceMs) continue
    out.keys.push(it.issue_key)
    if (me && assignedTo(it, me)) out.mine += 1
  }
  return out.keys.length === 0 ? null : out
}

/**
 * The keys the click hands to the view: changedSince's list through
 * normalizeKeys — the same trim/uppercase/first-wins/KEYS_CAP cap the URL
 * parser applies, so the strip cannot build a view the view layer would
 * refuse. `capped` tells the caller the label's n and the view's row count
 * are about to differ.
 */
export function viewKeys(keys: readonly string[]): { keys: string[]; capped: boolean } {
  const n = normalizeKeys(keys)
  return { keys: n.keys, capped: n.given > n.keys.length }
}

/**
 * The one line: "Since last session 3d ago · 12 issues changed · 2 of them
 * assigned here". The mine part rides only when the identity resolved and at
 * least one change is mine (G5: quiet until there is a reason). Singular
 * forms are their own keys — the i18n runtime has no plural infrastructure
 * (resumeLabel's rule).
 */
export function stripLabel(
  delta: SessionDelta,
  sinceAgo: string,
  t: TranslateFn,
  me: PersonRef | null,
): string {
  const parts = [t('list.sessionSince', { ago: sinceAgo })]
  const n = delta.keys.length
  parts.push(t(n === 1 ? 'list.sessionChangedOne' : 'list.sessionChanged', { n }))
  if (me && delta.mine > 0) parts.push(t('list.sessionMine', { k: delta.mine }))
  return parts.join(' · ')
}
