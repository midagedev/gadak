package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// GDK-1665, measured on Jira Software 11.3.11 DC: Cloud's board listing
// carries location.projectKey on every board; Server's does not, so every
// boards.project_key the plain listing fills on Cloud lands empty on Server
// and the board→project mapping is gone. The backfill is two-fold, and this
// test exercises both halves in one sync:
//
//   - ② GET /board?projectKeyOrId=KEY, one request per *configured*
//     project — the path the seed scripts use, and the bounded one for the
//     every-tick importAgile (GDK-1661): board 1 of configured NMB resolves
//     through it;
//   - ① GET /board/{id}/project, one request per board still unmapped after
//     ② — boards of projects the config does not list (here: MTX) and the
//     workspace with no configured projects at all.
//
// FAIL-first on the unmodified tree: both boards stored project_key = ”
// and neither project route was requested — the listing's payload order was
// the only input the importer had.
func TestServerBoardListingBackfillsProjectKeys(t *testing.T) {
	issue := map[string]any{
		"id": "9001", "key": "NMB-1",
		"fields": map[string]any{
			"summary":   "issue that keeps the NMB project in scope",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10004", "name": "Bug"},
			"status": map[string]any{
				"id": "5", "name": "Done",
				"statusCategory": map[string]any{"key": "done"},
			},
			"created": "2026-09-01T10:00:00.000+0000",
			"updated": "2026-09-02T10:00:00.000+0000",
		},
	}
	issueJSON, err := json.Marshal(issue)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var projectScoped, perBoardProject int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/search":
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":100,"total":1,"issues":[` + string(issueJSON) + `]}`))
		case "/rest/api/2/status":
			_, _ = w.Write([]byte(`[{"id":"5","name":"Done","statusCategory":{"key":"done"}}]`))
		case "/rest/api/2/priority":
			_, _ = w.Write([]byte(`[{"id":"2","name":"High"}]`))
		case "/rest/api/2/issueLinkType":
			_, _ = w.Write([]byte(`{"issueLinkTypes":[]}`))
		case "/rest/api/2/filter/favourite": // Server has no /filter/my (GDK-1652)
			_, _ = w.Write([]byte(`[]`))
		case "/rest/api/2/field":
			_, _ = w.Write([]byte(`[]`))
		case "/rest/api/2/project/NMB/versions":
			_, _ = w.Write([]byte(`[]`))
		case "/rest/agile/1.0/board":
			// The Server shape: no location on any board (the measure's
			// four-board listing, trimmed to two). The project-scoped
			// variant ② is the same path with projectKeyOrId set.
			if key := r.URL.Query().Get("projectKeyOrId"); key != "" {
				mu.Lock()
				projectScoped++
				mu.Unlock()
				if key != "NMB" {
					t.Errorf("project-scoped board listing asked for %q", key)
				}
				_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":1,"name":"NMB board","type":"scrum"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[` +
				`{"id":1,"name":"NMB board","type":"scrum"},` +
				`{"id":2,"name":"MTX board","type":"scrum"}]}`))
		case "/rest/agile/1.0/board/1/sprint", "/rest/agile/1.0/board/2/sprint":
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[]}`))
		case "/rest/agile/1.0/board/2/project":
			// Fallback ①: the board the config does not name.
			mu.Lock()
			perBoardProject++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"id":"10002","key":"MTX","name":"Matrix"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0 // no sleeping in tests

	cfg := &config.Config{
		Kind: config.OriginJiraServer, Site: srv.URL, Token: "pat-token",
		Projects: []string{"NMB"},
	}
	db := newMirror(t)
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}

	boardKey := func(id int64) string {
		var key string
		if err := db.DB.QueryRow(`SELECT project_key FROM boards WHERE source_id = ? AND id = ?`, SourceID, id).Scan(&key); err != nil {
			t.Fatalf("board %d: %v", id, err)
		}
		return key
	}
	if got := boardKey(1); got != "NMB" {
		t.Errorf("board 1 project_key = %q, want NMB — the configured project resolves through the project-scoped listing", got)
	}
	if got := boardKey(2); got != "MTX" {
		t.Errorf("board 2 project_key = %q, want MTX — an unconfigured board resolves through the per-board project read", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if projectScoped == 0 {
		t.Error("the project-scoped board listing (②) was never requested")
	}
	if perBoardProject == 0 {
		t.Error("the per-board project read (①) was never requested")
	}
	if projectScoped > 1 {
		t.Errorf("project-scoped listing ran %d times, want one per configured project", projectScoped)
	}
}

// The same backfill on a workspace with no configured projects: there is no
// ② to run (no project key to scope by), so every unmapped board takes the
// per-board read ① — the issue's stated split, pinned here so an empty
// Projects list can never take the ② branch.
//
// FAIL-first on the unmodified tree: board 7 stored project_key = ” and
// /board/7/project was never requested.
func TestServerBoardBackfillWithoutConfiguredProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/agile/1.0/board":
			if r.URL.Query().Get("projectKeyOrId") != "" {
				t.Errorf("project-scoped listing requested on a workspace with no configured projects: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":7,"name":"solo board","type":"kanban"}]}`))
		case "/rest/agile/1.0/board/7/sprint":
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[]}`))
		case "/rest/agile/1.0/board/7/project":
			// The paged envelope Jira Software answers this route with —
			// the other shape (a bare project object) is board 2 above.
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":"10003","key":"SOLO","name":"Solo"}]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0

	db := newMirror(t)
	// The boards rows carry a source_id foreign key; without the source the
	// insert fails and importAgile logs it instead of returning.
	if err := db.DB.UpsertSource(context.Background(), store.Source{ID: SourceID, Kind: "jira", BaseURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	importAgile(context.Background(), client, nil, db.DB, Options{})

	var key string
	if err := db.DB.QueryRow(`SELECT project_key FROM boards WHERE source_id = ? AND id = 7`, SourceID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if key != "SOLO" {
		t.Fatalf("board 7 project_key = %q, want SOLO through the per-board read", key)
	}
}

// A Cloud-shaped listing — location.projectKey present — needs neither
// backfill: no board project request leaves the pass, and the key the
// location carried survives. The backfill is for the boards the listing left
// unmapped, not a second opinion on every board.
func TestCloudBoardLocationNeedsNoBackfill(t *testing.T) {
	var asked bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board" && r.URL.Query().Get("projectKeyOrId") == "":
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":1,"name":"NMB board","type":"scrum","location":{"projectKey":"NMB"}}]}`))
		case strings.HasSuffix(r.URL.Path, "/project"):
			asked = true
			http.Error(w, "no", http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/rest/agile/1.0/board/"):
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[]}`))
		default:
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "tok")
	client.Retries, client.Backoff = 1, 0

	db := newMirror(t)
	// The boards rows carry a source_id foreign key; without the source the
	// insert fails and importAgile logs it instead of returning.
	if err := db.DB.UpsertSource(context.Background(), store.Source{ID: SourceID, Kind: "jira", BaseURL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	importAgile(context.Background(), client, &config.Config{Projects: []string{"NMB"}}, db.DB, Options{})

	var key string
	if err := db.DB.QueryRow(`SELECT project_key FROM boards WHERE source_id = ? AND id = 1`, SourceID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if key != "NMB" {
		t.Fatalf("board 1 project_key = %q, want the location's NMB", key)
	}
	if asked {
		t.Error("a board the location already mapped must not pay the per-board project read")
	}
}
