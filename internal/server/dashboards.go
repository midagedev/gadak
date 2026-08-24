package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/config"
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
// there — no host may ever be added to it beyond the request's own.
//
// [GDK-792] vendorSrc is the one sanctioned widening: a path-scoped source
// for the embedded vendor libraries, so a dashboard can chart with uPlot or
// three without a CDN (the no-outbound rule moves, not bends). The render
// document is sandboxed without allow-same-origin, so its origin is opaque
// and CSP 'self' matches nothing there — the source must name the host the
// frame's own URLs came from, which is why this composes per request.
//
// [GDK-808] libsSrc is the second, conditional widening: the local
// lib-cache route, joined to script-src only when the dashboard declares
// libs. Undeclared dashboards must keep the exact pre-808 policy byte for
// byte — least privilege is the default, and the existing exact-match
// assertion pins it. Style stays vendor-only: a lib is JavaScript served as
// application/javascript, and nosniff already refuses to load it as CSS.
func dashboardCSP(vendorSrc, libsSrc string) string {
	scriptSrc, styleSrc := "'unsafe-inline'", "'unsafe-inline'"
	if vendorSrc != "" {
		scriptSrc += " " + vendorSrc
		styleSrc += " " + vendorSrc
	}
	if libsSrc != "" {
		scriptSrc += " " + libsSrc
	}
	return "default-src 'none'; script-src " + scriptSrc + "; style-src " + styleSrc + "; img-src data:"
}

// vendorCSPSource turns a request Host into the vendor path's CSP source
// expression. Anything that is not a plain hostname, an IPv6 literal, or
// either of those with a numeric port yields "" — the caller then omits the
// source entirely (fail closed: a hostile Host may narrow the policy, never
// widen it). GuardBrowser already confines real traffic to loopback Hosts;
// this does not assume it ran.
func vendorCSPSource(host string, tls bool) string {
	return cspHostSource(host, tls, dashVendorPath)
}

// libsCSPSource is vendorCSPSource for the lib-cache route (GDK-808). The
// same host validation — one owner, two path scopes.
func libsCSPSource(host string, tls bool) string {
	return cspHostSource(host, tls, dashLibsPath)
}

// cspHostSource is the single owner of "which Host may appear in the render
// CSP": scheme from the request, then the host[:port] validated byte by byte
// and joined to a fixed path scope.
func cspHostSource(host string, tls bool, pathScope string) string {
	scheme := "http"
	if tls {
		scheme = "https"
	}
	authority := host
	if strings.HasPrefix(authority, "[") {
		end := strings.Index(authority, "]")
		if end < 0 || !isIPv6Literal(authority[1:end]) {
			return ""
		}
		if rest := authority[end+1:]; rest != "" && (rest[0] != ':' || !isPort(rest[1:])) {
			return ""
		}
	} else if i := strings.LastIndex(authority, ":"); i >= 0 {
		if !isPort(authority[i+1:]) {
			return ""
		}
		authority = authority[:i]
		if !isHostname(authority) {
			return ""
		}
		authority = host // keep the port: sources are host[:port]
	} else if !isHostname(authority) {
		return ""
	}
	if len(authority) == 0 || len(authority) > 253 {
		return ""
	}
	return scheme + "://" + authority + pathScope
}

// isHostname reports a dotted hostname without a single CSP-breaking byte:
// alnum and '-' per label, no empty or hyphen-edge label. (No wildcards — a
// Host header has no business being "*.com".)
func isHostname(s string) bool {
	if s == "" {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// isIPv6Literal accepts only the bytes an IPv6 text form can contain.
func isIPv6Literal(s string) bool {
	if s == "" || !strings.Contains(s, ":") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' || c == ':' || c == '.') {
			return false
		}
	}
	return true
}

