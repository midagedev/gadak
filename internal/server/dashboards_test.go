package server

// Contract ↔ assertion map for the /api/v1/dashboards/ surface (GDK-781).
// Every route contract has a normal and a violation/boundary assertion:
//
//	contract clause                     | assertions
//	------------------------------------+----------------------------------------------
//	GET list → {version, dashboards}    | TestDashboardSaveListDelete (version moves on save/update/delete)
//	POST save = upsert by name          | TestDashboardSaveListDelete (same name keeps id, 1 row)
//	POST validation failure → 400+reason| TestDashboardSaveValidation (empty html, both sql+jql, name, body, size)
//	DELETE unknown id → 404             | TestDashboardSaveListDelete, TestDashboardDataErrors
//	render: html bytes + exact CSP      | TestDashboardRenderCSP (headers, body bytes; [GDK-792] vendor
//	                                    |  source per-host, only-host assertion, fail-closed on bad Host;
//	                                    |  [GDK-808] undeclared libs keep the policy byte-identical)
//	GET {id}/ → row with config         | TestDashboardGetRow (web host's datasource map read)
//	vendor/{file}: embed whitelist      | TestDashboardVendorRoute (uPlot assets + types + CORS; rest 404)
//	libs/{file}: local cache, hash-pinned| TestDashboardLibRoute (200+fixed type+CORS; unknown 404; tampered
//	                                    |  bytes 500 — re-hashed at serve time; traversal ids miss)
//	save: libs must exist in the cache  | TestDashboardSaveLibs (unknown id 400 + lib add recipe; known 201)
//	render: declared libs injected      | TestDashboardRenderLibs (order, headless fallback, CSP widening
//	                                    |  script-src-only; undeclared = no injection, old policy)
//	render: corrupt stored row → 500    | TestDashboardRenderCorruptConfig
//	data sql → read-only execution      | TestDashboardDataSQL (+ display-name warning axis)
//	data jql → same shape, fixed cols   | TestDashboardDataJQL
//	data: write SQL refused at HTTP      | TestDashboardDataWriteSQLRefused (mirror row unchanged)
//	data errors: 404/400 named codes    | TestDashboardDataErrors (unknown id/name, sql/jql errors)
//	absorb: stored names win            | TestDashboardAbsorb (merged list response, id collision reminted)
//	hostile input classes               | TestDashboardSaveValidation (oversized html, XSS name),
//	                                    | TestDashboardDataErrors (path-traversal id)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/dashboards"
	"github.com/midagedev/gadak/internal/store"
)

func dashHandler(t *testing.T) http.Handler {
	t.Helper()
	db, cfg := fixture(t)
	return New(db, cfg)
}

