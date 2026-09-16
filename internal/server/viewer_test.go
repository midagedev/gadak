package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// Viewer identity (GDK-1966): who is on the other side of the serve, as far
// as this process can attest. `tailscale serve` proxies from loopback and
// stamps Tailscale-User-* headers it verified itself; the same headers from
// any other peer are just headers, and must read as nobody.

func TestViewerFromLoopbackTailscaleHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, viewerBase, nil)
	req.RemoteAddr = "127.0.0.1:52100"
	req.Header.Set("Tailscale-User-Login", "kim@example.com")
	req.Header.Set("Tailscale-User-Name", "Kim")
	got := viewerFrom(req)
	if got.Login != "kim@example.com" || got.Name != "Kim" || got.Source != "tailscale" {
		t.Fatalf("viewerFrom() = %+v, want {kim@example.com Kim tailscale}", got)
	}
}

func TestViewerFromNonLoopbackHeadersIgnored(t *testing.T) {
	// The phone reaching the serve's own port carries nothing trustworthy;
	// a hostile peer on the LAN can set these headers to anything.
	req := httptest.NewRequest(http.MethodGet, viewerBase, nil)
	req.RemoteAddr = "192.0.2.9:52100"
	req.Header.Set("Tailscale-User-Login", "kim@example.com")
	req.Header.Set("Tailscale-User-Name", "Kim")
	if got := viewerFrom(req); got.Source != "none" || got.Login != "" || got.Name != "" {
		t.Fatalf("viewerFrom(non-loopback) = %+v, want the zero viewer with source none", got)
	}
}

func TestViewerFromLoopbackWithoutHeadersIsNone(t *testing.T) {
	// The local user's own browser: loopback, no proxy headers — nobody
	// special, which is the loopback single-user model exactly as before.
	req := httptest.NewRequest(http.MethodGet, viewerBase, nil)
	req.RemoteAddr = "127.0.0.1:52100"
	if got := viewerFrom(req); got != (Viewer{Source: "none"}) {
		t.Fatalf("viewerFrom(no headers) = %+v, want {none}", got)
	}
}

func TestViewerRouteAnswersDocument(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodGet, viewerBase, nil)
	req.Host = "127.0.0.1:7777"
	req.RemoteAddr = "127.0.0.1:52100"
	req.Header.Set("Tailscale-User-Login", "kim@example.com")
	req.Header.Set("Tailscale-User-Name", "Kim")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	got := decode[Viewer](t, rec)
	if got.Login != "kim@example.com" || got.Name != "Kim" || got.Source != "tailscale" {
		t.Fatalf("viewer document = %+v, want the loopback-proxied viewer", got)
	}

	// The viewer document carries identity, not authority: a non-loopback
	// peer asking the same route reads "none", same as viewerFrom.
	req = httptest.NewRequest(http.MethodGet, viewerBase, nil)
	req.Host = "192.0.2.9:7777"
	req.RemoteAddr = "192.0.2.9:52100"
	req.Header.Set("Tailscale-User-Login", "kim@example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("IP-literal Host status %d, want 200 (allowed host, no headers trusted): %s", rec.Code, rec.Body.String())
	}
	if got := decode[Viewer](t, rec); got.Source != "none" || got.Login != "" {
		t.Fatalf("viewer document = %+v, want none (headers untrusted off loopback)", got)
	}
}

// The write-attribution half: a trusted viewer's login local part becomes
// the actor slug, validated exactly the way `gadak config set actor`
// validates one. Anything that does not validate attributes to nobody —
// the write then falls back to the session actor, never fails.
func TestViewerActorDerivation(t *testing.T) {
	cases := []struct {
		login string
		slug  string
		ok    bool
	}{
		{"kim@example.com", "kim", true},
		{"kim", "kim", true},                 // no @: the whole login is local
		{"Kim@Example.com", "Kim", true},     // no mangling — validated verbatim
		{"has space@example.com", "", false}, // whitespace is not a slug
		{"", "", false},
		{strings.Repeat("a", 129) + "@example.com", "", false}, // over the cap
	}
	for _, c := range cases {
		v := Viewer{Login: c.login, Name: "Kim", Source: "tailscale"}
		slug, _, ok := viewerActor(v)
		if ok != c.ok || slug != c.slug {
			t.Errorf("viewerActor(%q) = (%q, %v), want (%q, %v)", c.login, slug, ok, c.slug, c.ok)
		}
	}
}

