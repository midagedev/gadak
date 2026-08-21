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

import { config, hasServerVerb } from '../lib/config'
import { router, setParams } from '../lib/router.svelte'
import { issues } from './issues.svelte'
import { me } from './me.svelte'
import type { IssueLite, Member, PageLite, SearchMatch } from '../lib/types'
import {
  cloneFilters,
  configToParams,
  defaultColumns,
  deployStateOf,
  DYN_FIELD_PREFIX,
  effectiveCategory,
  emptyConfig,
  emptyFilters,
  filtersMatchIgnoringQuery,
  hasAnyFilter,
  isStale,
  isViewParam,
  subtractFilters,
  KEYS_CAP,
  matchesIdFirst,
  matchesMulti,
  MULTI_FIELDS,
  NEGATION_BASE,
  NEGATION_FIELDS,
  normalizeKeys,
  orderColumns,
  parseView,
  prioritySortRank,
  type NormalizedKeys,
  type ColumnKey,
  type GroupBy,
  type MultiField,
  type NegationField,
  type RangeField,
  type SortKey,
  type StatusCategory,
  type ViewConfig,
  type ViewFilters,
} from '../lib/view-config'
import { inRange as calendarInRange, localZone, type CalendarZone } from '../lib/calendar'
import * as api from '../lib/api'
import {
  categoryLabel,
  deployStateLabel,
  fieldLabel,
  t,
} from '../lib/i18n'
import { write } from './write.svelte'
import { builtinViews } from '../lib/builtin-views'

/** Outcome of `runServerSearch`. The engine does not toast or write `pages`. */
export type ServerSearchOutcome =
  | { status: 'empty' }
  | { status: 'ok'; pages: PageLite[] }
  | { status: 'error' }
  /** No server FTS in this deployment (static snapshot) — not a failure. */
  | { status: 'unavailable' }

/* ── Derived group types ── */

export interface GroupCounts {
  total: number
  category: Record<StatusCategory, number>
  severity: Record<string, number>
}

export interface IssueGroup {
  key: string
  label: string
  /**
   * Monospace identifier rendered before the label (epic key). Set only when the
   * group is named by a real issue, so `label` stays the human part alone.
   */
  prefix?: string
  items: IssueLite[]
  counts: GroupCounts
}

/** One active filter chip rendered by FilterBar. */
export interface ActiveChip {
  /** Which field/flag/range this chip represents. 'field' = discovered custom-field axis. */
  kind: 'multi' | 'flag' | 'range' | 'field' | 'keys'
  field: string
  value?: string // multi value
  label: string // display string
  /**
   * Negation chips only (GDK-438): [before, negWord, after] — the label split
   * around the negation word so FilterBar can weight just that word (the
   * polarity has to survive a scan; word order differs per locale).
   */
  negParts?: [string, string, string]
}

/* ── facet (value distribution for add dropdowns) ── */

export interface FacetValue {
  value: string
  label: string
  count: number
}

class FiltersStore {
  /**
   * GDK-479: filters the last named view applied (or the matching builtin on
   * a keep-url boot). Chip rendering subtracts these so the view highlight is
   * the only expression of the defaults. `setViewOrigin(null)` latches empty
   * (ad-hoc JQL / CLI hash): every current value is user-added.
   */
  #origin = $state<ViewFilters>(emptyFilters())
  #originLatched = $state(false)