// saveDash posts one dashboard config and returns the saved row's id.
func saveDash(t *testing.T, h http.Handler, name, config string) *string {
	t.Helper()
	rec := send(t, h, http.MethodPost, dashBase, `{"name":`+mustJSON(name)+`,"config":`+config+`}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("save %s: status %d: %s", name, rec.Code, rec.Body.String())
	}
	var saved store.Dashboard
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("save decode: %v", err)
	}
	return &saved.ID
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestDashboardSaveListDelete(t *testing.T) {
	h := dashHandler(t)

	rec := send(t, h, http.MethodGet, dashBase, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Version    int               `json:"version"`
		Dashboards []listedDashboard `json:"dashboards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Version != 0 || len(list.Dashboards) != 0 {
		t.Fatalf("empty list = %+v", list)
	}

	id := saveDash(t, h, "Triage", `{"html":"<p>one</p>"}`)
	rec = send(t, h, http.MethodGet, dashBase, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Version != 1 || len(list.Dashboards) != 1 {
		t.Fatalf("after save = %+v", list)
	}
	got := list.Dashboards[0]
	if got.ID != *id || got.Name != "Triage" || got.UpdatedAt == "" {
		t.Fatalf("listed = %+v", got)
	}

	// Same name updates the row it owns: stable id, still one row, version moves.
	id2 := saveDash(t, h, "Triage", `{"html":"<p>two</p>"}`)
	if *id2 != *id {
		t.Fatalf("same-name save minted id %s, want %s", *id2, *id)
	}
	rec = send(t, h, http.MethodGet, dashBase, "")
	json.Unmarshal(rec.Body.Bytes(), &list)
	if list.Version != 2 || len(list.Dashboards) != 1 {
		t.Fatalf("after update = %+v", list)
	}

	if rec := send(t, h, http.MethodDelete, dashBase+*id+"/", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(t, h, http.MethodDelete, dashBase+*id+"/", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("double delete: %d, want 404", rec.Code)
	}
	if rec := send(t, h, http.MethodGet, dashBase+*id+"/render/", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("render after delete: %d, want 404", rec.Code)
	}
}

func TestDashboardSaveValidation(t *testing.T) {
	h := dashHandler(t)
	cases := []struct {
		name string
		body string
		code string
	}{
		{"empty html", `{"name":"x","config":{"html":""}}`, "invalid_config"},
		{"html missing", `{"name":"x","config":{"datasources":{}}}`, "invalid_config"},
		{"sql and jql", `{"name":"x","config":{"html":"<p/>","datasources":{"a":{"sql":"select 1","jql":"project = NMB"}}}}`, "invalid_config"},
		{"bad datasource name", `{"name":"x","config":{"html":"<p/>","datasources":{"Bad Name":{"sql":"select 1"}}}}`, "invalid_config"},
		{"unknown config field", `{"name":"x","config":{"html":"<p/>","queries":{}}}`, "invalid_config"},
		{"name required", `{"config":{"html":"<p/>"}}`, "name_required"},
		{"garbage body", `not json`, "invalid_body"},
	}
	for _, tc := range cases {
		rec := send(t, h, http.MethodPost, dashBase, tc.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", tc.name, rec.Code, rec.Body.String())
			continue
		}
		var e struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error != tc.code {
			t.Errorf("%s: body %s, want error %s", tc.name, rec.Body.String(), tc.code)
		}
	}
	// A validation failure must leave nothing behind: next-save still mints
	// the first id, version still 0.
	rec := send(t, h, http.MethodGet, dashBase, "")
	var list struct {
		Version    int               `json:"version"`
		Dashboards []listedDashboard `json:"dashboards"`
	}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if list.Version != 0 || len(list.Dashboards) != 0 {
		t.Fatalf("rejected saves left state: %+v", list)
	}

	// Hostile html: a multi-megabyte body is refused with a named code, not
	// an OOM. Build it without printing it.
	big := `{"name":"big","config":{"html":"<p>` + strings.Repeat("x", 9<<20) + `</p>"}}`
	rec = send(t, h, http.MethodPost, dashBase, big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized html: %d, want 413", rec.Code)
	}

	// A dashboard NAME is free text; it must never reach a response as raw
	// markup. encoding/json escapes it — this pins that no future handler
	// switches to string concatenation.
	rec = send(t, h, http.MethodPost, dashBase, `{"name":"<script>alert(1)</script>","config":{"html":"<p/>"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("xss-name save: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "<script>") {
		t.Fatalf("raw markup in response: %s", rec.Body.String())
	}
	send(t, h, http.MethodDelete, dashBase+lastID(t, h)+"/", "")
}

// lastID reads the single row's id from the list (test helper for cleanup).
func lastID(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := send(t, h, http.MethodGet, dashBase, "")
	var list struct {
		Dashboards []listedDashboard `json:"dashboards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Dashboards) != 1 {
		t.Fatalf("lastID: %d rows", len(list.Dashboards))
	}
	return list.Dashboards[0].ID
}

func TestDashboardRenderCSP(t *testing.T) {
	h := dashHandler(t)
	id := saveDash(t, h, "Wall", `{"html":"<!doctype html><html><body><h1>triage</h1></body></html>"}`)
	req := testRequest(http.MethodGet, dashBase+*id+"/render/", nil)
	req.Host = "127.0.0.1:7877"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("render: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	if rec.Body.String() != `<!doctype html><html><body><h1>triage</h1></body></html>` {
		t.Errorf("body = %q", rec.Body.String())
	}
	csp := rec.Header().Get("Content-Security-Policy")
	// [GDK-792, 2026-08-24] The exact-match constant became a per-request
	// composition: the vendor path joined script-src/style-src so dashboards
	// can load uPlot/three from the same server instead of a CDN. The source
	// expression is path-scoped and composed from the request's own host
	// because the render document is sandboxed without allow-same-origin —
	// its origin is opaque, where 'self' matches nothing (verified in
	// Playwright: e2e/dashboards.spec.ts loads vendor scripts inside the
	// frame). The unchanged half of the contract is asserted below: default-src
	// stays 'none', inline-only script/style stay, and the ONLY host in the
	// policy is the request's own — no external host may ever join.
	wantSrc := "http://127.0.0.1:7877" + dashVendorPath
	if !strings.HasPrefix(csp, "default-src 'none'; ") {
		t.Errorf("CSP lost default-src 'none': %q", csp)
	}
	if !strings.Contains(csp, "script-src 'unsafe-inline' "+wantSrc+";") {
		t.Errorf("script-src vendor source missing: %q", csp)
	}
	if !strings.Contains(csp, "style-src 'unsafe-inline' "+wantSrc+";") {
		t.Errorf("style-src vendor source missing: %q", csp)
	}
	if !strings.HasSuffix(csp, "img-src data:") {
		t.Errorf("img-src changed: %q", csp)
	}
	// No host beyond the request's own appears anywhere in the policy: every
	// scheme://host[:port] token must be the vendor source above. This is the
	// "external hosts stay zero" assertion the pre-792 constant carried.
	for _, tok := range strings.FieldsFunc(csp, func(r rune) bool { return r == ' ' || r == ';' }) {
		if i := strings.Index(tok, "://"); i >= 0 {
			if tok != wantSrc {
				t.Errorf("unexpected host in CSP: %q (want only %q)", tok, wantSrc)
			}
		}
	}
	// A hostile/invalid Host never widens the policy: vendor sources are
	// omitted (fail closed) rather than echoed. GuardBrowser confines the
	// requests that reach the handler to loopback shapes already, so the
	// fail-closed edge is asserted on the composer itself — it must not
	// assume the guard ran (tests, future mounts).
	for _, host := range []string{"", "evil.com/../x", "host with spaces", "host;injection"} {
		if got := vendorCSPSource(host, false); got != "" {
			t.Errorf("vendorCSPSource(%q) = %q, want empty (fail closed)", host, got)
		}
	}
	if got := vendorCSPSource("[::1]:7877", false); got != "http://[::1]:7877"+dashVendorPath {
		t.Errorf("ipv6 vendor source = %q", got)
	}
	if got := vendorCSPSource("localhost", true); got != "https://localhost"+dashVendorPath {
		t.Errorf("tls vendor source = %q", got)
	}
	if got := dashboardCSP("", ""); got != `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:` {
		t.Errorf("fail-closed CSP drifted: %q", got)
	}
}

// TestDashboardVendorRoute (GDK-792): vendor assets are served from an embed
// whitelist — exact filenames only, correct content types, CORS-open for ES
// module imports from the opaque-origin sandbox frame. Everything else 404s.
func TestDashboardVendorRoute(t *testing.T) {
	h := dashHandler(t)
	for name, ctype := range map[string]string{
		"uPlot.iife.min.js": "text/javascript; charset=utf-8",
		"uPlot.min.css":     "text/css; charset=utf-8",
	} {
		rec := send(t, h, http.MethodGet, dashBase+"vendor/"+name, "")
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", name, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != ctype {
			t.Errorf("%s: Content-Type %q, want %q", name, got, ctype)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("%s: ACAO %q, want * (module imports from the sandboxed frame)", name, got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: nosniff %q", name, got)
		}
		if rec.Body.Len() < 1000 {
			t.Errorf("%s: body %d bytes, want the real asset", name, rec.Body.Len())
		}
	}
	// Whitelist boundary: unlisted names, traversal shapes, the removed
	// three builds ([GDK-808] — now lib-cache business), and the license
	// file (embedded for NOTICE, not HTTP-served) never serve vendor bytes.
	// ../-shapes answer with ServeMux's clean-path redirect (307 + its small
	// boilerplate body), not 404 — what matters (same as the traversal case
	// in TestDashboardDataErrors) is that no asset body comes back.
	for _, name := range []string{
		"evil.js", "uPlot.iife.min.css", "LICENSE-uplot",
		"three.module.min.js", "three.core.min.js",
		"sub/dir/uPlot.iife.min.js", "uPlot.iife.min.js%00",
	} {
		if rec := send(t, h, http.MethodGet, dashBase+"vendor/"+name, ""); rec.Code != http.StatusNotFound {
			t.Errorf("vendor/%s: status %d, want 404", name, rec.Code)
		}
	}
	if rec := send(t, h, http.MethodGet, dashBase+"vendor/../uPlot.iife.min.js", ""); rec.Code == http.StatusOK || rec.Body.Len() >= 1000 {
		t.Errorf("vendor traversal: status %d, body %d bytes — must not serve", rec.Code, rec.Body.Len())
	}
}

// TestDashboardGetRow (GDK-782): GET {id}/ is the web host's config read —
// the parent page needs the datasources map to know which data routes to run.
func TestDashboardGetRow(t *testing.T) {
	h := dashHandler(t)
	id := saveDash(t, h, "Triage", `{"html":"<p/>","datasources":{"by_status":{"sql":"select 1"}}}`)
	rec := send(t, h, http.MethodGet, dashBase+*id+"/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get row: %d %s", rec.Code, rec.Body.String())
	}
	var row struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Config    json.RawMessage `json:"config"`
		UpdatedAt string          `json:"updated_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &row); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if row.ID != *id || row.Name != "Triage" || row.UpdatedAt == "" {
		t.Fatalf("row = %+v", row)
	}
	if !strings.Contains(string(row.Config), "by_status") {
		t.Fatalf("config missing datasources: %s", row.Config)
	}
	if rec := send(t, h, http.MethodGet, dashBase+"nosuch/", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d, want 404", rec.Code)
	}
}

func TestDashboardRenderCorruptConfig(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	// The store does not validate (validation lives at writers), so a torn
	// or hand-edited row can exist. render/data must diagnose, not serve it.
	saved, err := db.SaveDashboard(context.Background(), "broken", `{"html":`)
	if err != nil {
		t.Fatalf("seed corrupt: %v", err)
	}
	if rec := send(t, h, http.MethodGet, dashBase+saved.ID+"/render/", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("render corrupt: %d, want 500", rec.Code)
	}
	if rec := send(t, h, http.MethodGet, dashBase+saved.ID+"/data/a/", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("data corrupt: %d, want 500", rec.Code)
	}
}

func TestDashboardDataSQL(t *testing.T) {
	h := dashHandler(t)
	id := saveDash(t, h, "Counts", `{
		"html":"<p/>",
		"datasources": {
			"by_cat": {"sql": "SELECT status_category, COUNT(*) AS n FROM issues GROUP BY 1 ORDER BY 1"},
			"trap":   {"sql": "SELECT key FROM issues WHERE status = 'In Progress'"}
		}
	}`)
	rec := send(t, h, http.MethodGet, dashBase+*id+"/data/by_cat/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("data: %d %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Columns   []string `json:"columns"`
		Rows      [][]any  `json:"rows"`
		Truncated bool     `json:"truncated"`
		Warning   string   `json:"warning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Columns) != 2 || res.Columns[0] != "status_category" || len(res.Rows) != 3 {
		t.Fatalf("by_cat = %+v", res)
	}
	// The fixture's statuses are Korean; the display-name trap datasource
	// answers 0 rows WITH the warning — an English display name never
	// silently empties a card.
	rec = send(t, h, http.MethodGet, dashBase+*id+"/data/trap/", "")
	json.Unmarshal(rec.Body.Bytes(), &res)
	if len(res.Rows) != 0 || res.Warning == "" {
		t.Fatalf("trap rows=%d warning=%q", len(res.Rows), res.Warning)
	}
}

func TestDashboardDataJQL(t *testing.T) {
	h := dashHandler(t)
	id := saveDash(t, h, "Mine", `{
		"html":"<p/>",
		"datasources": {"mine": {"jql": "assignee = currentUser() AND resolution is EMPTY"}}
	}`)
	rec := send(t, h, http.MethodGet, dashBase+*id+"/data/mine/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("data: %d %s", rec.Code, rec.Body.String())
	}
	var res struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []string{"issue_key", "summary", "status_category", "status", "priority_rank", "updated_at"}
	if len(res.Columns) != len(want) {
		t.Fatalf("columns = %v", res.Columns)
	}
	for i, c := range want {
		if res.Columns[i] != c {
			t.Fatalf("columns = %v", res.Columns)
		}
	}
	if len(res.Rows) != 1 || res.Rows[0][0] != "NMB-1" {
		t.Fatalf("mine rows = %v", res.Rows)
	}
}

func TestDashboardDataWriteSQLRefused(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	id := saveDash(t, h, "Evil", `{
		"html":"<p/>",
		"datasources": {"pwn": {"sql": "UPDATE issues SET title = 'pwned' WHERE key = 'NMB-1'"}}
	}`)
	rec := send(t, h, http.MethodGet, dashBase+*id+"/data/pwn/", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("write datasource: %d %s, want 400", rec.Code, rec.Body.String())
	}
	var e struct {
		Error string `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &e)
	if e.Error != "sql_error" {
		t.Fatalf("error code = %s", e.Error)
	}
	// The mirror row is what it was — checked on the same db the handler
	// serves, not a fresh fixture.
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatalf("lites: %v", err)
	}
	for _, l := range lites {
		if l.IssueKey == "NMB-1" && l.Summary == "pwned" {
			t.Fatal("write SQL mutated the mirror through the API")
		}
	}
}

func TestDashboardDataErrors(t *testing.T) {
	h := dashHandler(t)
	id := saveDash(t, h, "E", `{
		"html":"<p/>",
		"datasources": {
			"bad_sql": {"sql": "SELECT no_such_column FROM issues"},
			"bad_jql": {"jql": "watchers > 1"},
			"parse_jql": {"jql": "project ="}
		}
	}`)
	cases := []struct {
		path string
		code string
		want int
	}{
		{dashBase + "nosuchid/render/", "not_found", http.StatusNotFound},
		{dashBase + *id + "/data/nosuch/", "datasource_not_found", http.StatusNotFound},
		{dashBase + *id + "/data/bad_sql/", "sql_error", http.StatusBadRequest},
		{dashBase + *id + "/data/bad_jql/", "jql_error", http.StatusBadRequest},
		{dashBase + *id + "/data/parse_jql/", "jql_error", http.StatusBadRequest},
	}
	for _, tc := range cases {
		rec := send(t, h, http.MethodGet, tc.path, "")
		if rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", tc.path, rec.Code, tc.want, rec.Body.String())
			continue
		}
		var e struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error != tc.code {
			t.Errorf("%s: body %s, want error %s", tc.path, rec.Body.String(), tc.code)
		}
	}
	// Path traversal as an id never reaches a row: the mux cleans ../ and
	// answers a redirect; the cleaned path misses in the store. What matters
	// is that no render body comes back.
	trav := send(t, h, http.MethodGet, dashBase+"../..%2F/render/", "")
	if trav.Code == http.StatusOK || strings.Contains(trav.Body.String(), "<p>") {
		t.Fatalf("traversal id served a dashboard: %d %s", trav.Code, trav.Body.String())
	}
	// The refused jql/sql calls never mutated anything.
	var list struct {
		Version int `json:"version"`
	}
	rec := send(t, h, http.MethodGet, dashBase, "")
	json.Unmarshal(rec.Body.Bytes(), &list)
	if list.Version != 1 {
		t.Fatalf("version = %d, want 1", list.Version)
	}
}

func TestDashboardAbsorb(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	kept := saveDash(t, h, "kept", `{"html":"<p>stored</p>"}`)

	// Incoming: a name the server owns (must lose), and a fresh name whose
	// id collides with the stored row (must be reminted, not overwrite).
	incoming := `{"dashboards":[
		{"id":"x1","name":"kept","config":{"html":"<p>incoming</p>"}},
		{"id":"` + *kept + `","name":"fresh","config":{"html":"<p>new</p>"}}
	]}`
	rec := send(t, h, http.MethodPost, dashBase+"absorb/", incoming)
	if rec.Code != http.StatusOK {
		t.Fatalf("absorb: %d %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Version    int               `json:"version"`
		Dashboards []listedDashboard `json:"dashboards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Dashboards) != 2 {
		t.Fatalf("after absorb = %+v", list.Dashboards)
	}
	var freshID string
	for _, d := range list.Dashboards {
		switch d.Name {
		case "kept":
			if d.ID != *kept {
				t.Fatalf("stored row lost its id: %+v", d)
			}
		case "fresh":
			freshID = d.ID
		}
	}
	if freshID == "" || freshID == *kept {
		t.Fatalf("colliding id was not reminted: %q vs %q", freshID, *kept)
	}
	// The name the server owned kept its own config.
	stored, err := db.Dashboard(context.Background(), *kept)
	if err != nil {
		t.Fatalf("stored: %v", err)
	}
	if !strings.Contains(string(stored.Config), "stored") {
		t.Fatalf("stored config replaced by incoming: %s", stored.Config)
	}
}

// dashLibsHandler is dashHandler plus the two things lib tests need: a
// GADAK_HOME (the serve route reads the cache from this server's profile
// dir) and that home's lib cache directory, so a test can seed or tamper
// with the exact bytes the route serves.
func dashLibsHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	db, cfg := fixture(t)
	return New(db, cfg), dashboards.LibsDir(home)
}

