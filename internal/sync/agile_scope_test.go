package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// GDK-2069, measured by a user on a public Jira 10.3 Server with
// projects=[CALCITE]: the site lists 265 boards, none of them the configured
// project's, and a quiet incremental sync still spent 563 requests (559 of
// them agile, 115 s) — the board listing was site-wide and every kanban
// board was asked for sprints it cannot have, once per 60 s serve tick.
// These tests pin the scoped shape: a workspace with configured projects
// lists boards per project, never reads the site-wide listing or the
// per-board project fallback, and asks for sprints only from boards that can
// hold them (kanban cannot; scrum and simple can).
//
// FAIL-first on the unmodified tree, quoted in the round report: the
// site-wide /board listing ran once, /board/{id}/project ran per
// unconfigured board, and every board — kanban included — was asked for
// sprints.

// agileCounts tallies requests by route, the axis the GDK-2069 contract is
// written on. Handlers run on the server's goroutines, so every bump locks.
type agileCounts struct {
	mu           sync.Mutex
	unscoped     int            // GET /board with no projectKeyOrId
	boardProject int            // GET /board/{id}/project
	scoped       map[string]int // GET /board?projectKeyOrId=KEY
	sprint       map[int64]int  // GET /board/{id}/sprint, by board id
}

func newAgileCounts() *agileCounts {
	return &agileCounts{scoped: map[string]int{}, sprint: map[int64]int{}}
}

func (a *agileCounts) bumpUnscoped()             { a.mu.Lock(); a.unscoped++; a.mu.Unlock() }
func (a *agileCounts) bumpBoardProject()         { a.mu.Lock(); a.boardProject++; a.mu.Unlock() }
func (a *agileCounts) bumpScoped(project string) { a.mu.Lock(); a.scoped[project]++; a.mu.Unlock() }
func (a *agileCounts) bumpSprint(boardID int64)  { a.mu.Lock(); a.sprint[boardID]++; a.mu.Unlock() }

func (a *agileCounts) snapshot() (unscoped, boardProject int, scoped map[string]int, sprint map[int64]int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	scoped = map[string]int{}
	for k, v := range a.scoped {
		scoped[k] = v
	}
	sprint = map[int64]int{}
	for k, v := range a.sprint {
		sprint[k] = v
	}
	return a.unscoped, a.boardProject, scoped, sprint
}

// agileBoard builds one board row. locationKey "" is the Server shape — no
// location key at all, the measured GDK-1665 case — anything else carries
// location.projectKey the way Cloud does.
func agileBoard(id int, name, typ, locationKey string) string {
	loc := ""
	if locationKey != "" {
		loc = `,"location":{"projectKey":"` + locationKey + `"}`
	}
	return `{"id":` + strconv.Itoa(id) + `,"name":"` + name + `","type":"` + typ + `"` + loc + `}`
}

// agileValues wraps board or sprint rows in the one-page envelope both
// Jiras answer (isLast is authoritative; total is absent on Server).
func agileValues(values ...string) string {
	return `{"maxResults":50,"startAt":0,"isLast":true,"values":[` + strings.Join(values, ",") + `]}`
}

// agileSprintBoardID reads the board id out of
// /rest/agile/1.0/board/{id}/sprint. The route guard at the call site fixed
// the shape, so the parse cannot fail.
func agileSprintBoardID(path string) int64 {
	id, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(path, "/rest/agile/1.0/board/"), "/sprint"), 10, 64)
	return id
}

// agileScopeMirror is a mirror whose source row exists — boards carry a
// source_id foreign key, and without the source the ReplaceAgile insert
// fails and importAgile logs it instead of returning.
func agileScopeMirror(t *testing.T, srvURL string) *mirror {
	t.Helper()
	db := newMirror(t)
	if err := db.DB.UpsertSource(context.Background(), store.Source{ID: SourceID, Kind: "jira", BaseURL: srvURL}); err != nil {
		t.Fatal(err)
	}
	return db
}

