// Wire shapes the companion reads. Field names are the server's
// (internal/server/read.go, internal/store/read.go) — a subset: the phone
// only parses what it paints. Adding fields is safe, renaming is not.
//
// GDK-1132: every shape a web client also parses is OWNED by
// web/src/lib/types.ts, reached for as `import type` (erased at build — the
// same mechanism ViewFilters below proved). The phone's subset is a Pick of
// the owner, so a field's type and optionality cannot drift between the two
// clients again: they are read off the one owner at compile time, and a
// rename on the desk breaks `npm run check` here instead of lying quietly.
// The measured drift this replaced: the phone promised `status_id: string`
// where the wire omits the field on older cached rows, and
// `priority_rank: number` where the owner says `number | null`.

/**
 * The desktop's saved-view schema, imported as a type so there is exactly one
 * definition of what a stored view means (web/src/lib/view-config.ts). This is
 * an `import type`: it is erased at build time, so the phone bundle does not
 * pull in the desktop's config/feature-flag modules.
 */
export type { ViewConfig, ViewFilters } from '../../../web/src/lib/view-config'

/**
 * Shared wire shapes, all type-only from the desktop's definition:
 * AdfNode / DetailAttachment (GDK-1497) — one meaning of "a description
 * document" and "an attachment row", rendered by the same renderer
 * (web/src/lib/adf.ts); FlowSummary / HistoryEntry (GDK-1495) — the learned
 * flow the stale threshold reads, and one changelog entry the resume card
 * counts. The `Web*` aliases feed the Picks below (GDK-1132).
 */
import type {
  AdfNode,
  DetailAttachment,
  FlowSummary,
  HistoryEntry,
  BootstrapResponse as WebBootstrapResponse,
  DetailComment as WebDetailComment,
  DetailResponse as WebDetailResponse,
  IssueLite as WebIssueLite,
  LinkedIssue as WebLinkedIssue,
  PageComment as WebPageComment,
  PageDetail as WebPageDetail,
  PageLite as WebPageLite,
  PagesResponse as WebPagesResponse,
  SavedView as WebSavedView,
  SearchMatch as WebSearchMatch,
  SearchResponse as WebSearchResponse,
  SourceView as WebSourceView,
} from '../../../web/src/lib/types'
export type { AdfNode, DetailAttachment, FlowSummary, HistoryEntry }

/**
 * The wire row (bootstrap, delta, write answers) — the fields the phone
 * paints, with the owner's optionality: `status_id`, `issue_type_id`,
 * `priority_id` and `assignee_id` are optional because older cached rows
 * omit them, and `priority_rank` is `number | null` (null = unranked,
 * sort last). Field semantics live on the owner; the two the phone reads
 * hardest are `status_category` (the only status axis logic may key on) and
 * `reporter_id`/`reporter_email` (the delegation ledger's half of
 * person-match, GDK-1495 ④ — id first, email the fallback).
 */
export type IssueLite = Pick<
  WebIssueLite,
  | 'issue_key'
  | 'summary'
  | 'project_key'
  | 'issue_type'
  | 'issue_type_id'
  | 'status'
  | 'status_id'
  | 'status_category'
  | 'priority'
  | 'priority_id'
  | 'priority_rank'
  | 'assignee'
  | 'assignee_id'
  | 'assignee_email'
  | 'reporter'
  | 'reporter_id'
  | 'reporter_email'
  | 'created_at'
  | 'updated_at'
  | 'comment_count'
  | 'reopen_count'
  | 'duedate'
  | 'started_at'
  | 'status_changed_at'
>

export interface Me {
  email: string | null
  account_id: string | null
  name: string | null
}

/**
 * GET `bootstrap/` — the four fields the phone drinks. `flow` is the learned
 * stale threshold (p85 cycle time), sent only when the workspace has a
 * distribution to learn from and no threshold is set — the server owns that
 * precedence; absent → the row age falls back to the shared default
 * (GDK-1495 ②). The session-strip boundary is not one of these: it rides
 * the X-Gadak-Session-Boundary response header, surfaced by lib/api as
 * `sessionBoundary` (the 0.21 body field is gone since 0.22, GDK-1548).
 * `issues` is re-typed to the narrowed row above — a Pick keeps the owner's
 * array element whole, and the phone's literals are narrower than that.
 */
export type BootstrapResponse = Pick<
  WebBootstrapResponse,
  'server_time' | 'sync_version' | 'flow'
> & {
  issues: IssueLite[]
}

/** One comment under an issue — `raw_body` is the ADF (GDK-1497), always
 *  present as an object or null; `body` is the plain-text fallback. */
export type DetailComment = Pick<
  WebDetailComment,
  'comment_id' | 'author' | 'created_at' | 'raw_body' | 'body'
>

