package mcp

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/midagedev/scry/internal/store"
)

// Tool names match contracts/agent.md. Do not add tools without updating that
// contract: every extra tool is context the agent must read before acting.
const (
	toolQuery  = "scry_query"
	toolSearch = "scry_search"
	toolIssue  = "scry_issue"
	toolStatus = "scry_status"
)

// toolQueryDescription is long on purpose: agents without AGENTS.md only see
// this text, so the schema summary, the localization trap, and two working
// examples have to live here.
const toolQueryDescription = `Run a read-only SQL query against the local scry mirror (SQLite): Jira issues and Confluence wiki pages in one file.

Schema essentials:
- items: source-neutral spine — id, key, title, body_text, created_at, updated_at
- issues: Jira projection joined on issues.item_id = items.id — key, project_key,
  status, status_id, status_category (new|inprogress|done), priority, priority_rank,
  assignee, assignee_email, assignee_id, reporter, labels/components/fix_versions
  (JSON arrays; use json_each), parent_key (direct parent), epic_key (nearest epic
  ancestor — group/aggregate on this), hierarchy_level (1=epic, 0=standard,
  -1=sub-task), reopen_count (times an issue left done and came back; 0 is normal,
  >0 is the signal), reopened_at, status_changed_at, resolved_at, comment_count.
- issues_full: VIEW of issues plus summary (the item title) — prefer it whenever
  the answer needs a human-readable title, no join required.
- pages: Confluence projection joined on pages.item_id = items.id — space_key,
  parent_id (page tree), version, labels (JSON array), body_adf, excerpt
  (≤200-rune body preview). Page title and body_text live on items
  (items.kind = 'page'; issues are kind 'issue').
- comments, attachments, changelog, links: hang off items.id
- item_refs: text-derived cross-refs — item_id (source), target_kind
  ('issue'|'page'), target_key (issue key or page id string), via ('url'|'text').
  Page bodies → issue keys; issue bodies/comments → page ids. Target need not
  exist; join items on key+kind to filter live rows. Index on (target_kind, target_key).
- items_fts: FTS5 over titles, bodies, and comment text of BOTH kinds
  (WHERE items_fts MATCH 'term'; join items, then issues or pages by kind)
- sync_state: watermark, version, last_error, last_full_sync_at, schema_version

CRITICAL: filter on status_category / status_id / issue_type_id, never on display
names. Jira localizes status.name and issuetype.name per account, so
  WHERE status = 'In Progress'   -- WRONG: empty on a Korean account
  WHERE status_category = 'inprogress'  -- RIGHT: stable everywhere

Only SELECT or WITH is allowed. Results are capped (default 200 rows, hard max
1000; response also byte-capped). When truncated, the result says so — tighten
LIMIT or columns and retry. Never write to this database; writes go through Jira.

Examples:
1) Open work for someone, most urgent first:
  SELECT key, status, priority, summary FROM issues_full
  WHERE assignee_email = 'dana@example.com' AND status_category != 'done'
  ORDER BY priority_rank, updated_at DESC LIMIT 20;

2) Issues stuck in progress the longest:
  SELECT key, status, ROUND(julianday('now') - julianday(status_changed_at), 1) AS days
  FROM issues WHERE status_category = 'inprogress' ORDER BY days DESC LIMIT 20;

3) Which epic carries the most reopened work:
  SELECT epic_key, COUNT(*) AS reopened FROM issues
  WHERE reopen_count > 0 AND epic_key IS NOT NULL
  GROUP BY epic_key ORDER BY reopened DESC LIMIT 5;

4) Wiki pages about an area (join pages through items):
  SELECT it.title, p.space_key FROM items_fts f
  JOIN items it ON it.rowid = f.rowid
  JOIN pages p ON p.item_id = it.id
  WHERE items_fts MATCH 'billing' LIMIT 10;`

const toolSearchDescription = `Full-text search over issue and page titles, bodies, and comments (FTS5).
Returns matching issue keys (with summary/status), page PageLite rows, and matches
(key → {field: title|body|comment, snippet: plain text ~120 chars}), best match first.
Prefer scry_query for relational or aggregated questions; use this for free-text recall.`

const toolIssueDescription = `Fetch one issue by key with full detail: list fields plus description,
comments, attachments, changelog history, and linked issues.
Use when you need the whole conversation around a single key (e.g. NMB-140).`

