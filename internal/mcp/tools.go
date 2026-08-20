package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/uifocus"
)

// issueKeyPat is the same positional key shape as cmd/gadak/views.go
// (`^[A-Z][A-Z0-9]*-\d+$`, compared after ToUpper). Copied because extracting
// it would require editing views.go, which this track cannot touch.
var issueKeyPat = regexp.MustCompile(`^[A-Z][A-Z0-9]*-\d+$`)

// Tool names match contracts/agent.md. Do not add tools without updating that
// contract: every extra tool is context the agent must read before acting.
const (
	toolQuery  = "gadak_query"
	toolSearch = "gadak_search"
	toolIssue  = "gadak_issue"
	toolStatus = "gadak_status"
	toolShow   = "gadak_show"
)

// toolQueryDescription is long on purpose: agents without AGENTS.md only see
// this text, so the schema summary, the localization trap, and two working
// examples have to live here.
const toolQueryDescription = `Default tool for anything countable, grouped, joined, historical, or derived (reopen_count, status_changed_at, epic_key).
Run a read-only SQL query against the local gadak mirror (SQLite): Jira issues and Confluence wiki pages in one file.

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
  (WHERE items_fts MATCH 'term'; join items, then issues or pages by kind).
  CJK mid-compound matches: '결제' hits '간편결제' (cjk_bigram column). English
  middles still miss ('ency' ≠ 'idempotency').
- sync_state: watermark, version, last_error, last_full_sync_at, schema_version

CRITICAL: filter on status_category / status_id / issue_type_id, never on display
names. Jira localizes status.name and issuetype.name per account, so
  WHERE status = 'In Progress'   -- WRONG: empty on a Korean account
  WHERE status_category = 'inprogress'  -- RIGHT: stable everywhere

Only SELECT or WITH is allowed. Results are capped (default 200 rows, hard max
1000; response also byte-capped). When truncated, the result says so — tighten
LIMIT or columns and retry. Never write to this database; writes go through the origin (Jira, another machine's serve, or the built-in standalone tracker).

Examples:
1) Open work for someone, most urgent first:
  SELECT key, status, priority, summary FROM issues_full
  WHERE assignee_id = 'account-id-from-the-mirror' AND status_category != 'done'
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

const toolSearchDescription = `Find issues and wiki pages by a recalled phrase (FTS5 over titles, bodies, comments of both).

Use ONLY when the user does not have keys yet and is remembering wording
("the ticket about webhook retry"). Argument: {query: string, limit?: number}.
Aliases: text, q.

CJK mid-compound matches: '결제' hits '간편결제' (cjk_bigram column). English
middles still miss ('ency' ≠ 'idempotency').

Do NOT use this for counts, grouping, "who is loaded", "what is stuck",
"what was reopened", time windows, or epic rollups. Those are gadak_query.

Returns {total, issues:[{key,summary,status}], pages, matches}.
Then hydrate one key with gadak_issue, or present keys with gadak_show.`

const toolIssueDescription = `Fetch one issue by key with full detail: list fields plus description,
comments, attachments, changelog history, and linked issues.
Use when you need the whole conversation around a single key (e.g. NMB-140).`

const toolStatusDescription = `Return mirror freshness: watermark, version, last_error, last_full_sync_at,
schema_version, row counts (issues, comments), and the workspace kind
(connected|standalone) with its origin. A paired workspace is kind connected
plus a pairing object (endpoint, label). Check this before acting on
answers that matter — a stalled watermark can mean a quiet project or a broken sync.`

// toolShowDescription is the presentation tool: it writes ui-focus and returns
// no issue rows. Numbers are from the running UI (500 ms poll) and uifocus.MaxAge.
const toolShowDescription = `Focus the running gadak app or serve tab on a view. This tool only presents — it does not return issue data or answers. The running gadak window picks the request up within 500 ms while visible (2 minute TTL); gadak_show does not open a window itself.

