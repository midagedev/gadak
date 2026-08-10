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
  CommentsByAuthorResponse,
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
  NotificationConfig,
  NotificationPreferences,
  PageDetail,
  PagesResponse,
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

/** Parse a JSON response; throw ApiError on 4xx/5xx. */
async function json<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await raw(path, init)
  if (!res.ok) {
    throw new ApiError(res.status, `${init?.method ?? 'GET'} ${path} → ${res.status}`)
  }
  return (await res.json()) as T
}

/* ── bootstrap (ETag / 304) ── */

export type BootstrapResult =
  | { status: 'ok'; data: BootstrapResponse; etag: string | null }
  | { status: 'not_modified' }

/**
 * Load all issues + members. Pass prior `etag` (If-None-Match) to get `not_modified` on 304.
 * ETag shape is `"in-<sync_version>"` (server). Echo it on the next call.
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
 * Changes since `since`. Pass prior `membersVersion` so the server can omit members
 * when the hash matches (payload diet). Without it, server always includes members.
 */
export function getDelta(since: string, membersVersion?: string): Promise<DeltaResponse> {
  let path = `delta/?since=${encodeURIComponent(since)}`
  if (membersVersion) path += `&mv=${encodeURIComponent(membersVersion)}`
  return json<DeltaResponse>(path)
}

/* ── detail (on demand) ── */

export function getDetail(issueKey: string): Promise<DetailResponse> {
  return json<DetailResponse>(`${encodeURIComponent(issueKey)}/detail/`)
}

/* ── Full-text search (server) ── */

export function search(q: string, limit = 200): Promise<SearchResponse> {
  return json<SearchResponse>(`search/?q=${encodeURIComponent(q)}&limit=${limit}`)
}

/* ── Mirrored wiki pages (docs) ── */

/** Every mirrored page, without bodies. Small enough to hold in memory. */
export function getPages(): Promise<PagesResponse> {
  return json<PagesResponse>('pages/')
}

/** One page with its body ADF and comments. */
export function getPageDetail(key: string): Promise<PageDetail> {
  return json<PageDetail>(`pages/${encodeURIComponent(key)}/`)
}

/**
 * Re-fetch one issue from the upstream tracker into the mirror.
 * Response shape matches other write endpoints (refreshed IssueLite).
 */
export function resyncIssue(issueKey: string): Promise<IssueWriteResponse> {
  return jsonW<IssueWriteResponse>(`${encodeURIComponent(issueKey)}/resync/`, {
    method: 'POST',
  })
}

/**
 * Re-fetch one mirrored wiki page. 204 with no body on success.
 */
export async function resyncPage(pageId: string): Promise<void> {
  const res = await raw(`pages/${encodeURIComponent(pageId)}/resync/`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(res.status, `POST pages/${pageId}/resync/ → ${res.status}`)
  }
}

/* ── People axis ── */

/**
 * One person's comments, newest first. `authorId` is the source account id
 * (`comments.author_id`), which is what `Member.jira_account_id` carries.
 * Unlike the rest of the read path this is a request: comment bodies are not
 * in the client pool, and holding every comment in memory to answer one panel
 * would cost the boot budget the pool is spent on.
 */
export function getCommentsByAuthor(
  authorId: string,
  limit = 50,
): Promise<CommentsByAuthorResponse> {
  return json<CommentsByAuthorResponse>(
    `people/${encodeURIComponent(authorId)}/comments/?limit=${limit}`,
  )
}

/* ── Personal feed / read state ── */

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

/* ── Saved views ── */

export function getViews(): Promise<ViewsResponse> {
  return json<ViewsResponse>('views/')
}

export function createView(
  name: string,
  config: import('./view-config').ViewConfig,
): Promise<SavedView> {
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

/* ── Watches ── */

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

/* ── Favorites (response shape matches watches) ── */

export function getFavorites(): Promise<WatchesResponse> {
  return json<WatchesResponse>('favorites/')
}

export async function addFavorite(issueKey: string): Promise<void> {
  const res = await raw(`favorites/${encodeURIComponent(issueKey)}/`, { method: 'PUT' })
  if (!res.ok) throw new ApiError(res.status, `PUT favorites/${issueKey}/ → ${res.status}`)
}

export async function removeFavorite(issueKey: string): Promise<void> {
  const res = await raw(`favorites/${encodeURIComponent(issueKey)}/`, { method: 'DELETE' })
  if (!res.ok && res.status !== 404) {
    throw new ApiError(res.status, `DELETE favorites/${issueKey}/ → ${res.status}`)
  }
}

/* ── Write — with error-body parsing ──
 * Write endpoints return { error, jira_errors? } on failure. 409 credential_required is
 *  exposed as ApiError.code so callers can open the credential dialog.
 */

/** Write-response parser — on !ok, throw ApiError carrying body error/jira_errors. */
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
      /* No body / non-JSON — keep default message */
    }
    throw new ApiError(res.status, message, code, jiraErrors)
  }
  return (await res.json()) as T
}

