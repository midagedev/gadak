package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateV18ThroughV20Stepwise applies v19 then v20 on a temp DB and
// prints each user_version so the gate can show 18→19→20 actually ran.
func TestMigrateV18ThroughV20Stepwise(t *testing.T) {
	path := filepath.Join(t.TempDir(), "step.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 18; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind) VALUES ('jira', 'jira');
		INSERT INTO items (id, source_id, kind, key, title, created_at, updated_at, synced_at)
		VALUES ('jira:1', 'jira', 'issue', 'NMB-1', 'one', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO issues (item_id, key, project_key, priority_rank, reopen_count, comment_count)
		VALUES ('jira:1', 'NMB-1', 'NMB', 0, 0, 0);
		INSERT INTO changelog (id, item_id, at, author, field) VALUES ('jira:h1', 'jira:1', '2026-01-02', 'Kim', 'status');
		INSERT INTO attachments (id, item_id, filename, author) VALUES ('jira:a1', 'jira:1', 't.log', 'Kim');
		INSERT INTO spaces (source_id, key, name, kind, homepage_id)
		VALUES ('confluence', 'AAA', 'Alpha', 'global', '1000');
		PRAGMA user_version = 18`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	var uv int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	t.Logf("step v18: user_version=%d", uv)
	if uv != 18 {
		raw.Close()
		t.Fatalf("want 18, got %d", uv)
	}

	if _, err := raw.Exec(migrations[18]); err != nil {
		raw.Close()
		t.Fatalf("apply v19: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 19`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	var wm sql.NullString
	if err := raw.QueryRow(`SELECT watermark FROM spaces WHERE key = 'AAA'`).Scan(&wm); err != nil {
		raw.Close()
		t.Fatalf("v19 spaces.watermark: %v", err)
	}
	t.Logf("step v19: user_version=%d spaces.watermark_null=%v", uv, !wm.Valid)
	if uv != 19 || wm.Valid {
		raw.Close()
		t.Fatalf("v19 check failed uv=%d wm=%v", uv, wm)
	}

	if _, err := raw.Exec(migrations[19]); err != nil {
		raw.Close()
		t.Fatalf("apply v20: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 20`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	var clID, atID sql.NullString
	if err := raw.QueryRow(`SELECT author_id FROM changelog WHERE id = 'jira:h1'`).Scan(&clID); err != nil {
		raw.Close()
		t.Fatalf("v20 changelog.author_id: %v", err)
	}
	if err := raw.QueryRow(`SELECT author_id FROM attachments WHERE id = 'jira:a1'`).Scan(&atID); err != nil {
		raw.Close()
		t.Fatalf("v20 attachments.author_id: %v", err)
	}
	t.Logf("step v20: user_version=%d changelog.author_id_null=%v attachments.author_id_null=%v", uv, !clID.Valid, !atID.Valid)
	raw.Close()
	if uv != 20 || clID.Valid || atID.Valid {
		t.Fatalf("v20 check failed uv=%d cl=%v at=%v", uv, clID, atID)
	}
}

// TestMigrateV19ToV20PreservesAuthorIDNull is FAIL-first for I7: v20 adds
// nullable author_id on changelog and attachments; existing rows survive with
// NULL so name-fallback feed exclusion still works for legacy rows.
func TestMigrateV19ToV20PreservesAuthorIDNull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v19.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 19; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 19`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind) VALUES ('jira', 'jira');
		INSERT INTO items (id, source_id, kind, key, title, created_at, updated_at, synced_at)
		VALUES ('jira:1', 'jira', 'issue', 'NMB-1', 'one', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO issues (item_id, key, project_key, priority_rank, reopen_count, comment_count)
		VALUES ('jira:1', 'NMB-1', 'NMB', 0, 0, 0);
		INSERT INTO changelog (id, item_id, at, author, field, from_value, to_value)
		VALUES ('jira:h1', 'jira:1', '2026-01-02', 'Kim', 'status', 'To Do', 'Done');
		INSERT INTO attachments (id, item_id, filename, author, created_at)
		VALUES ('jira:a1', 'jira:1', 'trace.log', 'Kim', '2026-01-02');
	`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v19: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if got := db.SchemaVersion(); got < 20 {
		t.Fatalf("schema version %d, want ≥ 20", got)
	}

	var author string
	var authorID sql.NullString
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT author, author_id FROM changelog WHERE id = 'jira:h1'`).
		Scan(&author, &authorID); err != nil {
		t.Fatalf("changelog after v20: %v", err)
	}
	if author != "Kim" {
		t.Errorf("changelog author = %q, want Kim (row must be preserved)", author)
	}
	if authorID.Valid {
		t.Errorf("changelog.author_id = %q, want NULL on migrated row", authorID.String)
	}
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT author, author_id FROM attachments WHERE id = 'jira:a1'`).
		Scan(&author, &authorID); err != nil {
		t.Fatalf("attachments after v20: %v", err)
	}
	if author != "Kim" {
		t.Errorf("attachment author = %q, want Kim", author)
	}
	if authorID.Valid {
		t.Errorf("attachments.author_id = %q, want NULL on migrated row", authorID.String)
	}
}

// TestChangelogAttachmentAuthorIDWrittenAndFeedSelfExclude is FAIL-first for
// I7 write + feed: new sync rows store author_id, and two accounts that share
// a display name do not collide when excluding self.
func TestChangelogAttachmentAuthorIDWrittenAndFeedSelfExclude(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if err := db.UpsertSource(ctx, Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, Batch{
		Categories: map[string]string{"1": "new", "3": "inprogress"},
		Records: []IssueRecord{{
			Item: Item{
				ID: "jira:1", SourceID: "jira", ExternalID: "1", Key: "NMB-1",
				Title: "same name", Author: "Kim", AuthorID: "acc-other",
				CreatedAt: "2026-07-20T00:00:00.000Z", UpdatedAt: "2026-08-04T00:00:00.000Z",
			},
			Issue: Issue{
				ProjectKey: "NMB", Status: "In Progress", StatusID: "3",
				StatusCategory: "inprogress",
				Assignee:       "Me", AssigneeID: "acc-me", AssigneeEmail: "me@example.com",
			},
			Attachments: []Attachment{{
				ID: "jira:a-me", ExternalID: "a-me", Filename: "mine.png",
				Author: "Kim", AuthorID: "acc-me", CreatedAt: "2026-08-03T12:00:00.000Z",
			}, {
				ID: "jira:a-ot", ExternalID: "a-ot", Filename: "theirs.png",
				Author: "Kim", AuthorID: "acc-other", CreatedAt: "2026-08-03T13:00:00.000Z",
			}},
			Changelog: []ChangeEntry{
				{
					ID: "jira:h-me", At: "2026-08-02T09:00:00.000Z",
					Author: "Kim", AuthorID: "acc-me",
					Field: "status", FromValue: "To Do", ToValue: "In Progress",
				},
				{
					ID: "jira:h-ot", At: "2026-08-02T10:00:00.000Z",
					Author: "Kim", AuthorID: "acc-other",
					Field: "status", FromValue: "In Progress", ToValue: "In Progress",
				},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	var gotID sql.NullString
	if err := db.sql.QueryRowContext(ctx, `SELECT author_id FROM changelog WHERE id = 'jira:h-me'`).Scan(&gotID); err != nil {
		t.Fatal(err)
	}
	if !gotID.Valid || gotID.String != "acc-me" {
		t.Errorf("I7 write: changelog.author_id = %v, want acc-me", gotID)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT author_id FROM attachments WHERE id = 'jira:a-me'`).Scan(&gotID); err != nil {
		t.Fatal(err)
	}
	if !gotID.Valid || gotID.String != "acc-me" {
		t.Errorf("I7 write: attachments.author_id = %v, want acc-me", gotID)
	}

	res, err := db.Feed(ctx, FeedOpts{
		Focus: FeedFocusAll, Limit: 100,
		Me:  FeedIdentity{AccountID: "acc-me", DisplayName: "Kim"},
		Now: frozenNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := byEventID(res.Items)
	if _, ok := m["cl:jira:h-me"]; ok {
		t.Error("I7: own changelog (same display name) must be excluded by author_id")
	}
	if _, ok := m["at:jira:a-me"]; ok {
		t.Error("I7: own attachment (same display name) must be excluded by author_id")
	}
	if _, ok := m["cl:jira:h-ot"]; !ok {
		t.Error("I7: other Kim's changelog must remain (author_id differs)")
	}
	if _, ok := m["at:jira:a-ot"]; !ok {
		t.Error("I7: other Kim's attachment must remain (author_id differs)")
	}

	d, err := db.Detail(ctx, "NMB-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.History) < 2 {
		t.Fatalf("history len %d", len(d.History))
	}
	foundMe := false
	for _, h := range d.History {
		if h.AuthorID == "acc-me" {
			foundMe = true
		}
	}
	if !foundMe {
		t.Error("I7 read: Detail.History missing author_id")
	}
	foundAtt := false
	for _, a := range d.Attachments {
		if a.AuthorID == "acc-me" {
			foundAtt = true
		}
	}
	if !foundAtt {
		t.Error("I7 read: Detail.Attachments missing author_id")
	}
}

// TestMigrateV26ToV27ResolutionID is FAIL-first for GDK-520: a v26-era
// database gains issues.resolution_id as ” (no changelog backfill), and
// issues_full still exposes summary, description_text, and the new column.
func TestMigrateV26ToV27ResolutionID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v26.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// Apply v1–v25. schemaV26 copies personal state into local.* and needs
	// that attach; it is irrelevant to resolution_id. user_version is then
	// set to 26 so Open applies only v27.
	for i := 0; i < personalStateCopyVersion-1; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind) VALUES ('jira', 'jira');
		INSERT INTO items (id, source_id, kind, key, title, body_text, created_at, updated_at, synced_at)
		VALUES ('jira:1', 'jira', 'issue', 'NMB-1', 'one', 'the flattened body', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO issues (item_id, key, project_key, priority_rank, reopen_count, comment_count)
		VALUES ('jira:1', 'NMB-1', 'NMB', 0, 0, 0)`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 26"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v26: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var resid string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT resolution_id FROM issues WHERE key = 'NMB-1'`).Scan(&resid); err != nil {
		t.Fatalf("issues.resolution_id after v27: %v", err)
	}
	if resid != "" {
		t.Errorf("resolution_id = %q, want '' (no changelog backfill; next sync rewrites)", resid)
	}

	var summary, desc string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT summary, description_text, resolution_id FROM issues_full WHERE key = 'NMB-1'`).
		Scan(&summary, &desc, &resid); err != nil {
		t.Fatalf("issues_full after v27: %v", err)
	}
	if summary != "one" {
		t.Errorf("summary = %q, want one", summary)
	}
	if desc != "the flattened body" {
		t.Errorf("description_text = %q, want the flattened body (v23 expression must survive the rebuild)", desc)
	}
	if resid != "" {
		t.Errorf("issues_full.resolution_id = %q, want ''", resid)
	}
}
