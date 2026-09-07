package sync

// GDK-1400 — FAIL-first reproduction, written by the investigation round.
//
// Symptom in the field: a `gadak serve` host's mirror held 134 of its origin's
// 1,399 issues; every incremental pass reported "0 fetched … (27 unchanged)"
// and `gadak sync --full` was the only repair. The origin had received the
// other 1,265 issues from a bulk import (`gadak migrate`, GDK-1262) that
// preserves each issue's ORIGINAL created/updated stamps — so they landed in
// the origin with `updated` OLDER than the mirror's watermark.
//
// The incremental pass asks the origin `updated >= <watermark - 2m>`
// (internal/sync/sync.go:1282 incrementalJQL, :1360 jqlTime, :43 overlap) and
// the watermark only ever moves forward (internal/store/write.go:1250-1256).
// A row written behind the watermark is therefore invisible to every future
// incremental pass — permanently, not until the next tick.
//
// The fake origin below is the one thing the existing sync_test.go fakeSite
// does NOT do: it honours the `updated >=` clause, the way issuetap's JQL
// engine does (issuetap internal/jql/jql.go:397, filtering on iss.Updated) and
// the way Jira Cloud does. That honesty is what makes the defect visible.
//
// This test is RED on the current tree. Do not weaken it to make it green —
// the fix belongs in the sync/import contract (reset the watermark after a
// bulk import, or notice divergence in the reconcile pass, which already
// fetches every upstream key: internal/sync/sync.go:786 reconcile).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
)

// honestSite is a minimal origin that actually applies the `updated >=` floor
// in the JQL it is handed. Everything else is the smallest payload the
// connector accepts.
type honestSite struct {
	t  *testing.T
	mu sync.Mutex
	// issues is the origin's record, keyed by issue key.
	issues map[string]json.RawMessage
	// updated mirrors each issue's `updated` stamp for the filter.
	updated map[string]time.Time
	// project mirrors each issue's project key for the `project in (…)` filter.
	project map[string]string
	// searchJQLs records every JQL this site was asked (sync + reconcile).
	searchJQLs []string
	// keyBatches records the size of each `key in (…)` clause the site was
	// asked, so a test can assert the keyed fetch is actually batched.
	keyBatches []int
}

func newHonestSite(t *testing.T) *honestSite {
	return &honestSite{t: t, issues: map[string]json.RawMessage{}, updated: map[string]time.Time{},
		project: map[string]string{}}
}

// put writes an issue into the origin at an explicit `updated` stamp. Passing
// a stamp in the past is exactly what a bulk import does: `gadak migrate`
// carries the source issue's own created/updated through the seed document
// (internal/migrate/migrate.go:96-97, filled at :341) and issuetap's ingest
// honours them verbatim (issuetap internal/store/store.go:697-703).
func (s *honestSite) put(key string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stamp := at.Format(jira.Layout)
	project := key
	if i := strings.Index(key, "-"); i > 0 {
		project = key[:i]
	}
	body, err := json.Marshal(map[string]any{
		"id": strconv.Itoa(20000 + len(s.issues)), "key": key,
		"fields": map[string]any{
			"summary":   "issue " + key,
			"project":   map[string]any{"key": project},
			"issuetype": map[string]any{"id": "10004", "name": "Task"},
			"status": map[string]any{"id": "1", "name": "To Do",
				"statusCategory": map[string]any{"key": "new"}},
			"priority": map[string]any{"id": "3", "name": "Medium"},
			"created":  stamp,
			"updated":  stamp,
			"comment":  map[string]any{"total": 0, "comments": []any{}},
		},
		"changelog": map[string]any{"total": 0, "histories": []any{}},
	})
	if err != nil {
		s.t.Fatal(err)
	}
	s.issues[key] = body
	s.updated[key] = at
	s.project[key] = project
}

var floorRe = regexp.MustCompile(`updated >= "([^"]+)"`)

