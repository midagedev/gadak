// Wire shapes the companion reads. Field names are the server's
// (internal/server/read.go, internal/store/read.go) — a subset: the phone
// only parses what it paints. Adding fields is safe, renaming is not.

export interface IssueLite {
  issue_key: string
  summary: string
  project_key: string
  issue_type: string
  status: string
  status_id: string
  /** new | inprogress | done — the only status axis logic may key on. */
  status_category: string
  priority: string | null
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

/** Pairing metadata — never the token (that lives in the Keychain). */
export interface PairMeta {
  endpoint: string
  label: string
  expires_at: string
}
