package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/midagedev/gadak/internal/store"
)

// History lives in local.db (ATTACHed as local). It never goes to Jira.
// Search query text is not written to the process log.

func (s *server) handlePostVisit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind string `json:"kind"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	body.Kind = strings.TrimSpace(body.Kind)
	body.Key = strings.TrimSpace(body.Key)
	if body.Kind != store.VisitKindIssue && body.Kind != store.VisitKindPage {
		fail(w, http.StatusBadRequest, "invalid_kind")
		return
	}
	if body.Key == "" {
		fail(w, http.StatusBadRequest, "key_required")
		return
	}
	// The server decides the source: the POST body has no field for it, so a
	// client can never claim to be something other than the UI.
	v, err := s.db.RecordVisit(r.Context(), body.Kind, body.Key, store.VisitSourceUI)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *server) handlePostSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query       string `json:"query"`
		ResultCount int    `json:"result_count"`
		OpenedKind  string `json:"opened_kind"`
		OpenedKey   string `json:"opened_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if body.ResultCount < 0 {
		fail(w, http.StatusBadRequest, "invalid_result_count")
		return
	}
	body.OpenedKind = strings.TrimSpace(body.OpenedKind)
	body.OpenedKey = strings.TrimSpace(body.OpenedKey)
	if (body.OpenedKind == "") != (body.OpenedKey == "") {
		fail(w, http.StatusBadRequest, "opened_pair_required")
		return
	}
	if body.OpenedKind != "" && body.OpenedKind != store.VisitKindIssue && body.OpenedKind != store.VisitKindPage {
		fail(w, http.StatusBadRequest, "invalid_kind")
		return
	}
	// query may be empty (the search box submitted nothing). Do not log it.
	srow, err := s.db.RecordSearch(r.Context(), body.Query, body.ResultCount, body.OpenedKind, body.OpenedKey)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, srow)
}

func (s *server) handlePatchSearch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "invalid_id")
		return
	}
	var body struct {
		OpenedKind string `json:"opened_kind"`
		OpenedKey  string `json:"opened_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	body.OpenedKind = strings.TrimSpace(body.OpenedKind)
	body.OpenedKey = strings.TrimSpace(body.OpenedKey)
	if body.OpenedKind != store.VisitKindIssue && body.OpenedKind != store.VisitKindPage {
		fail(w, http.StatusBadRequest, "invalid_kind")
		return
	}
	if body.OpenedKey == "" {
		fail(w, http.StatusBadRequest, "opened_key_required")
		return
	}
	srow, err := s.db.SetSearchOpened(r.Context(), id, body.OpenedKind, body.OpenedKey)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, srow)
}

func (s *server) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := store.HistoryOpts{
		Kind:   strings.TrimSpace(q.Get("kind")),
		Cursor: strings.TrimSpace(q.Get("cursor")),
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			fail(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		opts.Limit = n
	}
	switch opts.Kind {
	case "", store.VisitKindIssue, store.VisitKindPage, "search":
	default:
		fail(w, http.StatusBadRequest, "invalid_kind")
		return
	}
	page, err := s.db.History(r.Context(), opts)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			fail(w, http.StatusBadRequest, "invalid_cursor")
			return
		}
		serverError(w, r, err)
		return
	}
	if page.Items == nil {
		page.Items = []store.HistoryItem{}
	}
	writeJSON(w, http.StatusOK, page)
}

// visitedKeysCap bounds GET history/visited/. A person who has opened more
// distinct issues than this in the retention window loses the mark on the
// oldest ones, which is the cheap failure; an unbounded answer is not.
const visitedKeysCap = 5000

// handleGetVisited answers "which keys has this browser opened, and when" for
// one kind in a single request (GDK-1344). The history/ timeline pages and
// mixes searches in; a list marking 10k rows needs the folded set, once.
func (s *server) handleGetVisited(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	switch kind {
	case store.VisitKindIssue, store.VisitKindPage:
	default:
		fail(w, http.StatusBadRequest, "invalid_kind")
		return
	}
	items, err := s.db.VisitedKeys(r.Context(), kind, visitedKeysCap)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if items == nil {
		items = []store.RecentVisit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "truncated": len(items) == visitedKeysCap})
}
