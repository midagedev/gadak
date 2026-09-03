package migrate

import (
	"context"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// seedMirror builds a throwaway mirror carrying the three spike rules'
// hazards: priorities encountered lowest-first, links stored single-sided
// per item, and a changelog referencing a status no issue currently sits in.
func seedMirror(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://x.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	_, err = db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress", "10001": "done"},
		Records: []store.IssueRecord{
			{
				Item: store.Item{ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1", Key: "T-1",
					Title: "alpha", BodyText: "body alpha",
					CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "T", IssueType: "Task", IssueTypeID: "10001",
					Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
					Priority: "Lowest", PriorityID: "5",
					AssigneeID: "acc-a", ReporterID: "acc-gone",
					DescriptionADF: []byte(`{"type":"doc","content":[{"type":"codeBlock","content":[]},{"type":"mediaSingle","content":[{"type":"media"}]}]}`)},
				Comments: []store.Comment{{ID: "jira:c1", ExternalID: "c1", AuthorID: "acc-a",
					BodyText: "first", CreatedAt: "2026-01-01T01:00:00.000Z"}},
				Attachments: []store.Attachment{{ID: "jira:a1", ExternalID: "901", Filename: "notes.txt",
					MimeType: "text/plain", Size: 5, AuthorID: "acc-a", CreatedAt: "2026-01-01T02:00:00.000Z"}},
				Changelog: []store.ChangeEntry{{ID: "jira:h1", At: "2026-01-01T03:00:00.000Z", AuthorID: "acc-a",
					Field: "status", FromID: "1", FromValue: "할 일", ToID: "3", ToValue: "진행 중"}},
				Links: []store.Link{
					{Type: "blocks", Direction: "outward", TargetKey: "T-2"},
					{Type: "relates to", Direction: "outward", TargetKey: "X-9"},
				},
				// acc-a enters the account catalog; acc-gone deliberately
				// does not (the departed-member case).
				Users: []store.UserAccount{{AccountID: "acc-a", Name: "Alice", Email: "a@example.com"}},
			},
			{
				Item: store.Item{ID: "jira:2", SourceID: "jira", Kind: "issue", ExternalID: "2", Key: "T-2",
					Title: "beta", CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-03T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "T", IssueType: "Task", IssueTypeID: "10001",
					Status: "완료", StatusID: "10001", StatusCategory: "done",
					Priority: "Highest", PriorityID: "1"},
				Links: []store.Link{{Type: "blocks", Direction: "inward", TargetKey: "T-1"}},
			},
			{
				Item: store.Item{ID: "jira:3", SourceID: "jira", Kind: "issue", ExternalID: "3", Key: "T-3",
					Title: "gamma", CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-04T00:00:00.000Z"},
				Issue: store.Issue{ProjectKey: "T", IssueType: "Task", IssueTypeID: "10001",
					Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
					Priority: "Custom", PriorityID: "10"},
			},
		},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func build(t *testing.T) (*Doc, *Stats) {
	t.Helper()
	sqlDB, err := store.OpenReadOnly(seedMirror(t))
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	doc, st, err := Build(context.Background(), sqlDB, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return doc, st
}

// Rule 1: catalog order is priority_rank — numeric id order, never the
// encounter order (T-1 carries Lowest and is read first).
func TestPrioritiesEmitInIDOrder(t *testing.T) {
	doc, _ := build(t)
	var got []string
	for _, p := range doc.Priorities {
		got = append(got, p.ID)
	}
	want := []string{"1", "5", "10"}
	if len(got) != len(want) {
		t.Fatalf("priorities %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("priorities %v, want %v", got, want)
		}
	}
}

// Rule 2: each issue's own mirror rows emit verbatim, both directions — the
// fixture load is single-sided, so a one-sided emit would leave the pair
// missing on one end. Out-of-set targets drop and are counted.
func TestLinksEmitBothSidesVerbatim(t *testing.T) {
	doc, st := build(t)
	byKey := map[string]Issue{}
	for _, is := range doc.Issues {
		byKey[is.Key] = is
	}
	a, b := byKey["T-1"], byKey["T-2"]
	if len(a.Links) != 1 || a.Links[0].Outward != "T-2" || a.Links[0].Type != "blocks" {
		t.Fatalf("T-1 links %+v", a.Links)
	}
	if len(b.Links) != 1 || b.Links[0].Inward != "T-1" {
		t.Fatalf("T-2 links %+v", b.Links)
	}
	if st.DroppedLinks != 1 {
		t.Fatalf("dropped links %d, want 1 (X-9 is outside the set)", st.DroppedLinks)
	}
}

// Rule 3: the status catalog includes ids only the changelog references
// ("1" here), with the display name recovered from the changelog values and
// the mirror category mapped to the fixture vocabulary
// (inprogress → indeterminate).
func TestHistoryStatusIDsEnterCatalog(t *testing.T) {
	doc, st := build(t)
	got := map[string]Status{}
	for _, s := range doc.Statuses {
		got[s.ID] = s
	}
	if s, ok := got["1"]; !ok || s.Name != "할 일" || s.Category != "new" {
		t.Fatalf("history-only status: %+v (present %v)", got["1"], ok)
	}
	if got["3"].Category != "indeterminate" {
		t.Fatalf("inprogress must map to indeterminate: %+v", got["3"])
	}
	if got["10001"].Category != "done" {
		t.Fatalf("done category: %+v", got["10001"])
	}
	if len(st.UnnamedStatuses) != 0 {
		t.Fatalf("no status should be unnamed here: %v", st.UnnamedStatuses)
	}
}

func TestGhostUsersAndLossCounts(t *testing.T) {
	doc, st := build(t)
	var ghost bool
	for _, u := range doc.Users {
		if u.AccountID == "acc-gone" && u.DisplayName == "acc-gone" {
			ghost = true
		}
	}
	if !ghost || len(st.MissingUsers) != 1 || st.MissingUsers[0] != "acc-gone" {
		t.Fatalf("ghost user missing: users=%+v missing=%v", doc.Users, st.MissingUsers)
	}
	// GDK-1382: the same nodes, now counted as formatting carried through
	// the fixture's ADF slot rather than as loss.
	if st.FmtCodeBlock != 1 || st.FmtMedia != 1 || st.FmtTable != 0 {
		t.Fatalf("formatting counts code=%d media=%d table=%d", st.FmtCodeBlock, st.FmtMedia, st.FmtTable)
	}
	var t1 Issue
	for _, is := range doc.Issues {
		if is.Key == "T-1" {
			t1 = is
		}
	}
	if !strings.Contains(t1.DescriptionADF, `"type":"codeBlock"`) {
		t.Fatalf("T-1 must carry its description ADF verbatim (GDK-1382), got %q", t1.DescriptionADF)
	}
}

func TestInlineAttachments(t *testing.T) {
	doc := &Doc{Issues: []Issue{{Key: "T-1", Attachments: []Attachment{
		{Filename: "a.txt", MimeType: "text/plain", ContentID: "1"},
		{Filename: "b.bin", MimeType: "application/octet-stream", ContentID: "2"},
		{Filename: "gone.txt", MimeType: "text/plain", ContentID: "3"},
		{Filename: "linear.png", MimeType: "image/png", ContentID: "4", SourceURL: "https://uploads.linear.app/x"},
		{Filename: "huge.bin", ContentID: "5", Size: MaxAttachmentBytes + 1},
	}}}}
	st := &Stats{}
	bin := []byte{0x00, 0xFF, 0x10}
	InlineAttachments(context.Background(), doc, func(_ context.Context, id string) (int, []byte, error) {
		switch id {
		case "1":
			return 200, []byte("hello\n"), nil
		case "2":
			return 200, bin, nil
		case "3":
			return 404, nil, nil
		}
		t.Fatalf("unexpected fetch %q", id)
		return 0, nil, nil
	}, st)
	atts := doc.Issues[0].Attachments
	if atts[0].Text != "hello\n" || atts[0].DataBase64 != "" {
		t.Fatalf("text inline: %+v", atts[0])
	}
	if atts[1].DataBase64 != base64.StdEncoding.EncodeToString(bin) {
		t.Fatalf("binary inline: %+v", atts[1])
	}
	if st.AttachInlined != 2 || st.AttachMissing != 1 || st.AttachSkipURL != 1 || st.AttachTooLarge != 1 {
		t.Fatalf("stats %+v", st)
	}
}

// GDK-1361: status_catalog is written by sync passes, so a mirror that never
// ran one — a snapshot, examples/demo.db — has an empty table. The issues
// still carry status_category, and that is what the catalog must be built
// from: with the table empty this fell to "new" for every status, and the
// migrated board opened with Done and In Progress as open issues.
func TestStatusCategoriesSurviveEmptyCatalog(t *testing.T) {
	path := seedMirror(t)
	rw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open rw: %v", err)
	}
	if _, err := rw.Exec(`DELETE FROM status_catalog`); err != nil {
		t.Fatalf("empty status_catalog: %v", err)
	}
	if err := rw.Close(); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	doc, _, err := Build(context.Background(), sqlDB, Options{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got := map[string]string{}
	for _, s := range doc.Statuses {
		got[s.ID] = s.Category
	}
	if got["3"] != "indeterminate" || got["10001"] != "done" {
		t.Fatalf("categories with an empty status_catalog: %v, want 3→indeterminate, 10001→done", got)
	}
}
