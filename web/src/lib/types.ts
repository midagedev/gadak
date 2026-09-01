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
  /** Stable Jira status id. Older cached rows may omit it. */
  status_id?: string
  status_category: string // Raw Jira category string (effective bucket is StatusCategory)
  issue_type: string
  /** Stable Jira issue-type id. Older cached rows may omit it. */
  issue_type_id?: string
  priority: string | null
  /** Stable Jira priority id. Older cached rows and mirrors not yet
   *  resynced may omit it or send '' — match falls back to `priority`. */
  priority_id?: string | null
  priority_rank: number | null
  severity: string | null

  assignee: string | null
  /** Stable Jira accountId. Older cached rows may omit it. */
  assignee_id?: string | null
  assignee_email: string | null
  reporter: string | null
  /** Stable Jira accountId. Older servers/cached rows may omit it. */
  reporter_id?: string | null
  reporter_email: string | null

  labels: string[]
  fix_versions: string[]
  components: string[]

  team_group: string | null
  /** Nearest epic (hierarchy level 1) ancestor, derived server-side. Null when none. */
  epic_key: string | null
  /** Direct parent issue, which is the epic only when the parent happens to be one. */
  parent_key: string | null
  /** Source tree rank from issues.hierarchy_level (epic 1, standard 0, sub-task -1). Older cached rows may omit it. */
  hierarchy_level?: number
  source_project: string | null
  /** Origin that owns the row (`jira` / `linear`). Older caches omit it. */
  source?: string

  created_at: string | null // ISO8601
  updated_at: string | null
  resolved_at: string | null
  status_changed_at: string | null
  /** Calendar date (YYYY-MM-DD). Null when unset. Older cached rows may omit it. */
  duedate?: string | null

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
  /** Account ids that touched this issue — comment, changelog entry, or
   *  dev-panel link (server's issue_actors view). Older servers omit.
   *  The actor filter and grouping narrow on this, never on display names. */
  actor_ids?: string[]
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
  /** Origin account type: 'agent' (standalone) or 'app' (Cloud). Older servers omit. */
  account_type?: string | null
  /** Server's judgement that this account is a bot (account_type agent/app).
   *  The badge renders on this flag alone — the client never re-derives it. */
  is_bot?: boolean
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

export interface TokenExpiry {
  state: 'ok' | 'expiring' | 'expired' | 'unknown'
  days_left?: number
  expires_at?: string
  source?: 'user' | 'assumed' | string
  urgent?: boolean
  message?: string
}

export interface SyncHealth {
  overall: 'healthy' | 'warning' | 'failed'
  checked_at: string
  sources: SyncSourceHealth[]
  token_expiry?: TokenExpiry
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
  /** Origin account type of the author ('agent'|'app'). Older servers omit. */
  author_account_type?: string | null
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
  /** Author's account id — attribution without display names. Older servers omit. */
  author_id?: string | null
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
  /** Who attached the link (dev-panel actor) — a different axis from the
   *  PR's own author: a bot linking a human's PR keeps both. Older servers omit. */
  linked_by?: string | null
  linked_by_id?: string | null
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
  /** Plain/markdown body when description_adf is empty (Linear). Older servers omit. */
  description_text?: string
  attachments: DetailAttachment[]
  comments: DetailComment[]
  history: HistoryEntry[]
  linked_issues: LinkedIssue[]
  linked_prs: LinkedPr[]
  qa_context: QaIssueContext | null
  /** Deploy status detail. Older servers omit → hide the detail section. */
  deploy?: DeployDetail
  /** Pages this issue's own text names. Text-derived, so it exists without
   *  anyone having drawn a link in Jira. Omitted when empty. */
  ref_pages?: PageLite[]
  /** Pages whose text names this issue — the other direction of the same
   *  derivation. Omitted when empty. */
  backlink_pages?: PageLite[]
  /** Lifecycle spans (server's store.Durations, same numbers the CLI prints).
   *  Absent when the changelog cannot answer — the chip renders nothing. */
  wait_ms?: number | null
  progress_ms?: number | null
  /** Cross-workspace / external pointers (GDK-1032). Omitted when empty. */
  refs?: IssueRef[]
}

/** One reference out of this issue: a pointer at an issue in another
 *  workspace (gadak://<workspace>/<KEY>) or any external URL. When this
 *  machine mirrors the named workspace the server hydrates the target's
 *  live state and sets `hydrated`; otherwise the pointer stands with no
 *  live half, which is a state to show, not an error. */
