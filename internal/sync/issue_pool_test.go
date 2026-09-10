package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/jira"
)

// The Jira fetch-pool tests (GDK-1674). fakeSite (sync_test.go) stays the
// shared stand-in for the mapping tests; these need their own fixture for the
// same reason the Confluence pool tests did: a max in-flight gauge with a
// deterministic rendezvous barrier on the per-issue comment GETs, and one-shot
// 429 / persistent 500 injection on a chosen issue.
//
// Every issue in the corpus carries comment.total=2 with an empty inline
// list — the search response truncation that makes build issue its per-issue
// GET /issue/{key}/comment. That GET is the request the pool overlaps; the
// changelog overflow read rides the same worker shape and is not separately
// exercised (one worker's whole item is build, whatever it fetches).

// issuePoolCorpus builds n single-comment-overflow issues in one project
// whose stamps ascend with the slice index, so listing order == index order
// == commit order.
func issuePoolCorpus(n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("NMB-%d", i+1)
	}
	return out
}

type issuePoolFixture struct {
	mu       sync.Mutex
	srv      *httptest.Server
	keys     []string
	pageSize int

	inFlight    int
	maxInFlight int

	// barrierLeft counts down on the first comment GETs; each holds until
	// the count reaches zero (or the timeout). With barrierLeft == pool
	// width, max in-flight is forced to the width itself, deterministically.
	// Zero (the default) disables it: the countdown goes negative and the
	// wait is skipped.
	barrierLeft int

	throttleKey string // next comment GET on this key: 429 + Retry-After: 1 (once)
	failKey     string // every comment GET on this key: 500

	commentGETs []string
}

func newIssuePoolFixture(t *testing.T, keys ...string) *issuePoolFixture {
	t.Helper()
	f := &issuePoolFixture{keys: keys, pageSize: 5}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.srv = srv
	return f
}

func (f *issuePoolFixture) client() *jira.Client {
	c := jira.New(f.srv.URL, "someone@example.com", "secret-token")
	c.Retries, c.Backoff = 4, time.Millisecond
	return c
}

func (f *issuePoolFixture) setBarrier(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.barrierLeft = n
}

func (f *issuePoolFixture) maxInFlightSeen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInFlight
}

func (f *issuePoolFixture) commentsFetched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commentGETs...)
}

func (f *issuePoolFixture) enter() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
}

func (f *issuePoolFixture) exit() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight--
}

// rendezvous holds this comment GET until barrierLeft reaches zero or the
// timeout passes (750ms: instant when the pool fills the barrier, and cheap
// enough in the serial FAIL-first case where it can never fill).
func (f *issuePoolFixture) rendezvous() {
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

func (f *issuePoolFixture) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/rest/api/3/status":
		w.Write(statusesJSON("en"))
	case r.URL.Path == "/rest/api/3/priority":
		w.Write(prioritiesJSON("en"))
	case r.URL.Path == "/rest/api/3/issueLinkType":
		_ = json.NewEncoder(w).Encode(map[string]any{"issueLinkTypes": []map[string]any{
			{"id": "10000", "name": "Blocks", "inward": "is blocked by", "outward": "blocks"},
		}})
	case r.URL.Path == "/rest/api/3/field":
		w.Write([]byte(`[]`))
	case r.URL.Path == "/rest/api/3/search/approximate-count":
		f.mu.Lock()
		n := len(f.keys)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]int{"count": n})
	case r.URL.Path == "/rest/api/3/search/jql":
		f.serveSearch(w, r)
	case strings.HasSuffix(r.URL.Path, "/comment"):
		f.serveComment(w, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/"), "/comment"))
	case r.URL.Path == "/rest/api/3/filter/my":
		w.Write([]byte(`[]`))
	case strings.HasPrefix(r.URL.Path, "/rest/api/3/project/") && strings.HasSuffix(r.URL.Path, "/versions"):
		w.Write([]byte(`[]`))
	case strings.HasPrefix(r.URL.Path, "/rest/agile/1.0/"):
		http.Error(w, `{"errorMessages":["no agile"]}`, http.StatusNotFound)
	default:
		http.NotFound(w, r)
	}
}

// serveSearch answers every search (the sync chain and the reconcile key
// scan) with the corpus in slice order, pageSize issues per page.
func (f *issuePoolFixture) serveSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	keys, size := append([]string(nil), f.keys...), f.pageSize
	f.mu.Unlock()
	offset, _ := strconv.Atoi(body.NextPageToken)
	end := min(offset+size, len(keys))
	page := map[string]any{"issues": make([]map[string]any, 0, max(end-offset, 0))}
	for _, k := range keys[min(offset, len(keys)):end] {
		page["issues"] = append(page["issues"].([]map[string]any), f.issueJSON(k))
	}
	if end < len(keys) {
		page["nextPageToken"] = strconv.Itoa(end)
	} else {
		page["isLast"] = true
	}
	writeJSON(w, page)
}

