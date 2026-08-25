// Pure domain logic for the queue and search — no DOM, no fetch, fully
// unit-tested. Repo contract: logic keys on status_category and
// priority_rank, never on display names (display names are labels only).

import type { IssueLite, Me } from './types'

/** Open = not in the done category. Resolution text is not consulted. */
export function openIssues(issues: IssueLite[]): IssueLite[] {
  return issues.filter((i) => i.status_category !== 'done')
}

/**
 * Mine = assigned to the paired identity. Account id wins (stable across
 * localized names); email is the fallback for mirrors that predate ids.
 */
export function isMine(issue: IssueLite, me: Me | null): boolean {
  if (!me) return false
  if (me.account_id && issue.assignee_id) return issue.assignee_id === me.account_id
  if (me.email && issue.assignee_email) return issue.assignee_email === me.email
  return false
}

/** True when the serve knows who its user is at all. */
export function hasIdentity(me: Me | null): boolean {
  return !!me && (!!me.account_id || !!me.email)
}

/** Rank 0 means the mirror never saw a priority — sort those last, not first. */
function rankKey(i: IssueLite): number {
  return i.priority_rank > 0 ? i.priority_rank : Number.MAX_SAFE_INTEGER
}

/** priority_rank asc, then updated_at desc, then key for stability. */
export function sortQueue(issues: IssueLite[]): IssueLite[] {
  return [...issues].sort((a, b) => {
    const r = rankKey(a) - rankKey(b)
    if (r !== 0) return r
    const u = (b.updated_at ?? '').localeCompare(a.updated_at ?? '')
    if (u !== 0) return u
    return a.issue_key.localeCompare(b.issue_key)
  })
}

export interface PrioritySection {
  /** Display label — never used as a key by logic. */
  label: string
  rank: number
  issues: IssueLite[]
}

/** Groups a sorted queue into priority sections, in rank order. */
export function groupByPriority(sorted: IssueLite[]): PrioritySection[] {
  const sections: PrioritySection[] = []
  for (const issue of sorted) {
    const rank = rankKey(issue)
    const last = sections[sections.length - 1]
    if (last && last.rank === rank) {
      last.issues.push(issue)
    } else {
      sections.push({ label: issue.priority ?? 'No priority', rank, issues: [issue] })
    }
  }
  return sections
}

export type QueueScope = 'mine' | 'all'

export interface QueueView {
  sections: PrioritySection[]
  total: number
  /** The scope actually shown (a Mine request falls back to All when empty). */
  scope: QueueScope
  /** True when a Mine request fell back to All. */
  fellBack: boolean
}

/** The queue in one call: filter, scope (with honest fallback), sort, group. */
export function buildQueue(issues: IssueLite[], me: Me | null, want: QueueScope): QueueView {
  const open = openIssues(issues)
  if (want === 'mine' && hasIdentity(me)) {
    const mine = open.filter((i) => isMine(i, me))
    if (mine.length > 0) {
      const sorted = sortQueue(mine)
      return { sections: groupByPriority(sorted), total: mine.length, scope: 'mine', fellBack: false }
    }
    const sorted = sortQueue(open)
    return { sections: groupByPriority(sorted), total: open.length, scope: 'all', fellBack: true }
  }
  const sorted = sortQueue(open)
  return { sections: groupByPriority(sorted), total: open.length, scope: 'all', fellBack: want === 'mine' }
}

/** Instant local match over key + summary, case-insensitive. */
export function matchLocal(issues: IssueLite[], query: string): IssueLite[] {
  const q = query.trim().toLowerCase()
  if (q === '') return []
  return sortQueue(
    issues.filter(
      (i) => i.issue_key.toLowerCase().includes(q) || i.summary.toLowerCase().includes(q),
    ),
  )
}

/** Merges server search keys into local hits without duplicating rows. */
export function mergeSearch(local: IssueLite[], serverKeys: string[], all: IssueLite[]): IssueLite[] {
  const seen = new Set(local.map((i) => i.issue_key))
  const byKey = new Map(all.map((i) => [i.issue_key, i]))
  const merged = [...local]
  for (const key of serverKeys) {
    if (seen.has(key)) continue
    const row = byKey.get(key)
    if (row) {
      merged.push(row)
      seen.add(key)
    }
  }
  return merged
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** Compact relative time: now / 5m / 3h / 2d / Aug 12. Bad input → ''. */
export function relTime(iso: string | null | undefined, now: Date = new Date()): string {
  if (!iso) return ''
  const t = new Date(iso)
  if (isNaN(t.getTime())) return ''
  const sec = Math.floor((now.getTime() - t.getTime()) / 1000)
  if (sec < 60) return 'now'
  if (sec < 3600) return `${Math.floor(sec / 60)}m`
  if (sec < 86400) return `${Math.floor(sec / 3600)}h`
  if (sec < 7 * 86400) return `${Math.floor(sec / 86400)}d`
  return `${MONTHS[t.getMonth()]} ${t.getDate()}`
}

/** Status token for the ink spine: reopened rows override their category. */
export function spineToken(issue: IssueLite): string {
  if (issue.reopen_count > 0 && issue.status_category !== 'done') return 'reopen'
  return issue.status_category || 'new'
}
