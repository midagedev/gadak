/*
 * Issue Navigator — REST client (contract §1)
 *
 * base comes from runtime config(); dev proxies via Vite to the local server.
 * No session token: identity is the Jira credential stored by the server.
 * Reads work without a credential on loopback; writes need one configured.
 */

import { config } from './config'
import type { ScryFeatures } from './config'
import type {
  AttachmentUploadResponse,
  BootstrapResponse,
  CommentMention,
  CommentWriteResponse,
  CreateIssuePayload,
  CreateMetaResponse,
  DeltaResponse,
  DetailResponse,
  EditMetaResponse,
  FeedFocus,
  FeedResponse,
  FeedUnreadCounts,
  IssueWriteResponse,
  JiraCredential,
  MentionsResponse,
  NotificationConfig,
  NotificationPreferences,
  SavedView,
  SearchResponse,
  TransitionsResponse,
  UsersResponse,
  ViewsResponse,
  WatchesResponse,
  WriteMeta,
} from './types'

// Runtime config (built artifacts stay tenant-neutral).

/**
 * Lets callers distinguish credential-missing (409) and similar failures.
 * `code` is the server body `error` string (e.g. 'credential_required');
 * `jiraErrors` carries raw Jira field errors when present.
 */
export class ApiError extends Error {
  status: number
  code: string | null
  jiraErrors: Record<string, unknown> | null
  constructor(
    status: number,
    message: string,
    code: string | null = null,
    jiraErrors: Record<string, unknown> | null = null,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.jiraErrors = jiraErrors
  }
}

/** Shared fetch — path is relative to the API base. */
async function raw(path: string, init?: RequestInit): Promise<Response> {
  return fetch(config().apiBase + path, {
    credentials: 'same-origin',
    ...init,
  })
}

/** JSON 응답을 파싱하고 4xx/5xx 는 ApiError 로 던진다. */
async function json<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await raw(path, init)
  if (!res.ok) {
    throw new ApiError(res.status, `${init?.method ?? 'GET'} ${path} → ${res.status}`)
  }
  return (await res.json()) as T
}

/* ── bootstrap (ETag / 304 지원) ── */

export type BootstrapResult =
  | { status: 'ok'; data: BootstrapResponse; etag: string | null }
  | { status: 'not_modified' }

/**
 * 전체 이슈 + 멤버 로드. `etag`(직전 If-None-Match) 를 넘기면 304 시 `not_modified` 를 준다.
 * ETag 는 `"in-<sync_version>"` 형태(서버). 다음 호출에 그대로 되돌려주면 된다.
 */
export async function getBootstrap(etag?: string | null): Promise<BootstrapResult> {
  const headers = new Headers()
  if (etag) headers.set('If-None-Match', etag)
  const res = await raw('bootstrap/', { headers })
  if (res.status === 304) return { status: 'not_modified' }
  if (!res.ok) throw new ApiError(res.status, `GET bootstrap/ → ${res.status}`)
  return {
    status: 'ok',
    data: (await res.json()) as BootstrapResponse,
    etag: res.headers.get('ETag'),
  }
}

/* ── delta ── */

/**
 * since 이후 변경분. `membersVersion`(직전 members_version)을 넘기면 서버 해시와 같을 때
 * 응답에서 members 를 생략한다(페이로드 다이어트). 없으면 서버가 members 를 항상 포함.
 */
export function getDelta(since: string, membersVersion?: string): Promise<DeltaResponse> {
  let path = `delta/?since=${encodeURIComponent(since)}`
  if (membersVersion) path += `&mv=${encodeURIComponent(membersVersion)}`
  return json<DeltaResponse>(path)
}

/* ── detail (온디맨드) ── */

export function getDetail(issueKey: string): Promise<DetailResponse> {
  return json<DetailResponse>(`${encodeURIComponent(issueKey)}/detail/`)
}

/* ── 전문 검색 (서버) ── */

export function search(q: string, limit = 200): Promise<SearchResponse> {
  return json<SearchResponse>(`search/?q=${encodeURIComponent(q)}&limit=${limit}`)
}

/* ── 멘션 ── */

export function getMentions(email: string): Promise<MentionsResponse> {
  return json<MentionsResponse>(`mentions/?email=${encodeURIComponent(email)}`)
}

/* ── 프레즌스 티켓 ──
 * WS 접속 전에 단발성 티켓을 발급받는다. 응답은 redacted-tool 표준 api_response 래퍼
 *  ({ data: { ticket } })거나 평문({ ticket })일 수 있어 둘 다 흡수한다.
 */

export async function getPresenceTicket(): Promise<string> {
  const res = await json<{ data?: { ticket?: string }; ticket?: string }>('presence-ticket/')
  const ticket = res.data?.ticket ?? res.ticket
  if (!ticket) throw new ApiError(0, 'presence-ticket: 응답에 ticket 없음')
  return ticket
}

/* ── 개인 피드 / 읽음 ── */

export function getFeed(focus: FeedFocus = 'all', limit = 80): Promise<FeedResponse> {
  return json<FeedResponse>(`feed/?focus=${encodeURIComponent(focus)}&limit=${limit}`)
}