const toolStatusDescription = `Return mirror freshness: watermark, version, last_error, last_full_sync_at,
schema_version, and row counts (issues, comments). Check this before acting on
answers that matter — a stalled watermark can mean a quiet project or a broken sync.`

// Tool is one entry in tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolDefinitions() []Tool {
	return []Tool{
		{
			Name:        toolQuery,
			Description: toolQueryDescription,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{
						"type":        "string",
						"description": "A single SELECT or WITH statement. Multi-statement and non-SELECT SQL are rejected.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Max rows to return (default 200, hard max 1000).",
						"minimum":     1,
						"maximum":     hardRowLimit,
					},
				},
				"required":             []string{"sql"},
				"additionalProperties": false,
			},
		},
		{
			Name:        toolSearch,
			Description: toolSearchDescription,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "Free-text search query (FTS5 syntax; plain phrases work).",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum matches (default 20, hard max 1000).",
						"minimum":     1,
						"maximum":     hardRowLimit,
					},
				},
				"required":             []string{"text"},
				"additionalProperties": false,
			},
		},
		{
			Name:        toolIssue,
			Description: toolIssueDescription,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "Issue key, e.g. NMB-140 (case-insensitive).",
					},
				},
				"required":             []string{"key"},
				"additionalProperties": false,
			},
		},
		{
			Name:        toolStatus,
			Description: toolStatusDescription,
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
	}
}

// callTool dispatches a tools/call. Failures that the agent can fix (bad SQL,
// unknown key) return isError content, not JSON-RPC errors. This is the only
// place that turns a Go error into (textResult(err.Error()), true).
func (s *Server) callTool(name string, args map[string]any) (content []contentItem, isError bool) {
	if err := s.ensureDB(); err != nil {
		return textResult(err.Error()), true
	}
	var (
		out []contentItem
		err error
	)
	switch name {
	case toolQuery:
		out, err = s.toolQuery(args)
	case toolSearch:
		out, err = s.toolSearch(args)
	case toolIssue:
		out, err = s.toolIssue(args)
	case toolStatus:
		out, err = s.toolStatus(args)
	default:
		// Unknown tool is a protocol-level invalid params (caller should list first).
		return textResult(fmt.Sprintf("unknown tool %q — use tools/list", name)), true
	}
	if err != nil {
		return textResult(err.Error()), true
	}
	return out, false
}

func (s *Server) toolQuery(args map[string]any) ([]contentItem, error) {
	sqlText, ok := stringArg(args, "sql")
	if !ok || strings.TrimSpace(sqlText) == "" {
		return nil, errors.New("scry_query requires {sql: string}")
	}
	limit := intArg(args, "limit", 0)
	res, err := runQuery(s.DBPath, sqlText, limit)
	if err != nil {
		return nil, err
	}
	return marshalResult(res)
}

func (s *Server) toolSearch(args map[string]any) ([]contentItem, error) {
	text, ok := stringArg(args, "text")
	if !ok || strings.TrimSpace(text) == "" {
		return nil, errors.New("scry_search requires {text: string}")
	}
	limit := intArg(args, "limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > hardRowLimit {
		limit = hardRowLimit
	}
	res, err := s.db.Search(text, limit)
	if err != nil {
		return nil, err
	}
	// Best match first: IssueLites order is not FTS rank, so re-order by key list.
	all, err := s.db.IssueLites()
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]store.IssueLite, len(all))
	for _, l := range all {
		byKey[l.IssueKey] = l
	}
	type hit struct {
		Key     string `json:"key"`
		Summary string `json:"summary"`
		Status  string `json:"status"`
	}
	hits := make([]hit, 0, len(res.Keys))
	for _, k := range res.Keys {
		if l, ok := byKey[k]; ok {
			hits = append(hits, hit{Key: l.IssueKey, Summary: l.Summary, Status: l.Status})
		} else {
			hits = append(hits, hit{Key: k})
		}
	}
	pages := res.Pages
	if pages == nil {
		pages = []store.PageLite{}
	}
	matches := res.Matches
	if matches == nil {
		matches = map[string]store.SearchMatch{}
	}
	return marshalResult(map[string]any{"total": res.Total, "issues": hits, "pages": pages, "matches": matches})
}

