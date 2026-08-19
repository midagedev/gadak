package server

import (
	"log"
	"net/http"

	"github.com/midagedev/gadak/internal/origin"
)

// handleOriginREST forwards method, path, query, headers, and body to this
// process's embedded issuetap handler. It is not a mirror-write API: the
// SQLite mirror is never the target. Writes go through the workspace origin
// (issuetap here, Jira on a connected workspace — which 404s below). Loopback
// single-user model: no extra auth (decision 0003).
func (s *server) handleOriginREST(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	if cfg == nil || !cfg.IsStandalone() {
		handleNotFound(w, r)
		return
	}
	h := s.standaloneOrigin()
	if h == nil {
		log.Printf("server: origin passthrough unavailable")
		fail(w, http.StatusBadGateway, "origin_unavailable")
		return
	}
	http.StripPrefix(origin.RESTPrefix, h).ServeHTTP(w, r)
}

func (s *server) standaloneOrigin() http.Handler {
	if s.originH != nil {
		return s.originH
	}
	s.originOnce.Do(func() {
		if s.originH != nil {
			return
		}
		h, err := origin.StandaloneHandler(s.config())
		if err != nil {
			log.Printf("server: origin passthrough: %v", err)
			return
		}
		s.originH = h
	})
	return s.originH
}

// BindOriginHandler pins the passthrough target. Tests use it so they can
// evict origin.live (simulating a second process) without reconstructing
// a second issuetap graph on the next request.
func (h *Handler) BindOriginHandler(next http.Handler) {
	if h == nil || h.s == nil {
		return
	}
	h.s.originH = next
}
