package server

// internal/server retro endpoint — contract ↔ assertion map.
//
//	C1 the document is the same compute the CLI prints → TestRetroEndpoint
//	   (bucket count matches retro.Buckets for the same since; counts equal
//	   key lengths over the wire)
//	C2 bad since is a 400 carrying the parse error            → TestRetroEndpoint
//	C3 store failure is a 500 (serverError path, shared)       → not re-tested here
//	C4 session_gap: same parser and sentence as the CLI flag,
//	   400 below the bound, effective gap in definitions      → TestRetroEndpoint
//	C5 the flow rows ride along: wip age max and cycle p50/p85
//	   in the bucket shape, cycle values exactly when the
//	   sample list is non-empty                               → TestRetroEndpoint
//
// FAIL-first: before the route existed this test got a 404 from
// handleNotFound; the handler file and the registration are what turn it
// green. The route literal also cannot collide with {key}/detail/ — literal
// beats wildcard in ServeMux, and TestRoutesRegister guards the panic.
//
// Spec correction, measured against the code: the task spec said
// GET /api/v1/retro/, but every Handler-mux API route lives under apiBase
// (/api/v1/issues/) and mirror_gate.go serveScopeAdmits admits exactly
// apiBase/authBase/dashBase for paired DNS hosts — a top-level path would be
// unreachable there. The endpoint is /api/v1/issues/retro/.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

