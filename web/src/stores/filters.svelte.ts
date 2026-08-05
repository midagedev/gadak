/*
 * Issue Navigator — filter/display/derived-list store ([explore], contract §2)
 *
 * Design: **the URL hash is the single source of truth for the view**.
 *  - filters/display derive from router.params (parseConfig) → "view = URL" holds naturally.
 *  - Every change (chip add/remove · grouping · sort · query) updates the URL via
 *    setParams(replace) → derived values recompute.
 *  - Selecting an issue (?issue) must not refilter: an intermediate stable string
 *    (#viewKey) of view-only params blocks refilter (perf discipline).
 *
 * All derived values (visibleIssues/groups/facets) are local — no server round-trip.
 */

import { config } from '../lib/config'
import { router, setParams } from '../lib/router.svelte'
import { issues } from './issues.svelte'
import { me } from './me.svelte'
import { extractChosung, isChosungQuery } from '../lib/korean'
import type { IssueLite } from '../lib/types'
import {
  configToParams,
  defaultColumns,
  deployStateOf,
  effectiveCategory,
  emptyConfig,
  hasAnyFilter,
  isStale,
  MULTI_FIELDS,
  orderColumns,
  parseConfig,
  VIEW_PARAM_KEYS,
  type ColumnKey,
  type GroupBy,
  type MultiField,
  type SortKey,
  type StatusCategory,
  type ViewConfig,
  type ViewFilters,
} from '../lib/view-config'
import * as api from '../lib/api'
import {
  categoryLabel,
  deployStateLabel,
  fieldLabel,
  t,
} from '../lib/i18n'
import { write } from './write.svelte'

/* ── Derived group types ── */

export interface GroupCounts {
  total: number
  category: Record<StatusCategory, number>
  severity: Record<string, number>
}

export interface IssueGroup {
  key: string
  label: string
  items: IssueLite[]
  counts: GroupCounts
}

/** One active filter chip rendered by FilterBar. */
export interface ActiveChip {
  /** Which field/flag/range this chip represents. */
  kind: 'multi' | 'flag' | 'range'
  field: string
  value?: string // multi value
  label: string // display string
}

/* ── facet (value distribution for add dropdowns) ── */

export interface FacetValue {
  value: string
  label: string
  count: number
}

class FiltersStore {
  /* URL → stable string of view-only params. Unchanged by issue selection → blocks refilter. */
  #viewKey = $derived(VIEW_PARAM_KEYS.map((k) => `${k}=${router.params.get(k) ?? ''}`).join('&'))

  /* Config re-parsed only when #viewKey actually changes. */
  #config = $derived(parseFromKey(this.#viewKey))

  get filters(): ViewFilters {
    return this.#config.filters
  }
  get display(): ViewDisplayRef {
    return this.#config.display
  }

  /**
   * Sort actually applied. With a query and default sort (updated), auto-promote
   *  to relevance. (An explicit other sort wins — but updated is indistinguishable
   *  from "unset", so during search we treat it as relevance. Sort UI shows this too.)
   */
  get effectiveSort(): SortKey {
    const { filters: f, display: d } = this.#config
    if (f.q.trim() && d.sort === 'updated') return 'relevance'
    return d.sort
  }

  /** Stable string of view (filter+display) params only. Unchanged by data updates →
   *  list uses this as the signal to reset scroll/cursor only when the view really changed. */
  get viewKey(): string {
    return this.#viewKey
  }

  /* ── Server full-text search results (volatile; not part of view serialization) ── */
  serverMatchKeys = $state<string[]>([])
  serverMatchQuery = $state('')
  searching = $state(false)
  /** Last query that failed body search (UI can offer Retry). */
  searchError = $state<string | null>(null)

