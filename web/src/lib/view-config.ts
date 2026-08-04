/*
 * Issue Navigator — 뷰 설정(필터 + 디스플레이)의 타입·직렬화·파싱 ([explore])
 *
 * "뷰 = 필터+디스플레이 직렬화 객체"(작업 스펙). 이 모듈이 그 스키마의 단일 정의다.
 *  - URL 해시 쿼리파람 ↔ ViewConfig 왕복 (setParams 로 반영, router.params 에서 복원)
 *  - 저장 뷰(개인 localStorage / 팀 api)의 config 로도 그대로 직렬화된다(서버는 불투명 JSON 취급)
 *
 * 원칙: 필터는 "무엇을"(다중값 OR + 불리언 + 기간 + 텍스트), 디스플레이는 "어떻게"(그룹핑/정렬).
 *  둘은 독립 축이며 모두 로컬 파생으로 계산된다(서버 왕복 없음).
 */

import { config, feature, type ScryFeatures } from './config'
import type { DeployState, IssueLite } from './types'

/* ── 필터 상태 ── */

/** 다중값 필터 필드(전부 OR 매칭, 필드 간은 AND). */
export interface ViewFilters {
  status_category: string[] // new | inprogress | done (유효 분류)
  status: string[] // Jira 원본 상태 문자열
  assignee_email: string[]
  reporter_email: string[]
  d1_group: string[]
  labels: string[]
  priority: string[]
  severity: string[]
  issue_type: string[]
  components: string[]
  fix_versions: string[]
  environment: string[]
  browser: string[]
  dev_project_number: string[]
  found_version: string[]
  occurrence: string[]
  solution: string[]
  critical_phenomenon: string[]
  development_area: string[]
  development_test_assignee_email: string[]
  development_test_result: string[]
  qa_run: string[]
  qa_suite: string[]
  qa_impact: string[]
  deploy_state: string[] // 배포 단계 (merged/dev/qa_preview/qa/prod)
  cs: string[]
  jira_project: string[] // issue_key prefix (DEN / DBO / CRWN ...)
  source_project: string[]
  // 불리언 플래그
  reopened: boolean
  unassigned: boolean
  stale: boolean
  // 기간 (ISO date, inclusive). null = 미설정
  created_from: string | null
  created_to: string | null
  updated_from: string | null
  updated_to: string | null
  // 로컬 텍스트 질의(키/제목/담당자/라벨 즉시 매칭)
  q: string
}

export type GroupBy =
  | 'none'
  | 'status_category'
  | 'status'
  | 'assignee'
  | 'priority'
  | 'severity'
  | 'd1_group'
  | 'product'
  | 'issue_type'
  | 'development_test_result'
  | 'qa_impact'
  | 'source_project'
  | 'epic'
// 'relevance' 는 검색어가 있을 때의 관련도순. 기본 정렬(updated)에서 자동 승격되며
// 명시 선택 시에만 URL 에 직렬화된다(구 URL 은 relevance 를 모르므로 하위호환 유지).
export type SortKey = 'updated' | 'created' | 'priority' | 'reopen_count' | 'relevance'
export type SortDir = 'asc' | 'desc'

/* ── 리스트 컬럼(행에 노출할 후행 필드) ──
 *  레이아웃은 "밀집 행 유지 + 필드 on/off" — 체크한 컬럼만 행 우측에 렌더한다.
 *  컬럼 구성은 display 의 일부라 URL·저장 뷰에 함께 직렬화된다(뷰별 컬럼).
 */
export const COLUMNS = [
  { key: 'assignee', label: '담당자' },
  { key: 'updated', label: '갱신 시간' },
  { key: 'labels', label: '라벨' },
  { key: 'reopen', label: '재오픈' },
  { key: 'stale', label: '정체(경과)' },
  { key: 'qa_impact', label: 'QA 영향' },
  { key: 'deploy', label: '배포 단계' },
  { key: 'severity', label: '심각도' },
  { key: 'issue_type', label: '유형' },
  { key: 'status', label: '상태' },
  { key: 'reporter', label: '보고자' },
  { key: 'comment_count', label: '댓글수' },
  { key: 'fix_versions', label: 'Fix Version' },
  { key: 'components', label: '컴포넌트' },
  { key: 'created', label: '생성 시간' },
  { key: 'environment', label: '환경' },
  { key: 'd1_group', label: '파트' },
  { key: 'dev_test_result', label: '개발검증 결과' },
] as const
export type ColumnKey = (typeof COLUMNS)[number]['key']
const COLUMN_KEYS = COLUMNS.map((c) => c.key) as readonly ColumnKey[]

/** 선택 기능에 딸린 컬럼 — 해당 플래그가 꺼지면 카탈로그에서 사라진다. */
const COLUMN_FEATURE: Partial<Record<ColumnKey, keyof ScryFeatures>> = {
  qa_impact: 'qa',
  deploy: 'deploy',
  d1_group: 'teamGroups',
}

function columnEnabled(key: ColumnKey): boolean {
  const f = COLUMN_FEATURE[key]
  return !f || feature(f)
}