func (s *Server) toolIssue(args map[string]any) ([]contentItem, error) {
	key, ok := stringArg(args, "key")
	if !ok || strings.TrimSpace(key) == "" {
		return nil, errors.New("scry_issue requires {key: string}")
	}
	key = strings.ToUpper(strings.TrimSpace(key))
	d, err := s.db.Detail(key)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("%s is not in the mirror — check the key, or run `scry sync`", key)
	}
	if err != nil {
		return nil, err
	}
	all, err := s.db.IssueLites()
	if err != nil {
		return nil, err
	}
	var lite *store.IssueLite
	for i := range all {
		if all[i].IssueKey == key {
			lite = &all[i]
			break
		}
	}
	// Same shape as `scry issue --json`: list row plus flattened detail fields.
	body := map[string]any{"issue_key": d.IssueKey, "description_adf": d.DescriptionADF,
		"comments": d.Comments, "attachments": d.Attachments, "history": d.History,
		"linked_issues": d.LinkedIssues}
	if lite != nil {
		body["issue"] = *lite
	}
	return marshalResult(body)
}

func (s *Server) toolStatus(args map[string]any) ([]contentItem, error) {
	_ = args
	st := map[string]any{"profile": s.Profile}
	ss, err := s.db.SyncState("jira")
	if err != nil {
		return nil, err
	}
	st["watermark"] = ss.Watermark
	st["version"] = ss.Version
	st["schema_version"] = ss.SchemaVersion
	st["sync_count"] = ss.SyncCount
	if ss.FirstSyncAt != nil {
		st["first_sync_at"] = *ss.FirstSyncAt
	}
	if ss.LastFullSyncAt != nil {
		st["last_full_sync_at"] = *ss.LastFullSyncAt
	} else {
		st["last_full_sync_at"] = ""
	}
	if ss.LastError != nil && *ss.LastError != "" {
		st["last_error"] = *ss.LastError
	}
	if ss.SyncedAt != nil {
		st["synced_at"] = *ss.SyncedAt
	}
	// Counts match `scry status --json`.
	for name, q := range map[string]string{
		"issues":   "SELECT COUNT(*) FROM issues",
		"comments": "SELECT COUNT(*) FROM comments",
	} {
		var n int
		if err := countQuery(s.DBPath, q, &n); err == nil {
			st[name] = n
		}
	}
	return marshalResult(st)
}

func countQuery(dbPath, q string, n *int) error {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	return db.QueryRow(q).Scan(n)
}

func stringArg(args map[string]any, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	v, ok := args[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func intArg(args map[string]any, key string, def int) int {
	if args == nil {
		return def
	}
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return def
		}
		return int(i)
	default:
		return def
	}
}

func textResult(msg string) []contentItem {
	return []contentItem{{Type: "text", Text: msg}}
}

// marshalResult encodes v as JSON text content, enforcing the byte budget by
// dropping trailing rows when the payload is a queryResult.
func marshalResult(v any) ([]contentItem, error) {
	if qr, ok := v.(*queryResult); ok {
		return marshalQueryResult(qr)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b) > maxResultBytes {
		// Non-query payloads (detail/search/status) are still capped: return a
		// short note rather than blowing the agent context.
		return nil, fmt.Errorf("result exceeds %d bytes (%d); narrow the request (e.g. lower limit, or use scry_query for specific columns)", maxResultBytes, len(b))
	}
	return []contentItem{{Type: "text", Text: string(b)}}, nil
}

func marshalQueryResult(qr *queryResult) ([]contentItem, error) {
	for {
		b, err := json.MarshalIndent(qr, "", "  ")
		if err != nil {
			return nil, err
		}
		if len(b) <= maxResultBytes {
			return []contentItem{{Type: "text", Text: string(b)}}, nil
		}
		if len(qr.Rows) == 0 {
			return nil, fmt.Errorf("a single row exceeds the %d-byte response cap; select fewer columns", maxResultBytes)
		}
		// Drop the last half of remaining rows until it fits.
		cut := len(qr.Rows) / 2
		if cut < 1 {
			cut = 1
		}
		qr.Rows = qr.Rows[:len(qr.Rows)-cut]
		qr.Count = len(qr.Rows)
		qr.Truncated = true
		qr.TruncationReason = fmt.Sprintf("response exceeded %d bytes; rows dropped — select fewer columns or lower limit", maxResultBytes)
	}
}

// dbMissingMessage is the human guidance when the mirror file is absent.
func dbMissingMessage(path string) string {
	if _, err := os.Stat(path); err != nil {
		return fmt.Sprintf("no mirror at %s — run `scry init && scry sync` first", path)
	}
	return ""
}
