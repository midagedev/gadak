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
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

// retroDefaultSince is the endpoint's window when the request names none:
// four ISO weeks, a month of columns. The CLI default (14d) stays the CLI's;
// a panel that wants the CLI's window says so with ?since=14d.
const retroDefaultSince = "4w"

// retroBoardJSON is one board of the 409 ambiguous-board body: the rows a
// picker needs, in the shape boards/ serves them.
type retroBoardJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

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
	// absent means the config default (retro.sessionGap, itself 30m unset).
	sessionGap := retro.SessionGap
	if gapRaw := strings.TrimSpace(r.URL.Query().Get("session_gap")); gapRaw != "" {
		sessionGap, err = retro.ParseSessionGap(gapRaw)
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if v := strings.TrimSpace(s.config().EffectiveRetroSessionGap()); v != "" {
		d, gerr := retro.ParseSessionGap(v)
		if gerr != nil {
			fail(w, http.StatusBadRequest, fmt.Sprintf("config retro.sessionGap: %v", gerr))
			return
		}
		sessionGap = d
	}
	// ReadOnly() is the store's own read-only accessor: mode=ro handle with
	// local.db attached, the same view `gadak retro` computes against, so
	// this endpoint cannot take the mirror's write lock either.
	// by=sprint cuts the report by sprint window instead of ISO week
	// (GDK-1693). Anything else is a typo, not a third mode: answering a
	// misspelled `by` with weekly columns would look like the parameter
	// worked.
	opts := retro.Options{SessionGap: sessionGap}
	switch by := strings.TrimSpace(r.URL.Query().Get("by")); by {
	case "", "week":
	case "sprint":
		opts.BySprint = true
	default:
		fail(w, http.StatusBadRequest, fmt.Sprintf("by wants week or sprint (got %q)", by))
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("board")); raw != "" {
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil {
			fail(w, http.StatusBadRequest, "board wants a board id")
			return
		}
		opts.BoardID = id
	}
	db, err := s.db.ReadOnly()
	if err != nil {
		serverError(w, r, err)
		return
	}
	defer db.Close()
	rep, err := retro.Compute(r.Context(), db, store.FeedIdentityOf(s.config()), since, time.Now(), opts)
	if err != nil {
		// A workspace with no sprints, or several boards and no choice, is
		// the caller asking for a report that cannot be built — 409, with
		// the sentence the error already carries, not a 500.
		//
		// The body carries a code and, for the ambiguous case, the board
		// list itself (GDK-1713). `err.Error()` is the CLI's sentence — it
		// names `--board`, a flag no web reader has — so a surface with a
		// picker needs the rows, not the prose. The message stays for the
		// callers that only have a line to print.
		var amb *retro.ErrAmbiguousBoard
		if errors.As(err, &amb) {
			boards := make([]retroBoardJSON, 0, len(amb.Boards))
			for _, b := range amb.Boards {
				boards = append(boards, retroBoardJSON{ID: b.ID, Name: b.Name})
			}
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "ambiguous_board",
				"message": err.Error(),
				"boards":  boards,
			})
			return
		}
		if errors.Is(err, retro.ErrNoSprints) {
			failMsg(w, http.StatusConflict, "no_sprints", err.Error())
			return
		}
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep.JSON())
}
