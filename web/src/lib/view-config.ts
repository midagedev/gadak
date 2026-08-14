/*
 * Issue Navigator — view config (filters + display) types/serialize/parse ([explore])
 *
 * "View = serialized filter+display object" (work spec). This module is the single
 * schema definition.
 *  - Round-trip URL hash query params ↔ ViewConfig (setParams writes, router.params restores)
 *  - Same shape serializes into saved views (personal localStorage / team API);
 *    server treats config as opaque JSON
 *
 * Principle: filters are "what" (multi-value OR + booleans + ranges + text);
 *  display is "how" (grouping/sort). Independent axes; both computed as local
 *  derivations (no server round-trip).
 */

import { config, feature, type GadakFeatures } from './config'
import { columnLabel, deployStateLabel } from './i18n'
import type { DeployState, IssueLite } from './types'

/* ── Filter state ── */

/** Multi-value filter fields (OR within a field, AND across fields). */
export interface ViewFilters {
  status_category: string[] // new | inprogress | done (effective buckets)
  status: string[] // Raw Jira status strings
  /** Compatibility name: values are Jira account IDs when available, legacy emails otherwise. */
  assignee_email: string[]
  /** Compatibility name: values are Jira account IDs when available, legacy emails otherwise. */
  reporter_email: string[]
  team_group: string[]
  labels: string[]
  priority: string[]
  severity: string[]
  issue_type: string[]
  components: string[]
  fix_versions: string[]
  qa_run: string[]
  qa_suite: string[]
  qa_impact: string[]
  deploy_state: string[] // Deploy stage (merged/dev/qa_preview/qa/prod)
  jira_project: string[] // issue_key prefix (ABC / XYZ ...)
  source_project: string[]
  /**
   * Exact issue keys, given order. URL `ks` (comma-joined, uppercased).
   * Empty = no constraint. Agent ranking when no explicit sort is set.
   */
  keys: string[]
  /**
   * Discovered custom-field axes, keyed by spec alias. Which axes exist comes
   * from bootstrap field_specs, not from this schema — a board that uses 30
   * fields gets 30 possible axes, one that uses 2 gets 2. Serialized as
   * `f.<alias>` URL params.
   */
  fields: Record<string, string[]>
  // Boolean flags
  reopened: boolean
  unassigned: boolean
  stale: boolean
  // Date ranges (ISO date, inclusive). null = unset
  created_from: string | null
  created_to: string | null
  updated_from: string | null
  updated_to: string | null
  // Local text query (instant match on key/title/assignee/labels)
  q: string
}

export type GroupBy =
  | 'none'
  | 'status_category'
  | 'status'
  | 'assignee'
  | 'priority'
  | 'severity'
  | 'team_group'
  | 'product'
  | 'issue_type'
  | 'development_test_result'
  | 'qa_impact'
  | 'source_project'
  | 'epic'
// 'relevance' = rank by search relevance when a query is present. Auto-promoted from
// the default sort (updated); only serialized into the URL when explicitly chosen
// (older URLs do not know relevance — keep backward compatible).
export type SortKey = 'updated' | 'created' | 'priority' | 'reopen_count' | 'relevance' | 'keys'
export type SortDir = 'asc' | 'desc'

/* ── List columns (trailing fields shown on a row) ──
 *  Layout is "keep dense rows + field on/off" — only checked columns render on the right.
 *  Column set is part of display, so it serializes into URL and saved views (per-view columns).
 */
export const COLUMN_KEYS_ALL = [
  'assignee',
  'updated',
  'labels',
  'reopen',
  'stale',
  'qa_impact',
  'deploy',
  'severity',
  'issue_type',
  'status',
  'reporter',
  'comment_count',
  'fix_versions',
  'components',
  'created',
  'environment',
  'team_group',
  'dev_test_result',
] as const
export type ColumnKey = (typeof COLUMN_KEYS_ALL)[number]

/** Column catalog entry (label from active locale). */
export interface ColumnDef {
  key: ColumnKey
  label: string
}

