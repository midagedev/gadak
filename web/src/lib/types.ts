/*
 * Issue Navigator — API 타입 (계약 §1 의 TypeScript 표현)
 *
 * 원칙: 필드명은 서버 응답 그대로 snake_case 를 유지한다.
 *  변환 레이어를 두지 않아야(성능) 대량 이슈를 그대로 메모리 풀/IndexedDB 에 넣을 수 있다.
 */

/** 이슈 상태의 유효 분류. 서버 `status_category` 는 Jira 원본이며, UI 는 이 3분류로 색을 입힌다. */
export type StatusCategory = 'new' | 'inprogress' | 'done'

export type QaImpactState = 'blocking' | 'retest' | 'verified' | 'linked' | ''

/**
 * 이슈별 배포 단계(사전계산). 단계 진행: merged → dev(릴리즈) →
 *  qa_preview(qa 릴리즈, 스왑 전) → qa(스왑 완료=QA 확인 가능) → prod.
 *  none = 연결 PR 자체가 없음.
 */
export type DeployState = 'none' | 'merged' | 'dev' | 'qa_preview' | 'qa' | 'prod'

/** 배포에 이슈 픽스를 담은 릴리즈 참조(태그 + 시각). */
export interface DeployReleaseRef {
  tag: string
  at: string
}

/** IssueLite 에 실리는 경량 배포 현황(사전계산). */
export interface DeployStatus {
  state: DeployState
  merged_prs: number
  total_prs: number
  dev: DeployReleaseRef | null
  qa_release: DeployReleaseRef | null
  qa_swapped_at: string | null
  prod_at: string | null
}

export interface QaRef {
  key: string
  label: string
}

export interface QaSuiteRef extends QaRef {
  path: string
}

/**
 * 리스트/필터/검색이 읽는 경량 이슈. bootstrap/delta 가 이 형태의 배열을 준다.
 * (본문/코멘트/히스토리는 제외 — 상세는 온디맨드 detail 로 가져온다.)
 */
export interface IssueLite {
  issue_key: string
  summary: string
  status: string
  status_category: string // Jira 원본 카테고리 문자열 (분류는 StatusCategory 로 별도 계산)
  issue_type: string
  priority: string | null
  priority_rank: number | null
  severity: string | null

  assignee: string | null
  assignee_email: string | null
  reporter: string | null
  reporter_email: string | null

  labels: string[]
  fix_versions: string[]
  components: string[]

  d1_group: string | null
  epic_key: string | null
  source_project: string | null

  created_at: string | null // ISO8601
  updated_at: string | null
  resolved_at: string | null
  status_changed_at: string | null

  working_hours_in_status: number | null
  reopen_count: number
  reopened_at: string | null
  reopen_reason: string | null

  comment_count: number
  dev_project_number: string | null
  related_project_number: string | null

  environment: string | null
  browser: string | null
  found_version: string | null
  occurrence: string | null
  solution: string | null
  critical_phenomenon: string | null
  development_area: string | null
  cs: string | null
  development_test_assignee: string | null
  development_test_assignee_email: string | null
  development_test_result: string | null

  qa_impact_state: QaImpactState
  qa_impact_label: string
  qa_runs: QaRef[]
  qa_suites: QaSuiteRef[]

  /** 배포 단계(사전계산). 구 서버는 미전송(undefined), 기본값으로 빈 객체가 올 수도 있음. */
  deploy_status?: DeployStatus
}

/** 팀 멤버 (이름/아바타/파트). bootstrap 에 동봉. */
export interface Member {
  email: string
  name: string
  display_name: string | null
  profile_image: string | null
  department: string | null
  job_role: string | null
  group: string | null
  status: string | null
  /** Jira accountId — 담당자 지정에 사용(서버 호출 없이 로컬 지정). 백엔드가 채워준다. */
  jira_account_id?: string | null
}

export type SyncSourceStatus =
  | 'healthy'
  | 'running'
  | 'paused'
  | 'idle'
  | 'stale'
  | 'failed'
  | 'missing'