// issueJSON is one search hit: the minimum fields build maps, plus the
// truncated comment block (total 2, inline none) that forces the per-issue
// comment GET, and an empty changelog block so no changelog GET is issued.
func (f *issuePoolFixture) issueJSON(key string) map[string]any {
	// The key's numeric suffix — never len(key): NMB-1…NMB-9 share a length,
	// and one shared origin id would collapse the corpus into one upserted
	// row (items.id is the primary key).
	n, _ := strconv.Atoi(strings.TrimPrefix(key, "NMB-"))
	user := func(id, name string) map[string]any {
		return map[string]any{"accountId": id, "displayName": name}
	}
	return map[string]any{
		"id": fmt.Sprintf("1%04d", n), "key": key,
		"fields": map[string]any{
			"summary":   "pool issue " + key,
			"project":   map[string]any{"key": "NMB"},
			"issuetype": issueTypeObj("10002", "en"),
			"status":    statusObj("3", "en"),
			"reporter":  user("acc-sam", "Sam"),
			"creator":   user("acc-sam", "Sam"),
			"created":   "2026-09-01T10:00:00.000+0900",
			"updated":   fmt.Sprintf("2026-09-01T10:%02d:00.000+0900", n),
			"comment":   map[string]any{"total": 2, "comments": []any{}},
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	}
}

// serveComment is the per-issue overflow read the pool overlaps: the
// in-flight gauge, the rendezvous barrier, and the 429 / 500 injections all
// live here.
func (f *issuePoolFixture) serveComment(w http.ResponseWriter, key string) {
	f.enter()
	defer f.exit()
	f.mu.Lock()
	f.commentGETs = append(f.commentGETs, key)
	throttle, fail := f.throttleKey == key, f.failKey == key
	if throttle {
		f.throttleKey = ""
	}
	f.mu.Unlock()
	f.rendezvous()
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
	body := func(id string) map[string]any {
		return map[string]any{"id": id, "author": map[string]any{"accountId": "acc-dana", "displayName": "Dana"},
			"body":    adfDoc("overflow comment " + id + " on " + key),
			"created": "2026-09-02T10:00:00.000+0900", "updated": "2026-09-02T10:00:00.000+0900"}
	}
	writeJSON(w, map[string]any{
		"comments":   []map[string]any{body(key + "-c1"), body(key + "-c2")},
		"total":      2,
		"maxResults": 100,
		"startAt":    0,
	})
}

// issuePoolRun is one full Jira pass over the fixture at the given width.
// Returns the fresh mirror plus the captured log lines.
func issuePoolRun(t *testing.T, f *issuePoolFixture, width int) (*mirror, []string, error) {
	t.Helper()
	setFetchWidth(t, width)
	client := f.client()
	db := newMirror(t)
	var logs []string
	_, err := Run(context.Background(), testConfig(), db.DB, Options{
		Full:   true,
		Client: client,
		Log:    func(s string) { logs = append(logs, s) },
	})
	if err != nil {
		for _, l := range logs {
			t.Logf("log: %s", l)
		}
	}
	return db, logs, err
}

// (a) The pool bounds origin overlap at the configured width and the single
// writer still commits in listing order. FAIL-first for GDK-1674: against
// the serial page loop the barrier of 4 can never fill (max in-flight 1), so
// this test goes red there.
func TestIssuePoolWidthBoundedAndCommitOrderHeld(t *testing.T) {
	f := newIssuePoolFixture(t, issuePoolCorpus(10)...)
	f.setBarrier(4)

	db, _, err := issuePoolRun(t, f, 4)
	if err != nil {
		t.Fatal(err)
	}

	if got := f.maxInFlightSeen(); got != 4 {
		t.Fatalf("max in-flight comment GETs = %d, want 4 (the configured width)", got)
	}

	// Commit order: the single writer upserts the batch in listing order, so
	// on a fresh mirror items rowid order == search listing order == corpus
	// order.
	var keys []string
	rows, err := db.raw(t).Query(`SELECT key FROM items WHERE source_id='jira' ORDER BY rowid`)
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
	want := f.keys
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("commit order = %v, want listing order %v", keys, want)
	}

	// Every overflow read fetched exactly once: the pool adds no requests.
	// Both sides sorted — the claim is one GET per issue, not GET order.
	got := f.commentsFetched()
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("comment fetches = %v, want exactly one per issue (%v)", got, want)
	}
}