/** Labels computed at call time for the active locale. */
export function COLUMNS(): ColumnDef[] {
  return COLUMN_KEYS_ALL.map((key) => ({ key, label: columnLabel(key) }))
}

const COLUMN_KEYS = COLUMN_KEYS_ALL as readonly ColumnKey[]

/** Columns tied to optional features — drop from the catalog when the flag is off. */
const COLUMN_FEATURE: Partial<Record<ColumnKey, keyof GadakFeatures>> = {
  qa_impact: 'qa',
  deploy: 'deploy',
  team_group: 'teamGroups',
}

function columnEnabled(key: ColumnKey): boolean {
  const f = COLUMN_FEATURE[key]
  return !f || feature(f)
}

/** Catalog the columns menu exposes (excludes columns for disabled features). */
export function columnCatalog(): ColumnDef[] {
  return COLUMNS().filter((c) => columnEnabled(c.key))
}

const DEFAULT_COLUMN_KEYS: ColumnKey[] = [
  'assignee',
  'updated',
  'labels',
  'reopen',
  'stale',
  'qa_impact',
  'deploy',
]

/** Default visible columns — matches current row behavior (conditional badges only when data exists). */
export function defaultColumns(): ColumnKey[] {
  return DEFAULT_COLUMN_KEYS.filter(columnEnabled)
}

function isColumnKey(v: string): v is ColumnKey {
  return (COLUMN_KEYS as readonly string[]).includes(v)
}

/** Normalize an arbitrary key list into catalog order (valid+enabled only; empty list allowed = all off). */
export function orderColumns(keys: readonly string[]): ColumnKey[] {
  const set = new Set(keys.filter(isColumnKey) as ColumnKey[])
  return COLUMN_KEYS.filter((k) => set.has(k) && columnEnabled(k))
}

export interface ViewDisplay {
  group_by: GroupBy
  sort: SortKey
  dir: SortDir
  columns: ColumnKey[]
}

export interface ViewConfig {
  filters: ViewFilters
  display: ViewDisplay
}

/* ── Multi-value field list (reused by meta-driven logic) ── */

export const MULTI_FIELDS = [
  'status_category',
  'status',
  'assignee_email',
  'reporter_email',
  'team_group',
  'labels',
  'priority',
  'severity',
  'issue_type',
  'components',
  'fix_versions',
  'qa_run',
  'qa_suite',
  'qa_impact',
  'deploy_state',
  'jira_project',
  'source_project',
  'keys',
] as const
export type MultiField = (typeof MULTI_FIELDS)[number]

/** Serialized as a multi-value param but not offered as a facet picker. */
const HIDDEN_MULTI: ReadonlySet<MultiField> = new Set(['keys'])

/** Max keys accepted from a URL or saved view (same cap as the CLI). */
export const KEYS_CAP = 500

/** Trim, uppercase, de-dupe (first wins), cap. Order is meaning. */
export function normalizeKeys(keys: readonly string[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const raw of keys) {
    const k = raw.trim().toUpperCase()
    if (!k || seen.has(k)) continue
    seen.add(k)
    out.push(k)
    if (out.length >= KEYS_CAP) break
  }
  return out
}

/** Filter fields from optional features — invalid in both filter menu and URL when flag is off. */
const FIELD_FEATURE: Partial<Record<MultiField, keyof GadakFeatures>> = {
  team_group: 'teamGroups',
  qa_run: 'qa',
  qa_suite: 'qa',
  qa_impact: 'qa',
  deploy_state: 'deploy',
}

export function fieldEnabled(field: MultiField): boolean {
  const f = FIELD_FEATURE[field]
  return !f || feature(f)
}

/** Fields the filter menu exposes (excludes fields for disabled features). */
export function filterFields(): MultiField[] {
  return MULTI_FIELDS.filter((f) => !HIDDEN_MULTI.has(f) && fieldEnabled(f))
}

/** Grouping axes from optional features. */
const GROUP_FEATURE: Partial<Record<GroupBy, keyof GadakFeatures>> = {
  team_group: 'teamGroups',
  product: 'teamGroups',
  qa_impact: 'qa',
}

