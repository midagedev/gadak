package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
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

// TestMigrateV27ToV28CommentVisibility is FAIL-first for GDK-511: a v27
// mirror that already has a comments row keeps that row, and the three new
// columns take their defaults (” / ” / NULL). No changelog backfill.
func TestMigrateV27ToV28CommentVisibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v27.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// v1–v25, then v27 (resolution_id). schemaV26 copies personal state and
	// is irrelevant here; user_version is set to 27 so Open applies only v28.
	for i := 0; i < personalStateCopyVersion-1; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	if _, err := raw.Exec(schemaV27); err != nil {
		raw.Close()
		t.Fatalf("apply v27: %v", err)
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind) VALUES ('jira', 'jira');
		INSERT INTO items (id, source_id, kind, key, title, body_text, created_at, updated_at, synced_at)
		VALUES ('jira:1', 'jira', 'issue', 'NMB-1', 'one', 'the flattened body', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO issues (item_id, key, project_key, priority_rank, reopen_count, comment_count)
		VALUES ('jira:1', 'NMB-1', 'NMB', 0, 0, 0);
		INSERT INTO comments (id, item_id, external_id, author, author_id, body_text, created_at, updated_at)
		VALUES ('jira:c1', 'jira:1', 'c1', 'Dana', 'acc-dana', 'hello', '2026-01-01', '2026-01-01')`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 27"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v27: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if got := db.SchemaVersion(); got != len(migrations) {
		t.Errorf("schema version %d, want %d", got, len(migrations))
	}

	var n int
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM comments WHERE id = 'jira:c1'`).Scan(&n); err != nil {
		t.Fatalf("comments row: %v", err)
	}
	if n != 1 {
		t.Errorf("comments rows = %d, want 1 (v28 must not drop existing comments)", n)
	}

	var visType, visValue, body string
	var jsd sql.NullInt64
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT visibility_type, visibility_value, jsd_public, body_text FROM comments WHERE id = 'jira:c1'`).
		Scan(&visType, &visValue, &jsd, &body); err != nil {
		t.Fatalf("v28 columns: %v", err)
	}
	if visType != "" || visValue != "" {
		t.Errorf("visibility = %q/%q, want ''/'' (no backfill; next sync rewrites)", visType, visValue)
	}
	if jsd.Valid {
		t.Errorf("jsd_public valid=%v val=%d, want NULL (marker absent)", jsd.Valid, jsd.Int64)
	}
	if body != "hello" {
		t.Errorf("body_text = %q, want hello (row must survive)", body)
	}
}

// TestMigrateV29ToV30SprintColumns is FAIL-first for GDK-518: a v29 mirror
// gains issues.sprint_id / sprint_name / sprint_state as NULL (no backfill)
// and issues_full exposes them. Open lands at the binary's current
// PRAGMA user_version (not a frozen "30") so a later migration does not
// force this test to bump a constant.
func TestMigrateV29ToV30SprintColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v29.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < personalStateCopyVersion-1; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	for _, stmt := range []string{schemaV27, schemaV28, schemaV29} {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("apply later v27-v29: %v", err)
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
	if _, err := raw.Exec("PRAGMA user_version = 29"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v29: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if got := db.SchemaVersion(); got != len(migrations) {
		t.Errorf("schema version %d, want %d (Open lands at the binary's level)", got, len(migrations))
	}
	var uv int
	if err := db.sql.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if uv != len(migrations) {
		t.Errorf("PRAGMA user_version = %d, want %d (Open lands at the binary's level)", uv, len(migrations))
	}

	var id sql.NullInt64
	var name, state sql.NullString
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT sprint_id, sprint_name, sprint_state FROM issues WHERE key = 'NMB-1'`).
		Scan(&id, &name, &state); err != nil {
		t.Fatalf("issues sprint columns after v30: %v", err)
	}
	if id.Valid || name.Valid || state.Valid {
		t.Errorf("sprint columns = %v/%v/%v, want NULL (no backfill; next sync rewrites)", id, name, state)
	}

	var summary, desc string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT summary, description_text, sprint_id FROM issues_full WHERE key = 'NMB-1'`).
		Scan(&summary, &desc, &id); err != nil {
		t.Fatalf("issues_full after v30: %v", err)
	}
	if summary != "one" {
		t.Errorf("summary = %q, want one", summary)
	}
	if desc != "the flattened body" {
		t.Errorf("description_text = %q, want the flattened body (v23 expression must survive the rebuild)", desc)
	}
	if id.Valid {
		t.Errorf("issues_full.sprint_id valid=%v, want NULL", id.Valid)
	}

	var idx sql.NullString
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT sql FROM sqlite_master WHERE name = 'issues_sprint'`).Scan(&idx); err != nil {
		t.Fatalf("issues_sprint index: %v", err)
	}
	if idx.String == "" || !strings.Contains(idx.String, "sprint_id") {
		t.Errorf("issues_sprint ddl = %q", idx.String)
	}
}