export interface IssueRef {
  id: string
  url: string
  relationship?: string
  title?: string
  workspace?: string
  key?: string
  summary?: string
  status?: string
  status_category?: string
  assignee?: string
  hydrated: boolean
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
  /** Editor kind when inline-editable: option | multi_option | user | version_array | component_array. */
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
  /** Set only when a newer release than the running build is published. */
  latest_version?: string
  release_url?: string
  /** GitHub release body. Absent or empty → banner stays a link, no dialog. */
  release_notes?: string
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
  /** Set only when a newer release than the running build is published. */
  latest_version?: string
  release_url?: string
  /** GitHub release body. Absent or empty → banner stays a link, no dialog. */
  release_notes?: string
}

/** Which column of the full-text index a hit came from, and the text around it.
 *  `snippet` is plain text — the server never marks it up, so the client
 *  highlights it against its own query. */
export interface SearchMatch {
  field: 'title' | 'body' | 'comment'
  snippet: string
}

/** GET `search/?q=` response. */
export interface SearchResponse {
  keys: string[]
  total: number
  /** Mirrored wiki pages matching the same query. Older servers omit. */
  pages?: PageLite[]
  /** Why each returned issue or page matched, keyed by its key. Older servers omit. */
  matches?: Record<string, SearchMatch>
}

/* ── Personal history (local.db; never sent to Jira) ── */

/** POST `history/visits/` body `kind`. Server rejects anything else. */
export type HistoryVisitKind = 'issue' | 'page'

/** POST `history/visits/` response. */
export interface VisitEvent {
  id: number
  kind: HistoryVisitKind
  key: string
  viewed_at: string
}

/** POST / PATCH `history/searches/` response. */
export interface SearchEvent {
  id: number
  query: string
  searched_at: string
  result_count: number
  opened_kind: string | null
  opened_key: string | null
}

/**
 * One GET `history/` row. `type` is `visit` or `search`; visit fields and
 * search fields are omitted on the other kind (server `omitempty`).
 */
export interface HistoryItem {
  type: 'visit' | 'search'
  id: number
  kind?: string
  key?: string
  query?: string
  result_count?: number | null
  opened_kind?: string | null
  opened_key?: string | null
  at: string
}

/** GET `history/` page. `next_cursor` is absent on the last page. */
export interface HistoryPage {
  items: HistoryItem[]
  next_cursor?: string
}

/* ── Mirrored wiki pages (docs) ── */

/** One mirrored wiki page, without body. Sidebar rows and search hits use this. */
export interface PageLite {
  key: string
  title: string
  space_key: string
  /** Human-readable space name. Empty until the space is mirrored, and absent
   *  on older servers — both fall back to `space_key` for display. */
  space_name?: string
  /** Content id of the space's homepage (root page). Empty until learned, and
   *  absent on older servers — both mean "do not drop the first breadcrumb
   *  ancestor by identity". */
  space_homepage_id?: string
  parent_id: string | null
  author: string | null
  /** Stable author account id. Older servers/cached rows omit it — group
   *  by this when present, else the display name. */
  author_id?: string | null
  updated_at: string | null
  version: number
  url: string
  /** First ~200 characters of the body as plain text. Empty when the page has
   *  no body, and absent on older servers — both mean "render no preview". */
  excerpt?: string
  /** Confluence labels on the page. The server always sends an array (empty,
   *  never null); older ones omit the field, so read it as `?? []`. */
  labels?: string[]
}

/** One comment on a mirrored page. Pages have no comment ids, so nothing to reply to. */
export interface PageComment {
  author: string | null
  created_at: string | null
  body_adf: AdfNode | null
  body_text: string
}

/** GET `pages/` response. */
export interface PagesResponse {
  pages: PageLite[]
  total: number
}

/** GET `pages/{key}/` response — PageLite plus body and comments. */
export interface PageDetail extends PageLite {
  body_adf: AdfNode | null
  comments: PageComment[]
  /** Issue keys this page's own text names. Only keys the mirror actually
   *  holds — the server drops the rest. Omitted when empty. */
  ref_issue_keys?: string[]
  /** Issue keys whose text names this page. Same filter, other direction. */
  backlink_issue_keys?: string[]
}

/* ── People axis ── */

/** One comment in `GET people/{author_id}/comments/`, joined to what it is on.
 *  `key` is that parent's key, so a row opens the issue or the page it lives in. */
export interface AuthorComment {
  key: string
  kind: 'issue' | 'page'
  title: string
  /** Plain text, whitespace-normalized, hard-cut at 160 runes by the server. */
  snippet: string
  created_at: string
}

/** GET `people/{author_id}/comments/?limit=` response. An unknown id answers
 *  200 with `total: 0`, never 404 — the panel draws an empty person. */