export function groupByEnabled(by: GroupBy): boolean {
  const f = GROUP_FEATURE[by]
  return !f || feature(f)
}

export const FLAG_FIELDS = ['reopened', 'unassigned', 'stale'] as const
export type FlagField = (typeof FLAG_FIELDS)[number]

/* ── Defaults ── */

export function emptyFilters(): ViewFilters {
  return {
    status_category: [],
    status: [],
    assignee_email: [],
    reporter_email: [],
    team_group: [],
    labels: [],
    priority: [],
    severity: [],
    issue_type: [],
    components: [],
    fix_versions: [],
    qa_run: [],
    qa_suite: [],
    qa_impact: [],
    deploy_state: [],
    jira_project: [],
    source_project: [],
    keys: [],
    fields: {},
    reopened: false,
    unassigned: false,
    stale: false,
    created_from: null,
    created_to: null,
    updated_from: null,
    updated_to: null,
    q: '',
  }
}

export function defaultDisplay(): ViewDisplay {
  return {
    group_by: 'status_category',
    sort: 'updated',
    dir: 'desc',
    columns: defaultColumns(),
  }
}

export function emptyConfig(): ViewConfig {
  return { filters: emptyFilters(), display: defaultDisplay() }
}

/* ── URL param key map (short keys keep URLs compact) ──
 *  Selected issue (?issue) and active view (?view) are not part of view serialization
 *  (owned by selection / sidebar respectively).
 */

const MULTI_KEY: Record<MultiField, string> = {
  status_category: 'sc',
  status: 'st',
  assignee_email: 'as',
  reporter_email: 'rp',
  team_group: 'gr',
  labels: 'lb',
  priority: 'pr',
  severity: 'sv',
  issue_type: 'ty',
  components: 'co',
  fix_versions: 'fx',
  qa_run: 'qr',
  qa_suite: 'qs',
  qa_impact: 'qi',
  deploy_state: 'ds',
  jira_project: 'pj',
  source_project: 'spj',
  keys: 'ks',
}

/**
 * Discovered-field axes serialize as `f.<alias>` params. The alias is the
 * stable key discovery keeps across re-runs, so links survive re-discovery.
 */
export const DYN_FIELD_PREFIX = 'f.'

/** Every view-affecting param, dynamic axes included (`issue` is selection, not view). */
export function isViewParam(key: string): boolean {
  return key.startsWith(DYN_FIELD_PREFIX) || VIEW_PARAM_KEYS.includes(key)
}

const RANGE_KEY = {
  created_from: 'cf',
  created_to: 'ct',
  updated_from: 'uf',
  updated_to: 'ut',
} as const

const FLAG_KEY = 'fl' // Comma-joined flag list
const Q_KEY = 'q'
const GROUP_KEY = 'g'
const SORT_KEY = 's'
const DIR_KEY = 'd'
const COLS_KEY = 'cl' // Comma-joined column list. All off = 'none'
const COLS_NONE = 'none'

/** Every param key involved in view serialization (stable viewKey; fixed order). */
export const VIEW_PARAM_KEYS: string[] = [
  Q_KEY,
  ...Object.values(MULTI_KEY),
  ...Object.values(RANGE_KEY),
  FLAG_KEY,
  GROUP_KEY,
  SORT_KEY,
  DIR_KEY,
  COLS_KEY,
]

/* ── Parse: URLSearchParams → ViewConfig ── */

