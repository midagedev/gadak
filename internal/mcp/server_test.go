package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/uifocus"
)

// demoDB copies examples/demo.db into a temp dir so tests never touch the tree.
func demoDB(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "examples", "demo.db")
	in, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read demo.db: %v (run tests from module root or with package path that resolves examples/)", err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gadak.db")
	if err := os.WriteFile(dst, in, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}
	return dst
}

// session drives a full stdio-style exchange against an in-memory pair of pipes.
func session(t *testing.T, dbPath string, frames ...string) []rpcResponse {
	t.Helper()
	return sessionProfile(t, dbPath, "", frames...)
}

func sessionProfile(t *testing.T, dbPath, profile string, frames ...string) []rpcResponse {
	t.Helper()
	if os.Getenv("GADAK_HOME") == "" {
		t.Setenv("GADAK_HOME", t.TempDir())
	}
	in := strings.Join(frames, "\n") + "\n"
	var out bytes.Buffer
	srv := New(dbPath, profile, "test")
	if err := srv.Serve(strings.NewReader(in), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	dec := json.NewDecoder(&out)
	var resps []rpcResponse
	for {
		var r rpcResponse
		if err := dec.Decode(&r); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode response: %v\nstdout was:\n%s", err, out.String())
		}
		resps = append(resps, r)
	}
	return resps
}

