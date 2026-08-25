package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// The mirror REST (GDK-883): the surface a paired client reads and writes.
// `tailscale serve` forwards the MagicDNS Host, which the rebinding guard
// rejects by name — PairedMirrorHostExempt steps aside for mirror-REST
// /api/v1/ paths while active tokens exist, and mirrorGate then demands a
// ScopeServe Bearer. The guard vouches for the Host, the gate vouches for
// the token; neither trusts the other's half.
//
// A serve-scope Bearer opens every Handler-mux /api/v1/ route the loopback
// web UI can call (issues/, auth/, dashboards/). Two things stay out
// because they were never the mirror REST: the origin passthrough
// (/api/v1/origin/…, origin-scope only) and non-API paths (the SPA,
// /config.json, docs), which stay behind the rebinding guard for DNS Hosts.

// serveScopeAdmits reports whether a serve-scope Bearer may reach this
// method+path through a DNS-named Host. The surface is the whole mirror
// REST registered on the Handler mux — apiBase, authBase, dashBase — not a
// path-by-path allowlist (GDK-883). method is unused: the mux, not this
// gate, binds verbs.
//
// The origin passthrough never returns true here (pairing.AdmitsOrigin is
// that door). Non-API paths and outer-mux routes such as
// /api/v1/workspaces (no mirrorGate behind the exemption) also return
// false, so the rebinding guard keeps them closed for DNS Hosts.
//
// GDK-863: a serve token must never open a shell. No terminal route exists
// in this package today; whoever adds one puts the scope check on that
// route (do not admit a shell here).
func serveScopeAdmits(method, path string) bool {
	_ = method
	if path == origin.RESTPrefix || strings.HasPrefix(path, origin.RESTPrefix+"/") {
		return false
	}
	return strings.HasPrefix(path, apiBase) ||
		strings.HasPrefix(path, authBase) ||
		strings.HasPrefix(path, dashBase)
}

// PairedMirrorHostExempt lets GuardBrowser pass a DNS-named Host for
// mirror-REST requests while active pairing tokens exist — the same
// probe shape as PairedOriginHostExempt: an empty-bearer Authorize answers
// "does the gate have anything to check" without accepting anything.
// VerdictOff (or an unreadable store, which fails closed) keeps today's
// forbidden_host, so an unpaired serve never widens for the phone. dir is
// resolved per request: pairing.json can appear while a serve is running.
func PairedMirrorHostExempt(dir func() string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		if !serveScopeAdmits(r.Method, r.URL.Path) {
			return false
		}
		d := dir()
		if d == "" {
			return false
		}
		v, err := pairing.Authorize(d, "", time.Now())
		return err == nil && v == pairing.VerdictReject
	}
}

// mirrorGate gates DNS-named Hosts (the shape `tailscale serve` forwards)
// down to the mirror REST with a serve-scope Bearer. It sits inside
// GuardBrowser, after the Host and Origin checks, and is deliberately a
// separate middleware from pairingGate: the passthrough and the mirror
// have different inputs (raw REST paths vs. the whole mirror REST),
// different scope doors, and different no-token behavior.
//
// Loopback, *.localhost, IP literals, and empty Host never reach a
// decision here — the local web UI stays unauthenticated and
// byte-identical (decision 0003). A DNS Host this predicate refuses is
// likewise passed through unread; the guard has no exemption for it and
// answers forbidden_host. Only the exempted intersection speaks:
//
//	store unreadable          → 500 internal_error (fail closed)
//	no active tokens anymore  → 403 forbidden_host (nothing vouches for
//	                             the Host now; the guard would not step
//	                             aside either)
//	active tokens, no Bearer  → 401 pairing_rejected (+reason)
//	valid token, wrong scope  → 403 scope_rejected
//	serve-scope Bearer        → through
func (s *server) mirrorGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowedHost(r.Host) || !serveScopeAdmits(r.Method, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		cfg := s.config()
		if cfg == nil {
			log.Printf("server: mirror gate: no config for %s %s", r.Method, r.URL.Path)
			fail(w, http.StatusForbidden, "forbidden_host")
			return
		}
		verdict, meta, err := pairing.AuthorizeMeta(cfg.Directory(), bearerToken(r), time.Now())
		if err != nil {
			log.Printf("server: mirror gate: %s %s: %v", r.Method, r.URL.Path, err)
			fail(w, http.StatusInternalServerError, "internal_error")
			return
		}
		switch verdict {
		case pairing.VerdictOff:
			// Tokens vanished between the guard's probe and here: nothing
			// vouches for this Host anymore.
			log.Printf("server: mirror gate: pairing went away on %s %s", r.Method, r.URL.Path)
			fail(w, http.StatusForbidden, "forbidden_host")
			return
		case pairing.VerdictReject:
			_, reason := pairing.Explain(cfg.Directory(), bearerToken(r), time.Now())
			failPairing(w, reason)
			return
		case pairing.VerdictAccept:
			if meta.Scope != pairing.ScopeServe {
				log.Printf("server: mirror gate: label %q scope %q denied on %s %s",
					meta.Label, meta.Scope, r.Method, r.URL.Path)
				fail(w, http.StatusForbidden, "scope_rejected")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
