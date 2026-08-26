package origin

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// TestGDK333FailFirstTwoSessionsInvisible is the F′ stage 1 seam: two
// constructStandalone sessions on the same persist path share writes
// through WAL. The persist lock is gone (GDK-936). FAIL-first (lock
// no-op, 2026-08-26): the previous busy assertion went red because the
// second construct succeeded, and B already saw A's issue.
func TestGDK333FailFirstTwoSessionsInvisible(t *testing.T) {
	persist := filepath.Join(t.TempDir(), filepath.FromSlash(PersistRel))
	a, err := constructStandalone(persist, nil, config.ResolvedActor{}, "en")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeSession(a) })

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

	b, err := constructStandalone(persist, nil, config.ResolvedActor{}, "en")
	if err != nil {
		t.Fatalf("second constructStandalone: %v", err)
	}
	t.Cleanup(func() { closeSession(b) })
	if !searchKey(t, b.client, key) {
		t.Fatalf("issue %s missing on second session — WAL did not share the write", key)
	}
	if !searchKey(t, a.client, key) {
		t.Fatalf("issue %s missing on its own session", key)
	}
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