func TestProtocolRoundTrip(t *testing.T) {
	db := demoDB(t)
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"gadak_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"gadak_search","arguments":{"text":"upload","limit":3}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"gadak_query","arguments":{"sql":"SELECT key, status_category FROM issues LIMIT 2","limit":2}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"gadak_issue","arguments":{"key":"NMA-1"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"gadak_show","arguments":{"keys":["NMA-1","NMA-2"]}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"ping"}`,
	)
	// notifications/initialized produces no response → 8 responses for 9 frames.
	if len(resps) != 8 {
		t.Fatalf("got %d responses, want 8", len(resps))
	}

	// initialize: speak our version even when client asked for an older one.
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
		Capabilities map[string]any `json:"capabilities"`
	}
	mustResult(t, resps[0], &init)
	if init.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocolVersion = %q, want %q", init.ProtocolVersion, ProtocolVersion)
	}
	if init.ServerInfo.Name != "gadak" {
		t.Errorf("serverInfo.name = %q", init.ServerInfo.Name)
	}
	if _, ok := init.Capabilities["tools"]; !ok {
		t.Error("capabilities.tools missing")
	}

	// tools/list: exactly the five contracted tools.
	var list struct {
		Tools []Tool `json:"tools"`
	}
	mustResult(t, resps[1], &list)
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("tool %s has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %s has nil inputSchema", tool.Name)
		}
	}
	for _, want := range []string{toolQuery, toolSearch, toolIssue, toolStatus, toolShow} {
		if !names[want] {
			t.Errorf("tools/list missing %s", want)
		}
	}
	if len(list.Tools) != 5 {
		t.Errorf("tools/list has %d tools, want 5", len(list.Tools))
	}
	// gadak_query description must carry the localization warning and examples.
	var qdesc string
	for _, tool := range list.Tools {
		if tool.Name == toolQuery {
			qdesc = tool.Description
		}
	}
	for _, needle := range []string{"status_category", "In Progress", "SELECT", "inprogress", "description_text"} {
		if !strings.Contains(qdesc, needle) {
			t.Errorf("gadak_query description missing %q", needle)
		}
	}
	var sdesc string
	for _, tool := range list.Tools {
		if tool.Name == toolStatus {
			sdesc = tool.Description
		}
	}
	for _, needle := range []string{"paired", "connected", "pairing", "custom_fields", "frozen"} {
		if !strings.Contains(sdesc, needle) {
			t.Errorf("gadak_status description missing %q", needle)
		}
	}

	// gadak_status
	statusText := callText(t, resps[2])
	var status map[string]any
	if err := json.Unmarshal([]byte(statusText), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, statusText)
	}
	if _, ok := status["issues"]; !ok {
		t.Errorf("status missing issues: %v", status)
	}
	if _, ok := status["watermark"]; !ok {
		t.Errorf("status missing watermark: %v", status)
	}
	if _, ok := status["custom_fields"]; !ok {
		t.Errorf("status missing custom_fields: %v", status)
	}

	// gadak_search
	searchText := callText(t, resps[3])
	var search struct {
		Total  int `json:"total"`
		Issues []struct {
			Key     string `json:"key"`
			Summary string `json:"summary"`
			Status  string `json:"status"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(searchText), &search); err != nil {
		t.Fatalf("search JSON: %v\n%s", err, searchText)
	}
	if search.Total < 0 {
		t.Errorf("search total negative")
	}
	// demo.db has real data; "upload" may or may not hit — shape is what we assert.
	for _, h := range search.Issues {
		if h.Key == "" {
			t.Error("search hit missing key")
		}
	}

	// gadak_query
	queryText := callText(t, resps[4])
	var qr queryResult
	if err := json.Unmarshal([]byte(queryText), &qr); err != nil {
		t.Fatalf("query JSON: %v\n%s", err, queryText)
	}
	if len(qr.Columns) < 2 {
		t.Errorf("query columns = %v", qr.Columns)
	}
	if qr.Count == 0 || len(qr.Rows) == 0 {
		t.Errorf("query returned no rows: %+v", qr)
	}

	// gadak_issue
	issueText := callText(t, resps[5])
	var issue map[string]any
	if err := json.Unmarshal([]byte(issueText), &issue); err != nil {
		t.Fatalf("issue JSON: %v\n%s", err, issueText)
	}
	if issue["issue_key"] != "NMA-1" {
		t.Errorf("issue_key = %v", issue["issue_key"])
	}
	if issue["key"] != issue["issue_key"] {
		t.Errorf("key alias diverged: issue_key=%v key=%v", issue["issue_key"], issue["key"])
	}
	if nested, ok := issue["issue"].(map[string]any); ok {
		if nested["key"] != nested["issue_key"] {
			t.Errorf("nested IssueLite key alias diverged: %v", nested)
		}
	}
	if _, ok := issue["comments"]; !ok {
		t.Error("issue missing comments")
	}
	if _, ok := issue["issue"]; !ok {
		t.Error("issue missing list row (IssueLite)")
	}
	if _, ok := issue["history"]; !ok {
		t.Error("issue missing history")
	}
	if _, ok := issue["description_text"]; !ok {
		t.Error("issue missing description_text")
	}
	if _, ok := issue["dev_links"]; !ok {
		t.Error("issue missing dev_links")
	}

	var idesc string
	for _, tool := range list.Tools {
		if tool.Name == toolIssue {
			idesc = tool.Description
		}
	}
	for _, needle := range []string{"description_text", "keys"} {
		if !strings.Contains(idesc, needle) {
			t.Errorf("gadak_issue description missing %q", needle)
		}
	}

	// gadak_show
	var showDesc string
	for _, tool := range list.Tools {
		if tool.Name == toolShow {
			showDesc = tool.Description
		}
	}
	for _, needle := range []string{"500 ms", "2 minute", "does not return", "does not open"} {
		if !strings.Contains(showDesc, needle) {
			t.Errorf("gadak_show description missing %q", needle)
		}
	}
	showText := callText(t, resps[6])
	var show struct {
		Hash        string   `json:"hash"`
		Applied     []string `json:"applied"`
		Unsupported []string `json:"unsupported"`
		File        string   `json:"file"`
	}
	if err := json.Unmarshal([]byte(showText), &show); err != nil {
		t.Fatalf("show JSON: %v\n%s", err, showText)
	}
	if show.Hash != "ks=NMA-1,NMA-2" {
		t.Errorf("show hash = %q, want ks=NMA-1,NMA-2", show.Hash)
	}
	if show.File == "" {
		t.Error("show missing focus file path")
	}

	// ping
	if resps[7].Error != nil {
		t.Errorf("ping error: %+v", resps[7].Error)
	}
}

func TestWriteSQLRejected(t *testing.T) {
	db := demoDB(t)
	for _, sql := range []string{
		`INSERT INTO issues (key) VALUES ('X')`,
		`UPDATE issues SET status = 'x'`,
		`DELETE FROM issues`,
		`DROP TABLE issues`,
		`PRAGMA journal_mode`,
		`ATTACH DATABASE 'x.db' AS x`,
		`SELECT 1; DROP TABLE issues`,
	} {
		t.Run(sql, func(t *testing.T) {
			args, _ := json.Marshal(map[string]any{
				"name":      toolQuery,
				"arguments": map[string]any{"sql": sql},
			})
			resps := session(t, db,
				`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(args)+`}`,
			)
			if len(resps) != 1 {
				t.Fatalf("want 1 response, got %d", len(resps))
			}
			if resps[0].Error != nil {
				t.Fatalf("want tool-level error, got JSON-RPC: %+v", resps[0].Error)
			}
			var cr callResult
			raw, _ := json.Marshal(resps[0].Result)
			if err := json.Unmarshal(raw, &cr); err != nil {
				t.Fatal(err)
			}
			if !cr.IsError {
				t.Fatalf("expected isError for %q; content=%v", sql, cr.Content)
			}
			if len(cr.Content) == 0 || cr.Content[0].Text == "" {
				t.Fatal("empty error text")
			}
		})
	}
}

func TestRowLimitTruncation(t *testing.T) {
	db := demoDB(t)
	// demo.db has 519 issues; limit 3 must set truncated when more rows exist.
	args, _ := json.Marshal(map[string]any{
		"name": toolQuery,
		"arguments": map[string]any{
			"sql":   "SELECT key FROM issues",
			"limit": 3,
		},
	})
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(args)+`}`,
	)
	text := callText(t, resps[0])
	var qr queryResult
	if err := json.Unmarshal([]byte(text), &qr); err != nil {
		t.Fatal(err)
	}
	if qr.Count != 3 {
		t.Errorf("count = %d, want 3", qr.Count)
	}
	if !qr.Truncated {
		t.Error("expected truncated=true when more rows exist")
	}
	if qr.TruncationReason == "" {
		t.Error("expected truncation_reason")
	}
	if qr.RowLimit != 3 {
		t.Errorf("row_limit = %d", qr.RowLimit)
	}
}

func TestHardRowLimit(t *testing.T) {
	if got := clampLimit(5000); got != hardRowLimit {
		t.Errorf("clampLimit(5000) = %d, want %d", got, hardRowLimit)
	}
	if got := clampLimit(0); got != defaultRowLimit {
		t.Errorf("clampLimit(0) = %d, want %d", got, defaultRowLimit)
	}
}

