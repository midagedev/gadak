//go:build searchscale

package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"
)

// TestSearch20kProfile attributes store.Search milliseconds on the 20k fixture
// to query stages. Run:
//
//	go test -tags searchscale ./internal/store/ -count=1 -run TestSearch20kProfile -v -timeout 15m
//
// Does not run under plain `go test ./...`.
func TestSearch20kProfile(t *testing.T) {
	db := seedSearchScale(t, searchScaleN)
	ctx := context.Background()

	var nItems, nFTS, nComments int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM items`).Scan(&nItems); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM items_fts`).Scan(&nFTS); err != nil {
		t.Fatal(err)
	}
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM comments`).Scan(&nComments); err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture items=%d fts=%d comments=%d", nItems, nFTS, nComments)

	shapes := []struct {
		name  string
		q     string
		limit int
	}{
		{"p limit 5", "p", 5},
		{"p limit 20", "p", 20},
		{"p limit 50", "p", 50},
		{"korean 2-char limit 20", "로그", 20},
		{"retry high-hit limit 20", "retry", 20},
		{"0-hit limit 20", "zzznomatchtokenzzz", 20},
		{"key lookup NMB-1", "NMB-1", 20},
	}

	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			match := ftsPrefixQuery(sh.q)
			var hits int
			if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM items_fts WHERE items_fts MATCH ?`, match).Scan(&hits); err != nil {
				t.Fatalf("count MATCH %q: %v", match, err)
			}
			t.Logf("q=%q match=%q fts_hits=%d limit=%d", sh.q, match, hits, sh.limit)

			stages := profileStages(match, sh.limit)
			fmt.Printf("\n== %s  match=%q  fts_hits=%d ==\n", sh.name, match, hits)
			for _, st := range stages {
				med, err := medianSQL(ctx, db.sql, st.sql, st.args...)
				if err != nil {
					t.Fatalf("%s: %v", st.name, err)
				}
				fmt.Printf("  %-28s  %7.2f ms\n", st.name, med)
				t.Logf("  %-28s  %7.2f ms", st.name, med)
			}

			medSearch, err := medianFn(func() error {
				_, err := db.Search(ctx, sh.q, sh.limit)
				return err
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			fmt.Printf("  %-28s  %7.2f ms\n", "store.Search (Go)", medSearch)
			t.Logf("  %-28s  %7.2f ms", "store.Search (Go)", medSearch)

			if looksLikeKey(sh.q) {
				medKey, err := medianFn(func() error {
					_, err := db.lookupKeyHits(ctx, sh.q, sh.limit)
					return err
				})
				if err != nil {
					t.Fatalf("lookupKeyHits: %v", err)
				}
				fmt.Printf("  %-28s  %7.2f ms\n", "lookupKeyHits", medKey)
				t.Logf("  %-28s  %7.2f ms", "lookupKeyHits", medKey)
			}
		})
	}
}

type profileStage struct {
	name string
	sql  string
	args []any
}

