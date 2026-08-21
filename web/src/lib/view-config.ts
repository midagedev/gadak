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
import { columnLabel } from './i18n'
import type { DeployState, HistoryEntry, IssueLite } from './types'

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
   * Negation twins of the two project axes (GDK-438): the include list
   * narrows first, then these subtract (a value in both lists is excluded —
   * exclude wins). Empty = no constraint. URL `pjn` / `spjn`.
   */
  jira_project_not: string[]
  source_project_not: string[]
  /**
   * Exact issue keys, given order. URL `ks` (comma-joined, uppercased).
   * Empty = no constraint. Agent ranking when no explicit sort is set.
   * Unset grouping defaults to none so that order is not shredded into buckets.
   */
  keys: string[]
  /**
   * Exact parent issue keys. URL `pk` (comma-joined). Case-insensitive.
   * Empty = no constraint. Hidden from the facet picker (same as `keys`).
   */
  parent: string[]
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
  due_from: string | null
  due_to: string | null
  resolved_from: string | null
  resolved_to: string | null
  // Local text query (instant match on key/title/assignee/labels)
  q: string
}

/** Date-range axes (URL cf/ct, uf/ut, df/dt, rf/rt). */
export type RangeField = 'created' | 'updated' | 'due' | 'resolved'

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
export type SortKey = 'updated' | 'created' | 'due' | 'priority' | 'reopen_count' | 'relevance' | 'keys'
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
  'due',
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
  'parent',
] as const
export type MultiField = (typeof MULTI_FIELDS)[number]

/** Serialized as a multi-value param but not offered as a facet picker. */
const HIDDEN_MULTI: ReadonlySet<MultiField> = new Set(['keys', 'parent'])

/* ── Multi-value negation axes (GDK-438) ──
 *  `<field>_not` excludes values after the include list has narrowed
 *  (intersection minus difference; exclude wins on overlap). These are NOT
 *  part of MULTI_FIELDS — that list drives the filter menu / facets / chips
 *  as include axes. The negation twins ride along in the same components
 *  via negationOf(). Extending negation to another axis = entries in the
 *  three maps below + one matchesMulti call in filterIssues.
 */
export const NEGATABLE_MULTI = ['jira_project', 'source_project'] as const
export type NegatableField = (typeof NEGATABLE_MULTI)[number]
export type NegationField = `${NegatableField}_not`

export const NEGATION_FIELDS: readonly NegationField[] = NEGATABLE_MULTI.map(
  (f): NegationField => `${f}_not`,
)

/** Which include field a negation field subtracts from. */
export const NEGATION_BASE = {
  jira_project_not: 'jira_project',
  source_project_not: 'source_project',
} as const satisfies Record<NegationField, NegatableField>

/** Inverse of NEGATION_BASE — whether an axis offers exclusion. */
export const FIELD_NEGATION = {
  jira_project: 'jira_project_not',
  source_project: 'source_project_not',
} as const satisfies Record<NegatableField, NegationField>

/** The negation twin of a multi field, or null when the axis is include-only. */
export function negationOf(field: MultiField): NegationField | null {
  return (NEGATABLE_MULTI as readonly string[]).includes(field)
    ? FIELD_NEGATION[field as NegatableField]
    : null
}

/**
 * Max keys accepted from a URL or saved view.
 * Same ceiling as `MaxKeys` in internal/jql/types.go (CLI CheckKeyLimit /
 * KeyLimitMessage). The two constants are one contract; view-config.test.ts
 * "KEYS_CAP matches jql.MaxKeys" fails if they drift.
 *
 * Truncation keeps the first KEYS_CAP after trim / uppercase / first-wins.
 * It is not silent: `normalizeKeys` / `parseView` return `given` (unique
 * count before the cap). On a live view, `filters.keysNormalization` is
 * the one-step answer to "why is this smaller than the ks I asked for?".
 */
export const KEYS_CAP = 500

