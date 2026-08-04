package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// seedBenchDB writes n synthetic issues with a fixed PRNG seed so timings are
// comparable across runs. Mirrors tools/bench-fixture without a subprocess.
func seedBenchDB(tb testing.TB, n int, seed int64) (*store.DB, *config.Config) {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "bench.db")
	db, err := store.Open(path)
	if err != nil {
		tb.Fatalf("open: %v", err)
	}
	tb.Cleanup(func() { db.Close() })

	if err := db.UpsertSource(store.Source{
		ID: "jira", Kind: "jira", BaseURL: "https://example.invalid",
	}); err != nil {
		tb.Fatalf("source: %v", err)
	}

	rng := rand.New(rand.NewSource(seed))
	categories := map[string]string{"1": "new", "3": "inprogress", "5": "done"}
	priorities := []string{"Highest", "High", "Medium", "Low", "Lowest"}
	statuses := []struct{ name, id, cat string }{
		{"To Do", "1", "new"},
		{"In Progress", "3", "inprogress"},
		{"Done", "5", "done"},
	}
	projects := []string{"NMB", "NMA", "NMS"}
	types := []struct{ name, id string }{
		{"Bug", "10004"}, {"Task", "10002"}, {"Story", "10001"},
	}
	emptyADF := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	vocab := []string{"webhook", "gateway", "invoice", "retry", "timeout", "cache"}

	const batchSize = 250
	for start := 0; start < n; start += batchSize {
		end := start + batchSize
		if end > n {
			end = n
		}
		recs := make([]store.IssueRecord, 0, end-start)
		for i := start; i < end; i++ {
			proj := projects[i%len(projects)]
			num := i + 1
			key := fmt.Sprintf("%s-%d", proj, num)
			extID := fmt.Sprintf("%d", 10000+i)
			st := statuses[rng.Intn(len(statuses))]
			ty := types[rng.Intn(len(types))]
			pr := priorities[rng.Intn(len(priorities))]
			created := base.Add(time.Duration(i) * time.Hour)
			updated := created.Add(time.Duration(rng.Intn(48)) * time.Hour)
			title := fmt.Sprintf("bench issue %d about %s", num, vocab[rng.Intn(len(vocab))])
			if i == 0 {
				title = "benchneedle primary search target"
			}
			body := fmt.Sprintf("Synthetic body for %s payload-%d.", key, i%97)
			rec := store.IssueRecord{
				Item: store.Item{
					ID: "jira:" + extID, SourceID: "jira", Kind: "issue", ExternalID: extID,
					Key: key, Title: title, BodyText: body,
					CreatedAt: created.UTC().Format("2006-01-02T15:04:05.000Z"),
					UpdatedAt: updated.UTC().Format("2006-01-02T15:04:05.000Z"),
				},
				Issue: store.Issue{
					ProjectKey: proj, IssueType: ty.name, IssueTypeID: ty.id,
					Status: st.name, StatusID: st.id, StatusCategory: st.cat,
					Priority: pr, DescriptionADF: emptyADF,
				},
			}
			if i == 0 || rng.Intn(100) < 25 {
				cBody := "comment text " + vocab[rng.Intn(len(vocab))]
				if i == 0 {
					cBody = "comment carries benchneedle for fts"
				}
				cAt := updated.UTC().Format("2006-01-02T15:04:05.000Z")
				rec.Comments = []store.Comment{{
					ID: "jira:c-" + extID, ExternalID: "c-" + extID,
					Author: "Ada", BodyADF: emptyADF, BodyText: cBody,
					CreatedAt: cAt, UpdatedAt: cAt,
				}}
			}
			recs = append(recs, rec)
		}
		if _, err := db.UpsertIssues(store.Batch{
			Categories: categories, Priorities: priorities, Records: recs, Force: true,
		}); err != nil {
			tb.Fatalf("upsert %d-%d: %v", start, end-1, err)
		}
	}

	cfg := &config.Config{
		Site: "https://example.invalid", Projects: []string{"NMB", "NMA", "NMS"},
	}
	return db, cfg
}

// TestBenchSmoke1k is a short CI-friendly check that the bench fixture path
// produces a working bootstrap and FTS hit. Not a latency gate.
func TestBenchSmoke1k(t *testing.T) {
	db, cfg := seedBenchDB(t, 1000, 42)
	h := New(db, cfg)

	rec := get(t, h, apiBase+"bootstrap/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bootstrap status %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[bootstrapResponse](t, rec)
	if len(body.Issues) != 1000 {
		t.Fatalf("bootstrap issues %d, want 1000", len(body.Issues))
	}

	got := decode[struct {
		Keys  []string `json:"keys"`
		Total int      `json:"total"`
	}](t, get(t, h, apiBase+"search/?q=benchneedle&limit=10", nil))
	if got.Total < 1 || len(got.Keys) < 1 {
		t.Fatalf("search miss: %+v", got)
	}
}

// BenchmarkBootstrap10k measures a full bootstrap response over 10k issues.
// The 50 ms / 10k product target is recorded in gates.md from make bench output;
// this benchmark never fails the suite on wall time (machine variance).
func BenchmarkBootstrap10k(b *testing.B) {
	db, cfg := seedBenchDB(b, 10000, 42)
	h := New(db, cfg)

	// Warm once so the first timed iteration is not cold-cache dominated.
	req := httptest.NewRequest(http.MethodGet, apiBase+"bootstrap/", nil)
	warm := httptest.NewRecorder()
	h.ServeHTTP(warm, req)
	if warm.Code != http.StatusOK {
		b.Fatalf("warm bootstrap %d", warm.Code)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
		if rec.Body.Len() < 1000 {
			b.Fatalf("suspiciously small body: %d bytes", rec.Body.Len())
		}
	}
}

// BenchmarkSearch10k measures an FTS search over the 10k fixture.
func BenchmarkSearch10k(b *testing.B) {
	db, cfg := seedBenchDB(b, 10000, 42)
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodGet, apiBase+"search/?q=benchneedle&limit=50", nil)
	warm := httptest.NewRecorder()
	h.ServeHTTP(warm, req)
	if warm.Code != http.StatusOK {
		b.Fatalf("warm search %d", warm.Code)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}