  /* URL → stable string of view-only params. Unchanged by issue selection → blocks refilter.
   * Sorted so param order never changes the key; includes dynamic `f.<alias>` axes. */
  #viewKey = $derived(
    [...router.params.entries()]
      .filter(([k]) => isViewParam(k))
      .sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))
      .map(([k, v]) => `${k}=${v}`)
      .join('&'),
  )

  /* Config re-parsed only when #viewKey actually changes. */
  #config = $derived(parseFromKey(this.#viewKey))

  get filters(): ViewFilters {
    return this.#config.filters
  }
  get display(): ViewDisplayRef {
    return this.#config.display
  }

  /** Shared column set — every IssueRow reads this instead of allocating its own. */
  #columnSet = $derived(new Set(this.#config.display.columns))
  get columnSet(): Set<ColumnKey> {
    return this.#columnSet
  }

  /**
   * Sort actually applied. With a query and default sort (updated), auto-promote
   *  to relevance. (An explicit other sort wins — but updated is indistinguishable
   *  from "unset", so during search we treat it as relevance. Sort UI shows this too.)
   */
  get effectiveSort(): SortKey {
    const { filters: f, display: d } = this.#config
    if (d.sort !== 'updated') return d.sort
    // keys-only: keep the given order. q→relevance must not fire here.
    if (f.keys.length && !f.q.trim()) return 'keys'
    if (f.q.trim()) return 'relevance'
    return d.sort
  }

  /** Stable string of view (filter+display) params only. Unchanged by data updates →
   *  list uses this as the signal to reset scroll/cursor only when the view really changed. */
  get viewKey(): string {
    return this.#viewKey
  }

  /**
   * Unique-given vs kept for the current URL `ks`.
   * One-step debug: `given > keys.length` means the view was capped
   * (KEYS_CAP == internal/jql.MaxKeys).
   */
  get keysNormalization(): NormalizedKeys {
    const ks = router.params.get('ks')
    return normalizeKeys(ks ? ks.split(',') : [])
  }

  #keysToastSig = ''

  /**
   * Toast once per distinct truncated key list.
   * Reset the sig only when given < KEYS_CAP (cannot be a truncated view).
   * A 500-key list might be the cap remnant of a 501-key paste — do not
   * reset there or applyConfig/search would re-toast the same overflow.
   */
  notifyKeysCapped(nk: NormalizedKeys): void {
    if (nk.given <= KEYS_CAP) {
      if (nk.given < KEYS_CAP) this.#keysToastSig = ''
      return
    }
    const sig = `${nk.given}:${nk.keys.join(',')}`
    if (sig === this.#keysToastSig) return
    this.#keysToastSig = sig
    write.toast(
      t('filter.keysCapped', { given: nk.given, limit: KEYS_CAP, shown: nk.keys.length }),
      'info',
    )
  }

  /* ── Server full-text search results (volatile; not part of view serialization) ── */
  serverMatchKeys = $state<string[]>([])
  serverMatchQuery = $state('')
  searching = $state(false)
  /** Last query that failed body search (UI can offer Retry). */
  searchQuery = $state<string | null>(null)
  /** Why each hit matched, keyed by issue or page key. Empty against older servers. */
  serverMatches = $state<Record<string, SearchMatch>>({})

  /** The reason a key came back, when the server said one. Whether it is worth
   *  a line is the row's call (`matchEvidence`): a title hit is silent only
   *  while the row really shows that title highlighted. */
  matchReason(key: string): SearchMatch | null {
    const m = this.serverMatches[key]
    if (!m || !m.snippet) return null
    return m
  }

  /* ── Filter result (flat, group-agnostic, sorted) ── */
  visibleIssues = $derived.by(() => {
    const f = this.#config.filters
    const list = filterIssues(issues.allIssues, f)
    const sort = this.effectiveSort
    // Relevance needs query · recency · personalization together — pass context.
    const ctx: RelevanceContext | undefined =
      sort === 'relevance' ? buildRelevanceContext(f.q) : undefined
    return sortIssues(list, sort, this.#config.display.dir, ctx, f.keys, this.serverMatchKeys)
  })

  /* ── Grouping (display.group_by). group_by=none → single group. ── */
  groups = $derived.by(() => buildGroups(this.visibleIssues, this.#config.display.group_by))

  /* ── Reveal request: "bring this group's section into view" ──
   * A state channel rather than a DOM lookup — the breakdown bar names a group,
   * the list decides where that group sits (it owns the virtual scroll). The
   * nonce is what makes a second click on the same chip a second request. */
  #reveal = $state<{ key: string; nonce: number } | null>(null)

  /** Latest reveal request, or null when none was ever made. */
  get revealRequest(): { key: string; nonce: number } | null {
    return this.#reveal
  }

  /** Ask the list to scroll to a group. No-op on the list side when the group
   *  is not on screen (filtered away, or no list mounted). */
  revealGroup(key: string): void {
    this.#reveal = { key, nonce: (this.#reveal?.nonce ?? 0) + 1 }
  }

  /**
   * Discovered filter axes for the current scope. Only bounded value sets make
   * useful facets, so role facet/user qualifies; plain text/date/url does not.
   * When the view is scoped to projects, axes the scoped boards never fill are
   * dropped — a team board does not offer a QA board's thirty fields.
   */
  dynamicAxes = $derived.by<{ key: string; label: string }[]>(() => {
    const scoped = this.#config.filters.jira_project
    const usage = issues.fieldUsage
    const out: { key: string; label: string }[] = []
    for (const spec of issues.fieldSpecs) {
      if (spec.role !== 'facet' && spec.role !== 'user') continue
      if ((STATIC_FIELD_NAMES as readonly string[]).includes(spec.alias)) continue
      const used = scoped.length
        ? scoped.some((p) => (usage[p]?.[spec.alias] ?? 0) > 0)
        : Object.values(usage).some((m) => (m[spec.alias] ?? 0) > 0)
      // No usage data at all (older cache) → show rather than hide.
      if (Object.keys(usage).length > 0 && !used) continue
      out.push({ key: spec.alias, label: axisLabel(spec.alias, spec.label) })
    }
    return out
  })

  /** Facet values for discovered axes (same shape as `facets`, keyed by alias). */
  dynamicFacets = $derived.by<Record<string, FacetValue[]>>(() => {
    const axes = this.dynamicAxes
    if (!axes.length) return {}
    const counters: Record<string, Map<string, number>> = {}
    for (const a of axes) counters[a.key] = new Map()
    for (const it of issues.allIssues) {
      for (const a of axes) {
        for (const value of rowFieldValues(it, a.key)) bump(counters[a.key], value)
      }
    }
    const out: Record<string, FacetValue[]> = {}
    for (const a of axes) {
      const values: FacetValue[] = [...counters[a.key].entries()].map(([value, count]) => ({
        value,
        count,
        label: value,
      }))
      values.sort((x, y) => y.count - x.count || (x.label < y.label ? -1 : 1))
      out[a.key] = values
    }
    return out
  })

  /* ── FilterBar active chips ── */
  activeChips = $derived.by(() =>
    buildChips(
      subtractFilters(this.#config.filters, this.#origin),
      issues.members,
      issues.allIssues,
      new Map(issues.fieldSpecs.map((s) => [s.alias, axisLabel(s.alias, s.label)])),
    ),
  )

  hasFilters = $derived(hasAnyFilter(this.#config.filters))

  /** User-added chips only (GDK-479). Query is not a chip. */
  hasUserChips = $derived(this.activeChips.length > 0)

  /**
   * Whether the current view is already scoped to "my issues" (my account ID or email in
   *  assignee or reporter filters). Whole list is then mine, so per-row highlight
   *  is noise — turn it off.
   */
  scopedToMe = $derived.by(() => {
    const mine = [me.accountId, me.email].filter((v): v is string => !!v)
    if (!mine.length) return false
    const f = this.#config.filters
    const has = (arr: string[]) => arr.some((x) => mine.some((v) => sameIdentity(x, v)))
    return has(f.assignee_email) || has(f.reporter_email)
  })

  /** Whether this issue is assigned to me (account ID first, email compatibility fallback). */
  isMine(issue: IssueLite): boolean {
    return (
      issueMatchesPerson(issue, 'assignee', me.accountId) ||
      issueMatchesPerson(issue, 'assignee', me.email)
    )
  }

  /** facet: value distribution over the full pool (add dropdowns). Includes member name labels. */
  facets = $derived.by(() => buildFacets(issues.allIssues, issues.members))

  /* ── Mutations: all via URL update (replace, no history push) ── */

  #apply(config: ViewConfig): void {
    const params = configToParams(config)
    // A dynamic axis dropped from the config must clear its URL param too —
    // configToParams only emits keys for axes still present.
    for (const [k] of router.params) {
      if (k.startsWith(DYN_FIELD_PREFIX) && !(k in params)) params[k] = null
    }
    setParams(params, true)
  }

  /** Independent copy of current config (arrays cloned). Starting point for mutations. */
  private snapshot(): ViewConfig {
    return mergeConfig(this.#config)
  }

  /** Toggle a multi value (remove if present, else add). Negation twins share this. */
  toggleValue(field: MultiField | NegationField, value: string): void {
    const c = this.snapshot()
    const arr = c.filters[field]
    c.filters[field] = arr.includes(value) ? arr.filter((v) => v !== value) : [...arr, value]
    this.#apply(c)
  }

  /* ── Discovered-axis mutations (fields bucket, `f.<alias>` params) ── */

  toggleFieldValue(alias: string, value: string): void {
    const c = this.snapshot()
    const arr = c.filters.fields[alias] ?? []
    const next = arr.includes(value) ? arr.filter((v) => v !== value) : [...arr, value]
    if (next.length) c.filters.fields[alias] = next
    else delete c.filters.fields[alias]
    this.#apply(c)
  }

  addFieldValue(alias: string, value: string): void {
    if (!value) return
    if ((this.#config.filters.fields[alias] ?? []).includes(value)) return
    const c = this.snapshot()
    c.filters.fields[alias] = [...(c.filters.fields[alias] ?? []), value]
    this.#apply(c)
  }

  removeFieldValue(alias: string, value: string): void {
    const c = this.snapshot()
    const next = (c.filters.fields[alias] ?? []).filter((v) => v !== value)
    if (next.length) c.filters.fields[alias] = next
    else delete c.filters.fields[alias]
    this.#apply(c)
  }

  /** Add a multi value (chip click = add filter; ignore duplicates). */
  addValue(field: MultiField | NegationField, value: string): void {
    if (!value) return
    if (this.#config.filters[field].includes(value)) return
    const c = this.snapshot()
    c.filters[field] = [...c.filters[field], value]
    this.#apply(c)
  }

  removeValue(field: MultiField | NegationField, value: string): void {
    const c = this.snapshot()
    c.filters[field] = c.filters[field].filter((v) => v !== value)
    this.#apply(c)
  }

  /** Drop the keys axis (one chip, not per-key). */
  clearKeys(): void {
    if (!this.#config.filters.keys.length) return
    const c = this.snapshot()
    c.filters.keys = []
    this.#apply(c)
  }

  toggleFlag(flag: 'reopened' | 'unassigned' | 'stale'): void {
    const c = this.snapshot()
    c.filters[flag] = !c.filters[flag]
    this.#apply(c)
  }

  setRange(field: RangeField, from: string | null, to: string | null): void {
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
    this.notifyKeysCapped(normalizeKeys(config.filters?.keys ?? []))
    this.#apply(mergeConfig(config))
  }

  /**
   * Replace filters with a parsed JQL result. Keeps the current grouping and
   * columns; takes ORDER BY when the query named one.
   */
  applyJqlResult(
    parsed: {
      filters: ViewFilters
      display?: { sort?: string; dir?: string }
    },
  ): void {
    const cur = this.snapshot()
    const next = emptyConfig()
    next.filters = { ...emptyFilters(), ...parsed.filters }
    next.display.group_by = cur.display.group_by
    next.display.columns = cur.display.columns
    const sort = parsed.display?.sort
    if (
      sort === 'updated' ||
      sort === 'created' ||
      sort === 'due' ||
      sort === 'priority' ||
      sort === 'reopen_count'
    ) {
      next.display.sort = sort
    } else {
      next.display.sort = cur.display.sort
    }
    const dir = parsed.display?.dir
    next.display.dir = dir === 'asc' || dir === 'desc' ? dir : cur.display.dir
    this.applyConfig(next)
    this.setViewOrigin(null)
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

  /**
   * Named-view apply: hide these filters as chips. `null` latches empty
   * (JQL / CLI hash): every current value is a user chip.
   */
  setViewOrigin(f: ViewFilters | null): void {
    this.#origin = f ? cloneFilters(f) : emptyFilters()
    this.#originLatched = true
  }

  /**
   * keep-url boot: if the hash already is a builtin, latch it as origin so
   * category chips do not duplicate the highlight. No-op when a named view
   * already latched. Call from App after applyStartupView — `$effect` in this
   * module's constructor is an orphan (import-time, not component init).
   */
  latchOriginFromBuiltins(): void {
    if (this.#originLatched) return
    const current = this.#config.filters
    for (const v of builtinViews()) {
      if (filtersMatchIgnoringQuery(current, v.config.filters)) {
        this.setViewOrigin(v.config.filters)
        return
      }
    }
    this.setViewOrigin(null)
  }

  /** Drop user-added chips; restore the applied view. Leaves `q` and display. */
  clearUserFilters(): void {
    const c = this.snapshot()
    const q = c.filters.q
    c.filters = cloneFilters(this.#origin)
    c.filters.q = q
    this.#apply(c)
  }

  /* ── Server full-text search ── */

  async runServerSearch(): Promise<ServerSearchOutcome> {
    const q = this.#config.filters.q.trim()
    if (!q) return { status: 'empty' }
    // One gate for every caller (SearchBox, ListView, palette, docs filter,
    // history): a snapshot with no server FTS never sends the request that
    // would 404. Client-side title/key matching is untouched — this is "not
    // applicable here", not "failed".
    if (!hasServerVerb('bodySearch')) return { status: 'unavailable' }
    this.searching = true
    this.serverMatchQuery = q
    try {
      const res = await api.search(q, 200)
      this.serverMatchKeys = res.keys
      // Why each hit matched — issues and pages share one map, so it stays here
      // rather than being split across two stores.
      this.serverMatches = res.matches ?? {}
      this.searchQuery = null
      const resultKeys = [...(res.keys ?? []), ...(res.pages ?? []).map((p) => p.key)]
      me.recordSearch(q, res.total ?? resultKeys.length, resultKeys)
      return { status: 'ok', pages: res.pages ?? [] }
    } catch (e) {
      console.warn('[filters] 서버 검색 실패', e)
      this.serverMatchKeys = []
      this.serverMatches = {}
      this.searchQuery = q
      return { status: 'error' }
    } finally {
      this.searching = false
    }
  }

  clearServerSearch(): void {
    if (this.serverMatchKeys.length) this.serverMatchKeys = []
    this.serverMatches = {}
    this.serverMatchQuery = ''
    this.searchQuery = null
    me.clearSearchLink()
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
  // $derived — no toast here. App.svelte $effect + applyConfig notify.
  return parseView(new URLSearchParams(viewKey)).config
}

/** Normalize config to full shape (partial-config defense) + clone array refs. */
function mergeConfig(c: ViewConfig): ViewConfig {
  const base = emptyConfig()
  Object.assign(base.filters, c.filters)
  Object.assign(base.display, c.display)
  // Clone array refs. Columns may include ones for disabled features in a saved view — normalize.
  for (const field of MULTI_FIELDS) base.filters[field] = [...(c.filters[field] ?? [])]
  // Negation twins clone the same way (`?? []` covers saved views that predate them).
  for (const field of NEGATION_FIELDS) base.filters[field] = [...(c.filters[field] ?? [])]
  base.filters.keys = normalizeKeys(base.filters.keys).keys
  // Saved views from before dynamic axes have no fields record at all.
  base.filters.fields = {}
  for (const [alias, arr] of Object.entries(c.filters.fields ?? {})) {
    if (arr?.length) base.filters.fields[alias] = [...arr]
  }
  base.display.columns = orderColumns(c.display.columns ?? base.display.columns)
  return base
}

const IN_RANK: Record<StatusCategory, number> = { inprogress: 0, new: 1, done: 2 }

/** Row fields the static filter/facet machinery already owns — dynamic axes must not duplicate them. */
const STATIC_FIELD_NAMES = [
  ...MULTI_FIELDS,
  'assignee',
  'reporter',
  'summary',
  'labels',
  'priority',
  'status',
  'severity',
] as const

/** Discovered-axis display label: i18n override for well-known aliases, else the Jira name. */
function axisLabel(alias: string, label: string): string {
  const viaI18n = fieldLabel(alias)
  return viaI18n !== alias ? viaI18n : label || alias
}

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

/**
 * A discovered field's row value: sync flattens to string or string[]; older
 * mirrors may still carry comma-joined strings, which split covers.
 */
export function rowFieldValues(issue: IssueLite, alias: string): string[] {
  const v = (issue as unknown as Record<string, unknown>)[alias]
  if (v == null) return []
  if (Array.isArray(v)) return v.filter((x): x is string => typeof x === 'string')
  if (typeof v === 'string') return splitStoredValues(v)
  if (typeof v === 'number') return [String(v)]
  return []
}

/** AND across dynamic axes, OR within one — same semantics as the static fields. */
function matchesDynamicFields(fields: Record<string, string[]>, it: IssueLite): boolean {
  for (const alias in fields) {
    const selected = fields[alias]
    if (!selected.length) continue
    if (!matchesSelected(selected, rowFieldValues(it, alias))) return false
  }
  return true
}

/** Local text match (key/title/assignee/labels). */
function textMatch(issue: IssueLite, needle: string): boolean {
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
  // Short issue-key form (den1234). A needle that normalizes to '' must not
  // run includes('') — that would match every row.
  const nk = normKey(needle)
  if (nk && normKey(issue.issue_key).includes(nk)) return true
  return false
}

export function filterIssues(
  all: IssueLite[],
  f: ViewFilters,
  zone: CalendarZone = localZone(),
): IssueLite[] {
  const raw = f.q.trim()
  const needle = raw.toLowerCase()
  const out: IssueLite[] = []
  for (const it of all) {
    if (f.status_category.length && !f.status_category.includes(effectiveCategory(it))) continue
    if (f.status.length && !matchesIdFirst(f.status, it.status_id, it.status)) continue
    if (f.assignee_email.length && !f.assignee_email.some((v) => issueMatchesPerson(it, 'assignee', v)))
      continue
    if (f.reporter_email.length && !f.reporter_email.some((v) => issueMatchesPerson(it, 'reporter', v)))
      continue
    if (f.team_group.length && !(it.team_group && f.team_group.includes(it.team_group))) continue
    if (f.priority.length && !matchesIdFirst(f.priority, it.priority_id, it.priority, true)) continue
    if (f.severity.length && !(it.severity && f.severity.includes(it.severity))) continue
    if (f.issue_type.length && !matchesIdFirst(f.issue_type, it.issue_type_id, it.issue_type)) continue
    if (!matchesSelected(f.components, it.components)) continue
    if (!matchesSelected(f.fix_versions, it.fix_versions)) continue
    if (!matchesDynamicFields(f.fields, it)) continue
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
    if (!matchesMulti(f.jira_project, f.jira_project_not, jiraProjectOf(it))) continue
    if (!matchesMulti(f.source_project, f.source_project_not, it.source_project ?? '')) continue
    if (f.labels.length && !it.labels.some((l) => f.labels.includes(l))) continue

    if (f.keys.length) {
      const want = new Set(f.keys.map((k) => k.toUpperCase()))
      if (!want.has(it.issue_key.toUpperCase())) continue
    }
    if (f.parent.length) {
      const want = new Set(f.parent.map((k) => k.toUpperCase()))
      if (!want.has((it.parent_key ?? '').toUpperCase())) continue
    }

    if (f.reopened && !(it.reopen_count > 0)) continue
    if (f.unassigned && hasIssuePerson(it, 'assignee')) continue
    if (f.stale && !isStale(it)) continue

    if (
      (f.created_from || f.created_to) &&
      !calendarInRange(it.created_at, 'instant', f.created_from, f.created_to, zone)
    )
      continue
    if (
      (f.updated_from || f.updated_to) &&
      !calendarInRange(it.updated_at, 'instant', f.updated_from, f.updated_to, zone)
    )
      continue
    if (
      (f.due_from || f.due_to) &&
      !calendarInRange(it.duedate, 'date', f.due_from, f.due_to, zone)
    )
      continue
    if (
      (f.resolved_from || f.resolved_to) &&
      !calendarInRange(it.resolved_at, 'instant', f.resolved_from, f.resolved_to, zone)
    )
      continue

    if (needle && !textMatch(it, needle)) continue
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
 *  needle is already trim·lowercased.
 */
export interface RelevanceContext {
  needle: string
  now: number
  myEmail: string | null
  myAccountId: string | null
  recentKeys: Set<string>
}

/** Context snapshot for relevance (personalization from me store). */
function buildRelevanceContext(rawQuery: string): RelevanceContext {
  const raw = rawQuery.trim()
  return {
    needle: raw.toLowerCase(),
    now: Date.now(),
    myEmail: me.email,
    myAccountId: me.accountId,
    recentKeys: new Set(me.recentIssues.map((v) => v.key)),
  }
}

const DAY_MS = 86_400_000

function relevanceScore(issue: IssueLite, ctx: RelevanceContext): number {
  const { needle } = ctx
  const keyLower = issue.issue_key.toLowerCase()
  const summaryLower = issue.summary.toLowerCase()

  // Match strength (strongest location only as base) — key > title > assignee·labels.
  let base = 0
  if (keyLower === needle || normKey(issue.issue_key) === normKey(needle)) base = 1000
  else if (keyLower.startsWith(needle) || normKey(issue.issue_key).startsWith(normKey(needle)))
    base = 500
  else if (summaryLower.startsWith(needle)) base = 300
  else if (summaryLower.includes(needle)) base = 100

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
  if (
    issueMatchesPerson(issue, 'assignee', ctx.myAccountId) ||
    issueMatchesPerson(issue, 'assignee', ctx.myEmail)
  )
    score += 40
  if (ctx.recentKeys.has(issue.issue_key)) score += 25

  return score
}

export function sortIssues(
  list: IssueLite[],
  sort: SortKey,
  dir: 'asc' | 'desc',
  ctx?: RelevanceContext,
  keyOrder?: readonly string[],
  serverOrder?: readonly string[],
): IssueLite[] {
  const d: 1 | -1 = dir === 'asc' ? 1 : -1
  const arr = [...list]
  if (sort === 'keys') {
    const order = new Map<string, number>()
    for (const [i, raw] of (keyOrder ?? []).entries()) {
      const k = raw.toUpperCase()
      if (!order.has(k)) order.set(k, i)
    }
    arr.sort((a, b) => {
      const ai = order.get(a.issue_key.toUpperCase()) ?? Number.MAX_SAFE_INTEGER
      const bi = order.get(b.issue_key.toUpperCase()) ?? Number.MAX_SAFE_INTEGER
      return ai - bi
    })
    return arr
  }
  if (sort === 'relevance') {
    // Relevance ignores dir (always higher score first). Cache scores; ties by newest update.
    // When the server ranked a set, that order is the primary key for those
    // issues; local score only ranks issues the server did not return.
    const rc = ctx ?? buildRelevanceContext('')
    const scoreOf = new Map<string, number>()
    for (const it of arr) scoreOf.set(it.issue_key, relevanceScore(it, rc))
    let serverAt: Map<string, number> | undefined
    if (serverOrder?.length) {
      serverAt = new Map()
      for (const [i, raw] of serverOrder.entries()) {
        const k = raw.toUpperCase()
        if (!serverAt.has(k)) serverAt.set(k, i)
      }
    }
    arr.sort((a, b) => {
      if (serverAt) {
        const ai = serverAt.get(a.issue_key.toUpperCase())
        const bi = serverAt.get(b.issue_key.toUpperCase())
        const aIn = ai !== undefined
        const bIn = bi !== undefined
        if (aIn && bIn && ai !== bi) return (ai as number) - (bi as number)
        if (aIn && !bIn) return -1
        if (!aIn && bIn) return 1
      }
      const diff = (scoreOf.get(b.issue_key) ?? 0) - (scoreOf.get(a.issue_key) ?? 0)
      return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
    })
    return arr
  }
  arr.sort((a, b) => {
    switch (sort) {
      case 'created':
        return cmpStr(a.created_at, b.created_at, d)
      case 'due':
        return cmpStr(a.duedate ?? '', b.duedate ?? '', d)
      case 'reopen_count': {
        const diff = (a.reopen_count - b.reopen_count) * d
        return diff !== 0 ? diff : cmpStr(a.updated_at, b.updated_at, -1)
      }
      case 'priority': {
        // priority_rank: lower = higher priority. Unset is 0 on the wire (not
        // null) and always sorts last, so untriaged never outranks Highest.
        const ar = prioritySortRank(a.priority_rank)
        const br = prioritySortRank(b.priority_rank)
        const aUnset = ar === Number.POSITIVE_INFINITY
        const bUnset = br === Number.POSITIVE_INFINITY
        if (aUnset && bUnset) return cmpStr(a.updated_at, b.updated_at, -1)
        if (aUnset) return 1
        if (bUnset) return -1
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

function groupKeyOf(
  issue: IssueLite,
  by: GroupBy,
): { key: string; label: string; prefix?: string } {
  switch (by) {
    case 'status_category': {
      const category = effectiveCategory(issue)
      const label = categoryLabel(category)
      return { key: category, label }
    }
    case 'status':
      return {
        key: issue.status_id || issue.status || '(none)',
        label: issue.status || t('group.noStatus'),
      }
    case 'assignee':
      return personIdentity(issue, 'assignee')
        ? {
            key: personIdentity(issue, 'assignee')!,
            label: issue.assignee || issue.assignee_email || personIdentity(issue, 'assignee')!,
          }
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
      return {
        key: issue.issue_type_id || issue.issue_type || '',
        label: issue.issue_type || t('group.noType'),
      }
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
    case 'epic': {
      const epicKey = issue.epic_key
      if (!epicKey) return { key: '', label: t('group.noEpic') }
      // The epic is mirrored like any other issue, so its summary is already in
      // the pool — key alone when it is not (partial mirror / narrowed project set).
      const epic = issues.pool.get(epicKey)
      return epic
        ? { key: epicKey, label: epic.summary, prefix: epicKey }
        : { key: epicKey, label: epicKey }
    }
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
    const { key, label, prefix } = groupKeyOf(it, by)
    let g = map.get(key)
    if (!g) {
      g = { key, label, prefix, items: [], counts: emptyCounts() }
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
    // Epics sort by key so one project's epics stay adjacent; summaries would
    // interleave projects and the order would move whenever an epic is renamed.
    if (by === 'epic') return a.key < b.key ? -1 : a.key > b.key ? 1 : 0
    return a.label < b.label ? -1 : a.label > b.label ? 1 : 0
  })
  return groups
}

function rankOf(issue: IssueLite | undefined): number {
  return prioritySortRank(issue?.priority_rank)
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
  members: Map<string, Member>,
  all: IssueLite[],
  fieldLabels: Map<string, string>,
): ActiveChip[] {
  const chips: ActiveChip[] = []
  if (f.keys.length) {
    chips.push({
      kind: 'keys',
      field: 'keys',
      label: t('filter.chipKeys', { n: f.keys.length }),
    })
  }
  for (const field of MULTI_FIELDS) {
    if (field === 'keys') continue
    for (const value of f[field]) {
      let label = value
      if (field === 'status_category') label = CATEGORY_LABEL(value)
      else if (field === 'assignee_email' || field === 'reporter_email')
        label = facetLabel(field, value, all, members)
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
  // Negation chips (GDK-438): labeled with the base axis's name — a negation
  // field has no fieldLabel entry of its own and should not grow one.
  for (const field of NEGATION_FIELDS) {
    for (const value of f[field] ?? []) {
      // Split the label around the negation word via a sentinel: the word's
      // position is the locale's business, the emphasis is the chip's.
      const negWord = t('filter.chipNegWord')
      const raw = t('filter.chipFieldValueNot', {
        field: FIELD_LABEL(NEGATION_BASE[field]),
        value,
        neg: '\u0000',
      })
      const [before, after] = raw.split('\u0000')
      chips.push({
        kind: 'multi',
        field,
        value,
        label: raw.replace('\u0000', negWord),
        negParts: [before ?? '', negWord, after ?? ''],
      })
    }
  }
  for (const [alias, values] of Object.entries(f.fields)) {
    const name = fieldLabels.get(alias) ?? FIELD_LABEL(alias)
    for (const value of values) {
      chips.push({ kind: 'field', field: alias, value, label: t('filter.chipFieldValue', { field: name, value }) })
    }
  }
  if (f.reopened) chips.push({ kind: 'flag', field: 'reopened', label: t('filter.flagReopened') })
  if (f.unassigned) chips.push({ kind: 'flag', field: 'unassigned', label: t('filter.flagUnassigned') })
  if (f.stale) chips.push({ kind: 'flag', field: 'stale', label: t('filter.flagStale') })
  if (f.created_from || f.created_to)
    chips.push({ kind: 'range', field: 'created', label: t('filter.chipCreatedRange', { from: f.created_from ?? '', to: f.created_to ?? '' }) })
  if (f.updated_from || f.updated_to)
    chips.push({ kind: 'range', field: 'updated', label: t('filter.chipUpdatedRange', { from: f.updated_from ?? '', to: f.updated_to ?? '' }) })
  if (f.due_from || f.due_to)
    chips.push({ kind: 'range', field: 'due', label: t('filter.chipDueRange', { from: f.due_from ?? '', to: f.due_to ?? '' }) })
  if (f.resolved_from || f.resolved_to)
    chips.push({
      kind: 'range',
      field: 'resolved',
      label: t('filter.chipResolvedRange', { from: f.resolved_from ?? '', to: f.resolved_to ?? '' }),
    })
  return chips
}

/* ── facet ── */

function buildFacets(
  all: IssueLite[],
  members: Map<string, Member>,
): Record<MultiField, FacetValue[]> {
  const counters: Record<string, Map<string, number>> = {}
  for (const field of MULTI_FIELDS) {
    if (field === 'keys' || field === 'parent') continue
    counters[field] = new Map()
  }
  const assigneeLabels = new Map<string, string>()
  const reporterLabels = new Map<string, string>()
  const statusLabels = new Map<string, string>()
  const typeLabels = new Map<string, string>()
  const priorityLabels = new Map<string, string>()

  for (const it of all) {
    bump(counters.status_category, effectiveCategory(it))
    const statusToken = it.status_id || it.status
    if (statusToken) {
      bump(counters.status, statusToken)
      if (!statusLabels.has(statusToken)) statusLabels.set(statusToken, it.status || statusToken)
    }
    const assignee = personIdentity(it, 'assignee')
    const reporter = personIdentity(it, 'reporter')
    bump(counters.assignee_email, assignee)
    bump(counters.reporter_email, reporter)
    if (assignee && !assigneeLabels.has(assignee))
      assigneeLabels.set(assignee, it.assignee || it.assignee_email || assignee)
    if (reporter && !reporterLabels.has(reporter))
      reporterLabels.set(reporter, it.reporter || it.reporter_email || reporter)
    if (it.team_group) bump(counters.team_group, it.team_group)
    {
      const priorityToken = it.priority_id || it.priority
      if (priorityToken) {
        bump(counters.priority, priorityToken)
        if (!priorityLabels.has(priorityToken)) priorityLabels.set(priorityToken, it.priority || priorityToken)
      }
    }
    if (it.severity) bump(counters.severity, it.severity)
    {
      const typeToken = it.issue_type_id || it.issue_type
      if (typeToken) {
        bump(counters.issue_type, typeToken)
        if (!typeLabels.has(typeToken)) typeLabels.set(typeToken, it.issue_type || typeToken)
      }
    }
    for (const value of it.components) bump(counters.components, value)
    for (const value of it.fix_versions) bump(counters.fix_versions, value)
    for (const run of it.qa_runs ?? []) bump(counters.qa_run, run.key)
    for (const suite of it.qa_suites ?? []) bump(counters.qa_suite, suite.key)
    if (it.qa_impact_state) bump(counters.qa_impact, it.qa_impact_state)
    {
      // Deploy stage: 'none' (not in any release) is noise — omit from facets.
      const ds = deployStateOf(it)
      if (ds !== 'none') bump(counters.deploy_state, ds)
    }
    bump(counters.jira_project, jiraProjectOf(it))
    if (it.source_project) bump(counters.source_project, it.source_project)
    for (const l of it.labels) bump(counters.labels, l)
  }

  const out = {} as Record<MultiField, FacetValue[]>
  out.keys = []
  out.parent = []
  for (const field of MULTI_FIELDS) {
    if (field === 'keys' || field === 'parent') continue
    const values: FacetValue[] = [...counters[field].entries()].map(([value, count]) => {
      const personLabel =
        field === 'assignee_email'
          ? assigneeLabels.get(value)
          : field === 'reporter_email'
            ? reporterLabels.get(value)
            : undefined
      const named =
        field === 'status'
          ? statusLabels.get(value)
          : field === 'issue_type'
            ? typeLabels.get(value)
            : field === 'priority'
              ? priorityLabels.get(value)
              : undefined
      return { value, count, label: personLabel ?? named ?? facetLabel(field, value, all, members) }
    })
    values.sort((a, b) => b.count - a.count || (a.label < b.label ? -1 : 1))
    out[field] = values
  }
  return out
}

function facetLabel(
  field: MultiField,
  value: string,
  all: IssueLite[],
  members: Map<string, Member>,
): string {
  if (field === 'status_category') return CATEGORY_LABEL(value)
  if (field === 'assignee_email' || field === 'reporter_email') {
    const role = field === 'assignee_email' ? 'assignee' : 'reporter'
    for (const issue of all) {
      if (!issueMatchesPerson(issue, role, value)) continue
      return issue[role] || issue[`${role}_email`] || value
    }
    const direct = members.get(value)
    if (direct) return direct.name
    for (const member of members.values()) {
      if (member.jira_account_id === value) return member.name
    }
    return value
  }
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

type PersonRole = 'assignee' | 'reporter'

export function personIdentity(issue: IssueLite, role: PersonRole): string | null {
  return issue[`${role}_id`] || issue[`${role}_email`] || null
}

function sameIdentity(a: string, b: string): boolean {
  return a === b || (a.includes('@') && b.includes('@') && a.toLowerCase() === b.toLowerCase())
}

export function issueMatchesPerson(
  issue: IssueLite,
  role: PersonRole,
  value: string | null | undefined,
): boolean {
  if (!value) return false
  const id = issue[`${role}_id`]
  const email = issue[`${role}_email`]
  if ((id && sameIdentity(id, value)) || (email && sameIdentity(email, value))) return true
  // During a cache upgrade either side can temporarily carry only one identity
  // shape. The member directory bridges a known email alias and its account ID
  // without adding a network call to the filter path.
  const member = value.includes('@') ? issues.memberOf(value) : issues.memberOfAccountId(value)
  if (!member) return false
  if (id && member.jira_account_id && sameIdentity(id, member.jira_account_id)) return true
  return Boolean(email && sameIdentity(email, member.email))
}

function hasIssuePerson(issue: IssueLite, role: PersonRole): boolean {
  return Boolean(issue[`${role}_id`] || issue[role] || issue[`${role}_email`])
}

function bump(m: Map<string, number>, key: string | null | undefined): void {
  if (!key) return
  m.set(key, (m.get(key) ?? 0) + 1)
}

export const filters = new FiltersStore()
