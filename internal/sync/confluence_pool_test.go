package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/confluence"
)

// The fetch-pool tests (GDK-1673). confFixture (confluence_test.go) is
// another round's file to stay shareable with; these tests need their own
// stand-in anyway, because the pool makes three things observable that a
// plain fixture cannot: a max in-flight gauge with a deterministic rendezvous
// barrier, and one-shot 429 / persistent 500 injection on a chosen page body.

// poolComment / poolPage are the fixture corpus. The slice order of pages is
// the CQL listing order (order by lastmodified asc), which is the order the
// single writer must commit in.
type poolComment struct {
	id, body, when string
}

type poolPage struct {
	id, space, title, body string
	version                int
	when                   string
	comments               []poolComment
}

// poolCorpus builds n single-comment pages in one space whose stamps ascend
// with the slice index, so listing order == index order == commit order.
func poolCorpus(space string, n int) []*poolPage {
	out := make([]*poolPage, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%d", 6100+i)
		out[i] = &poolPage{
			id: id, space: space, title: "Page " + id,
			version: 1,
			when:    fmt.Sprintf("2026-09-05T10:%02d:%02d.000Z", i/60, i%60),
			body:    confADF("body of " + id),
			comments: []poolComment{{
				id:   fmt.Sprintf("7%d", 1000+i),
				body: confADF("comment on " + id),
				when: fmt.Sprintf("2026-09-05T11:%02d:%02d.000Z", i/60, i%60),
			}},
		}
	}
	return out
}

// poolFixture serves the same wire shapes confFixture does (space listing,
// per-space GET, CQL search, page body, child comments, version listing) and
// instruments concurrency: every request passes through enter/exit so
// maxInFlight tracks the true overlap the origin saw, body GETs can hold at a
// rendezvous barrier until `width` of them overlap, and one page id can be
// told to answer 429 (once, with Retry-After) or 500 (always).
type poolFixture struct {
	mu     sync.Mutex
	srv    *httptest.Server
	pages  []*poolPage
	spaces map[string]string

	inFlight    int
	maxInFlight int

	// barrierLeft counts down on the first body GETs; each holds until the
	// count reaches zero (or the timeout). With barrierLeft == pool width,
	// max in-flight is forced to the width itself, deterministically — no
	// race between worker startup and fast fixtures. Zero (the default)
	// disables it: the countdown goes negative and the wait is skipped.
	barrierLeft int

	throttleID string // next body GET on this id: 429 + Retry-After: 1 (once)
	failID     string // every body GET on this id: 500

	bodyGETs []string
}

func newPoolFixture(t *testing.T, pages ...*poolPage) *poolFixture {
	t.Helper()
	f := &poolFixture{pages: pages, spaces: map[string]string{"AAA": "Alpha Space"}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.srv = srv
	return f
}

func (f *poolFixture) client() *confluence.Client {
	c := confluence.New(f.srv.URL, "user@example.invalid", "secret-token")
	c.Retries, c.Backoff, c.PauseBetween = 4, time.Millisecond, 0
	return c
}

func (f *poolFixture) setBarrier(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.barrierLeft = n
}

func (f *poolFixture) maxInFlightSeen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func (f *poolFixture) bodiesFetched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.bodyGETs...)
}

func (f *poolFixture) page(id string) *poolPage {
	for _, p := range f.pages {
		if p.id == id {
			return p
		}
	}
	return nil
}

// comment finds a top-level comment by id, for the reply-level GET.
func (f *poolFixture) comment(id string) *poolComment {
	for _, p := range f.pages {
		for i := range p.comments {
			if p.comments[i].id == id {
				return &p.comments[i]
			}
		}
	}
	return nil
}

func (f *poolFixture) enter() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
}

func (f *poolFixture) exit() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight--
}

