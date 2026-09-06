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
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/retro"
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
