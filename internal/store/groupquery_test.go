package store

import (
	"context"
	"testing"
)

func TestGroupQueryHits(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	if _, err := db.sql.ExecContext(ctx, `
		INSERT INTO sources (id, kind) VALUES ('jira', 'jira');
		INSERT INTO items (id, source_id, kind, external_id, key, title, created_at, updated_at, synced_at)
		VALUES
			('1','jira','issue','1','NMB-1','one','t','t','t'),
			('2','jira','issue','2','NMB-2','two','t','t','t'),
			('3','jira','issue','3','DESK-1','desk','t','t','t');
		INSERT INTO issues_raw (item_id, key, project_key, labels, components, custom, priority_rank, reopen_count, comment_count)
		VALUES
			('1','NMB-1','NMB','["skip-triage"]','[]','{}',0,0,0),
			('2','NMB-2','NMB','["backend"]','["payments-api"]','{"billing_code":"invoice"}',0,0,0),
			('3','DESK-1','DESK','[]','[]','{}',0,0,0);
	`); err != nil {
		t.Fatal(err)
	}

	hits, err := db.GroupQueryHits(ctx, `
		SELECT i.key,
			CASE
				WHEN EXISTS (SELECT 1 FROM json_each(i.labels) e WHERE e.value = 'skip-triage') THEN ''
				WHEN i.components REGEXP '(?i)payment' OR coalesce(json_extract(i.custom,'$.billing_code'),'') REGEXP '(?i)invoice' THEN 'billing'
				WHEN EXISTS (SELECT 1 FROM json_each(i.labels) e WHERE e.value = 'backend') THEN 'platform'
				ELSE NULL
			END
		FROM issues i
	`)
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := hits["NMB-1"]; !ok || g != "" {
		t.Fatalf("NMB-1: present=%v group=%q (want unclassified)", ok, g)
	}
	if hits["NMB-2"] != "billing" {
		t.Fatalf("NMB-2 = %q, want billing", hits["NMB-2"])
	}
	if _, ok := hits["DESK-1"]; ok {
		t.Fatalf("DESK-1 should fall through, got %q", hits["DESK-1"])
	}
}

func TestGroupQueryHits_RejectsWrite(t *testing.T) {
	db := openTemp(t)
	if _, err := db.GroupQueryHits(context.Background(), "DELETE FROM issues"); err == nil {
		t.Fatal("want error")
	}
}