// rendezvous holds this body GET until barrierLeft reaches zero or the
// timeout passes (750ms: instant when the pool fills the barrier, and cheap
// enough in the serial FAIL-first case where it can never fill).
func (f *poolFixture) rendezvous() {
	f.mu.Lock()
	f.barrierLeft--
	f.mu.Unlock()
	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		done := f.barrierLeft <= 0
		f.mu.Unlock()
		if done {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (f *poolFixture) serve(w http.ResponseWriter, r *http.Request) {
	// The client's base is site+"/wiki" (Cloud layout); serve both shapes.
	p := strings.TrimPrefix(r.URL.Path, "/wiki")
	switch {
	case strings.HasPrefix(p, "/rest/api/content/search"):
		f.serveSearch(w)
	case strings.HasSuffix(p, "/child/comment"):
		f.serveComments(w, strings.TrimSuffix(strings.TrimPrefix(p, "/rest/api/content/"), "/child/comment"))
	case strings.HasSuffix(p, "/child/attachment"):
		// No fixture page carries attachments yet; the empty listing is the
		// honest answer (GDK-1541 listing rides every body fetch).
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "size": 0, "limit": 100})
	case strings.HasSuffix(p, "/version"):
		f.serveVersions(w, strings.TrimSuffix(strings.TrimPrefix(p, "/rest/api/content/"), "/version"))
	case strings.HasPrefix(p, "/rest/api/space/"):
		f.serveSpace(w, strings.TrimPrefix(p, "/rest/api/space/"))
	case p == "/rest/api/space":
		f.serveSpaces(w)
	case strings.HasPrefix(p, "/rest/api/content/"):
		f.serveBody(w, strings.TrimPrefix(p, "/rest/api/content/"))
	default:
		http.NotFound(w, r)
	}
}

// serveSearch answers the CQL listing. Only type=page queries are produced by
// a full pass (the comments-only CQL is an incremental-chunk concern), so the
// cql parameter is not parsed — the whole corpus comes back in slice order.
func (f *poolFixture) serveSearch(w http.ResponseWriter) {
	results := make([]map[string]any, len(f.pages))
	for i, pg := range f.pages {
		results[i] = f.hitJSON(pg)
	}
	writeJSON(w, map[string]any{"results": results, "size": len(results), "limit": 50, "start": 0})
}

func (f *poolFixture) serveSpaces(w http.ResponseWriter) {
	results := []map[string]any{}
	for key, name := range f.spaces {
		results = append(results, map[string]any{"key": key, "name": name, "type": "global"})
	}
	writeJSON(w, map[string]any{"results": results, "size": len(results), "limit": 100, "start": 0})
}

func (f *poolFixture) serveSpace(w http.ResponseWriter, key string) {
	name, ok := f.spaces[key]
	if !ok {
		http.Error(w, "no such space", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"key": key, "name": name, "type": "global"})
}

func (f *poolFixture) hitJSON(pg *poolPage) map[string]any {
	return map[string]any{
		"id": pg.id, "type": "page", "title": pg.title,
		"space":   map[string]any{"key": pg.space, "name": f.spaces[pg.space]},
		"version": map[string]any{"number": pg.version, "when": pg.when},
	}
}

func (f *poolFixture) fullJSON(pg *poolPage) map[string]any {
	m := f.hitJSON(pg)
	m["status"] = "current"
	m["body"] = map[string]any{
		"atlas_doc_format": map[string]any{"value": pg.body, "representation": "atlas_doc_format"},
	}
	m["ancestors"] = []any{}
	m["metadata"] = map[string]any{"labels": map[string]any{"results": []any{}, "size": 0, "limit": 25, "start": 0}}
	m["version"].(map[string]any)["by"] = map[string]any{"accountId": "acc-1", "displayName": "Ada Example"}
	return m
}

func (f *poolFixture) serveBody(w http.ResponseWriter, id string) {
	f.enter()
	defer f.exit()

	f.mu.Lock()
	f.bodyGETs = append(f.bodyGETs, id)
	throttle, fail := f.throttleID == id, f.failID == id
	if throttle {
		f.throttleID = ""
	}
	f.mu.Unlock()

	if fail {
		http.Error(w, "injected server error", http.StatusInternalServerError)
		return
	}
	if throttle {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
		return
	}
	f.rendezvous()
	pg := f.page(id)
	if pg == nil {
		http.Error(w, "no such page", http.StatusNotFound)
		return
	}
	writeJSON(w, f.fullJSON(pg))
}