func TestRejectNonSelectUnit(t *testing.T) {
	if err := rejectNonSelect("SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := rejectNonSelect("with x as (select 1) select * from x"); err != nil {
		t.Fatal(err)
	}
	if err := rejectNonSelect("-- comment\nSELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := rejectNonSelect("/* block */ SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := rejectNonSelect("INSERT INTO t VALUES (1)"); err == nil {
		t.Fatal("expected reject insert")
	}
	if err := rejectNonSelect("SELECT 1; SELECT 2"); err == nil {
		t.Fatal("expected reject multi-statement")
	}
}

// GDK-485: gadak_status must carry the same pairing object as `gadak status --json`.
func TestStatusSurfacesPairing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := pairing.SaveRemote(dir, pairing.Remote{
		Endpoint: "https://home.ts.net:8443", Token: "pair-token", Label: "laptop",
	}); err != nil {
		t.Fatal(err)
	}

	db := demoDB(t)
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_status","arguments":{}}}`,
	)
	statusText := callText(t, resps[0])
	var status map[string]any
	if err := json.Unmarshal([]byte(statusText), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, statusText)
	}
	if kind, _ := status["kind"].(string); kind != config.KindConnected {
		t.Fatalf("kind = %v, want connected (paired is not a new kind)", status["kind"])
	}
	p, _ := status["pairing"].(map[string]any)
	if p == nil {
		t.Fatalf("gadak_status missing pairing object: %s", statusText)
	}
	if p["endpoint"] != "https://home.ts.net:8443" || p["label"] != "laptop" {
		t.Fatalf("pairing = %+v", p)
	}
	if _, ok := p["token"]; ok {
		t.Fatal("pairing object leaked the device token")
	}
}

// GDK-526: gadak_status.schema_version is the live PRAGMA, not the lagging
// sync_state column. Other freshness fields still come from the row.
func TestStatusSchemaVersionMatchesLivePRAGMA(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	path := filepath.Join(home, "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	const watermark = "2026-01-15T12:00:00.000Z"
	if err := db.RecordSync(ctx, "jira", store.SyncResult{Watermark: watermark}); err != nil {
		t.Fatal(err)
	}
	live := db.SchemaVersion()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	stale := live - 5
	if stale < 1 {
		t.Fatalf("live schema %d is too low to plant a stale value", live)
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE sync_state SET schema_version = ?, last_error = ?, version = ?`, stale, "planted last_error", 77); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	resps := session(t, path,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_status","arguments":{}}}`,
	)
	statusText := callText(t, resps[0])
	var status map[string]any
	if err := json.Unmarshal([]byte(statusText), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, statusText)
	}
	got, _ := status["schema_version"].(float64)
	if int(got) != live {
		t.Fatalf("gadak_status schema_version = %v, want live PRAGMA %d (row planted at %d)", status["schema_version"], live, stale)
	}
	if status["watermark"] != watermark {
		t.Errorf("watermark = %v, want %q", status["watermark"], watermark)
	}
	if status["last_error"] != "planted last_error" {
		t.Errorf("last_error = %v", status["last_error"])
	}
	if status["version"] != float64(77) {
		t.Errorf("version = %v, want 77", status["version"])
	}
	synced, _ := status["synced_at"].(string)
	if synced == "" {
		t.Error("synced_at missing; must still come from the row")
	}
}

// GDK-522: gadak_status carries the same custom_fields object as status --json.
func TestStatusCustomFieldsMapped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{
		Fields: []config.FieldSpec{
			{Alias: "story_points", Label: "Story Points", IDs: []string{"customfield_1"}, Role: "plain"},
			{Alias: "severity", Label: "Severity", IDs: []string{"customfield_2"}, Role: "facet"},
		},
		FieldsAppliedAt: "2026-08-21T12:00:00.000Z",
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	db := demoDB(t)
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_status","arguments":{}}}`,
	)
	statusText := callText(t, resps[0])
	var status map[string]any
	if err := json.Unmarshal([]byte(statusText), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, statusText)
	}
	cf, ok := status["custom_fields"].(map[string]any)
	if !ok {
		t.Fatalf("gadak_status missing custom_fields object: %s", statusText)
	}
	if mapped, _ := cf["mapped"].(float64); int(mapped) != 2 {
		t.Fatalf("custom_fields.mapped = %v, want 2", cf["mapped"])
	}
	if at, _ := cf["applied_at"].(string); at != "2026-08-21T12:00:00.000Z" {
		t.Fatalf("custom_fields.applied_at = %q", at)
	}
	for _, k := range []string{"issues", "watermark", "schema_version"} {
		if _, ok := status[k]; !ok {
			t.Errorf("gadak_status lost %q", k)
		}
	}
}

// GDK-568: gadak_status carries the same frozen bool as `gadak status --json`.
func TestStatusSurfacesFrozen(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Frozen: true}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	db := demoDB(t)
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_status","arguments":{}}}`,
	)
	statusText := callText(t, resps[0])
	var status map[string]any
	if err := json.Unmarshal([]byte(statusText), &status); err != nil {
		t.Fatalf("status JSON: %v\n%s", err, statusText)
	}
	frozen, ok := status["frozen"].(bool)
	if !ok {
		t.Fatalf("gadak_status missing frozen bool: %s", statusText)
	}
	if !frozen {
		t.Fatalf("frozen = %v, want true", status["frozen"])
	}
}

