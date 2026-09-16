package server

import (
	"mime"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// Viewer identity (GDK-1966): who is asking, as far as this process can
// attest. The phone does not pair and does not authenticate — it opens the
// serve's address in a browser over the tailnet, and when that address is
// fronted by `tailscale serve`, the proxy is on this machine's loopback and
// stamps Tailscale-User-* headers it verified itself (tailnet identity, not
// something a page can set). Those headers are read here and nowhere else.
//
// A phone that reaches the serve's port directly — the same bytes minus the
// proxy — carries nothing trustworthy, and reads as nobody: the serve stays
// the loopback single-user surface it always was (decision 0003), with the
// viewer as the one optional layer on top.

const (
	viewerSourceTailscale = "tailscale"
	viewerSourceNone      = "none"
)

// Viewer is GET viewer/'s document. Source says who vouched for it:
// "tailscale" (loopback proxy attested the person) or "none".
type Viewer struct {
	Login  string `json:"login"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// viewerFrom reads the request's viewer. The headers count only when the
// bytes came from this machine's loopback — where `tailscale serve` proxies
// from. Any other peer (LAN, a direct tailnet hit on the serve's own port)
// is just setting headers, and gets the nobody answer. An empty RemoteAddr
// is an in-process call (terminalLocal's rule): no proxy there either, and
// the same nobody answer.
func viewerFrom(r *http.Request) Viewer {
	login := strings.TrimSpace(r.Header.Get("Tailscale-User-Login"))
	if login == "" {
		// Headers first: this runs on every request, and the peer check
		// below may ask the kernel for interface addresses.
		return Viewer{Source: viewerSourceNone}
	}
	if !viewerTrustedPeer(r.RemoteAddr) {
		return Viewer{Source: viewerSourceNone}
	}
	return Viewer{
		Login:  login,
		Name:   decodeHeaderWord(r.Header.Get("Tailscale-User-Name")),
		Source: viewerSourceTailscale,
	}
}

// decodeHeaderWord unfolds an RFC 2047 encoded-word. Measured 2026-09-16
// against a live `tailscale serve`: a Korean display name arrives as
// `=?utf-8?q?=EA=B9=80...?=`, which would otherwise be shown — and stamped
// as the actor's name — verbatim. Anything that is not an encoded word is
// returned trimmed, as it came.
func decodeHeaderWord(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if dec, err := (&mime.WordDecoder{}).DecodeHeader(v); err == nil {
		return strings.TrimSpace(dec)
	}
	return v
}

// viewerTrustedPeer is the loopback half of terminalLocal's rule: where the
// bytes came from, not what the Host header claims. This is about trusting
// a proxy's attestation, so the Host side of terminalLocal does not apply —
// a `tailscale serve` request arrives with the MagicDNS Host, and that is
// exactly the request whose headers must be read.
func viewerTrustedPeer(addr string) bool {
	if addr == "" {
		return true
	}
	ip := net.ParseIP(stripHostPort(addr))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	// A serve bound to its tailnet address (`--addr 100.x.y.z:7777
	// --allow-remote`, the runbook's shape) is reached by `tailscale serve`
	// from that same address, not from 127.0.0.1 — measured 2026-09-16:
	// the proxy dials the backend's own IP. A peer that IS one of this
	// machine's interface addresses is this machine; another tailnet node
	// cannot present it (WireGuard binds the source to the key), and a
	// process already on this host is inside the trust boundary anyway.
	return isLocalInterfaceIP(ip)
}

// isLocalInterfaceIP reports whether ip is bound to one of this host's
// interfaces. The answer is cached briefly: interface lists move rarely,
// and this sits on the request path.
func isLocalInterfaceIP(ip net.IP) bool {
	localIPsMu.Lock()
	defer localIPsMu.Unlock()
	if time.Since(localIPsAt) > localIPsTTL || localIPs == nil {
		localIPs = map[string]bool{}
		if addrs, err := net.InterfaceAddrs(); err == nil {
			for _, a := range addrs {
				if n, ok := a.(*net.IPNet); ok && n.IP != nil {
					localIPs[n.IP.String()] = true
				}
			}
		}
		localIPsAt = time.Now()
	}
	return localIPs[ip.String()]
}

var (
	localIPsMu  sync.Mutex
	localIPs    map[string]bool
	localIPsAt  time.Time
	localIPsTTL = 30 * time.Second
)

// viewerActor derives the acting identity for write attribution from a
// trusted viewer: the login's local part (before @), validated exactly the
// way `gadak config set actor` validates a slug — verbatim, no mangling.
// ok is false when nothing usable is attested; the caller then leaves the
// request's identity alone and the write falls back to the session actor.
func viewerActor(v Viewer) (slug, name string, ok bool) {
	if v.Source != viewerSourceTailscale || strings.TrimSpace(v.Login) == "" {
		return "", "", false
	}
	local, _, _ := strings.Cut(strings.TrimSpace(v.Login), "@")
	a, err := config.ValidateActor(local, v.Name)
	if err != nil || a == nil {
		return "", "", false
	}
	return a.Slug, a.Name, true
}

// withViewerActor is ServeHTTP's one hook: a trusted viewer on a gadak
// origin rides the request as a context override, and handlerTransport
// stamps it over the session actor for this request's writes only. Every
// other shape — non-gadak origin (the header is an issuetap extension and
// noise anywhere else), untrusted peer, unusable slug — returns the request
// untouched.
func (s *server) withViewerActor(r *http.Request) *http.Request {
	cfg := s.config()
	if cfg == nil || cfg.OriginType() != config.OriginGadak {
		return r
	}
	slug, name, ok := viewerActor(viewerFrom(r))
	if !ok {
		return r
	}
	return r.WithContext(origin.WithViewerActor(r.Context(), slug, name))
}

// handleViewer answers GET viewer/: the loopback proxy's attested person,
// or "none". Read-only — identity, not authority; it says who is asking,
// never what they may do.
func (s *server) handleViewer(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, viewerFrom(r))
}
