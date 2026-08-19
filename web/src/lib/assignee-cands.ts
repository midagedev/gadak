/*
 * Assignee candidate ranking + server-search merge.
 *
 * One owner for the empty-query groups (me / reporter / recent / same team /
 * rest) and the typed list (local members + GET users/ fallback). Detail
 * AssigneePicker and the list BulkBar both consume this so a user the
 * detail search can find is also findable from list `a`.
 */

import { searchUsers } from './api'
import type { JiraUser, Member } from './types'

/** Cap on local member hits in typed mode — same bound AssigneePicker used. */
export const ASSIGNEE_TYPED_LOCAL_CAP = 8

export type AssigneeCandOrigin = 'member' | 'server'

export interface AssigneeCand {
  key: string
  account_id: string | null
  display_name: string
  email: string | null
  member?: Member
  avatar_url?: string | null
  label?: string
  origin: AssigneeCandOrigin
}

export interface AssigneeGroupContext {
  reporter?: Member
  teamGroup?: string | null
}

export function candOfMember(m: Member, label?: string): AssigneeCand {
  return {
    key: m.jira_account_id ?? m.email,
    account_id: m.jira_account_id ?? null,
    display_name: m.display_name || m.name,
    email: m.email,
    member: m,
    label,
    origin: 'member',
  }
}

export function candOfUser(u: JiraUser): AssigneeCand {
  return {
    key: u.account_id,
    account_id: u.account_id,
    display_name: u.display_name,
    email: u.email || null,
    avatar_url: u.avatar_url,
    origin: 'server',
  }
}

/**
 * Personalized groups (no query). Empty groups are dropped. Dedupes by
 * account_id across groups, first group wins.
 */
export function groupedAssigneeCands(input: {
  members: Iterable<Member>
  me?: Member
  context?: AssigneeGroupContext | null
  recentAccountIds: readonly string[]
  assignToMeLabel: string
  compare: (a: string, b: string) => number
}): AssigneeCand[][] {
  const memberByAccount = new Map<string, Member>()
  const assignable: Member[] = []
  for (const m of input.members) {
    if (m.jira_account_id) memberByAccount.set(m.jira_account_id, m)
    if (m.status !== 'RESIGN' && m.jira_account_id) assignable.push(m)
  }

  const seen = new Set<string>()
  const take = (list: (Member | undefined)[], label?: string): AssigneeCand[] => {
    const r: AssigneeCand[] = []
    for (const m of list) {
      const acc = m?.jira_account_id
      if (!m || !acc || seen.has(acc)) continue
      seen.add(acc)
      r.push(candOfMember(m, label))
    }
    return r
  }

  const byName = (a: Member, b: Member) =>
    input.compare(a.display_name || a.name, b.display_name || b.name)
  const team = input.context?.teamGroup
  const g1 = take([input.me], input.assignToMeLabel)
  const g2 = take([input.context?.reporter])
  const g3 = take(input.recentAccountIds.map((acc) => memberByAccount.get(acc)))
  const g4 = take(assignable.filter((m) => team && m.group === team).sort(byName))
  const g5 = take([...assignable].sort(byName))
  return [g1, g2, g3, g4, g5].filter((g) => g.length)
}

/**
 * Typed search: local members matching the query (cap), then server users
 * whose email is not already in the local hits.
 */
export function typedAssigneeCands(input: {
  query: string
  members: Iterable<Member>
  serverUsers: readonly JiraUser[]
  localCap?: number
}): AssigneeCand[] {
  const q = input.query.trim().toLowerCase()
  if (!q) return []
  const cap = input.localCap ?? ASSIGNEE_TYPED_LOCAL_CAP
  const seenEmail = new Set<string>()
  const local: AssigneeCand[] = []
  for (const m of input.members) {
    if (m.status === 'RESIGN') continue
    const hay = `${m.name} ${m.display_name ?? ''} ${m.email}`.toLowerCase()
    if (!hay.includes(q)) continue
    seenEmail.add(m.email.toLowerCase())
    local.push(candOfMember(m))
    if (local.length >= cap) break
  }
  const server = input.serverUsers
    .filter((u) => !u.email || !seenEmail.has(u.email.toLowerCase()))
    .map(candOfUser)
  return [...local, ...server]
}

export type ResolveAssigneeResult =
  | { ok: true; user: JiraUser }
  | { ok: false; reason: 'not-found' | 'search-failed' }

/** Members with an account_id assign immediately; others re-resolve via users/. */
export async function resolveAssigneeCand(c: AssigneeCand): Promise<ResolveAssigneeResult> {
  if (c.account_id) {
    return {
      ok: true,
      user: {
        account_id: c.account_id,
        display_name: c.display_name,
        email: c.email ?? '',
        avatar_url: c.avatar_url ?? '',
        active: true,
      },
    }
  }
  try {
    const res = await searchUsers(c.email || c.display_name)
    const match =
      res.users.find((u) => u.email && c.email && u.email.toLowerCase() === c.email.toLowerCase()) ??
      res.users[0]
    if (!match) return { ok: false, reason: 'not-found' }
    return { ok: true, user: match }
  } catch {
    return { ok: false, reason: 'search-failed' }
  }
}
