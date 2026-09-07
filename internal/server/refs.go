package server

// Cross-workspace references on the detail response (GDK-1032). The stored
// pointer is just a URL; what makes it worth showing is hydration — the
// target's current status and assignee, read out of that workspace's own
// mirror file. No network, and never a write: the other workspace is opened
// read-only.

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"database/sql"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// refScheme is the URL form of a cross-workspace pointer. Kept in step with
// cmd/gadak's refScheme — the CLI writes these and the server reads them.
const refScheme = "gadak://"

// detailRef is one reference as the client sees it.
type detailRef struct {
	ID           string `json:"id"`
	Relationship string `json:"relationship,omitempty"`
	URL          string `json:"url"`
	Title        string `json:"title,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	Key          string `json:"key,omitempty"`
	// Live fields, present only when this machine mirrors the target.
	Summary  string `json:"summary,omitempty"`
	Status   string `json:"status,omitempty"`
	Category string `json:"status_category,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	// Hydrated says the live fields were read just now. False means the
	// pointer is fine but this machine does not mirror that workspace —
	// a state the client shows, never an error.
	Hydrated bool `json:"hydrated"`
}

// hydrateRefs turns stored remote links into the client's shape, reading
// each named workspace's mirror at most once per request.
func hydrateRefs(ctx context.Context, links []store.RemoteLink) []detailRef {
	if len(links) == 0 {
		return nil
	}
	out := make([]detailRef, 0, len(links))
	cache := map[string]*refMirror{}
	for _, l := range links {
		ref := detailRef{ID: l.ID, Relationship: l.Relationship, URL: l.URL, Title: l.Title, Summary: l.Summary}
		if ws, key, ok := parseRefURL(l.URL); ok {
			ref.Workspace, ref.Key = ws, key
			m, seen := cache[ws]
			if !seen {
				m = openRefMirror(ws)
				cache[ws] = m
			}
			if m != nil {
				if lite, found := m.lookup(ctx, key); found {
					ref.Summary, ref.Status, ref.Category, ref.Assignee =
						lite.summary, lite.status, lite.category, lite.assignee
					ref.Hydrated = true
				}
			}
		}
		out = append(out, ref)
	}
	for _, m := range cache {
		m.close()
	}
	return out
}

// parseRefURL splits gadak://<workspace>/<KEY>. Any other URL is a plain
// external pointer with nothing local to read.
func parseRefURL(url string) (workspace, key string, ok bool) {
	if !strings.HasPrefix(url, refScheme) {
		return "", "", false
	}
	ws, k, found := strings.Cut(strings.TrimPrefix(url, refScheme), "/")
	if !found || ws == "" || k == "" {
		return "", "", false
	}
	return ws, k, true
}

type refLite struct {
	summary  string
	status   string
	category string
	assignee string
}

// refMirror is one other workspace's mirror, opened read-only for the life
// of a request. nil means "not available here" — a missing workspace, a
// mirror that was never synced, or a file this process may not read. None
// of those is an error: the pointer stands, it just has no live half.
type refMirror struct {
	db   *sql.DB
	once sync.Once
}

func openRefMirror(workspace string) *refMirror {
	path, err := config.DBPathFor(workspace)
	if err != nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	db, err := store.OpenReadOnly(path)
	if err != nil {
		return nil
	}
	return &refMirror{db: db}
}

func (m *refMirror) lookup(ctx context.Context, key string) (refLite, bool) {
	if m == nil || m.db == nil {
		return refLite{}, false
	}
	// A foreign mirror is another process's file; a lock wait must not hold
	// the detail request open.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var lite refLite
	err := m.db.QueryRowContext(ctx, `
		SELECT COALESCE(summary,''), COALESCE(status,''), COALESCE(status_category,''),
		       COALESCE(assignee,'')
		FROM issues_full WHERE key = ? LIMIT 1`, key).
		Scan(&lite.summary, &lite.status, &lite.category, &lite.assignee)
	if err != nil {
		return refLite{}, false
	}
	return lite, true
}

func (m *refMirror) close() {
	if m == nil || m.db == nil {
		return
	}
	m.once.Do(func() { _ = m.db.Close() })
}
