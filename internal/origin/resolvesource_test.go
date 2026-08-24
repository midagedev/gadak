package origin_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// ResolveCreateSource is the create-side router both write surfaces call —
// CLI withCreateSession and REST createWriter (GDK-820). These tests pin the
// routing rule at its single owner so the two callers cannot drift apart
// again; the copies they used to carry are deleted.

const seedStamp = "2026-08-01T00:00:00.000Z"

func seedProject(t *testing.T, db *store.DB, src, project string) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), store.Source{ID: src, Kind: src}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: src + ":p1", SourceID: src, Kind: "issue", ExternalID: "p1",
				Key: project + "-1", Title: "seed row", CreatedAt: seedStamp, UpdatedAt: seedStamp,
			},
			Issue: store.Issue{
				ProjectKey: project, IssueType: "Issue", IssueTypeID: "issue",
				Status: "Todo", StatusID: "1", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func openMirror(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestResolveCreateSourceRoutes(t *testing.T) {
	ctx := context.Background()
	// A connected workspace that also has a Linear key: both origins exist,
	// so only the mirror (or a Linear-only workspace) may pick Linear.
	both := &config.Config{
		Site: "https://example.atlassian.net", Email: "e@example.com", Token: "t",
		Linear: &config.LinearConfig{APIKey: "k"},
	}
	linearOnly := &config.Config{Linear: &config.LinearConfig{APIKey: "k"}}

	db := openMirror(t)
	seedProject(t, db, "linear", "MID")

	if src, err := origin.ResolveCreateSource(ctx, both, db, "MID"); err != nil || src != "linear" {
		t.Fatalf("mirrored linear project = (%q, %v), want linear", src, err)
	}
	// Unknown project with both credentials: default Jira-family origin.
	if src, err := origin.ResolveCreateSource(ctx, both, db, "NOPE"); err != nil || src != "" {
		t.Fatalf("unknown project, both credentials = (%q, %v), want empty", src, err)
	}
	// Linear-only workspace routes to Linear before any team is mirrored.
	if src, err := origin.ResolveCreateSource(ctx, linearOnly, db, "NOPE"); err != nil || src != "linear" {
		t.Fatalf("linear-only workspace = (%q, %v), want linear", src, err)
	}
	// The db guard: a nil mirror (no read handle) still answers by credential.
	if src, err := origin.ResolveCreateSource(ctx, linearOnly, nil, "MID"); err != nil || src != "linear" {
		t.Fatalf("nil db, linear-only = (%q, %v), want linear", src, err)
	}
	// Whitespace-only project is the same as no project.
	if src, err := origin.ResolveCreateSource(ctx, both, db, "  "); err != nil || src != "" {
		t.Fatalf("blank project = (%q, %v), want empty", src, err)
	}
}

func TestResolveCreateSourceRefusesAmbiguousProject(t *testing.T) {
	ctx := context.Background()
	// Same shape as KeySource's collision refusal: a project minted by both
	// origins must not be routed by luck. The REST caller maps this error to
	// 409; the routing owner just has to surface it untouched.
	db := openMirror(t)
	seedProject(t, db, "linear", "AMB")
	seedProject(t, db, "jira", "AMB")

	_, err := origin.ResolveCreateSource(ctx, &config.Config{}, db, "AMB")
	if !errors.Is(err, store.ErrKeyAmbiguous) {
		t.Fatalf("ambiguous project err = %v, want ErrKeyAmbiguous", err)
	}
}
