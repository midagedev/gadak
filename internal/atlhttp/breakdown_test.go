package atlhttp

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestClassifyRequestDialects pins the classifier against one real route of
// every dialect the clients speak (GDK-1672): Jira Cloud v3, Jira Server v2,
// Confluence Cloud v1 (client-relative — /wiki lives in the base), the
// built-in tracker (same shapes), the v2 pages route, and Agile. Paths are
// the exact strings the clients pass to DoRaw, query strings included where
// the client builds them that way.
func TestClassifyRequestDialects(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		// Jira Cloud v3.
		{"POST", "/rest/api/3/search/jql", KindSearchPages},
		{"POST", "/rest/api/3/search/approximate-count", KindSearchPages},
		{"GET", "/rest/api/3/search?jql=order+by+updated&maxResults=100", KindSearchPages},
		{"GET", "/rest/api/3/issue/PROJ-42/comment?startAt=0&maxResults=100", KindIssueComment},
		{"GET", "/rest/api/3/issue/PROJ-42/changelog?startAt=0&maxResults=100", KindIssueChangelog},
		{"GET", "/rest/api/3/myself", KindOther},
		{"POST", "/rest/api/3/issue", KindOther}, // create is a write, not a read kind
		{"POST", "/rest/api/3/issue/PROJ-42/comment", KindOther},
		// Jira Server / Data Center v2.
		{"POST", "/rest/api/2/search", KindSearchPages},
		{"GET", "/rest/api/2/issue/ABC-1/comment?startAt=100&maxResults=100", KindIssueComment},
		{"GET", "/rest/api/2/issue/ABC-1/changelog?startAt=0&maxResults=100", KindIssueChangelog},
		// Confluence v1 content tree — the same routes the built-in tracker
		// serves in-process.
		{"GET", "/rest/api/content/search?cql=type%3Dpage&limit=50&expand=version%2Cspace", KindPageList},
		{"GET", "/rest/api/content/987?expand=body.atlas_doc_format,version,space,ancestors,metadata.labels", KindPageBody},
		{"GET", "/rest/api/content/987/child/comment?expand=body.atlas_doc_format,version&limit=100&start=0", KindPageComments},
		{"GET", "/rest/api/content/987/version?limit=100", KindPageVersions},
		{"GET", "/rest/api/space?limit=100&start=0", KindOther},
		{"POST", "/rest/api/content", KindOther},          // page create is a write
		{"PUT", "/rest/api/content/987", KindOther},       // page update is a write
		{"POST", "/rest/api/content/987", KindOther},      // comment create is a write
		{"GET", "/rest/api/content/search", KindPageList}, // next-page follow keeps its kind
		// The v2 pages route (spec'd even though issuetap marks it
		// unsupported in v0): classified as listing, prefix-matched.
		{"GET", "/api/v2/pages?limit=25", KindPageList},
		{"GET", "/api/v2/pages/123", KindPageList},
		// A routed /wiki prefix is tolerated (callers that pass the mounted
		// path) — the Confluence client itself keeps /wiki in its base.
		{"GET", "/wiki/rest/api/content/987/version?limit=100", KindPageVersions},
		// Agile: same prefix on every dialect that has it.
		{"GET", "/rest/agile/1.0/board?maxResults=50", KindAgile},
		{"GET", "/rest/agile/1.0/board/7/sprint?startAt=0&maxResults=50", KindAgile},
		{"POST", "/rest/agile/1.0/board/7/sprint", KindAgile},
	}
	for _, tc := range cases {
		if got := ClassifyRequest(tc.method, tc.path); got != tc.want {
			t.Errorf("ClassifyRequest(%s, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestBreakdownNoteTakeResets(t *testing.T) {
	var b Breakdown
	b.Note(http.MethodGet, "/rest/api/3/issue/A-1/comment", 1500*time.Millisecond)
	b.Note(http.MethodGet, "/rest/api/3/issue/A-2/comment", 500*time.Millisecond)
	b.Note(http.MethodPost, "/rest/api/3/search/jql", 0) // zero duration still counts
	b.NoteSleep(100 * time.Millisecond)
	b.NoteSleep(-time.Second) // ignored, like Meter.NoteWait

	snap := b.Take()
	if len(snap.Kinds) != 2 {
		t.Fatalf("kinds = %+v, want search-pages then issue-comment (canonical order)", snap.Kinds)
	}
	if snap.Kinds[0].Kind != KindSearchPages || snap.Kinds[0].Count != 1 || snap.Kinds[0].WallMS != 0 {
		t.Errorf("search-pages row = %+v", snap.Kinds[0])
	}
	if snap.Kinds[1].Kind != KindIssueComment || snap.Kinds[1].Count != 2 || snap.Kinds[1].WallMS != 2000 {
		t.Errorf("issue-comment row = %+v", snap.Kinds[1])
	}
	if snap.Total != 3 || snap.WallMS != 2000 {
		t.Errorf("total = %d wall = %dms, want 3 / 2000ms", snap.Total, snap.WallMS)
	}
	if snap.SleepMS != 100 {
		t.Errorf("sleep = %dms, want 100", snap.SleepMS)
	}

	// Take resets: the second pass must not inherit the first's requests.
	again := b.Take()
	if again.Total != 0 || again.SleepMS != 0 || len(again.Kinds) != 0 {
		t.Errorf("second take = %+v, want empty", again)
	}
}

// TestDoRawRecordsBreakdownAndHeaders drives the transport end to end: the
// per-attempt wall lands in the right kind bucket (a slow handler makes the
// measurement assertable), and DoRawWithHeaders returns the origin's headers.
func TestDoRawRecordsBreakdownAndHeaders(t *testing.T) {
	var b Breakdown
	cfg, _ := testCfg(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A real pause so the wall assertion cannot race a sub-millisecond
		// localhost round trip; sleeps overshoot, never undershoot.
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("X-RateLimit-Remaining", "5")
		w.WriteHeader(http.StatusOK)
	}))
	cfg.Breakdown = &b

	if _, _, err := do(t, cfg, http.MethodGet, "/rest/api/3/issue/K-9/comment?startAt=0", nil, false, false); err != nil {
		t.Fatal(err)
	}
	status, hdr, _, err := DoRawWithHeaders(context.Background(), cfg, http.MethodGet, "/rest/api/3/issue/K-9/comment?startAt=100", nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := hdr.Get("X-RateLimit-Remaining"); got != "5" {
		t.Errorf("header X-RateLimit-Remaining = %q, want 5", got)
	}

	snap := b.Take()
	if snap.Total != 2 {
		t.Fatalf("total = %d, want 2", snap.Total)
	}
	row := snap.Kinds[0]
	if row.Kind != KindIssueComment || row.Count != 2 {
		t.Fatalf("row = %+v, want 2 issue-comment", row)
	}
	if row.WallMS < 4 {
		t.Errorf("issue-comment wall = %dms, want the round trips counted", row.WallMS)
	}
}