// GDK-569: gadak_issue JSON includes description_text from store.Detail.
func TestIssueCarriesDescriptionText(t *testing.T) {
	db := demoDB(t)
	cr := callToolRaw(t, db, toolIssue, map[string]any{"key": "NMA-1"})
	if cr.IsError {
		t.Fatalf("gadak_issue: %v", cr.Content)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &body); err != nil {
		t.Fatalf("issue JSON: %v\n%s", err, cr.Content[0].Text)
	}
	text, _ := body["description_text"].(string)
	if text == "" {
		t.Fatalf("description_text empty or missing: %s", cr.Content[0].Text)
	}
	if !strings.Contains(text, "idempotency") {
		t.Errorf("description_text = %q, want the mirrored body", text)
	}
	for _, k := range []string{"dev_links", "ref_pages", "backlink_pages"} {
		if _, ok := body[k]; !ok {
			t.Errorf("issue missing %q", k)
		}
	}
}

// GDK-552: gadak_issue accepts {keys: [...]} and wraps several documents.
func TestIssueBulkKeys(t *testing.T) {
	db := demoDB(t)
	cr := callToolRaw(t, db, toolIssue, map[string]any{"keys": []string{"NMA-1", "NMA-2"}})
	if cr.IsError {
		t.Fatalf("gadak_issue keys: %v", cr.Content)
	}
	var wrap struct {
		Issues  []map[string]any `json:"issues"`
		Missing []string         `json:"missing"`
	}
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &wrap); err != nil {
		t.Fatalf("bulk JSON: %v\n%s", err, cr.Content[0].Text)
	}
	if len(wrap.Issues) != 2 {
		t.Fatalf("issues len = %d, want 2", len(wrap.Issues))
	}
	if wrap.Issues[0]["issue_key"] != "NMA-1" || wrap.Issues[1]["issue_key"] != "NMA-2" {
		t.Errorf("order = %v, %v", wrap.Issues[0]["issue_key"], wrap.Issues[1]["issue_key"])
	}
	if _, ok := wrap.Issues[0]["description_text"]; !ok {
		t.Error("bulk document missing description_text")
	}
	if len(wrap.Missing) != 0 {
		t.Errorf("missing = %v", wrap.Missing)
	}

	cr = callToolRaw(t, db, toolIssue, map[string]any{"keys": []string{"NMA-1", "NOPE-1"}})
	if cr.IsError {
		t.Fatalf("partial bulk should succeed: %v", cr.Content)
	}
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &wrap); err != nil {
		t.Fatal(err)
	}
	if len(wrap.Issues) != 1 || wrap.Issues[0]["issue_key"] != "NMA-1" {
		t.Errorf("partial issues = %+v", wrap.Issues)
	}
	if len(wrap.Missing) != 1 || wrap.Missing[0] != "NOPE-1" {
		t.Errorf("missing = %v, want [NOPE-1]", wrap.Missing)
	}

	cr = callToolRaw(t, db, toolIssue, map[string]any{"key": "NMA-1", "keys": []string{"NMA-2"}})
	if !cr.IsError {
		t.Fatalf("key+keys must be isError; content=%v", cr.Content)
	}
	if !strings.Contains(cr.Content[0].Text, "exactly one") {
		t.Errorf("want exclusive-args error, got %s", cr.Content[0].Text)
	}
}

// GDK-552: a name miss lists available views; it must not tell a shell-less
// host to run `gadak views`.
func TestShowNameMissListsAvailable(t *testing.T) {
	dbPath := demoDB(t)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := json.RawMessage(`{"filters":{"jira_project":["NMA"]},"display":{}}`)
	if err := db.PutSavedView(context.Background(), store.SavedView{
		ID: "cli-available-view-zz9", Name: "Available view zz9", Config: cfg,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cr := callToolRaw(t, dbPath, toolShow, map[string]any{"name": "no-such-view-xyz"})
	if !cr.IsError {
		t.Fatalf("expected isError; content=%v", cr.Content)
	}
	text := cr.Content[0].Text
	if strings.Contains(text, "gadak views") {
		t.Errorf("shell-less miss still names gadak views: %s", text)
	}
	if !strings.Contains(text, "Available view zz9") {
		t.Errorf("miss must list available views, got %s", text)
	}
	if !strings.Contains(text, "available:") {
		t.Errorf("want available: prefix, got %s", text)
	}
}

func TestMissingDBToolError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.db")
	resps := session(t, path,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_status","arguments":{}}}`,
	)
	var cr callResult
	raw, _ := json.Marshal(resps[0].Result)
	_ = json.Unmarshal(raw, &cr)
	if !cr.IsError {
		t.Fatal("expected isError when DB missing")
	}
	if !strings.HasPrefix(cr.Content[0].Text, "ERROR:") {
		t.Errorf("isError must start with ERROR:, got %s", cr.Content[0].Text)
	}
	if !strings.Contains(cr.Content[0].Text, "gadak init") {
		t.Errorf("guidance missing: %s", cr.Content[0].Text)
	}
}

func TestShowWritesFocusForProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	db := demoDB(t)
	args, _ := json.Marshal(map[string]any{
		"name":      toolShow,
		"arguments": map[string]any{"keys": []string{"NMA-1", "NMA-2"}},
	})
	resps := sessionProfile(t, db, "work",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(args)+`}`,
	)
	text := callText(t, resps[0])
	var body struct {
		Hash        string   `json:"hash"`
		Applied     []string `json:"applied"`
		Unsupported []string `json:"unsupported"`
		File        string   `json:"file"`
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("show JSON: %v\n%s", err, text)
	}
	wantHash := "ks=NMA-1,NMA-2"
	if body.Hash != wantHash {
		t.Fatalf("hash = %q, want %q", body.Hash, wantHash)
	}
	wantFile, err := uifocus.PathFor("work")
	if err != nil {
		t.Fatal(err)
	}
	if body.File != wantFile {
		t.Fatalf("file = %q, want %q", body.File, wantFile)
	}
	raw, err := os.ReadFile(wantFile)
	if err != nil {
		t.Fatalf("read focus file: %v", err)
	}
	var req struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("focus file JSON: %v\n%s", err, raw)
	}
	if req.Hash != wantHash {
		t.Fatalf("focus file hash = %q, want %q", req.Hash, wantHash)
	}
	if _, err := os.Stat(filepath.Join(home, "ui-focus.json")); !os.IsNotExist(err) {
		t.Fatalf("default-profile focus file should be absent: %v", err)
	}
}

