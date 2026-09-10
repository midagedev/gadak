// Pure domain logic for the issue list and search — no DOM, no fetch, fully
// unit-tested. Repo contract: logic keys on status_category and
// priority_rank, never on display names (display names are labels only).

import { collator, locale, t, type MessageKey } from './i18n'
import type {
  DetailComment,
  DetailResponse,
  FlowSummary,
  IssueLite,
  Me,
  PageLite,
  SavedViewDoc,
  SearchMatch,
  SourceViewDoc,
  ViewFilters,
  VisitedRow,
} from './types'

/*
 * ── The 0.21 awareness rules, borrowed whole (GDK-1495 / GDK-1497 A4) ──
 *
 * The session boundary, the work-item age and its threshold, the resume
 * diff and the five built-in views are *decisions*, and the desktop already
 * owns every one of them. The phone imports the functions rather than
 * re-spelling them: two surfaces that disagree about when an issue is stale,
 * or about how long a session lasts, are worse than either answer alone.
 *
 * These modules are pure — no fetch, no DOM, no store reads. `view-config`
 * reaches `lib/config` for the *set* threshold, which on the phone is the
 * shared DEFAULTS object (72h) because no config.json is ever loaded here;
 * that is the desktop's own precedence step 3, so the phone lands on the
 * same number for the same reason. The phone's boundary gate
 * (web-boundary.test.ts) bans importing that config store directly, and this
 * file does not — it registers its flow source through the seam view-config
 * exposes for exactly this (`setStaleFlowSource`).
 *
 * The one adaptation is the row shape: the phone's `IssueLite` is a subset of
 * the desktop's (types.ts — "the phone only parses what it paints"), with the
 * same field names. Every borrowed predicate reads only fields the phone
 * carries, so the seam is a cast at the call, made once, here.
 */
import type { IssueLite as WebIssueLite } from '../../../web/src/lib/types'
import {
  isStale,
  setStaleFlowSource,
  staleThresholdHoursEffective,
  staleThresholdLearned,
  staleThresholdSamples,
  workAge,
  type AgeBasis,
} from '../../../web/src/lib/view-config'
import {
  changedSince,
  relatchBoundary,
  stripLabel,
  SESSION_GAP_MS,
  type SessionDelta,
} from '../../../web/src/lib/session-strip'
import {
  pickSince,
  resumeDelta,
  resumeLabel,
  type ResumeDelta,
} from '../../../web/src/lib/resume-card'
import { isSamePerson, type PersonRef } from '../../../web/src/lib/person-match'
import { builtinViews } from '../../../web/src/lib/builtin-views'
import { highlightSegments } from '../../../web/src/lib/format'
import { formatAbs, localZone } from '../../../web/src/lib/calendar'

export { relatchBoundary, SESSION_GAP_MS }
export type { AgeBasis, ResumeDelta, SessionDelta }

/** The one seam: a phone row read as the desktop row it is a subset of. */
function asWebRow(issue: IssueLite): WebIssueLite {
  return issue as unknown as WebIssueLite
}

/** The paired identity as person-match's reference. Null stays null. */
function personRef(me: Me | null): PersonRef | null {
  if (!me) return null
  return { accountId: me.account_id, email: me.email }
}

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

/*
 * "Is this issue mine?" has one owner, and it is not this file: `isSamePerson`
 * in web/src/lib/person-match.ts, reached through `matchesFilters`' `mine`
 * flag. A second local predicate lived here until GDK-1542 and answered
 * slightly differently (case-sensitive email, no id→email fallthrough), which
 * is how the phone came to offer the same question twice.
 */

/** True when the serve knows who its user is at all. */
export function hasIdentity(me: Me | null): boolean {
  return !!me && (!!me.account_id || !!me.email)
}

/* ── ② row age: the work clock, and the line it is measured against ── */

/**
 * Registers where the learned threshold comes from. Called once by the store
 * with a reader over the live bootstrap payload, so the age bands stay
 * reactive without this module knowing the store exists. `null` returns the
 * phone to the shared default.
 */
export function setFlow(flow: FlowSummary | null): void {
  setStaleFlowSource(() => flow)
}

/** Work-item age and which clock it read: started_at, else the status
 *  clock, else nothing. The desktop's `workAge`, verbatim. */