/** Result of cap + first-wins de-dupe. Callers must take `.keys`; `.given` is in the same object. */
export interface NormalizedKeys {
  /** First KEYS_CAP unique keys, first-wins order. */
  keys: string[]
  /**
   * Unique keys after trim/uppercase/first-wins, before the cap.
   * `given > keys.length` means truncation. Equal (including 0 and KEYS_CAP)
   * means none. Same number CheckKeyLimit sees after SplitKeys.
   */
  given: number
}

/** Trim, uppercase, de-dupe (first wins), cap. Order is meaning. */
export function normalizeKeys(keys: readonly string[]): NormalizedKeys {
  const out: string[] = []
  const seen = new Set<string>()
  for (const raw of keys) {
    const k = raw.trim().toUpperCase()
    if (!k || seen.has(k)) continue
    seen.add(k)
    if (out.length < KEYS_CAP) out.push(k)
  }
  return { keys: out, given: seen.size }
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

const FLAG_FIELDS = ['reopened', 'unassigned', 'stale'] as const
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
    jira_project_not: [],
    source_project_not: [],
    keys: [],
    parent: [],
    fields: {},
    reopened: false,
    unassigned: false,
    stale: false,
    created_from: null,
    created_to: null,
    updated_from: null,
    updated_to: null,
    due_from: null,
    due_to: null,
    resolved_from: null,
    resolved_to: null,
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

/**
 * Grouping used when the URL / saved view did not name one.
 * Keys views stay flat so `views open --keys` first-seen order is the
 * painted order. An explicit `g=` or a saved `group_by` still wins.
 * `keys` is optional: Jira-imported views and pre-keys saved views omit it,
 * and SidebarNav calls configToParams on those at boot.
 */
export function defaultGroupBy(filters: { keys?: readonly string[] } | null | undefined): GroupBy {
  return (filters?.keys?.length ?? 0) > 0 ? 'none' : 'status_category'
}

export function emptyConfig(): ViewConfig {
  return { filters: emptyFilters(), display: defaultDisplay() }
}

const RANGE_AXES = ['created', 'updated', 'due', 'resolved'] as const

/** Independent copy of a filter bag (arrays cloned). */
export function cloneFilters(f: ViewFilters): ViewFilters {
  const out = emptyFilters()
  Object.assign(out, f)
  for (const field of MULTI_FIELDS) out[field] = [...(f[field] ?? [])]
  for (const field of NEGATION_FIELDS) out[field] = [...(f[field] ?? [])]
  out.fields = {}
  for (const [alias, arr] of Object.entries(f.fields ?? {})) {
    if (arr?.length) out.fields[alias] = [...arr]
  }
  return out
}

function sameMembers(a: readonly string[] | undefined, b: readonly string[] | undefined): boolean {
  const x = a ?? []
  const y = b ?? []
  if (x.length !== y.length) return false
  const ys = new Set(y)
  return x.every((v) => ys.has(v))
}

/**
 * GDK-479: view-default vs user-added. `q` is not a chip (the search box owns it).
 */
export function filtersMatchIgnoringQuery(a: ViewFilters, b: ViewFilters): boolean {
  for (const field of MULTI_FIELDS) {
    if (!sameMembers(a[field], b[field])) return false
  }
  for (const field of NEGATION_FIELDS) {
    if (!sameMembers(a[field], b[field])) return false
  }
  if (a.reopened !== b.reopened || a.unassigned !== b.unassigned || a.stale !== b.stale) return false
  for (const axis of RANGE_AXES) {
    if (a[`${axis}_from`] !== b[`${axis}_from`] || a[`${axis}_to`] !== b[`${axis}_to`]) return false
  }
  const aliases = new Set([...Object.keys(a.fields ?? {}), ...Object.keys(b.fields ?? {})])
  for (const alias of aliases) {
    if (!sameMembers(a.fields?.[alias], b.fields?.[alias])) return false
  }
  return true
}

/**
 * Values in `current` that are not in `origin`. Used to render user-added chips
 * without duplicating the applied view's defaults. `q` is copied through so a
 * caller that also inspects the query does not have to merge it back.
 */
export function subtractFilters(current: ViewFilters, origin: ViewFilters): ViewFilters {
  const out = emptyFilters()
  out.q = current.q
  for (const field of MULTI_FIELDS) {
    if (field === 'keys') continue
    const skip = new Set(origin[field] ?? [])
    out[field] = current[field].filter((v) => !skip.has(v))
  }
  // Keys is one chip for the whole list, not a per-value chip.
  out.keys = sameMembers(current.keys, origin.keys) ? [] : [...current.keys]
  for (const field of NEGATION_FIELDS) {
    const skip = new Set(origin[field] ?? [])
    out[field] = (current[field] ?? []).filter((v) => !skip.has(v))
  }
  out.reopened = current.reopened && !origin.reopened
  out.unassigned = current.unassigned && !origin.unassigned
  out.stale = current.stale && !origin.stale
  for (const axis of RANGE_AXES) {
    if (current[`${axis}_from`] === origin[`${axis}_from`] && current[`${axis}_to`] === origin[`${axis}_to`]) {
      continue
    }
    out[`${axis}_from`] = current[`${axis}_from`]
    out[`${axis}_to`] = current[`${axis}_to`]
  }
  for (const [alias, values] of Object.entries(current.fields ?? {})) {
    const skip = new Set(origin.fields?.[alias] ?? [])
    const extra = values.filter((v) => !skip.has(v))
    if (extra.length) out.fields[alias] = extra
  }
  return out
}

/* ── URL param key map (short keys keep URLs compact) ──
 *  Selected issue (?issue) and active view (?view) are not part of view serialization
 *  (owned by selection / sidebar respectively).
 */

const MULTI_KEY = {
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
  parent: 'pk',
} as const satisfies Record<MultiField, string>

/** Negation twins serialize with an n-suffixed short key (pjn / spjn). */
export const NEGATION_KEY = {
  jira_project_not: 'pjn',
  source_project_not: 'spjn',
} as const satisfies Record<NegationField, string>

/**
 * Discovered-field axes serialize as `f.<alias>` params. The alias is the
 * stable key discovery keeps across re-runs, so links survive re-discovery.
 */
export const DYN_FIELD_PREFIX = 'f.'

/** Every view-affecting param, dynamic axes included (`issue` is selection, not view). */
export function isViewParam(key: string): boolean {
  return key.startsWith(DYN_FIELD_PREFIX) || (VIEW_PARAM_KEYS as readonly string[]).includes(key)
}

const RANGE_KEY = {
  created_from: 'cf',
  created_to: 'ct',
  updated_from: 'uf',
  updated_to: 'ut',
  due_from: 'df',
  due_to: 'dt',
  resolved_from: 'rf',
  resolved_to: 'rt',
} as const

const FLAG_KEY = 'fl' // Comma-joined flag list
const Q_KEY = 'q'
const GROUP_KEY = 'g'
const SORT_KEY = 's'
const DIR_KEY = 'd'
const COLS_KEY = 'cl' // Comma-joined column list. All off = 'none'
const COLS_NONE = 'none'

type StaticViewAlias =
  | typeof Q_KEY
  | (typeof MULTI_KEY)[keyof typeof MULTI_KEY]
  | (typeof NEGATION_KEY)[keyof typeof NEGATION_KEY]
  | (typeof RANGE_KEY)[keyof typeof RANGE_KEY]
  | typeof FLAG_KEY
  | typeof GROUP_KEY
  | typeof SORT_KEY
  | typeof DIR_KEY
  | typeof COLS_KEY

/** Every param key involved in view serialization (stable viewKey; fixed order). */
export const VIEW_PARAM_KEYS = [
  Q_KEY,
  ...Object.values(MULTI_KEY),
  ...Object.values(NEGATION_KEY),
  ...Object.values(RANGE_KEY),
  FLAG_KEY,
  GROUP_KEY,
  SORT_KEY,
  DIR_KEY,
  COLS_KEY,
] as const satisfies readonly StaticViewAlias[]

export type ViewParamKey = (typeof VIEW_PARAM_KEYS)[number]

/* ── Parse: URLSearchParams → ViewConfig ── */

function splitList(v: string | null): string[] {
  if (!v) return []
  return v
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

export function parseView(params: URLSearchParams): { config: ViewConfig; keys: NormalizedKeys } {
  const f = emptyFilters()
  for (const field of MULTI_FIELDS) {
    // Disabled-feature fields stay off even from the URL (shared links must not revive dead filters).
    f[field] = fieldEnabled(field) ? splitList(params.get(MULTI_KEY[field])) : []
  }
  for (const field of NEGATION_FIELDS) {
    // Same disabled-feature rule as the include list — a negation param must
    // not revive a dead axis either. Unknown to legacy parsers: they simply
    // never read this key (measured; see view-config-negation.test.ts).
    f[field] = fieldEnabled(NEGATION_BASE[field]) ? splitList(params.get(NEGATION_KEY[field])) : []
  }
  const keys = normalizeKeys(f.keys)
  f.keys = keys.keys
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
  f.due_from = params.get(RANGE_KEY.due_from)
  f.due_to = params.get(RANGE_KEY.due_to)
  f.resolved_from = params.get(RANGE_KEY.resolved_from)
  f.resolved_to = params.get(RANGE_KEY.resolved_to)
  f.q = params.get(Q_KEY) ?? ''

  const d = defaultDisplay()
  d.group_by = defaultGroupBy(f)
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

  return { config: { filters: f, display: d }, keys }
}

export function parseConfig(params: URLSearchParams): ViewConfig {
  return parseView(params).config
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
  return ['updated', 'created', 'due', 'priority', 'reopen_count', 'relevance', 'keys'].includes(v)
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
  for (const field of NEGATION_FIELDS) {
    const arr = f[field] ?? []
    out[NEGATION_KEY[field]] = arr.length ? arr.join(',') : null
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
  out[RANGE_KEY.due_from] = f.due_from || null
  out[RANGE_KEY.due_to] = f.due_to || null
  out[RANGE_KEY.resolved_from] = f.resolved_from || null
  out[RANGE_KEY.resolved_to] = f.resolved_to || null
  out[Q_KEY] = f.q ? f.q : null

  // Omit g when it matches the contextual default (status_category normally;
  // none on a keys view). Explicit g=status_category on a keys view must
  // serialize or parseConfig would flatten it again.
  out[GROUP_KEY] = d.group_by !== defaultGroupBy(f) ? d.group_by : null
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

export type StatusCategory = 'new' | 'inprogress' | 'done'

/**
 * Times a category decision ran with an empty status_category.
 * Integer increment only — no per-row log. Read from the console or a test.
 */
let missingStatusCategoryCount = 0

export function missingStatusCategorySeen(): number {
  return missingStatusCategoryCount
}

/**
 * Times a priority filter matched by display name rather than by id.
 * Integer increment only — no per-row log. Read from the console or a test.
 */
let priorityNameFallbackCount = 0

export function priorityNameFallbackSeen(): number {
  return priorityNameFallbackCount
}

/**
 * Times effectiveCategory took the unknown-key fallback (non-empty, not a
 * trusted key or folded alias). Integer increment only — no per-row log.
 */
let categoryFallbackCount = 0

export function categoryFallbackSeen(): number {
  return categoryFallbackCount
}

/**
 * Filter token match: id first, display name only as a fallback for legacy
 * saved views that still store a localized name. Empty `selected` is no
 * constraint (same as the other multi-value axes).
 *
 * `notePriorityFallback` is the cheap signal for the priority axis: a
 * silently-empty filter used to be the only symptom when priority_id was
 * missing. Status/type leave it off.
 */
export function matchesIdFirst(
  selected: string[],
  id: string | null | undefined,
  name: string | null | undefined,
  notePriorityFallback = false,
): boolean {
  if (selected.length === 0) return true
  const idv = (id ?? '').trim()
  if (idv && selected.includes(idv)) return true
  const namev = name ?? ''
  if (namev && selected.includes(namev)) {
    if (notePriorityFallback) priorityNameFallbackCount++
    if (import.meta.env?.DEV && idv) {
      console.debug('[filter] name fallback', { id: idv, name: namev })
    }
    return true
  }
  return false
}

/**
 * Multi-value match with a negation twin (GDK-438): the include list narrows
 * first, the exclude list subtracts after — intersection minus difference, so
 * a value in both lists is excluded (exclude wins). Empty lists are no
 * constraint, same as the include-only axes. `value` is the row's single
 * token for the axis ('' when the row has none: excluded by nothing, and
 * still kept out by a non-empty include list, matching the pre-negation
 * include behavior).
 */
export function matchesMulti(include: string[], exclude: string[], value: string): boolean {
  if (include.length && !include.includes(value)) return false
  if (exclude.length && exclude.includes(value)) return false
  return true
}

/**
 * Sort key for priority_rank. The wire sends 0 for unset (not null); 0 must
 * sort as the lowest priority, never as more urgent than Highest (usually 1).
 */
export function prioritySortRank(rank: number | null | undefined): number {
  if (rank == null || rank === 0) return Number.POSITIVE_INFINITY
  return rank
}

/**
 * Trust a real new|inprogress|done status_category. Fold Jira's
 * indeterminate (REST key for in progress) and the aliases the deleted
 * web mappers already accepted (todo → new, complete/completed → done).
 * Never a status display name.
 *
 * Accepts an issue or a raw key so the list, the transition control, and
 * transition to_category strings share one decision.
 */
export function effectiveCategory(issueOrCat: IssueLite | string | null | undefined): StatusCategory {
  const raw =
    typeof issueOrCat === 'string' || issueOrCat == null
      ? (issueOrCat ?? '')
      : (issueOrCat.status_category ?? '')
  const sc = raw.toLowerCase()
  if (sc === 'new' || sc === 'todo') return 'new'
  if (sc === 'inprogress' || sc === 'indeterminate') return 'inprogress'
  if (sc === 'done' || sc === 'complete' || sc === 'completed') return 'done'
  if (!sc) missingStatusCategoryCount++
  else categoryFallbackCount++
  return 'inprogress'
}

/**
 * Reopen is a done-category → non-done status transition. Never a name match.
 * When both from_category and to_category are empty, return false: an unpainted
 * badge is a missing hint; a wrongly painted one is a false claim about the
 * issue's history.
 */
export function isReopen(e: Pick<HistoryEntry, 'field' | 'from_category' | 'to_category'>): boolean {
  if (e.field !== 'status') return false
  if (e.from_category || e.to_category) return e.from_category === 'done' && e.to_category !== 'done'
  missingStatusCategoryCount++
  return false
}

/**
 * Issue deploy stage. Missing deploy_status (older server) or empty object → 'none'.
 *  (Same meaning as backend precompute — front only derives.)
 */
export function deployStateOf(issue: IssueLite): DeployState {
  return issue.deploy_status?.state ?? 'none'
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
  // `?? []` — callers can pass partial filters from saved views that predate
  // the negation keys.
  for (const field of NEGATION_FIELDS) if ((f[field] ?? []).length) return true
  for (const alias in f.fields) if (f.fields[alias].length) return true
  if (f.reopened || f.unassigned || f.stale) return true
  if (
    f.created_from ||
    f.created_to ||
    f.updated_from ||
    f.updated_to ||
    f.due_from ||
    f.due_to ||
    f.resolved_from ||
    f.resolved_to
  )
    return true
  if (f.q.trim()) return true
  return false
}