func TestShowInputExclusive(t *testing.T) {
	db := demoDB(t)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"none", map[string]any{}},
		{"keys+jql", map[string]any{"keys": []string{"NMA-1"}, "jql": "project = NMA"}},
		{"issue+name", map[string]any{"issue": "NMA-1", "name": "x"}},
		{"all four", map[string]any{
			"jql": "project = NMA", "keys": []string{"NMA-1"},
			"issue": "NMA-1", "name": "x",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{"name": toolShow, "arguments": tc.args})
			resps := session(t, db,
				`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(payload)+`}`,
			)
			if len(resps) != 1 {
				t.Fatalf("want 1 response, got %d", len(resps))
			}
			if resps[0].Error != nil {
				t.Fatalf("want tool-level error, got JSON-RPC: %+v", resps[0].Error)
			}
			var cr callResult
			raw, _ := json.Marshal(resps[0].Result)
			if err := json.Unmarshal(raw, &cr); err != nil {
				t.Fatal(err)
			}
			if !cr.IsError {
				t.Fatalf("expected isError; content=%v", cr.Content)
			}
			if len(cr.Content) == 0 || !strings.HasPrefix(cr.Content[0].Text, "ERROR:") {
				t.Fatalf("isError must start with ERROR:, got %v", cr.Content)
			}
			if !strings.Contains(cr.Content[0].Text, "exactly one") {
				t.Fatalf("want exactly-one error, got %v", cr.Content)
			}
		})
	}
}

func TestShowJQLUnsupportedPartial(t *testing.T) {
	db := demoDB(t)
	args, _ := json.Marshal(map[string]any{
		"name":      toolShow,
		"arguments": map[string]any{"jql": `project = NMA AND sprint = "Sprint 41"`},
	})
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(args)+`}`,
	)
	text := callText(t, resps[0])
	var body struct {
		Hash        string   `json:"hash"`
		Applied     []string `json:"applied"`
		Unsupported []string `json:"unsupported"`
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("show JSON: %v\n%s", err, text)
	}
	if !strings.Contains(body.Hash, "pj=NMA") {
		t.Fatalf("supported project was not applied: hash=%q", body.Hash)
	}
	if strings.Contains(strings.ToLower(body.Hash), "sprint") {
		t.Fatalf("unsupported sprint leaked into hash: %q", body.Hash)
	}
	joined := strings.Join(body.Unsupported, " | ")
	if !strings.Contains(strings.ToLower(joined), "sprint") {
		t.Fatalf("unsupported missing sprint: %v", body.Unsupported)
	}
	applied := strings.Join(body.Applied, " ")
	if !strings.Contains(applied, "project") {
		t.Fatalf("applied missing project: %v", body.Applied)
	}
}

func TestShowNameUsesStoredViewLookup(t *testing.T) {
	dbPath := demoDB(t)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := json.RawMessage(`{"filters":{"jira_project":["NMA"]},"display":{}}`)
	if err := db.PutSavedView(context.Background(), store.SavedView{
		ID: "cli-tm-m-unique-view-zz9", Name: "TM-M unique view zz9", Config: cfg,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	f := jql.EmptyFilter()
	f.JiraProject = []string{"NMA"}
	wantHash := jql.Hash(f, jql.Display{})

	args, _ := json.Marshal(map[string]any{
		"name":      toolShow,
		"arguments": map[string]any{"name": "TM-M unique view zz9"},
	})
	resps := session(t, dbPath,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(args)+`}`,
	)
	text := callText(t, resps[0])
	var body struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("show JSON: %v\n%s", err, text)
	}
	if body.Hash != wantHash {
		t.Fatalf("name hash = %q, want %q (same as views.HashFromConfig)", body.Hash, wantHash)
	}
}

func TestShowKeysNoMirror(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.db")
	args, _ := json.Marshal(map[string]any{
		"name":      toolShow,
		"arguments": map[string]any{"keys": []string{"NMA-1", "NMA-2"}},
	})
	resps := session(t, path,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(args)+`}`,
	)
	text := callText(t, resps[0])
	if !strings.Contains(text, "ks=NMA-1,NMA-2") {
		t.Fatalf("keys without mirror: %s", text)
	}
}

func TestUnknownToolProtocolError(t *testing.T) {
	db := demoDB(t)
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_delete","arguments":{}}}`,
	)
	if resps[0].Error == nil {
		t.Fatal("expected JSON-RPC error for unknown tool")
	}
	if resps[0].Error.Code != codeInvalidParams {
		t.Errorf("code = %d", resps[0].Error.Code)
	}
}

