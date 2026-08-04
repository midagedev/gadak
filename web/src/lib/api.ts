/*
 * Issue Navigator — REST 클라이언트 (계약 §1)
 *
 * base 는 런타임 config() 에서 온다. dev 는 Vite 프록시가 로컬 서버로 넘긴다.
 * 인증: localStorage `scry_token` 이 있으면 `Authorization: Token <t>` 를 붙인다.
 *  (읽기는 익명 허용, 쓰기는 IsAuthenticated — 토큰/세션 쿠키 둘 다 지원)
 */

import { config } from './config'
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

// 런타임 config 에서 읽는다(빌드 산출물은 테넌트 중립).

/** localStorage 에 저장된 API 토큰 키. */
export const TOKEN_KEY = 'scry_token'

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

/**
 * 인증 실패(401)·자격증명 미설정(409) 등을 호출부가 구분할 수 있도록 하는 에러.
 * `code` 는 서버 에러 바디의 `error` 문자열(예: 'credential_required'),
 * `jiraErrors` 는 Jira 원본 필드 에러(있으면 그대로 보여줄 수 있게).
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

function authHeaders(extra?: HeadersInit): Headers {
  const h = new Headers(extra)
  const token = getToken()
  if (token) h.set('Authorization', `Token ${token}`)
  return h
}

/** 공통 fetch — 상대 경로(BASE 기준)를 받아 Response 를 돌려준다. */
async function raw(path: string, init?: RequestInit): Promise<Response> {
  return fetch(config().apiBase + path, {
    credentials: 'same-origin', // 세션 인증 폴백
    ...init,
    headers: authHeaders(init?.headers),
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
  const res = await raw('bootstrap/', { headers: authHeaders(headers) })
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

/* ── 쓰기 메타 (익명 읽기) — 전환 맵 + create-meta 선반영 ── */

export function getWriteMeta(): Promise<WriteMeta> {
  return json<WriteMeta>('meta/write/')
}