export interface SyncSourceHealth {
  key: 'jira' | 'qase' | 'members'
  label: string
  status: SyncSourceStatus
  synced_at: string | null
  message: string
}

export interface SyncHealth {
  overall: 'healthy' | 'warning' | 'failed'
  checked_at: string
  sources: SyncSourceHealth[]
}

/* ── ADF (Atlassian Document Format) ──
 * detail 의 description_adf / comment.raw_body 원본. 렌더는 [detail] 의 adf.ts 소관.
 * 여기서는 재귀 노드 형태만 최소로 선언한다.
 */
export interface AdfNode {
  type: string
  text?: string
  content?: AdfNode[]
  attrs?: Record<string, unknown>
  marks?: Array<{ type: string; attrs?: Record<string, unknown> }>
  [key: string]: unknown
}

/** 상세 패널의 코멘트 1건. */
export interface DetailComment {
  comment_id: string
  author: string | null
  author_email: string | null
  author_account_id?: string | null // 답글(멘션) 대상 지정용
  body: string // plain text
  raw_body: AdfNode | null // ADF 원본
  created_at: string | null
}

/** 상태/담당자/우선순위 변경 이력 1건 (시간순). */
export interface HistoryEntry {
  at: string | null
  field: string
  from: string | null
  to: string | null
  by: string | null
}

/** 연결된 이슈 1건. */
export interface LinkedIssue {
  key: string
  type: string
  direction: string
  summary: string | null
}

/** 연결된 PR 1건 (PrSnapshot). */
export interface LinkedPr {
  number: number
  title: string
  url: string
  state: string
  repo: string | null
  author: string | null
}

/** Jira 원본 첨부 + private S3 재생 캐시 메타데이터. */
export interface DetailAttachment {
  id: string
  filename: string
  mime_type: string
  size: number
  media_id: string
  media_collection: string
  is_image: boolean
  is_video: boolean
  cache_status: 'pending' | 'caching' | 'ready' | 'failed'
  created_at: string | null
  /** same-origin redacted-tool URL. 최초 요청 시 S3 캐시 후 presigned URL로 리다이렉트. */
  content_url: string
}

export interface QaLinkedCase {
  qase_case_id: number
  case_id: string
  title: string
  status: string
  result_time: string | null
  suite: QaSuiteRef
}

export interface QaRunContext {
  key: string
  qase_run_id: number
  title: string
  product_code: string
  product_label: string
  url: string
  completion: number
  executed: number
  total: number
  state: Exclude<QaImpactState, ''>
  state_label: string
  linked_case_count: number
  status_counts: Record<string, number>
  suites: QaSuiteRef[]
  cases: QaLinkedCase[]
}

export interface QaIssueContext {
  state: Exclude<QaImpactState, ''>
  state_label: string
  runs: QaRunContext[]
  suites: QaSuiteRef[]
}

/** 상세 배포 근거 — 포함 릴리즈 1건(태그 + 링크 + 시각 + 채널). 방어적 파싱(모두 optional). */
export interface DeployReleaseEvidence {
  tag: string
  html_url?: string | null
  at?: string | null
  /** 릴리즈 채널 힌트(dev/qa/prod 등). 서버 구현에 따라 없을 수 있음. */
  channel?: string | null
}

/** 상세 배포 근거 — PR별 포함 여부 1건. */
export interface DeployPrInclusion {
  number: number
  title?: string | null
  url?: string | null
  repo?: string | null
  merged?: boolean
  /** 어느 릴리즈에 포함되었는지(태그). 미포함이면 null/미전송. */
  included_in?: string | null
}

/**
 * detail 응답의 배포 상세 — 경량 DeployStatus 전체 + 근거(포함 릴리즈/PR별 포함/최근 스왑).
 * 구 서버 호환을 위해 전 필드 optional. state 없으면 섹션을 렌더하지 않는다.
 */
