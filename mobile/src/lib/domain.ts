// Pure domain logic for the issue list and search — no DOM, no fetch, fully
// unit-tested. Repo contract: logic keys on status_category and
// priority_rank, never on display names (display names are labels only).

import { t } from './i18n'
import type {
  DetailComment,
  IssueLite,
  Me,
  SavedViewDoc,
  SourceViewDoc,
  ViewFilters,
} from './types'

/**
 * Fold the aliases Jira and the older web mappers emit into the three
 * buckets logic may key on. Mirrors web/src/lib/view-config.ts
 * `effectiveCategory` — the desktop writes its saved views against that
 * folding, so the phone must read them the same way. `categoryAliases()`
 * below is the parity surface the drift test reads.
 */
const CATEGORY_ALIASES: Record<string, 'new' | 'inprogress' | 'done'> = {
  new: 'new',
  todo: 'new',
  inprogress: 'inprogress',
  indeterminate: 'inprogress',
  done: 'done',
  complete: 'done',
  completed: 'done',
}

/** The alias table, for the parity test against the desktop's owner. */
export function categoryAliases(): Record<string, string> {
  return { ...CATEGORY_ALIASES }
}

/** Effective status bucket. Unknown / missing reads as inprogress, like the desk. */
export function effectiveCategory(issue: IssueLite): 'new' | 'inprogress' | 'done' {
  return CATEGORY_ALIASES[(issue.status_category ?? '').toLowerCase()] ?? 'inprogress'
}