// End to end: a loopback-proxied request on a gadak origin carries the
// viewer actor in its context, so handlerTransport stamps the person's slug
// over the session actor for that request's writes (origin's
// transport_viewer_test.go pins the stamping itself).
func TestServeHTTPAttachesViewerActorContext(t *testing.T) {
	db, cfg := fixture(t)
	cfg.Kind = config.OriginGadak
	h := New(db, cfg)

	req := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req.Host = "127.0.0.1:7777"
	req.RemoteAddr = "127.0.0.1:52100"
	req.Header.Set("Tailscale-User-Login", "kim@example.com")
	req.Header.Set("Tailscale-User-Name", "Kim")
	ctxReq := h.s.withViewerActor(req)
	if ctxReq == req {
		t.Fatal("withViewerActor returned the request unchanged for a trusted viewer on a gadak origin")
	}

	// A non-gadak origin (Jira) never sees the override: the header is an
	// issuetap extension and would be noise on any other origin.
	jiraCfg := *cfg
	jiraCfg.Kind = ""
	req2 := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req2.RemoteAddr = "127.0.0.1:52100"
	req2.Header.Set("Tailscale-User-Login", "kim@example.com")
	h2 := New(db, &jiraCfg)
	if got := h2.s.withViewerActor(req2); got != req2 {
		t.Fatal("withViewerActor attached the viewer actor on a non-gadak origin")
	}

	// Untrusted peer: no override even on a gadak origin.
	req3 := httptest.NewRequest(http.MethodPost, apiBase+"NMB-1/comment/", nil)
	req3.RemoteAddr = "192.0.2.9:52100"
	req3.Header.Set("Tailscale-User-Login", "kim@example.com")
	if got := h.s.withViewerActor(req3); got != req3 {
		t.Fatal("withViewerActor attached the viewer actor for a non-loopback peer")
	}
}

// The proxy's peer address is the backend's own when the serve is bound to
// its tailnet IP (measured 2026-09-16 on the GDK host): the machine's own
// interface address counts as this machine, a foreign address never does.
func TestViewerTrustsOwnInterfaceAddress(t *testing.T) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip("no interface list")
	}
	var own string
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP != nil && !n.IP.IsLoopback() && n.IP.To4() != nil {
			own = n.IP.String()
			break
		}
	}
	if own == "" {
		t.Skip("no non-loopback IPv4 interface on this host")
	}
	if !viewerTrustedPeer(own + ":51234") {
		t.Fatalf("own interface address %s should be a trusted peer", own)
	}
	if viewerTrustedPeer("192.0.2.77:51234") {
		t.Fatal("a foreign address must not be a trusted peer")
	}
}

// Tailscale sends non-ASCII display names as RFC 2047 encoded-words
// (measured 2026-09-16: a Korean name arrived as =?utf-8?q?…?=).
func TestViewerDecodesEncodedName(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/viewer/", nil)
	r.RemoteAddr = "127.0.0.1:40000"
	r.Header.Set("Tailscale-User-Login", "dana@example.com")
	r.Header.Set("Tailscale-User-Name", "=?utf-8?q?=EA=B9=80=ED=98=84=EC=B2=A0?=")
	v := viewerFrom(r)
	if v.Name != "김현철" {
		t.Fatalf("Name = %q, want the decoded word", v.Name)
	}
	r.Header.Set("Tailscale-User-Name", "Dana Whitfield")
	if v := viewerFrom(r); v.Name != "Dana Whitfield" {
		t.Fatalf("plain name changed: %q", v.Name)
	}
}
