package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// PairedOriginHostExempt lets GuardBrowser pass a DNS-named Host — which the
// rebinding check otherwise rejects — for origin-passthrough requests while
// active pairing tokens exist. Measured on a real tailnet (GDK-443): tailscale
// serve forwards the original `<machine>.<tailnet>.ts.net` Host upstream, so
// without this every paired request died as forbidden_host before the Bearer
// gate could speak. Authorize with an empty bearer answers "do tokens exist"
// without accepting anything: VerdictOff (or an error, which fails closed)
// keeps today's rejection, VerdictReject means pairingGate will demand the
// Bearer right after this. dir is resolved per request — pairing.json can
// appear while a serve is running.
func PairedOriginHostExempt(dir func() string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		if !strings.HasPrefix(r.URL.Path, origin.RESTPrefix) {
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

// handleOriginREST forwards method, path, query, headers, and body to this
// process's embedded issuetap handler. It is not a mirror-write API: the
// SQLite mirror is never the target. Writes go through the workspace origin
// (issuetap here, Jira on a connected workspace — which 404s below). Loopback
// single-user model: no extra auth (decision 0003) — except the pairing gate
// (GDK-433), which applies while at least one active pairing token exists.
func (s *server) handleOriginREST(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	if cfg == nil || !cfg.IsStandalone() {
		handleNotFound(w, r)
		return
	}
	if !s.pairingGate(w, r, cfg) {
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

// pairingGate is the GDK-433 authorization point for everything under
// origin.RESTPrefix — this handler is the single choke point both `gadak
// serve` and the desktop app's origin-only listener forward to.
//
// The serve binds loopback and the exposure is done by a proxy (tailscale
// serve), so remote requests arrive as loopback too: network origin cannot
// distinguish trust, and the token is the only identity. While any active
// pairing token exists, a request must carry a valid Bearer; there is no
// loopback bypass, so the home machine's own routed CLI writes need a
// paired token as well. With no active token the gate is off and the
// passthrough behaves exactly as before (implicit loopback trust).
//
// On accept, the Bearer is rewritten into the in-process Basic credential
// so the embedded issuetap graph sees the Authorization shape it has always
// seen — the gate authenticates the caller, then speaks to the origin as
// the in-process user. The token itself never reaches the origin process.
//
// Since GDK-797 the accept is scope-checked: a token must carry a scope
// that admits the passthrough (origin, local-routing, or the empty scope
// of tokens minted before scopes mattered). A serve token — minted for the
// mirror allowlist — is refused with scope_rejected, so a leaked phone
// token cannot reach raw REST even though it authenticates.
func (s *server) pairingGate(w http.ResponseWriter, r *http.Request, cfg *config.Config) bool {
	verdict, meta, err := pairing.AuthorizeMeta(cfg.Directory(), bearerToken(r), time.Now())
	if err != nil {
		// Unreadable/corrupt store fails closed: tokens may exist.
		log.Printf("server: pairing gate: %s %s: %v", r.Method, r.URL.Path, err)
		fail(w, http.StatusInternalServerError, "internal_error")
		return false
	}
	switch verdict {
	case pairing.VerdictReject:
		_, reason := pairing.Explain(cfg.Directory(), bearerToken(r), time.Now())
		failPairing(w, reason)
		return false
	case pairing.VerdictAccept:
		if !pairing.AdmitsOrigin(meta.Scope) {
			log.Printf("server: pairing gate: label %q scope %q denied on %s %s",
				meta.Label, meta.Scope, r.Method, r.URL.Path)
			fail(w, http.StatusForbidden, "scope_rejected")
			return false
		}
		r.Header.Set("Authorization", "Basic "+origin.InProcessAuthB64())
	}
	return true
}

// failPairing is the 401 body the remote FoldPairedError reads: a stable
// error code plus a reason that is detailed only for tokens the store has
// seen (GDK-453).
func failPairing(w http.ResponseWriter, reason pairing.Reason) {
	if reason == "" {
		reason = pairing.ReasonUnknown
	}
	w.Header().Set("X-Gadak-Pairing", string(reason))
	writeJSON(w, http.StatusUnauthorized, struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}{Error: "pairing_rejected", Reason: string(reason)})
}

// bearerToken extracts the Bearer credential from an Authorization header.
// Any other scheme (including the in-process Basic a local CLI sends) is
// "no token": indistinguishable from a missing header at the gate.
func bearerToken(r *http.Request) string {
	scheme, rest, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if !strings.EqualFold(strings.TrimSpace(scheme), "Bearer") {
		return ""
	}
	return strings.TrimSpace(rest)
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