func TestStdoutIsOnlyJSONRPC(t *testing.T) {
	// Guard against accidental log lines: every stdout line must parse as JSON-RPC.
	db := demoDB(t)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
	var out bytes.Buffer
	if err := New(db, "", "test").Serve(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("line %d is not JSON: %q (%v)", i, line, err)
		}
		if msg["jsonrpc"] != "2.0" {
			t.Fatalf("line %d missing jsonrpc: %s", i, line)
		}
	}
}

func TestSearchAcceptsQueryAliasAndQ(t *testing.T) {
	db := demoDB(t)
	for _, args := range []map[string]any{
		{"query": "upload"},
		{"q": "upload"},
		{"text": "upload"},
	} {
		t.Run(fmtArgs(args), func(t *testing.T) {
			cr := callToolRaw(t, db, toolSearch, args)
			if cr.IsError {
				t.Fatalf("isError for %v: %v", args, cr.Content)
			}
			if len(cr.Content) == 0 {
				t.Fatal("empty content")
			}
			var search struct {
				Total  int              `json:"total"`
				Issues []map[string]any `json:"issues"`
				Pages  []map[string]any `json:"pages"`
			}
			if err := json.Unmarshal([]byte(cr.Content[0].Text), &search); err != nil {
				t.Fatalf("search JSON: %v\n%s", err, cr.Content[0].Text)
			}
			if search.Total <= 0 && len(search.Issues) == 0 && len(search.Pages) == 0 {
				t.Fatalf("expected search hits for upload via %v; body=%s", args, cr.Content[0].Text)
			}
		})
	}
}

func TestSearchQueryArgPriority(t *testing.T) {
	text, via, ok := searchQueryArg(map[string]any{"query": "a", "text": "b", "q": "c"})
	if !ok || text != "a" || via != "query" {
		t.Fatalf("query should win: text=%q via=%q ok=%v", text, via, ok)
	}
	text, via, ok = searchQueryArg(map[string]any{"query": "  ", "text": "b", "q": "c"})
	if !ok || text != "b" || via != "text" {
		t.Fatalf("empty query falls through to text: text=%q via=%q ok=%v", text, via, ok)
	}
	text, via, ok = searchQueryArg(map[string]any{"q": "c"})
	if !ok || text != "c" || via != "q" {
		t.Fatalf("q alone: text=%q via=%q ok=%v", text, via, ok)
	}
	if _, _, ok = searchQueryArg(map[string]any{"limit": 3}); ok {
		t.Fatal("limit-only must not yield a query string")
	}
}

func TestSearchMissingQueryIsErrorNotEmptyResult(t *testing.T) {
	db := demoDB(t)
	cr := callToolRaw(t, db, toolSearch, map[string]any{"limit": 3})
	if !cr.IsError {
		t.Fatalf("expected isError; content=%v", cr.Content)
	}
	if len(cr.Content) == 0 {
		t.Fatal("empty error content")
	}
	text := cr.Content[0].Text
	if !strings.HasPrefix(text, "ERROR:") {
		t.Errorf("error must start with ERROR:, got %q", text)
	}
	if !strings.Contains(text, "argument keys: [limit]") {
		t.Errorf("missing argument keys: [limit] in %q", text)
	}
	if !strings.Contains(text, "not an empty search result") {
		t.Errorf("missing empty-result disclaimer in %q", text)
	}
}

// TestToolDescriptionsOriginAndCJK is GDK-471: gadak_query must not summarise
// writes as Jira-only (standalone and paired origins exist), and gadak_search
// must carry the CJK mid-compound sentence. Primary search argument stays query.
func TestToolDescriptionsOriginAndCJK(t *testing.T) {
	var query, search Tool
	for _, def := range toolDefinitions() {
		switch def.Name {
		case toolQuery:
			query = def
		case toolSearch:
			search = def
		}
	}
	if query.Name == "" || search.Name == "" {
		t.Fatal("missing gadak_query or gadak_search")
	}
	if !strings.Contains(query.Description, "writes go through the origin") {
		t.Errorf("gadak_query missing origin write sentence:\n%s", query.Description)
	}
	if strings.Contains(query.Description, "writes go through Jira.") {
		t.Errorf("gadak_query still says writes go through Jira only")
	}
	if !strings.Contains(search.Description, "결제") {
		t.Errorf("gadak_search missing CJK mid-compound sentence:\n%s", search.Description)
	}
	if !strings.Contains(search.Description, "{query:") {
		t.Errorf("gadak_search must keep query as the primary argument:\n%s", search.Description)
	}
}