export function rowAge(issue: IssueLite): { hours: number; basis: AgeBasis } {
  return workAge(asWebRow(issue))
}

/** Whether the row has been underway longer than the threshold in force.
 *  Done is never stale, however old. */
export function rowIsStale(issue: IssueLite): boolean {
  return isStale(asWebRow(issue))
}

/** The badge's number. Day-based, floored at 1 so sub-day reads "day 1". */
export function rowAgeDays(issue: IssueLite): number {
  return Math.max(1, Math.round(rowAge(issue).hours / 24))
}

/**
 * Weight follows magnitude, in multiples of the effective threshold — the
 * desktop's ratios (GDK-1336). Null when the row is not stale: a single
 * maximum-emphasis mark on every row warns about nothing.
 */
export function rowAgeBand(issue: IssueLite): 'quiet' | 'mid' | 'loud' | null {
  if (!rowIsStale(issue)) return null
  const threshold = staleThresholdHoursEffective()
  if (!(threshold > 0)) return 'loud'
  const ratio = rowAge(issue).hours / threshold
  if (ratio <= 2) return 'quiet'
  if (ratio <= 4) return 'mid'
  return 'loud'
}

/**
 * The row names its own rule (G7): which clock the number came from, and —
 * when the threshold was learned rather than defaulted — what the 85% line
 * is and how many finished issues it stands on.
 */
export function rowAgeTitle(issue: IssueLite): string {
  const n = rowAgeDays(issue)
  const started = rowAge(issue).basis === 'started'
  if (!staleThresholdLearned()) {
    return t(started ? 'list.staleDaysStarted' : 'list.staleDays', { n })
  }
  return t(started ? 'list.staleDaysStartedLearned' : 'list.staleDaysLearned', {
    n,
    p: Math.max(1, Math.round(staleThresholdHoursEffective() / 24)),
    s: staleThresholdSamples(),
  })
}

/* ── ① session strip: what changed since the previous session ── */

/**
 * The frozen answer for one session: which rows moved after the boundary and
 * how many of them are this account's. Null when nothing moved — the caller
 * renders no strip, never an empty one.
 */
export function sessionDelta(
  issues: IssueLite[],
  since: string,
  me: Me | null,
): SessionDelta | null {
  return changedSince(issues.map(asWebRow), since, personRef(me))
}

/** The one line, from the desktop catalog. `ago` is already formatted. */
export function sessionLine(delta: SessionDelta, ago: string, me: Me | null): string {
  return stripLabel(delta, ago, t, delta.mine > 0 ? personRef(me) : null)
}

/* ── ③ resume card: what changed since this issue was last opened ── */

/**
 * The diff boundary: the previous person read of this issue, or null when
 * there is none. The serve sends the two newest `ui` visits; the newest may
 * be this very open on a desk that is looking at the same issue, which is
 * what the freshness window in `pickSince` is for.
 */
export function resumeSince(detail: DetailResponse): string | null {
  return pickSince(detail.last_visited_at, detail.previous_visit_at)
}

/** Counts what happened after `since`. Null when nothing did. */
export function resumeChanges(
  detail: DetailResponse,
  since: string | null,
): ResumeDelta | null {
  return resumeDelta(
    { history: detail.history ?? [], comments: (detail.comments ?? []) as never },
    since,
  )
}

/** The card's one line, from the desktop catalog. `ago` is already formatted. */
export function resumeLine(delta: ResumeDelta, ago: string): string {
  return resumeLabel(delta, ago, t)
}