// The scoped listing reads one project's boards and nothing else: no
// site-wide /board, no /board/{id}/project (the listing itself names the
// project), no sprint read for the kanban board. Thirty boards on the site,
// two of them NMB's — the public-server shape (265 visible, 0 configured) at
// a count a regression cannot hide behind.
func TestAgileScopedBoardListingSkipsSiteWideReads(t *testing.T) {
	counts := newAgileCounts()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/rest/agile/1.0/board":
			if key := r.URL.Query().Get("projectKeyOrId"); key != "" {
				counts.bumpScoped(key)
				if key != "NMB" {
					t.Errorf("scoped listing asked for %q, want NMB only", key)
					http.Error(w, "no", http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(agileValues(
					agileBoard(1, "NMB scrum board", "scrum", ""),
					agileBoard(2, "NMB kanban board", "kanban", ""),
				)))
				return
			}
			counts.bumpUnscoped()
			values := make([]string, 0, 30)
			for id := 1; id <= 30; id++ {
				name, typ := "Other board "+strconv.Itoa(id), "scrum"
				switch id {
				case 1:
					name, typ = "NMB scrum board", "scrum"
				case 2:
					name, typ = "NMB kanban board", "kanban"
				default:
					if id%2 == 0 {
						typ = "kanban"
					}
				}
				values = append(values, agileBoard(id, name, typ, ""))
			}
			_, _ = w.Write([]byte(agileValues(values...)))
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/sprint"):
			id := agileSprintBoardID(p)
			counts.bumpSprint(id)
			switch {
			case id == 1:
				_, _ = w.Write([]byte(agileValues(`{"id":501,"name":"NMB sprint 1","state":"active","originBoardId":1}`)))
			case id%2 == 0:
				// The kanban answer the measure saw, kept so a regression
				// request reads as itself in the counts.
				http.Error(w, `{"errorMessages":["The board doesn't support sprints."]}`, http.StatusBadRequest)
			default:
				_, _ = w.Write([]byte(agileValues()))
			}
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/project"):
			counts.bumpBoardProject()
			_, _ = w.Write([]byte(`{"id":"10090","key":"OTHER","name":"Other"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, p)
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
	db := agileScopeMirror(t, srv.URL)
	importAgile(context.Background(), client, cfg, db.DB, Options{})

	unscoped, boardProject, scoped, sprint := counts.snapshot()
	if unscoped != 0 {
		t.Errorf("site-wide /board listing ran %d times, want 0 — configured projects scope it away (GDK-2069)", unscoped)
	}
	if scoped["NMB"] != 1 {
		t.Errorf("project-scoped listing ran %d times for NMB, want exactly 1", scoped["NMB"])
	}
	if boardProject != 0 {
		t.Errorf("/board/{id}/project ran %d times, want 0 — the scoped listing already names the project", boardProject)
	}
	if sprint[1] != 1 || sprint[2] != 0 || len(sprint) != 1 {
		t.Errorf("sprint listings = %v, want board 1 once and board 2 (kanban) never", sprint)
	}
	var n int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM boards WHERE source_id = ?`, SourceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("boards rows = %d, want 2 — the twenty-eight unconfigured boards must not be mirrored", n)
	}
	var key1, key2, typ2 string
	if err := db.DB.QueryRow(`SELECT project_key FROM boards WHERE source_id = ? AND id = 1`, SourceID).Scan(&key1); err != nil {
		t.Fatal(err)
	}
	if key1 != "NMB" {
		t.Errorf("board 1 project_key = %q, want NMB — the asked project is the answer when the listing carries no location", key1)
	}
	if err := db.DB.QueryRow(`SELECT project_key, type FROM boards WHERE source_id = ? AND id = 2`, SourceID).Scan(&key2, &typ2); err != nil {
		t.Fatal(err)
	}
	if key2 != "NMB" || typ2 != "kanban" {
		t.Errorf("board 2 = %q/%q, want NMB/kanban — the kanban row is stored, only its sprint read is skipped", key2, typ2)
	}
	var sprintN int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM sprints WHERE source_id = ?`, SourceID).Scan(&sprintN); err != nil {
		t.Fatal(err)
	}
	if sprintN != 1 {
		t.Errorf("sprints rows = %d, want 1 (board 1's)", sprintN)
	}
}

// A board whose filter references two configured projects comes back from
// both listings — that is Jira's documented meaning of the projectKeyOrId
// filter. It is one board: stored once, sprints asked once, and the first
// configured project wins its project key where the listing carries no
// location (the Server shape); where it carries one (board 3, a cross-project
// board living in LOC), the location wins over the project asked.
func TestAgileScopedListingDedupesSharedBoards(t *testing.T) {
	counts := newAgileCounts()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/rest/agile/1.0/board":
			switch r.URL.Query().Get("projectKeyOrId") {
			case "NMB":
				counts.bumpScoped("NMB")
				_, _ = w.Write([]byte(agileValues(
					agileBoard(1, "shared scrum board", "scrum", ""),
					agileBoard(2, "NMB kanban board", "kanban", ""),
				)))
			case "NMA":
				counts.bumpScoped("NMA")
				_, _ = w.Write([]byte(agileValues(
					agileBoard(1, "shared scrum board", "scrum", ""),
					agileBoard(3, "NMA scrum board", "scrum", "LOC"),
				)))
			default:
				counts.bumpUnscoped()
				_, _ = w.Write([]byte(agileValues(
					agileBoard(1, "shared scrum board", "scrum", ""),
					agileBoard(2, "NMB kanban board", "kanban", ""),
					agileBoard(3, "NMA scrum board", "scrum", "LOC"),
				)))
			}
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/sprint"):
			id := agileSprintBoardID(p)
			counts.bumpSprint(id)
			switch id {
			case 1:
				_, _ = w.Write([]byte(agileValues(`{"id":701,"name":"shared board sprint","state":"active","originBoardId":1}`)))
			case 3:
				_, _ = w.Write([]byte(agileValues(`{"id":703,"name":"NMA sprint","state":"future","originBoardId":3}`)))
			default:
				http.Error(w, `{"errorMessages":["The board doesn't support sprints."]}`, http.StatusBadRequest)
			}
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/project"):
			counts.bumpBoardProject()
			http.Error(w, "no", http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, p)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0

	cfg := &config.Config{
		Kind: config.OriginJiraServer, Site: srv.URL, Token: "pat-token",
		Projects: []string{"NMB", "NMA"},
	}
	db := agileScopeMirror(t, srv.URL)
	importAgile(context.Background(), client, cfg, db.DB, Options{})

	unscoped, boardProject, scoped, sprint := counts.snapshot()
	if unscoped != 0 {
		t.Errorf("site-wide /board listing ran %d times, want 0", unscoped)
	}
	if scoped["NMB"] != 1 || scoped["NMA"] != 1 {
		t.Errorf("scoped listings = %v, want exactly one per configured project", scoped)
	}
	if boardProject != 0 {
		t.Errorf("/board/{id}/project ran %d times, want 0", boardProject)
	}
	if sprint[1] != 1 {
		t.Errorf("board 1 sprint listing ran %d times, want 1 — a board two listings return is still one board", sprint[1])
	}
	if sprint[2] != 0 || len(sprint) != 2 {
		t.Errorf("sprint listings = %v, want boards 1 and 3 once each and the kanban never", sprint)
	}
	var n, n1 int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM boards WHERE source_id = ?`, SourceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("boards rows = %d, want 3 (ids 1, 2, 3)", n)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM boards WHERE source_id = ? AND id = 1`, SourceID).Scan(&n1); err != nil {
		t.Fatal(err)
	}
	if n1 != 1 {
		t.Errorf("board 1 stored %d times, want 1 — the union de-duplicates by board id", n1)
	}
	var key1, key3 string
	if err := db.DB.QueryRow(`SELECT project_key FROM boards WHERE source_id = ? AND id = 1`, SourceID).Scan(&key1); err != nil {
		t.Fatal(err)
	}
	if key1 != "NMB" {
		t.Errorf("board 1 project_key = %q, want NMB — the first configured project wins when the listing carries no location", key1)
	}
	if err := db.DB.QueryRow(`SELECT project_key FROM boards WHERE source_id = ? AND id = 3`, SourceID).Scan(&key3); err != nil {
		t.Fatal(err)
	}
	if key3 != "LOC" {
		t.Errorf("board 3 project_key = %q, want LOC — the location the listing carried beats the project asked", key3)
	}
}

// ReplaceAgile is wholesale, so a pass whose NMA listing 500s must not store
// the NMB-only union — that would delete NMA's boards and sprints. The pass
// keeps the previous rows, exactly like a failed site-wide listing always
// has, says one line naming the project, and asks for nothing else.
func TestAgileScopedListingFailureKeepsPreviousRows(t *testing.T) {
	counts := newAgileCounts()
	nmaFails := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/rest/agile/1.0/board":
			switch r.URL.Query().Get("projectKeyOrId") {
			case "NMB":
				counts.bumpScoped("NMB")
				_, _ = w.Write([]byte(agileValues(
					agileBoard(1, "NMB scrum board", "scrum", ""),
				)))
			case "NMA":
				counts.bumpScoped("NMA")
				if nmaFails {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				_, _ = w.Write([]byte(agileValues(
					agileBoard(9, "NMA scrum board", "scrum", ""),
				)))
			default:
				counts.bumpUnscoped()
				_, _ = w.Write([]byte(agileValues(
					agileBoard(1, "NMB scrum board", "scrum", ""),
					agileBoard(9, "NMA scrum board", "scrum", ""),
				)))
			}
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/sprint"):
			id := agileSprintBoardID(p)
			counts.bumpSprint(id)
			switch id {
			case 1:
				_, _ = w.Write([]byte(agileValues(`{"id":601,"name":"NMB sprint","state":"active","originBoardId":1}`)))
			case 9:
				_, _ = w.Write([]byte(agileValues(`{"id":609,"name":"NMA sprint","state":"active","originBoardId":9}`)))
			default:
				t.Errorf("unexpected sprint read for board %d", id)
				http.Error(w, "no", http.StatusNotFound)
			}
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/project"):
			counts.bumpBoardProject()
			http.Error(w, "no", http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, p)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0

	cfg := &config.Config{
		Kind: config.OriginJiraServer, Site: srv.URL, Token: "pat-token",
		Projects: []string{"NMB", "NMA"},
	}
	db := agileScopeMirror(t, srv.URL)
	var lines []string
	log := func(line string) { lines = append(lines, line) }

	importAgile(context.Background(), client, cfg, db.DB, Options{Log: log})
	var n, sprintN int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM boards WHERE source_id = ?`, SourceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM sprints WHERE source_id = ?`, SourceID).Scan(&sprintN); err != nil {
		t.Fatal(err)
	}
	if n != 2 || sprintN != 2 {
		t.Fatalf("after the healthy pass: %d boards, %d sprints, want 2 and 2", n, sprintN)
	}

	// The NMA listing starts failing. This pass must keep every row above.
	nmaFails = true
	importAgile(context.Background(), client, cfg, db.DB, Options{Log: log})

	if !strings.Contains(strings.Join(lines, "\n"), "project NMA listing skipped") {
		t.Errorf("the failed listing must be one line naming the project; log = %q", lines)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM boards WHERE source_id = ?`, SourceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("boards rows = %d after the failed pass, want 2 — ReplaceAgile must not run on a partial union", n)
	}
	var id int64
	if err := db.DB.QueryRow(`SELECT id FROM boards WHERE source_id = ? AND id = 9`, SourceID).Scan(&id); err != nil {
		t.Errorf("NMA's board 9 was deleted by the failed pass: %v", err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM sprints WHERE source_id = ?`, SourceID).Scan(&sprintN); err != nil {
		t.Fatal(err)
	}
	if sprintN != 2 {
		t.Errorf("sprints rows = %d after the failed pass, want 2", sprintN)
	}
	unscoped, boardProject, _, sprint := counts.snapshot()
	if unscoped != 0 {
		t.Errorf("site-wide /board listing ran %d times, want 0", unscoped)
	}
	if boardProject != 0 {
		t.Errorf("/board/{id}/project ran %d times, want 0", boardProject)
	}
	if sprint[1] != 1 || sprint[9] != 1 || len(sprint) != 2 {
		t.Errorf("sprint listings = %v, want the two healthy-pass reads only — a failed pass asks for nothing else", sprint)
	}
}

// The kanban rule is the board's shape, not the listing's: the site-wide
// path (no configured projects) skips the sprint read for a kanban board
// too, and still stores its row. The measured spam was exactly this ask on
// every kanban board of a public server.
func TestAgileUnscopedKanbanBoardsAreNotAskedForSprints(t *testing.T) {
	counts := newAgileCounts()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/rest/agile/1.0/board":
			if key := r.URL.Query().Get("projectKeyOrId"); key != "" {
				counts.bumpScoped(key)
				t.Errorf("scoped listing on a workspace with no configured projects: %q", key)
				http.Error(w, "no", http.StatusNotFound)
				return
			}
			counts.bumpUnscoped()
			_, _ = w.Write([]byte(agileValues(
				agileBoard(11, "solo scrum board", "scrum", "NMB"),
				agileBoard(12, "solo kanban board", "kanban", "NMB"),
			)))
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/sprint"):
			id := agileSprintBoardID(p)
			counts.bumpSprint(id)
			if id == 11 {
				_, _ = w.Write([]byte(agileValues(`{"id":111,"name":"solo sprint","state":"active","originBoardId":11}`)))
				return
			}
			http.Error(w, `{"errorMessages":["The board doesn't support sprints."]}`, http.StatusBadRequest)
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/project"):
			counts.bumpBoardProject()
			http.Error(w, "no", http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, p)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0

	db := agileScopeMirror(t, srv.URL)
	importAgile(context.Background(), client, nil, db.DB, Options{})

	unscoped, boardProject, scoped, sprint := counts.snapshot()
	if unscoped != 1 {
		t.Errorf("site-wide /board listing ran %d times, want 1 — no configured projects keeps today's listing", unscoped)
	}
	if len(scoped) != 0 {
		t.Errorf("scoped listings = %v, want none", scoped)
	}
	if boardProject != 0 {
		t.Errorf("/board/{id}/project ran %d times, want 0 — the location mapped both boards", boardProject)
	}
	if sprint[11] != 1 || sprint[12] != 0 || len(sprint) != 1 {
		t.Errorf("sprint listings = %v, want board 11 once and board 12 (kanban) never", sprint)
	}
	var typ12 string
	if err := db.DB.QueryRow(`SELECT type FROM boards WHERE source_id = ? AND id = 12`, SourceID).Scan(&typ12); err != nil {
		t.Fatalf("kanban board row: %v — it must be stored, only its sprint read is skipped", err)
	}
	if typ12 != "kanban" {
		t.Errorf("board 12 type = %q, want kanban", typ12)
	}
}

// "" and "  " are not projects. A config whose every entry is blank has no
// scope, and no scope means the whole site: the unscoped listing with its
// backfill, never a scoped read per blank entry.
func TestAgileBlankProjectsKeepTheSiteWideListing(t *testing.T) {
	counts := newAgileCounts()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/rest/agile/1.0/board":
			if key := r.URL.Query().Get("projectKeyOrId"); key != "" {
				counts.bumpScoped(key)
				t.Errorf("a blank entry became a scoped listing: %q", key)
				http.Error(w, "no", http.StatusNotFound)
				return
			}
			counts.bumpUnscoped()
			_, _ = w.Write([]byte(agileValues(
				agileBoard(21, "solo board", "scrum", "BLK"),
			)))
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/sprint"):
			counts.bumpSprint(agileSprintBoardID(p))
			_, _ = w.Write([]byte(agileValues(`{"id":211,"name":"solo sprint","state":"active","originBoardId":21}`)))
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/project"):
			counts.bumpBoardProject()
			http.Error(w, "no", http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, p)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0

	cfg := &config.Config{
		Kind: config.OriginJiraServer, Site: srv.URL, Token: "pat-token",
		Projects: []string{"", "   "},
	}
	db := agileScopeMirror(t, srv.URL)
	importAgile(context.Background(), client, cfg, db.DB, Options{})

	unscoped, boardProject, scoped, sprint := counts.snapshot()
	if unscoped != 1 {
		t.Errorf("site-wide /board listing ran %d times, want 1 — all-blank projects is no scope", unscoped)
	}
	if len(scoped) != 0 {
		t.Errorf("scoped listings = %v, want none", scoped)
	}
	if boardProject != 0 {
		t.Errorf("/board/{id}/project ran %d times, want 0 — the location mapped the board", boardProject)
	}
	if sprint[21] != 1 || len(sprint) != 1 {
		t.Errorf("sprint listings = %v, want board 21 once", sprint)
	}
	var key string
	if err := db.DB.QueryRow(`SELECT project_key FROM boards WHERE source_id = ? AND id = 21`, SourceID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if key != "BLK" {
		t.Errorf("board 21 project_key = %q, want BLK from the location", key)
	}
}

// Every configured project answering "no Agile here" is the scoped form of a
// site without Jira Software, and the site-wide path keeps the previous rows
// on that answer (listBoards' ErrNoAgile case). The scoped path must agree:
// a pass where no listing found Agile at all replaces nothing. A project
// with no boards next to one that has them is still an ordinary empty
// answer — the healthy project's rows are what the pass stores.
// FAIL-first (2026-11-03, before the noAgile count in listBoards): boards
// rows = 0 after the all-404 pass, want 1.
func TestAgileScopedAllNoAgileKeepsPreviousRows(t *testing.T) {
	gone := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/rest/agile/1.0/board" && r.URL.Query().Get("projectKeyOrId") != "":
			if gone {
				http.Error(w, "no agile", http.StatusNotFound)
				return
			}
			if r.URL.Query().Get("projectKeyOrId") == "NMB" {
				_, _ = w.Write([]byte(agileValues(agileBoard(1, "NMB scrum board", "scrum", ""))))
				return
			}
			_, _ = w.Write([]byte(agileValues()))
		case strings.HasPrefix(p, "/rest/agile/1.0/board/") && strings.HasSuffix(p, "/sprint"):
			_, _ = w.Write([]byte(agileValues(`{"id":601,"name":"NMB sprint","state":"active","originBoardId":1}`)))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0
	cfg := &config.Config{
		Kind: config.OriginJiraServer, Site: srv.URL, Token: "pat-token",
		Projects: []string{"NMB", "NMA"},
	}
	db := agileScopeMirror(t, srv.URL)
	count := func() int {
		var n int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM boards WHERE source_id = ?`, SourceID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	importAgile(context.Background(), client, cfg, db.DB, Options{})
	if n := count(); n != 1 {
		t.Fatalf("after the healthy pass (NMB one board, NMA none): boards rows = %d, want 1", n)
	}
	gone = true
	importAgile(context.Background(), client, cfg, db.DB, Options{})
	if n := count(); n != 1 {
		t.Errorf("boards rows = %d after the all-404 pass, want 1 — every listing saying no Agile must keep the previous rows", n)
	}
}