func profileStages(match string, limit int) []profileStage {
	titleMatch := "title : " + match
	bodyMatch := "body_text : " + match
	commentMatch := "comments_text : " + match
	rank := ftsRankSQL()
	return []profileStage{
		{
			name: "a MATCH+rank+LIMIT",
			sql: `SELECT f.rowid, ` + rank + `
				FROM items_fts f
				WHERE items_fts MATCH ?
				ORDER BY ` + rank + `
				LIMIT ?`,
			args: []any{match, limit},
		},
		{
			name: "b +snippet x3",
			sql: `SELECT f.rowid, ` + rank + `,
				snippet(items_fts, 0, char(1), char(2), '…', 18),
				snippet(items_fts, 1, char(1), char(2), '…', 18),
				snippet(items_fts, 2, char(1), char(2), '…', 18)
				FROM items_fts f
				WHERE items_fts MATCH ?
				ORDER BY ` + rank + `
				LIMIT ?`,
			args: []any{match, limit},
		},
		{
			name: "c +EXISTS x3",
			sql: `SELECT f.rowid, ` + rank + `,
				EXISTS(SELECT 1 FROM items_fts WHERE rowid = f.rowid AND items_fts MATCH ?),
				EXISTS(SELECT 1 FROM items_fts WHERE rowid = f.rowid AND items_fts MATCH ?),
				EXISTS(SELECT 1 FROM items_fts WHERE rowid = f.rowid AND items_fts MATCH ?)
				FROM items_fts f
				WHERE items_fts MATCH ?
				ORDER BY ` + rank + `
				LIMIT ?`,
			args: []any{titleMatch, bodyMatch, commentMatch, match, limit},
		},
		{
			name: "d +comments group_concat",
			sql: `SELECT f.rowid, ` + rank + `,
				COALESCE((SELECT group_concat(c.body_text, char(10)) FROM comments c WHERE c.item_id = it.id), '')
				FROM items_fts f
				JOIN items it ON it.rowid = f.rowid
				WHERE items_fts MATCH ?
				ORDER BY ` + rank + `
				LIMIT ?`,
			args: []any{match, limit},
		},
		{
			name: "e +body_text",
			sql: `SELECT f.rowid, ` + rank + `, COALESCE(it.body_text, '')
				FROM items_fts f
				JOIN items it ON it.rowid = f.rowid
				WHERE items_fts MATCH ?
				ORDER BY ` + rank + `
				LIMIT ?`,
			args: []any{match, limit},
		},
		{
			name: "f current full SELECT",
			sql: `SELECT it.kind, COALESCE(it.key, ''), COALESCE(it.title, ''),
				       COALESCE(it.author, ''), COALESCE(it.author_id, ''), COALESCE(it.updated_at, ''), COALESCE(it.url, ''),
				       COALESCE(p.space_key, ''), COALESCE(sp.name, ''), COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''),
				       COALESCE(p.version, 0), COALESCE(p.excerpt, ''), COALESCE(p.labels, '[]'),
				       snippet(items_fts, 0, char(1), char(2), '…', 18),
				       snippet(items_fts, 1, char(1), char(2), '…', 18),
				       snippet(items_fts, 2, char(1), char(2), '…', 18),
				       COALESCE(it.body_text, ''),
				       COALESCE((SELECT group_concat(c.body_text, char(10)) FROM comments c WHERE c.item_id = it.id), ''),
				       EXISTS(SELECT 1 FROM items_fts WHERE rowid = f.rowid AND items_fts MATCH ?),
				       EXISTS(SELECT 1 FROM items_fts WHERE rowid = f.rowid AND items_fts MATCH ?),
				       EXISTS(SELECT 1 FROM items_fts WHERE rowid = f.rowid AND items_fts MATCH ?),
				       ` + rank + `
				FROM items_fts f
				JOIN items it ON it.rowid = f.rowid
				LEFT JOIN pages p ON p.item_id = it.id
				LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key
				WHERE items_fts MATCH ?
				ORDER BY ` + rank + `
				LIMIT ?`,
			args: []any{titleMatch, bodyMatch, commentMatch, match, limit},
		},
		{
			name: "g two-pass (rank then payload)",
			sql: `SELECT it.kind, COALESCE(it.key, ''), COALESCE(it.title, ''),
				       COALESCE(it.author, ''), COALESCE(it.author_id, ''), COALESCE(it.updated_at, ''), COALESCE(it.url, ''),
				       COALESCE(p.space_key, ''), COALESCE(sp.name, ''), COALESCE(sp.homepage_id, ''), COALESCE(p.parent_id, ''),
				       COALESCE(p.version, 0), COALESCE(p.excerpt, ''), COALESCE(p.labels, '[]'),
				       snippet(items_fts, 0, char(1), char(2), '…', 18),
				       snippet(items_fts, 1, char(1), char(2), '…', 18),
				       snippet(items_fts, 2, char(1), char(2), '…', 18),
				       COALESCE(it.body_text, ''),
				       COALESCE((SELECT group_concat(c.body_text, char(10)) FROM comments c WHERE c.item_id = it.id), ''),
				       EXISTS(SELECT 1 FROM items_fts WHERE rowid = ranked.rowid AND items_fts MATCH ?),
				       EXISTS(SELECT 1 FROM items_fts WHERE rowid = ranked.rowid AND items_fts MATCH ?),
				       EXISTS(SELECT 1 FROM items_fts WHERE rowid = ranked.rowid AND items_fts MATCH ?),
				       ranked.rank
				FROM (
					SELECT rowid, ` + rank + ` AS rank
					FROM items_fts
					WHERE items_fts MATCH ?
					ORDER BY ` + rank + `
					LIMIT ?
				) ranked
				JOIN items_fts ON items_fts.rowid = ranked.rowid
				JOIN items it ON it.rowid = ranked.rowid
				LEFT JOIN pages p ON p.item_id = it.id
				LEFT JOIN spaces sp ON sp.source_id = it.source_id AND sp.key = p.space_key`,
			args: []any{titleMatch, bodyMatch, commentMatch, match, limit},
		},
	}
}