Pass exactly one of: jql (documented JQL subset or navigator URL), keys (issue keys, given order, cap 500), issue (one key's detail), or name (stored or synced view; same lookup as gadak views open <name>). SQL answers; show presents.

Writes a local ui-focus file for this process profile and returns {hash, applied, unsupported, file}. Does not write to the mirror or to Jira.`

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
					"query": map[string]any{
						"type":        "string",
						"description": "Free-text search query (FTS5 syntax; plain phrases work).",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "Alias of query.",
					},
					"q": map[string]any{
						"type":        "string",
						"description": "Alias of query.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum matches (default 20, hard max 1000).",
						"minimum":     1,
						"maximum":     hardRowLimit,
					},
				},
				"required":             []string{},
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
		{
			Name:        toolShow,
			Description: toolShowDescription,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"jql": map[string]any{
						"type":        "string",
						"description": "JQL subset or navigator URL. Unsupported clauses are listed and not applied.",
					},
					"keys": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"maxItems":    jql.MaxKeys,
						"description": "Issue keys in given order (cap 500). Writes ks= comma-joined.",
					},
					"issue": map[string]any{
						"type":        "string",
						"description": "Issue key to focus in detail (e.g. NMB-140). Writes issue=KEY.",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Saved view or synced Jira filter name (same lookup as gadak views open <name>).",
					},
				},
				"additionalProperties": false,
			},
		},
	}
}

// callTool dispatches a tools/call. Failures that the agent can fix (bad SQL,
// unknown key) return isError content, not JSON-RPC errors. This is the only
// place that turns a Go error into isError text; withErrorPrefix owns the
// ERROR: token so models cannot mistake a failure for an empty result.
func (s *Server) callTool(name string, args map[string]any) (content []contentItem, isError bool) {
	// gadak_show keys/issue/jql do not read the mirror (same as views open --keys / KEY / --jql).
	// Name lookup opens it inside toolShow. Other tools still require the file.
	if name != toolShow {
		if err := s.ensureDB(); err != nil {
			return textResult(withErrorPrefix(err.Error())), true
		}
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
	case toolShow:
		out, err = s.toolShow(args)
	default:
		// Unknown tool is a protocol-level invalid params (caller should list first).
		return textResult(withErrorPrefix(fmt.Sprintf("unknown tool %q — use tools/list", name))), true
	}
	if err != nil {
		return textResult(withErrorPrefix(err.Error())), true
	}
	return out, false
}

func (s *Server) toolQuery(args map[string]any) ([]contentItem, error) {
	sqlText, ok := stringArg(args, "sql")
	if !ok || strings.TrimSpace(sqlText) == "" {
		return nil, errors.New("gadak_query requires {sql: string}")
	}
	limit := intArg(args, "limit", 0)
	res, err := runQuery(s.DBPath, sqlText, limit)
	if err != nil {
		return nil, err
	}
	return s.marshalResult(res)
}

func (s *Server) toolSearch(args map[string]any) ([]contentItem, error) {
	text, via, ok := searchQueryArg(args)
	if !ok {
		return nil, fmt.Errorf("ERROR: gadak_search missing query string. You sent argument keys: [%s].\nRetry with {query: string, limit?: number}. Aliases accepted: text, q.\nThis is not an empty search result.", receivedArgKeys(args))
	}
	if via == "query" {
		Logf("search arg via %q", via)
	} else {
		Logf("search arg via alias %q", via)
	}
	limit := intArg(args, "limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > hardRowLimit {
		limit = hardRowLimit
	}
	res, err := s.db.Search(context.Background(), text, limit)
	if err != nil {
		return nil, err
	}
	// Best match first: IssueLitesByKeys returns rows in the ranked key order.
	// Missing keys (FTS hit without an issues row) still emit {key} below.
	lites, err := s.db.IssueLitesByKeys(context.Background(), res.Keys)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]store.IssueLite, len(lites))
	for _, l := range lites {
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
	return s.marshalResult(map[string]any{"total": res.Total, "issues": hits, "pages": pages, "matches": matches})
}

func (s *Server) toolIssue(args map[string]any) ([]contentItem, error) {
	key, ok := stringArg(args, "key")
	if !ok || strings.TrimSpace(key) == "" {
		return nil, errors.New("gadak_issue requires {key: string}")
	}
	key = strings.ToUpper(strings.TrimSpace(key))
	d, err := s.db.Detail(context.Background(), key)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("%s is not in the mirror — check the key, or run `gadak sync`", key)
	}
	if err != nil {
		return nil, err
	}
	lites, err := s.db.IssueLitesByKeys(context.Background(), []string{key})
	if err != nil {
		return nil, err
	}
	var lite *store.IssueLite
	if len(lites) > 0 {
		lite = &lites[0]
	}
	// Same shape as `gadak issue --json`: list row plus flattened detail fields.
	body := map[string]any{"issue_key": d.IssueKey, "description_adf": d.DescriptionADF,
		"comments": d.Comments, "attachments": d.Attachments, "history": d.History,
		"linked_issues": d.LinkedIssues}
	if lite != nil {
		body["issue"] = *lite
	}
	return s.marshalIssueResult(body)
}

func (s *Server) toolStatus(args map[string]any) ([]contentItem, error) {
	_ = args
	st := map[string]any{"profile": s.Profile}
	// A shell-less host must be able to tell standalone from connected
	// (GDK-420); origin.Describe is the single owner of that verdict.
	if cfg, err := config.LoadFor(s.Profile); err == nil {
		kind, originDesc := origin.Describe(cfg)
		st["kind"] = kind
		st["origin"] = originDesc
		// Same pairing object as `gadak status --json` (origin.PairedStatus).
		if rem, err := origin.PairedStatus(cfg); err != nil {
			st["pairing_error"] = err.Error()
		} else if rem != nil {
			st["pairing"] = map[string]string{
				"endpoint": rem.Endpoint,
				"label":    rem.Label,
			}
		}
	}
	ss, err := s.db.SyncState(context.Background(), "jira")
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
	// Counts match `gadak status --json`.
	for name, q := range map[string]string{
		"issues":   "SELECT COUNT(*) FROM issues",
		"comments": "SELECT COUNT(*) FROM comments",
	} {
		var n int
		if err := countQuery(s.DBPath, q, &n); err == nil {
			st[name] = n
		}
	}
	return s.marshalResult(st)
}

func (s *Server) toolShow(args map[string]any) ([]contentItem, error) {
	which, err := showInputs(args)
	if err != nil {
		return nil, err
	}
	var (
		hash        string
		applied     []string
		unsupported []string
	)
	switch which {
	case "jql":
		text, _ := stringArg(args, "jql")
		hash, applied, unsupported, err = s.hashFromJQL(strings.TrimSpace(text))
		if err != nil {
			return nil, err
		}
	case "keys":
		raw, _, kerr := stringSliceArg(args, "keys")
		if kerr != nil {
			return nil, kerr
		}
		keys := jql.SplitKeys(strings.Join(raw, ","))
		if err := jql.CheckKeyLimit(len(keys)); err != nil {
			return nil, err
		}
		if len(keys) == 0 {
			return nil, errors.New("gadak_show keys is empty")
		}
		f := jql.EmptyFilter()
		f.Keys = keys
		hash = jql.Hash(f, jql.Display{})
		applied = []string{"keys"}
	case "issue":
		raw, _ := stringArg(args, "issue")
		k := strings.ToUpper(strings.TrimSpace(raw))
		if !issueKeyPat.MatchString(k) {
			return nil, fmt.Errorf("gadak_show issue %q is not a Jira key (want ABC-123)", raw)
		}
		hash = "issue=" + k
		applied = []string{"issue"}
	case "name":
		name, _ := stringArg(args, "name")
		name = strings.TrimSpace(name)
		if err := s.ensureDB(); err != nil {
			return nil, err
		}
		v, err := findView(s.db, name)
		if err != nil {
			return nil, err
		}
		if v.Hash == "" {
			return nil, fmt.Errorf("view %q has no gadak hash — nothing to focus", v.Name)
		}
		hash, applied, unsupported = v.Hash, v.Applied, v.Unsupported
	default:
		return nil, errors.New("gadak_show requires exactly one of jql, keys, issue, or name")
	}
	if applied == nil {
		applied = []string{}
	}
	if unsupported == nil {
		unsupported = []string{}
	}
	if err := uifocus.WriteFor(s.Profile, hash); err != nil {
		return nil, err
	}
	path, err := uifocus.PathFor(s.Profile)
	if err != nil {
		return nil, err
	}
	return s.marshalResult(map[string]any{
		"hash":        hash,
		"applied":     applied,
		"unsupported": unsupported,
		"file":        path,
	})
}

// showInputs returns the single provided axis, or an error when zero or several
// of jql/keys/issue/name are set. Empty strings do not count; a present keys
// array does (even if empty — that fails later as "keys is empty").
func showInputs(args map[string]any) (string, error) {
	var fields []string
	if s, ok := stringArg(args, "jql"); ok && strings.TrimSpace(s) != "" {
		fields = append(fields, "jql")
	}
	if _, present, err := stringSliceArg(args, "keys"); err != nil {
		return "", err
	} else if present {
		fields = append(fields, "keys")
	}
	if s, ok := stringArg(args, "issue"); ok && strings.TrimSpace(s) != "" {
		fields = append(fields, "issue")
	}
	if s, ok := stringArg(args, "name"); ok && strings.TrimSpace(s) != "" {
		fields = append(fields, "name")
	}
	if len(fields) != 1 {
		if len(fields) == 0 {
			return "", errors.New("gadak_show requires exactly one of jql, keys, issue, or name")
		}
		return "", fmt.Errorf("gadak_show requires exactly one of jql, keys, issue, or name (got %s)", strings.Join(fields, ", "))
	}
	return fields[0], nil
}

// hashFromJQL matches cmd/gadak/views.go hashFromJQL: parse, resolve identity
// with a nil roster (views open --jql does not load people), return Hash.
// Applied/unsupported are returned instead of being printed to stderr.
func (s *Server) hashFromJQL(text string) (hash string, applied, unsupported []string, err error) {
	cfg, loadErr := config.LoadFor(s.Profile)
	me := jql.Identity{}
	if loadErr == nil {
		me = jql.Identity{Email: cfg.Email, AccountID: cfg.AccountID}
	}
	parsed := jql.Parse(text, jql.Opts{Email: me.Email, AccountID: me.AccountID})
	if parsed.Error != "" {
		return "", nil, nil, fmt.Errorf("jql: %s", parsed.Message)
	}
	jql.ResolveIdentity(&parsed, nil, me)
	return jql.Hash(parsed.Filters, parsed.Display), parsed.Applied, parsed.Unsupported, nil
}

// listedView and findView are a copy of cmd/gadak/views.go loadViews/findView/
// hashFromConfig (including the id-suffix and substring match). Extracting
// would require editing views.go, which this track cannot touch.
type listedView struct {
	Kind        string
	ID          string
	Name        string
	Hash        string
	Applied     []string
	Unsupported []string
}

func loadViews(db *store.DB) ([]listedView, error) {
	var out []listedView
	src, err := db.SourceQueries(context.Background(), "jira")
	if err != nil {
		return nil, err
	}
	for _, q := range src {
		out = append(out, listedView{
			Kind: "jira", ID: q.ID, Name: q.Name,
			Hash: hashFromConfig(q.Config), Applied: q.Applied, Unsupported: q.Unsupported,
		})
	}
	saved, err := db.SavedViews(context.Background())
	if err != nil {
		return nil, err
	}
	for _, s := range saved {
		out = append(out, listedView{
			Kind: "saved", ID: s.ID, Name: s.Name,
			Hash: hashFromConfig(s.Config),
		})
	}
	return out, nil
}

func findView(db *store.DB, name string) (listedView, error) {
	list, err := loadViews(db)
	if err != nil {
		return listedView{}, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	var exact, sub []listedView
	for _, v := range list {
		id := strings.ToLower(v.ID)
		nm := strings.ToLower(v.Name)
		ext := ""
		if i := strings.LastIndex(v.ID, ":"); i >= 0 {
			ext = strings.ToLower(v.ID[i+1:])
		}
		if id == want || nm == want || ext == want {
			exact = append(exact, v)
			continue
		}
		if strings.Contains(nm, want) || strings.Contains(id, want) {
			sub = append(sub, v)
		}
	}
	hits := exact
	if len(hits) == 0 {
		hits = sub
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return listedView{}, fmt.Errorf("no view matching %q — run `gadak views` (sync first if you expected Jira filters)", name)
	default:
		names := make([]string, len(hits))
		for i, h := range hits {
			names[i] = h.Name
		}
		return listedView{}, fmt.Errorf("%q matches %d views — be more specific: %s", name, len(hits), strings.Join(names, "; "))
	}
}

func hashFromConfig(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var c struct {
		Filters jql.Filter  `json:"filters"`
		Display jql.Display `json:"display"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return ""
	}
	return jql.Hash(c.Filters, c.Display)
}