// TestMigrateV30ToV31VersionCatalog is FAIL-first for GDK-532: a v30 mirror
// gains the versions table and issues.fix_version_ids as NULL (no backfill)
// and issues_full exposes the new column. Open lands at the binary's current
// PRAGMA user_version.
func TestMigrateV30ToV31VersionCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v30.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < personalStateCopyVersion-1; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	for _, stmt := range []string{schemaV27, schemaV28, schemaV29, schemaV30} {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("apply later v27-v30: %v", err)
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
	if _, err := raw.Exec("PRAGMA user_version = 30"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v30: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if got := db.SchemaVersion(); got != len(migrations) {
		t.Errorf("schema version %d, want %d (Open lands at the binary's level)", got, len(migrations))
	}
	var uv int
	if err := db.sql.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if uv != len(migrations) {
		t.Errorf("PRAGMA user_version = %d, want %d (Open lands at the binary's level)", uv, len(migrations))
	}

	var n int
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='versions'`).Scan(&n); err != nil {
		t.Fatalf("versions table: %v", err)
	}
	if n != 1 {
		t.Errorf("versions tables = %d, want 1", n)
	}

	var ids sql.NullString
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT fix_version_ids FROM issues WHERE key = 'NMB-1'`).Scan(&ids); err != nil {
		t.Fatalf("issues.fix_version_ids after v31: %v", err)
	}
	if ids.Valid {
		t.Errorf("fix_version_ids = %q, want NULL (no backfill; next sync rewrites)", ids.String)
	}

	var summary, desc string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT summary, description_text, fix_version_ids FROM issues_full WHERE key = 'NMB-1'`).
		Scan(&summary, &desc, &ids); err != nil {
		t.Fatalf("issues_full after v31: %v", err)
	}
	if summary != "one" {
		t.Errorf("summary = %q, want one", summary)
	}
	if desc != "the flattened body" {
		t.Errorf("description_text = %q, want the flattened body (v23 expression must survive the rebuild)", desc)
	}
	if ids.Valid {
		t.Errorf("issues_full.fix_version_ids valid=%v, want NULL", ids.Valid)
	}

	var idx sql.NullString
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT sql FROM sqlite_master WHERE name = 'versions_project'`).Scan(&idx); err != nil {
		t.Fatalf("versions_project index: %v", err)
	}
	if idx.String == "" || !strings.Contains(idx.String, "project_key") {
		t.Errorf("versions_project ddl = %q", idx.String)
	}
}

// TestMigrateV31ToV32SecurityLevel is FAIL-first for GDK-519: a v31 mirror
// gains issues.security_level_id / security_level as NULL (no backfill) and
// issues_full exposes them. Open lands at the binary's current PRAGMA
// user_version (not a frozen "32") so a later migration does not force this
// test to bump a constant.
func TestMigrateV31ToV32SecurityLevel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v31.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < personalStateCopyVersion-1; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	for _, stmt := range []string{schemaV27, schemaV28, schemaV29, schemaV30, schemaV31} {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("apply later v27-v31: %v", err)
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
	if _, err := raw.Exec("PRAGMA user_version = 31"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v31: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if got := db.SchemaVersion(); got != len(migrations) {
		t.Errorf("schema version %d, want %d (Open lands at the binary's level)", got, len(migrations))
	}
	var uv int
	if err := db.sql.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if uv != len(migrations) {
		t.Errorf("PRAGMA user_version = %d, want %d (Open lands at the binary's level)", uv, len(migrations))
	}

	var id, name sql.NullString
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT security_level_id, security_level FROM issues WHERE key = 'NMB-1'`).
		Scan(&id, &name); err != nil {
		t.Fatalf("issues security columns after v32: %v", err)
	}
	if id.Valid || name.Valid {
		t.Errorf("security columns = %v/%v, want NULL (no backfill; next sync rewrites)", id, name)
	}

	var summary, desc string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT summary, description_text, security_level_id FROM issues_full WHERE key = 'NMB-1'`).
		Scan(&summary, &desc, &id); err != nil {
		t.Fatalf("issues_full after v32: %v", err)
	}
	if summary != "one" {
		t.Errorf("summary = %q, want one", summary)
	}
	if desc != "the flattened body" {
		t.Errorf("description_text = %q, want the flattened body (v23 expression must survive the rebuild)", desc)
	}
	if id.Valid {
		t.Errorf("issues_full.security_level_id valid=%v, want NULL", id.Valid)
	}
}

