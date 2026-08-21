package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

// The fixture uses the fictional project key from data-model.md's examples and
// status ids only — no site's real ids appear anywhere (Constitution Article 7).
var fixtureCategories = map[string]string{"1": "new", "3": "inprogress", "5": "done"}

// fixtureNow is frozen so ago() is comparable across calls: a fixture built on
// one side of a second boundary must still equal the value asserted on the other.
var fixtureNow = time.Now().UTC().Truncate(time.Second)

func ago(days int) string {
	return fixtureNow.AddDate(0, 0, -days).Format("2006-01-02T15:04:05Z")
}

func fixture() Batch {
	return Batch{
		Categories: fixtureCategories,
		Priorities: []string{"Highest", "High", "Medium", "Low"},
		Records: []IssueRecord{
			{
				Item: Item{
					ID: "jira:10001", SourceID: "jira", Kind: "issue", ExternalID: "10001",
					Key: "NMB-1", Title: "Duplicate charges on card retry",
					BodyText: "The idempotency key is dropped when the gateway times out.",
					Author:   "Reporter One", AuthorID: "acc-r1",
					URL:       "https://example.invalid/browse/NMB-1",
					CreatedAt: ago(60), UpdatedAt: ago(2),
				},
				Issue: Issue{
					ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
					Status: "Done", StatusID: "5", StatusCategory: "done",
					Priority: "High", Assignee: "Ada Lovelace", AssigneeID: "acc-ada",
					AssigneeEmail: "ada@example.invalid", Reporter: "Reporter One", ReporterID: "acc-r1",
					ReporterEmail: "reporter.one@example.invalid",
					Labels:        []string{"payments", "regression"}, FixVersions: []string{"2026.8.0"},
					Resolution:     "Fixed",
					DescriptionADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
					Custom:         map[string]any{"severity": "S2"},
					Raw:            json.RawMessage(`{"id":"10001"}`),
				},
				Comments: []Comment{{
					ID: "jira:c-1", ExternalID: "c-1", Author: "Ada Lovelace", AuthorID: "acc-ada",
					BodyADF:   json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
					BodyText:  "Reproduced by forcing a retry against the sandbox gateway.",
					CreatedAt: ago(25), UpdatedAt: ago(25),
				}},
				Attachments: []Attachment{{
					ID: "jira:a-1", ExternalID: "10021", Filename: "trace.har",
					MimeType: "application/json", Size: 4096, Author: "Ada Lovelace", CreatedAt: ago(25),
				}},
				Changelog: []ChangeEntry{
					{ID: "jira:h-1", At: ago(40), Author: "Ada Lovelace", Field: "status", FromValue: "To Do", FromID: "1", ToValue: "In Progress", ToID: "3"},
					{ID: "jira:h-2", At: ago(30), Author: "Ada Lovelace", Field: "status", FromValue: "In Progress", FromID: "3", ToValue: "Done", ToID: "5"},
					{ID: "jira:h-3", At: ago(20), Author: "Reporter One", Field: "status", FromValue: "Done", FromID: "5", ToValue: "In Progress", ToID: "3"},
					{ID: "jira:h-4", At: ago(2), Author: "Ada Lovelace", Field: "status", FromValue: "In Progress", FromID: "3", ToValue: "Done", ToID: "5"},
					{ID: "jira:h-5", At: ago(41), Author: "Reporter One", Field: "assignee", ToValue: "Ada Lovelace", ToID: "acc-ada"},
				},
				Links: []Link{
					{Type: "Blocks", Direction: "outward", TargetKey: "NMB-2"},
					{Type: "Relates", Direction: "outward", TargetKey: "OUT-9"},
				},
			},
			{
				Item: Item{
					ID: "jira:10002", SourceID: "jira", Kind: "issue", ExternalID: "10002",
					Key: "NMB-2", Title: "Gateway timeout budget is too generous",
					BodyText: "Cut the budget to 5s.", Author: "Reporter One", AuthorID: "acc-r1",
					CreatedAt: ago(30), UpdatedAt: ago(5),
				},
				Issue: Issue{
					ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10002",
					Status: "In Progress", StatusID: "3", StatusCategory: "inprogress",
					Priority: "Medium", Assignee: "Ada Lovelace", AssigneeID: "acc-ada",
					Reporter: "Reporter One", ReporterID: "acc-r1",
				},
				Changelog: []ChangeEntry{
					{ID: "jira:h-6", At: ago(5), Field: "status", FromValue: "To Do", FromID: "1", ToValue: "In Progress", ToID: "3"},
				},
			},
			{
				Item: Item{
					ID: "jira:10003", SourceID: "jira", Kind: "issue", ExternalID: "10003",
					Key: "NMB-3", Title: "Refund webhook arrives twice",
					BodyText:  "Second delivery has the same event id.",
					CreatedAt: ago(3), UpdatedAt: ago(3),
				},
				Issue: Issue{
					ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
		},
	}
}

func seed(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	n, err := db.UpsertIssues(context.Background(), fixture())
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("seeded %d issues, want 3", n)
	}
}

func TestUpsertAndReadBack(t *testing.T) {
	db := openTemp(t)
	seed(t, db)

	lites, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(lites) != 3 {
		t.Fatalf("%d issues, want 3", len(lites))
	}
	byKey := map[string]IssueLite{}
	for _, l := range lites {
		byKey[l.IssueKey] = l
	}

	one := byKey["NMB-1"]
	if one.ReopenCount != 1 {
		t.Errorf("reopen_count = %d, want 1", one.ReopenCount)
	}
	if one.ResolvedAt == nil || *one.ResolvedAt != ago(2) {
		t.Errorf("resolved_at = %v, want %s", one.ResolvedAt, ago(2))
	}
	if one.CommentCount != 1 {
		t.Errorf("comment_count = %d, want 1", one.CommentCount)
	}
	if one.PriorityRank != 2 {
		t.Errorf("priority_rank for High = %d, want 2", one.PriorityRank)
	}
	if len(one.Labels) != 2 || one.Labels[0] != "payments" {
		t.Errorf("labels = %v", one.Labels)
	}
	if one.ReporterEmail == nil || *one.ReporterEmail != "reporter.one@example.invalid" {
		t.Errorf("reporter_email = %v", one.ReporterEmail)
	}
	if one.ReporterID == nil || *one.ReporterID != "acc-r1" {
		t.Errorf("reporter_id = %v", one.ReporterID)
	}
	// Configured field aliases ride along so the server can spread them.
	if one.Custom["severity"] != "S2" {
		t.Errorf("custom = %v, want severity S2", one.Custom)
	}
	if three := byKey["NMB-3"]; len(three.Custom) != 0 {
		t.Errorf("empty custom decoded as %v", three.Custom)
	}
	if three := byKey["NMB-3"]; three.Assignee != nil {
		t.Errorf("unassigned issue has assignee %q", *three.Assignee)
	}
	if three := byKey["NMB-3"]; len(three.Labels) != 0 {
		t.Errorf("absent labels decoded as %v, want empty", three.Labels)
	}
}

func TestUpsertSprintColumns(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	b := fixture()
	id12 := int64(12)
	b.Records[0].Issue.SprintID = &id12
	b.Records[0].Issue.SprintName = "Sprint 41"
	b.Records[0].Issue.SprintState = "active"
	id13 := int64(13)
	b.Records[1].Issue.SprintID = &id13
	b.Records[1].Issue.SprintState = "closed"
	if _, err := db.UpsertIssues(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM issues WHERE sprint_id = 12`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("sprint_id = 12 matched %d rows, want 1", n)
	}
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM issues_full WHERE sprint_state = 'active'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("sprint_state = active matched %d rows, want 1", n)
	}
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]IssueLite{}
	for _, l := range lites {
		byKey[l.IssueKey] = l
	}
	one := byKey["NMB-1"]
	if one.SprintID == nil || *one.SprintID != 12 {
		t.Errorf("NMB-1 sprint_id = %v, want 12", one.SprintID)
	}
	if one.SprintState == nil || *one.SprintState != "active" {
		t.Errorf("NMB-1 sprint_state = %v, want active", one.SprintState)
	}
	three := byKey["NMB-3"]
	if three.SprintID != nil {
		t.Errorf("NMB-3 sprint_id = %v, want NULL", three.SprintID)
	}
}

func TestUpsertFixVersionIDs(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	b := fixture()
	b.Records[0].Issue.FixVersions = []string{"v2.5"}
	b.Records[0].Issue.FixVersionIDs = []string{"10012"}
	if _, err := db.UpsertIssues(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	var names, ids string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT fix_versions, fix_version_ids FROM issues WHERE key = 'NMB-1'`).Scan(&names, &ids); err != nil {
		t.Fatal(err)
	}
	if names != `["v2.5"]` || ids != `["10012"]` {
		t.Errorf("fix_versions=%s fix_version_ids=%s, want names and ids in the same order", names, ids)
	}
	var fullIDs string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT fix_version_ids FROM issues_full WHERE key = 'NMB-1'`).Scan(&fullIDs); err != nil {
		t.Fatalf("issues_full.fix_version_ids: %v", err)
	}
	if fullIDs != `["10012"]` {
		t.Errorf("issues_full.fix_version_ids = %s, want [\"10012\"]", fullIDs)
	}
}

func TestReplaceProjectVersionsUpsertAndPrune(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	first := []VersionRow{
		{ID: "10012", Name: "v2.5", Released: true, ReleaseDate: "2026-08-20"},
		{ID: "10013", Name: "v2.6", Archived: true},
	}
	if err := db.ReplaceProjectVersions(ctx, "NMB", first); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceProjectVersions(ctx, "NMB", first); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM versions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("versions after re-upsert = %d, want 2 (no duplicates)", n)
	}
	second := []VersionRow{
		{ID: "10012", Name: "v2.5", Released: true, ReleaseDate: "2026-08-20"},
	}
	if err := db.ReplaceProjectVersions(ctx, "NMB", second); err != nil {
		t.Fatal(err)
	}
	var gone int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM versions WHERE id = '10013'`).Scan(&gone); err != nil {
		t.Fatal(err)
	}
	if gone != 0 {
		t.Errorf("id 10013 still present after it left the catalog")
	}
	var keptName string
	if err := db.sql.QueryRowContext(ctx, `SELECT name FROM versions WHERE id = '10012'`).Scan(&keptName); err != nil {
		t.Fatal(err)
	}
	if keptName != "v2.5" {
		t.Errorf("kept name = %q", keptName)
	}
}