export function markFeedRead(payload: {
  event_ids?: string[]
  issue_keys?: string[]
  all?: boolean
}): Promise<{ updated: number; unread_counts: FeedUnreadCounts }> {
  return json('feed/read/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

/* ── Web Push ── */

export function getNotificationConfig(): Promise<NotificationConfig> {
  return json<NotificationConfig>('notifications/config/')
}

export function updateNotificationPreferences(
  preferences: Partial<NotificationPreferences>,
): Promise<NotificationConfig> {
  return json<NotificationConfig>('notifications/config/', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(preferences),
  })
}

export interface PushSubscriptionPayload {
  endpoint: string
  keys: { p256dh: string; auth: string }
}

export function savePushSubscription(
  subscription: PushSubscriptionPayload,
): Promise<{ subscribed: true; id: string }> {
  return json('notifications/subscription/', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(subscription),
  })
}

export function deletePushSubscription(
  endpoint: string,
): Promise<{ subscribed: false }> {
  return json('notifications/subscription/', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint }),
  })
}

/* ── 저장 뷰 ── */

export function getViews(): Promise<ViewsResponse> {
  return json<ViewsResponse>('views/')
}

export function createView(name: string, config: Record<string, unknown>): Promise<SavedView> {
  return json<SavedView>('views/', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, config }),
  })
}

export async function deleteView(id: string): Promise<void> {
  const res = await raw(`views/${encodeURIComponent(id)}/`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) {
    throw new ApiError(res.status, `DELETE views/${id}/ → ${res.status}`)
  }
}

/* ── 워치 ── */

export function getWatches(): Promise<WatchesResponse> {
  return json<WatchesResponse>('watches/')
}

export async function addWatch(issueKey: string): Promise<void> {
  const res = await raw(`watches/${encodeURIComponent(issueKey)}/`, { method: 'PUT' })
  if (!res.ok) throw new ApiError(res.status, `PUT watches/${issueKey}/ → ${res.status}`)
}

export async function removeWatch(issueKey: string): Promise<void> {
  const res = await raw(`watches/${encodeURIComponent(issueKey)}/`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) {
    throw new ApiError(res.status, `DELETE watches/${issueKey}/ → ${res.status}`)
  }
}

/* ── 쓰기(Write) — 에러 바디 파싱 포함 ──
 * 쓰기 엔드포인트는 실패 시 { error, jira_errors? } 를 준다. 409 credential_required 는
 *  ApiError.code 로 구분되어 호출부가 자격증명 다이얼로그를 열 수 있다.
 */

/** 쓰기 응답 파서 — !ok 면 바디의 error/jira_errors 를 담아 ApiError 로 던진다. */
async function jsonW<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await raw(path, init)
  if (!res.ok) {
    let code: string | null = null
    let jiraErrors: Record<string, unknown> | null = null
    let message = `${init?.method ?? 'GET'} ${path} → ${res.status}`
    try {
      const body = (await res.json()) as { error?: string; jira_errors?: Record<string, unknown> }
      if (body.error) {
        code = body.error
        message = body.error
      }
      if (body.jira_errors) jiraErrors = body.jira_errors
    } catch {
      /* 바디 없음/비 JSON — 기본 메시지 유지 */
    }
    throw new ApiError(res.status, message, code, jiraErrors)
  }
  return (await res.json()) as T
}

const JSON_HEADERS = { 'Content-Type': 'application/json' }

/* ── 자격증명 (개인 Jira API 토큰) ── */

export function getCredential(): Promise<JiraCredential> {
  return jsonW<JiraCredential>('credential/')
}

export function saveCredential(jiraEmail: string, apiToken: string): Promise<JiraCredential> {
  return jsonW<JiraCredential>('credential/', {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ jira_email: jiraEmail, api_token: apiToken }),
  })
}

export function deleteCredential(): Promise<JiraCredential> {
  return jsonW<JiraCredential>('credential/', { method: 'DELETE' })
}

/* ── 첫 실행 온보딩 (loopback 전용) ──
 * `credential/` 과 달리 site 까지 받는다: 첫 실행에는 저장된 사이트가 없고,
 * `PUT credential/` 은 site 없이는 검증 대상이 없어 거부한다.
 */

export interface AvailableProject {
  key: string
  name: string
  projectTypeKey: string
}

/** POST sync/ · GET sync/progress/ 의 공통 응답. 자격증명 정보는 담기지 않는다. */
export interface SyncProgress {
  running: boolean
  /** idle | syncing | done | error */
  phase: string
  fetched: number
  changed: number
  deleted: number
  done: boolean
  error: string
  started_at: string
  finished_at: string
}

/** 사이트+이메일+토큰을 /myself 로 검증한 뒤 저장한다. 실패 시 ApiError.code 로 구분. */
export function connectJira(
  site: string,
  jiraEmail: string,
  apiToken: string,
): Promise<JiraCredential> {
  return jsonW<JiraCredential>('onboarding/connect/', {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ site, jira_email: jiraEmail, api_token: apiToken }),
  })
}