const JSON_HEADERS = { 'Content-Type': 'application/json' }

/* ── Credentials (personal Jira API token) ── */

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

/* ── First-run onboarding (loopback only) ──
 * Unlike `credential/`, this also takes site: first run has no stored site, and
 * `PUT credential/` rejects without a site because there is nothing to verify against.
 */

export interface AvailableProject {
  key: string
  name: string
  projectTypeKey: string
}

/** Shared response of POST sync/ · GET sync/progress/. Never carries credential data. */
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
  /**
   * Any pass running in the server process, including the background watch
   * loop — the fields above describe only the one-shot job this client can
   * start. Absent on servers older than this field; treat that as "unknown",
   * never as "idle".
   */
  activity?: {
    running: boolean
    /** issues | documents, '' when nothing is in flight. */
    source: string
    /** Running total for the current source's pass, reset when it changes. */
    fetched: number
    changed: number
    started_at: string
  }
}

/** Verify site+email+token via /myself, then store. Failures distinguished by ApiError.code. */
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

/** Real project list for the site. `truncated` means the list was capped at 500. */
export function getAvailableProjects(): Promise<{
  projects: AvailableProject[]
  truncated: boolean
}> {
  return jsonW<{ projects: AvailableProject[]; truncated: boolean }>('projects/available/')
}

/**
 * Start a background sync. Default mode is full (onboarding first run).
 * Daily "Sync now" uses `incremental`. 409 sync_in_progress when already running.
 */
export function startSync(mode: 'full' | 'incremental' = 'full'): Promise<SyncProgress> {
  return jsonW<SyncProgress>('sync/', {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ mode }),
  })
}

/** @deprecated Prefer startSync('full') — kept for call-site clarity in onboarding. */
export function startFullSync(): Promise<SyncProgress> {
  return startSync('full')
}

export function getSyncProgress(): Promise<SyncProgress> {
  return jsonW<SyncProgress>('sync/progress/')
}

/* ── Status transitions ── */

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

/* ── Comments ── */

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
 * Upload a comment attachment (multipart). Do not set Content-Type yourself so the
 * browser can add the boundary (skip JSON_HEADERS). Returns uploaded attachment meta.
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

/* ── Assignee ── */

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

/* ── QA field inline edit ── */

/** PATCH <key>/fields/ — change an allowed QA field (option id / accountId / version id array). */
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

/** GET <key>/editmeta/ — allowed edit values (options/versions). Non-editable fields omitted. */
export function getEditMeta(issueKey: string): Promise<EditMetaResponse> {
  return jsonW<EditMetaResponse>(`${encodeURIComponent(issueKey)}/editmeta/`)
}

/* ── Issue create ── */

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

/* ── User search (for assignee picker) ── */

export function searchUsers(q: string): Promise<UsersResponse> {
  return jsonW<UsersResponse>(`users/?q=${encodeURIComponent(q)}`)
}

/* ── Server settings (loopback only) ──
 * Editable slice of `~/.scry/config.json`. Credentials are not included (credential/ owns those).
 * Any field may be absent in the response, so everything is optional.
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

/** Group-assignment rule. First match wins top-to-bottom; conditions AND, list values OR; empty condition always true. */
export interface SettingsGroupRule {
  group: string
  projects?: string[]
  labels?: string[]
  components?: string[]
}

/** Read-only instance facts from GET settings/. Never carries secrets. */
export interface SettingsRuntime {
  profile: string
  dbPath: string
  dbSizeBytes: number
  dbSizeHuman: string
  dbModifiedAt?: string | null
  configPath: string
  issueCount: number
  commentCount: number
  schemaVersion: number
  watermark?: string
  syncVersion: number
  lastFullSyncAt?: string | null
  lastError?: string | null
  scryVersion: string
  /** Placeholder when syncIntervalSec is 0. */
  defaultSyncIntervalSec: number
  /** Placeholder when reconcileIntervalSec is 0. */
  defaultReconcileIntervalSec: number
  /** Our own outbound Jira volume, not Jira's remaining budget. */
  apiUsage?: ApiUsageSummary
}

export interface ApiUsageDay {
  day: string
  requests: number
  throttled: number
  server_errors: number
  retries: number
  wait_ms: number
  last_throttled_at?: string | null
}

export interface ApiUsageSummary {
  today: ApiUsageDay
  last_7_days: ApiUsageDay
}

/** One discovered/configured custom field as settings carries it (full shape, ids included). */
export interface SettingsFieldSpec {
  alias: string
  label: string
  ids: string[]
  role: string
  kind?: string
  /** true = discovery-owned (regenerated by `scry fields --apply`); false = user-pinned. */
  auto?: boolean
}

