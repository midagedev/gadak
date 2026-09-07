package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

/*
 * The session strip's server half (spec r2-session, Part A). Clause table —
 * the clauses this file owns; the client half of C1 (field parsing) and C2–C8
 * live in web/src/lib/session-strip.test.ts and e2e/session-strip.spec.ts.
 *
 *   C1 last_session_ended_at is the previous session's end, gap 30m —
 *      TestBootstrapCarriesLastSessionEnd
 *        ① two old sessions + a current read → the now-2h stamp on the wire
 *        ② an agent (cli) read between them does not move it
 *   C1 absent when there is no previous session —
 *      TestBootstrapLastSessionEndAbsentWithoutVisits
 *        ① zero visits → the key is absent from the JSON, not zero-valued
 *   C1 a local.db read error never fails bootstrap —
 *      TestBootstrapSurvivesLocalReadError
 *        ① bootstrap still answers 200 with the visits table gone
 *        ② the key is absent rather than zero-valued
 *   C1 bootstrap only — TestDeltaOmitsLastSessionEnd
 *        ① the delta response never carries the key (the boundary is the
 *          tab's birth, so a delta-only tab must not learn one)
 *
 * FAIL-first: before the Part A edit this file does not compile —
 * bootstrapResponse has no LastSessionEndedAt (run
 * `go test ./internal/server/ -run TestBootstrap -count=1` and read the build
 * output; failfirst evidence server-session-prechange.out, in this round's
 * session scratchpad).
 *
 * Visit stamps are written straight into local.db (a third connection, no
 * ATTACH — the same road detail_visits_test.go takes) because RecordVisit
 * stamps Now() and cannot express "two hours ago".
 */

// seedVisit rows one read with an explicit stamp into the fixture's local.db.
func seedVisit(t *testing.T, dbPath string, at time.Time, source string) {
	t.Helper()
	local, err := sql.Open("sqlite", "file:"+filepath.Join(filepath.Dir(dbPath), "local.db"))
	if err != nil {
		t.Fatalf("open local.db: %v", err)
	}
	defer local.Close()
	// Unqualified `visits`: this connection has no ATTACH (the hook skips
	// local.db as the main file), so the table is main.visits here.
	if _, err := local.Exec(
		`INSERT INTO visits (kind, key, viewed_at, source) VALUES (?,?,?,?)`,
		store.VisitKindIssue, "NMB-1", at.UTC().Format(config.ISOMilli), source); err != nil {
		t.Fatalf("seed visit at %s: %v", at, err)
	}
}

func TestBootstrapCarriesLastSessionEnd(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)
	now := time.Now().UTC()
	// Session 1 (3d back), session 2 (2h back), current read (1m). The
	// boundary is session 2's read; a cli read at 1h must not move it.
	seedVisit(t, path, now.Add(-72*time.Hour), store.VisitSourceUI)
	seedVisit(t, path, now.Add(-72*time.Hour+5*time.Minute), "")
	prev := now.Add(-2 * time.Hour)
	seedVisit(t, path, prev, store.VisitSourceUI)
	seedVisit(t, path, now.Add(-time.Hour), store.VisitSourceCLI)
	seedVisit(t, path, now.Add(-time.Minute), store.VisitSourceUI)

	body := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if body.LastSessionEndedAt == "" {
		t.Fatal("last_session_ended_at absent with a previous session on record")
	}
	want := prev.Format(config.ISOMilli)
	if body.LastSessionEndedAt != want {
		t.Fatalf("last_session_ended_at = %q, want the previous session's end %q", body.LastSessionEndedAt, want)
	}
	// ISOMilli on the wire — the client's Date.parse and the strip's relative
	// time both assume an ISO instant.
	if _, err := time.Parse(config.ISOMilli, body.LastSessionEndedAt); err != nil {
		t.Fatalf("last_session_ended_at %q is not ISOMilli: %v", body.LastSessionEndedAt, err)
	}
}

func TestBootstrapLastSessionEndAbsentWithoutVisits(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, ok := raw["last_session_ended_at"]; ok {
		t.Fatalf("last_session_ended_at present with no visits: %s", v)
	}
}