func TestDetailAssembly(t *testing.T) {
	db := openTemp(t)
	seed(t, db)

	d, err := db.Detail(context.Background(), "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Comments) != 1 || d.Comments[0].Author != "Ada Lovelace" {
		t.Errorf("comments = %+v", d.Comments)
	}
	// Flattened text is the client's fallback when ADF will not render.
	if len(d.Comments) == 1 && d.Comments[0].Body == "" {
		t.Error("comment body_text not exposed")
	}
	if len(d.Attachments) != 1 || d.Attachments[0].Filename != "trace.har" {
		t.Errorf("attachments = %+v", d.Attachments)
	}
	if len(d.History) != 5 {
		t.Errorf("history = %d rows, want 5", len(d.History))
	}
	// Status ids travel with the row so a caller can resolve a category without
	// matching a localized name.
	for _, h := range d.History {
		if h.Field == "status" && (h.FromID == "" && h.ToID == "") {
			t.Errorf("status history row lost its ids: %+v", h)
		}
	}
	if len(d.LinkedIssues) != 2 {
		t.Fatalf("links = %+v", d.LinkedIssues)
	}
	var mirrored, outside DetailLink
	for _, l := range d.LinkedIssues {
		if l.Key == "NMB-2" {
			mirrored = l
		}
		if l.Key == "OUT-9" {
			outside = l
		}
	}
	if mirrored.Summary == "" || mirrored.StatusCategory != "inprogress" {
		t.Errorf("link into the mirror not resolved: %+v", mirrored)
	}
	if outside.Summary != "" || outside.StatusCategory != "" {
		t.Errorf("link outside the mirror invented data: %+v", outside)
	}
	if len(d.DescriptionADF) == 0 {
		t.Error("description_adf missing")
	}

	if _, err := db.Detail(context.Background(), "NMB-404"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown key error = %v, want ErrNotFound", err)
	}
}

