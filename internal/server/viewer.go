package server

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
// "tailscale" (loopback proxy attested the person) or "none". Declared is
// the X-Gadak-Actor-Name this request carried when it was honoured
// (GDK-1973) — the name a person typed in Settings — and the empty string
// otherwise: present, never omitted, same rule as withViewerActor applies.
type Viewer struct {
	Login    string `json:"login"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	Declared string `json:"declared"`
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
	if local == "" {
		// A degenerate login ("@example.com") has no local part to be.
		return "", "", false
	}
	a, err := config.ValidateActor(local, v.Name, "")
	if err != nil || a == nil {
		return "", "", false
	}
	return a.Slug, a.Name, true
}

// declaredActorMaxRunes caps the self-declared display name (GDK-1973): a
// name over the cap is ignored, not truncated — a truncated name would be
// a different person's attribution.
const declaredActorMaxRunes = 64

// declaredActorName reads X-Gadak-Actor-Name: the UTF-8 display name a
// person typed in Settings, percent-encoded by the client and decoded
// here. ok is false when the value is absent, blank, undecodable, or over
// the cap — each of those means "no declaration", never an error: the
// header buys attribution, and a bad declaration attributes to nobody.
func declaredActorName(r *http.Request) (string, bool) {
	raw := strings.TrimSpace(r.Header.Get("X-Gadak-Actor-Name"))
	if raw == "" {
		return "", false
	}
	name, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > declaredActorMaxRunes {
		return "", false
	}
	return name, true
}

// withViewerActor is ServeHTTP's one hook, and the single owner of the
// request-identity precedence (GDK-1973):
//
//  1. a verified Tailscale viewer (viewerFrom, loopback proxy attested) —
//     always a person;
//  2. X-Gadak-Actor-Name, the self-declared name — a person, attribution
//     only, never authority: no gate reads it (the terminal gate's refusal
//     is pinned in terminal_test.go);
//  3. the serve process's own actor (env/config, ResolveActor);
//  4. the origin's default user.
//
// An attestation that is present but unusable is not a vacancy — a garbage
// login does not fall through to the declaration. Non-gadak origins skip
// the whole ladder's first two rungs: X-Issuetap-* is an issuetap
// extension and noise anywhere else, and the request goes back untouched.
func (s *server) withViewerActor(r *http.Request) *http.Request {
	cfg := s.config()
	if cfg == nil || cfg.OriginType() != config.OriginGadak {
		return r
	}
	if v := viewerFrom(r); v.Source == viewerSourceTailscale {
		slug, name, ok := viewerActor(v)
		if !ok {
			return r
		}
		// A verified viewer is a person (GDK-1973): the proxy attested a
		// human's tailnet account.
		return r.WithContext(origin.WithViewerActor(r.Context(), slug, name, config.ActorKindPerson))
	}
	name, ok := declaredActorName(r)
	if !ok {
		return r
	}
	return r.WithContext(origin.WithViewerActor(r.Context(), config.PersonSlug(name), name, config.ActorKindPerson))
}

// handleViewer answers GET viewer/: the loopback proxy's attested person,
// or "none", plus the declared name when this request's declaration is one
// withViewerActor would honour — the same gate, read through the same
// helpers, so the document cannot disagree with the writes. Read-only —
// identity, not authority; it says who is asking, never what they may do.
func (s *server) handleViewer(w http.ResponseWriter, r *http.Request) {
	v := viewerFrom(r)
	if s.config() != nil && s.config().OriginType() == config.OriginGadak && v.Source != viewerSourceTailscale {
		if name, ok := declaredActorName(r); ok {
			v.Declared = name
		}
	}
	writeJSON(w, http.StatusOK, v)
}
