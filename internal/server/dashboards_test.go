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
//	render: html bytes + exact CSP      | TestDashboardRenderCSP (all four headers, body bytes)
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
	"net/http"
	"strings"
	"testing"

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
	rec := send(t, h, http.MethodGet, dashBase+*id+"/render/", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("render: %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != dashboardCSP {
		t.Errorf("CSP = %q, want %q", got, dashboardCSP)
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
	// The CSP constant itself is the contract — no host may ever join it.
	if dashboardCSP != `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:` {
		t.Errorf("dashboardCSP drifted: %q", dashboardCSP)
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