func (f *poolFixture) serveComments(w http.ResponseWriter, pageID string) {
	f.enter()
	defer f.exit()
	pg := f.page(pageID)
	if pg == nil && f.comment(pageID) == nil {
		http.Error(w, "no such page", http.StatusNotFound)
		return
	}
	var results []map[string]any
	if pg != nil {
		results = make([]map[string]any, len(pg.comments))
		for i, cm := range pg.comments {
			results[i] = map[string]any{
				"id": cm.id,
				"body": map[string]any{
					"atlas_doc_format": map[string]any{"value": cm.body, "representation": "atlas_doc_format"},
				},
				"version": map[string]any{
					"number": 1, "when": cm.when,
					"by": map[string]any{"accountId": "acc-2", "displayName": "Bo Example"},
				},
			}
		}
	}
	// A GET on a comment id is the reply level; this corpus has no replies,
	// and an empty page is the answer. A 404 here would make c.Comments fail
	// whole, which fetchPageRecord tolerates by dropping the page's comments —
	// exactly the silent strip this fixture must not cause.
	writeJSON(w, map[string]any{"results": results, "size": len(results), "limit": 100, "start": 0})
}

// serveVersions answers the page-version listing with one row per version.
func (f *poolFixture) serveVersions(w http.ResponseWriter, pageID string) {
	f.enter()
	defer f.exit()
	pg := f.page(pageID)
	if pg == nil {
		http.Error(w, "no such page", http.StatusNotFound)
		return
	}
	results := make([]map[string]any, pg.version)
	for i := 1; i <= pg.version; i++ {
		results[i-1] = map[string]any{
			"number": i, "when": pg.when, "message": "", "minorEdit": false,
			"by": map[string]any{"accountId": "acc-1", "displayName": "Ada Example"},
		}
	}
	writeJSON(w, map[string]any{"results": results, "size": len(results), "limit": 100, "start": 0})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// setFetchWidth points the pass's pool width at n for this test and restores
// it after. Tests in this package run sequentially, so the package-level knob
// is safe to move.
func setFetchWidth(t *testing.T, n int) {
	t.Helper()
	old := FetchConcurrency
	FetchConcurrency = n
	t.Cleanup(func() { FetchConcurrency = old })
}

// poolRun is one full Confluence pass over a pool fixture at the given width.
// Returns the fresh mirror plus the captured log lines.
func poolRun(t *testing.T, f *poolFixture, width int) (*mirror, []string, error) {
	t.Helper()
	setFetchWidth(t, width)
	client := f.client()
	db := newMirror(t)
	var logs []string
	_, err := RunConfluence(context.Background(), confCfg([]string{"AAA"}), db.DB, Options{
		Full:             true,
		ConfluenceClient: client,
		Log:              func(s string) { logs = append(logs, s) },
	})
	if err != nil {
		for _, l := range logs {
			t.Logf("log: %s", l)
		}
	}
	return db, logs, err
}

// (a) The pool bounds origin overlap at the configured width and the single
// writer still commits in listing order. FAIL-first for GDK-1673: against the
// serial pass with width 4 configured this reports max in-flight 1.
func TestConfluencePoolWidthBoundedAndCommitOrderHeld(t *testing.T) {
	f := newPoolFixture(t, poolCorpus("AAA", 6)...)
	f.setBarrier(4)

	db, _, err := poolRun(t, f, 4)
	if err != nil {
		t.Fatal(err)
	}

	if got := f.maxInFlightSeen(); got != 4 {
		t.Fatalf("max in-flight origin requests = %d, want 4 (the configured width)", got)
	}

	// Commit order: the single writer upserts the batch in listing order, so
	// on a fresh mirror items rowid order == CQL listing order == corpus order.
	var keys []string
	rows, err := db.raw(t).Query(`SELECT key FROM items WHERE source_id='confluence' ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, pg := range f.pages {
		want = append(want, pg.id)
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("commit order = %v, want listing order %v", keys, want)
	}

	// Every body fetched exactly once: the pool adds no requests.
	got := f.bodiesFetched()
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("body fetches = %v, want exactly one per page (%v)", got, want)
	}
}

// (b) A 429 mid-pass halves the effective width (AIMD), the worker honors
// Retry-After (the transport waits it out), and the pass still commits every
// page. The summary line names the configured width, the width it sank to,
// and the throttle count.
//
// The halving is observed per fetch window (meter delta), so a second worker
// whose window happens to straddle the same 429 also reports it and the width
// can sink to 1 — over-reaction is the documented Throttle behavior. The
// assertion accepts 4/2 and 4/1; the throttle count is exactly 1 because the
// fixture injects exactly one 429 response.
func TestConfluencePool429HalvesWidthAndCompletes(t *testing.T) {
	f := newPoolFixture(t, poolCorpus("AAA", 6)...)
	f.mu.Lock()
	f.throttleID = f.pages[1].id
	f.mu.Unlock()

	setFetchWidth(t, 4)
	client := f.client()
	db := newMirror(t)
	var logs []string
	start := time.Now()
	_, err := RunConfluence(context.Background(), confCfg([]string{"AAA"}), db.DB, Options{
		Full:             true,
		ConfluenceClient: client,
		Log:              func(s string) { logs = append(logs, s) },
	})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	// Every page committed despite the throttle.
	var pages int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if pages != 6 {
		t.Fatalf("pages after a throttled pass = %d, want 6", pages)
	}

	// Retry-After: 1 was honored — the throttled fetch waited ~1s before its
	// retry, which is the "the worker waits" half of the contract.
	if elapsed < 900*time.Millisecond {
		t.Fatalf("pass took %s; the injected Retry-After: 1 was not waited out", elapsed)
	}

	sawSummary := false
	for _, l := range logs {
		if !strings.HasPrefix(l, "confluence: concurrency=4/") {
			continue
		}
		sawSummary = true
		if !strings.HasSuffix(l, " throttled=1") {
			t.Fatalf("summary line = %q, want it to end with throttled=1", l)
		}
		min := strings.TrimSuffix(strings.TrimPrefix(l, "confluence: concurrency=4/"), " throttled=1")
		if min != "2" && min != "1" {
			t.Fatalf("summary line = %q, want AIMD to have sunk the effective width to 2 or 1", l)
		}
	}
	if !sawSummary {
		t.Fatalf("no concurrency summary line in logs: %q", logs)
	}
}

// (c) Width 1 must produce byte-identical mirror rows: the pool is a
// scheduling change, not a data change. Two fresh mirrors over one shared
// fixture (the stored url column embeds the server's address, so two servers
// could never match), one pass at width 1 and one at width 4; every
// origin-derived column must match. (The serial pre-1673 pass is not runnable
// from inside the tree; width 1 through the pool is its stand-in — same
// single worker, same order, same request count, PauseBetween on.)
// items.synced_at is the one column that legitimately differs (it is gadak's
// own clock, not the origin's), so it is not selected.
func TestConfluencePoolWidth1RowsMatchWidth4(t *testing.T) {
	f := newPoolFixture(t, poolCorpus("AAA", 6)...)
	before1 := len(f.bodiesFetched())
	db1, _, err := poolRun(t, f, 1)
	if err != nil {
		t.Fatal(err)
	}
	mid := len(f.bodiesFetched())
	db4, _, err := poolRun(t, f, 4)
	if err != nil {
		t.Fatal(err)
	}
	after := len(f.bodiesFetched())

	dump := func(m *mirror) string {
		var b strings.Builder
		q := func(sql string) {
			rows, err := m.raw(t).Query(sql)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			cols, err := rows.Columns()
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					t.Fatal(err)
				}
				for i, v := range vals {
					fmt.Fprintf(&b, "%s=%v|", cols[i], v)
				}
				b.WriteByte('\n')
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		}
		q(`SELECT id, kind, external_id, key, title, body_text, author, author_id, url, created_at, updated_at
		   FROM items WHERE source_id='confluence' ORDER BY key`)
		q(`SELECT p.item_id, p.space_key, p.parent_id, p.version, p.status, p.body_adf, p.labels, p.excerpt
		   FROM pages p JOIN items i ON i.id=p.item_id WHERE i.source_id='confluence' ORDER BY p.item_id`)
		q(`SELECT c.id, c.item_id, c.external_id, c.author, c.author_id, c.body_adf, c.body_text, c.created_at, c.updated_at
		   FROM comments c JOIN items i ON i.id=c.item_id WHERE i.source_id='confluence' ORDER BY c.id`)
		q(`SELECT v.item_id, v.number, v.created_at, v.author_id, v.author_name, v.message, v.minor_edit
		   FROM page_versions v JOIN items i ON i.id=v.item_id WHERE i.source_id='confluence' ORDER BY v.item_id, v.number`)
		q(`SELECT key, name, kind FROM spaces WHERE source_id='confluence' ORDER BY key`)
		return b.String()
	}

	if d1, d4 := dump(db1), dump(db4); d1 != d4 {
		t.Fatalf("width-1 and width-4 passes produced different mirror rows:\n--- width 1 ---\n%s\n--- width 4 ---\n%s", d1, d4)
	}

	// Same request count both ways: the pool must add no requests.
	if a, b := mid-before1, after-mid; a != 6 || b != 6 {
		t.Fatalf("body fetch counts width1=%d width4=%d, want 6/6", a, b)
	}
}

// (d) One failing body GET cancels the whole pass (errgroup semantics) and
// the watermark does not advance: no per-space floor, no source watermark,
// last_error recorded. With a single sub-batch-size chunk the uncommitted
// batch is discarded — the pre-1673 pass had the same shape, and the
// invariant under test is the watermark, not the batch.
func TestConfluencePoolBodyErrorCancelsAndKeepsWatermark(t *testing.T) {
	f := newPoolFixture(t, poolCorpus("AAA", 6)...)
	f.mu.Lock()
	f.failID = f.pages[3].id
	f.mu.Unlock()

	setFetchWidth(t, 4)
	client := f.client()
	db := newMirror(t)
	_, err := RunConfluence(context.Background(), confCfg([]string{"AAA"}), db.DB, Options{
		Full:             true,
		ConfluenceClient: client,
	})
	if err == nil {
		t.Fatal("a 500 on a page body must fail the pass")
	}

	var pages int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if pages != 0 {
		t.Fatalf("pages after a failed first pass = %d, want 0 (single uncommitted batch)", pages)
	}
	wms, err := db.ConfluenceSpaceWatermarks(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	// The map is keyed by every scoped space (value empty until a pass
	// earns one), so the assertion is on the value, not presence.
	if wm := wms["AAA"]; wm != "" {
		t.Fatalf("space watermark advanced past a failed pass: %q", wm)
	}
	state, err := db.SyncState(context.Background(), ConfluenceSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Watermark != "" {
		t.Fatalf("source watermark = %q after a failed pass, want empty", state.Watermark)
	}
	if state.LastError == nil || !strings.Contains(*state.LastError, "500") {
		t.Fatalf("last_error not recorded: %+v", state.LastError)
	}
}

// --- Throttle / fetchOrdered unit contracts ---

// AIMD arithmetic: halve on throttle floored at 1, grow by one after
// throttleGrowAfter consecutive clean windows, never past configured, and a
// throttle resets the clean streak.
func TestThrottleHalvesFloorsAndRegrows(t *testing.T) {
	th := newThrottle(8)
	if th.Effective() != 8 || th.MinEffective() != 8 {
		t.Fatalf("fresh throttle: effective=%d min=%d, want 8/8", th.Effective(), th.MinEffective())
	}
	for _, want := range []int{4, 2, 1, 1} {
		th.NoteThrottle()
		if th.Effective() != want {
			t.Fatalf("after throttle: effective=%d, want %d", th.Effective(), want)
		}
	}
	if th.MinEffective() != 1 {
		t.Fatalf("min effective=%d, want 1", th.MinEffective())
	}
	// 19 cleans: not yet. The 20th grows to 2.
	for i := 0; i < 19; i++ {
		th.NoteClean()
	}
	if th.Effective() != 1 {
		t.Fatalf("after 19 cleans: effective=%d, want 1", th.Effective())
	}
	th.NoteClean()
	if th.Effective() != 2 {
		t.Fatalf("after 20 cleans: effective=%d, want 2", th.Effective())
	}
	// A throttle resets the streak: halve to 1, then one clean is not growth.
	th.NoteThrottle()
	if th.Effective() != 1 {
		t.Fatalf("after throttle from 2: effective=%d, want 1", th.Effective())
	}
	th.NoteClean()
	if th.Effective() != 1 {
		t.Fatalf("one clean after throttle: effective=%d, want 1 (streak reset)", th.Effective())
	}
	// Growth never passes the configured width.
	th2 := newThrottle(2)
	th2.NoteClean()
	if th2.Effective() != 2 {
		t.Fatalf("clean at configured width: effective=%d, want 2 (no growth past configured)", th2.Effective())
	}
}

// The admission semaphore actually gates: with width n, the n+1th Acquire
// blocks until a Release, and a cancelled context unblocks a waiter.
func TestThrottleAcquireGatesAtEffectiveWidth(t *testing.T) {
	th := newThrottle(3)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := th.Acquire(ctx); err != nil {
			t.Fatal(err)
		}
	}
	acquired := make(chan error, 1)
	go func() { acquired <- th.Acquire(ctx) }()
	select {
	case err := <-acquired:
		t.Fatalf("4th acquire at width 3 returned %v; want it blocked", err)
	case <-time.After(50 * time.Millisecond):
	}
	th.Release()
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatalf("acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("acquire stayed blocked after a release")
	}
	// A cancelled context unblocks a waiting acquire.
	go func() {
		if err := th.Acquire(ctx); err != nil { // takes the freed slot back
			t.Errorf("re-acquire after release: %v", err)
		}
	}()
	time.Sleep(20 * time.Millisecond) // let the goroutine take the slot
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := th.Acquire(cctx); err == nil {
		t.Fatal("cancelled acquire must return ctx.Err, not nil")
	}
}

// fetchOrdered: emit runs on the caller's goroutine, in item order, however
// the fetches interleave; the first error stops emission and is returned.
func TestFetchOrderedEmitsInOrderStopsAtFirstError(t *testing.T) {
	th := newThrottle(4)
	var got []int
	items := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	err := fetchOrdered(context.Background(), th, items,
		func(_ context.Context, i int) (int, error) {
			// Scatter completion order away from item order.
			time.Sleep(time.Duration((i*7)%5) * time.Millisecond)
			if i == 6 {
				return 0, errors.New("boom on 6")
			}
			return i, nil
		},
		func(i, v int) error {
			if v != i {
				t.Errorf("emit(%d, %d): value does not match index", i, v)
			}
			got = append(got, i)
			return nil
		})
	if err == nil || err.Error() != "boom on 6" {
		t.Fatalf("err = %v, want boom on 6", err)
	}
	for i, v := range got {
		if i != v {
			t.Fatalf("emit order = %v, want a strict prefix in item order", got)
		}
	}
	if len(got) > 7 {
		t.Fatalf("emit ran past the failing item: %v", got)
	}
}

// fetchOrdered bounds in-flight fetches at the configured width, made
// deterministic with a gate: the first `width` fetches hold until all of them
// overlap — impossible unless the executor really runs width concurrently —
// then release.
func TestFetchOrderedBoundsInFlightAtWidth(t *testing.T) {
	const width = 3
	th := newThrottle(width)
	gate := make(chan struct{})
	var gateOnce sync.Once
	var inflight atomic.Int32
	var maxSeen atomic.Int32
	err := fetchOrdered(context.Background(), th, []int{0, 1, 2, 3, 4, 5},
		func(_ context.Context, _ int) (struct{}, error) {
			cur := inflight.Add(1)
			for {
				old := maxSeen.Load()
				if cur <= old || maxSeen.CompareAndSwap(old, cur) {
					break
				}
			}
			if cur == width {
				// The gate only has to open once; later windows can hit
				// `width` again and must not close a closed channel.
				gateOnce.Do(func() { close(gate) })
			}
			select {
			case <-gate:
			case <-time.After(2 * time.Second):
			}
			inflight.Add(-1)
			return struct{}{}, nil
		},
		func(i int, _ struct{}) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := maxSeen.Load(); got != width {
		t.Fatalf("max in-flight = %d, want %d", got, width)
	}
}
