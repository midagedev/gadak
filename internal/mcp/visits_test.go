package mcp

// GDK-631: reads over MCP leave the same trail CLI reads leave, and
// gadak_recents walks it back.

import (
	"context"
	"database/sql"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"

	_ "modernc.org/sqlite"
)

// visitRecorderFile is the one file allowed to reach the mirror's detail and
// search readers. Everything else in the package goes through loadDetail /
// runSearch and therefore records.
const visitRecorderFile = "visits.go"

// The class this round closes: a read path that reaches the mirror directly
// records nothing, and nothing says so — the defect is invisible until an
// agent's recents is empty. Rather than trusting the next tool's author to
// remember the call, the recorder owns the two readers.
//
// FAIL-first: on the pre-fix source this named tools.go for both Detail and
// Search.
func TestMirrorReadsGoThroughTheVisitRecorder(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == visitRecorderFile {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		t.Fatal("parsed no package files — the assertion would pass vacuously")
	}
	recorded := map[string]bool{"Detail": true, "Search": true}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !recorded[sel.Sel.Name] {
				return true
			}
			recv, ok := sel.X.(*ast.SelectorExpr)
			if !ok || recv.Sel.Name != "db" {
				return true
			}
			pos := fset.Position(call.Pos())
			t.Errorf("%s:%d calls s.db.%s directly; go through %s (loadDetail / runSearch) so the read is recorded",
				filepath.Base(pos.Filename), pos.Line, sel.Sel.Name, visitRecorderFile)
			return true
		})
	}
}

// openLocal opens local.db beside the mirror, read-only, for assertions.
func openLocal(t *testing.T, mirrorPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+store.LocalPath(mirrorPath)+"?mode=ro")
	if err != nil {
		t.Fatalf("open local.db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestIssueToolRecordsVisit(t *testing.T) {
	dbPath := demoDB(t)
	resps := session(t, dbPath,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_issue","arguments":{"key":"NMA-1"}}}`,
	)
	if len(resps) != 1 || resps[0].Error != nil {
		t.Fatalf("gadak_issue: %+v", resps)
	}
	local := openLocal(t, dbPath)
	var kind, key, source string
	err := local.QueryRow(`SELECT kind, key, source FROM visits ORDER BY id DESC LIMIT 1`).Scan(&kind, &key, &source)
	if err != nil {
		t.Fatalf("no visit row after gadak_issue: %v", err)
	}
	if kind != store.VisitKindIssue || key != "NMA-1" || source != store.VisitSourceMCP {
		t.Fatalf("got (%s, %s, %s), want (issue, NMA-1, mcp)", kind, key, source)
	}
}

// A key the mirror does not have is not a read: the CLI's notFound keys never
// reach its recorder either.
func TestIssueToolDoesNotRecordMissingKey(t *testing.T) {
	dbPath := demoDB(t)
	session(t, dbPath,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_issue","arguments":{"key":"NOPE-999"}}}`,
	)
	local := openLocal(t, dbPath)
	var n int
	if err := local.QueryRow(`SELECT COUNT(*) FROM visits WHERE key = 'NOPE-999'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("recorded %d visits for a key that is not in the mirror", n)
	}
}

func TestSearchToolRecordsSearch(t *testing.T) {
	dbPath := demoDB(t)
	resps := session(t, dbPath,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_search","arguments":{"query":"upload","limit":3}}}`,
	)
	if len(resps) != 1 || resps[0].Error != nil {
		t.Fatalf("gadak_search: %+v", resps)
	}
	local := openLocal(t, dbPath)
	var query string
	var count int
	if err := local.QueryRow(`SELECT query, result_count FROM searches ORDER BY id DESC LIMIT 1`).Scan(&query, &count); err != nil {
		t.Fatalf("no search row after gadak_search: %v", err)
	}
	if query != "upload" || count < 0 {
		t.Fatalf("got (%q, %d), want (upload, >=0)", query, count)
	}
}

// callResultBody decodes one tools/call result's single text item as JSON.
func callResultBody(t *testing.T, resp rpcResponse) map[string]any {
	t.Helper()
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Content []contentItem `json:"content"`
		IsError bool          `json:"isError"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("tool answered an error: %+v", res)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &body); err != nil {
		t.Fatalf("result is not JSON: %v\n%s", err, res.Content[0].Text)
	}
	return body
}

// The round trip a shell-less agent actually makes: read an issue, then ask
// recents — over the protocol, so dispatch is exercised too (GDK-769: a tool
// can be listed and still refused by name).
func TestRecentsToolWalksBackTheTrail(t *testing.T) {
	dbPath := demoDB(t)
	resps := session(t, dbPath,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_issue","arguments":{"key":"NMA-1"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gadak_recents","arguments":{"limit":5}}}`,
	)
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2", len(resps))
	}
	if resps[1].Error != nil {
		t.Fatalf("gadak_recents is not callable: %+v", resps[1].Error)
	}
	body := callResultBody(t, resps[1])
	rows, _ := body["recents"].([]any)
	if len(rows) == 0 {
		t.Fatalf("gadak_recents answered no rows after a gadak_issue read: %v", body)
	}
	first, _ := rows[0].(map[string]any)
	if first["key"] != "NMA-1" || first["kind"] != store.VisitKindIssue {
		t.Fatalf("newest recent is %v, want NMA-1/issue", first)
	}
	if first["viewed_at"] == "" {
		t.Fatalf("recent row has no viewed_at: %v", first)
	}
}

func TestRecentsToolRefusesUnknownArgument(t *testing.T) {
	dbPath := demoDB(t)
	resps := session(t, dbPath,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gadak_recents","arguments":{"kind":"issue"}}}`,
	)
	var res struct {
		Content []contentItem `json:"content"`
		IsError bool          `json:"isError"`
	}
	raw, _ := json.Marshal(resps[0].Result)
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].Text, "does not take kind") {
		t.Fatalf("unknown argument was not named: %+v", res)
	}
}

// Recording is a side effect, never the answer: a local history that cannot
// take the row must not fail the read. Same contract as the CLI's.
func TestVisitRecordingIsBestEffort(t *testing.T) {
	dbPath := demoDB(t)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	s := &Server{DBPath: dbPath, db: db}
	// An invalid kind is the cheapest way to make RecordVisit fail without
	// touching the file system; the helper must swallow it.
	s.recordVisitBestEffort(context.Background(), "not-a-kind", "NMA-1")
	s.recordSearchBestEffort(context.Background(), "anything", -1)
}
