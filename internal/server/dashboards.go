package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/midagedev/gadak/internal/dashboards"
	"github.com/midagedev/gadak/internal/store"
)

// Agent-authored dashboards (GDK-781): an HTML document plus named
// datasources, saved like a view (local.db, survives a mirror wipe) and
// served for an iframe. The row set and the change counter live in the
// store; config interpretation (validation, execution) is internal/dashboards;
// these handlers are only the HTTP shape.

// dashboardCSP is the render response's whole network policy. It is what
// makes agent-written HTML safe to serve: no origins, no fetches, inline
// script/style only (the document is hand-authored, not bundled), and
// data: images (inline SVG/charts). In the desktop shell the browser guard's
// Origin check cannot see dashboards (desktop strips Origin on /api/ before
// this server runs), so this header is the only network-blocking layer
// there — no host may ever be added to it.
const dashboardCSP = `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:`

// dashboardBodyLimit bounds one save. 8 MiB is ~40× the largest dashboard
// anyone should write; it exists so a runaway generator fails with a named
// error instead of an OOM.
const dashboardBodyLimit = 8 << 20

// listedDashboard is one row of the list response: identity fields only.
// The full config travels on `show`/render, not on every poll.
type listedDashboard struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	UpdatedAt string `json:"updated_at"`
}

func (s *server) dashboardList(w http.ResponseWriter, r *http.Request) {
	stored, err := s.db.Dashboards(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	version, err := s.db.DashboardVersion(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	out := make([]listedDashboard, 0, len(stored))
	for _, d := range stored {
		out = append(out, listedDashboard{ID: d.ID, Name: d.Name, UpdatedAt: d.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version, "dashboards": out})
}

// handleSaveDashboard is POST dashboards/: same name updates the row that
// already owns it (the saved-views convention — save is an upsert by name).
// A config that fails ParseConfig is a 400 with the reason, because the
// caller of this endpoint is an agent: the reason is the fix.
func (s *server) handleSaveDashboard(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, dashboardBodyLimit))
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if len(body) >= dashboardBodyLimit {
		fail(w, http.StatusRequestEntityTooLarge, "body_too_large")
		return
	}
	var req struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if req.Name == "" {
		fail(w, http.StatusBadRequest, "name_required")
		return
	}
	// Validate before the row exists: a rejected config leaves no state, and
	// the reason rides along because this endpoint's caller is an agent.
	if _, err := dashboards.ParseConfig(req.Config); err != nil {
		failMsg(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	saved, err := s.db.SaveDashboard(r.Context(), req.Name, string(req.Config))
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

func (s *server) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteDashboard(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, http.StatusNotFound, "not_found")
			return
		}
		serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRenderDashboard serves the document bytes for an iframe. The stored
// config is parsed (not trusted) so a corrupt row answers 500 here, not a
// blank frame with no diagnosis.
func (s *server) handleRenderDashboard(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := s.dashboardConfig(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", dashboardCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(cfg.HTML))
}

// handleDashboardData executes one named datasource and answers the
// {columns, rows, truncated, warning?} document. SQL errors are 400s (the
// query came from the dashboard's config, so the author must see the
// driver's message), not 500s.
func (s *server) handleDashboardData(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := s.dashboardConfig(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	src, ok := cfg.Datasources[name]
	if !ok {
		fail(w, http.StatusNotFound, "datasource_not_found")
		return
	}
	if src.SQL != "" {
		ro, err := s.db.ReadOnly()
		if err != nil {
			serverError(w, r, err)
			return
		}
		defer ro.Close()
		res, err := dashboards.ExecuteSQL(ro, src.SQL)
		if err != nil {
			failMsg(w, http.StatusBadRequest, "sql_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	res, err := dashboards.ExecuteJQL(r.Context(), s.db, configuredIdentity(s, ""), src.JQL)
	if err != nil {
		failMsg(w, http.StatusBadRequest, "jql_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleAbsorbDashboards is the one-shot export/import merge —
// handleAbsorbViews, for dashboards. Stored names win; id collisions get a
// fresh id. The response is the merged list document so the caller adopts
// the post-merge state in one round trip.
func (s *server) handleAbsorbDashboards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dashboards []store.Dashboard `json:"dashboards"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, dashboardBodyLimit)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if err := s.db.AbsorbDashboards(r.Context(), req.Dashboards); err != nil {
		serverError(w, r, err)
		return
	}
	s.dashboardList(w, r)
}

// dashboardConfig resolves {id} to a row and its parsed config, writing the
// error response when either step fails. Unknown id is 404; a stored config
// that no longer parses is 500 (the row predates or outlives this binary's
// validator — an operator problem, not an author's).
func (s *server) dashboardConfig(w http.ResponseWriter, r *http.Request) (store.Dashboard, dashboards.Config, bool) {
	var cfg dashboards.Config
	d, err := s.db.Dashboard(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "not_found")
		return d, cfg, false
	}
	if err != nil {
		serverError(w, r, err)
		return d, cfg, false
	}
	cfg, err = dashboards.ParseConfig(d.Config)
	if err != nil {
		serverError(w, r, err)
		return d, cfg, false
	}
	return d, cfg, true
}

// failMsg is fail plus a reason: `{"error": code, "message": reason}`. Used
// where the reason is the fix — config validation and SQL/JQL errors an
// agent must read to correct the dashboard they just saved.
func failMsg(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}
