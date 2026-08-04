/*
 * Issue Navigator — 필터/디스플레이/파생 리스트 스토어 ([explore], 계약 §2)
 *
 * 설계: **URL 해시가 뷰의 단일 진실원**이다.
 *  - filters/display 는 router.params 에서 파생(parseConfig) → "뷰 = URL" 이 자연스럽게 성립.
 *  - 모든 변경(칩 추가/제거·그룹핑·정렬·검색어)은 setParams(replace) 로 URL 을 갱신 → 파생 재계산.
 *  - 선택 이슈(?issue) 변경이 필터 재계산을 유발하지 않도록, 뷰 관련 파람만 추린
 *    안정 문자열(#viewKey)을 중간 파생으로 두어 리필터를 차단한다(성능 규율).
 *
 * 모든 파생(visibleIssues/groups/facets)은 로컬 연산 — 서버 왕복 없음.
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

/* ── 파생 그룹 타입 ── */

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

/** FilterBar 가 렌더하는 활성 필터 칩 1개. */
export interface ActiveChip {
  /** 어떤 필드/플래그/기간인지. */
  kind: 'multi' | 'flag' | 'range'
  field: string
  value?: string // multi 값
  label: string // 표시 문자열
}

/* ── facet(추가 드롭다운용 값 분포) ── */

export interface FacetValue {
  value: string
  label: string
  count: number
}

class FiltersStore {
  /* URL → 뷰 관련 파람만 추린 안정 문자열. 선택 이슈 변경엔 불변 → 리필터 차단. */
  #viewKey = $derived(VIEW_PARAM_KEYS.map((k) => `${k}=${router.params.get(k) ?? ''}`).join('&'))

  /* #viewKey 가 실제로 바뀔 때만 재파싱되는 config. */
  #config = $derived(parseFromKey(this.#viewKey))

  get filters(): ViewFilters {
    return this.#config.filters
  }
  get display(): ViewDisplayRef {
    return this.#config.display
  }

  /**
   * 실제 적용되는 정렬. 검색어가 있고 정렬이 기본값(updated)이면 관련도순으로 자동 승격한다.
   *  (사용자가 명시적으로 다른 정렬을 고르면 그것을 우선 — 단, updated 는 "미선택"과
   *   구분되지 않으므로 검색 중엔 관련도로 본다. 정렬 UI 표시도 이 값을 따른다.)
   */
  get effectiveSort(): SortKey {
    const { filters: f, display: d } = this.#config
    if (f.q.trim() && d.sort === 'updated') return 'relevance'
    return d.sort
  }

  /** 뷰(필터+디스플레이) 관련 파람만 담은 안정 문자열. 데이터 변경엔 불변 →
   *  리스트가 "뷰가 실제로 바뀐 경우"에만 스크롤/커서를 리셋하는 신호로 쓴다. */
  get viewKey(): string {
    return this.#viewKey
  }

  /* ── 서버 전문검색 결과(휘발성, 뷰 직렬화 대상 아님) ── */
  serverMatchKeys = $state<string[]>([])
  serverMatchQuery = $state('')
  searching = $state(false)
  /** Last query that failed body search (UI can offer Retry). */
  searchError = $state<string | null>(null)