/** 사이트의 실제 프로젝트 목록. `truncated` 면 상한(500)에서 잘렸다는 뜻. */
export function getAvailableProjects(): Promise<{
  projects: AvailableProject[]
  truncated: boolean
}> {
  return jsonW<{ projects: AvailableProject[]; truncated: boolean }>('projects/available/')
}

/** 풀 싱크를 백그라운드로 시작. 이미 돌고 있으면 409 sync_in_progress. */
export function startFullSync(): Promise<SyncProgress> {
  return jsonW<SyncProgress>('sync/', { method: 'POST' })
}

export function getSyncProgress(): Promise<SyncProgress> {
  return jsonW<SyncProgress>('sync/progress/')
}

/* ── 상태 전환 ── */

export function getTransitions(issueKey: string): Promise<TransitionsResponse> {
  return jsonW<TransitionsResponse>(`${encodeURIComponent(issueKey)}/transitions/`)
}

export function doTransition(issueKey: string, transitionId: string): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/transition/`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ transition_id: transitionId }),
  })
}

/* ── 코멘트 ── */

export function postComment(
  issueKey: string,
  text: string,
  mentions: CommentMention[] = [],
  attachmentIds: string[] = [],
): Promise<CommentWriteResponse> {
  return jsonW<CommentWriteResponse>(`${encodeURIComponent(issueKey)}/comment/`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ text, mentions, attachment_ids: attachmentIds }),
  })
}

/**
 * 코멘트용 첨부 업로드 (multipart). Content-Type 은 브라우저가 boundary 와 함께 세팅하도록
 * 직접 지정하지 않는다(JSON_HEADERS 안 씀). 업로드된 첨부 메타를 돌려준다.
 */
export function uploadCommentAttachment(
  issueKey: string,
  file: File,
): Promise<AttachmentUploadResponse> {
  const fd = new FormData()
  fd.append('file', file)
  return jsonW<AttachmentUploadResponse>(`${encodeURIComponent(issueKey)}/attachments/`, {
    method: 'POST',
    body: fd,
  })
}

/* ── 담당자 ── */

export function setAssignee(
  issueKey: string,
  accountId: string | null,
): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/assignee/`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ account_id: accountId }),
  })
}

/* ── QA 필드 인라인 편집 ── */

/** PATCH <key>/fields/ — 허용 QA 필드 값 변경(옵션 id / accountId / 버전 id 배열). */
export function setIssueField(
  issueKey: string,
  field: string,
  value: string | string[] | null,
): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/fields/`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify({ field, value }),
  })
}

/** GET <key>/editmeta/ — 편집용 허용값(옵션/버전). 편집 불가 필드는 생략된다. */
export function getEditMeta(issueKey: string): Promise<EditMetaResponse> {
  return jsonW<EditMetaResponse>(`${encodeURIComponent(issueKey)}/editmeta/`)
}

/* ── 이슈 생성 ── */

export function createIssue(payload: CreateIssuePayload): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>('create/', {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify(payload),
  })
}

export function getCreateMeta(): Promise<CreateMetaResponse> {
  return jsonW<CreateMetaResponse>('create-meta/')
}

/* ── 사용자 검색 (담당자 지정용) ── */

export function searchUsers(q: string): Promise<UsersResponse> {
  return jsonW<UsersResponse>(`users/?q=${encodeURIComponent(q)}`)
}

/* ── 서버 설정 (loopback 전용) ──
 * `~/.scry/config.json` 의 편집 가능 부분. 자격증명은 포함되지 않는다(credential/ 담당).
 * 응답에서 어떤 필드든 빠질 수 있으므로 전부 optional.
 */

export interface SettingsMember {
  email: string
  name?: string
  display_name?: string
  group?: string
  department?: string
  job_role?: string
  jira_account_id?: string
  avatar_url?: string
}

/** 그룹 판정 규칙. 위에서 아래로 첫 매치 승, 조건끼리 AND·목록 내 OR, 빈 조건은 항상 참. */
export interface SettingsGroupRule {
  group: string
  projects?: string[]
  labels?: string[]
  components?: string[]
}

export interface ScrySettings {
  projects?: string[]
  fieldMap?: Record<string, string>
  bodyFields?: string[]
  editableFields?: Record<string, string>
  members?: SettingsMember[]
  groupRules?: SettingsGroupRule[]
  groupLabels?: Record<string, string>
  groupColors?: Record<string, string>
  productByGroup?: Record<string, { key: string; label: string }>
  features?: Partial<ScryFeatures>
  qaDashboardUrl?: string
  staleThresholdHours?: number
}

export function getSettings(): Promise<ScrySettings> {
  return jsonW<ScrySettings>('settings/')
}

/** 전체 교체(PUT). 받은 값을 그대로 저장하므로 부분 전송하면 나머지가 지워진다. */
export function putSettings(settings: ScrySettings): Promise<ScrySettings> {
  return jsonW<ScrySettings>('settings/', {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify(settings),
  })
}

/* ── 쓰기 메타 (익명 읽기) — 전환 맵 + create-meta 선반영 ── */

export function getWriteMeta(): Promise<WriteMeta> {
  return json<WriteMeta>('meta/write/')
}
