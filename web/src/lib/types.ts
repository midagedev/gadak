/*
 * Issue Navigator — API types (TypeScript of contract §1)
 *
 * Principle: keep field names as snake_case from the server response.
 *  No transform layer (performance) so bulk issues land as-is in the memory pool / IndexedDB.
 */

/** Effective status buckets. Server `status_category` is Jira's raw value; UI colors by these 3. */
export type StatusCategory = 'new' | 'inprogress' | 'done'

export type QaImpactState = 'blocking' | 'retest' | 'verified' | 'linked' | ''

/**
 * Per-issue deploy stage (precomputed). Progression: merged → dev (release) →
 *  qa_preview (qa release, pre-swap) → qa (swapped = QA can verify) → prod.
 *  none = no linked PR at all.
 */
export type DeployState = 'none' | 'merged' | 'dev' | 'qa_preview' | 'qa' | 'prod'

/** Release that carried the issue fix (tag + timestamp). */
export interface DeployReleaseRef {
  tag: string
  at: string
}

/** Lightweight deploy status embedded on IssueLite (precomputed). */
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
 * Lightweight issue for list/filter/search. bootstrap/delta return arrays of these.
 * (Body/comments/history omitted — detail is fetched on demand.)
 */
export interface IssueLite {
  issue_key: string
  summary: string
  status: string
  status_category: string // Raw Jira category string (effective bucket is StatusCategory)
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

  team_group: string | null
  epic_key: string | null
  source_project: string | null

  created_at: string | null // ISO8601
  updated_at: string | null
  resolved_at: string | null
  status_changed_at: string | null

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

  /** Deploy stage (precomputed). Older servers omit (undefined); empty object may arrive as default. */
  deploy_status?: DeployStatus
}