func TestToolsListQueryBeforeSearch(t *testing.T) {
	db := demoDB(t)
	resps := session(t, db, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var list struct {
		Tools []Tool `json:"tools"`
	}
	mustResult(t, resps[0], &list)
	qi, si := -1, -1
	for i, tool := range list.Tools {
		switch tool.Name {
		case toolQuery:
			qi = i
		case toolSearch:
			si = i
			props, _ := tool.InputSchema["properties"].(map[string]any)
			if props == nil {
				t.Fatal("gadak_search inputSchema.properties missing")
			}
			for _, key := range []string{"query", "text", "q"} {
				if _, ok := props[key]; !ok {
					t.Errorf("gadak_search schema missing %q", key)
				}
			}
			rawReq, ok := tool.InputSchema["required"]
			if !ok {
				t.Error("gadak_search required missing, want []")
			} else if req, isArr := rawReq.([]any); !isArr {
				t.Errorf("gadak_search required = %#v (%T), want []", rawReq, rawReq)
			} else if len(req) != 0 {
				t.Errorf("gadak_search required = %#v, want empty", rawReq)
			}
		}
	}
	if qi < 0 || si < 0 {
		t.Fatalf("missing tools: query=%d search=%d", qi, si)
	}
	if qi >= si {
		t.Fatalf("gadak_query index %d is not before gadak_search %d", qi, si)
	}
}

func TestInitializeInstructionsPreferQuery(t *testing.T) {
	db := demoDB(t)
	resps := session(t, db, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	var init struct {
		Instructions string `json:"instructions"`
	}
	mustResult(t, resps[0], &init)
	if strings.Contains(init.Instructions, "If you have a shell") {
		t.Errorf("instructions still contain install-time shell advice: %q", init.Instructions)
	}
	if !strings.Contains(init.Instructions, "gadak_query is the default") {
		t.Errorf("instructions missing gadak_query is the default: %q", init.Instructions)
	}
}

func TestIssueTruncatesCommentsUnderByteCap(t *testing.T) {
	db := demoDB(t)
	seeded := seedLargeIssueComments(t, db, "NMA-1", 40, 8000)
	// Production cap (256KiB). 40×8KiB comments exceed it; recovery must
	// return a truncated body instead of isError.
	cr := callToolRaw(t, db, toolIssue, map[string]any{"key": "NMA-1"})
	if cr.IsError {
		t.Fatalf("gadak_issue should truncate, not error: %v", cr.Content)
	}
	if len(cr.Content) == 0 {
		t.Fatal("empty content")
	}
	var body struct {
		Truncated       bool `json:"truncated"`
		CommentsOmitted int  `json:"comments_omitted"`
		Comments        []struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
		} `json:"comments"`
	}
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &body); err != nil {
		t.Fatalf("issue JSON: %v\n%s", err, cr.Content[0].Text)
	}
	if !body.Truncated {
		t.Error("expected truncated=true")
	}
	if body.CommentsOmitted <= 0 {
		t.Errorf("comments_omitted = %d, want > 0", body.CommentsOmitted)
	}
	if body.CommentsOmitted+len(body.Comments) != seeded {
		t.Errorf("omitted %d + kept %d != seeded %d", body.CommentsOmitted, len(body.Comments), seeded)
	}
	// Newest kept: last seeded comment (highest created_at) must remain.
	if len(body.Comments) == 0 {
		t.Fatal("kept no comments")
	}
	last := body.Comments[len(body.Comments)-1]
	if last.ID != "mcp-fat-039" {
		t.Errorf("newest kept id = %q, want mcp-fat-039 (oldest-first store order)", last.ID)
	}
}