// TestMigrateV32ToV33DevLinkAuthorActorBranch applies v1–v32 stepwise, seeds
// one v29 dev_links row, and opens with this build: v33 must add
// author/actor/actor_name/branch without touching the seeded row (no
// backfill; the next sync rewrites). This is also the guard for a schemaV33
// const that never made it into the migrations slice — an unused const
// compiles, doctor's audit compares against this same build's migrations and
// so cannot see it, and only a column read after Open fails.
func TestMigrateV32ToV33DevLinkAuthorActorBranch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v32.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < personalStateCopyVersion-1; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	for _, stmt := range []string{schemaV27, schemaV28, schemaV29, schemaV30, schemaV31, schemaV32} {
		if _, err := raw.Exec(stmt); err != nil {
			raw.Close()
			t.Fatalf("apply later v27-v32: %v", err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO sources (id, kind) VALUES ('jira', 'jira');
		INSERT INTO items (id, source_id, kind, key, title, body_text, created_at, updated_at, synced_at)
		VALUES ('jira:1', 'jira', 'issue', 'NMB-1', 'one', 'the flattened body', '2026-01-01', '2026-01-02', '2026-01-02');
		INSERT INTO issues (item_id, key, project_key, priority_rank, reopen_count, comment_count)
		VALUES ('jira:1', 'NMB-1', 'NMB', 0, 0, 0);
		INSERT INTO dev_links (item_id, url, title)
		VALUES ('jira:1', 'https://github.com/o/r/pull/7', 'pre-v33 link');`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 32"); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v32: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if got := db.SchemaVersion(); got != len(migrations) {
		t.Errorf("schema version %d, want %d (Open lands at the binary's level)", got, len(migrations))
	}
	var uv int
	if err := db.sql.QueryRowContext(context.Background(), `PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if uv != len(migrations) {
		t.Errorf("PRAGMA user_version = %d, want %d (Open lands at the binary's level)", uv, len(migrations))
	}

	var author, actor, actorName, branch string
	if err := db.sql.QueryRowContext(context.Background(),
		`SELECT author, actor, actor_name, branch FROM dev_links WHERE url = 'https://github.com/o/r/pull/7'`).
		Scan(&author, &actor, &actorName, &branch); err != nil {
		t.Fatalf("dev_links v33 columns after Open: %v", err)
	}
	if author != "" || actor != "" || actorName != "" || branch != "" {
		t.Errorf("seeded row = %q/%q/%q/%q, want all empty (no backfill; next sync rewrites)",
			author, actor, actorName, branch)
	}
}

// TestMigrateV37ToV38DropsMirrorPersonalTables is FAIL-first for GDK-824:
// the frozen mirror-side leftovers of the v26 copy (saved_views, watches,
// favorites, feed_reads) are dropped by v38. Audit D2, corrected by
// measurement: the mirror connection has local.db ATTACHed, and an
// unqualified name resolves main first — so while the leftover existed,
// `SELECT * FROM saved_views` silently answered with the frozen
// migration-time snapshot (or an empty table), shadowing local.db's truth.
// After the drop the unqualified name falls through to local.*, the
// current truth. local.db itself is not touched: a watch written there
// after the upgrade reads back through the production path.
func TestMigrateV37ToV38DropsMirrorPersonalTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v37.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// v1–v25 then v27–v37 (schemaV26 is the personal-state copy and is
	// skipped like the other vN fixtures); user_version = 37 so Open
	// applies only v38.
	for i := 0; i < personalStateCopyVersion-1; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			raw.Close()
			t.Fatalf("apply v%d: %v", i+1, err)
		}
	}
	for _, m := range migrations[personalStateCopyVersion:37] {
		if _, err := raw.Exec(m); err != nil {
			raw.Close()
			t.Fatalf("apply v27-v37: %v", err)
		}
	}
	for _, q := range []string{
		`INSERT INTO saved_views (id, name, config, created_at, updated_at) VALUES ('stale','Frozen','{}','2026-01-01T00:00:00.000Z','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO watches (key, created_at) VALUES ('STALE-1','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO favorites (key, created_at) VALUES ('STALE-2','2026-01-01T00:00:00.000Z')`,
		`INSERT INTO feed_reads (event_id, read_at) VALUES ('cl:stale','2026-01-01T00:00:00.000Z')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			raw.Close()
			t.Fatalf("seed leftover: %v", err)
		}
	}
	if _, err := raw.Exec(`PRAGMA user_version = 37`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open after v37: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	if got := db.SchemaVersion(); got != len(migrations) {
		t.Errorf("schema version %d, want %d", got, len(migrations))
	}
	var n int
	if err := db.sql.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name IN ('saved_views','watches','favorites','feed_reads')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("mirror-side personal tables after v38 = %d, want 0 (dropped)", n)
	}
	// Audit D2: with the shadowing leftover gone, an unqualified read
	// resolves through to local.db — the current truth, not a stale
	// snapshot and not the frozen STALE-1 row.
	if err := db.SetWatch(ctx, "LIVE-1", true); err != nil {
		t.Fatalf("SetWatch after v38: %v", err)
	}
	var key string
	if err := db.sql.QueryRowContext(ctx, `SELECT key FROM watches`).Scan(&key); err != nil {
		t.Fatalf("unqualified watches read after v38: %v", err)
	}
	if key != "LIVE-1" {
		t.Errorf("unqualified watches = %q, want LIVE-1 (resolved through to local.db; the frozen STALE-1 snapshot is gone)", key)
	}
	watches, err := db.Watches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 1 || watches[0] != "LIVE-1" {
		t.Errorf("Watches after v38 = %v, want [LIVE-1]", watches)
	}
}
