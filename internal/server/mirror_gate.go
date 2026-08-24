package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/pairing"
)

// The mirror REST allowlist (GDK-797): the surface a paired phone
// companion reads. `tailscale serve` forwards the MagicDNS Host, which the
// rebinding guard rejects by name — PairedMirrorHostExempt steps aside for
// exactly the paths below while active tokens exist, and mirrorGate then
// demands a ScopeServe Bearer. The guard vouches for the Host (only
// allowlisted paths, only while the gate can authenticate), the gate
// vouches for the token; neither trusts the other's half.

// mirrorAllowlisted reports whether method+path is on the phone's mirror
// allowlist. Reads the companion needs to paint a board (bootstrap, delta,
// search, jql, feed, views, auth/me) plus the detail/transitions reads and
// the comment/transition writes — the writes ride the existing mutate
// path, so no new write API opens here. Everything else — credential,
// settings, onboarding, sync, create, dashboards, SPA docs, config.json,
// attachment content — stays behind the rebinding guard for DNS Hosts.
// path is r.URL.Path (query strings are not part of the contract).
func mirrorAllowlisted(method, path string) bool {
	switch method + " " + path {
	case "GET /api/v1/auth/me/",
		"GET /api/v1/issues/bootstrap/",
		"GET /api/v1/issues/delta/",
		"GET /api/v1/issues/search/",
		"GET /api/v1/issues/jql/",
		"POST /api/v1/issues/jql/",
		"GET /api/v1/issues/feed/",
		"POST /api/v1/issues/feed/read/",
		"GET /api/v1/issues/views/":
		return true
	}
	_, action, ok := mirrorKeyAction(path)
	if !ok {
		return false
	}
	switch action {
	case "detail", "transitions":
		return method == http.MethodGet
	case "comment", "transition":
		return method == http.MethodPost
	}
	return false
}

// mirrorKeyAction splits "<KEY>/<action>/" under the issues base into its
// two segments, refusing anything deeper or emptier. The key itself is not
// validated here — the mux routes it, and a stray key merely 404s inside
// the allowlist rather than widening it.
func mirrorKeyAction(path string) (key, action string, ok bool) {
	rest, has := strings.CutPrefix(path, apiBase)
	if !has || rest == "" {
		return "", "", false
	}
	k, a, cut := strings.Cut(strings.TrimSuffix(rest, "/"), "/")
	if !cut || k == "" || a == "" || strings.Contains(a, "/") {
		return "", "", false
	}
	return k, a, true
}

// PairedMirrorHostExempt lets GuardBrowser pass a DNS-named Host for
// allowlisted mirror requests while active pairing tokens exist — the same
// probe shape as PairedOriginHostExempt: an empty-bearer Authorize answers
// "does the gate have anything to check" without accepting anything.
// VerdictOff (or an unreadable store, which fails closed) keeps today's
// forbidden_host, so an unpaired serve never widens for the phone. dir is
// resolved per request: pairing.json can appear while a serve is running.
func PairedMirrorHostExempt(dir func() string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		if !mirrorAllowlisted(r.Method, r.URL.Path) {
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
// down to the mirror allowlist with a serve-scope Bearer. It sits inside
// GuardBrowser, after the Host and Origin checks, and is deliberately a
// separate middleware from pairingGate: the passthrough and the mirror
// have different inputs (raw REST paths vs. an allowlist), different
// scope doors, and different no-token behavior.
//
// Loopback, *.localhost, IP literals, and empty Host never reach a
// decision here — the local web UI stays unauthenticated and
// byte-identical (decision 0003). A DNS Host outside the allowlist is
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
		if allowedHost(r.Host) || !mirrorAllowlisted(r.Method, r.URL.Path) {
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