/** Confluence slice of settings. Present in the GET response only while the
 *  source is configured — absence is how the UI knows to hide the scope picker,
 *  and PUTting the key with the source off is a 400. */
export interface SettingsConfluence {
  /**
   * Turn the source on or off. Absent on the way in (the server omits the whole
   * block while off); sent on the way out, because a bare `spaces` is rejected
   * unless the source is already on — which is what used to make this screen
   * unable to enable Confluence at all.
   */
  enabled?: boolean
  /** Mirrored space keys. Empty = every global space, but only while enabled. */
  spaces: string[]
}

export interface ScrySettings {
  projects?: string[]
  confluence?: SettingsConfluence
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
  /** Incremental sync period in seconds. 0 = server default. Min 15 when set. */
  syncIntervalSec?: number
  /** Reconcile (deletion) period in seconds. 0 = server default. Min 300 when set. */
  reconcileIntervalSec?: number
  /** Field-spec edits. Only sent when the user touched the section — absence preserves discovery output. */
  fields?: SettingsFieldSpec[]
  /** Connection panel (read-only). */
  site?: string
  hasCredential?: boolean
  /** Instance facts (read-only; ignored on PUT). */
  runtime?: SettingsRuntime
  /** Discovered field specs (read-only; edit via `fields`). */
  fieldSpecs?: SettingsFieldSpec[]
  /** project → alias → filled (read-only). */
  fieldUsage?: Record<string, Record<string, number>>
}

/** One recorded sync pass (meaningful runs only: changed something, full, or failed). */
export interface SyncRun {
  kind: string // full | incremental (+reconcile)
  started_at: string
  finished_at: string
  fetched: number
  changed: number
  deleted: number
  error?: string
}

/**
 * GET sync/runs/ — newest first. Older servers 404 → treat as empty.
 *
 * `source` selects which connector's history to read; omitted means Jira, the
 * only one the endpoint used to serve. A server that predates the parameter
 * ignores it and answers with Jira's runs, so callers asking for Confluence
 * must not read a non-empty answer as proof Confluence ever ran — check
 * `source` on the response.
 */
export async function getSyncRuns(source?: 'jira' | 'confluence'): Promise<SyncRun[]> {
  try {
    const path = source ? `sync/runs/?source=${source}` : 'sync/runs/'
    const res = await jsonW<{ runs: SyncRun[]; source?: string }>(path)
    if (source && res.source !== source) return []
    return res.runs ?? []
  } catch {
    return []
  }
}

/* ── workspaces (one process, several profile mirrors) ── */

/** One profile the running serve can mount. Never carries credentials. */
export interface WorkspaceInfo {
  name: string
  site?: string
  projects?: string[]
  /** True for the profile serve was started with (its API is at the root). */
  active?: boolean
  error?: string
}

/**
 * List mountable workspaces. Deliberately fetched at the origin root, not the
 * API base: the list is one process-wide fact, the same from every mount.
 * Older servers and the hosted demo answer 404 → empty list, section hidden.
 */
export async function getWorkspaces(): Promise<WorkspaceInfo[]> {
  try {
    const res = await fetch('/api/v1/workspaces', { credentials: 'same-origin' })
    if (!res.ok) return []
    const doc = (await res.json()) as { workspaces?: WorkspaceInfo[] }
    return doc.workspaces ?? []
  } catch {
    return []
  }
}

export function getSettings(): Promise<ScrySettings> {
  return jsonW<ScrySettings>('settings/')
}

/** One Confluence space offered by the scope picker. */
export interface SettingsSpace {
  key: string
  name: string
  /** global | personal. Personal spaces are noise for most mirrors. */
  type: string
  selected: boolean
}

/**
 * Live space list for the settings picker (global first). `all_global_when_empty`
 * says the stored scope is empty, which the sync reads as "every global space" —
 * so an empty selection here is a meaning, not a missing answer.
 * 400 confluence_not_configured when the source is off.
 */
export function getSettingsSpaces(): Promise<{
  spaces: SettingsSpace[]
  all_global_when_empty: boolean
}> {
  // `enabled` is the source's on/off state. The list itself is live discovery
  // and needs only a credential, so it answers while Confluence is off too —
  // which is what lets someone choose spaces before turning it on.
  return jsonW<{
    spaces: SettingsSpace[]
    all_global_when_empty: boolean
    enabled: boolean
  }>('settings/spaces/')
}

/** Full replace (PUT). Values are stored as sent — partial payloads wipe the rest. */
export function putSettings(settings: ScrySettings): Promise<ScrySettings> {
  return jsonW<ScrySettings>('settings/', {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify(settings),
  })
}

/* ── Write meta (anonymous read) — transition map + create-meta prefetched ── */

export function getWriteMeta(): Promise<WriteMeta> {
  return json<WriteMeta>('meta/write/')
}