function splitList(v: string | null): string[] {
  if (!v) return []
  return v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

export function parseConfig(params: URLSearchParams): ViewConfig {
  const f = emptyFilters()
  for (const field of MULTI_FIELDS) {
    // Disabled-feature fields stay off even from the URL (shared links must not revive dead filters).
    f[field] = fieldEnabled(field) ? splitList(params.get(MULTI_KEY[field])) : []
  }
  f.keys = normalizeKeys(f.keys)
  // Discovered-field axes: every `f.<alias>` param. Unknown aliases parse too —
  // a shared link may name a field this mirror simply has not discovered yet.
  for (const [key, raw] of params) {
    if (!key.startsWith(DYN_FIELD_PREFIX)) continue
    const alias = key.slice(DYN_FIELD_PREFIX.length)
    const values = splitList(raw)
    if (alias && values.length) f.fields[alias] = values
  }
  const flags = splitList(params.get(FLAG_KEY))
  f.reopened = flags.includes('reopened')
  f.unassigned = flags.includes('unassigned')
  f.stale = flags.includes('stale')
  f.created_from = params.get(RANGE_KEY.created_from)
  f.created_to = params.get(RANGE_KEY.created_to)
  f.updated_from = params.get(RANGE_KEY.updated_from)
  f.updated_to = params.get(RANGE_KEY.updated_to)
  f.q = params.get(Q_KEY) ?? ''

  const d = defaultDisplay()
  const g = params.get(GROUP_KEY)
  if (g && isGroupBy(g)) d.group_by = g
  const s = params.get(SORT_KEY)
  if (s && isSortKey(s)) d.sort = s
  const dir = params.get(DIR_KEY)
  if (dir === 'asc' || dir === 'desc') d.dir = dir
  // Empty string (cl=) means "unset" → keep defaults: #viewKey re-serialization always
  // emits `cl=` for unset keys too. All-off is only the explicit sentinel 'none'.
  const cl = params.get(COLS_KEY)
  if (cl) d.columns = cl === COLS_NONE ? [] : orderColumns(splitList(cl))

  return { filters: f, display: d }
}

function isGroupBy(v: string): v is GroupBy {
  return (
    [
      'none',
      'status_category',
      'status',
      'assignee',
      'priority',
      'severity',
      'team_group',
      'product',
      'issue_type',
      'development_test_result',
      'qa_impact',
      'source_project',
      'epic',
    ].includes(v) && groupByEnabled(v as GroupBy)
  )
}
function isSortKey(v: string): v is SortKey {
  return ['updated', 'created', 'priority', 'reopen_count', 'relevance', 'keys'].includes(v)
}

/* ── Serialize: ViewConfig → URL param delta ──
 *  Empty values become null so setParams drops the key (keeps URLs clean).
 */

export function configToParams(config: ViewConfig): Record<string, string | null> {
  const f = config.filters ?? emptyFilters()
  const d = config.display ?? defaultDisplay()
  const out: Record<string, string | null> = {}

  for (const field of MULTI_FIELDS) {
    const arr = f[field] ?? []
    out[MULTI_KEY[field]] = arr.length ? arr.join(',') : null
  }
  for (const [alias, arr] of Object.entries(f.fields ?? {})) {
    out[DYN_FIELD_PREFIX + alias] = arr.length ? arr.join(',') : null
  }
  const flags: string[] = []
  if (f.reopened) flags.push('reopened')
  if (f.unassigned) flags.push('unassigned')
  if (f.stale) flags.push('stale')
  out[FLAG_KEY] = flags.length ? flags.join(',') : null

  out[RANGE_KEY.created_from] = f.created_from || null
  out[RANGE_KEY.created_to] = f.created_to || null
  out[RANGE_KEY.updated_from] = f.updated_from || null
  out[RANGE_KEY.updated_to] = f.updated_to || null
  out[Q_KEY] = f.q ? f.q : null

  // Status-category grouping is the default. Explicit g=none preserves "no sections".
  out[GROUP_KEY] = d.group_by !== 'status_category' ? d.group_by : null
  out[SORT_KEY] = d.sort !== 'updated' ? d.sort : null
  out[DIR_KEY] = d.dir !== 'desc' ? d.dir : null

  // Columns: omit when default (clean URL); all-off preserved as 'none'.
  // A Jira-imported view may omit columns; treat that as the catalog default.
  const cols = d.columns ?? defaultColumns()
  const def = defaultColumns()
  const colsEqDefault = cols.length === def.length && cols.every((c, i) => c === def[i])
  out[COLS_KEY] = colsEqDefault ? null : cols.length ? cols.join(',') : COLS_NONE

  return out
}

/* ── Filter application helpers ── */

/**
 * Fallback when status_category is missing. Status names vary by site/account language
 * and cannot be trusted, so keep only generic names that mean the same on every Jira.
 */
export const RESOLVED_STATUS_NAMES = new Set([
  'resolved',
  'closed',
  'done',
  '해결됨',
  '종료',
  '완료',
])

export type StatusCategory = 'new' | 'inprogress' | 'done'

/**
 * Filter token match: id first, display name only as a fallback for legacy
 * saved views that still store a localized name. Empty `selected` is no
 * constraint (same as the other multi-value axes).
 */
export function matchesIdFirst(
  selected: string[],
  id: string | null | undefined,
  name: string | null | undefined,
): boolean {
  if (selected.length === 0) return true
  const idv = (id ?? '').trim()
  if (idv && selected.includes(idv)) return true
  const namev = name ?? ''
  if (namev && selected.includes(namev)) {
    if (import.meta.env?.DEV && idv) {
      console.debug('[filter] name fallback', { id: idv, name: namev })
    }
    return true
  }
  return false
}

/**
 * Sort key for priority_rank. The wire sends 0 for unset (not null); 0 must
 * sort as the lowest priority, never as more urgent than Highest (usually 1).
 */
export function prioritySortRank(rank: number | null | undefined): number {
  if (rank == null || rank === 0) return Number.POSITIVE_INFINITY
  return rank
}

/** Trust server status_category first; fall back to status name only when absent. */
export function effectiveCategory(issue: IssueLite): StatusCategory {
  const sc = (issue.status_category ?? '').toLowerCase()
  if (sc === 'new' || sc === 'inprogress' || sc === 'done') return sc
  if (RESOLVED_STATUS_NAMES.has((issue.status ?? '').trim().toLowerCase())) return 'done'
  return 'inprogress'
}

/**
 * Issue deploy stage. Missing deploy_status (older server) or empty object → 'none'.
 *  (Same meaning as backend precompute — front only derives.)
 */
export function deployStateOf(issue: IssueLite): DeployState {
  return issue.deploy_status?.state ?? 'none'
}

/** Deploy-stage labels (shared by filter facets/chips) — active locale. */
export function DEPLOY_STATE_LABEL(): Record<DeployState, string> {
  return {
    none: deployStateLabel('none'),
    merged: deployStateLabel('merged'),
    dev: deployStateLabel('dev'),
    qa_preview: deployStateLabel('qa_preview'),
    qa: deployStateLabel('qa'),
    prod: deployStateLabel('prod'),
  }
}

/** Label for a single deploy stage. */
export function deployLabel(state: DeployState): string {
  return deployStateLabel(state)
}

/**
 * Hours spent in the current status. Prefer status_changed_at; fall back to updated_at.
 * 0 when both are missing — no evidence, so do not count as stale.
 */
export function statusAgeHours(issue: IssueLite): number {
  const iso = issue.status_changed_at ?? issue.updated_at
  if (!iso) return 0
  const t = Date.parse(iso)
  return Number.isFinite(t) ? Math.max(0, (Date.now() - t) / 3_600_000) : 0
}

/** Stale check: not done + hours in current status exceed config.staleThresholdHours. */
export function isStale(issue: IssueLite): boolean {
  if (effectiveCategory(issue) === 'done') return false
  return statusAgeHours(issue) > config().staleThresholdHours
}

/** Whether any filter is active (for save-view button). Callers decide whether to exclude q. */
export function hasAnyFilter(f: ViewFilters): boolean {
  for (const field of MULTI_FIELDS) if (f[field].length) return true
  for (const alias in f.fields) if (f.fields[alias].length) return true
  if (f.reopened || f.unassigned || f.stale) return true
  if (f.created_from || f.created_to || f.updated_from || f.updated_to) return true
  if (f.q.trim()) return true
  return false
}
