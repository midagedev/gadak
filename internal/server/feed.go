package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/midagedev/scry/internal/store"
)

// handleGetFeed answers GET /api/v1/issues/feed/?focus=&limit=
// Contract: web/src/lib/types.ts FeedResponse — items + unread_counts.
func (s *server) handleGetFeed(w http.ResponseWriter, r *http.Request) {
	focus := store.FeedFocus(strings.TrimSpace(r.URL.Query().Get("focus")))
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			fail(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = n
	}
	res, err := s.db.Feed(store.FeedOpts{
		Focus: focus,
		Limit: limit,
		Me:    s.feedIdentity(),
	})
	if err != nil {
		serverError(w, r, err)
		return
	}
	if res.Items == nil {
		res.Items = []store.FeedItem{}
	}
	writeJSON(w, http.StatusOK, res)
}

// handleMarkFeedRead answers POST /api/v1/issues/feed/read/
// body: {event_ids?: string[], issue_keys?: string[], all?: boolean}
func (s *server) handleMarkFeedRead(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EventIDs  []string `json:"event_ids"`
		IssueKeys []string `json:"issue_keys"`
		All       bool     `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if !body.All && len(body.EventIDs) == 0 && len(body.IssueKeys) == 0 {
		fail(w, http.StatusBadRequest, "nothing_to_mark")
		return
	}
	res, err := s.db.MarkFeedRead(store.MarkFeedReadOpts{
		EventIDs:  body.EventIDs,
		IssueKeys: body.IssueKeys,
		All:       body.All,
		Me:        s.feedIdentity(),
	})
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) feedIdentity() store.FeedIdentity {
	cfg := s.config()
	return store.FeedIdentity{
		AccountID:   cfg.AccountID,
		Email:       cfg.Email,
		DisplayName: cfg.TokenOwner,
	}
}