/** Rank 0 means the mirror never saw a priority — sort those last, not first. */
function rankKey(i: IssueLite): number {
  // Null — never 0 — is "no rank" on the owner's contract (GDK-1132); both
  // sort last, exactly where 0 already did.
  const rank = i.priority_rank
  return rank !== null && rank > 0 ? rank : Number.MAX_SAFE_INTEGER
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

/**
 * Which picker section a scope belongs to; also the order they render in.
 *
 * "Assigned to me" used to hold a section of its own here, then a row of its
 * own inside `builtin` (vision FIX 2026-09-07). It is gone entirely now
 * (GDK-1542): it was the desk's `my-work` view asked a second time by a
 * hardcoded phone predicate, and the sheet showed both — same count, two
 * names. The phone's "mine" is `builtin:my-work`, whose filters come from
 * the shared catalog like every other built-in's.
 */
export type ScopeSection = 'builtin' | 'views' | 'filters' | 'docs'

/**
 * The desktop builtin `my-work` — the phone's "mine". Not a hardcoded
 * predicate: `web/src/lib/builtin-views.ts` owns what it selects
 * (`{mine: true, status_category: ['inprogress', 'new']}`), and
 * `matchesFilters` applies it through the same person-match the desk uses.
 */
export const SCOPE_MY_WORK = 'builtin:my-work'
/** The desktop builtin `all-open`, which the phone already ran as its "All". */
export const SCOPE_ALL_OPEN = 'builtin:all-open'
/**
 * Whole-mirror documents plate. Named `docs.tabUpdated` ("Updated"), not a
 * second "Documents" under the Documents heading: the desk's all-documents
 * surface is Viewed / Updated / Authors, and this plate is `updated_at` desc.
 */
export const SCOPE_DOCS_UPDATED = 'docs:updated'

export function docsSpaceScopeId(spaceKey: string): string {
  return `docs:space:${spaceKey}`
}

export interface Scope {
  id: string
  section: ScopeSection
  /** One discriminator: issue plates vs document plates. */
  kind: 'issues' | 'pages'
  /**
   * Display name. Catalog-owned for the two hardcoded scopes; for saved views
   * and imported filters it is the name the developer typed at the desk.
   */
  name: string
  /** Stored desktop filters. null for the two hardcoded scopes and for pages. */
  filters: Partial<ViewFilters> | null
  /**
   * Axes the phone cannot evaluate on an `IssueLite`. Non-empty means the row
   * is offered disabled: showing the full list under someone else's view name
   * would be a lie (docs/decisions/0007 — refuse unsupported out loud).
   */
  unsupported: string[]
  /**
   * Space this page-scope selects. Null on the whole-mirror Updated plate.
   * Keyed on `space_key`, never on the display name.
   */
  spaceKey?: string | null
  /**
   * Which reading stance a built-in belongs to (THEORY.md "Two stances"):
   * `mine` is the contributor's question, `team` the steward's. Only the
   * built-in section carries it, and only to wear the desk's own sub-labels
   * inside that section — grouping, never filtering.
   */
  stance?: 'mine' | 'team'
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
  // The three identity/exception flags the built-in views are made of
  // (GDK-1495 ④). `mine` and `delegated` need the paired identity, which
  // `matchesFilters` takes; without one they select nothing, exactly as the
  // desk's do. `reopened` needs only the row's own derived count.
  'mine',
  'delegated',
  'reopened',
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
 * columns existed carry '' and fall through to the name on both surfaces —
 * and on the owner's contract (GDK-1132) the ids are optional besides:
 * absent is the same "no id" as empty.
 */
function matchesIdFirst(
  selected: string[],
  id: string | null | undefined,
  name: string | null,
): boolean {
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
export function matchesFilters(
  issue: IssueLite,
  f: Partial<ViewFilters>,
  me: Me | null = null,
): boolean {
  // The identity flags first: they are the cheapest refusal, and without an
  // identity they refuse everything rather than quietly widening the view.
  if (f.mine || f.delegated) {
    const ref = personRef(me)
    if (!ref) return false
    const assigned = isSamePerson(issue.assignee_id, issue.assignee_email, ref)
    if (f.mine && !assigned) return false
    if (f.delegated) {
      const reported = isSamePerson(issue.reporter_id ?? null, issue.reporter_email ?? null, ref)
      if (!reported || assigned) return false
    }
  }
  if (f.reopened && issue.reopen_count <= 0) return false

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
export function applyFilters(
  issues: IssueLite[],
  f: Partial<ViewFilters>,
  me: Me | null = null,
): IssueLite[] {
  return issues.filter((i) => matchesFilters(i, f, me))
}

/**
 * The picker's scope list, in section order. Names come from the desktop —
 * the built-in catalog for the five, the i18n catalog for the documents
 * plate, the developer's own text for saved views and imported filters. The
 * phone invents nothing here.
 */
export function buildScopes(
  views: SavedViewDoc[],
  sources: SourceViewDoc[],
  me: Me | null,
  pages: PageLite[] = [],
): Scope[] {
  const out: Scope[] = []
  /*
   * The desk's five built-ins (GDK-1495 ④), in the desk's order and under
   * the desk's names — two in the contributor stance, three in the steward's
   * (web/src/lib/builtin-views.ts owns which five and what each one filters).
   * Five, not six: the phone's own "Assigned to me" row was `my-work` asked
   * twice and left with GDK-1542.
   * The phone consumes the `filters` half only: grouping and sort are the
   * phone's own (priority sections, DESIGN.md §5), and there is no URL here
   * for the desk's `fl=` parameters to travel in — the same config is
   * applied in memory instead.
   *
   * The two identity views are absent, not disabled, without an identity:
   * an anonymous reader has no "mine" (the desk hides them for the same
   * reason). All-open keeps its existing id so a phone that has one stored
   * lands on the same row after the update.
   */
  for (const view of builtinViews()) {
    if (view.needsIdentity && !hasIdentity(me)) continue
    const filters = view.config.filters
    out.push({
      id: view.id === 'all-open' ? SCOPE_ALL_OPEN : `builtin:${view.id}`,
      section: 'builtin',
      kind: 'issues',
      name: view.name,
      filters,
      unsupported: unsupportedAxes(filters),
      stance: view.stance,
    })
  }
  for (const v of views) {
    const filters = v.config?.filters ?? null
    out.push({
      id: `view:${v.id}`,
      section: 'views',
      kind: 'issues',
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
    out.push({
      id: `source:${s.id}`,
      section: 'filters',
      kind: 'issues',
      name: s.name,
      filters,
      unsupported,
    })
  }
  if (pages.length > 0) {
    out.push({
      id: SCOPE_DOCS_UPDATED,
      section: 'docs',
      kind: 'pages',
      name: t('docs.tabUpdated'),
      filters: null,
      unsupported: [],
      spaceKey: null,
    })
    const names = new Map<string, string>()
    for (const p of pages) {
      if (!p.space_key || names.has(p.space_key)) continue
      names.set(p.space_key, spaceLabel(p))
    }
    const keys = [...names.keys()].sort((a, b) => {
      const byName = collator().compare(names.get(a)!, names.get(b)!)
      return byName !== 0 ? byName : a.localeCompare(b)
    })
    for (const key of keys) {
      out.push({
        id: docsSpaceScopeId(key),
        section: 'docs',
        kind: 'pages',
        name: names.get(key)!,
        filters: null,
        unsupported: [],
        spaceKey: key,
      })
    }
  }
  return out
}

/**
 * The scope a phone with nothing stored opens on — the desk's first-run rule
 * (web/src/lib/startup-view.ts:86: my-work when identified *and* holding open
 * assigned work, else the open pool). The phone takes the identity half here;
 * the count half it cannot take, because the want is fixed before any
 * snapshot exists. It says the same thing later instead: an empty my-work
 * paints All open with `fellBack` and the screen says why (buildList below),
 * so a first run with no assigned work still lands on the pool — one frame
 * later than the desk, and out loud.
 */
export function defaultScopeId(me: Me | null): string {
  return hasIdentity(me) ? SCOPE_MY_WORK : SCOPE_ALL_OPEN
}

/**
 * Rewrites a scope id persisted by an older build (GDK-1542). The one owner
 * of that mapping: the phone's hardcoded `'me'` was the desk's `my-work`
 * asked twice, so a phone that had it stored lands on the row that answers
 * the same question rather than falling back to the pool.
 */
export function migrateScopeId(stored: string): string {
  return stored === 'me' ? SCOPE_MY_WORK : stored
}

/**
 * The scope the app should paint: the wanted one, else the default for this
 * identity, else the first offered. The default is named rather than taken
 * positionally — the first supported row happens to be it today, and that is
 * an accident of catalog order, not a decision.
 */
export function resolveScope(scopes: Scope[], wantId: string | null, me: Me | null = null): Scope | null {
  const ok = (s: Scope) => s.unsupported.length === 0
  const hit = scopes.find((s) => s.id === wantId && ok(s))
  if (hit) return hit
  // A deleted (or newly unsupported) saved view falls back silently.
  const fallbackId = defaultScopeId(me)
  return scopes.find((s) => s.id === fallbackId && ok(s)) ?? scopes.find(ok) ?? null
}

/** Rows a scope selects, before sorting. Null for a scope the phone refuses. */
export function scopeIssues(issues: IssueLite[], me: Me | null, scope: Scope): IssueLite[] | null {
  if (scope.kind === 'pages') return null
  if (scope.unsupported.length > 0) return null
  if (scope.id === SCOPE_ALL_OPEN) return openIssues(issues)
  return scope.filters ? applyFilters(issues, scope.filters, me) : null
}

/** Match count for a picker row (GDK-886). Null = the row is disabled. */
export function scopeCount(
  issues: IssueLite[],
  me: Me | null,
  scope: Scope,
  pages: PageLite[] = [],
): number | null {
  if (scope.kind === 'pages') return scopePages(pages, scope).length
  const rows = scopeIssues(issues, me, scope)
  return rows === null ? null : rows.length
}

/** Display label for a page's space: `space_name`, else `space_key`. */
export function spaceLabel(page: PageLite): string {
  const name = (page.space_name ?? '').trim()
  return name || page.space_key
}

/** `updated_at` desc, then key for stability. */
export function sortPages(pages: PageLite[]): PageLite[] {
  return [...pages].sort((a, b) => {
    const u = (b.updated_at ?? '').localeCompare(a.updated_at ?? '')
    if (u !== 0) return u
    return a.key.localeCompare(b.key)
  })
}

/** Pages a documents scope selects, already sorted. */
export function scopePages(pages: PageLite[], scope: Scope): PageLite[] {
  if (scope.kind !== 'pages') return []
  const rows = scope.spaceKey ? pages.filter((p) => p.space_key === scope.spaceKey) : pages
  return sortPages(rows)
}

/**
 * Split `body_text` on blank lines. Single newlines stay inside a paragraph
 * (the renderer uses `white-space: pre-wrap`). Empty input → no paragraphs.
 */
export function bodyParagraphs(text: string): string[] {
  const trimmed = text.replace(/\r\n/g, '\n').trim()
  if (trimmed === '') return []
  return trimmed.split(/\n{2,}/)
}

export interface IssueListView {
  sections: PrioritySection[]
  total: number
  /** The scope actually painted — an empty My issues falls back. */
  scopeId: string
  /** True when the fallback fired, so the screen can say why. */
  fellBack: boolean
}

/** The list in one call: select by scope (with honest fallback), sort, group. */
export function buildList(issues: IssueLite[], me: Me | null, scope: Scope): IssueListView {
  if (scope.kind === 'pages') {
    return { sections: [], total: 0, scopeId: scope.id, fellBack: false }
  }
  const rows = scopeIssues(issues, me, scope)
  if (rows === null || (scope.id === SCOPE_MY_WORK && rows.length === 0)) {
    // No identity, an empty plate, or a scope the phone refuses: All open,
    // said out loud rather than an empty screen under someone else's name.
    // Only My issues gets the empty-plate fallback: it is the phone's default
    // scope, so an empty one is a first-run condition, not a chosen filter —
    // an empty saved view is what the developer asked for and stays empty.
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

/*
 * A doc row's snippet line (GDK-890) — the phone rendering of the web's
 * matchEvidence rule (web/src/lib/search-match.ts). The serve's FTS snippet
 * for a title hit IS the title, so drawing it under the title spends the
 * row's second line saying the same thing twice — "Pairing runbook · In
 * GDK · Pairing runbook" was a real row shape. When the query is already
 * visible in the title the row carries its own reason and the snippet
 * drops; the meta clauses (author · time · space) come back in its place.
 *
 * The "does the title show the query" verdict is the web's own word-match,
 * imported from the web module rather than re-spelled, so the two surfaces
 * cannot drift on what that means — including its conservatism: the
 * server matches each token separately while this checks whole words, so
 * a multi-word query can hit on the server yet leave the title silent;
 * that row keeps its snippet, exactly as on the web. The one difference
 * from matchEvidence is deliberate: the web DROPS a row whose evidence is
 * null, the phone keeps it and shows meta — a row the user can still tap
 * is not noise to hide, and Search never fed this a row the server did
 * not return.
 */
export function docSnippet(
  match: SearchMatch | null | undefined,
  title: string,
  q: string,
): string {
  const query = q.trim()
  if (query !== '' && highlightSegments(title, query).some((seg) => seg.hit)) return ''
  return match?.snippet ?? ''
}

/*
 * Month-day labels follow the active locale: `Aug 12` / `8월 12일` /
 * `8月12日` (GDK-1704 — this was a hardcoded English MONTHS array, so ko/ja
 * phones read English months in every row folio). Formatters are cached per
 * locale because folioDate runs once per list row and constructing
 * Intl.DateTimeFormat is the expensive half of that call.
 */
const monthDayFormatters = new Map<string, Intl.DateTimeFormat>()

function monthDay(localeTag: string): Intl.DateTimeFormat {
  let fmt = monthDayFormatters.get(localeTag)
  if (!fmt) {
    fmt = new Intl.DateTimeFormat(localeTag, { month: 'short', day: 'numeric' })
    monthDayFormatters.set(localeTag, fmt)
  }
  return fmt
}

function calendarLabel(d: Date, localeTag: string): string {
  return monthDay(localeTag).format(d)
}

/**
 * Right-hand folio on a ledger row: calendar only (`Aug 12`).
 * `relTime` stays the recency chip (sync age, comments); a list of mixed
 * ages must not switch grammar at the 7-day step.
 */
export function folioDate(
  iso: string | null | undefined,
  localeTag: string = locale(),
): string {
  if (!iso) return ''
  const ts = new Date(iso)
  if (isNaN(ts.getTime())) return ''
  return calendarLabel(ts, localeTag)
}

/** Compact relative time: just now / 5m / 3h / 2d / Aug 12. Bad input → ''. */
export function relTime(
  iso: string | null | undefined,
  now: Date = new Date(),
  localeTag: string = locale(),
): string {
  if (!iso) return ''
  const ts = new Date(iso)
  if (isNaN(ts.getTime())) return ''
  const sec = Math.floor((now.getTime() - ts.getTime()) / 1000)
  // Same catalog key the web's compact relative time reads (GDK-1704 —
  // this branch used to hardcode the English word 'now').
  if (sec < 60) return t('time.justNow')
  if (sec < 3600) return `${Math.floor(sec / 60)}m`
  if (sec < 86400) return `${Math.floor(sec / 3600)}h`
  if (sec < 7 * 86400) return `${Math.floor(sec / 86400)}d`
  return calendarLabel(ts, localeTag)
}

/**
 * Offer-expiry line on the pairing tab: `Sep 9, 2026` in `localeTag`
 * (GDK-1704 — PairingTab used to hardcode 'en-US', so ko/ja phones read
 * an English date). '' for missing or malformed input; the caller renders
 * nothing then.
 */
export function offerExpiry(iso: string, localeTag: string = locale()): string {
  if (!iso) return ''
  const ts = new Date(iso)
  if (isNaN(ts.getTime())) return ''
  return ts.toLocaleDateString(localeTag, { month: 'short', day: 'numeric', year: 'numeric' })
}

/**
 * Due-date label (GDK-875). duedate is a *date* kind — YYYY-MM-DD stored as
 * written — and this must never `new Date(ymd)` it: that reads UTC midnight,
 * which a western zone renders as the previous calendar day. The desk's
 * calendar module owns that rule and the absolute form (year + 2-digit
 * month/day, per locale); the phone borrows the formatter rather than the
 * format, so the two surfaces cannot say different days.
 */
export function dueDateLabel(ymd: string | null | undefined, localeTag: string = locale()): string {
  return formatAbs(ymd, 'date', localZone(), localeTag)
}

/**
 * The visit ledger after one more read (GDK-875): the key moves to the front
 * at `at` — its newest visit — and everything else keeps its order and its
 * stamp. Pure on purpose: the store assigns the return; it never pushes.
 */
export function foldVisit(visits: VisitedRow[], key: string, at: string): VisitedRow[] {
  return [{ key, viewed_at: at }, ...visits.filter((v) => v.key !== key)]
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
    raw_body: null,
    body: text,
  }
}

/* ── Glance strip (GDK-871): the feed as the Issues queue's first band ── */

/**
 * One activity row from GET issues/feed/ (internal/store/feed.go FeedItem).
 * The phone paints the identifier, the event, the actor and the time — the
 * fields below are the subset it reads. The wire shapes normally live in
 * ./types, which sits outside this round's file list, so they are declared
 * here beside the selection logic that consumes them.
 */
export interface FeedItem {
  event_id: string
  issue_key: string
  event_type: string
  occurred_at: string | null
  actor_name: string
  /** Why this row is relevant to the paired identity: assignee / reporter / mention / watched. */
  reasons: string[]
  read_at: string | null
}

export interface FeedUnreadCounts {
  all: number
  assignee: number
  reporter: number
  mention: number
}

export interface FeedResponse {
  items: FeedItem[]
  unread_counts: FeedUnreadCounts
}

/** POST issues/feed/read/ reply (internal/store/feed.go MarkFeedReadResult). */
export interface MarkFeedReadResponse {
  updated: number
  unread_counts: FeedUnreadCounts
}

/** The desktop feed's own event-type words — reused, never re-authored (§3.6). */
const FEED_KIND_KEYS: Record<string, MessageKey> = {
  created: 'feed.kindCreated',
  status_changed: 'feed.kindStatus',
  reopened: 'feed.kindReopen',
  assigned: 'feed.kindAssignee',
  comment_added: 'feed.kindComment',
  attachment_added: 'feed.kindAttachment',
  fields_changed: 'feed.kindField',
}

/** Human phrase for an event type; an unknown type shows itself, not a blank. */
export function feedKindLabel(eventType: string): string {
  const key = FEED_KIND_KEYS[eventType]
  return key ? t(key) : eventType
}

/** Rows the strip shows: unread only, newest first, capped (null stamps last). */
export function glanceRows(items: FeedItem[], limit: number = 3): FeedItem[] {
  return [...items]
    .filter((i) => !i.read_at)
    .sort((a, b) => {
      const ka = a.occurred_at ?? ''
      const kb = b.occurred_at ?? ''
      if (ka === '' || kb === '') {
        if (ka !== kb) return ka === '' ? 1 : -1
      } else {
        const d = kb.localeCompare(ka)
        if (d !== 0) return d
      }
      return a.event_id.localeCompare(b.event_id)
    })
    .slice(0, limit)
}

/**
 * Unread delta a set of rows contributes — store.countUnread restricted to
 * these rows, so a local decrement cannot drift from the server's arithmetic.
 */
function unreadDelta(items: FeedItem[]): FeedUnreadCounts {
  const d = { all: 0, assignee: 0, reporter: 0, mention: 0 }
  for (const i of items) {
    if (i.read_at) continue
    d.all++
    if (i.reasons.includes('assignee')) d.assignee++
    if (i.reasons.includes('reporter')) d.reporter++
    if (i.reasons.includes('mention')) d.mention++
  }
  return d
}

/**
 * Local feed state after a read receipt lands: the marked rows drop, the
 * counts come from the server's reply when it carries them (it always does
 * on a 200) and otherwise fall back to the mirrored decrement. `null` keys
 * mean "all": every row goes and the counts zero, because the local window
 * (limit 20) can be shorter than what "all" cleared.
 */
export function feedAfterRead(
  feed: FeedResponse,
  issueKeys: string[] | null,
  counts?: FeedUnreadCounts,
): FeedResponse {
  if (issueKeys === null) {
    return { items: [], unread_counts: counts ?? { all: 0, assignee: 0, reporter: 0, mention: 0 } }
  }
  const keys = new Set(issueKeys)
  const d = unreadDelta(feed.items.filter((i) => keys.has(i.issue_key)))
  const prev = feed.unread_counts
  return {
    items: feed.items.filter((i) => !keys.has(i.issue_key)),
    unread_counts:
      counts ?? {
        all: Math.max(0, prev.all - d.all),
        assignee: Math.max(0, prev.assignee - d.assignee),
        reporter: Math.max(0, prev.reporter - d.reporter),
        mention: Math.max(0, prev.mention - d.mention),
      },
  }
}