// (b) A 429 mid-pass halves the effective width (AIMD), the worker honors
// Retry-After (the transport waits it out), and the pass still commits every
// issue. The summary line names the configured width, the width it sank to,
// and the throttle count.
func TestIssuePool429HalvesWidthAndCompletes(t *testing.T) {
	f := newIssuePoolFixture(t, issuePoolCorpus(6)...)
	f.mu.Lock()
	f.throttleKey = f.keys[1]
	f.mu.Unlock()

	setFetchWidth(t, 4)
	client := f.client()
	db := newMirror(t)
	var logs []string
	start := time.Now()
	_, err := Run(context.Background(), testConfig(), db.DB, Options{
		Full:   true,
		Client: client,
		Log:    func(s string) { logs = append(logs, s) },
	})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	// Every issue committed despite the throttle — both its comments too.
	var issues int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&issues); err != nil {
		t.Fatal(err)
	}
	if issues != 6 {
		t.Fatalf("issues after a throttled pass = %d, want 6", issues)
	}
	var comments int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if comments != 12 {
		t.Fatalf("comments after a throttled pass = %d, want 12 (2 per issue)", comments)
	}

	// Retry-After: 1 was honored — the throttled fetch waited ~1s before its
	// retry, which is the "the worker waits" half of the contract.
	if elapsed < 900*time.Millisecond {
		t.Fatalf("pass took %s; the injected Retry-After: 1 was not waited out", elapsed)
	}

	sawSummary := false
	for _, l := range logs {
		if !strings.HasPrefix(l, "jira: concurrency=4/") {
			continue
		}
		sawSummary = true
		if !strings.HasSuffix(l, " throttled=1") {
			t.Fatalf("summary line = %q, want it to end with throttled=1", l)
		}
		min := strings.TrimSuffix(strings.TrimPrefix(l, "jira: concurrency=4/"), " throttled=1")
		if min != "2" && min != "1" {
			t.Fatalf("summary line = %q, want AIMD to have sunk the effective width to 2 or 1", l)
		}
	}
	if !sawSummary {
		t.Fatalf("no concurrency summary line in logs: %q", logs)
	}
}

// (c) Width 1 must produce identical mirror rows: the pool is a scheduling
// change, not a data change. Two fresh mirrors over one shared fixture (the
// stored url column embeds the server's address, so two servers could never
// match), one pass at width 1 and one at width 4; every origin-derived
// column must match. items.synced_at is the one column that legitimately
// differs (it is gadak's own clock, not the origin's), so it is not
// selected. (The serial pre-1674 pass is not runnable from inside the tree;
// width 1 through the pool is its stand-in.)
func TestIssuePoolWidth1RowsMatchWidth4(t *testing.T) {
	f := newIssuePoolFixture(t, issuePoolCorpus(6)...)
	before1 := len(f.commentsFetched())
	db1, _, err := issuePoolRun(t, f, 1)
	if err != nil {
		t.Fatal(err)
	}
	mid := len(f.commentsFetched())
	db4, _, err := issuePoolRun(t, f, 4)
	if err != nil {
		t.Fatal(err)
	}
	after := len(f.commentsFetched())

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
		   FROM items WHERE source_id='jira' ORDER BY key`)
		q(`SELECT i.key, i.project_key, i.issue_type, i.issue_type_id, i.status, i.status_id, i.status_category,
		          i.priority, i.priority_id, i.assignee, i.reporter, i.created_at, i.updated_at
		   FROM issues i JOIN items it ON it.id=i.item_id WHERE it.source_id='jira' ORDER BY i.key`)
		q(`SELECT c.id, c.item_id, c.external_id, c.author, c.author_id, c.body_text, c.created_at, c.updated_at
		   FROM comments c JOIN items it ON it.id=c.item_id WHERE it.source_id='jira' ORDER BY c.id`)
		return b.String()
	}

	if d1, d4 := dump(db1), dump(db4); d1 != d4 {
		t.Fatalf("width-1 and width-4 passes produced different mirror rows:\n--- width 1 ---\n%s\n--- width 4 ---\n%s", d1, d4)
	}

	// Same request count both ways: the pool must add no requests.
	if a, b := mid-before1, after-mid; a != 6 || b != 6 {
		t.Fatalf("comment fetch counts width1=%d width4=%d, want 6/6", a, b)
	}
}

// (d) One failing comment GET cancels the whole page (errgroup semantics)
// and the watermark does not advance: no source watermark, last_error
// recorded, nothing committed. The invariant under test is the watermark —
// the serial pass had the same failure shape.
func TestIssuePoolCommentErrorCancelsAndKeepsWatermark(t *testing.T) {
	f := newIssuePoolFixture(t, issuePoolCorpus(6)...)
	f.mu.Lock()
	f.failKey = f.keys[3]
	f.mu.Unlock()

	setFetchWidth(t, 4)
	client := f.client()
	db := newMirror(t)
	_, err := Run(context.Background(), testConfig(), db.DB, Options{
		Full:   true,
		Client: client,
	})
	if err == nil {
		t.Fatal("a 500 on a comment fetch must fail the pass")
	}

	var issues int
	if err := db.raw(t).QueryRow(`SELECT COUNT(*) FROM issues`).Scan(&issues); err != nil {
		t.Fatal(err)
	}
	if issues != 0 {
		t.Fatalf("issues after a failed first pass = %d, want 0 (the failed page never commits)", issues)
	}
	state, err := db.SyncState(context.Background(), SourceID)
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