/** Open = not in the done category. Resolution text is not consulted. */
export function openIssues(issues: IssueLite[]): IssueLite[] {
  return issues.filter((i) => effectiveCategory(i) !== 'done')
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
export function sortIssues(issues: IssueLite[]): IssueLite[] {
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

/** Groups a sorted list into priority sections, in rank order. */
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

/* ── Scopes: the heading is the current scope's name (DESIGN.md §2) ── */

/** Which picker section a scope belongs to; also the order they render in. */
export type ScopeSection = 'me' | 'builtin' | 'views' | 'filters'

/** The desktop's hardcoded "Assigned to me" — not a saved view (personal.go sends none). */
export const SCOPE_ME = 'me'
/** The desktop builtin `all-open`, which the phone already ran as its "All". */
export const SCOPE_ALL_OPEN = 'builtin:all-open'

export interface Scope {
  id: string
  section: ScopeSection
  /**
   * Display name. Catalog-owned for the two hardcoded scopes; for saved views
   * and imported filters it is the name the developer typed at the desk.
   */
  name: string
  /** Stored desktop filters. null for the two hardcoded scopes. */
  filters: Partial<ViewFilters> | null
  /**
   * Axes the phone cannot evaluate on an `IssueLite`. Non-empty means the row
   * is offered disabled: showing the full list under someone else's view name
   * would be a lie (docs/decisions/0007 — refuse unsupported out loud).
   */
  unsupported: string[]
}

/**
 * The axes an `IssueLite` can actually answer. Everything else — labels,
 * actor, reporter, components, fix_versions, team_group, severity, qa_*,
 * deploy_*, source_project, date ranges, the text query, dynamic `fields` —
 * is either absent from the phone's row shape or needs data the snapshot does
 * not carry.
 */
const HONORED_AXES = new Set([
  'status_category',
  'status_category_not',
  'assignee_email',
  'assignee_email_not',
  'unassigned',
  'issue_type',
  'priority',
  'jira_project',
  'jira_project_not',
])

function axisIsSet(value: unknown): boolean {
  if (value == null) return false
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'string') return value !== ''
  if (typeof value === 'boolean') return value
  if (typeof value === 'object') return Object.keys(value as object).length > 0
  return true
}

/**
 * Names every axis this view sets that the phone cannot honor. Empty list =
 * the phone can paint this view faithfully.
 */
export function unsupportedAxes(filters: Partial<ViewFilters> | null | undefined): string[] {
  if (!filters) return ['config']
  const out: string[] = []
  for (const [axis, value] of Object.entries(filters)) {
    if (axis === 'fields') {
      for (const [alias, values] of Object.entries((value ?? {}) as Record<string, unknown>)) {
        if (axisIsSet(values)) out.push(`fields.${alias}`)
      }
      continue
    }
    if (!axisIsSet(value)) continue
    if (HONORED_AXES.has(axis)) continue
    out.push(axis)
  }
  return out.sort()
}

/** Jira project = the issue key's prefix (web/src/lib/…/filters `jiraProjectOf`). */
function projectOf(issue: IssueLite): string {
  const sep = issue.issue_key.indexOf('-')
  return sep > 0 ? issue.issue_key.slice(0, sep) : ''
}

/** Case-insensitive only for emails, matching the desktop's `sameIdentity`. */
function sameIdentity(a: string, b: string): boolean {
  return a === b || (a.includes('@') && b.includes('@') && a.toLowerCase() === b.toLowerCase())
}

/** One assignee filter value (an account id or a legacy email) against a row. */
function matchesAssignee(issue: IssueLite, value: string): boolean {
  if (!value) return false
  if (issue.assignee_id && sameIdentity(issue.assignee_id, value)) return true
  return !!issue.assignee_email && sameIdentity(issue.assignee_email, value)
}

/**
 * Stable id first, stored display name as the fallback — the desktop's
 * `matchesIdFirst`. The value in a saved view is whatever the desk's facet
 * produced for that axis, so comparing it to `issue_type` / `priority` is
 * consuming the desktop's stored contract, not the phone keying logic on a
 * display name (CLAUDE.md's display-name trap). Rows synced before the id
 * columns existed carry '' and fall through to the name on both surfaces.
 */
function matchesIdFirst(selected: string[], id: string, name: string | null): boolean {
  if (selected.length === 0) return true
  if (id && selected.includes(id)) return true
  return !!name && selected.includes(name)
}

function matchesMulti(include: string[], exclude: string[], value: string): boolean {
  if (include.length && !include.includes(value)) return false
  if (exclude.length && exclude.includes(value)) return false
  return true
}

const NONE: string[] = []

/** True when the row satisfies every honored axis of a stored view. */
export function matchesFilters(issue: IssueLite, f: Partial<ViewFilters>): boolean {
  const cat = effectiveCategory(issue)
  const sc = f.status_category ?? NONE
  if (sc.length && !sc.includes(cat)) return false
  const scNot = f.status_category_not ?? NONE
  if (scNot.length && scNot.includes(cat)) return false

  const asg = f.assignee_email ?? NONE
  if (asg.length && !asg.some((v) => matchesAssignee(issue, v))) return false
  const asgNot = f.assignee_email_not ?? NONE
  if (asgNot.length && asgNot.some((v) => matchesAssignee(issue, v))) return false
  if (f.unassigned && (issue.assignee_id || issue.assignee_email || issue.assignee)) return false

  if (!matchesIdFirst(f.issue_type ?? NONE, issue.issue_type_id, issue.issue_type)) return false
  if (!matchesIdFirst(f.priority ?? NONE, issue.priority_id, issue.priority)) return false
  if (!matchesMulti(f.jira_project ?? NONE, f.jira_project_not ?? NONE, projectOf(issue))) return false
  return true
}

/** In-memory apply over the snapshot — no server round trip (plan §5, move 2). */
export function applyFilters(issues: IssueLite[], f: Partial<ViewFilters>): IssueLite[] {
  return issues.filter((i) => matchesFilters(i, f))
}

/**
 * The picker's scope list, in section order. Names come from the desktop —
 * the catalog for the two hardcoded scopes, the developer's own text for the
 * rest. The phone invents nothing here.
 */
export function buildScopes(
  views: SavedViewDoc[],
  sources: SourceViewDoc[],
  me: Me | null,
): Scope[] {
  const out: Scope[] = []
  if (hasIdentity(me)) {
    out.push({
      id: SCOPE_ME,
      section: 'me',
      name: t('personal.myAssignee'),
      filters: null,
      unsupported: [],
    })
  }
  out.push({
    id: SCOPE_ALL_OPEN,
    section: 'builtin',
    name: t('view.allOpen.name'),
    filters: null,
    unsupported: [],
  })
  for (const v of views) {
    const filters = v.config?.filters ?? null
    out.push({
      id: `view:${v.id}`,
      section: 'views',
      name: v.name,
      filters,
      unsupported: unsupportedAxes(filters),
    })
  }
  for (const s of sources) {
    const filters = s.config?.filters ?? null
    // A clause the desktop's JQL importer could not compile is unsupported
    // here too — the compiled config simply does not carry it (decision 0007).
    const unsupported = [...new Set([...(s.unsupported ?? []), ...unsupportedAxes(filters)])].sort()
    out.push({ id: `source:${s.id}`, section: 'filters', name: s.name, filters, unsupported })
  }
  return out
}

/** The scope the app should paint: the wanted one, else the first offered. */
export function resolveScope(scopes: Scope[], wantId: string | null): Scope | null {
  const hit = scopes.find((s) => s.id === wantId && s.unsupported.length === 0)
  if (hit) return hit
  // A deleted (or newly unsupported) saved view falls back silently.
  return scopes.find((s) => s.unsupported.length === 0) ?? null
}

/** Rows a scope selects, before sorting. Null for a scope the phone refuses. */
export function scopeIssues(issues: IssueLite[], me: Me | null, scope: Scope): IssueLite[] | null {
  if (scope.unsupported.length > 0) return null
  if (scope.id === SCOPE_ME) {
    if (!hasIdentity(me)) return null
    return openIssues(issues).filter((i) => isMine(i, me))
  }
  if (scope.id === SCOPE_ALL_OPEN) return openIssues(issues)
  return scope.filters ? applyFilters(issues, scope.filters) : null
}

/** Match count for a picker row (GDK-886). Null = the row is disabled. */
export function scopeCount(issues: IssueLite[], me: Me | null, scope: Scope): number | null {
  const rows = scopeIssues(issues, me, scope)
  return rows === null ? null : rows.length
}

export interface IssueListView {
  sections: PrioritySection[]
  total: number
  /** The scope actually painted — an empty "Assigned to me" falls back. */
  scopeId: string
  /** True when the fallback fired, so the screen can say why. */
  fellBack: boolean
}

/** The list in one call: select by scope (with honest fallback), sort, group. */
export function buildList(issues: IssueLite[], me: Me | null, scope: Scope): IssueListView {
  const rows = scopeIssues(issues, me, scope)
  if (scope.id === SCOPE_ME && rows !== null && rows.length > 0) {
    const sorted = sortIssues(rows)
    return { sections: groupByPriority(sorted), total: rows.length, scopeId: SCOPE_ME, fellBack: false }
  }
  if (rows === null || (scope.id === SCOPE_ME && rows.length === 0)) {
    // No identity, an empty plate, or a scope the phone refuses: All open,
    // said out loud rather than an empty screen under someone else's name.
    const open = sortIssues(openIssues(issues))
    return {
      sections: groupByPriority(open),
      total: open.length,
      scopeId: SCOPE_ALL_OPEN,
      fellBack: true,
    }
  }
  const sorted = sortIssues(rows)
  return { sections: groupByPriority(sorted), total: rows.length, scopeId: scope.id, fellBack: false }
}

/** Instant local match over key + summary, case-insensitive. */
export function matchLocal(issues: IssueLite[], query: string): IssueLite[] {
  const q = query.trim().toLowerCase()
  if (q === '') return []
  return sortIssues(
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

/**
 * RAM overlay on a comment thread (DESIGN.md §5). Does not mutate `comments`.
 * The pending row is dropped when the origin reply already carries its id.
 */
export function overlayComments(
  comments: DetailComment[],
  pending: DetailComment | null,
): DetailComment[] {
  if (!pending) return comments
  if (comments.some((c) => c.comment_id === pending.comment_id)) return comments
  return [...comments, pending]
}

/** Temp comment for the overlay. The id is not an origin id and must not be persisted. */
export function pendingComment(
  text: string,
  me: Me | null,
  now: Date = new Date(),
  id = `temp-${now.getTime()}`,
): DetailComment {
  const author = me?.name || me?.email || null
  return {
    comment_id: id,
    author: author === '' ? null : author,
    created_at: now.toISOString(),
    body: text,
  }
}