func TestSearchReachesCommentText(t *testing.T) {
	db := openTemp(t)
	seed(t, db)

	// "sandbox" appears only in a comment body.
	res, err := db.Search(context.Background(), "sandbox", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || res.Keys[0] != "NMB-1" {
		t.Errorf("comment search = %v, want [NMB-1]", res.Keys)
	}
	// Unparseable FTS input falls back to a phrase match instead of erroring.
	if _, err := db.Search(context.Background(), `retry AND`, 10); err != nil {
		t.Errorf("malformed query: %v", err)
	}
	if res, err := db.Search(context.Background(), "webhook", 10); err != nil || len(res.Keys) != 1 {
		t.Errorf("body search = %v (%v)", res.Keys, err)
	}
}

// seedKoreanSearch upserts generic Korean/English issues for FTS prefix tests.
// Titles use particles and conjugated verbs so whole-token MATCH fails without
// prefix rewrite (unicode61 treats 로그인이 as one token).
func seedKoreanSearch(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	b := Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			{
				Item: Item{
					ID: "jira:kr1", SourceID: "jira", Kind: "issue", ExternalID: "kr1",
					Key: "KR-1", Title: "로그인이 간헐적으로 실패합니다",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KR", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:kr2", SourceID: "jira", Kind: "issue", ExternalID: "kr2",
					Key: "KR-2", Title: "결제 오류",
					BodyText:  "결제 모듈이 응답하지 않습니다",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KR", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:en1", SourceID: "jira", Kind: "issue", ExternalID: "en1",
					Key: "EN-1", Title: "Payment retries fail",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "EN", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:kr3", SourceID: "jira", Kind: "issue", ExternalID: "kr3",
					Key: "KR-3", Title: "로그인 화면 깨짐",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KR", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
		},
	}
	if _, err := db.UpsertIssues(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

func containsKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}

func TestSearchKoreanAndPrefix(t *testing.T) {
	db := openTemp(t)
	seedKoreanSearch(t, db)

	// 1. Particle-attached noun: 로그인이 is one FTS token; bare "로그인" needs prefix.
	res, err := db.Search(context.Background(), "로그인", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KR-1") {
		t.Errorf("Search(%q) = %v, want KR-1 (particle form)", "로그인", res.Keys)
	}

	// 2. Conjugated verb in body: 응답하지 → bare "응답".
	res, err = db.Search(context.Background(), "응답", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KR-2") {
		t.Errorf("Search(%q) = %v, want KR-2 (conjugated form)", "응답", res.Keys)
	}

	// 3. English stem prefix: retries ← retri.
	res, err = db.Search(context.Background(), "retri", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "EN-1") {
		t.Errorf("Search(%q) = %v, want EN-1", "retri", res.Keys)
	}

	// 4. Quoted phrase passes through: match consecutive tokens only.
	res, err = db.Search(context.Background(), `"로그인 화면"`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KR-3") {
		t.Errorf("phrase Search = %v, want KR-3", res.Keys)
	}
	if containsKey(res.Keys, "KR-1") {
		t.Errorf("phrase Search matched KR-1 (particle title), keys=%v", res.Keys)
	}

	// 5. Two bare tokens → implicit AND via space-joined prefixes.
	res, err = db.Search(context.Background(), "로그인 실패", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KR-1") {
		t.Errorf("AND Search = %v, want KR-1", res.Keys)
	}
	if containsKey(res.Keys, "KR-3") {
		t.Errorf("AND Search matched KR-3 (login only), keys=%v", res.Keys)
	}
}

// TestUpsertPagesUnchangedDoesNotBumpVersion is FAIL-first for C4: a quiet
// wiki's overlap re-fetch must not move sync_state.version. A real body or
// comment change must still bump (the old "always rewrite" existed so
// comments-only edits were not dropped).
func TestUpsertPagesUnchangedDoesNotBumpVersion(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "confluence", Kind: "confluence"}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"same"}]}]}`)
	cm := Comment{
		ID: "confluence:c1", ExternalID: "c1", Author: "Lee", AuthorID: "acc-lee",
		BodyADF:   json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
		BodyText:  "first",
		CreatedAt: ago(2), UpdatedAt: ago(2),
	}
	rec := PageRecord{
		Item: Item{
			ID: "confluence:p1", SourceID: "confluence", Kind: "page", ExternalID: "p1",
			Key: "p1", Title: "Doc", BodyText: "same",
			Author: "Dana", AuthorID: "acc-dana",
			URL:       "https://x.example/wiki/spaces/ENG/pages/p1",
			CreatedAt: ago(3), UpdatedAt: ago(1),
		},
		Page:     Page{SpaceKey: "ENG", Version: 2, Status: "current", Labels: []string{"ops"}, BodyADF: adf},
		Comments: []Comment{cm},
	}
	if n, err := db.UpsertPages(ctx, []PageRecord{rec}); err != nil || n != 1 {
		t.Fatalf("seed UpsertPages n=%d err=%v", n, err)
	}
	before, err := db.SyncState(ctx, "confluence")
	if err != nil {
		t.Fatal(err)
	}

	n, err := db.UpsertPages(ctx, []PageRecord{rec})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("C4: unchanged re-upsert changed %d rows, want 0", n)
	}
	after, err := db.SyncState(ctx, "confluence")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version {
		t.Errorf("C4: version moved %d -> %d on unchanged page", before.Version, after.Version)
	}

	// Body change still bumps.
	changed := rec
	changed.Item.BodyText = "edited"
	changed.Page.BodyADF = json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"edited"}]}]}`)
	changed.Page.Version = 3
	if n, err := db.UpsertPages(ctx, []PageRecord{changed}); err != nil || n != 1 {
		t.Fatalf("body change n=%d err=%v, want 1", n, err)
	}
	bodyState, _ := db.SyncState(ctx, "confluence")
	if bodyState.Version <= after.Version {
		t.Errorf("C4 regression: body change did not bump version (%d)", bodyState.Version)
	}

	// Comment-only change still bumps.
	withComment := changed
	withComment.Comments = append(withComment.Comments, Comment{
		ID: "confluence:c2", ExternalID: "c2", Author: "Pat", AuthorID: "acc-pat",
		BodyText: "new comment", CreatedAt: ago(0), UpdatedAt: ago(0),
	})
	if n, err := db.UpsertPages(ctx, []PageRecord{withComment}); err != nil || n != 1 {
		t.Fatalf("comment change n=%d err=%v, want 1", n, err)
	}
	cmState, _ := db.SyncState(ctx, "confluence")
	if cmState.Version <= bodyState.Version {
		t.Errorf("C4 regression: comment change did not bump version (%d)", cmState.Version)
	}
}