func TestRetroEndpoint(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"retro/?since=4w", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("since=4w: %d %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Buckets []struct {
			Partial    bool     `json:"partial"`
			InProgress *int     `json:"in progress"`
			Closed     *int     `json:"closed"`
			WipP85     *float64 `json:"wip age p85"`
			WipMax     *float64 `json:"wip age max"`
			CycleP50   *float64 `json:"cycle p50"`
			CycleP85   *float64 `json:"cycle p85"`
			Mismatch   int      `json:"mismatch"`
			Keys       struct {
				Closed     []string `json:"closed"`
				InProgress []string `json:"in progress"`
				Mismatch   []string `json:"mismatch"`
				Cycle      []string `json:"cycle"`
			} `json:"keys"`
		} `json:"buckets"`
		Definitions map[string]string `json:"definitions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}

	// Same bucket lattice the CLI computes for the same window. The handler
	// takes its own time.Now(); the count only moves across a Monday
	// midnight, so the equality holds unless the test straddles one.
	if want := len(retro.Buckets(time.Now(), 4*7*24*time.Hour)); len(doc.Buckets) != want {
		t.Fatalf("buckets = %d, want %d", len(doc.Buckets), want)
	}
	if len(doc.Definitions) == 0 {
		t.Fatal("definitions object is empty")
	}

	// The fixture: one in-progress issue (NMB-1), nothing closed and no
	// done-word comments inside a 4w window — the partial week pins the
	// compute over the wire, every bucket keeps counts equal to key lengths.
	last := doc.Buckets[len(doc.Buckets)-1]
	if !last.Partial {
		t.Fatal("last bucket must be the partial current week")
	}
	if last.InProgress == nil || *last.InProgress != 1 {
		t.Fatalf("partial week in progress = %v, want 1", last.InProgress)
	}
	if len(last.Keys.InProgress) != 1 || last.Keys.InProgress[0] != "NMB-1" {
		t.Fatalf("partial week in-progress keys = %v, want [NMB-1]", last.Keys.InProgress)
	}
	// The fixture's one in-progress issue has a status change (2026-07-03),
	// so the ages list behind the partial week's wip rows is non-empty: p85
	// and max exist together or not at all — they come from the same list.
	// FAIL-first: before the wip age max row existed the field was absent
	// from this struct and from the document.
	if (last.WipP85 == nil) != (last.WipMax == nil) {
		t.Fatalf("partial week wip p85/max = %v/%v — both derive from one ages list", last.WipP85, last.WipMax)
	}
	if last.WipMax == nil {
		t.Fatal("partial week wip age max should exist: NMB-1 is in progress with a status change")
	}
	for bi, b := range doc.Buckets {
		if b.Closed != nil && len(b.Keys.Closed) != *b.Closed {
			t.Fatalf("bucket %d closed = %v, %d keys", bi, *b.Closed, len(b.Keys.Closed))
		}
		if b.InProgress != nil && len(b.Keys.InProgress) != *b.InProgress {
			t.Fatalf("bucket %d in progress = %v, %d keys", bi, *b.InProgress, len(b.Keys.InProgress))
		}
		if len(b.Keys.Mismatch) != b.Mismatch {
			t.Fatalf("bucket %d mismatch = %d, %d keys", bi, b.Mismatch, len(b.Keys.Mismatch))
		}
		// C5: the cycle sample list is the whole story — no count field rides
		// along, so p50 and p85 exist exactly when it is non-empty. The empty
		// array must still be [], not null.
		if b.Keys.Cycle == nil {
			t.Fatalf("bucket %d keys.cycle must marshal as [], not null", bi)
		}
		if (b.CycleP50 == nil) != (len(b.Keys.Cycle) == 0) || (b.CycleP85 == nil) != (len(b.Keys.Cycle) == 0) {
			t.Fatalf("bucket %d cycle p50/p85 = %v/%v with %d keys — values exist exactly when keys do",
				bi, b.CycleP50, b.CycleP85, len(b.Keys.Cycle))
		}
	}

	// Default since: same document without the parameter — and the default
	// session gap names itself in the footer (30m).
	rec = get(t, h, apiBase+"retro/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("default since: %d %s", rec.Code, rec.Body.String())
	}
	var defaults struct {
		Definitions map[string]string `json:"definitions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &defaults); err != nil {
		t.Fatalf("decode default document: %v", err)
	}
	if !strings.Contains(defaults.Definitions["sessions"], "exceeds 30m") {
		t.Fatalf("default definitions[sessions] should carry the 30m gap: %q", defaults.Definitions["sessions"])
	}

	// A bad since is a 400 that carries the parse error text.
	rec = get(t, h, apiBase+"retro/?since=abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("since=abc: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "--since wants") {
		t.Fatalf("400 body must carry the parse error text: %s", body)
	}

	// session_gap (C4): the same parser and the same sentence as the CLI
	// flag. Below the bound it is a 400 naming both walls; told 45m it runs
	// and the footer prints the effective gap. FAIL-first: before the
	// parameter existed the query string was ignored entirely.
	rec = get(t, h, apiBase+"retro/?session_gap=1m", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("session_gap=1m: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "5m") || !strings.Contains(body, "24h") {
		t.Fatalf("400 body must name both bounds: %s", body)
	}
	rec = get(t, h, apiBase+"retro/?session_gap=45m", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("session_gap=45m: %d %s", rec.Code, rec.Body.String())
	}
	var told struct {
		Definitions map[string]string `json:"definitions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &told); err != nil {
		t.Fatalf("decode 45m document: %v", err)
	}
	if !strings.Contains(told.Definitions["sessions"], "exceeds 45m") {
		t.Fatalf("session_gap=45m definitions[sessions]: %q", told.Definitions["sessions"])
	}
}

// The session gap's config default (retro.sessionGap) — contract ↔
// assertion (FAIL-first: before the handler read the config, an absent
// session_gap parameter always computed with 30m, so the 45m row failed):
// absent parameter + retro.sessionGap=45m → footer 45m; an explicit
// session_gap parameter still beats the config; a bad stored value is a
// 400 naming the config key.
func TestRetroEndpointSessionGapConfigDefault(t *testing.T) {
	db, cfg := fixture(t)
	cfg.Retro = &config.RetroConfig{SessionGap: "45m"}
	h := New(db, cfg)

	rec := get(t, h, apiBase+"retro/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("config gap: %d %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Definitions map[string]string `json:"definitions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(doc.Definitions["sessions"], "exceeds 45m") {
		t.Fatalf("config sessionGap=45m must reach the footer: %q", doc.Definitions["sessions"])
	}

	// The parameter still beats the config.
	rec = get(t, h, apiBase+"retro/?session_gap=15m", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("session_gap=15m: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(doc.Definitions["sessions"], "exceeds 15m") {
		t.Fatalf("parameter must beat the config: %q", doc.Definitions["sessions"])
	}

	// A bad stored value is a 400 that names the config key.
	cfg.Retro = &config.RetroConfig{SessionGap: "banana"}
	h = New(db, cfg)
	rec = get(t, h, apiBase+"retro/", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad stored gap: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "retro.sessionGap") {
		t.Fatalf("400 must name the config key: %s", body)
	}
}

// TestRetroAmbiguousBoardCarriesTheBoards — the 409 that refuses a sprint cut
// on a mirror with several sprint-bearing boards has to hand the picker its
// rows (GDK-1713). FAIL-first: the endpoint used to answer
// `{"error": "<the CLI sentence>"}` — prose that names `--board`, a flag no
// web reader has — so the surface had nothing to build a choice out of.
func TestRetroAmbiguousBoardCarriesTheBoards(t *testing.T) {
	db, cfg := fixture(t)
	if err := db.ReplaceAgile(context.Background(), "jira",
		[]store.BoardRow{{ID: 1, Name: "Team board", Type: "scrum"}, {ID: 2, Name: "Platform board", Type: "scrum"}},
		[]store.SprintRow{
			{ID: 41, BoardID: 1, Name: "Sprint 41", State: "closed", StartAt: "2026-08-12T00:00:00Z", EndAt: "2026-08-26T00:00:00Z"},
			{ID: 51, BoardID: 2, Name: "Platform 7", State: "closed", StartAt: "2026-08-19T00:00:00Z", EndAt: "2026-09-02T00:00:00Z"},
		}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	rec := get(t, h, apiBase+"retro/?by=sprint", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("by=sprint with two boards: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Boards  []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"boards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "ambiguous_board" {
		t.Errorf("error = %q, want ambiguous_board — the code is what the surface branches on", body.Error)
	}
	if len(body.Boards) != 2 {
		t.Fatalf("boards = %d, want both", len(body.Boards))
	}
	if body.Boards[0].ID != 1 || body.Boards[0].Name != "Team board" {
		t.Errorf("first board = %+v, want id 1 Team board", body.Boards[0])
	}
	if !strings.Contains(body.Message, "several boards") {
		t.Errorf("message = %q, want the report's own sentence kept", body.Message)
	}

	// Naming one resolves it — the same id the body just handed over.
	rec = get(t, h, apiBase+"retro/?by=sprint&board=2", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("board=2: %d %s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Buckets []struct {
			Name string `json:"name"`
		} `json:"buckets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Buckets) != 1 || doc.Buckets[0].Name != "Platform 7" {
		t.Errorf("board=2 buckets = %+v, want just Platform 7", doc.Buckets)
	}
}

// TestBoardsEndpoint — the picker's source list (GDK-1713). The flag is the
// report's own predicate, so a board with no dated sprint is offered as a
// board but never as a sprint cut.
func TestBoardsEndpoint(t *testing.T) {
	db, cfg := fixture(t)
	if err := db.ReplaceAgile(context.Background(), "jira",
		[]store.BoardRow{{ID: 1, Name: "Team board", Type: "scrum"}, {ID: 9, Name: "Support", Type: "kanban"}},
		[]store.SprintRow{
			{ID: 41, BoardID: 1, Name: "Sprint 41", State: "closed", StartAt: "2026-08-12T00:00:00Z", EndAt: "2026-08-26T00:00:00Z"},
		}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)

	rec := get(t, h, apiBase+"boards/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("boards/: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Boards []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Type       string `json:"type"`
			HasSprints bool   `json:"has_sprints"`
		} `json:"boards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Boards) != 2 {
		t.Fatalf("boards = %d, want 2", len(body.Boards))
	}
	if !body.Boards[0].HasSprints || body.Boards[1].HasSprints {
		t.Errorf("has_sprints = %v/%v, want true/false", body.Boards[0].HasSprints, body.Boards[1].HasSprints)
	}
	if body.Boards[1].Type != "kanban" {
		t.Errorf("board 9 type = %q, want kanban", body.Boards[1].Type)
	}
}

// TestRetroEndpointCarriesTheMaterials — the web surface reads this document
// and nothing else, so every field of the materials contract has to survive
// the encoder: present, and an array where the renderer iterates rather than
// a null it would have to guard. FAIL-first: before internal/retro/materials.go
// none of these keys existed and the decode below found them absent.
func TestRetroEndpointCarriesTheMaterials(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"retro/?since=4w", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, top := range []string{"aging", "actions"} {
		v, ok := raw[top]
		if !ok {
			t.Fatalf("the document has no %q", top)
		}
		if string(v) == "null" {
			t.Errorf("%q is null", top)
		}
	}
	var aging struct {
		P85   *float64 `json:"p85_days"`
		Items []struct {
			Key         string  `json:"key"`
			Days        float64 `json:"days"`
			Summary     string  `json:"summary"`
			IssueTypeID string  `json:"issue_type_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw["aging"], &aging); err != nil {
		t.Fatalf("aging: %v", err)
	}
	if aging.Items == nil {
		t.Error("aging.items must be an array")
	}
	for i := 1; i < len(aging.Items); i++ {
		if aging.Items[i-1].Days < aging.Items[i].Days {
			t.Fatalf("aging is not oldest-first at %d", i)
		}
	}
	if len(aging.Items) > 0 && aging.P85 == nil {
		t.Error("p85_days must exist when the tail is non-empty")
	}
	var actions []struct {
		Key    string `json:"key"`
		Metric string `json:"metric"`
	}
	if err := json.Unmarshal(raw["actions"], &actions); err != nil {
		t.Fatalf("actions: %v", err)
	}

	var buckets []map[string]json.RawMessage
	if err := json.Unmarshal(raw["buckets"], &buckets); err != nil {
		t.Fatalf("buckets: %v", err)
	}
	if len(buckets) == 0 {
		t.Fatal("no buckets")
	}
	for bi, b := range buckets {
		for _, field := range []string{"events", "surprises", "closed_by_type",
			"closed_by_epic", "cycle_points"} {
			v, ok := b[field]
			if !ok {
				t.Errorf("bucket %d has no %q", bi, field)
				continue
			}
			if string(v) == "null" {
				t.Errorf("bucket %d %q is null, want []", bi, field)
			}
		}
		for _, field := range []string{"unplanned", "seen_not_moved", "moved_not_seen"} {
			v, ok := b[field]
			if !ok {
				t.Errorf("bucket %d has no %q", bi, field)
				continue
			}
			var obj struct {
				Keys []string `json:"keys"`
			}
			if err := json.Unmarshal(v, &obj); err != nil {
				t.Errorf("bucket %d %q: %v", bi, field, err)
				continue
			}
			if obj.Keys == nil {
				t.Errorf("bucket %d %q.keys is null, want []", bi, field)
			}
		}
		// closed_by_type partitions the closed row above it.
		var closed *int
		var byType []struct {
			Count int `json:"count"`
		}
		_ = json.Unmarshal(b["closed"], &closed)
		if err := json.Unmarshal(b["closed_by_type"], &byType); err != nil {
			t.Errorf("bucket %d closed_by_type: %v", bi, err)
			continue
		}
		if closed != nil {
			sum := 0
			for _, tc := range byType {
				sum += tc.Count
			}
			if sum != *closed {
				t.Errorf("bucket %d: closed_by_type sums to %d, closed is %d", bi, sum, *closed)
			}
		}
	}
	// The definitions name the new rows too, so a reader of the document has
	// the rule beside every list.
	var defs struct {
		Definitions map[string]string `json:"definitions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &defs); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"aging", "events", "surprises", "closed_by_type",
		"closed_by_epic", "unplanned", "cycle_points", "seen_not_moved",
		"moved_not_seen", "actions"} {
		if defs.Definitions[name] == "" {
			t.Errorf("no definition for %q in the document", name)
		}
	}
}
