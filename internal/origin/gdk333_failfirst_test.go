package origin

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

// TestGDK333FailFirstTwoSessionsInvisible is the pre-fix shape of the
// defect: two constructStandalone graphs on the same persist path do not
// share memory. A create on A is invisible on B. After the routing fix
// this remains true of two embeddings — the fix is that the CLI no longer
// constructs B when a live serve owns A. The assertion is the inverse of
// what we want from a routed Client (visibility); it documents the hazard.
func TestGDK333FailFirstTwoSessionsInvisible(t *testing.T) {
	persist := filepath.Join(t.TempDir(), "origin", "issuetap.yaml")
	a, err := constructStandalone(persist)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(a) })
	b, err := constructStandalone(persist)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(b) })

	ctx := context.Background()
	key, err := a.client.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": DefaultProjectKey},
		"summary":   "gdk-333 two-session hazard",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("CreateIssue on A: %v", err)
	}
	if !strings.HasPrefix(key, DefaultProjectKey+"-") {
		t.Fatalf("key %q", key)
	}

	found := searchKey(t, b.client, key)
	if found {
		t.Fatalf("session B unexpectedly sees %s — two embeddings now share memory; the routing fix needs revisit", key)
	}
	// FAIL-first (2026-08-19, unmodified constructStandalone):
	// "FAIL-first GDK-333: issue STD-1 created on session A is invisible on session B (two embedded graphs)"
	t.Logf("GDK-333 hazard still holds: %s created on A is invisible on B", key)
}

func searchKey(t *testing.T, c *jira.Client, key string) bool {
	t.Helper()
	found := false
	err := c.Search(context.Background(), `key = "`+key+`"`, []string{"summary"}, false, func(issues []jira.Issue) error {
		for _, iss := range issues {
			if iss.Key == key {
				found = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	return found
}
