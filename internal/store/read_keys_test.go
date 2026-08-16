package store

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

func TestIssueLitesByKeys(t *testing.T) {
	db := openTemp(t)
	seed(t, db)

	all, err := db.IssueLites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("fixture size %d, want 3", len(all))
	}
	byKey := make(map[string]IssueLite, len(all))
	for _, l := range all {
		byKey[l.IssueKey] = l
	}

	// Requested order, not IssueLites' updated_at DESC (NMB-1, NMB-3, NMB-2).
	got, err := db.IssueLitesByKeys(context.Background(), []string{"NMB-2", "NMB-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].IssueKey != "NMB-2" || got[1].IssueKey != "NMB-1" {
		t.Fatalf("order = %v, want [NMB-2 NMB-1]", issueKeysOf(got))
	}
	if !reflect.DeepEqual(got[0], byKey["NMB-2"]) || !reflect.DeepEqual(got[1], byKey["NMB-1"]) {
		t.Fatal("row content diverged from IssueLites for the same key")
	}

	// Missing keys are skipped; duplicates are preserved in request order.
	got, err = db.IssueLitesByKeys(context.Background(), []string{"NMB-3", "NOPE", "NMB-3", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].IssueKey != "NMB-3" || got[1].IssueKey != "NMB-3" {
		t.Fatalf("missing/dup = %v, want [NMB-3 NMB-3]", issueKeysOf(got))
	}

	empty, err := db.IssueLitesByKeys(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("nil keys = %#v, want empty slice", empty)
	}

	// The IN-list must use issues_key, not a full issues scan.
	rows, err := db.sql.QueryContext(context.Background(),
		`EXPLAIN QUERY PLAN `+issueLiteSelect+` WHERE i.key IN (?, ?)`, "NMB-1", "NMB-2")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var selid, order, from int
		var detail string
		if err := rows.Scan(&selid, &order, &from, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan, " | ")
	if !strings.Contains(joined, "issues_key") {
		t.Fatalf("expected SEARCH via issues_key, plan=%q", joined)
	}
	if strings.Contains(strings.ToLower(joined), "scan issues") && !strings.Contains(joined, "issues_key") {
		t.Fatalf("full scan of issues: %q", joined)
	}
}

func issueKeysOf(rows []IssueLite) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.IssueKey
	}
	return out
}

// lookupViaFullScan is the HEAD MCP/CLI hydration path, kept so the benchmark
// can measure it against IssueLitesByKeys on the same 5k fixture.
func lookupViaFullScan(db *DB, keys []string) ([]IssueLite, error) {
	all, err := db.IssueLites(context.Background())
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]IssueLite, len(all))
	for _, l := range all {
		byKey[l.IssueKey] = l
	}
	out := make([]IssueLite, 0, len(keys))
	for _, k := range keys {
		if l, ok := byKey[k]; ok {
			out = append(out, l)
		}
	}
	return out, nil
}

var issueLiteSink []IssueLite

func BenchmarkIssueLiteLookup5k(b *testing.B) {
	db := seedManyIssues(b, 5000)
	keys := []string{
		"NMB-1", "NMA-2", "NMS-3", "NMB-100", "NMA-250",
		"NMS-400", "NMB-800", "NMA-1200", "NMS-2000", "NMB-2500",
		"NMA-3000", "NMS-3500", "NMB-4000", "NMA-4500", "NMS-4999",
		"NMB-17", "NMA-99", "NMS-512", "NMB-1024", "MISSING-1",
	}

	// Confirm the two paths agree on this fixture before timing them.
	want, err := lookupViaFullScan(db, keys)
	if err != nil {
		b.Fatal(err)
	}
	got, err := db.IssueLitesByKeys(context.Background(), keys)
	if err != nil {
		b.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		b.Fatalf("ByKeys != full-scan: got %v want %v", issueKeysOf(got), issueKeysOf(want))
	}

	b.Run("FullScan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out, err := lookupViaFullScan(db, keys)
			if err != nil {
				b.Fatal(err)
			}
			issueLiteSink = out
		}
	})
	b.Run("ByKeys", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			out, err := db.IssueLitesByKeys(context.Background(), keys)
			if err != nil {
				b.Fatal(err)
			}
			issueLiteSink = out
		}
	})
}

func seedManyIssues(tb testing.TB, n int) *DB {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "gadak.db")
	db, err := Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		tb.Fatal(err)
	}
	categories := map[string]string{"1": "new", "3": "inprogress", "5": "done"}
	projects := []string{"NMB", "NMA", "NMS"}
	statuses := []struct{ name, id, cat string }{
		{"To Do", "1", "new"},
		{"In Progress", "3", "inprogress"},
		{"Done", "5", "done"},
	}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	const batchSize = 250
	for start := 0; start < n; start += batchSize {
		end := start + batchSize
		if end > n {
			end = n
		}
		recs := make([]IssueRecord, 0, end-start)
		for i := start; i < end; i++ {
			proj := projects[i%len(projects)]
			st := statuses[i%len(statuses)]
			num := i + 1
			extID := fmt.Sprintf("%d", 10000+i)
			created := base.Add(time.Duration(i) * time.Hour)
			recs = append(recs, IssueRecord{
				Item: Item{
					ID: "jira:" + extID, SourceID: "jira", Kind: "issue", ExternalID: extID,
					Key: fmt.Sprintf("%s-%d", proj, num), Title: fmt.Sprintf("bench issue %d", num),
					CreatedAt: created.UTC().Format(config.ISOMilli),
					UpdatedAt: created.UTC().Format(config.ISOMilli),
				},
				Issue: Issue{
					ProjectKey: proj, IssueType: "Task", IssueTypeID: "10002",
					Status: st.name, StatusID: st.id, StatusCategory: st.cat,
				},
			})
		}
		if _, err := db.UpsertIssues(context.Background(), Batch{Categories: categories, Records: recs, Force: true}); err != nil {
			tb.Fatalf("upsert %d-%d: %v", start, end-1, err)
		}
	}
	return db
}