func TestBootstrapSurvivesLocalReadError(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)
	seedVisit(t, path, time.Now().UTC().Add(-2*time.Hour), store.VisitSourceUI)
	before := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if before.LastSessionEndedAt == "" {
		t.Fatal("sanity: last_session_ended_at absent while visits are readable")
	}

	// Break the person-read table in a way no API can. The next bootstrap
	// must still answer: the strip is an enrichment and never fails the
	// response it rides on (the flowFields rule).
	local, err := sql.Open("sqlite", "file:"+filepath.Join(filepath.Dir(path), "local.db"))
	if err != nil {
		t.Fatalf("open local.db: %v", err)
	}
	defer local.Close()
	if _, err := local.Exec(`DROP TABLE visits`); err != nil {
		t.Fatalf("drop visits: %v", err)
	}

	rec := get(t, h, apiBase+"bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d with visits unreadable; local.db must not fail bootstrap", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, ok := raw["last_session_ended_at"]; ok {
		t.Fatalf("last_session_ended_at present with visits unreadable: %s", v)
	}
}

func TestDeltaOmitsLastSessionEnd(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)
	seedVisit(t, path, time.Now().UTC().Add(-2*time.Hour), store.VisitSourceUI)

	boot := decode[bootstrapResponse](t, get(t, h, apiBase+"bootstrap/", nil))
	if boot.LastSessionEndedAt == "" {
		t.Fatal("sanity: bootstrap carries the boundary")
	}
	rec := get(t, h, apiBase+"delta/?since="+boot.ServerTime, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delta status = %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, ok := raw["last_session_ended_at"]; ok {
		t.Fatalf("delta carries last_session_ended_at (%s); the boundary rides bootstrap only", v)
	}
}

/*
 * GDK-1537 — the boundary must survive a 304.
 *
 * The body-only contract above has a hole the clauses above cannot see: a
 * client that already holds the issue set sends If-None-Match, the server
 * answers 304 with no body at all, and the boundary — which has nothing to do
 * with the issue set, being computed from local.visits — goes missing exactly
 * on the cold start against an unchanged mirror. The strip is then absent on
 * the one morning nothing happened overnight.
 *
 * The closure is a response header, legal on 304 (RFC 9110 §15.4.5) and set
 * before the conditional branch. It is not folded into the ETag on purpose:
 * the ETag is about the issue set, and a boundary in it would force a full
 * re-hydration at the start of every session.
 *
 *   C9 the 200 carries X-Gadak-Session-Boundary, equal to the body field —
 *      TestBootstrapBoundaryHeaderMatchesBody
 *   C9 the 304 carries the same header —
 *      TestBootstrapBoundaryHeaderSurvives304
 *   C9 absent means absent on both codes (no empty header) —
 *      TestBootstrapBoundaryHeaderAbsentWithoutVisits
 *   C9 the delta carries the header too, because a warm web tab syncs by
 *      delta and never asks bootstrap again — TestDeltaCarriesBoundaryHeader
 *      (the body key stays absent there: TestDeltaOmitsLastSessionEnd)
 *   C9 a HEAD answers it, so `curl -sI …/issues/bootstrap/` is the operator's
 *      one-line probe — TestBootstrapBoundaryHeaderOnHEAD
 *
 * FAIL-first evidence: on the pre-change source all four fail with an empty
 * header (server-1537-prechange.out in this round's scratchpad).
 */

func TestBootstrapBoundaryHeaderMatchesBody(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)
	prev := time.Now().UTC().Add(-2 * time.Hour)
	seedVisit(t, path, prev, store.VisitSourceUI)

	rec := get(t, h, apiBase+"bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d", rec.Code)
	}
	body := decode[bootstrapResponse](t, rec)
	got := rec.Header().Get(sessionBoundaryHeader)
	if got == "" {
		t.Fatalf("%s absent on 200 with a previous session on record", sessionBoundaryHeader)
	}
	if got != body.LastSessionEndedAt {
		t.Fatalf("%s = %q, body last_session_ended_at = %q; one value, two seats",
			sessionBoundaryHeader, got, body.LastSessionEndedAt)
	}
	if _, err := time.Parse(config.ISOMilli, got); err != nil {
		t.Fatalf("%s %q is not ISOMilli: %v", sessionBoundaryHeader, got, err)
	}
}

