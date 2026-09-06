package server

// GET /api/v1/issues/retro/ — the weekly retrospective document `gadak
// retro` prints, served for the surface that will render it. Read-only, same
// compute, same definitions strings, same per-bucket key sets. Registered
// under apiBase beside bootstrap/, so the whole mirror REST (loopback UI and
// serve-scope Bearer alike, mirror_gate.go serveScopeAdmits) reaches it —
// a top-level /api/v1/retro/ would sit outside every admitted prefix. No
// ETag: the document moves with the wall clock (the partial bucket ends at
// now), so a conditional GET would be a lie half the time.

import (
	"net/http"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

// retroDefaultSince is the endpoint's window when the request names none:
// four ISO weeks, a month of columns. The CLI default (14d) stays the CLI's;
// a panel that wants the CLI's window says so with ?since=14d.
const retroDefaultSince = "4w"

func (s *server) handleRetro(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("since"))
	if raw == "" {
		raw = retroDefaultSince
	}
	since, err := retro.ParseSince(raw)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// session_gap: the same parser and the same sentence as the CLI flag;
	// absent means the 30m default.
	sessionGap := retro.SessionGap
	if gapRaw := strings.TrimSpace(r.URL.Query().Get("session_gap")); gapRaw != "" {
		sessionGap, err = retro.ParseSessionGap(gapRaw)
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// ReadOnly() is the store's own read-only accessor: mode=ro handle with
	// local.db attached, the same view `gadak retro` computes against, so
	// this endpoint cannot take the mirror's write lock either.
	db, err := s.db.ReadOnly()
	if err != nil {
		serverError(w, r, err)
		return
	}
	defer db.Close()
	rep, err := retro.Compute(r.Context(), db, store.FeedIdentityOf(s.config()), since, time.Now(), retro.Options{SessionGap: sessionGap})
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep.JSON())
}
