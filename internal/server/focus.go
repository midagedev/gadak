package server

import (
	"net/http"

	"github.com/midagedev/gadak/internal/uifocus"
)

// GET ui-focus/ — one-shot hash the CLI left for this profile's UI.
// 204 when nothing is pending. The file is consumed on read.
func (s *server) handleUIFocus(w http.ResponseWriter, r *http.Request) {
	hash, ok, err := uifocus.TakeFor(s.profile)
	if err != nil {
		serverError(w, r, err)
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"hash": hash})
}
