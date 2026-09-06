/*
 * "Is this issue mine?" — the one place that answers it.
 *
 * Identity arrives from GET auth/me/ (me.svelte.ts: accountId, email). Issues
 * carry both an id and an email per role, and either may be missing: a site
 * that hides emails leaves assignee_email empty (the skill file's warning),
 * an older mirror may lack assignee_id. So the match is id first, then a
 * case-insensitive email — never a display name (names localize and collide).
 *
 * Pure and store-free on purpose: the filters store ("mine" / "delegated"
 * flags), the detail panel (WIP count) and the session strip all import it,
 * so one rule serves every surface (THEORY.md §4½ — same data, two stances).
 *
 * Known gap: on the built-in tracker with no credential the identity is an
 * actor slug (`claude:…`) and me.accountId is null, so nothing matches —
 * the views that need identity stay hidden there (GDK draft: retro self
 * identity on the built-in tracker).
 */

import type { IssueLite } from './types'

export interface PersonRef {
  accountId: string | null
  email: string | null
}

/** id first, then email (case-insensitive). Empty on either side never matches. */
export function isSamePerson(
  id: string | null | undefined,
  email: string | null | undefined,
  me: PersonRef,
): boolean {
  if (me.accountId && id && id === me.accountId) return true
  if (me.email && email && email.toLowerCase() === me.email.toLowerCase()) return true
  return false
}

/** The issue's assignee is `me`. */
export function assignedTo(issue: IssueLite, me: PersonRef): boolean {
  return isSamePerson(issue.assignee_id, issue.assignee_email, me)
}

/** The issue's reporter is `me`. */
export function reportedBy(issue: IssueLite, me: PersonRef): boolean {
  return isSamePerson(issue.reporter_id, issue.reporter_email, me)
}

/** Reported by `me`, held by someone else (or nobody): the delegation ledger's row. */
export function delegatedBy(issue: IssueLite, me: PersonRef): boolean {
  return reportedBy(issue, me) && !assignedTo(issue, me)
}
