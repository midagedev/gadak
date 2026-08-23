package origin

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// TestGDK333FailFirstTwoSessionsInvisible documented the pre-fix defect:
// two constructStandalone graphs on the same persist path did not share
// memory — a create on A was invisible on B, and the last Close silently
// won. GDK-333 closed the happy path by routing; GDK-343 closes the rest:
// the persist lock now makes the second construction impossible at all, so
// the assertion is that B fails with ErrWorkspaceBusy instead of opening
// an invisible sibling graph.
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

	// FAIL-first (2026-08-19, unmodified constructStandalone): this second
	// construction succeeded and B could not see A's issue — two embedded
	// graphs over one file. Since GDK-343 it must fail on the persist lock.
	b, err := constructStandalone(persist, nil, config.ResolvedActor{}, "en")
	if err == nil {
		closeSession(b)
		t.Fatal("second constructStandalone succeeded — the GDK-333/343 double-graph hazard is back")
	}
	if !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("second constructStandalone error = %v, want ErrWorkspaceBusy", err)
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
