package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/midagedev/gadak/internal/store"
)

// Recents live in local.db (ATTACHed as local). They never go to Jira.
// This is the owner of recent-use history (picker ranking). The web
// localStorage helper is a first-paint cache only.

func (s *server) registerRecency(mux *http.ServeMux) {
	mux.HandleFunc("GET "+apiBase+"recents/{$}", s.handleGetRecents)
	mux.HandleFunc("POST "+apiBase+"recents/{$}", s.handlePostRecent)
	mux.HandleFunc("POST "+apiBase+"recents/absorb/{$}", s.handleAbsorbRecents)
}

type recentsDoc struct {
	Items []store.Recent `json:"items"`
}

func (s *server) handleGetRecents(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	items, err := s.db.Recents(r.Context(), kind)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if items == nil {
		items = []store.Recent{}
	}
	writeJSON(w, http.StatusOK, recentsDoc{Items: items})
}

func (s *server) handlePostRecent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	body.Kind = strings.TrimSpace(body.Kind)
	body.Value = strings.TrimSpace(body.Value)
	if body.Kind == "" {
		fail(w, http.StatusBadRequest, "kind_required")
		return
	}
	if body.Value == "" {
		fail(w, http.StatusBadRequest, "value_required")
		return
	}
	item, err := s.db.RecordRecent(r.Context(), body.Kind, body.Value)
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *server) handleAbsorbRecents(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kinds map[string][]string `json:"kinds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if err := s.db.AbsorbRecents(r.Context(), body.Kinds); err != nil {
		serverError(w, r, err)
		return
	}
	items, err := s.db.Recents(r.Context(), "")
	if err != nil {
		serverError(w, r, err)
		return
	}
	if items == nil {
		items = []store.Recent{}
	}
	writeJSON(w, http.StatusOK, recentsDoc{Items: items})
}