func isPort(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// dashboardBodyLimit bounds one save. 8 MiB is ~40× the largest dashboard
// anyone should write; it exists so a runaway generator fails with a named
// error instead of an OOM.
const dashboardBodyLimit = 8 << 20

// dashVendorPath is the vendor-asset prefix served inside the dashboards API
// (GDK-792): the pinned uPlot build an authored dashboard may load instead
// of a CDN. It is also the path scope of the render response's script-src and
// style-src vendor source, so the CSP and the route cannot drift apart.
const dashVendorPath = "/api/v1/dashboards/vendor/"

// dashLibsPath is the lib-cache prefix (GDK-808): dashboards render
// `lib add`-ed libraries from here, hash-verified at serve time. Same
// relationship to the CSP as dashVendorPath — the constants are the policy.
const dashLibsPath = "/api/v1/dashboards/libs/"

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
	cfg, err := dashboards.ParseConfig(req.Config)
	if err != nil {
		failMsg(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}
	// A declared lib must already be in the cache (GDK-808): render injects
	// the id into a script tag, so a typo would otherwise save fine and
	// surface as a silently-blank frame later. The message carries the
	// recipe — the caller is an agent that can act on it. A manifest that
	// cannot be read is a 500, not a 400: that is an operator problem, not
	// an author's.
	if len(cfg.Libs) > 0 {
		missing, err := dashboards.LibsExist(s.libsDir(), cfg.Libs)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if len(missing) > 0 {
			failMsg(w, http.StatusBadRequest, "unknown_lib",
				fmt.Sprintf("lib %s is not in the cache — run `gadak dashboards lib add <url>` and re-save with the id it prints (see `gadak dashboards lib list`)", missing[0]))
			return
		}
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
// blank frame with no diagnosis. Declared libs (GDK-808) are injected as
// deferred scripts in declaration order and widen script-src with the local
// libs route — an undeclared dashboard renders byte-identical to pre-808.
func (s *server) handleRenderDashboard(w http.ResponseWriter, r *http.Request) {
	_, cfg, ok := s.dashboardConfig(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The vendor source names the host this very response came from: the
	// sandboxed frame cannot state its own origin ('self' is null there), so
	// it needs the explicit host+path to load vendor libraries. Fail closed
	// on a Host that does not parse (vendorCSPSource returns ""). The libs
	// source joins script-src only when this dashboard declares libs.
	var libsSrc string
	if len(cfg.Libs) > 0 {
		libsSrc = libsCSPSource(r.Host, r.TLS != nil)
	}
	w.Header().Set("Content-Security-Policy", dashboardCSP(vendorCSPSource(r.Host, r.TLS != nil), libsSrc))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(injectLibScripts(cfg.HTML, cfg.Libs)))
}

// injectLibScripts splices one deferred <script> per declared lib into the
// document's head, in declaration order. defer executes them in document
// order after parse — the author's inline listeners register during parse,
// then the libraries exist, then the parent's data pushes land: a chart
// library is ready before the first postMessage that wants it. The
// onerror marker makes a refused lib visible on the wall (the serve route
// 500s a tampered cache; this attribute is what turns that into something
// a dashboard can show instead of silently missing charts). Ids come from a
// config that passed ParseConfig's LibIDPattern, so the splice cannot
// introduce markup. A headless document (no <head>, no <html>) gets the
// tags prepended — the browser would have synthesized that head anyway.
func injectLibScripts(html string, libs []string) string {
	if len(libs) == 0 {
		return html
	}
	var b strings.Builder
	for _, id := range libs {
		// No trailing slash: the route is the vendor route's no-slash shape
		// (a trailing-slash pattern would be ambiguous with {id}/render/).
		b.WriteString(`<script src="` + dashLibsPath + id + `" defer onerror="document.documentElement.setAttribute('data-gadak-lib-error','` + id + `')"></script>`)
	}
	tags := b.String()
	for _, open := range []string{"<head>", "<head ", "<head\t"} {
		if i := strings.Index(strings.ToLower(html), open); i >= 0 {
			i += len(open)
			return html[:i] + tags + html[i:]
		}
	}
	if i := strings.Index(strings.ToLower(html), "<html"); i >= 0 {
		// after the html open tag's closing '>'
		if j := strings.IndexByte(html[i:], '>'); j >= 0 {
			i += j + 1
			return html[:i] + tags + html[i:]
		}
	}
	return tags + html
}

// handleDashboardLib serves one cached library (GDK-808). The route is the
// only place cache bytes leave the machine's disk: the manifest entry is
// resolved by exact id (a path-shaped id simply misses — the pattern forbids
// separators), and the bytes are re-hashed against the pin before they are
// written, so a cache file modified after lib add answers 500 here instead
// of executing. Unknown id is 404, like the vendor route. Content type is
// fixed — the cache stores bytes, not claims about them.
func (s *server) handleDashboardLib(w http.ResponseWriter, r *http.Request) {
	_, body, err := dashboards.LibReadVerified(s.libsDir(), r.PathValue("file"))
	if errors.Is(err, dashboards.ErrLibNotFound) {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		// ErrLibCorrupt: bytes on disk no longer match the pin (or the
		// manifest does not parse). serverError logs the restore recipe the
		// error carries, and nothing executes.
		serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// Same reason as the vendor route: ES-module imports from the
	// opaque-origin sandbox frame are CORS requests.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(body)
}

// libsDir is this server's lib-cache directory (profile-scoped, like the
// attachment cache — one profile, one cache).
func (s *server) libsDir() string {
	d, err := config.DirFor(s.profile)
	if err != nil {
		return ""
	}
	return dashboards.LibsDir(d)
}

// handleGetDashboard is GET {id}/ — the row itself, config included. The web
// host reads it to learn the datasources map (which data routes to execute
// for the frame); updated_at and config ride along so the live-update poll
// (GDK-793) can detect a change in this one fetch.
func (s *server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.db.Dashboard(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleDashboardVendor (GDK-792) serves one embedded vendor asset from the
// fixed whitelist in DashVendorFile. Everything not on the table 404s — the
// whitelist is the table, not the directory, so licenses embedded for NOTICE
// never serve and neither does anything a future file drop adds.
func handleDashboardVendor(w http.ResponseWriter, r *http.Request) {
	body, contentType, ok := gadak.DashVendorFile(r.PathValue("file"))
	if !ok {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// The frame loading this is sandboxed without allow-same-origin, so its
	// subresource fetches are CORS-mode requests from an opaque origin.
	// Without ACAO the load fails to evaluate even with CSP permission —
	// this header is what makes the vendor recipe usable, not a politeness.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write(body)
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