/** 컬럼 메뉴가 노출할 카탈로그(꺼진 기능의 컬럼 제외). */
export function columnCatalog(): (typeof COLUMNS)[number][] {
  return COLUMNS.filter((c) => columnEnabled(c.key))
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

/** 기본 노출 컬럼 — 현재 행 동작과 동일(조건부 배지는 데이터 있을 때만 나타남). */
export function defaultColumns(): ColumnKey[] {
  return DEFAULT_COLUMN_KEYS.filter(columnEnabled)
}

function isColumnKey(v: string): v is ColumnKey {
  return (COLUMN_KEYS as readonly string[]).includes(v)
}

/** 임의 키 목록을 카탈로그 순서로 정규화(유효+활성 키만, 빈 목록 허용 — 전부 끄기 가능). */
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

/* ── 다중값 필드 목록(메타 주도 로직에 재사용) ── */

export const MULTI_FIELDS = [
  'status_category',
  'status',
  'assignee_email',
  'reporter_email',
  'd1_group',
  'labels',
  'priority',
  'severity',
  'issue_type',
  'components',
  'fix_versions',
  'environment',
  'browser',
  'dev_project_number',
  'found_version',
  'occurrence',
  'solution',
  'critical_phenomenon',
  'development_area',
  'development_test_assignee_email',
  'development_test_result',
  'qa_run',
  'qa_suite',
  'qa_impact',
  'deploy_state',
  'cs',
  'jira_project',
  'source_project',
] as const
export type MultiField = (typeof MULTI_FIELDS)[number]

/** 선택 기능에서 오는 필터 필드 — 플래그가 꺼지면 필터 메뉴/URL 양쪽에서 무효. */
const FIELD_FEATURE: Partial<Record<MultiField, keyof ScryFeatures>> = {
  d1_group: 'teamGroups',
  qa_run: 'qa',
  qa_suite: 'qa',
  qa_impact: 'qa',
  deploy_state: 'deploy',
}

export function fieldEnabled(field: MultiField): boolean {
  const f = FIELD_FEATURE[field]
  return !f || feature(f)
}

/** 필터 메뉴가 노출할 필드 목록(꺼진 기능의 필드 제외). */
export function filterFields(): MultiField[] {
  return MULTI_FIELDS.filter(fieldEnabled)
}

/** 선택 기능에서 오는 그룹핑 축. */
const GROUP_FEATURE: Partial<Record<GroupBy, keyof ScryFeatures>> = {
  d1_group: 'teamGroups',
  product: 'teamGroups',
  qa_impact: 'qa',
}

export function groupByEnabled(by: GroupBy): boolean {
  const f = GROUP_FEATURE[by]
  return !f || feature(f)
}

export const FLAG_FIELDS = ['reopened', 'unassigned', 'stale'] as const
export type FlagField = (typeof FLAG_FIELDS)[number]

/* ── 기본값 ── */

export function emptyFilters(): ViewFilters {
  return {
    status_category: [],
    status: [],
    assignee_email: [],
    reporter_email: [],
    d1_group: [],
    labels: [],
    priority: [],
    severity: [],
    issue_type: [],
    components: [],
    fix_versions: [],
    environment: [],
    browser: [],
    dev_project_number: [],
    found_version: [],
    occurrence: [],
    solution: [],
    critical_phenomenon: [],
    development_area: [],
    development_test_assignee_email: [],
    development_test_result: [],
    qa_run: [],
    qa_suite: [],
    qa_impact: [],
    deploy_state: [],
    cs: [],
    jira_project: [],
    source_project: [],
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

/* ── URL 파람 키 매핑(짧은 키로 URL 을 간결하게) ──
 *  선택 이슈(?issue)·활성 뷰(?view) 는 뷰 직렬화에 포함하지 않는다(각각 selection·사이드바 소관).
 */

const MULTI_KEY: Record<MultiField, string> = {
  status_category: 'sc',
  status: 'st',
  assignee_email: 'as',
  reporter_email: 'rp',
  d1_group: 'gr',
  labels: 'lb',
  priority: 'pr',
  severity: 'sv',
  issue_type: 'ty',
  components: 'co',
  fix_versions: 'fx',
  environment: 'en',
  browser: 'br',
  dev_project_number: 'dp',
  found_version: 'vr',
  occurrence: 'oc',
  solution: 'so',
  critical_phenomenon: 'cr',
  development_area: 'da',
  development_test_assignee_email: 'dta',
  development_test_result: 'dtr',
  qa_run: 'qr',
  qa_suite: 'qs',
  qa_impact: 'qi',
  deploy_state: 'ds',
  cs: 'cs',
  jira_project: 'pj',
  source_project: 'spj',
}

const RANGE_KEY = {
  created_from: 'cf',
  created_to: 'ct',
  updated_from: 'uf',
  updated_to: 'ut',
} as const

const FLAG_KEY = 'fl' // 콤마조인된 플래그 목록
const Q_KEY = 'q'
const GROUP_KEY = 'g'
const SORT_KEY = 's'
const DIR_KEY = 'd'
const COLS_KEY = 'cl' // 콤마조인된 컬럼 목록. 전부 끔 = 'none'
const COLS_NONE = 'none'

/** 뷰 직렬화에 관여하는 모든 파람 키(안정적인 viewKey 계산에 사용, 순서 고정). */
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

/* ── 파싱: URLSearchParams → ViewConfig ── */

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
    // 꺼진 기능의 필드는 URL 로도 켜지지 않는다(공유 링크가 죽은 필터를 되살리지 않게).
    f[field] = fieldEnabled(field) ? splitList(params.get(MULTI_KEY[field])) : []
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
  // 빈 문자열(cl=)은 "미설정"으로 보고 기본값 유지 — #viewKey 재직렬화가 미설정 키도
  // `cl=` 로 항상 붙이기 때문. 전부 끄기는 명시적 sentinel 'none' 으로만.
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
      'd1_group',
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
  return ['updated', 'created', 'priority', 'reopen_count', 'relevance'].includes(v)
}

/* ── 직렬화: ViewConfig → URL 파람 델타 ──
 *  값이 비면 null 을 넣어 setParams 가 해당 키를 제거하도록 한다(URL 청결 유지).
 */

export function configToParams(config: ViewConfig): Record<string, string | null> {
  const { filters: f, display: d } = config
  const out: Record<string, string | null> = {}

  for (const field of MULTI_FIELDS) {
    const arr = f[field]
    out[MULTI_KEY[field]] = arr.length ? arr.join(',') : null
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

  // 진행 단계가 기본값이다. 섹션 없음을 선택한 경우에는 g=none 을 명시해 보존한다.
  out[GROUP_KEY] = d.group_by !== 'status_category' ? d.group_by : null
  out[SORT_KEY] = d.sort !== 'updated' ? d.sort : null
  out[DIR_KEY] = d.dir !== 'desc' ? d.dir : null

  // 컬럼: 기본과 같으면 생략(URL 청결), 전부 끄면 'none' 으로 보존.
  const def = defaultColumns()
  const colsEqDefault =
    d.columns.length === def.length && d.columns.every((c, i) => c === def[i])
  out[COLS_KEY] = colsEqDefault ? null : d.columns.length ? d.columns.join(',') : COLS_NONE

  return out
}

/* ── 필터 적용 판정 ── */

/**
 * status_category 를 못 주는 응답용 폴백. 사이트/계정 언어마다 상태 이름이 달라
 * 신뢰할 수 없으므로 어느 Jira 에서나 같은 뜻인 일반 항목만 남긴다.
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

/** 서버가 준 status_category 를 1순위로 신뢰하고, 없을 때만 상태 이름으로 추정한다. */
export function effectiveCategory(issue: IssueLite): StatusCategory {
  const sc = (issue.status_category ?? '').toLowerCase()
  if (sc === 'new' || sc === 'inprogress' || sc === 'done') return sc
  if (RESOLVED_STATUS_NAMES.has((issue.status ?? '').trim().toLowerCase())) return 'done'
  return 'inprogress'
}

/**
 * 이슈의 배포 단계. deploy_status 미전송(구 서버)·빈 객체는 'none' 으로 정규화.
 *  (백엔드 사전계산과 동일 의미 — 프론트는 파생만 한다.)
 */
export function deployStateOf(issue: IssueLite): DeployState {
  return issue.deploy_status?.state ?? 'none'
}

/** 배포 단계 한국어 라벨(필터 파셋/칩 공용). */
export const DEPLOY_STATE_LABEL: Record<DeployState, string> = {
  none: '릴리즈 미포함',
  merged: '머지됨',
  dev: 'dev 릴리즈',
  qa_preview: 'QA 대기(스왑 전)',
  qa: 'QA 확인 가능',
  prod: 'prod 배포',
}

/**
 * 현재 상태로 들어온 뒤 경과 시간(h). status_changed_at 이 기준이고, 없으면 updated_at
 * 으로 대체한다. 둘 다 없으면 0 — 판정 근거가 없으므로 정체로 몰지 않는다.
 */
export function statusAgeHours(issue: IssueLite): number {
  const iso = issue.status_changed_at ?? issue.updated_at
  if (!iso) return 0
  const t = Date.parse(iso)
  return Number.isFinite(t) ? Math.max(0, (Date.now() - t) / 3_600_000) : 0
}

/** 정체 판정: 미완료 + 현재 상태 경과가 임계(config.staleThresholdHours) 초과. */
export function isStale(issue: IssueLite): boolean {
  if (effectiveCategory(issue) === 'done') return false
  return statusAgeHours(issue) > config().staleThresholdHours
}

/** 활성 필터가 하나라도 있는지(뷰 저장 버튼 노출용). q 제외 여부는 호출부에서 판단. */
export function hasAnyFilter(f: ViewFilters): boolean {
  for (const field of MULTI_FIELDS) if (f[field].length) return true
  if (f.reopened || f.unassigned || f.stale) return true
  if (f.created_from || f.created_to || f.updated_from || f.updated_to) return true
  if (f.q.trim()) return true
  return false
}