export interface DeployDetail extends Partial<DeployStatus> {
  releases?: DeployReleaseEvidence[]
  prs?: DeployPrInclusion[]
}

/** GET `<issue_key>/detail/` 응답. */
export interface DetailResponse {
  issue_key: string
  development_opinion: string
  description_adf: AdfNode | null
  attachments: DetailAttachment[]
  comments: DetailComment[]
  history: HistoryEntry[]
  linked_issues: LinkedIssue[]
  linked_prs: LinkedPr[]
  qa_context: QaIssueContext | null
  /** 배포 현황 상세. 구 서버는 미전송 → 상세 섹션 숨김. */
  deploy?: DeployDetail
}

/** GET `bootstrap/` 응답. */
export interface BootstrapResponse {
  server_time: string
  sync_version: number
  members: Member[]
  /** 멤버 셋 안정 해시. 이후 delta 의 mv 로 되돌려주면 변경 없을 때 members 전송이 생략된다. */
  members_version?: string // 구 서버는 미전송
  issues: IssueLite[]
  sync_health: SyncHealth
}

/** GET `delta/?since=&mv=` 응답. */
export interface DeltaResponse {
  server_time: string
  upserted: IssueLite[]
  deleted_keys: string[]
  /** mv 가 서버 해시와 같으면 서버가 생략한다 → 이 경우 기존 멤버를 유지한다. */
  members?: Member[]
  members_version?: string // 구 서버는 미전송
  sync_health: SyncHealth
}

/** GET `search/?q=` 응답. */
export interface SearchResponse {
  keys: string[]
  total: number
}

/** GET `mentions/?email=` 의 항목. */
export interface Mention {
  issue_key: string
  comment_id: string
  author: string | null
  body_excerpt: string // <=200자
  created_at: string | null
}

export interface MentionsResponse {
  mentions: Mention[]
}

export type FeedFocus = 'all' | 'assignee' | 'reporter' | 'mention'

export type FeedEventType =
  | 'created'
  | 'status_changed'
  | 'reopened'
  | 'assigned'
  | 'comment_added'
  | 'attachment_added'
  | 'fields_changed'