func TestUnchangedUpsertIsANoOp(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	before, err := db.SyncState(context.Background(), "jira")
	if err != nil {
		t.Fatal(err)
	}
	var syncedBefore string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT synced_at FROM items WHERE key = 'NMB-1'`).Scan(&syncedBefore); err != nil {
		t.Fatal(err)
	}

	n, err := db.UpsertIssues(context.Background(), fixture())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("re-upserting unchanged issues touched %d rows, want 0", n)
	}
	after, err := db.SyncState(context.Background(), "jira")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version {
		t.Errorf("version moved %d -> %d with no upstream change", before.Version, after.Version)
	}
	var syncedAfter string
	if err := db.sql.QueryRowContext(context.Background(), `SELECT synced_at FROM items WHERE key = 'NMB-1'`).Scan(&syncedAfter); err != nil {
		t.Fatal(err)
	}
	if syncedAfter != syncedBefore {
		t.Errorf("synced_at rewritten for an unchanged row")
	}

	// Force is the repair path: a full sync must be able to rewrite everything.
	forced := fixture()
	forced.Force = true
	if n, err := db.UpsertIssues(context.Background(), forced); err != nil || n != 3 {
		t.Errorf("forced upsert wrote %d rows (%v), want 3", n, err)
	}
	if forcedState, _ := db.SyncState(context.Background(), "jira"); forcedState.Version <= before.Version {
		t.Errorf("version %d did not move after a forced rewrite", forcedState.Version)
	}
}

func TestChangedIssueReplacesChildrenAndDerivedFields(t *testing.T) {
	db := openTemp(t)
	seed(t, db)

	b := fixture()
	// The issue moves back out of done: reopen count grows, resolved_at clears.
	r := &b.Records[0]
	r.Item.UpdatedAt = ago(1)
	r.Issue.Status, r.Issue.StatusID, r.Issue.StatusCategory = "In Progress", "3", "inprogress"
	r.Issue.Resolution = ""
	r.Changelog = append(r.Changelog, ChangeEntry{
		ID: "jira:h-7", At: ago(1), Field: "status", FromValue: "Done", FromID: "5", ToValue: "In Progress", ToID: "3",
	})
	r.Comments = nil
	if n, err := db.UpsertIssues(context.Background(), b); err != nil || n != 1 {
		t.Fatalf("upsert changed=%d err=%v, want 1", n, err)
	}

	var reopen, comments int
	var resolved, reopened *string
	if err := db.sql.QueryRowContext(context.Background(), `
		SELECT reopen_count, comment_count, resolved_at, reopened_at FROM issues WHERE key = 'NMB-1'`).
		Scan(&reopen, &comments, &resolved, &reopened); err != nil {
		t.Fatal(err)
	}
	if reopen != 2 {
		t.Errorf("reopen_count = %d, want 2", reopen)
	}
	if resolved != nil {
		t.Errorf("resolved_at = %q for an issue that is no longer done", *resolved)
	}
	if reopened == nil || *reopened != ago(1) {
		t.Errorf("reopened_at = %v, want %s", reopened, ago(1))
	}
	if comments != 0 {
		t.Errorf("comment_count = %d after the comment was removed upstream", comments)
	}
	var rows int
	if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM comments WHERE item_id = 'jira:10001'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d comment rows survived a shorter upstream list", rows)
	}
	// The FTS row is rebuilt, so the removed comment is no longer searchable.
	if res, err := db.Search(context.Background(), "sandbox", 10); err != nil || len(res.Keys) != 0 {
		t.Errorf("search still finds removed comment text: %v (%v)", res.Keys, err)
	}
}

func TestDeleteItemsCascadesAndTombstones(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	cursor := Now()

	n, err := db.DeleteItems(context.Background(), "jira", []string{"NMB-2", "NMB-nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted %d, want 1", n)
	}
	for _, table := range []string{"issues", "comments", "attachments", "changelog", "links"} {
		var rows int
		if err := db.sql.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM `+table+` WHERE item_id = 'jira:10002'`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Errorf("%s kept %d rows for a deleted item", table, rows)
		}
	}
	if res, err := db.Search(context.Background(), "budget", 10); err != nil || len(res.Keys) != 0 {
		t.Errorf("deleted item still in the full-text index: %v (%v)", res.Keys, err)
	}
	deleted, err := db.DeletedKeysSince(context.Background(), cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "NMB-2" {
		t.Errorf("deleted keys = %v, want [NMB-2]", deleted)
	}

	// An issue that comes back must stop being reported as deleted.
	b := fixture()
	b.Records = b.Records[1:2]
	if _, err := db.UpsertIssues(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if deleted, err := db.DeletedKeysSince(context.Background(), cursor); err != nil || len(deleted) != 0 {
		t.Errorf("resurrected issue still tombstoned: %v (%v)", deleted, err)
	}
}

func TestDeltaWindow(t *testing.T) {
	db := openTemp(t)
	seed(t, db)

	// The cursor bound is inclusive, so it has to be strictly later than the
	// seed writes for "nothing changed" to mean anything.
	time.Sleep(2 * time.Millisecond)
	cursor := Now()
	if rows, err := db.IssueLitesSince(context.Background(), cursor); err != nil {
		t.Fatal(err)
	} else if len(rows) != 0 {
		t.Errorf("%d rows changed after the cursor with no writes", len(rows))
	}

	b := fixture()
	b.Records = b.Records[2:]
	b.Records[0].Item.UpdatedAt = ago(0)
	if _, err := db.UpsertIssues(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	rows, err := db.IssueLitesSince(context.Background(), cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].IssueKey != "NMB-3" {
		t.Errorf("delta = %+v, want just NMB-3", rows)
	}
}

// GDK-526: sync_state.schema_version is only rewritten when a migration
// actually runs (or a sync records bookkeeping). After later Open-time
// migrations the column lags PRAGMA user_version. SyncState must still
// report the live level so status/MCP/doctor cannot disagree.
func TestSyncStateSchemaVersionFollowsPRAGMA(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	ctx := context.Background()
	const watermark = "2026-01-15T12:00:00.000Z"
	if err := db.RecordSync(ctx, "jira", SyncResult{Watermark: watermark}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(ctx, "jira", SyncResult{Err: fmt.Errorf("planted last_error")}); err != nil {
		t.Fatal(err)
	}
	live := db.SchemaVersion()
	stale := live - 5
	if stale < 1 {
		t.Fatalf("live schema %d is too low to plant a stale value", live)
	}
	const version int64 = 77
	if _, err := db.sql.Exec(`UPDATE sync_state SET schema_version = ?, version = ? WHERE source_id = ?`, stale, version, "jira"); err != nil {
		t.Fatal(err)
	}
	var col int
	if err := db.sql.QueryRow(`SELECT schema_version FROM sync_state WHERE source_id = ?`, "jira").Scan(&col); err != nil {
		t.Fatal(err)
	}
	if col != stale {
		t.Fatalf("planted schema_version = %d, want %d", col, stale)
	}

	st, err := db.SyncState(ctx, "jira")
	if err != nil {
		t.Fatal(err)
	}
	if st.SchemaVersion != live {
		t.Fatalf("SyncState.SchemaVersion = %d, want live PRAGMA %d (row still %d)", st.SchemaVersion, live, col)
	}
	if st.SchemaVersionRow != stale {
		t.Errorf("SchemaVersionRow = %d, want planted %d", st.SchemaVersionRow, stale)
	}
	if st.Watermark != watermark {
		t.Errorf("watermark = %q, want %q (must still come from the row)", st.Watermark, watermark)
	}
	if st.LastError == nil || *st.LastError != "planted last_error" {
		t.Errorf("last_error = %v, want planted last_error", st.LastError)
	}
	if st.Version != version {
		t.Errorf("version = %d, want %d (must still come from the row)", st.Version, version)
	}
	if st.SyncedAt == nil || *st.SyncedAt == "" {
		t.Error("synced_at empty; successful RecordSync should have stamped it")
	}
	if err := db.sql.QueryRow(`SELECT schema_version FROM sync_state WHERE source_id = ?`, "jira").Scan(&col); err != nil {
		t.Fatal(err)
	}
	if col != stale {
		t.Fatalf("SyncState rewrote the row to %d, want it left at %d", col, stale)
	}
}

func TestReplaceDevLinksBumpsVersion(t *testing.T) {
	db := openTemp(t)
	seed(t, db)
	before, err := db.SyncState(context.Background(), "jira")
	if err != nil {
		t.Fatal(err)
	}
	if before.Version == 0 {
		t.Fatal("seed should have advanced sync_state.version")
	}
	if err := db.ReplaceDevLinks(context.Background(), "NMB-1", []DevLink{{
		Kind: "pullrequest", URL: "https://github.com/example/app/pull/1",
		Title: "fix", Status: "open", UpdatedAt: "2026-08-21T00:00:00Z",
	}}); err != nil {
		t.Fatal(err)
	}
	after, err := db.SyncState(context.Background(), "jira")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version <= before.Version {
		t.Fatalf("ReplaceDevLinks left version at %d, want it to advance", after.Version)
	}
}

func TestRecordSyncWatermarkOnlyMovesForward(t *testing.T) {
	db := openTemp(t)
	seed(t, db)

	if err := db.RecordSync(context.Background(), "jira", SyncResult{Watermark: "2026-08-01T00:00:00Z", FullSync: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSync(context.Background(), "jira", SyncResult{Watermark: "2026-07-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	st, err := db.SyncState(context.Background(), "jira")
	if err != nil {
		t.Fatal(err)
	}
	if st.Watermark != "2026-08-01T00:00:00Z" {
		t.Errorf("watermark regressed to %q", st.Watermark)
	}
	if st.LastFullSyncAt == nil {
		t.Error("last_full_sync_at not stamped")
	}
	if st.LastError != nil {
		t.Errorf("last_error = %q after a clean run", *st.LastError)
	}
	if st.SyncedAt == nil {
		t.Error("synced_at not stamped after a clean run")
	}
	fresh := *st.SyncedAt

	// A failure records itself and leaves the mirror readable.
	if err := db.RecordSync(context.Background(), "jira", SyncResult{Err: fmt.Errorf("401 from the source")}); err != nil {
		t.Fatal(err)
	}
	st, _ = db.SyncState(context.Background(), "jira")
	if st.LastError == nil || *st.LastError != "401 from the source" {
		t.Errorf("last_error = %v", st.LastError)
	}
	if rows, err := db.IssueLites(context.Background()); err != nil || len(rows) != 3 {
		t.Errorf("mirror unreadable after a failed sync: %d rows (%v)", len(rows), err)
	}
	if st.Watermark != "2026-08-01T00:00:00Z" {
		t.Errorf("failed run moved the watermark to %q", st.Watermark)
	}
	if st.SyncedAt == nil || *st.SyncedAt != fresh {
		t.Errorf("synced_at = %v after a failed run, want the last good one (%s)", st.SyncedAt, fresh)
	}
}

func TestPersonalState(t *testing.T) {
	db := openTemp(t)

	if err := db.PutSavedView(context.Background(), SavedView{ID: "v1", Name: "Mine", Config: json.RawMessage(`{"assignee":"me"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.PutSavedView(context.Background(), SavedView{ID: "v1", Name: "Mine, renamed", Config: json.RawMessage(`{"assignee":"me"}`)}); err != nil {
		t.Fatal(err)
	}
	views, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Name != "Mine, renamed" || views[0].CreatedAt == "" {
		t.Fatalf("saved views = %+v", views)
	}
	if err := db.PutSavedView(context.Background(), SavedView{ID: "", Name: "no id"}); err == nil {
		t.Error("accepted a saved view without an id")
	}
	if err := db.DeleteSavedView(context.Background(), "v1"); err != nil {
		t.Fatal(err)
	}
	if views, _ := db.SavedViews(context.Background()); len(views) != 0 {
		t.Errorf("view survived deletion: %+v", views)
	}

	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSourceQueries(context.Background(), "jira", []SourceQuery{
		{ID: "jira:1", ExternalID: "1", Name: "Starred", QueryText: "project = NMA", Config: json.RawMessage(`{}`), Favourite: true},
		{ID: "jira:2", ExternalID: "2", Name: "Alpha", QueryText: "type = Bug", Config: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	qs, err := db.SourceQueries(context.Background(), "jira")
	if err != nil || len(qs) != 2 || qs[0].Name != "Starred" || qs[1].Name != "Alpha" {
		t.Fatalf("source_queries = %+v err=%v", qs, err)
	}
	if err := db.ReplaceSourceQueries(context.Background(), "jira", nil); err != nil {
		t.Fatal(err)
	}
	if qs, _ := db.SourceQueries(context.Background(), "jira"); len(qs) != 0 {
		t.Fatalf("replace empty left %+v", qs)
	}

	for _, c := range []struct {
		set  func(context.Context, string, bool) error
		get  func(context.Context) ([]string, error)
		name string
	}{
		{db.SetWatch, db.Watches, "watches"},
		{db.SetFavorite, db.Favorites, "favorites"},
	} {
		if err := c.set(context.Background(), "NMB-1", true); err != nil {
			t.Fatal(err)
		}
		if err := c.set(context.Background(), "NMB-1", true); err != nil {
			t.Fatal(err)
		}
		if keys, err := c.get(context.Background()); err != nil || len(keys) != 1 {
			t.Errorf("%s = %v (%v)", c.name, keys, err)
		}
		if err := c.set(context.Background(), "NMB-1", false); err != nil {
			t.Fatal(err)
		}
		if keys, _ := c.get(context.Background()); len(keys) != 0 {
			t.Errorf("%s kept %v after removal", c.name, keys)
		}
	}
}

func TestUpsertPreservesDevLinksWhenNotValid(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	rec := IssueRecord{
		Item: Item{
			ID: "jira:dl1", SourceID: "jira", Kind: "issue", ExternalID: "dl1",
			Key: "NMB-DL", Title: "has a pr", CreatedAt: ago(2), UpdatedAt: ago(2),
		},
		Issue: Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "1",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
		},
		DevLinks: []DevLink{{
			Kind: "pullrequest", URL: "https://github.com/o/r/pull/1", Title: "pr",
		}},
		DevLinksValid: true,
	}
	if _, err := db.UpsertIssues(ctx, Batch{Categories: fixtureCategories, Records: []IssueRecord{rec}}); err != nil {
		t.Fatal(err)
	}
	rec.DevLinks = nil
	rec.DevLinksValid = false
	rec.Item.Title = "rewritten"
	if _, err := db.UpsertIssues(ctx, Batch{Force: true, Categories: fixtureCategories, Records: []IssueRecord{rec}}); err != nil {
		t.Fatal(err)
	}
	d, err := db.Detail(ctx, "NMB-DL")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.DevLinks) != 1 || d.DevLinks[0].URL != "https://github.com/o/r/pull/1" {
		t.Fatalf("dev_links after invalid rewrite = %+v, want the seeded row", d.DevLinks)
	}
}

func TestUpsertDrainsDevLinksWhenValidEmpty(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	rec := IssueRecord{
		Item: Item{
			ID: "jira:dl2", SourceID: "jira", Kind: "issue", ExternalID: "dl2",
			Key: "NMB-DE", Title: "has a pr", CreatedAt: ago(2), UpdatedAt: ago(2),
		},
		Issue: Issue{
			ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "1",
			Status: "To Do", StatusID: "1", StatusCategory: "new",
		},
		DevLinks: []DevLink{{
			Kind: "pullrequest", URL: "https://github.com/o/r/pull/2", Title: "pr",
		}},
		DevLinksValid: true,
	}
	if _, err := db.UpsertIssues(ctx, Batch{Categories: fixtureCategories, Records: []IssueRecord{rec}}); err != nil {
		t.Fatal(err)
	}
	rec.DevLinks = nil
	rec.DevLinksValid = true
	if _, err := db.UpsertIssues(ctx, Batch{Force: true, Categories: fixtureCategories, Records: []IssueRecord{rec}}); err != nil {
		t.Fatal(err)
	}
	d, err := db.Detail(ctx, "NMB-DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.DevLinks) != 0 {
		t.Fatalf("successful empty must drain, still have %+v", d.DevLinks)
	}
}
