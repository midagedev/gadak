package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

// GDK-1691, measured on a paired home serve: its issuetap predates the
// built-in tracker's sprints (GDK-1666) and answers 501 on the agile board
// route. The mirror-side code is all present — only the origin cannot serve
// it — so what the sync owes the user is a sentence that says whose
// limitation it is, that the issue rows are fine, and the one action that
// closes it. The bare `boards: skipped (…501…)` the catch-all used to print
// reads like a flaky failure instead.
//
// FAIL-first on the pre-fix tree: the log carried the 501 but none of the
// three facts, and Boards() returned a wrapped APIError no caller could
// branch on.
func TestAgile501TeachesTheWayOutInsteadOfSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/agile/1.0/board" {
			http.Error(w, "not implemented", http.StatusNotImplemented)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.Error(w, "no", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0 // no sleeping in tests

	var lines []string
	db := newMirror(t)
	importAgile(context.Background(), client, nil, db.DB, Options{Log: func(line string) { lines = append(lines, line) }})

	if len(lines) == 0 {
		t.Fatal("importAgile logged nothing — the 501 must be said out loud, not swallowed")
	}
	joined := strings.Join(lines, "\n")
	// The three facts, one assertion each so a failure names which is gone.
	if !strings.Contains(joined, "501") {
		t.Errorf("log does not state the status: %q", joined)
	}
	if !strings.Contains(joined, "predates") {
		t.Errorf("log does not name whose limitation it is (the serve's age, not the mirror's): %q", joined)
	}
	if !strings.Contains(joined, "sync --full") {
		t.Errorf("log does not teach the action that closes it: %q", joined)
	}
	// And the mirror is not a failed sync: no boards row was invented for an
	// origin that could not list any, and none of the log lines reads as a
	// store failure.
	var n int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM boards`).Scan(&n); err != nil {
		t.Fatalf("boards table: %v", err)
	}
	if n != 0 {
		t.Errorf("boards rows = %d, want 0 — a 501 lists nothing, it does not fabricate", n)
	}
}
