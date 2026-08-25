// Wire shapes the companion reads. Field names are the server's
// (internal/server/read.go, internal/store/read.go) — a subset: the phone
// only parses what it paints. Adding fields is safe, renaming is not.

/**
 * The desktop's saved-view schema, imported as a type so there is exactly one
 * definition of what a stored view means (web/src/lib/view-config.ts). This is
 * an `import type`: it is erased at build time, so the phone bundle does not
 * pull in the desktop's config/feature-flag modules.
 */
export type { ViewConfig, ViewFilters } from '../../../web/src/lib/view-config'

export interface IssueLite {
  issue_key: string
  summary: string
  project_key: string
  issue_type: string
  /**
   * Stable Jira issue-type id. Empty on rows a sync has not rewritten since
   * the column was added; view filters fall back to the stored name then,
   * exactly as the desktop's matchesIdFirst does.
   */
  issue_type_id: string
  status: string
  status_id: string
  /** new | inprogress | done — the only status axis logic may key on. */
  status_category: string
  priority: string | null
  /** Stable Jira priority id; same empty-on-old-rows contract as issue_type_id. */
  priority_id: string
  /** Stable sort axis; display names never drive logic. 0 = unranked. */
  priority_rank: number
  assignee: string | null
  assignee_id: string | null
  assignee_email: string | null
  reporter: string | null
  created_at: string | null
  updated_at: string | null
  comment_count: number
  reopen_count: number
  duedate: string | null
}

export interface Me {
  email: string | null
  account_id: string | null
  name: string | null
}

export interface BootstrapResponse {
  server_time: string
  sync_version: number
  issues: IssueLite[]
}

export interface DetailComment {
  comment_id: string
  author: string | null
  created_at: string | null
  /** Plain-text fallback body — the phone renders text, never raw ADF. */
  body: string
}

export interface LinkedIssue {
  key: string
  type: string
  direction: string
  summary: string | null
  status_category: string | null
}

export interface DetailResponse {
  issue_key: string
  description_text?: string
  comments: DetailComment[]
  linked_issues: LinkedIssue[]
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

export interface SearchResponse {
  keys: string[]
  total: number
}

/* ── GET issues/views/ (internal/server/personal.go handleGetViews) ── */

/** A view the developer saved at the desk. `config` is an opaque ViewConfig. */
export interface SavedViewDoc {
  id: string
  name: string
  owner_email: string | null
  owner_name: string | null
  config: ViewConfigDoc | null
  created_at: string | null
  updated_at: string | null
}

/** A Jira filter imported at the desk, already compiled into a ViewConfig. */
export interface SourceViewDoc {
  id: string
  name: string
  config: ViewConfigDoc | null
  jql: string
  external_id?: string
  favourite: boolean
  owner?: string
  /** JQL clauses the desktop's importer could honor. */
  applied: string[]
  /** JQL clauses it could not — a non-empty list means "open on the desktop". */
  unsupported: string[]
}

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