export interface CommentsByAuthorResponse {
  /** Display name from the newest matching comment; '' when there are none. */
  author: string
  /** Full count for this author — `comments` is only the requested page of it. */
  total: number
  comments: AuthorComment[]
}

/** The feed's focus slices, in tab order. The list is the owner of the type
 *  (GDK-825): a second hand copy in App used to be the one an incoming `feed=`
 *  value was validated against — the copy could accept a focus the feed has
 *  no tab for, or reject one it does, and nothing would say so. */
export const FEED_FOCUSES = ['all', 'assignee', 'reporter', 'mention'] as const

export type FeedFocus = (typeof FEED_FOCUSES)[number]

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
 *
 * Default `C` is ViewConfig so call sites need no cast; the server still treats
 * config as opaque JSON. Override C only for raw/transport edges.
 * Inline import avoids a top-level cycle with view-config.ts.
 */
export interface SavedView<C = import('./view-config').ViewConfig> {
  id: string
  name: string
  owner_email: string | null
  owner_name: string | null
  config: C
  created_at: string | null
  updated_at: string | null
}

export interface ViewsResponse {
  views: SavedView[]
  /** Named queries mirrored from Jira (owned + starred filters). */
  source?: SourceView[]
}

export interface SourceView<C = import('./view-config').ViewConfig> {
  id: string
  name: string
  config: C
  jql: string
  /** Jira filter id, used to open `/issues/?filter=`. */
  external_id?: string
  favourite: boolean
  owner?: string
  applied: string[]
  unsupported: string[]
}

/* ── Agent dashboards (GDK-781 web host) ─────────────────────────────────── */

/**
 * One dashboard config as saved. `config` is the authored document verbatim:
 * html plus a datasources map. Kept opaque (`unknown`) here on purpose — the
 * parent host only needs the datasource NAMES to know which data routes to
 * run; interpretation is the frame's (html) and the server's (execution).
 */
export interface DashboardRow {
  id: string
  name: string
  config: {
    html: string
    datasources: Record<string, { sql?: string; jql?: string }>
  }
  created_at?: string
  updated_at?: string
}

export interface DashboardsResponse {
  /** Bumped on every save/update/delete — the live-update poll's signal. */
  version: number
  dashboards: Pick<DashboardRow, 'id' | 'name' | 'updated_at'>[]
}

/** GET {id}/data/{name}/ result — same document the frame receives via postMessage. */
export interface DashboardDataDoc {
  columns: string[]
  rows: unknown[][]
  truncated: boolean
  warning?: string
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
  /** True when the profile has a Linear block (cfg.Linear != nil). */
  linear?: boolean
}

/** One required screen field on a transition (GET <key>/transitions/). */
export interface TransitionField {
  id: string
  name: string
  type: string
  options: EditMetaOption[]
}

/** One transition option (GET <key>/transitions/). */
export interface Transition {
  id: string
  name: string
  to_status: string
  to_category: string // Jira statusCategory key (new/indeterminate/done)
  /** Required screen fields only; omitted when none. */
  fields?: TransitionField[]
}

export interface TransitionsResponse {
  transitions: Transition[]
}

/** One site priority (GET priorities/). Names follow the account language. */
export interface PriorityOption {
  id: string
  name: string
}

export interface PrioritiesResponse {
  priorities: PriorityOption[]
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
  /** option (single select) / user (userpicker) / version_array (version list) / component_array (component list) / parent (issue key). */
  kind: 'option' | 'user' | 'version_array' | 'component_array' | 'multi_option' | 'parent'
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
  /** True when this type requires a parent. Omitted when false. */
  subtask?: boolean
  /** Source tree rank: epic 1, standard 0, sub-task -1. Omitted when 0. */
  hierarchyLevel?: number
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

/** One field from GET create-meta/fields/ (GDK-254). Server passes origin facts through. */
export interface CreateFieldMeta {
  field_id: string
  name: string
  required: boolean
  has_default: boolean
  type: string
}

export interface CreateFieldsResponse {
  fields: CreateFieldMeta[]
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

/** POST create/ request body. Only summary is required (GDK-218): omitted
 * project/type resolve server-side (flag → profile default → sole option). */
export interface CreateIssuePayload {
  project_key?: string
  issue_type?: string // issue_type id from create-meta
  summary: string
  description_text?: string
  assignee_account_id?: string | null
  /** Site priority id from GET priorities/. Omit when unset. */
  priority_id?: string
  labels?: string[]
  /** Calendar date (YYYY-MM-DD). Omit when unset — do not send "". */
  duedate?: string
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
