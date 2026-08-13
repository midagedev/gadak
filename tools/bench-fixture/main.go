// Command bench-fixture builds a deterministic synthetic gadak.db for latency
// benchmarks (T6.7 / G5). No network: it only drives internal/store.
//
//	go run ./tools/bench-fixture -out /tmp/bench.db -issues 10000
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

func main() {
	out := flag.String("out", "", "path for the gadak.db to create (required)")
	n := flag.Int("issues", 10000, "number of issues to generate")
	seed := flag.Int64("seed", 42, "PRNG seed for reproducible fixtures")
	batchSize := flag.Int("batch", 250, "issues per UpsertIssues transaction")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "usage: bench-fixture -out <path> [-issues 10000] [-seed 42]")
		os.Exit(2)
	}
	if *n < 1 {
		fmt.Fprintln(os.Stderr, "-issues must be >= 1")
		os.Exit(2)
	}
	if err := generate(*out, *n, *seed, *batchSize); err != nil {
		fmt.Fprintf(os.Stderr, "bench-fixture: %v\n", err)
		os.Exit(1)
	}
}

func generate(path string, n int, seed int64, batchSize int) error {
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")

	db, err := store.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.UpsertSource(context.Background(), store.Source{
		ID: "jira", Kind: "jira", BaseURL: "https://example.invalid",
	}); err != nil {
		return err
	}

	rng := rand.New(rand.NewSource(seed))
	categories := map[string]string{
		"1": "new", "3": "inprogress", "5": "done",
	}
	priorities := []string{"Highest", "High", "Medium", "Low", "Lowest"}
	statuses := []struct {
		name, id, cat string
	}{
		{"To Do", "1", "new"},
		{"In Progress", "3", "inprogress"},
		{"Done", "5", "done"},
	}
	projects := []string{"NMB", "NMA", "NMS"}
	types := []struct{ name, id string }{
		{"Bug", "10004"}, {"Task", "10002"}, {"Story", "10001"},
	}
	assignees := []struct{ name, id string }{
		{"Ada Lovelace", "acc-ada"},
		{"Grace Hopper", "acc-grace"},
		{"Alan Turing", "acc-alan"},
		{"", ""}, // unassigned
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	emptyADF := json.RawMessage(`{"type":"doc","version":1,"content":[]}`)

	if batchSize < 1 {
		batchSize = 250
	}

	written := 0
	for start := 0; start < n; start += batchSize {
		end := start + batchSize
		if end > n {
			end = n
		}
		recs := make([]store.IssueRecord, 0, end-start)
		for i := start; i < end; i++ {
			recs = append(recs, makeRecord(rng, i, n, base, projects, types, statuses, priorities, assignees, emptyADF))
		}
		batch := store.Batch{
			Categories: categories,
			Priorities: priorities,
			Records:    recs,
			Force:      true,
		}
		got, err := db.UpsertIssues(context.Background(), batch)
		if err != nil {
			return fmt.Errorf("upsert batch %d-%d: %w", start, end-1, err)
		}
		written += got
	}

	if err := db.RecordSync(context.Background(), "jira", store.SyncResult{
		Watermark: base.Add(time.Duration(n) * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z"),
		FullSync:  true,
	}); err != nil {
		return fmt.Errorf("record sync: %w", err)
	}

	fmt.Printf("wrote %d issues to %s (seed=%d)\n", written, path, seed)
	return nil
}

func makeRecord(
	rng *rand.Rand,
	i, n int,
	base time.Time,
	projects []string,
	types []struct{ name, id string },
	statuses []struct{ name, id, cat string },
	priorities []string,
	assignees []struct{ name, id string },
	emptyADF json.RawMessage,
) store.IssueRecord {
	proj := projects[i%len(projects)]
	num := i + 1
	key := fmt.Sprintf("%s-%d", proj, num)
	extID := fmt.Sprintf("%d", 10000+i)
	itemID := "jira:" + extID

	st := statuses[rng.Intn(len(statuses))]
	ty := types[rng.Intn(len(types))]
	pr := priorities[rng.Intn(len(priorities))]
	as := assignees[rng.Intn(len(assignees))]

	created := base.Add(time.Duration(i) * time.Hour)
	updated := created.Add(time.Duration(rng.Intn(72)) * time.Hour)
	// Stable unique terms so FTS search hits are predictable.
	// Issue 0 always contains "benchneedle" in title; a fraction get comment hits.
	title := fmt.Sprintf("bench issue %d about %s", num, word(rng))
	if i == 0 {
		title = "benchneedle primary search target"
	}
	body := fmt.Sprintf("Synthetic body for %s. Keywords: %s %s payload-%d.",
		key, word(rng), word(rng), i%97)

	rec := store.IssueRecord{
		Item: store.Item{
			ID: itemID, SourceID: "jira", Kind: "issue", ExternalID: extID,
			Key: key, Title: title, BodyText: body,
			Author: "Reporter", AuthorID: "acc-reporter",
			URL:       "https://example.invalid/browse/" + key,
			CreatedAt: created.UTC().Format("2006-01-02T15:04:05.000Z"),
			UpdatedAt: updated.UTC().Format("2006-01-02T15:04:05.000Z"),
		},
		Issue: store.Issue{
			ProjectKey: proj, IssueType: ty.name, IssueTypeID: ty.id,
			Status: st.name, StatusID: st.id, StatusCategory: st.cat,
			Priority: pr, Assignee: as.name, AssigneeID: as.id,
			Reporter: "Reporter", ReporterID: "acc-reporter",
			Labels:         pickLabels(rng),
			DescriptionADF: emptyADF,
		},
	}

	// ~30% of issues get a comment; ~20% get a short changelog.
	if rng.Intn(100) < 30 || i == 0 {
		cBody := fmt.Sprintf("Comment on %s: %s", key, word(rng))
		if i == 0 {
			cBody = "comment carries benchneedle for fts"
		}
		cAt := updated.Add(-time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
		rec.Comments = []store.Comment{{
			ID: "jira:c-" + extID, ExternalID: "c-" + extID,
			Author: "Ada Lovelace", AuthorID: "acc-ada",
			BodyADF: emptyADF, BodyText: cBody,
			CreatedAt: cAt, UpdatedAt: cAt,
		}}
	}
	if rng.Intn(100) < 20 {
		from := statuses[0]
		to := st
		if to.id == from.id {
			to = statuses[1]
		}
		rec.Changelog = []store.ChangeEntry{{
			ID: "jira:h-" + extID, At: updated.UTC().Format("2006-01-02T15:04:05.000Z"),
			Author: "Ada Lovelace", Field: "status",
			FromValue: from.name, FromID: from.id,
			ToValue: to.name, ToID: to.id,
		}}
	}
	_ = n
	return rec
}

var vocab = []string{
	"webhook", "gateway", "invoice", "retry", "timeout", "cache", "schema",
	"migration", "auth", "session", "queue", "worker", "batch", "import",
	"export", "parity", "latency", "throttle", "quota", "mirror",
}

func word(rng *rand.Rand) string {
	return vocab[rng.Intn(len(vocab))]
}

func pickLabels(rng *rand.Rand) []string {
	n := rng.Intn(3)
	if n == 0 {
		return nil
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, word(rng))
	}
	return out
}
