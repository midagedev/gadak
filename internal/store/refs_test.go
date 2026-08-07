package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

// ── pure extraction ─────────────────────────────────────────────────────────

func TestExtractIssueRefsFromPageURL(t *testing.T) {
	adf := `{"type":"doc","content":[{"type":"text","text":"see https://x.example/browse/NMB-12 and /browse/DEMO-3"}]}`
	got := ExtractIssueRefsFromPage(adf, "", nil)
	want := []ItemRef{
		{TargetKind: "issue", TargetKey: "DEMO-3", Via: "url"},
		{TargetKind: "issue", TargetKey: "NMB-12", Via: "url"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("url refs = %+v, want %+v", got, want)
	}
}

func TestExtractIssueRefsFromPageTextFiltered(t *testing.T) {
	known := map[string]bool{"NMB": true, "DEMO": true}
	// SHA-256 and UTF-8 look like keys but are not known projects.
	body := `Fix NMB-42 and DEMO-7; also SHA-256 and UTF-8 and FOO-99.`
	got := ExtractIssueRefsFromPage("", body, known)
	want := []ItemRef{
		{TargetKind: "issue", TargetKey: "DEMO-7", Via: "text"},
		{TargetKind: "issue", TargetKey: "NMB-42", Via: "text"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("text refs = %+v, want %+v", got, want)
	}
	// Without known projects, no text hits.
	if got := ExtractIssueRefsFromPage("", body, nil); got != nil {
		t.Errorf("no known projects → %v, want nil", got)
	}
}

func TestExtractIssueRefsURLWinsOverText(t *testing.T) {
	known := map[string]bool{"NMB": true}
	adf := `link /browse/NMB-5 here`
	body := `mentions NMB-5 and NMB-9`
	got := ExtractIssueRefsFromPage(adf, body, known)
	// NMB-5 via=url (not text); NMB-9 via=text.
	want := []ItemRef{
		{TargetKind: "issue", TargetKey: "NMB-5", Via: "url"},
		{TargetKind: "issue", TargetKey: "NMB-9", Via: "text"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge = %+v, want %+v", got, want)
	}
}

func TestExtractPageRefsFromIssueURLShapes(t *testing.T) {
	body := `see https://x.example/wiki/spaces/ENG/pages/1001/Title and other`
	comments := []string{
		`pageId=2002 is the old link`,
		`also /wiki/spaces/DEMO/pages/1001 again`,
	}
	got := ExtractPageRefsFromIssue(body, comments)
	want := []ItemRef{
		{TargetKind: "page", TargetKey: "1001", Via: "url"},
		{TargetKind: "page", TargetKey: "2002", Via: "url"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("page refs = %+v, want %+v", got, want)
	}
}

func TestExtractPageRefsEmpty(t *testing.T) {
	if got := ExtractPageRefsFromIssue("", nil); got != nil {
		t.Errorf("empty = %v, want nil", got)
	}
	if got := ExtractPageRefsFromIssue("no links here", []string{"still none"}); got != nil {
		t.Errorf("no matches = %v, want nil", got)
	}
}

// ── upsert recompute ────────────────────────────────────────────────────────

func seedRefsFixture(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://j.example"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://j.example/wiki"}); err != nil {
		t.Fatal(err)
	}
	// Two issues so known project keys include NMB.
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{
			{
				Item: Item{
					ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
					Key: "NMB-1", Title: "one", BodyText: "mentions page /wiki/spaces/ENG/pages/100",
					CreatedAt: ago(2), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:2", SourceID: "jira", Kind: "issue", ExternalID: "2",
					Key: "NMB-2", Title: "two", BodyText: "pageId=200 in comment path",
					CreatedAt: ago(2), UpdatedAt: ago(1),
					// body has pageId; also add comment for 100
				},
				Issue: Issue{
					ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10001",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
				Comments: []Comment{{
					ID: "jira:c1", ExternalID: "c1", Author: "A",
					BodyText:  "see /wiki/spaces/ENG/pages/100",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	adf := json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"see /browse/NMB-1 and bare NMB-2 plus SHA-256"}]}]}`)
	if _, err := db.UpsertPages(context.Background(), []PageRecord{
		{
			Item: Item{
				ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
				Key: "100", Title: "Guide", BodyText: "see /browse/NMB-1 and bare NMB-2 plus SHA-256",
				Author: "Dana", URL: "https://j.example/wiki/spaces/ENG/pages/100",
				CreatedAt: ago(3), UpdatedAt: ago(1),
			},
			Page: Page{SpaceKey: "ENG", Version: 1, Status: "current", BodyADF: adf},
		},
		{
			Item: Item{
				ID: "confluence:200", SourceID: "confluence", Kind: "page", ExternalID: "200",
				Key: "200", Title: "Other", BodyText: "no issue keys here",
				Author: "Pat", URL: "https://j.example/wiki/spaces/ENG/pages/200",
				CreatedAt: ago(4), UpdatedAt: ago(2),
			},
			Page: Page{SpaceKey: "ENG", Version: 1, Status: "current",
				BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`)},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func loadRefs(t *testing.T, db *DB, itemID string) []ItemRef {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(), `
		SELECT target_kind, target_key, via FROM item_refs
		WHERE item_id = ? ORDER BY target_kind, target_key`, itemID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []ItemRef
	for rows.Next() {
		var r ItemRef
		if err := rows.Scan(&r.TargetKind, &r.TargetKey, &r.Via); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUpsertPagesWritesIssueRefs(t *testing.T) {
	db := openTemp(t)
	seedRefsFixture(t, db)

	got := loadRefs(t, db, "confluence:100")
	// NMB-1 via url (browse in ADF), NMB-2 via text (or url if also in ADF — both in same text).
	// ADF text contains /browse/NMB-1 and bare NMB-2; body_text same.
	// SHA-256 must not appear.
	want := []ItemRef{
		{TargetKind: "issue", TargetKey: "NMB-1", Via: "url"},
		{TargetKind: "issue", TargetKey: "NMB-2", Via: "text"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("page 100 refs = %+v, want %+v", got, want)
	}
	if got := loadRefs(t, db, "confluence:200"); len(got) != 0 {
		t.Errorf("page 200 refs = %+v, want empty", got)
	}
}

func TestUpsertIssuesWritesPageRefs(t *testing.T) {
	db := openTemp(t)
	seedRefsFixture(t, db)

	got1 := loadRefs(t, db, "jira:1")
	want1 := []ItemRef{{TargetKind: "page", TargetKey: "100", Via: "url"}}
	if !reflect.DeepEqual(got1, want1) {
		t.Errorf("NMB-1 refs = %+v, want %+v", got1, want1)
	}
	got2 := loadRefs(t, db, "jira:2")
	// body pageId=200 + comment pages/100
	want2 := []ItemRef{
		{TargetKind: "page", TargetKey: "100", Via: "url"},
		{TargetKind: "page", TargetKey: "200", Via: "url"},
	}
	if !reflect.DeepEqual(got2, want2) {
		t.Errorf("NMB-2 refs = %+v, want %+v", got2, want2)
	}
}

func TestUpsertRemovesStaleRefs(t *testing.T) {
	db := openTemp(t)
	seedRefsFixture(t, db)

	// Rewrite page without any issue mentions — old refs must disappear.
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
			Key: "100", Title: "Guide", BodyText: "cleaned body",
			Author: "Dana", URL: "https://j.example/wiki/spaces/ENG/pages/100",
			CreatedAt: ago(3), UpdatedAt: ago(0),
		},
		Page: Page{SpaceKey: "ENG", Version: 2, Status: "current",
			BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`)},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := loadRefs(t, db, "confluence:100"); len(got) != 0 {
		t.Errorf("after clean upsert refs = %+v, want empty", got)
	}

	// Rewrite issue without page links.
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: map[string]string{"1": "new"},
		Force:      true,
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMB-1", Title: "one", BodyText: "no pages",
				CreatedAt: ago(2), UpdatedAt: ago(0),
			},
			Issue: Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := loadRefs(t, db, "jira:1"); len(got) != 0 {
		t.Errorf("after clean issue upsert refs = %+v, want empty", got)
	}
}

// ── backfill ────────────────────────────────────────────────────────────────

func TestMigrateV16BackfillsItemRefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v15.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// Apply migrations 0..14 → user_version 15 (excerpt, no item_refs).
	for i := 0; i < 15; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 15`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	// Seed issue + page without going through Upsert (pre-v16 path).
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind, base_url) VALUES
		  ('jira', 'jira', 'https://j.example'),
		  ('confluence', 'confluence', 'https://j.example/wiki');
		INSERT INTO items (id, source_id, kind, external_id, key, title, body_text, author, url, created_at, updated_at, synced_at)
		VALUES
		  ('jira:1', 'jira', 'issue', '1', 'NMB-1', 'one',
		   'see /wiki/spaces/ENG/pages/55', 'A', 'https://j.example/browse/NMB-1',
		   '2026-01-01', '2026-01-02', '2026-01-02'),
		  ('confluence:55', 'confluence', 'page', '55', '55', 'Doc',
		   'mentions NMB-1 and UTF-8', 'B', 'https://j.example/wiki/spaces/ENG/pages/55',
		   '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO issues (item_id, key, project_key, priority_rank, reopen_count, comment_count)
		VALUES ('jira:1', 'NMB-1', 'NMB', 0, 0, 0);
		INSERT INTO pages (item_id, space_key, parent_id, version, status, body_adf, labels, excerpt)
		VALUES ('confluence:55', 'ENG', '', 1, 'current',
		        '{"type":"doc","content":[{"type":"text","text":"/browse/NMB-1"}]}',
		        '[]', 'mentions NMB-1');
	`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v15: %v", err)
	}
	defer db.Close()
	if got := db.SchemaVersion(); got < 16 {
		t.Fatalf("schema version %d, want ≥ 16", got)
	}

	// Page → issue (url from ADF).
	pageRefs := loadRefs(t, db, "confluence:55")
	wantPage := []ItemRef{{TargetKind: "issue", TargetKey: "NMB-1", Via: "url"}}
	if !reflect.DeepEqual(pageRefs, wantPage) {
		t.Errorf("backfill page refs = %+v, want %+v", pageRefs, wantPage)
	}
	// Issue → page.
	issueRefs := loadRefs(t, db, "jira:1")
	wantIssue := []ItemRef{{TargetKind: "page", TargetKey: "55", Via: "url"}}
	if !reflect.DeepEqual(issueRefs, wantIssue) {
		t.Errorf("backfill issue refs = %+v, want %+v", issueRefs, wantIssue)
	}
}

// ── detail API both directions ──────────────────────────────────────────────

func TestPageDetailRefAndBacklinkIssueKeys(t *testing.T) {
	db := openTemp(t)
	seedRefsFixture(t, db)

	d, err := db.PageDetail(context.Background(), "100")
	if err != nil || d == nil {
		t.Fatalf("PageDetail(100): %v %#v", err, d)
	}
	// Outgoing: NMB-1, NMB-2 (both mirrored), sorted.
	if !reflect.DeepEqual(d.RefIssueKeys, []string{"NMB-1", "NMB-2"}) {
		t.Errorf("RefIssueKeys = %v, want [NMB-1 NMB-2]", d.RefIssueKeys)
	}
	// Incoming: NMB-1 (body) and NMB-2 (comment) both point at page 100.
	if !reflect.DeepEqual(d.BacklinkIssueKeys, []string{"NMB-1", "NMB-2"}) {
		t.Errorf("BacklinkIssueKeys = %v, want [NMB-1 NMB-2]", d.BacklinkIssueKeys)
	}

	// Page 200: no outgoing; incoming NMB-2 (pageId=200).
	d2, err := db.PageDetail(context.Background(), "200")
	if err != nil || d2 == nil {
		t.Fatalf("PageDetail(200): %v %#v", err, d2)
	}
	if len(d2.RefIssueKeys) != 0 {
		t.Errorf("page 200 RefIssueKeys = %v, want empty", d2.RefIssueKeys)
	}
	if !reflect.DeepEqual(d2.BacklinkIssueKeys, []string{"NMB-2"}) {
		t.Errorf("page 200 BacklinkIssueKeys = %v, want [NMB-2]", d2.BacklinkIssueKeys)
	}

	// Unmirrored target is stored but not exposed.
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:300", SourceID: "confluence", Kind: "page", ExternalID: "300",
			Key: "300", Title: "Ghost", BodyText: "mentions /browse/NMB-99 only",
			CreatedAt: ago(1), UpdatedAt: ago(1),
		},
		Page: Page{SpaceKey: "ENG", Version: 1, Status: "current",
			BodyADF: json.RawMessage(`{"type":"doc","content":[{"type":"text","text":"/browse/NMB-99"}]}`)},
	}}); err != nil {
		t.Fatal(err)
	}
	// Raw row exists.
	if got := loadRefs(t, db, "confluence:300"); len(got) != 1 || got[0].TargetKey != "NMB-99" {
		t.Errorf("stored ghost ref = %+v", got)
	}
	d3, err := db.PageDetail(context.Background(), "300")
	if err != nil || d3 == nil {
		t.Fatalf("PageDetail(300): %v", err)
	}
	if len(d3.RefIssueKeys) != 0 {
		t.Errorf("ghost key exposed as %v, want omitted/empty", d3.RefIssueKeys)
	}
}

func TestIssueDetailRefAndBacklinkPages(t *testing.T) {
	db := openTemp(t)
	seedRefsFixture(t, db)

	d, err := db.Detail(context.Background(), "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	// NMB-1 body → page 100.
	if len(d.RefPages) != 1 || d.RefPages[0].Key != "100" {
		t.Errorf("RefPages = %+v, want [key=100]", d.RefPages)
	}
	// Page 100 mentions NMB-1 → backlink.
	if len(d.BacklinkPages) != 1 || d.BacklinkPages[0].Key != "100" {
		t.Errorf("BacklinkPages = %+v, want [key=100]", d.BacklinkPages)
	}
	// PageLite fields populated.
	if d.RefPages[0].Title != "Guide" || d.RefPages[0].SpaceKey != "ENG" {
		t.Errorf("RefPages lite = %+v", d.RefPages[0])
	}

	d2, err := db.Detail(context.Background(), "NMB-2")
	if err != nil {
		t.Fatal(err)
	}
	// NMB-2 → pages 100 and 200; order updated_at DESC (100 is newer).
	if len(d2.RefPages) != 2 {
		t.Fatalf("NMB-2 RefPages len = %d, want 2: %+v", len(d2.RefPages), d2.RefPages)
	}
	if d2.RefPages[0].Key != "100" || d2.RefPages[1].Key != "200" {
		t.Errorf("NMB-2 RefPages order = %s,%s want 100,200", d2.RefPages[0].Key, d2.RefPages[1].Key)
	}
	// Only page 100 mentions NMB-2.
	if len(d2.BacklinkPages) != 1 || d2.BacklinkPages[0].Key != "100" {
		t.Errorf("NMB-2 BacklinkPages = %+v, want [100]", d2.BacklinkPages)
	}
}

func TestDetailRefsJSONOmitEmpty(t *testing.T) {
	db := openTemp(t)
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{
		Categories: map[string]string{"1": "new"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:9", SourceID: "jira", Kind: "issue", ExternalID: "9",
				Key: "DEMO-9", Title: "lonely", BodyText: "nothing",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Issue: Issue{
				ProjectKey: "DEMO", IssueType: "Task", IssueTypeID: "1",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	d, err := db.Detail(context.Background(), "DEMO-9")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["ref_pages"]; ok {
		t.Errorf("empty ref_pages should omit, got %v", m["ref_pages"])
	}
	if _, ok := m["backlink_pages"]; ok {
		t.Errorf("empty backlink_pages should omit, got %v", m["backlink_pages"])
	}
}