// seedLib adds one library through the real LibAdd (loopback mock CDN) and
// returns its manifest entry.
func seedLib(t *testing.T, libsDir, body, name string) dashboards.Lib {
	t.Helper()
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(cdn.Close)
	lib, _, err := dashboards.LibAdd(context.Background(), libsDir, cdn.URL+"/"+name, false, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("seed lib add: %v", err)
	}
	return lib
}

// TestDashboardLibRoute (GDK-808): the libs route serves cache bytes with a
// fixed content type, CORS for the sandbox frame, and — the whole point of
// the pin — re-hashes the bytes before they leave. A cache file modified
// after lib add answers 500, never executes. Unknown and path-shaped ids
// miss like the vendor route. (The route is the vendor route's no-slash
// single-segment shape: a trailing-slash variant would be ambiguous with
// {id}/render/ and panic the mux.)
func TestDashboardLibRoute(t *testing.T) {
	h, libsDir := dashLibsHandler(t)
	lib := seedLib(t, libsDir, "marker=1;", "demo.iife.js")

	rec := send(t, h, http.MethodGet, dashBase+"libs/"+lib.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("serve lib: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/javascript" {
		t.Errorf("Content-Type = %q, want application/javascript (fixed — the cache stores bytes, not claims)", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("nosniff = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want * (module imports from the sandboxed frame)", got)
	}
	if rec.Body.String() != "marker=1;" {
		t.Errorf("body = %q", rec.Body.String())
	}

	// Unknown id and traversal shapes never reach a file. ../-shapes answer
	// with ServeMux's clean-path redirect — what matters (vendor route
	// convention) is that no cache body comes back.
	for _, id := range []string{"nosuch.js", "..%2f..%2fetc%2fpasswd", "sub/dir"} {
		if rec := send(t, h, http.MethodGet, dashBase+"libs/"+id, ""); rec.Code != http.StatusNotFound {
			t.Errorf("libs/%s: %d, want 404", id, rec.Code)
		}
	}
	if rec := send(t, h, http.MethodGet, dashBase+"libs/../"+lib.ID, ""); rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "marker=1;") {
		t.Errorf("libs traversal: status %d, body %q — must not serve cache bytes", rec.Code, rec.Body.String())
	}

	// THE pin: tampered bytes are refused at serve time (500), not executed.
	tampered := filepath.Join(libsDir, lib.ID)
	if err := os.WriteFile(tampered, []byte("marker=2; // evil"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rec := send(t, h, http.MethodGet, dashBase+"libs/"+lib.ID, ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("tampered cache served: status %d body %q, want 500 (sha384 re-verification)", rec.Code, rec.Body.String())
	}
	// A missing cache file is the same class: operator problem, 500.
	if err := os.Remove(tampered); err != nil {
		t.Fatal(err)
	}
	if rec := send(t, h, http.MethodGet, dashBase+"libs/"+lib.ID, ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("missing cache file served: %d, want 500", rec.Code)
	}
}

// TestDashboardSaveLibs (GDK-808): a config may only reference libs the
// cache actually holds. An unknown id is a 400 whose message carries the
// recipe, because this endpoint's caller is an agent that can act on it.
func TestDashboardSaveLibs(t *testing.T) {
	h, libsDir := dashLibsHandler(t)
	lib := seedLib(t, libsDir, "ok=1;", "ok.iife.js")

	rec := send(t, h, http.MethodPost, dashBase,
		`{"name":"badlib","config":{"html":"<p/>","libs":["ffffffffffffffff-nosuch.js"]}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown lib save: %d %s, want 400", rec.Code, rec.Body.String())
	}
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e.Error != "unknown_lib" {
		t.Fatalf("body %s, want error unknown_lib", rec.Body.String())
	}
	if !strings.Contains(e.Message, "ffffffffffffffff-nosuch.js") || !strings.Contains(e.Message, "gadak dashboards lib add") {
		t.Fatalf("message %q must name the id and the lib add recipe", e.Message)
	}

	// A declared id that exists saves fine, and the row round-trips it.
	id := saveDash(t, h, "withlib", `{"html":"<p/>","libs":["`+lib.ID+`"]}`)
	rec = send(t, h, http.MethodGet, dashBase+*id+"/", "")
	if !strings.Contains(rec.Body.String(), lib.ID) {
		t.Fatalf("saved row lost the libs declaration: %s", rec.Body.String())
	}
}

// TestDashboardRenderLibs (GDK-808): declared libs are injected as deferred
// scripts in declaration order (head, or prepended for headless documents)
// and widen only script-src with the libs path — style stays vendor-only,
// and a dashboard that declares nothing renders byte-identical to pre-808.
func TestDashboardRenderLibs(t *testing.T) {
	h, libsDir := dashLibsHandler(t)
	first := seedLib(t, libsDir, "one=1;", "one.iife.js")
	second := seedLib(t, libsDir, "two=2;", "two.iife.js")

	id := saveDash(t, h, "W", `{"html":"<!doctype html><html><head><title>t</title></head><body><p/></body></html>","libs":["`+first.ID+`","`+second.ID+`"]}`)
	req := testRequest(http.MethodGet, dashBase+*id+"/render/", nil)
	req.Host = "127.0.0.1:7877"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("render: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	tag1 := `<script src="/api/v1/dashboards/libs/` + first.ID + `" defer onerror="document.documentElement.setAttribute('data-gadak-lib-error','` + first.ID + `')"></script>`
	tag2 := `<script src="/api/v1/dashboards/libs/` + second.ID + `" defer onerror="document.documentElement.setAttribute('data-gadak-lib-error','` + second.ID + `')"></script>`
	// Injection point: right after <head>, in declaration order.
	headAt := strings.Index(strings.ToLower(body), "<head>")
	if i1, i2 := strings.Index(body, tag1), strings.Index(body, tag2); headAt < 0 || i1 < headAt || i2 < i1 {
		t.Fatalf("libs not injected after <head> in order:\n%s", body)
	}
	// CSP: libs path joins script-src only.
	csp := rec.Header().Get("Content-Security-Policy")
	wantLibs := "http://127.0.0.1:7877/api/v1/dashboards/libs/"
	if !strings.Contains(csp, "script-src 'unsafe-inline' http://127.0.0.1:7877"+dashVendorPath+" "+wantLibs+";") {
		t.Errorf("script-src with libs = %q", csp)
	}
	if strings.Contains(strings.SplitN(csp, "style-src ", 2)[1], "libs/") {
		t.Errorf("style-src widened with the libs path: %q", csp)
	}

	// Headless document: tags prepend (the browser synthesizes that head).
	id2 := saveDash(t, h, "H", `{"html":"<p>no head</p>","libs":["`+first.ID+`"]}`)
	rec = httptest.NewRecorder()
	req = testRequest(http.MethodGet, dashBase+*id2+"/render/", nil)
	req.Host = "127.0.0.1:7877"
	h.ServeHTTP(rec, req)
	if !strings.HasPrefix(rec.Body.String(), `<script src="/api/v1/dashboards/libs/`) {
		t.Fatalf("headless injection = %q", rec.Body.String())
	}

	// No declaration: policy and body unchanged — least privilege default.
	plain := saveDash(t, h, "P", `{"html":"<p>plain</p>"}`)
	req = testRequest(http.MethodGet, dashBase+*plain+"/render/", nil)
	req.Host = "127.0.0.1:7877"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.String() != "<p>plain</p>" {
		t.Errorf("undeclared render body changed: %q", rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); strings.Contains(csp, "libs/") {
		t.Errorf("undeclared render CSP widened: %q", csp)
	}
}