// keyInRe matches the keyed-fetch clause `key in ("A", "B")`. The origin must
// honour it as narrowly as it honours the `updated >=` floor: a fake that
// answered a keyed fetch with its whole corpus would make a broken batching
// loop look like a working one.
var keyInRe = regexp.MustCompile(`key in \(([^)]*)\)`)
var quotedRe = regexp.MustCompile(`"([^"]+)"`)

// projectRe matches both project clauses the pass emits: `project in ("A",
// "B")` for a scoped search and `project = "A"` for one leg of a full pass.
// A fake that ignored them would mirror an out-of-scope project and make a
// scope-widening test green before the code deserved it.
var projectRe = regexp.MustCompile(`project (?:in \(([^)]*)\)|= ("[^"]+"))`)

// matching is the origin's honest answer to one JQL: every issue at or after
// the `updated >=` floor, restricted to an explicit key list when the clause
// carries one, oldest first. No clause means every issue.
func (s *honestSite) matching(jql string) []json.RawMessage {
	var floor time.Time
	if m := floorRe.FindStringSubmatch(jql); m != nil {
		t, err := time.ParseInLocation("2006/01/02 15:04", m[1], time.Local)
		if err != nil {
			s.t.Fatalf("unparsable JQL floor %q: %v", m[1], err)
		}
		floor = t
	}
	var projects map[string]bool
	if m := projectRe.FindStringSubmatch(jql); m != nil {
		projects = map[string]bool{}
		for _, q := range quotedRe.FindAllStringSubmatch(m[1]+m[2], -1) {
			projects[q[1]] = true
		}
	}
	var only map[string]bool
	if m := keyInRe.FindStringSubmatch(jql); m != nil {
		only = map[string]bool{}
		for _, q := range quotedRe.FindAllStringSubmatch(m[1], -1) {
			only[q[1]] = true
		}
		s.keyBatches = append(s.keyBatches, len(only))
	}
	keys := make([]string, 0, len(s.issues))
	for k := range s.issues {
		if only != nil && !only[k] {
			continue
		}
		if projects != nil && !projects[s.project[k]] {
			continue
		}
		if floor.IsZero() || !s.updated[k].Before(floor) {
			keys = append(keys, k)
		}
	}
	// oldest first, which is what `ORDER BY updated ASC` promises.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && s.updated[keys[j]].Before(s.updated[keys[j-1]]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	out := make([]json.RawMessage, 0, len(keys))
	for _, k := range keys {
		out = append(out, s.issues[k])
	}
	return out
}

func (s *honestSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/rest/api/3/search/jql":
		var body struct {
			JQL           string `json:"jql"`
			Expand        string `json:"expand"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.t.Fatalf("bad search body: %v", err)
		}
		s.searchJQLs = append(s.searchJQLs, body.JQL)
		hits := s.matching(body.JQL)
		_ = json.NewEncoder(w).Encode(map[string]any{"issues": hits, "isLast": true})
	case r.URL.Path == "/rest/api/3/search/approximate-count":
		var body struct {
			JQL string `json:"jql"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]int{"count": len(s.matching(body.JQL))})
	case r.URL.Path == "/rest/api/3/status":
		w.Write([]byte(`[{"id":"1","name":"To Do","statusCategory":{"key":"new"}}]`))
	case r.URL.Path == "/rest/api/3/priority":
		w.Write([]byte(`[{"id":"3","name":"Medium"}]`))
	case r.URL.Path == "/rest/api/3/issueLinkType":
		w.Write([]byte(`{"issueLinkTypes":[]}`))
	case r.URL.Path == "/rest/api/3/field":
		w.Write([]byte(`[]`))
	case r.URL.Path == "/rest/api/3/filter/my":
		w.Write([]byte(`[]`))
	case strings.HasSuffix(r.URL.Path, "/versions"):
		w.Write([]byte(`[]`))
	default:
		// Anything else this pass wants is not what the test is about.
		w.Write([]byte(`{}`))
	}
}

