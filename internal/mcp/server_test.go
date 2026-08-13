package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	in := strings.Join(frames, "\n") + "\n"
	var out bytes.Buffer
	srv := New(dbPath, "", "test")
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
		`{"jsonrpc":"2.0","id":7,"method":"ping"}`,
	)
	// notifications/initialized produces no response → 7 responses for 8 frames.
	if len(resps) != 7 {
		t.Fatalf("got %d responses, want 7", len(resps))
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

	// tools/list: exactly the four contracted tools.
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
	for _, want := range []string{toolQuery, toolSearch, toolIssue, toolStatus} {
		if !names[want] {
			t.Errorf("tools/list missing %s", want)
		}
	}
	if len(list.Tools) != 4 {
		t.Errorf("tools/list has %d tools, want 4", len(list.Tools))
	}
	// gadak_query description must carry the localization warning and examples.
	var qdesc string
	for _, tool := range list.Tools {
		if tool.Name == toolQuery {
			qdesc = tool.Description
		}
	}
	for _, needle := range []string{"status_category", "In Progress", "SELECT", "inprogress"} {
		if !strings.Contains(qdesc, needle) {
			t.Errorf("gadak_query description missing %q", needle)
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
	if _, ok := issue["comments"]; !ok {
		t.Error("issue missing comments")
	}
	if _, ok := issue["history"]; !ok {
		t.Error("issue missing history")
	}

	// ping
	if resps[6].Error != nil {
		t.Errorf("ping error: %+v", resps[6].Error)
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
	if !strings.Contains(cr.Content[0].Text, "gadak init") {
		t.Errorf("guidance missing: %s", cr.Content[0].Text)
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
