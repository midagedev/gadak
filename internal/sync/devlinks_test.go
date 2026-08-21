package sync

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

func TestStandaloneSyncFetchesDevLinksWithFlagOff(t *testing.T) {
	// GDK-536: standalone (issuetap) always fetches, even when DevStatus is off.
	site := newSite(t, "en")
	site.devPRs = map[string][]jira.DevPR{
		"10001": {{ID: "pr-9", URL: "https://github.com/o/r/pull/9", Name: "from-origin", Status: jira.DevPROpen}},
	}
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.Kind = config.KindStandalone
	cfg.DevStatus = false

	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	if site.devStatusHits == 0 {
		t.Fatal("standalone sync with DevStatus=false never fetched dev-status")
	}
	d, err := db.DB.Detail(context.Background(), "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.DevLinks) != 1 || d.DevLinks[0].URL != "https://github.com/o/r/pull/9" {
		t.Fatalf("dev_links = %+v, want the origin PR", d.DevLinks)
	}
}

func TestDevStatusFetchErrorPreservesRows(t *testing.T) {
	site := newSite(t, "en")
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.DevStatus = true
	ctx := context.Background()

	if err := db.UpsertSource(ctx, store.Source{ID: SourceID, Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	seed := store.IssueRecord{
		Item: store.Item{
			// standalone namespaces item ids (itemNS) — seed the id sync
			// will actually rewrite, or the test proves nothing about the
			// rewrite path.
			ID: "jira:10001", SourceID: SourceID, Kind: "issue", ExternalID: "10001",
			Key: "NMB-1", Title: "seeded", CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
		},
		DevLinks: &store.DevLinksUpdate{Links: []store.DevLink{{
			Kind: "pullrequest", URL: "https://github.com/o/r/pull/keep", Title: "keep",
		}}},
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"}, Records: []store.IssueRecord{seed}}); err != nil {
		t.Fatal(err)
	}

	site.devStatusFail = true
	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	d, err := db.DB.Detail(ctx, "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range d.DevLinks {
		if l.URL == "https://github.com/o/r/pull/keep" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fetch error drained seeded link: %+v", d.DevLinks)
	}
}

func TestDevStatusSuccessfulEmptyDrains(t *testing.T) {
	site := newSite(t, "en")
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.DevStatus = true
	ctx := context.Background()

	if err := db.UpsertSource(ctx, store.Source{ID: SourceID, Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	seed := store.IssueRecord{
		Item: store.Item{
			// standalone namespaces item ids (itemNS) — seed the id sync
			// will actually rewrite, or the test proves nothing about the
			// rewrite path.
			ID: "jira:10001", SourceID: SourceID, Kind: "issue", ExternalID: "10001",
			Key: "NMB-1", Title: "seeded", CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
		},
		DevLinks: &store.DevLinksUpdate{Links: []store.DevLink{{
			Kind: "pullrequest", URL: "https://github.com/o/r/pull/gone", Title: "gone",
		}}},
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"}, Records: []store.IssueRecord{seed}}); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	d, err := db.DB.Detail(ctx, "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.DevLinks) != 0 {
		t.Fatalf("successful empty must drain, still have %+v", d.DevLinks)
	}
}

// TestSyncStoresDevLinkAuthorActorBranch pins the gadak half of GDK-589:
// a dev-status answer carrying author{name} / source{branch} /
// actor{accountId, displayName} lands in the v33 dev_links columns via the
// same DevLinksFromPRs path `gadak dev link`'s refresh uses. The axes stay
// separate columns — author is the PR's human, actor the linking agent.
func TestSyncStoresDevLinkAuthorActorBranch(t *testing.T) {
	site := newSite(t, "en")
	pr := jira.DevPR{
		ID: "pr-9", URL: "https://github.com/o/r/pull/9", Name: "from-origin",
		Status: jira.DevPROpen,
		Author: jira.DevPRAuthor{Name: "midagedev"},
		Source: jira.DevPRSource{Branch: "gdk-589-dev-link-actor"},
		Actor:  jira.DevPRActor{AccountID: "claude:354bff2b", DisplayName: "Claude (build 1)"},
	}
	site.devPRs = map[string][]jira.DevPR{"10001": {pr}}
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.Kind = config.KindStandalone

	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	d, err := db.DB.Detail(context.Background(), "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.DevLinks) != 1 {
		t.Fatalf("dev_links = %+v, want the origin PR", d.DevLinks)
	}
	l := d.DevLinks[0]
	if l.Author != "midagedev" || l.Branch != "gdk-589-dev-link-actor" {
		t.Errorf("author/branch = %q/%q, want midagedev / gdk-589-dev-link-actor", l.Author, l.Branch)
	}
	if l.Actor != "claude:354bff2b" || l.ActorName != "Claude (build 1)" {
		t.Errorf("actor/name = %q/%q, want claude:354bff2b / Claude (build 1)", l.Actor, l.ActorName)
	}

	// The plain-SQL read an agent would run (the spec's bar: `select * from
	// dev_links` must see the columns). QueryRow is the `gadak sql` surface.
	var author, actor, actorName, branch string
	if err := db.DB.QueryRow(`SELECT author, actor, actor_name, branch FROM dev_links WHERE url = ?`, pr.URL).
		Scan(&author, &actor, &actorName, &branch); err != nil {
		t.Fatalf("raw dev_links read: %v", err)
	}
	if author != "midagedev" || actor != "claude:354bff2b" || actorName != "Claude (build 1)" || branch != "gdk-589-dev-link-actor" {
		t.Errorf("dev_links row = %q/%q/%q/%q", author, actor, actorName, branch)
	}
}

// TestSyncKeepsDeploymentRowsThroughPRRewrite pins the gadak half of
// GDK-592: a dev-status answer enumerates pull requests only (its count is
// the summary's pullrequest count — the deployment/build detail vocabulary
// was never captured), so a sync rewrite replaces the PR rows and leaves
// the deployment/build rows `gadak dev deploy`/`dev build` wrote. A PR
// drain (count 0) still removes PRs.
func TestSyncKeepsDeploymentRowsThroughPRRewrite(t *testing.T) {
	site := newSite(t, "en")
	site.devPRs = map[string][]jira.DevPR{
		"10001": {{ID: "pr-9", URL: "https://github.com/o/r/pull/9", Name: "from-origin", Status: jira.DevPRMerged}},
	}
	client := site.start()
	db := newMirror(t)
	cfg := testConfig()
	cfg.Kind = config.KindStandalone
	ctx := context.Background()

	if err := db.UpsertSource(ctx, store.Source{ID: SourceID, Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	seed := store.IssueRecord{
		Item: store.Item{
			// standalone namespaces item ids (itemNS) — seed the id sync
			// will actually rewrite, or the test proves nothing about the
			// rewrite path.
			ID: "standalone-jira:10001", SourceID: SourceID, Kind: "issue", ExternalID: "10001",
			Key: "NMB-1", Title: "seeded", CreatedAt: "2026-07-01T00:00:00.000Z",
			UpdatedAt: "2026-08-01T00:00:00.000Z",
		},
		Issue: store.Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
		},
		// What `gadak dev deploy` / `dev build` would have left (via
		// ReplaceDevLinks): one env-keyed deployment, one url-keyed build.
		DevLinks: &store.DevLinksUpdate{Links: []store.DevLink{
			{Kind: "deployment", ExternalID: "environment:production", URL: "environment:production",
				Environment: "production", Status: "successful"},
			{Kind: "build", ExternalID: "592", URL: "https://ci.example/gadak/build/592",
				Status: "failed"},
		}},
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{Categories: map[string]string{"1": "new", "3": "inprogress", "5": "done"}, Records: []store.IssueRecord{seed}}); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	kinds := devLinkKinds(t, db, ctx)
	if kinds["pullrequest"] != 1 || kinds["deployment"] != 1 || kinds["build"] != 1 {
		t.Fatalf("after PR rewrite kinds = %v, want one of each — the PR answer must not drain the other kinds", kinds)
	}

	// PR count drops to 0: the PR row drains, the other kinds stay. The
	// issue's updated is bumped too — dev_links ride the issue rewrite, so
	// an unchanged issue would (correctly) keep every row as-is.
	site.devPRs = map[string][]jira.DevPR{}
	bumpNMB1Updated(t, site)
	if _, err := Run(ctx, cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	kinds = devLinkKinds(t, db, ctx)
	if kinds["pullrequest"] != 0 || kinds["deployment"] != 1 || kinds["build"] != 1 {
		t.Fatalf("after PR drain kinds = %v, want PRs gone, deployment/build kept", kinds)
	}
}

// bumpNMB1Updated rewrites the fake site's NMB-1 payload with a newer
// updated stamp so the next pass actually rewrites the issue (and with it
// the dev_links answer).
func bumpNMB1Updated(t *testing.T, site *fakeSite) {
	t.Helper()
	for i, raw := range site.issues {
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if doc["key"] != "NMB-1" {
			continue
		}
		doc["fields"].(map[string]any)["updated"] = "2026-08-03T10:00:00.000+0900"
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		site.issues[i] = out
		return
	}
	t.Fatal("fake site has no NMB-1 to bump")
}

func devLinkKinds(t *testing.T, db *mirror, ctx context.Context) map[string]int {
	t.Helper()
	d, err := db.DB.Detail(ctx, "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, l := range d.DevLinks {
		kinds[l.Kind]++
	}
	return kinds
}