func (s *honestSite) start() *jira.Client {
	srv := httptest.NewServer(s)
	s.t.Cleanup(srv.Close)
	c := jira.New(srv.URL, "someone@example.com", "secret-token")
	c.Retries, c.Backoff = 1, 0
	return c
}

func gdkConfig() *config.Config {
	return &config.Config{
		Site: "https://example.atlassian.net", Email: "someone@example.com", Token: "secret-token",
		Projects: []string{"GDK"},
	}
}

func mirrorKeys(t *testing.T, db *mirror) []string {
	t.Helper()
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(lites))
	for _, l := range lites {
		keys = append(keys, l.IssueKey)
	}
	return keys
}

// TestIncrementalSeesRowsWrittenBehindTheWatermark is the GDK-1400 contract:
// a row that appears in the origin carrying an `updated` stamp older than the
// mirror's watermark must still reach the mirror. Today it never does.
func TestIncrementalSeesRowsWrittenBehindTheWatermark(t *testing.T) {
	ctx := context.Background()
	site := newHonestSite(t)
	db := newMirror(t)
	cfg := gdkConfig()
	client := site.start()

	// The mirror has been following this origin for a while. One live issue,
	// so the first pass is full and the watermark lands on its stamp.
	live := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	site.put("GDK-900", live)

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	state, err := db.SyncState(ctx, SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Watermark == "" {
		t.Fatalf("no watermark after the first full pass")
	}
	t.Logf("watermark after the first pass: %s", state.Watermark)

	// Now the cutover: a bulk import lands three issues in the origin with
	// their ORIGINAL stamps — days behind the watermark. `gadak migrate` does
	// exactly this, and this host is not the host that ran it.
	site.put("GDK-1", live.Add(-72*time.Hour))
	site.put("GDK-2", live.Add(-48*time.Hour))
	site.put("GDK-3", live.Add(-24*time.Hour))

	// The next incremental tick. This is the pass the serve host runs forever.
	res, err := Run(ctx, cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Fatalf("this pass must be incremental; the defect only exists there")
	}
	t.Logf("incremental JQL: %v", site.searchJQLs)
	t.Logf("incremental pass: fetched %d changed %d unchanged %d", res.Fetched, res.Changed, res.Skips)

	got := mirrorKeys(t, db)
	if len(got) != 4 {
		t.Errorf("after the import the mirror holds %d issues %v, want 4 (GDK-1400: rows written behind the watermark are invisible to `updated >= watermark`)", len(got), got)
	}

	// The hourly reconcile pass does not repair it either, even though it
	// already fetches every upstream key in scope (sync.go:786) — it only
	// looks for mirror rows that vanished upstream, never for upstream keys
	// the mirror is missing.
	if _, err := Run(ctx, cfg, db.DB, Options{Reconcile: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	got = mirrorKeys(t, db)
	if len(got) != 4 {
		t.Errorf("after a reconcile pass the mirror holds %d issues %v, want 4 (the reconcile already has the full upstream key list and throws it away)", len(got), got)
	}

	// The tally the field diagnosis needed and could not get: the origin's own
	// key count for this scope, beside what the pass had to repair. Learning
	// that number took a second machine paired to the same origin.
	state, err = db.SyncState(ctx, SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Reconcile.UpstreamKeys != 4 {
		t.Errorf("reconcile.upstream_keys = %d, want 4", state.Reconcile.UpstreamKeys)
	}
	if state.Reconcile.GoneDeleted != 0 {
		t.Errorf("reconcile.gone_deleted = %d, want 0 — nothing vanished upstream", state.Reconcile.GoneDeleted)
	}
	if !state.Reconcile.Ran() {
		t.Error("reconcile tally has no timestamp")
	}
	if state.ScopeHash != "GDK" {
		t.Errorf("scope signature = %q, want %q", state.ScopeHash, "GDK")
	}

	// MissingFetched counts rows that actually landed, not keys that were
	// asked for. A pass that asked for a thousand and mirrored none must not
	// report a repair — that reading is what would send the next person
	// looking in the wrong place.
	if state.Reconcile.MissingFetched != 0 {
		t.Errorf("reconcile.missing_fetched = %d on a pass that had nothing left to fetch, want 0", state.Reconcile.MissingFetched)
	}
	mirrored, err := db.CountIssuesInScope(ctx, SourceID, cfg.Projects)
	if err != nil {
		t.Fatal(err)
	}
	if mirrored != state.Reconcile.UpstreamKeys {
		t.Errorf("mirror holds %d in scope, origin reported %d — the two must agree after a repair", mirrored, state.Reconcile.UpstreamKeys)
	}
}

// TestScopeChangeFetchesWhatTheNewProjectAlreadyHad is the second reachable
// trigger of the same class (GDK-1400): the watermark is one cursor per
// source_id, not per project. Adding a project to the scope therefore points a
// cursor earned on the old scope at issues the new project has had all along —
// every one of them older than it. Before the fix the new project's backlog
// simply never arrived, and no error was ever printed.
func TestScopeChangeFetchesWhatTheNewProjectAlreadyHad(t *testing.T) {
	ctx := context.Background()
	site := newHonestSite(t)
	db := newMirror(t)
	cfg := gdkConfig()
	client := site.start()

	// LOC is an old project. Its issues predate anything GDK has.
	now := time.Now().Truncate(time.Second)
	site.put("LOC-1", now.Add(-96*time.Hour))
	site.put("LOC-2", now.Add(-95*time.Hour))
	site.put("GDK-900", now.Add(-1*time.Hour))

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if got := mirrorKeys(t, db); len(got) != 1 {
		t.Fatalf("first pass mirrored %v, want only the configured project's GDK-900", got)
	}
	state, err := db.SyncState(ctx, SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ScopeHash != "GDK" {
		t.Fatalf("scope signature after the first pass = %q, want %q", state.ScopeHash, "GDK")
	}

	// The operator adds LOC to the scope. Every path that does this — the
	// command, a hand-edited config.json, a workspace copied in — arrives here
	// as a changed project list and nothing else.
	cfg.Projects = []string{"GDK", "LOC"}

	res, err := Run(ctx, cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Full {
		t.Errorf("a pass under a widened scope must be full; the stored watermark was earned on the old scope")
	}
	got := mirrorKeys(t, db)
	if len(got) != 3 {
		t.Errorf("after widening the scope the mirror holds %d issues %v, want 3 (GDK-1400: the new project's existing issues are all behind the cursor)", len(got), got)
	}
	state, err = db.SyncState(ctx, SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ScopeHash != "GDK,LOC" {
		t.Errorf("scope signature after the widened pass = %q, want %q", state.ScopeHash, "GDK,LOC")
	}

	// And it settles: the next tick is incremental again, because the scope
	// now matches what the watermark was earned under. A signature that never
	// converged would mean a full pass every tick forever.
	res, err = Run(ctx, cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Errorf("an unchanged scope must not force a second full pass")
	}
}

// TestScopeSignatureIgnoresOrderAndSpacing: the signature answers "is this the
// scope the watermark was earned under", so a reordered or re-spaced config
// list is the same scope and must not cost a full pass. "*" is the unscoped
// account and is a real value — the empty string means "never recorded", which
// is a pre-v44 mirror, and confusing the two would either force a full pass on
// every upgrade or never force one at all.
func TestScopeSignatureIgnoresOrderAndSpacing(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{nil, "*"},
		{[]string{}, "*"},
		{[]string{"  "}, "*"},
		{[]string{"GDK"}, "GDK"},
		{[]string{"LOC", "GDK"}, "GDK,LOC"},
		{[]string{"GDK ", " LOC"}, "GDK,LOC"},
		{[]string{"GDK", "GDK"}, "GDK"},
	} {
		if got := scopeSignature(tc.in); got != tc.want {
			t.Errorf("scopeSignature(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestReconcileBatchesTheKeyedFetch: the repair fetch is a `key in (…)` clause
// and must be split, not sent as one clause with every missing key in it. The
// field case was 1,265 missing keys; a single clause that long is a request
// body both Jira Cloud and issuetap have to parse in one piece, and one
// failure would re-read all of it.
func TestReconcileBatchesTheKeyedFetch(t *testing.T) {
	ctx := context.Background()
	site := newHonestSite(t)
	db := newMirror(t)
	cfg := gdkConfig()
	client := site.start()

	live := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	site.put("GDK-900", live)
	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}

	const missing = keyBatchSize*2 + 7
	for i := 0; i < missing; i++ {
		site.put(fmt.Sprintf("GDK-%d", i+1), live.Add(-time.Duration(i+1)*time.Minute))
	}

	if _, err := Run(ctx, cfg, db.DB, Options{Client: client}); err != nil {
		t.Fatal(err)
	}
	if got := len(mirrorKeys(t, db)); got != missing+1 {
		t.Fatalf("mirror holds %d issues, want %d", got, missing+1)
	}
	if len(site.keyBatches) != 3 {
		t.Fatalf("keyed fetch sent %d clauses %v, want 3 (%d, %d, 7)",
			len(site.keyBatches), site.keyBatches, keyBatchSize, keyBatchSize)
	}
	for i, n := range site.keyBatches {
		if n > keyBatchSize {
			t.Errorf("clause %d carried %d keys, over the %d cap", i, n, keyBatchSize)
		}
	}
}

// honestLinearSite is the Linear counterpart of honestSite: a GraphQL stub
// that actually applies `filter.updatedAt.gte`, the way Linear's API does.
// The canned-page stub in linear_test.go answers every cursor with the same
// body and so cannot show this defect at all.
type honestLinearSite struct {
	t       *testing.T
	nodes   []map[string]any
	updated map[string]string
	// gtes records the updatedAt floor of every query, so a test can tell an
	// incremental listing from the reconcile's unfiltered one.
	gtes []string
}

func (s *honestLinearSite) put(key, id, updatedAt string) {
	s.nodes = append(s.nodes, linearNode(key, id, "unstarted", "Todo", 0, "No priority",
		map[string]any{"updatedAt": updatedAt}))
	if s.updated == nil {
		s.updated = map[string]string{}
	}
	s.updated[key] = updatedAt
}

func (s *honestLinearSite) start() *linear.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables struct {
				Filter struct {
					UpdatedAt struct {
						Gte string `json:"gte"`
					} `json:"updatedAt"`
				} `json:"filter"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.t.Fatalf("bad request body: %v", err)
		}
		gte := req.Variables.Filter.UpdatedAt.Gte
		s.gtes = append(s.gtes, gte)
		hits := []map[string]any{}
		for _, n := range s.nodes {
			// Linear stamps are ISO-8601 UTC with milliseconds, so a
			// lexicographic compare is a chronological one.
			if gte == "" || s.updated[n["identifier"].(string)] >= gte {
				hits = append(hits, n)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(linearIssuesResponse(s.t, hits)))
	}))
	s.t.Cleanup(srv.Close)
	return testLinearClient(s.t, srv)
}

// TestLinearReconcileSeesRowsWrittenBehindTheWatermark is the Linear sibling of
// the Jira contract. The cursor is `filter.updatedAt.gte` rather than a JQL
// floor, but the shape of the defect is identical: an issue that arrives in
// the workspace carrying an older updatedAt — a bulk import, an issue moved in
// from another team — sits behind the cursor forever. The reconcile pass
// already lists every issue in scope to prove absence; committing the ones the
// mirror never received costs nothing more on the wire.
func TestLinearReconcileSeesRowsWrittenBehindTheWatermark(t *testing.T) {
	ctx := context.Background()
	site := &honestLinearSite{t: t}
	db := newMirror(t)
	cfg := linearTestConfig()

	site.put("FIX-900", "9", "2026-09-01T00:00:00.000Z")
	client := site.start()
	if _, err := RunLinear(ctx, cfg, db.DB, Options{Full: true, LinearClient: client}); err != nil {
		t.Fatal(err)
	}
	state, err := db.SyncState(ctx, LinearSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Watermark != "2026-09-01T00:00:00.000Z" {
		t.Fatalf("watermark after the first pass = %q", state.Watermark)
	}

	// The import lands, carrying the issues' original stamps.
	site.put("FIX-1", "1", "2026-08-20T00:00:00.000Z")
	site.put("FIX-2", "2", "2026-08-21T00:00:00.000Z")

	// A plain incremental tick cannot see them — that is the defect, and it is
	// permanent, not a delay.
	res, err := RunLinear(ctx, cfg, db.DB, Options{LinearClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full {
		t.Fatal("this pass must be incremental")
	}
	if got := len(mirrorKeys(t, db)); got != 1 {
		t.Fatalf("incremental pass mirrored %d issues, want 1 — the window cannot contain a backdated row", got)
	}

	// The hourly reconcile is what repairs it, unattended.
	if _, err := RunLinear(ctx, cfg, db.DB, Options{Reconcile: true, LinearClient: client}); err != nil {
		t.Fatal(err)
	}
	got := mirrorKeys(t, db)
	if len(got) != 3 {
		t.Fatalf("after reconcile the mirror holds %d issues %v, want 3 (GDK-1400)", len(got), got)
	}
	for _, key := range []string{"FIX-1", "FIX-2"} {
		lite, missing := findLite(t, db, key)
		if missing {
			t.Fatalf("%s not mirrored", key)
		}
		if lite.Summary != "issue "+key {
			t.Errorf("%s summary = %q — the repair must go through the ordinary page pipeline, not a stub row", key, lite.Summary)
		}
	}

	state, err = db.SyncState(ctx, LinearSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Reconcile.UpstreamKeys != 3 || state.Reconcile.MissingFetched != 2 || state.Reconcile.GoneDeleted != 0 {
		t.Errorf("reconcile tally = %+v, want 3 upstream / 2 fetched / 0 deleted", state.Reconcile)
	}
	if !state.Reconcile.Ran() {
		t.Error("reconcile tally has no timestamp")
	}
	// The watermark must not have been dragged backwards by the backdated rows.
	if state.Watermark != "2026-09-01T00:00:00.000Z" {
		t.Errorf("watermark after the repair = %q, want it unmoved", state.Watermark)
	}
}

// TestLinearScopeChangeForcesAFullPass: adding a team is the Linear spelling of
// widening the scope, and the watermark is one cursor per source, not per team.
func TestLinearScopeChangeForcesAFullPass(t *testing.T) {
	ctx := context.Background()
	site := &honestLinearSite{t: t}
	db := newMirror(t)
	cfg := linearTestConfig()
	cfg.Linear.TeamIDs = []string{"team-1"}

	site.put("FIX-900", "9", "2026-09-01T00:00:00.000Z")
	client := site.start()
	if _, err := RunLinear(ctx, cfg, db.DB, Options{Full: true, LinearClient: client}); err != nil {
		t.Fatal(err)
	}
	state, err := db.SyncState(ctx, LinearSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ScopeHash != "team-1" {
		t.Fatalf("scope signature = %q, want %q", state.ScopeHash, "team-1")
	}

	cfg.Linear.TeamIDs = []string{"team-1", "team-2"}
	res, err := RunLinear(ctx, cfg, db.DB, Options{LinearClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Full {
		t.Error("a pass under a widened team scope must be full (GDK-1400)")
	}
	state, err = db.SyncState(ctx, LinearSourceID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ScopeHash != "team-1,team-2" {
		t.Errorf("scope signature = %q, want %q", state.ScopeHash, "team-1,team-2")
	}
}