func medianSQL(ctx context.Context, sdb *sql.DB, query string, args ...any) (float64, error) {
	return medianFn(func() error {
		rows, err := sdb.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			return err
		}
		dest := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range dest {
			ptrs[i] = &dest[i]
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

// TestSearch20kMedians times store.Search only (no per-stage SQL) so
// before/after tables can be rebuilt without re-paying the EXISTS variants.
func TestSearch20kMedians(t *testing.T) {
	db := seedSearchScale(t, searchScaleN)
	ctx := context.Background()
	shapes := []struct {
		name  string
		q     string
		limit int
	}{
		{"p limit 5", "p", 5},
		{"p limit 20", "p", 20},
		{"p limit 50", "p", 50},
		{"korean 2-char limit 20", "로그", 20},
		{"retry high-hit limit 20", "retry", 20},
		{"0-hit limit 20", "zzznomatchtokenzzz", 20},
		{"key lookup NMB-1", "NMB-1", 20},
	}
	fmt.Printf("\n== store.Search medians (post-fix) ==\n")
	for _, sh := range shapes {
		med, err := medianFn(func() error {
			_, err := db.Search(ctx, sh.q, sh.limit)
			return err
		})
		if err != nil {
			t.Fatalf("%s: %v", sh.name, err)
		}
		res, err := db.Search(ctx, sh.q, sh.limit)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("  %-24s  %7.2f ms  hits=%d\n", sh.name, med, res.Total)
		t.Logf("%s median=%.2fms hits=%d", sh.name, med, res.Total)
	}
}

// search20kBudget is the store.Search wall-time budget for q="p" limit=20 on
// the 20k fixture (worst recorded keystroke shape).
//
// Measured 2026-08-17 on this machine (other Go builds running — not idle):
//
//	pre-fix  (one-shot SELECT + 3 EXISTS MATCH probes): median 1590.85 ms
//	post-fix (rowid+bm25 LIMIT, then payload on 20 rows): median 110.24 ms
//	samples  [109.97 109.98 110.07 110.24 110.58 110.75 111.83]
//
// 110.24 × 1.8 ≈ 198 ms, rounded to 200 ms. The 50 ms product keystroke
// budget is below the FTS `"p"*` MATCH+rank floor (~109 ms over 20k hits);
// items_fts DDL is frozen this round (no prefix= index). FAIL-first ran
// against 50 ms on the unmodified source (1590.85 ms).
const search20kBudget = 200 * time.Millisecond

// TestSearch20kBudget is the recurrence gate for GDK-166. Build-tagged so
// plain `go test ./...` stays fast.
func TestSearch20kBudget(t *testing.T) {
	db := seedSearchScale(t, searchScaleN)
	ctx := context.Background()

	// Warm page/stmt cache.
	if _, err := db.Search(ctx, "p", 20); err != nil {
		t.Fatal(err)
	}

	const samples = 7
	ms := make([]float64, 0, samples)
	var last SearchResult
	for i := 0; i < samples; i++ {
		t0 := time.Now()
		res, err := db.Search(ctx, "p", 20)
		elapsed := time.Since(t0)
		if err != nil {
			t.Fatal(err)
		}
		last = res
		ms = append(ms, float64(elapsed.Microseconds())/1000)
	}
	sort.Float64s(ms)
	med := time.Duration(ms[len(ms)/2] * float64(time.Millisecond))
	t.Logf("Search(p,20) samples_ms=%v median=%.2f hits=%d", ms, ms[len(ms)/2], last.Total)
	if last.Total != 20 {
		t.Fatalf("Search(p, 20) hits=%d, want 20 (a miss would make the budget vacuous)", last.Total)
	}
	if med > search20kBudget {
		t.Fatalf("Search(p, 20) median %s exceeds budget %s (samples_ms=%v hits=%d)",
			med, search20kBudget, ms, last.Total)
	}
}

// BenchmarkSearch20kP is the go-test -bench companion to TestSearch20kBudget.
func BenchmarkSearch20kP(b *testing.B) {
	db := seedSearchScale(b, searchScaleN)
	ctx := context.Background()
	if _, err := db.Search(ctx, "p", 20); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := db.Search(ctx, "p", 20); err != nil {
			b.Fatal(err)
		}
	}
}

func medianFn(fn func() error) (float64, error) {
	const samples = 7
	// Warm the page cache / stmt cache once; not counted.
	if err := fn(); err != nil {
		return 0, err
	}
	ms := make([]float64, 0, samples)
	for i := 0; i < samples; i++ {
		t0 := time.Now()
		if err := fn(); err != nil {
			return 0, err
		}
		ms = append(ms, float64(time.Since(t0).Microseconds())/1000)
	}
	sort.Float64s(ms)
	return ms[len(ms)/2], nil
}
