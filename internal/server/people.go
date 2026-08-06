package server

import (
	"net/http"
	"strconv"

	"github.com/midagedev/scry/internal/store"
)

// handlePeopleComments answers GET /api/v1/issues/people/{author_id}/comments/?limit=
// Contract: store.CommentsByAuthorResult — author, total, comments[].
// Unknown author_id → 200 with total 0 and empty comments (UI draws an empty header).
func (s *server) handlePeopleComments(w http.ResponseWriter, r *http.Request) {
	authorID := r.PathValue("author_id")
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			fail(w, http.StatusBadRequest, "invalid_limit")
			return
		}
		limit = n
	}
	res, err := s.db.CommentsByAuthor(authorID, limit)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if res.Comments == nil {
		res.Comments = []store.AuthorComment{}
	}
	writeJSON(w, http.StatusOK, res)
}
