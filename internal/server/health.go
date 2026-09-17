package server

import (
	"context"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

type syncSource struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Status   string  `json:"status"`
	SyncedAt *string `json:"synced_at"`
	Message  string  `json:"message"`
}

type syncHealth struct {
	Overall     string             `json:"overall"`
	CheckedAt   string             `json:"checked_at"`
	Sources     []syncSource       `json:"sources"`
	TokenExpiry config.TokenExpiry `json:"token_expiry"`
}

/* ── sync health ── */

// health maps the store's sync bookkeeping onto the shape the sidebar renders.
// The message is server text in one language — English, like every other string
// this process emits — and the client localizes only the status label. "ok" is
// not decoration: the client suppresses the tooltip line for exactly that value
// (web/src/components/sidebar/SidebarNav.svelte).
//
// Staleness is measured from the last run that finished without an error, never
// from the watermark: a quiet project leaves its watermark in the past forever
// and would read as permanently delayed.
//
// st is the Jira sync_state (bootstrap/delta still pass only that). Confluence
// is appended when a sources row exists — bootstrap/delta payloads stay issue-
// only and their ETags stay jira-version-only.
func (s *server) health(ctx context.Context, st store.SyncState) syncHealth {
	sources := []syncSource{s.sourceHealth("jira", "Jira", st)}
	if ok, err := s.db.HasSource(ctx, "confluence"); err == nil && ok {
		if cst, err := s.db.SyncState(ctx, "confluence"); err == nil {
			sources = append(sources, s.sourceHealth("confluence", "Confluence", cst))
		}
	}
	overall := "healthy"
	for _, src := range sources {
		switch src.Status {
		case "failed":
			overall = "failed"
		case "missing", "stale":
			if overall == "healthy" {
				overall = "warning"
			}
		}
	}
	// Token expiry is computed here (one owner: config.AssessTokenExpiry) and
	// attached so the chip can warn without a second clock. It does not change
	// overall: settledLabel treats overall==warning as a delayed *mirror*,
	// which an expiring token is not.
	//
	// The read below stays on time.Now, deliberately outside s.now(): the
	// Jira token expires in real time, and a pinned fixture clock must not
	// hide it (the exception and its reason live in wallClockAllowlist,
	// clock_gate_test.go).
	return syncHealth{
		Overall:     overall,
		CheckedAt:   store.Now(),
		Sources:     sources,
		TokenExpiry: s.config().TokenExpiryAt(time.Now().UTC()),
	}
}

// HealthzClock is the /healthz "clock" member (GDK-1975): which clock the
// request path runs on, and the instant it reads — so a harness serving a
// pinned fixture can tell, and a human debugging "why is Sprint 42 still
// running" can see the serve is answering as of the fixture's date.
// ISOMilli is the same RFC3339 layout every other timestamp here carries.
func HealthzClock() map[string]any {
	serverClock()
	if clockAt != nil {
		return map[string]any{"source": "pinned", "now": clockAt.UTC().Format(config.ISOMilli)}
	}
	return map[string]any{"source": "wall", "now": time.Now().UTC().Format(config.ISOMilli)}
}

func (s *server) sourceHealth(key, label string, st store.SyncState) syncSource {
	src := syncSource{Key: key, Label: label, Status: "healthy", Message: "ok", SyncedAt: st.SyncedAt}
	switch {
	case st.LastError != nil && *st.LastError != "":
		src.Status, src.Message = "failed", *st.LastError
	case st.SyncedAt == nil && st.Watermark == "" && st.LastFullSyncAt == nil:
		src.Status, src.Message = "missing", "not synced yet"
	case s.stale(st.SyncedAt):
		src.Status, src.Message = "stale", "last sync is behind"
	}
	if src.SyncedAt == nil {
		src.SyncedAt = st.LastFullSyncAt
	}
	return src
}

// stale allows a wide margin — ten missed incremental runs — because this only
// has to catch a sync loop that died without recording an error.
func (s *server) stale(syncedAt *string) bool {
	if syncedAt == nil {
		return false
	}
	at, err := time.Parse(time.RFC3339, *syncedAt)
	if err != nil {
		return false
	}
	interval := time.Duration(s.config().SyncIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	return time.Since(at) > 10*interval
}