func TestMarshalIssueResultFitsByDroppingOldestComments(t *testing.T) {
	// Unit path with an injected cap (production stays 256KiB).
	s := New("", "", "test")
	s.resultByteCap = 900
	comments := make([]store.DetailComment, 8)
	for i := range comments {
		comments[i] = store.DetailComment{
			ID:        fmt.Sprintf("c-%d", i),
			Body:      strings.Repeat("m", 200),
			CreatedAt: fmt.Sprintf("2026-01-01T00:00:%02dZ", i),
		}
	}
	body := map[string]any{
		"issue_key":       "NMA-1",
		"description_adf": json.RawMessage(`{}`),
		"comments":        comments,
		"attachments":     []store.DetailAttachment{},
		"history":         []store.DetailChange{},
		"linked_issues":   []store.DetailLink{},
	}
	out, err := s.marshalIssueResult(body)
	if err != nil {
		t.Fatalf("marshalIssueResult: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("empty content")
	}
	var got struct {
		Truncated       bool `json:"truncated"`
		CommentsOmitted int  `json:"comments_omitted"`
		Comments        []struct {
			ID string `json:"id"`
		} `json:"comments"`
	}
	if err := json.Unmarshal([]byte(out[0].Text), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Truncated {
		t.Error("expected truncated=true")
	}
	if got.CommentsOmitted <= 0 {
		t.Errorf("comments_omitted = %d", got.CommentsOmitted)
	}
	if len(got.Comments) == 0 || got.Comments[len(got.Comments)-1].ID != "c-7" {
		t.Errorf("expected newest comment c-7 kept, got %+v", got.Comments)
	}
}

func TestMarshalIssueResultErrorsWhenStillOverCap(t *testing.T) {
	s := New("", "", "test")
	s.resultByteCap = 200
	body := map[string]any{
		"issue_key":       "NMA-1",
		"description_adf": json.RawMessage(`"` + strings.Repeat("D", 400) + `"`),
		"comments":        []store.DetailComment{},
		"attachments":     []store.DetailAttachment{},
		"history":         []store.DetailChange{},
		"linked_issues":   []store.DetailLink{},
	}
	_, err := s.marshalIssueResult(body)
	if err == nil {
		t.Fatal("expected error when payload exceeds cap with no comments left")
	}
	if !strings.Contains(err.Error(), "result exceeds") {
		t.Errorf("want existing exceeds error, got %v", err)
	}
}

func TestSearchHydrationMatchesFullScanOrder(t *testing.T) {
	// The old path loaded every IssueLite then reordered by FTS keys. The new
	// path must emit the same {key,summary,status} rows in the same order.
	path := demoDB(t)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	res, err := db.Search(context.Background(), "upload", 3)
	if err != nil {
		t.Fatal(err)
	}
	all, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
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
	want := make([]hit, 0, len(res.Keys))
	for _, k := range res.Keys {
		if l, ok := byKey[k]; ok {
			want = append(want, hit{Key: l.IssueKey, Summary: l.Summary, Status: l.Status})
		} else {
			want = append(want, hit{Key: k})
		}
	}
	cr := callToolRaw(t, path, toolSearch, map[string]any{"query": "upload", "limit": 3})
	if cr.IsError {
		t.Fatalf("search error: %v", cr.Content)
	}
	var got struct {
		Issues []hit `json:"issues"`
	}
	if err := json.Unmarshal([]byte(cr.Content[0].Text), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Issues, want) {
		t.Fatalf("issues=%+v\nwant   =%+v", got.Issues, want)
	}
}

func TestToolsDoNotFullScanIssueLites(t *testing.T) {
	// GDK-10: gadak_search / gadak_issue used to load every IssueLite per call.
	// The store now has IssueLitesByKeys; these two handlers must ride it.
	src, err := os.ReadFile("tools.go")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(src, []byte(".IssueLites(")) {
		t.Fatal("tools.go still calls IssueLites (full mirror scan); use IssueLitesByKeys")
	}
	if !bytes.Contains(src, []byte(".IssueLitesByKeys(")) {
		t.Fatal("tools.go never calls IssueLitesByKeys")
	}
}

func TestWriteSQLErrorPrefixed(t *testing.T) {
	// A2: every isError body starts with ERROR: (one existing write-SQL path).
	db := demoDB(t)
	args, _ := json.Marshal(map[string]any{
		"name":      toolQuery,
		"arguments": map[string]any{"sql": "DELETE FROM issues"},
	})
	resps := session(t, db,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(args)+`}`,
	)
	var cr callResult
	raw, _ := json.Marshal(resps[0].Result)
	if err := json.Unmarshal(raw, &cr); err != nil {
		t.Fatal(err)
	}
	if !cr.IsError || len(cr.Content) == 0 {
		t.Fatalf("expected isError, got %+v", cr)
	}
	if !strings.HasPrefix(cr.Content[0].Text, "ERROR:") {
		t.Errorf("isError text must start with ERROR:, got %q", cr.Content[0].Text)
	}
}

func callToolRaw(t *testing.T, db string, name string, args map[string]any) callResult {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"name": name, "arguments": args})
	if err != nil {
		t.Fatal(err)
	}
	resps := session(t, db, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+string(payload)+`}`)
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("JSON-RPC error: %+v", resps[0].Error)
	}
	var cr callResult
	raw, _ := json.Marshal(resps[0].Result)
	if err := json.Unmarshal(raw, &cr); err != nil {
		t.Fatal(err)
	}
	return cr
}

func fmtArgs(args map[string]any) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}

func seedLargeIssueComments(t *testing.T, dbPath, key string, n, bodyBytes int) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var itemID string
	if err := db.QueryRow(`SELECT item_id FROM issues WHERE key = ?`, key).Scan(&itemID); err != nil {
		t.Fatalf("item_id for %s: %v", key, err)
	}
	body := strings.Repeat("x", bodyBytes)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("mcp-fat-%03d", i)
		at := fmt.Sprintf("2026-08-01T00:00:%02dZ", i)
		if _, err := db.Exec(
			`INSERT INTO comments (id, item_id, author, body_text, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
			id, itemID, "seed", body, at, at,
		); err != nil {
			t.Fatalf("insert comment %s: %v", id, err)
		}
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments WHERE item_id = ?`, itemID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

func mustResult(t *testing.T, r rpcResponse, dest any) {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("unexpected error: %+v", r.Error)
	}
	raw, err := json.Marshal(r.Result)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		t.Fatalf("unmarshal result: %v\n%s", err, raw)
	}
}

func callText(t *testing.T, r rpcResponse) string {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("JSON-RPC error: %+v", r.Error)
	}
	var cr callResult
	raw, _ := json.Marshal(r.Result)
	if err := json.Unmarshal(raw, &cr); err != nil {
		t.Fatalf("call result: %v", err)
	}
	if cr.IsError {
		t.Fatalf("tool isError: %v", cr.Content)
	}
	if len(cr.Content) == 0 {
		t.Fatal("empty content")
	}
	return cr.Content[0].Text
}