func TestBootstrapBoundaryHeaderSurvives304(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)
	seedVisit(t, path, time.Now().UTC().Add(-2*time.Hour), store.VisitSourceUI)

	first := get(t, h, apiBase+"bootstrap/", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d", first.Code)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("bootstrap sent no ETag; the 304 path cannot be exercised")
	}
	want := first.Header().Get(sessionBoundaryHeader)
	if want == "" {
		t.Fatalf("sanity: %s absent on the 200", sessionBoundaryHeader)
	}

	second := get(t, h, apiBase+"bootstrap/", map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional bootstrap status = %d, want 304", second.Code)
	}
	if n := second.Body.Len(); n != 0 {
		t.Fatalf("304 carried a %d-byte body; the header is the only seat left", n)
	}
	if got := second.Header().Get(sessionBoundaryHeader); got != want {
		t.Fatalf("%s on 304 = %q, want the 200's %q — a cold start against an "+
			"unchanged mirror must still learn the boundary",
			sessionBoundaryHeader, got, want)
	}
}

func TestBootstrapBoundaryHeaderAbsentWithoutVisits(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	first := get(t, h, apiBase+"bootstrap/", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d", first.Code)
	}
	if _, ok := first.Header()[http.CanonicalHeaderKey(sessionBoundaryHeader)]; ok {
		t.Fatalf("%s present with no visits: %q", sessionBoundaryHeader, first.Header().Get(sessionBoundaryHeader))
	}
	etag := first.Header().Get("ETag")
	second := get(t, h, apiBase+"bootstrap/", map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional bootstrap status = %d, want 304", second.Code)
	}
	if _, ok := second.Header()[http.CanonicalHeaderKey(sessionBoundaryHeader)]; ok {
		t.Fatalf("%s present on 304 with no visits: %q", sessionBoundaryHeader, second.Header().Get(sessionBoundaryHeader))
	}
}

// The operator's probe. `curl -sI <base>/api/v1/issues/bootstrap/` is now the
// one-line answer to "what boundary would this serve send?" — before this
// round the value was only reachable by fetching and parsing a whole
// bootstrap body, and on a 304 it was not reachable at all. ServeMux's `GET`
// pattern matches HEAD, so the probe costs nothing; this pins that, because a
// documented probe nobody tests is a hypothesis.
func TestBootstrapBoundaryHeaderOnHEAD(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)
	seedVisit(t, path, time.Now().UTC().Add(-2*time.Hour), store.VisitSourceUI)

	want := get(t, h, apiBase+"bootstrap/", nil).Header().Get(sessionBoundaryHeader)
	if want == "" {
		t.Fatalf("sanity: %s absent on the GET", sessionBoundaryHeader)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, testRequest(http.MethodHead, apiBase+"bootstrap/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD bootstrap status = %d; `curl -sI` is the documented probe", rec.Code)
	}
	if got := rec.Header().Get(sessionBoundaryHeader); got != want {
		t.Fatalf("%s on HEAD = %q, want %q", sessionBoundaryHeader, got, want)
	}
}

func TestDeltaCarriesBoundaryHeader(t *testing.T) {
	db, cfg, path := fixtureAt(t)
	h := New(db, cfg)
	seedVisit(t, path, time.Now().UTC().Add(-2*time.Hour), store.VisitSourceUI)

	boot := get(t, h, apiBase+"bootstrap/", nil)
	want := boot.Header().Get(sessionBoundaryHeader)
	if want == "" {
		t.Fatalf("sanity: %s absent on bootstrap", sessionBoundaryHeader)
	}
	body := decode[bootstrapResponse](t, boot)

	rec := get(t, h, apiBase+"delta/?since="+body.ServerTime, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delta status = %d", rec.Code)
	}
	if got := rec.Header().Get(sessionBoundaryHeader); got != want {
		t.Fatalf("%s on delta = %q, want %q — a warm tab syncs by delta and "+
			"never asks bootstrap again, so the boundary must ride it",
			sessionBoundaryHeader, got, want)
	}
}