/** One linked issue — the owner's shape as-is: the far side's chip rides
 *  `status_category`, empty when the target is outside the mirror. */
export type LinkedIssue = WebLinkedIssue

/**
 * GET `<key>/detail/` — the phone's slice. `description_md` is the format
 * the editor writes back (GDK-1497 A2; older serves predate it and the
 * editor falls back to `description_text`); `attachments` resolve the ADF's
 * media nodes; `history` is what the resume card counts (GDK-1495 ③); the
 * two visit fields are the serve's local.db person reads — absent, never
 * zero, when the issue was never opened in an app, and on a phone-only
 * workspace both stay absent so no card renders. `comments` is re-typed to
 * the narrowed row above, for the same reason as bootstrap's `issues`.
 */
export type DetailResponse = Pick<
  WebDetailResponse,
  | 'issue_key'
  | 'description_adf'
  | 'description_md'
  | 'description_text'
  | 'attachments'
  | 'last_visited_at'
  | 'previous_visit_at'
> & {
  comments: DetailComment[]
  linked_issues: LinkedIssue[]
  history: HistoryEntry[]
}

export interface TransitionDoc {
  id: string
  name: string
  to_status: string
  to_id: string
  /** new | inprogress | done */
  to_category: string
  /** Required screen fields; when present the phone refuses (desktop job). */
  fields?: { id: string; name: string }[]
}

/** Which FTS column a search hit came from — `title`, `body`, or `comment`
 *  (title wins over body over comment when more than one column hits). */
export type SearchMatch = WebSearchMatch

export type SearchResponse = WebSearchResponse

/* ── GET issues/pages/ (internal/store/read.go PageLite / PageDetail) ── */

/** One mirrored wiki page, without body — the owner's shape as-is. Picker
 *  rows and search hits use this. */
export type PageLite = WebPageLite

/** One comment on a mirrored page — `body_adf` the ADF (GDK-1497),
 *  `body_text` the plain-text fallback. Pages have no comment ids. */
export type PageComment = WebPageComment

/** GET `pages/{key}/` — PageLite plus the flattened body and comments. */
export type PageDetail = WebPageDetail

export type PagesResponse = WebPagesResponse

/* ── GET issues/views/ (internal/server/personal.go handleGetViews) ── */

/**
 * A stored config as it arrives on the wire: the desktop writes a full
 * ViewConfig, but older rows (and hand-written ones) may be missing axes, so
 * every field is optional here and the filter code treats absent as empty.
 */
export interface ViewConfigDoc {
  filters?: Partial<import('../../../web/src/lib/view-config').ViewFilters>
  /** Grouping and sort. The phone keeps its own (priority sections) — DESIGN.md §5. */
  display?: unknown
}

/**
 * The wire forms of the two view rows, parameterized off the desktop's
 * generics (GDK-1132): the phone's only real difference from SavedView /
 * SourceView is `config`, which on the wire is the opaque doc above (or
 * null), not the parsed ViewConfig the desk holds after loading.
 */
export type SavedViewDoc = WebSavedView<ViewConfigDoc | null>
export type SourceViewDoc = WebSourceView<ViewConfigDoc | null>

/** GET `issues/views/` — the serve always carries both lists. */
export interface ViewsResponse {
  views: SavedViewDoc[]
  source: SourceViewDoc[]
}

/** Pairing metadata — never the token (that lives in the Keychain). */
export interface PairMeta {
  endpoint: string
  label: string
  expires_at: string
}

/* ── Writes (internal/server/write.go) — the subset the phone sends ── */

/** One site priority (GET `<key>/priorities/`). Names follow the account language. */
export interface PriorityDoc {
  id: string
  name: string
}

export interface PrioritiesResponse {
  priorities: PriorityDoc[]
}

/** One assignee candidate (GET `<key>/users/?q=`). */
export interface UserDoc {
  account_id: string
  display_name: string
  email: string | null
  avatar_url: string | null
  active: boolean
}

export interface UsersResponse {
  users: UserDoc[]
}

/** GET create-meta/ — projects whose issue types the phone may file under. */
export interface CreateMetaProject {
  key: string
  name: string
  /** The phone never asks for a type (the server resolves the default), so
   *  this only tells whether a project offers any non-subtask type at all. */
  issue_types: Array<{ id: string; name: string; subtask?: boolean }>
}

export interface CreateMetaResponse {
  projects: CreateMetaProject[]
}

/** POST create/ payload — summary required, everything else optional. */
export interface CreateIssuePayload {
  project_key?: string
  issue_type?: string
  summary: string
  description_text?: string
}

/** Every PUT/POST write answers with the issue as the origin now holds it. */
export interface IssueWriteResponse {
  issue: IssueLite
  /** Preserved nodes the save dropped (the phone does not warn yet). */
  dropped?: string[]
}
