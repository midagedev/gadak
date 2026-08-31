package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/sync"
)

// Issue links (GDK-85). Catalog GET and create POST share the CLI's
// gadak link path (cmd/gadak/link.go): AsIssueLinker → IssueLinkTypes →
// resolve token → LinkIssues → refresh B then A. There is no delete on
// origin.IssueLinker (only IssueLinkTypes + LinkIssues), so this file
// does not open a delete route.

func (s *server) handleLinkTypes(w http.ResponseWriter, r *http.Request) {
	c, _, _, ok := s.keyWriter(w, r, r.PathValue("key"))
	if !ok {
		return
	}
	linker, err := origin.AsIssueLinker(c)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	list, err := linker.IssueLinkTypes(r.Context())
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	out := make([]map[string]string, 0, len(list))
	for _, t := range list {
		out = append(out, map[string]string{
			"id": t.ID, "name": t.Name, "inward": t.Inward, "outward": t.Outward,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"link_types": out})
}

func (s *server) handleLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	token := strings.TrimSpace(body.Type)
	if token == "" {
		fail(w, http.StatusBadRequest, "type_required")
		return
	}
	rawB := strings.TrimSpace(body.Key)
	b := strings.ToUpper(rawB)
	if rawB == "" {
		fail(w, http.StatusBadRequest, "key_required")
		return
	}
	if !fields.IssueKeyLiteral(b) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("key %q is not a Jira key (want ABC-123)", rawB),
		})
		return
	}
	key := r.PathValue("key")
	a := strings.ToUpper(strings.TrimSpace(key))
	if a == b {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("cannot link %s to itself", a),
		})
		return
	}

	c, cfg, src, ok := s.keyWriter(w, r, key)
	if !ok {
		return
	}
	linker, err := origin.AsIssueLinker(c)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	catalog, err := linker.IssueLinkTypes(r.Context())
	if err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	lt, inwardDescription, err := origin.ResolveLinkType(token, catalog)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// Jira displays type.outward when A is inwardIssue and type.inward when A
	// is outwardIssue. Put A on the end that makes the requested token the
	// phrase displayed on A.
	outward, inward := b, a
	if inwardDescription {
		outward, inward = a, b
	}
	if err := linker.LinkIssues(r.Context(), lt.ID, outward, inward); err != nil {
		failJira(w, r, s.config(), err)
		return
	}
	srcB, err := s.keySource(r.Context(), b)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":   "key_ambiguous",
			"message": err.Error(),
		})
		return
	}
	// B first so the A refresh that respondIssue reads is last, matching
	// cmd/gadak/link.go. A refresh failure after a landed write is not a
	// retryable 404 — the origin already accepted the link.
	if err := sync.RefreshIssue(r.Context(), cfg, s.db, b, srcB); err != nil {
		failMirrorStale(w, b, err)
		return
	}
	if err := sync.RefreshIssue(r.Context(), cfg, s.db, key, src); err != nil {
		failMirrorStale(w, key, err)
		return
	}
	s.respondIssue(w, r, key, map[string]any{
		"keys": []string{a, b},
		"type": map[string]string{
			"id":      lt.ID,
			"name":    lt.Name,
			"outward": lt.Outward,
			"inward":  lt.Inward,
		},
	})
}
