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
	v, err := s.db.RecordVisit(r.Context(), body.Kind, body.Key)
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