/** Team member (name/avatar/group). Bundled with bootstrap. */
export interface Member {
  email: string
  name: string
  display_name: string | null
  profile_image: string | null
  department: string | null
  job_role: string | null
  group: string | null
  status: string | null
  /** Jira accountId — used for assignee writes (local pick, no server round-trip). Backend fills. */
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
 * Raw shape of detail description_adf / comment.raw_body. Rendering lives in adf.ts ([detail]).
 * Here we only declare the recursive node shape.
 */
export interface AdfNode {
  type: string
  text?: string
  content?: AdfNode[]
  attrs?: Record<string, unknown>
  marks?: Array<{ type: string; attrs?: Record<string, unknown> }>
  [key: string]: unknown
}

/** One comment in the detail panel. */
export interface DetailComment {
  comment_id: string
  author: string | null
  author_email: string | null
  author_account_id?: string | null // For reply (mention) targeting
  body: string // plain text
  raw_body: AdfNode | null // Raw ADF
  created_at: string | null
}

/** One status/assignee/priority change (chronological). */
export interface HistoryEntry {
  at: string | null
  field: string
  from: string | null
  to: string | null
  by: string | null
  /** Pre/post category of a status transition (new|inprogress|done). Prefer over names for reopen. */
  from_category?: string | null
  to_category?: string | null
}

/** One linked issue. */
export interface LinkedIssue {
  key: string
  type: string
  direction: string
  summary: string | null
}

/** One linked PR (PrSnapshot). */
export interface LinkedPr {
  number: number
  title: string
  url: string
  state: string
  repo: string | null
  author: string | null
}

/** Jira attachment plus private S3 replay-cache metadata. */
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
  /** Same-origin attachment URL. Server serves straight from disk cache. */
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

/** Deploy evidence detail — one included release (tag + link + time + channel). All optional. */
export interface DeployReleaseEvidence {
  tag: string
  html_url?: string | null
  at?: string | null
  /** Release channel hint (dev/qa/prod, …). May be absent depending on server. */
  channel?: string | null
}

/** Deploy evidence detail — one PR inclusion row. */
export interface DeployPrInclusion {
  number: number
  title?: string | null
  url?: string | null
  repo?: string | null
  merged?: boolean
  /** Release tag that includes this PR. null/omitted if not included. */
  included_in?: string | null
}

/**
 * Deploy detail on the detail response — full lightweight DeployStatus plus evidence
 *  (included releases / per-PR inclusion / last swap). All fields optional for older servers;
 *  omit the section when state is missing.
 */
export interface DeployDetail extends Partial<DeployStatus> {
  releases?: DeployReleaseEvidence[]
  prs?: DeployPrInclusion[]
}

/** GET `<issue_key>/detail/` response. */
export interface DetailResponse {
  issue_key: string
  development_opinion: string
  /** Body-role custom field values (ADF documents) keyed by alias. Older servers omit. */
  bodies?: Record<string, AdfNode | string | null>
  description_adf: AdfNode | null
  attachments: DetailAttachment[]
  comments: DetailComment[]
  history: HistoryEntry[]
  linked_issues: LinkedIssue[]
  linked_prs: LinkedPr[]
  qa_context: QaIssueContext | null
  /** Deploy status detail. Older servers omit → hide the detail section. */
  deploy?: DeployDetail
}

/** GET `bootstrap/` response. */
/** One configured custom field, discovered from the Jira site catalog. */
export interface FieldSpec {
  /** Stable key. Issue rows carry the value under this name. */
  alias: string
  /** Jira display name, in the account's language. */
  label: string
  /** body = document (detail block) | facet = chips/filter | user | plain. */
  role: 'body' | 'facet' | 'user' | 'plain'
  /** Editor kind when inline-editable: option | multi_option | user | version_array. */
  kind?: string
}

export interface BootstrapResponse {
  server_time: string
  sync_version: number
  members: Member[]
  /** Stable hash of the member set. Echo as delta `mv` so unchanged members can be omitted. */
  members_version?: string // Older servers omit
  issues: IssueLite[]
  sync_health: SyncHealth
  /** Discovered custom fields. Older servers omit. */
  field_specs?: FieldSpec[]
  /** project → alias → filled count. Older servers omit. */
  field_usage?: Record<string, Record<string, number>>
}

/** GET `delta/?since=&mv=` response. */
export interface DeltaResponse {
  server_time: string
  upserted: IssueLite[]
  deleted_keys: string[]
  /** Omitted when `mv` matches the server hash → keep existing members. */
  members?: Member[]
  members_version?: string // Older servers omit
  sync_health: SyncHealth
  /** Discovery output; a delta-only tab must still learn about it. Older servers omit. */
  field_specs?: FieldSpec[]
  field_usage?: Record<string, Record<string, number>>
}

/** GET `search/?q=` response. */
export interface SearchResponse {
  keys: string[]
  total: number
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
  // Quiet hours (KST "HH:MM"). null = unused.
  quiet_start: string | null
  quiet_end: string | null
}

export interface NotificationConfig {
  enabled: boolean
  vapid_public_key: string
  preferences: NotificationPreferences
}

/**
 * Saved view (team-shared). `config` is opaque JSON the server does not interpret —
 *  front-end view state (filters/display); shape is defined by [explore].
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

/** Watch list response. */
export interface WatchesResponse {
  keys: string[]
}

/* ── Write API types (contract: write proxy) ────────────────────────────── */

/** GET/PUT/DELETE credential/ response — personal Jira API token status. Plain token never exposed. */
export interface JiraCredential {
  configured: boolean
  jira_email: string
  display_name: string
  verified_at: string | null
  token_hint: string
}

/** One transition option (GET <key>/transitions/). */
export interface Transition {
  id: string
  name: string
  to_status: string
  to_category: string // Jira statusCategory key (new/indeterminate/done)
}

export interface TransitionsResponse {
  transitions: Transition[]
}

/** One assignee candidate (GET users/?q=). */
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

/* ── QA field inline edit (editmeta) ── */

/** One option/version choice (id + display value). */
export interface EditMetaOption {
  id: string
  value: string
}

/** Meta for one editable field — kind + editable flag + choices. */
export interface EditMetaField {
  /** option (single select) / user (userpicker) / version_array (version list). */
  kind: 'option' | 'user' | 'version_array'
  editable: boolean
  /** Empty for user fields (user search replaces options). */
  options: EditMetaOption[]
}

/** GET <key>/editmeta/ — edit meta keyed by front-end field. Non-editable fields omitted. */
export interface EditMetaResponse {
  fields: Partial<Record<string, EditMetaField>>
}

/** Issue type (create-meta entry). */
export interface CreateMetaIssueType {
  id: string
  name: string
}

/** One creatable project (GET create-meta/). */
export interface CreateMetaProject {
  key: string
  name: string
  issue_types: CreateMetaIssueType[]
}

export interface CreateMetaResponse {
  projects: CreateMetaProject[]
}

/** New comment returned by POST <key>/comment/ (no raw_body — plain text body). */
export interface CreatedComment {
  comment_id: string
  author: string | null
  body: string
  created_at: string | null
}

/** Common write response — latest IssueLite. */
export interface IssueWriteResponse {
  issue: IssueLite
}

export interface CommentWriteResponse {
  issue: IssueLite
  comment: CreatedComment
}

/** One mention inserted in a comment (account_id + display name from front-end autocomplete). */
export interface CommentMention {
  account_id: string
  display_name: string
}

/** One uploaded attachment (POST <key>/attachments/ response). Used for comment inline embeds. */
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
 * GET meta/write/ response (anonymous read) — static write meta prefetched in bulk.
 *  - transitions: project → current status → available transitions (0ms dropdown).
 *  - create_meta: creatable projects/issue types (new-issue dialog).
 * Loaded at boot + IndexedDB cache + reload every 15 minutes.
 */
export interface WriteMeta {
  transitions: Record<string, Record<string, Transition[]>>
  create_meta: { projects: CreateMetaProject[] }
  updated_at: string | null
}

/** Write-meta record in the IndexedDB meta store. */
export interface WriteMetaCache {
  key: 'write'
  transitions: Record<string, Record<string, Transition[]>>
  projects: CreateMetaProject[]
  updated_at: string | null
  cached_at: string
}

/** POST create/ request body. */
export interface CreateIssuePayload {
  project_key: string
  issue_type: string // issue_type id from create-meta
  summary: string
  description_text?: string
  assignee_account_id?: string | null
  priority?: string
  labels?: string[]
}

/** Cache meta stored in the IndexedDB meta store. */
export interface CacheMeta {
  key: 'sync' // Singleton record key
  server_time: string
  sync_version: number
  members: Member[]
  members_version?: string // Member-set hash; used as next delta's mv (absent in older caches)
  sync_health?: SyncHealth
  field_specs?: FieldSpec[] // Discovered custom fields (absent in older caches)
  field_usage?: Record<string, Record<string, number>>
}
