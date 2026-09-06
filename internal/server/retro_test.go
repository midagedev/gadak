package server

// internal/server retro endpoint — contract ↔ assertion map.
//
//	C1 the document is the same compute the CLI prints → TestRetroEndpoint
//	   (bucket count matches retro.Buckets for the same since; counts equal
//	   key lengths over the wire)
//	C2 bad since is a 400 carrying the parse error            → TestRetroEndpoint
//	C3 store failure is a 500 (serverError path, shared)       → not re-tested here
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
			Partial    bool `json:"partial"`
			InProgress *int `json:"in progress"`
			Closed     *int `json:"closed"`
			Mismatch   int  `json:"mismatch"`
			Keys       struct {
				Closed     []string `json:"closed"`
				InProgress []string `json:"in progress"`
				Mismatch   []string `json:"mismatch"`
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
	}

	// Default since: same document without the parameter.
	rec = get(t, h, apiBase+"retro/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("default since: %d %s", rec.Code, rec.Body.String())
	}

	// A bad since is a 400 that carries the parse error text.
	rec = get(t, h, apiBase+"retro/?since=abc", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("since=abc: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "--since wants") {
		t.Fatalf("400 body must carry the parse error text: %s", body)
	}
}
