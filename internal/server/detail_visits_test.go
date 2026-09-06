package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

/*
 * The resume card's server half (spec Part A). Clause table — the clauses
 * this file owns; the client half of C1 (field parsing) and C2–C7 live in
 * web/src/lib/resume-card.test.ts and e2e/resume-card.spec.ts.
 *
 *   C1 ui-only timestamps — TestDetailVisitsCarryPersonReadTimestamps
 *        ① two ui rows → last/previous match those rows' viewed_at, newest first
 *        ② a cli row recorded between them is not picked as previous
 *   C1 absent when none — TestDetailVisitsAbsentWithoutVisits
 *        ① zero visits → both keys absent from the JSON
 *        ② a visit on another issue does not leak across keys
 *        ③ exactly one visit → previous_visit_at absent (nothing before it)
 *   C1 local.db read error never fails the detail —
 *      TestDetailSurvivesLocalReadError
 *        ① detail still answers 200 with the visits table gone
 *        ② both keys are absent rather than zero-valued
 *
 * FAIL-first: before the Part A edit this file does not compile —
 * detailResponse has no LastVisitedAt/PreviousVisitAt (run
 * `go test ./internal/server/ -run TestDetailVisits -count=1` and read the
 * build output).
 */

func TestDetailVisitsCarryPersonReadTimestamps(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)
	ctx := context.Background()

	// Distinct milliseconds, so newest-first is asserted on real ordering
	// (same-ms rows would make the two timestamps indistinguishable).
	uiOld, err := db.RecordVisit(ctx, store.VisitKindIssue, "NMB-1", store.VisitSourceUI)
	if err != nil {
		t.Fatalf("record ui: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	// An agent read between the two person reads: same issue, same key —
	// only the source separates it. It must never become a boundary.
	cliMid, err := db.RecordVisit(ctx, store.VisitKindIssue, "NMB-1", store.VisitSourceCLI)
	if err != nil {
		t.Fatalf("record cli: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	uiNew, err := db.RecordVisit(ctx, store.VisitKindIssue, "NMB-1", store.VisitSourceUI)
	if err != nil {
		t.Fatalf("record ui: %v", err)
	}

	d := decode[detailResponse](t, get(t, h, apiBase+"NMB-1/detail/", nil))
	if d.LastVisitedAt == nil || *d.LastVisitedAt != uiNew.ViewedAt {
		t.Fatalf("last_visited_at = %v, want newest ui visit %q", d.LastVisitedAt, uiNew.ViewedAt)
	}
	if d.PreviousVisitAt == nil || *d.PreviousVisitAt != uiOld.ViewedAt {
		t.Fatalf("previous_visit_at = %v, want older ui visit %q", d.PreviousVisitAt, uiOld.ViewedAt)
	}
	if d.PreviousVisitAt != nil && *d.PreviousVisitAt == cliMid.ViewedAt {
		t.Fatalf("previous_visit_at = cli visit %q; agent reads must not be a resume boundary", cliMid.ViewedAt)
	}
}

func TestDetailVisitsAbsentWithoutVisits(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	if _, err := db.RecordVisit(context.Background(), store.VisitKindIssue, "NMB-2", store.VisitSourceUI); err != nil {
		t.Fatalf("record: %v", err)
	}

	rec := get(t, h, apiBase+"NMB-1/detail/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["last_visited_at"]; ok {
		t.Fatalf("last_visited_at present for an issue with no visits: %s", raw["last_visited_at"])
	}
	if _, ok := raw["previous_visit_at"]; ok {
		t.Fatalf("previous_visit_at present for an issue with no visits: %s", raw["previous_visit_at"])
	}

	// One visit: there is a newest read but nothing before it, so the
	// previous key stays absent — one visit is not a resume boundary.
	if _, err := db.RecordVisit(context.Background(), store.VisitKindIssue, "NMB-1", store.VisitSourceUI); err != nil {
		t.Fatalf("record: %v", err)
	}
	raw = decode[map[string]json.RawMessage](t, get(t, h, apiBase+"NMB-1/detail/", nil))
	if _, ok := raw["last_visited_at"]; !ok {
		t.Fatal("last_visited_at absent after one ui visit")
	}
	if _, ok := raw["previous_visit_at"]; ok {
		t.Fatalf("previous_visit_at present after one visit: %s", raw["previous_visit_at"])
	}
}

func TestDetailSurvivesLocalReadError(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)

	if _, err := db.RecordVisit(context.Background(), store.VisitKindIssue, "NMB-1", store.VisitSourceUI); err != nil {
		t.Fatalf("record: %v", err)
	}
	before := decode[detailResponse](t, get(t, h, apiBase+"NMB-1/detail/", nil))
	if before.LastVisitedAt == nil {
		t.Fatal("sanity: last_visited_at absent while visits are readable")
	}

	// Break the person-read table in a way no API can (a third connection,
	// no ATTACH — the hook skips local.db as the main file). The next detail
	// GET must still answer: local.db is a personal-history cache, so a read
	// error leaves both keys absent rather than failing the whole detail.
	local, err := sql.Open("sqlite", "file:"+filepath.Join(filepath.Dir(path), "local.db"))
	if err != nil {
		t.Fatalf("open local.db: %v", err)
	}
	defer local.Close()
	if _, err := local.Exec(`DROP TABLE visits`); err != nil {
		t.Fatalf("drop visits: %v", err)
	}

	rec := get(t, h, apiBase+"NMB-1/detail/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d with visits unreadable; local.db must not fail the detail", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["last_visited_at"]; ok {
		t.Fatalf("last_visited_at present with visits unreadable: %s", raw["last_visited_at"])
	}
	if _, ok := raw["previous_visit_at"]; ok {
		t.Fatalf("previous_visit_at present with visits unreadable: %s", raw["previous_visit_at"])
	}
}