  /* ── Filter result (flat, group-agnostic, sorted) ── */
  visibleIssues = $derived.by(() => {
    const f = this.#config.filters
    const list = filterIssues(issues.allIssues, f)
    const sort = this.effectiveSort
    // Relevance needs query · recency · personalization together — pass context.
    const ctx: RelevanceContext | undefined =
      sort === 'relevance' ? buildRelevanceContext(f.q) : undefined
    return sortIssues(list, sort, this.#config.display.dir, ctx)
  })

  /* ── Grouping (display.group_by). group_by=none → single group. ── */
  groups = $derived.by(() => buildGroups(this.visibleIssues, this.#config.display.group_by))

  /* ── FilterBar active chips ── */
  activeChips = $derived.by(() =>
    buildChips(this.#config.filters, issues.members, issues.allIssues),
  )

  hasFilters = $derived(hasAnyFilter(this.#config.filters))

  /**
   * Whether the current view is already scoped to "my issues" (my email in
   *  assignee or reporter filters). Whole list is then mine, so per-row highlight
   *  is noise — turn it off.
   */
  scopedToMe = $derived.by(() => {
    const e = me.email?.toLowerCase()
    if (!e) return false
    const f = this.#config.filters
    const has = (arr: string[]) => arr.some((x) => x.toLowerCase() === e)
    return has(f.assignee_email) || has(f.reporter_email)
  })

  /** Whether this issue is assigned to me (highlight check). Case-insensitive. */
  isMine(issue: IssueLite): boolean {
    const e = me.email
    return (
      !!e &&
      !!issue.assignee_email &&
      issue.assignee_email.toLowerCase() === e.toLowerCase()
    )
  }

  /** facet: value distribution over the full pool (add dropdowns). Includes member name labels. */
  facets = $derived.by(() => buildFacets(issues.allIssues, issues.members))

  /* ── Mutations: all via URL update (replace, no history push) ── */

  #apply(config: ViewConfig): void {
    setParams(configToParams(config), true)
  }

  /** Independent copy of current config (arrays cloned). Starting point for mutations. */
  private snapshot(): ViewConfig {
    return mergeConfig(this.#config)
  }

  /** Toggle a multi value (remove if present, else add). */
  toggleValue(field: MultiField, value: string): void {
    const c = this.snapshot()
    const arr = c.filters[field]
    c.filters[field] = arr.includes(value) ? arr.filter((v) => v !== value) : [...arr, value]
    this.#apply(c)
  }

  /** Add a multi value (chip click = add filter; ignore duplicates). */
  addValue(field: MultiField, value: string): void {
    if (!value) return
    if (this.#config.filters[field].includes(value)) return
    const c = this.snapshot()
    c.filters[field] = [...c.filters[field], value]
    this.#apply(c)
  }

  removeValue(field: MultiField, value: string): void {
    const c = this.snapshot()
    c.filters[field] = c.filters[field].filter((v) => v !== value)
    this.#apply(c)
  }

  toggleFlag(flag: 'reopened' | 'unassigned' | 'stale'): void {
    const c = this.snapshot()
    c.filters[flag] = !c.filters[flag]
    this.#apply(c)
  }

  setRange(field: 'created' | 'updated', from: string | null, to: string | null): void {
    const c = this.snapshot()
    c.filters[`${field}_from`] = from
    c.filters[`${field}_to`] = to
    this.#apply(c)
  }

  setQuery(q: string): void {
    if (q === this.#config.filters.q) return
    const c = this.snapshot()
    c.filters.q = q
    this.#apply(c)
    // Local query change invalidates prior server-search results
    if (!q.trim()) this.clearServerSearch()
  }

  setGroupBy(g: GroupBy): void {
    const c = this.snapshot()
    c.display.group_by = g
    this.#apply(c)
  }

  setSort(s: SortKey): void {
    const c = this.snapshot()
    c.display.sort = s
    this.#apply(c)
  }

  toggleDir(): void {
    const c = this.snapshot()
    c.display.dir = c.display.dir === 'desc' ? 'asc' : 'desc'
    this.#apply(c)
  }

  /** Toggle a list column on/off (keeps catalog order; allowing all-off is intentional). */
  toggleColumn(key: ColumnKey): void {
    const c = this.snapshot()
    const cur = c.display.columns
    const next = cur.includes(key) ? cur.filter((k) => k !== key) : [...cur, key]
    c.display.columns = orderColumns(next)
    this.#apply(c)
  }

  /** Reset column set to defaults. */
  resetColumns(): void {
    const c = this.snapshot()
    c.display.columns = defaultColumns()
    this.#apply(c)
  }

  /** Apply a saved/default view wholesale. */
  applyConfig(config: ViewConfig): void {
    this.clearServerSearch()
    this.#apply(mergeConfig(config))
  }

  /** Current view config (for save). */
  currentConfig(): ViewConfig {
    return mergeConfig(this.#config)
  }

  /** Reset all filters/display (preserve selected issue). */
  clearAll(): void {
    this.clearServerSearch()
    this.#apply(emptyConfig())
  }

  /* ── Server full-text search ── */

  async runServerSearch(): Promise<void> {
    const q = this.#config.filters.q.trim()
    if (!q) return
    this.searching = true
    this.serverMatchQuery = q
    try {
      const res = await api.search(q, 200)
      this.serverMatchKeys = res.keys
      this.searchError = null
    } catch (e) {
      console.warn('[filters] 서버 검색 실패', e)
      this.serverMatchKeys = []
      this.searchError = q
      write.toast(t('list.searchFailed'), 'error')
    } finally {
      this.searching = false
    }
  }

  clearServerSearch(): void {
    if (this.serverMatchKeys.length) this.serverMatchKeys = []
    this.serverMatchQuery = ''
    this.searchError = null
  }

  /** Issues hit by body match but missing from local results (extra section). */
  serverExtraIssues = $derived.by(() => {
    if (!this.serverMatchKeys.length) return [] as IssueLite[]
    const shown = new Set(this.visibleIssues.map((i) => i.issue_key))
    const out: IssueLite[] = []
    for (const key of this.serverMatchKeys) {
      if (shown.has(key)) continue
      const it = issues.pool.get(key)
      if (it) out.push(it)
    }
    return out
  })
}

/* display type alias (readability). */
type ViewDisplayRef = ViewConfig['display']

/* ── Pure helpers ── */

function parseFromKey(viewKey: string): ViewConfig {
  // viewKey is "k=v&k=v" of view params only → rehydrate via URLSearchParams and parse.
  return parseConfig(new URLSearchParams(viewKey))
}

/** Normalize config to full shape (partial-config defense) + clone array refs. */
function mergeConfig(c: ViewConfig): ViewConfig {
  const base = emptyConfig()
  Object.assign(base.filters, c.filters)
  Object.assign(base.display, c.display)
  // Clone array refs. Columns may include ones for disabled features in a saved view — normalize.
  for (const field of MULTI_FIELDS) base.filters[field] = [...(c.filters[field] ?? [])]
  base.display.columns = orderColumns(c.display.columns ?? base.display.columns)
  return base
}

const IN_RANK: Record<StatusCategory, number> = { inprogress: 0, new: 1, done: 2 }

function normKey(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9가-힣]/g, '')
}

function jiraProjectOf(issue: IssueLite): string {
  const separator = issue.issue_key.indexOf('-')
  return separator > 0 ? issue.issue_key.slice(0, separator) : ''
}

function splitStoredValues(value: string | null | undefined): string[] {
  return (value ?? '')
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
}

function matchesSelected(selected: string[], values: string[]): boolean {
  return selected.length === 0 || values.some((value) => selected.includes(value))
}

/* ── Chosung (initial-consonant) cache ──
 *  title+assignee chosung string is computed once per issue; updated_at is the
 *  cache signature. Delta updates change the signature → auto-invalidate
 *  (avoids recomputing over ~10.5K issues).
 */
const chosungCache = new Map<string, { sig: string; text: string }>()

function issueChosung(issue: IssueLite): string {
  const sig = issue.updated_at ?? ''
  const cached = chosungCache.get(issue.issue_key)
  if (cached && cached.sig === sig) return cached.text
  const text = extractChosung(`${issue.summary} ${issue.assignee ?? ''}`)
  chosungCache.set(issue.issue_key, { sig, text })
  return text
}

/** Local text match (key/title/assignee/labels). chosungQuery also partial-matches chosung strings. */
function textMatch(issue: IssueLite, needle: string, chosungQuery: boolean): boolean {
  const hay = [
    issue.issue_key,
    issue.summary,
    issue.assignee ?? '',
    issue.assignee_email ?? '',
    issue.labels.join(' '),
  ]
    .join(' ')
    .toLowerCase()
  if (hay.includes(needle)) return true
  // Short issue-key form (den1234). Jamo queries (ㅋㅌㅂ) make normKey '' so
  // includes('') would match everything — skip empty results.
  const nk = normKey(needle)
  if (nk && normKey(issue.issue_key).includes(nk)) return true
  // Chosung query ("ㅋㅌㅂ") partial-matches title·assignee chosung strings
  if (chosungQuery && issueChosung(issue).includes(needle)) return true
  return false
}

function inRange(iso: string | null, from: string | null, to: string | null): boolean {
  if (!iso) return !from // no value: pass only when there is no lower bound
  const d = iso.slice(0, 10)
  if (from && d < from) return false
  if (to && d > to) return false
  return true
}

export function filterIssues(all: IssueLite[], f: ViewFilters): IssueLite[] {
  const raw = f.q.trim()
  const needle = raw.toLowerCase()
  // Chosung check once per query (outside the issue loop).
  const chosungQuery = raw ? isChosungQuery(raw) : false
  const out: IssueLite[] = []
  for (const it of all) {
    if (f.status_category.length && !f.status_category.includes(effectiveCategory(it))) continue
    if (f.status.length && !f.status.includes(it.status)) continue
    if (f.assignee_email.length && !(it.assignee_email && f.assignee_email.includes(it.assignee_email)))
      continue
    if (f.reporter_email.length && !(it.reporter_email && f.reporter_email.includes(it.reporter_email)))
      continue
    if (f.team_group.length && !(it.team_group && f.team_group.includes(it.team_group))) continue
    if (f.priority.length && !(it.priority && f.priority.includes(it.priority))) continue
    if (f.severity.length && !(it.severity && f.severity.includes(it.severity))) continue
    if (f.issue_type.length && !f.issue_type.includes(it.issue_type)) continue
    if (!matchesSelected(f.components, it.components)) continue
    if (!matchesSelected(f.fix_versions, it.fix_versions)) continue
    if (!matchesSelected(f.environment, splitStoredValues(it.environment))) continue
    if (!matchesSelected(f.browser, splitStoredValues(it.browser))) continue
    if (!matchesSelected(f.dev_project_number, splitStoredValues(it.dev_project_number))) continue
    if (!matchesSelected(f.found_version, splitStoredValues(it.found_version))) continue
    if (!matchesSelected(f.occurrence, splitStoredValues(it.occurrence))) continue
    if (!matchesSelected(f.solution, splitStoredValues(it.solution))) continue
    if (!matchesSelected(f.critical_phenomenon, splitStoredValues(it.critical_phenomenon))) continue
    if (!matchesSelected(f.development_area, splitStoredValues(it.development_area))) continue
    if (
      f.development_test_assignee_email.length &&
      !(
        it.development_test_assignee_email &&
        f.development_test_assignee_email.includes(it.development_test_assignee_email)
      )
    )
      continue
    if (
      f.development_test_result.length &&
      !(it.development_test_result && f.development_test_result.includes(it.development_test_result))
    )
      continue
    if (
      f.qa_run.length &&
      !(it.qa_runs ?? []).some((run) => f.qa_run.includes(run.key))
    )
      continue
    if (
      f.qa_suite.length &&
      !(it.qa_suites ?? []).some((suite) => f.qa_suite.includes(suite.key))
    )
      continue
    if (
      f.qa_impact.length &&
      !(it.qa_impact_state && f.qa_impact.includes(it.qa_impact_state))
    )
      continue
    if (f.deploy_state.length && !f.deploy_state.includes(deployStateOf(it))) continue
    if (!matchesSelected(f.cs, splitStoredValues(it.cs))) continue
    if (f.jira_project.length && !f.jira_project.includes(jiraProjectOf(it))) continue
    if (f.source_project.length && !(it.source_project && f.source_project.includes(it.source_project)))
      continue
    if (f.labels.length && !it.labels.some((l) => f.labels.includes(l))) continue

    if (f.reopened && !(it.reopen_count > 0)) continue
    if (f.unassigned && it.assignee_email) continue
    if (f.stale && !isStale(it)) continue

    if ((f.created_from || f.created_to) && !inRange(it.created_at, f.created_from, f.created_to))
      continue
    if ((f.updated_from || f.updated_to) && !inRange(it.updated_at, f.updated_from, f.updated_to))
      continue

    if (needle && !textMatch(it, needle, chosungQuery)) continue
    out.push(it)
  }
  return out
}

function cmpStr(a: string | null, b: string | null, dir: 1 | -1): number {
  const av = a ?? ''
  const bv = b ?? ''
  return av < bv ? -dir : av > bv ? dir : 0
}

/* ── Relevance ranking ──
 *  Score = match strength for needle + recency + personalization bonuses.
 *  needle is already trim·lowercased. chosungQuery also considers chosung strings.
 */
export interface RelevanceContext {
  needle: string
  chosungQuery: boolean
  now: number
  myEmail: string | null
  recentKeys: Set<string>
}

/** Context snapshot for relevance (personalization from me store). */
function buildRelevanceContext(rawQuery: string): RelevanceContext {
  const raw = rawQuery.trim()
  return {
    needle: raw.toLowerCase(),
    chosungQuery: raw ? isChosungQuery(raw) : false,
    now: Date.now(),
    myEmail: me.email,
    recentKeys: new Set(me.recent.map((v) => v.key)),
  }
}

const DAY_MS = 86_400_000

function relevanceScore(issue: IssueLite, ctx: RelevanceContext): number {
  const { needle } = ctx
  const keyLower = issue.issue_key.toLowerCase()
  const summaryLower = issue.summary.toLowerCase()

  // Match strength (strongest location only as base) — key > title > chosung > assignee·labels.
  let base = 0
  if (keyLower === needle || normKey(issue.issue_key) === normKey(needle)) base = 1000
  else if (keyLower.startsWith(needle) || normKey(issue.issue_key).startsWith(normKey(needle)))
    base = 500
  else if (summaryLower.startsWith(needle)) base = 300
  else if (summaryLower.includes(needle)) base = 100

  if (base === 0 && ctx.chosungQuery && issueChosung(issue).includes(needle)) base = 80

  if (base === 0) {
    const assignee = (issue.assignee ?? '').toLowerCase()
    const email = (issue.assignee_email ?? '').toLowerCase()
    if (
      assignee.includes(needle) ||
      email.includes(needle) ||
      issue.labels.some((l) => l.toLowerCase().includes(needle))
    )
      base = 60
  }

  // Recency bonus (by updated_at).
  let score = base
  if (issue.updated_at) {
    const age = ctx.now - Date.parse(issue.updated_at)
    if (age >= 0) {
      if (age <= 7 * DAY_MS) score += 30
      else if (age <= 30 * DAY_MS) score += 10
    }
  }

  // Personalization: assigned to me / recently viewed.
  if (ctx.myEmail && issue.assignee_email === ctx.myEmail) score += 40
  if (ctx.recentKeys.has(issue.issue_key)) score += 25

  return score
}

export function sortIssues(
  list: IssueLite[],
  sort: SortKey,
  dir: 'asc' | 'desc',
  ctx?: RelevanceContext,
): IssueLite[] {
  const d: 1 | -1 = dir === 'asc' ? 1 : -1
  const arr = [...list]
  if (sort === 'relevance') {
    // Relevance ignores dir (always higher score first). Cache scores; ties by newest update.
    const rc = ctx ?? buildRelevanceContext('')
    const scoreOf = new Map<string, number>()
    for (const it of arr) scoreOf.set(it.issue_key, relevanceScore(it, rc))
    arr.sort((a, b) => {
      const diff = (scoreOf.get(b.issue_key) ?? 0) - (scoreOf.get(a.issue_key) ?? 0)
      return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
    })
    return arr
  }
  arr.sort((a, b) => {
    switch (sort) {
      case 'created':
        return cmpStr(a.created_at, b.created_at, d)
      case 'reopen_count': {
        const diff = (a.reopen_count - b.reopen_count) * d
        return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
      }
      case 'priority': {
        // priority_rank: lower = higher priority. null always sorts last.
        const ar = a.priority_rank
        const br = b.priority_rank
        if (ar == null && br == null) return cmpStr(a.updated_at, b.updated_at, -1)
        if (ar == null) return 1
        if (br == null) return -1
        const diff = (ar - br) * d
        return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
      }
      case 'updated':
      default:
        return cmpStr(a.updated_at, b.updated_at, d)
    }
  })
  return arr
}

/* ── Grouping ── */

function groupKeyOf(issue: IssueLite, by: GroupBy): { key: string; label: string } {
  switch (by) {
    case 'status_category': {
      const category = effectiveCategory(issue)
      const label = categoryLabel(category, true)
      return { key: category, label }
    }
    case 'status':
      return { key: issue.status || '(none)', label: issue.status || t('group.noStatus') }
    case 'assignee':
      return issue.assignee_email
        ? { key: issue.assignee_email, label: issue.assignee || issue.assignee_email }
        : { key: '', label: t('common.unassigned') }
    case 'priority':
      return { key: issue.priority || '', label: issue.priority || t('group.noPriority') }
    case 'severity':
      return { key: issue.severity || '', label: issue.severity || t('group.noSeverity') }
    case 'team_group':
      return issue.team_group
        ? { key: issue.team_group, label: issue.team_group }
        : { key: '', label: t('common.unclassified') }
    case 'product':
      // Group→product mapping is org-specific; read from runtime config.
      return config().productByGroup[issue.team_group ?? ''] ?? { key: '', label: t('group.noProduct') }
    case 'issue_type':
      return { key: issue.issue_type || '', label: issue.issue_type || t('group.noType') }
    case 'development_test_result': {
      const result = issue.development_test_result?.trim()
      return result ? { key: result, label: result } : { key: 'none', label: t('group.none') }
    }
    case 'qa_impact':
      return issue.qa_impact_state
        ? { key: issue.qa_impact_state, label: issue.qa_impact_label }
        : { key: '', label: t('group.qaIrrelevant') }
    case 'source_project':
      return {
        key: issue.source_project || '',
        label: issue.source_project || t('group.noProject'),
      }
    case 'epic':
      return issue.epic_key ? { key: issue.epic_key, label: issue.epic_key } : { key: '', label: t('group.noEpic') }
    default:
      return { key: '', label: '' }
  }
}

function emptyCounts(): GroupCounts {
  return { total: 0, category: { new: 0, inprogress: 0, done: 0 }, severity: {} }
}

function accCounts(counts: GroupCounts, issue: IssueLite): void {
  counts.total++
  counts.category[effectiveCategory(issue)]++
  if (issue.severity) counts.severity[issue.severity] = (counts.severity[issue.severity] ?? 0) + 1
}

export function buildGroups(list: IssueLite[], by: GroupBy): IssueGroup[] {
  if (by === 'none') {
    const counts = emptyCounts()
    for (const it of list) accCounts(counts, it)
    return [{ key: '', label: '', items: list, counts }]
  }
  const map = new Map<string, IssueGroup>()
  for (const it of list) {
    const { key, label } = groupKeyOf(it, by)
    let g = map.get(key)
    if (!g) {
      g = { key, label, items: [], counts: emptyCounts() }
      map.set(key, g)
    }
    g.items.push(it)
    accCounts(g.counts, it)
  }
  const groups = [...map.values()]
  // Group sort: empty keys last; status/priority/severity by work order; else by name.
  groups.sort((a, b) => {
    const ae = a.key === ''
    const be = b.key === ''
    if (ae !== be) return ae ? 1 : -1
    if (by === 'priority') {
      const ar = rankOf(a.items[0])
      const br = rankOf(b.items[0])
      return ar - br
    }
    if (by === 'severity') {
      const severityRank: Record<string, number> = {
        Critical: 0,
        Major: 1,
        Minor: 2,
        Trivial: 3,
      }
      return (severityRank[a.key] ?? 99) - (severityRank[b.key] ?? 99)
    }
    if (by === 'status_category') {
      const ar = IN_RANK[a.key as StatusCategory]
      const br = IN_RANK[b.key as StatusCategory]
      return ar - br
    }
    if (by === 'product') {
      const productRank: Record<string, number> = {
        cloud: 0,
        crown: 1,
        batch: 2,
        backoffice: 3,
      }
      return (productRank[a.key] ?? 99) - (productRank[b.key] ?? 99)
    }
    if (by === 'development_test_result') {
      const resultRank: Record<string, number> = { fail: 0, none: 1, pass: 2 }
      return (resultRank[a.key.toLowerCase()] ?? 99) - (resultRank[b.key.toLowerCase()] ?? 99)
    }
    if (by === 'qa_impact') {
      const impactRank: Record<string, number> = {
        blocking: 0,
        retest: 1,
        linked: 2,
        verified: 3,
      }
      return (impactRank[a.key] ?? 99) - (impactRank[b.key] ?? 99)
    }
    if (by === 'status') {
      const ac = IN_RANK[effectiveCategory(a.items[0])]
      const bc = IN_RANK[effectiveCategory(b.items[0])]
      if (ac !== bc) return ac - bc
    }
    return a.label < b.label ? -1 : a.label > b.label ? 1 : 0
  })
  return groups
}

function rankOf(issue: IssueLite | undefined): number {
  return issue?.priority_rank ?? Number.MAX_SAFE_INTEGER
}

/* ── Active chips ── */

function FIELD_LABEL(field: string): string {
  return fieldLabel(field)
}

function CATEGORY_LABEL(value: string): string {
  if (value === 'new' || value === 'inprogress' || value === 'done') return categoryLabel(value)
  return value
}

function buildChips(
  f: ViewFilters,
  members: Map<string, { name: string }>,
  all: IssueLite[],
): ActiveChip[] {
  const chips: ActiveChip[] = []
  for (const field of MULTI_FIELDS) {
    for (const value of f[field]) {
      let label = value
      if (field === 'status_category') label = CATEGORY_LABEL(value)
      else if (
        field === 'assignee_email' ||
        field === 'reporter_email' ||
        field === 'development_test_assignee_email'
      )
        label = members.get(value)?.name ?? value
      else if (
        field === 'qa_run' ||
        field === 'qa_suite' ||
        field === 'qa_impact' ||
        field === 'deploy_state'
      )
        label = facetLabel(field, value, all, members)
      chips.push({ kind: 'multi', field, value, label: t('filter.chipFieldValue', { field: FIELD_LABEL(field), value: label }) })
    }
  }
  if (f.reopened) chips.push({ kind: 'flag', field: 'reopened', label: t('filter.flagReopened') })
  if (f.unassigned) chips.push({ kind: 'flag', field: 'unassigned', label: t('filter.flagUnassigned') })
  if (f.stale) chips.push({ kind: 'flag', field: 'stale', label: t('filter.flagStale') })
  if (f.created_from || f.created_to)
    chips.push({ kind: 'range', field: 'created', label: t('filter.chipCreatedRange', { from: f.created_from ?? '', to: f.created_to ?? '' }) })
  if (f.updated_from || f.updated_to)
    chips.push({ kind: 'range', field: 'updated', label: t('filter.chipUpdatedRange', { from: f.updated_from ?? '', to: f.updated_to ?? '' }) })
  return chips
}

/* ── facet ── */

function buildFacets(
  all: IssueLite[],
  members: Map<string, { name: string }>,
): Record<MultiField, FacetValue[]> {
  const counters: Record<string, Map<string, number>> = {}
  for (const field of MULTI_FIELDS) counters[field] = new Map()

  for (const it of all) {
    bump(counters.status_category, effectiveCategory(it))
    bump(counters.status, it.status)
    if (it.assignee_email) bump(counters.assignee_email, it.assignee_email)
    if (it.reporter_email) bump(counters.reporter_email, it.reporter_email)
    if (it.team_group) bump(counters.team_group, it.team_group)
    if (it.priority) bump(counters.priority, it.priority)
    if (it.severity) bump(counters.severity, it.severity)
    if (it.issue_type) bump(counters.issue_type, it.issue_type)
    for (const value of it.components) bump(counters.components, value)
    for (const value of it.fix_versions) bump(counters.fix_versions, value)
    for (const value of splitStoredValues(it.environment)) bump(counters.environment, value)
    for (const value of splitStoredValues(it.browser)) bump(counters.browser, value)
    for (const value of splitStoredValues(it.dev_project_number))
      bump(counters.dev_project_number, value)
    for (const value of splitStoredValues(it.found_version)) bump(counters.found_version, value)
    for (const value of splitStoredValues(it.occurrence)) bump(counters.occurrence, value)
    for (const value of splitStoredValues(it.solution)) bump(counters.solution, value)
    for (const value of splitStoredValues(it.critical_phenomenon))
      bump(counters.critical_phenomenon, value)
    for (const value of splitStoredValues(it.development_area))
      bump(counters.development_area, value)
    if (it.development_test_assignee_email)
      bump(counters.development_test_assignee_email, it.development_test_assignee_email)
    if (it.development_test_result)
      bump(counters.development_test_result, it.development_test_result)
    for (const run of it.qa_runs ?? []) bump(counters.qa_run, run.key)
    for (const suite of it.qa_suites ?? []) bump(counters.qa_suite, suite.key)
    if (it.qa_impact_state) bump(counters.qa_impact, it.qa_impact_state)
    {
      // Deploy stage: 'none' (not in any release) is noise — omit from facets.
      const ds = deployStateOf(it)
      if (ds !== 'none') bump(counters.deploy_state, ds)
    }
    for (const value of splitStoredValues(it.cs)) bump(counters.cs, value)
    bump(counters.jira_project, jiraProjectOf(it))
    if (it.source_project) bump(counters.source_project, it.source_project)
    for (const l of it.labels) bump(counters.labels, l)
  }

  const out = {} as Record<MultiField, FacetValue[]>
  for (const field of MULTI_FIELDS) {
    const values: FacetValue[] = [...counters[field].entries()].map(([value, count]) => ({
      value,
      count,
      label: facetLabel(field, value, all, members),
    }))
    values.sort((a, b) => b.count - a.count || (a.label < b.label ? -1 : 1))
    out[field] = values
  }
  return out
}

function facetLabel(
  field: MultiField,
  value: string,
  all: IssueLite[],
  members: Map<string, { name: string }>,
): string {
  if (field === 'status_category') return CATEGORY_LABEL(value)
  if (
    field === 'assignee_email' ||
    field === 'reporter_email' ||
    field === 'development_test_assignee_email'
  )
    return members.get(value)?.name ?? value
  if (field === 'qa_impact') {
    const labels: Record<string, string> = {
      blocking: t('filter.qaBlocking'),
      retest: t('filter.qaRetest'),
      verified: t('filter.qaVerified'),
      linked: t('filter.qaLinked'),
    }
    return labels[value] ?? value
  }
  if (field === 'deploy_state') {
    return deployStateLabel(value)
  }
  if (field === 'qa_run') {
    for (const issue of all) {
      const found = (issue.qa_runs ?? []).find((run) => run.key === value)
      if (found) return found.label
    }
  }
  if (field === 'qa_suite') {
    for (const issue of all) {
      const found = (issue.qa_suites ?? []).find((suite) => suite.key === value)
      if (found) return found.path || found.label
    }
  }
  return value
}

function bump(m: Map<string, number>, key: string | null | undefined): void {
  if (!key) return
  m.set(key, (m.get(key) ?? 0) + 1)
}

export const filters = new FiltersStore()