func countQuery(dbPath, q string, n *int) error {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	return db.QueryRow(q).Scan(n)
}

func stringSliceArg(args map[string]any, key string) (vals []string, present bool, err error) {
	if args == nil {
		return nil, false, nil
	}
	v, ok := args[key]
	if !ok || v == nil {
		return nil, false, nil
	}
	switch t := v.(type) {
	case []string:
		return t, true, nil
	case []any:
		out := make([]string, 0, len(t))
		for i, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, true, fmt.Errorf("gadak_show keys[%d] must be a string", i)
			}
			out = append(out, s)
		}
		return out, true, nil
	default:
		return nil, true, errors.New("gadak_show keys must be an array of strings")
	}
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

// searchQueryArg picks the first non-empty string among query, text, q.
func searchQueryArg(args map[string]any) (text, via string, ok bool) {
	for _, key := range []string{"query", "text", "q"} {
		s, present := stringArg(args, key)
		if present && strings.TrimSpace(s) != "" {
			return s, key, true
		}
	}
	return "", "", false
}

func receivedArgKeys(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
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

// withErrorPrefix is the single owner of the isError body prefix so a model
// cannot mistake a tool failure for an empty result.
func withErrorPrefix(msg string) string {
	if strings.HasPrefix(msg, "ERROR:") {
		return msg
	}
	return "ERROR: " + msg
}

func (s *Server) resultCap() int {
	if s != nil && s.resultByteCap > 0 {
		return s.resultByteCap
	}
	return maxResultBytes
}

// marshalResult encodes v as JSON text content, enforcing the byte budget by
// dropping trailing rows when the payload is a queryResult.
func (s *Server) marshalResult(v any) ([]contentItem, error) {
	capn := s.resultCap()
	if qr, ok := v.(*queryResult); ok {
		return marshalQueryResult(qr, capn)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b) > capn {
		// Non-query payloads (detail/search/status) are still capped: return a
		// short note rather than blowing the agent context.
		return nil, fmt.Errorf("result exceeds %d bytes (%d); narrow the request (e.g. lower limit, or use gadak_query for specific columns)", capn, len(b))
	}
	return []contentItem{{Type: "text", Text: string(b)}}, nil
}

// marshalIssueResult shrinks comments (newest kept) when the payload exceeds
// the byte cap. Search stays on marshalResult (its payload is already limited).
func (s *Server) marshalIssueResult(body map[string]any) ([]contentItem, error) {
	capn := s.resultCap()
	comments := issueComments(body)
	original := len(comments)
	for {
		b, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return nil, err
		}
		if len(b) <= capn {
			return []contentItem{{Type: "text", Text: string(b)}}, nil
		}
		if len(comments) == 0 {
			return nil, fmt.Errorf("result exceeds %d bytes (%d); narrow the request (e.g. lower limit, or use gadak_query for specific columns)", capn, len(b))
		}
		// Detail comments are oldest-first (created_at, id). Drop the oldest half.
		cut := len(comments) / 2
		if cut < 1 {
			cut = 1
		}
		comments = comments[cut:]
		body["comments"] = comments
		body["truncated"] = true
		body["comments_omitted"] = original - len(comments)
	}
}

