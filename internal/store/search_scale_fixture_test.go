//go:build searchscale

package store

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

// searchScaleN is the mirror size that opened GDK-166 (20k-item Raycast probe).
const searchScaleN = 20000

// searchScaleSeed pins the PRNG so timings are comparable across runs.
const searchScaleSeed = 42

// seedSearchScale writes n synthetic issues whose text is shaped to reproduce
// the 20k-mirror keystroke cost. Status/type/priority are keyed on ids and
// status_category only (never display names). Deterministic at searchScaleSeed.
//
// Query shapes this fixture is built to exercise:
//   - "p"        — prefix; every row carries at least one p-token
//   - "로그"     — 2-char Korean prefix on a subset of titles
//   - "retry"    — high-hit English token in every body
//   - "zzznomatchtokenzzz" — 0-hit
func seedSearchScale(tb testing.TB, n int) *DB {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "search-scale.db")
	db, err := Open(path)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	if err := db.UpsertSource(context.Background(), Source{
		ID: "jira", Kind: "jira", BaseURL: "https://example.invalid",
	}); err != nil {
		tb.Fatal(err)
	}

	rng := rand.New(rand.NewSource(searchScaleSeed))
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

	// p-tokens so `"p"*` matches the whole set (worst recorded shape).
	pWords := []string{"payment", "project", "payload", "process", "pipeline", "pagination", "parent", "priority", "production", "page"}
	// Long-enough body that COALESCE(body_text) and snippet fallback are not free.
	bodyPara := "The payment pipeline dropped the payload when the parent process restarted in production. "
	commentPara := "Reproduced on the payment page; parent process logs show a payload retry. "

	const batchSize = 500
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for start := 0; start < n; start += batchSize {
		end := start + batchSize
		if end > n {
			end = n
		}
		recs := make([]IssueRecord, 0, end-start)
		for i := start; i < end; i++ {
			proj := projects[i%len(projects)]
			num := i + 1
			key := fmt.Sprintf("%s-%d", proj, num)
			extID := fmt.Sprintf("%d", 10000+i)
			st := statuses[i%len(statuses)]
			ty := types[i%len(types)]
			pr := priorities[i%len(priorities)]
			created := base.Add(time.Duration(i) * time.Hour)
			updated := created.Add(time.Duration(rng.Intn(48)) * time.Hour)

			pw := pWords[i%len(pWords)]
			title := fmt.Sprintf("%s failed for %s on bench issue %d", pw, pWords[(i+3)%len(pWords)], num)
			if i%10 == 0 {
				// 2-char Korean prefix "로그" → `"로그"*` (agglutinative case).
				title = fmt.Sprintf("로그인 실패 %s %d", pw, num)
			}
			body := strings.Repeat(bodyPara, 6) + fmt.Sprintf("retry token-%d %s.", i%97, key)

			rec := IssueRecord{
				Item: Item{
					ID: "jira:" + extID, SourceID: "jira", Kind: "issue", ExternalID: extID,
					Key: key, Title: title, BodyText: body,
					CreatedAt: created.UTC().Format(config.ISOMilli),
					UpdatedAt: updated.UTC().Format(config.ISOMilli),
				},
				Issue: Issue{
					ProjectKey: proj, IssueType: ty.name, IssueTypeID: ty.id,
					Status: st.name, StatusID: st.id, StatusCategory: st.cat,
					Priority: pr,
				},
			}
			if i%4 == 0 {
				cAt := updated.UTC().Format(config.ISOMilli)
				rec.Comments = []Comment{{
					ID: "jira:c-" + extID, ExternalID: "c-" + extID,
					Author: "Ada", BodyText: commentPara + key,
					CreatedAt: cAt, UpdatedAt: cAt,
				}}
			}
			recs = append(recs, rec)
		}
		if _, err := db.UpsertIssues(context.Background(), Batch{
			Categories: categories, Priorities: priorities, Records: recs, Force: true,
		}); err != nil {
			tb.Fatalf("upsert %d-%d: %v", start, end-1, err)
		}
	}
	if _, err := db.sql.ExecContext(context.Background(), `ANALYZE`); err != nil {
		tb.Fatalf("ANALYZE: %v", err)
	}
	return db
}