export interface FeedItem {
  id: number
  event_id: string
  issue_key: string
  summary: string
  current_status: string
  event_type: FeedEventType
  occurred_at: string | null
  actor_name: string
  payload: Record<string, unknown>
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

export interface NotificationPreferences {
  notify_mentions: boolean
  notify_assigned: boolean
  notify_watched: boolean
  show_preview: boolean
  // 조용 시간 (KST 기준 "HH:MM"). null 이면 미사용.
  quiet_start: string | null
  quiet_end: string | null
}

export interface NotificationConfig {
  enabled: boolean
  vapid_public_key: string
  preferences: NotificationPreferences
}

/**
 * 저장된 뷰 (팀 공유). config 는 서버가 해석하지 않는 불투명 JSON —
 * 프론트 뷰 상태(필터/디스플레이) 직렬화이며 구조는 [explore] 가 정의한다.
 */
export interface SavedView {
  id: string
  name: string
  owner_email: string | null
  owner_name: string | null
  config: Record<string, unknown>
  created_at: string | null
  updated_at: string | null
}

export interface ViewsResponse {
  views: SavedView[]
}

/** 워치 목록 응답. */
export interface WatchesResponse {
  keys: string[]
}

/* ── 쓰기(Write) API 타입 (계약: 쓰기 프록시) ────────────────────────────── */

/** GET/PUT/DELETE credential/ 응답 — 개인 Jira API 토큰 설정 상태. 평문 토큰은 미노출. */
export interface JiraCredential {
  configured: boolean
  jira_email: string
  display_name: string
  verified_at: string | null
  token_hint: string
}

/** 상태 전환 후보 1건 (GET <key>/transitions/). */
export interface Transition {
  id: string
  name: string
  to_status: string
  to_category: string // Jira statusCategory key (new/indeterminate/done)
}

export interface TransitionsResponse {
  transitions: Transition[]
}

/** 담당자 후보 사용자 1건 (GET users/?q=). */
export interface JiraUser {
  account_id: string
  display_name: string
  email: string
  avatar_url: string
  active: boolean
}

export interface UsersResponse {
  users: JiraUser[]
}

/* ── QA 필드 인라인 편집 (editmeta) ── */

/** 단일 옵션/버전 선택지 (id + 표시값). */
export interface EditMetaOption {
  id: string
  value: string
}

/** 편집 가능한 한 필드의 메타 — 종류 + 편집가능여부 + 선택지. */
export interface EditMetaField {
  /** option(단일 select) / user(userpicker) / version_array(버전 배열). */
  kind: 'option' | 'user' | 'version_array'
  editable: boolean
  /** user 필드는 비어 있음(사용자 검색으로 대체). */
  options: EditMetaOption[]
}

/** GET <key>/editmeta/ — 프론트 키별 편집 메타. 편집 불가 필드는 생략. */
export interface EditMetaResponse {
  fields: Partial<Record<string, EditMetaField>>
}

/** 이슈 타입 (create-meta 항목). */
export interface CreateMetaIssueType {
  id: string
  name: string
}

/** 생성 가능한 프로젝트 1건 (GET create-meta/). */
export interface CreateMetaProject {
  key: string
  name: string
  issue_types: CreateMetaIssueType[]
}

export interface CreateMetaResponse {
  projects: CreateMetaProject[]
}

/** POST <key>/comment/ 가 돌려주는 새 코멘트 (raw_body 없음 — 평문 body). */
export interface CreatedComment {
  comment_id: string
  author: string | null
  body: string
  created_at: string | null
}

/** 쓰기 응답 공통 — 최신 IssueLite. */
export interface IssueWriteResponse {
  issue: IssueLite
}

export interface CommentWriteResponse {
  issue: IssueLite
  comment: CreatedComment
}

/** 코멘트에 삽입된 멘션 1건 (프론트가 자동완성으로 확정한 account_id + 표시이름). */
export interface CommentMention {
  account_id: string
  display_name: string
}

/** 업로드된 첨부 1건 (POST <key>/attachments/ 응답). 코멘트 인라인 임베드에 사용. */
export interface UploadedAttachment {
  id: string
  filename: string
  mime_type: string
  size: number
  media_id: string
  is_image: boolean
  is_video: boolean
  content_url: string
}

export interface AttachmentUploadResponse {
  attachments: UploadedAttachment[]
}

/**
 * GET meta/write/ 응답 (익명 읽기) — 쓰기에 필요한 정적 메타를 통째로 선반영.
 *  - transitions: project → 현재 status → 가능한 전환 목록 (0ms 드롭다운용).
 *  - create_meta: 생성 가능한 프로젝트/이슈타입 (새 이슈 다이얼로그용).
 * 부팅 시 로드 + IndexedDB 캐시 + 15분 주기 재로드.
 */
export interface WriteMeta {
  transitions: Record<string, Record<string, Transition[]>>
  create_meta: { projects: CreateMetaProject[] }
  updated_at: string | null
}

/** IndexedDB meta 스토어의 write 메타 레코드. */
export interface WriteMetaCache {
  key: 'write'
  transitions: Record<string, Record<string, Transition[]>>
  projects: CreateMetaProject[]
  updated_at: string | null
  cached_at: string
}

/** POST create/ 요청 바디. */
export interface CreateIssuePayload {
  project_key: string
  issue_type: string // create-meta 의 issue_type id
  summary: string
  description_text?: string
  assignee_account_id?: string | null
  priority?: string
  labels?: string[]
}

/** IndexedDB meta 스토어에 저장하는 캐시 메타. */
export interface CacheMeta {
  key: 'sync' // 단일 레코드 키
  server_time: string
  sync_version: number
  members: Member[]
  members_version?: string // 멤버 셋 해시. 다음 delta 의 mv 로 사용 (구 캐시엔 없음)
  sync_health?: SyncHealth
}
