package server

// Cross-workspace references on the detail response (GDK-1032). The stored
// pointer is just a URL; what makes it worth showing is hydration — the
// target's current status and assignee, read out of that workspace's own
// mirror file. No network, and never a write: the other workspace is opened
// read-only. The pointer grammar and the mirror read are owned by
// internal/reflink (GDK-1316); this file is the HTTP surface over them.

import (
	"context"
	"time"

	"github.com/midagedev/gadak/internal/reflink"
	"github.com/midagedev/gadak/internal/store"
)

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
	cache := map[string]*reflink.Mirror{}
	for _, l := range links {
		ref := detailRef{ID: l.ID, Relationship: l.Relationship, URL: l.URL, Title: l.Title, Summary: l.Summary}
		if ws, key, ok := reflink.Parse(l.URL); ok {
			ref.Workspace, ref.Key = ws, key
			m, seen := cache[ws]
			if !seen {
				m = reflink.OpenMirror(ws)
				cache[ws] = m
			}
			if m != nil {
				// A foreign mirror is another process's file; a lock wait
				// must not hold the detail request open.
				qctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				if lite, found := m.Lookup(qctx, key); found {
					ref.Summary, ref.Status, ref.Category, ref.Assignee =
						lite.Summary, lite.Status, lite.Category, lite.Assignee
					ref.Hydrated = true
				}
				cancel()
			}
		}
		out = append(out, ref)
	}
	for _, m := range cache {
		m.Close()
	}
	return out
}