func issueComments(body map[string]any) []store.DetailComment {
	if body == nil {
		return nil
	}
	c, ok := body["comments"].([]store.DetailComment)
	if !ok {
		return nil
	}
	return c
}

func marshalQueryResult(qr *queryResult, capn int) ([]contentItem, error) {
	for {
		b, err := json.MarshalIndent(qr, "", "  ")
		if err != nil {
			return nil, err
		}
		if len(b) <= capn {
			return []contentItem{{Type: "text", Text: string(b)}}, nil
		}
		if len(qr.Rows) == 0 {
			return nil, fmt.Errorf("a single row exceeds the %d-byte response cap; select fewer columns", capn)
		}
		// Drop the last half of remaining rows until it fits.
		cut := len(qr.Rows) / 2
		if cut < 1 {
			cut = 1
		}
		qr.Rows = qr.Rows[:len(qr.Rows)-cut]
		qr.Count = len(qr.Rows)
		qr.Truncated = true
		qr.TruncationReason = fmt.Sprintf("response exceeded %d bytes; rows dropped — select fewer columns or lower limit", capn)
	}
}

// dbMissingMessage is the human guidance when the mirror file is absent.
func dbMissingMessage(path string) string {
	if _, err := os.Stat(path); err != nil {
		return fmt.Sprintf("no mirror at %s — run `gadak init && gadak sync` first", path)
	}
	return ""
}