  /* ── 필터 적용 결과(그룹 무관 평면, 정렬 반영) ── */
  visibleIssues = $derived.by(() => {
    const f = this.#config.filters
    const list = filterIssues(issues.allIssues, f)
    const sort = this.effectiveSort
    // 관련도순은 검색어·최근성·개인화를 함께 봐야 하므로 컨텍스트를 넘긴다.
    const ctx: RelevanceContext | undefined =
      sort === 'relevance' ? buildRelevanceContext(f.q) : undefined
    return sortIssues(list, sort, this.#config.display.dir, ctx)
  })

  /* ── 그룹핑 결과(디스플레이 group_by 기준). group_by=none 이면 단일 그룹. ── */
  groups = $derived.by(() => buildGroups(this.visibleIssues, this.#config.display.group_by))

  /* ── FilterBar 활성 칩 ── */
  activeChips = $derived.by(() =>
    buildChips(this.#config.filters, issues.members, issues.allIssues),
  )

  hasFilters = $derived(hasAnyFilter(this.#config.filters))

  /**
   * 현재 뷰가 이미 "내 이슈"로 스코프됐는지(담당 또는 보고 필터에 내 이메일 포함).
   *  이때는 리스트 전체가 내 이슈라 개별 하이라이팅이 오히려 노이즈이므로 끈다.
   */
  scopedToMe = $derived.by(() => {
    const e = me.email?.toLowerCase()
    if (!e) return false
    const f = this.#config.filters
    const has = (arr: string[]) => arr.some((x) => x.toLowerCase() === e)
    return has(f.assignee_email) || has(f.reporter_email)
  })

  /** 이 이슈가 내 담당인지(하이라이팅 판정). 대소문자 무시. */
  isMine(issue: IssueLite): boolean {
    const e = me.email
    return (
      !!e &&
      !!issue.assignee_email &&
      issue.assignee_email.toLowerCase() === e.toLowerCase()
    )
  }

  /** facet: 현재 전체 풀 기준 값 분포(추가 드롭다운). member 이름 라벨 포함. */
  facets = $derived.by(() => buildFacets(issues.allIssues, issues.members))

  /* ── 변경 연산: 전부 URL 갱신(replace, history 미적재) ── */

  #apply(config: ViewConfig): void {
    setParams(configToParams(config), true)
  }

  /** 현재 config 의 독립 사본(배열까지 분리). 변경 연산의 시작점. */
  private snapshot(): ViewConfig {
    return mergeConfig(this.#config)
  }

  /** 다중값 토글(있으면 제거, 없으면 추가). */
  toggleValue(field: MultiField, value: string): void {
    const c = this.snapshot()
    const arr = c.filters[field]
    c.filters[field] = arr.includes(value) ? arr.filter((v) => v !== value) : [...arr, value]
    this.#apply(c)
  }

  /** 다중값 추가(칩 클릭=필터 추가; 중복은 무시). */
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
    // 로컬 질의가 바뀌면 이전 서버검색 결과는 무효
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

  /** 리스트 컬럼 on/off 토글(카탈로그 순서 유지, 전부 끄기 허용). */
  toggleColumn(key: ColumnKey): void {
    const c = this.snapshot()
    const cur = c.display.columns
    const next = cur.includes(key) ? cur.filter((k) => k !== key) : [...cur, key]
    c.display.columns = orderColumns(next)
    this.#apply(c)
  }

  /** 컬럼 구성을 기본값으로 되돌림. */
  resetColumns(): void {
    const c = this.snapshot()
    c.display.columns = defaultColumns()
    this.#apply(c)
  }

  /** 저장/기본 뷰를 통째 적용. */
  applyConfig(config: ViewConfig): void {
    this.clearServerSearch()
    this.#apply(mergeConfig(config))
  }

  /** 현재 뷰 config(저장용). */
  currentConfig(): ViewConfig {
    return mergeConfig(this.#config)
  }

  /** 전체 필터/디스플레이 초기화(선택 이슈는 보존). */
  clearAll(): void {
    this.clearServerSearch()
    this.#apply(emptyConfig())
  }

  /* ── 서버 전문검색 ── */

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

  /** 로컬 결과에 없지만 본문 매칭으로 잡힌 이슈(추가 섹션 렌더). */
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

/* display 타입 별칭(가독성). */
type ViewDisplayRef = ViewConfig['display']

/* ── 순수 헬퍼 ── */

function parseFromKey(viewKey: string): ViewConfig {
  // viewKey 는 뷰 파람만 담은 "k=v&k=v" 문자열 → URLSearchParams 로 복원해 파싱.
  return parseConfig(new URLSearchParams(viewKey))
}

/** config 를 완전한 형태로 정규화(부분 config 방어) + 배열 참조 분리. */
function mergeConfig(c: ViewConfig): ViewConfig {
  const base = emptyConfig()
  Object.assign(base.filters, c.filters)
  Object.assign(base.display, c.display)
  // 배열 참조 분리. 컬럼은 저장 뷰가 꺼진 기능의 컬럼을 들고 있을 수 있어 정규화한다.
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

/* ── 초성 캐시 ──
 *  제목+담당자의 초성열은 이슈당 1회만 계산하고, updated_at 을 시그니처로 캐시한다.
 *  delta 로 이슈가 갱신되면 시그니처가 바뀌어 자동 무효화된다(10.5K 대상 재계산 방지).
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

/** 로컬 텍스트 매칭(키/제목/담당자/라벨). chosungQuery 면 제목·담당자 초성열도 부분 매칭. */
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
  // 이슈 키 단축형(den1234) 대응. 자모 쿼리(ㅋㅌㅂ)는 normKey 가 '' 이 돼
  // includes('') 전체 매칭이 되므로 빈 결과는 건너뛴다.
  const nk = normKey(needle)
  if (nk && normKey(issue.issue_key).includes(nk)) return true
  // 초성 쿼리("ㅋㅌㅂ")는 제목·담당자 초성열에 부분 매칭
  if (chosungQuery && issueChosung(issue).includes(needle)) return true
  return false
}

function inRange(iso: string | null, from: string | null, to: string | null): boolean {
  if (!iso) return !from // 값 없으면 하한 없을 때만 통과
  const d = iso.slice(0, 10)
  if (from && d < from) return false
  if (to && d > to) return false
  return true
}

export function filterIssues(all: IssueLite[], f: ViewFilters): IssueLite[] {
  const raw = f.q.trim()
  const needle = raw.toLowerCase()
  // 초성 판정은 쿼리당 1회만(이슈 루프 밖).
  const chosungQuery = raw ? isChosungQuery(raw) : false
  const out: IssueLite[] = []
  for (const it of all) {
    if (f.status_category.length && !f.status_category.includes(effectiveCategory(it))) continue
    if (f.status.length && !f.status.includes(it.status)) continue
    if (f.assignee_email.length && !(it.assignee_email && f.assignee_email.includes(it.assignee_email)))
      continue
    if (f.reporter_email.length && !(it.reporter_email && f.reporter_email.includes(it.reporter_email)))
      continue
    if (f.d1_group.length && !(it.d1_group && f.d1_group.includes(it.d1_group))) continue
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

/* ── 관련도 랭킹 ──
 *  검색어(needle) 기준 매칭 강도 + 최근성 + 개인화 보너스로 점수를 매긴다.
 *  needle 은 이미 trim·소문자화된 쿼리. chosungQuery 면 초성열도 후보.
 */
export interface RelevanceContext {
  needle: string
  chosungQuery: boolean
  now: number
  myEmail: string | null
  recentKeys: Set<string>
}

/** 관련도 계산에 필요한 컨텍스트 스냅샷(개인화는 me 스토어에서). */
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

  // 매칭 강도(가장 강한 위치 하나만 base 로) — 키 > 제목 > 초성 > 담당자·라벨.
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

  // 최근성 보너스(갱신 기준).
  let score = base
  if (issue.updated_at) {
    const age = ctx.now - Date.parse(issue.updated_at)
    if (age >= 0) {
      if (age <= 7 * DAY_MS) score += 30
      else if (age <= 30 * DAY_MS) score += 10
    }
  }

  // 개인화 보너스: 내 담당 / 최근 본 이슈.
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
    // 관련도는 방향 무관(항상 높은 점수 우선). 점수 캐시로 재계산 방지, 동점은 최신 갱신순.
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
        // priority_rank: 작을수록 높은 우선순위. null 은 항상 뒤로.
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

/* ── 그룹핑 ── */

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
    case 'd1_group':
      return issue.d1_group
        ? { key: issue.d1_group, label: issue.d1_group }
        : { key: '', label: t('common.unclassified') }
    case 'product':
      // 그룹→제품 매핑은 조직마다 다르므로 런타임 config 에서 읽는다.
      return config().productByGroup[issue.d1_group ?? ''] ?? { key: '', label: t('group.noProduct') }
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
  // 그룹 정렬: 빈 키는 뒤로, 진행 단계/우선순위/심각도는 업무 순서, 나머지는 이름순.
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

/* ── 활성 칩 ── */

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
    if (it.d1_group) bump(counters.d1_group, it.d1_group)
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
      // 배포 단계: 'none'(릴리즈 미포함)은 노이즈라 파셋에서 제외한다.
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
