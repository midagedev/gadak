package server

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// GuardExempts are the ways a request can step past one of browserGuard's
// name-based checks — never both by one func, because the two checks stop
// different attackers and an exemption argued for one is not an argument
// for the other.
type GuardExempts struct {
	// Host widens only the Host (DNS-rebinding) check, for requests a later
	// gate authenticates by credential instead of by name — today the paired
	// origin passthrough (PairedOriginHostExempt), the paired mirror REST
	// (PairedMirrorHostExempt), and the terminal (PairedTerminalHostExempt),
	// whose Bearer requirements make the rebinding vector unmountable (a
	// browser cannot attach Authorization cross-origin without a preflight
	// this server never answers).
	Host []func(*http.Request) bool
	// Origin widens only the Origin (CSRF) check. Unlike Host exempts, an
	// Origin exempt must validate the credential itself, not just note that
	// one will be demanded later: on a serve with no pairing tokens there is
	// no later gate, and the Origin header is then the only thing between a
	// hostile page in *any* webview and this API (see
	// TestTerminalWebviewOriginCannotOpenTheSocket). Today: the packaged
	// app's webview identity with a proven pairing Bearer
	// (PairedAppOriginExempt, GDK-1120).
	Origin []func(*http.Request) bool
}

// GuardBrowser wraps next so Host/Origin checks run before any route.
// Mount this on the top-level serve mux so routes registered outside Handler
// (/config.json, /healthz, /api/v1/workspaces, /w/) cannot skip the guard.
func GuardBrowser(next http.Handler, ex GuardExempts) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !browserGuard(w, r, ex) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// browserGuard rejects browser-origin CSRF and DNS-rebinding before the mux.
// Returns false when the response was already written (caller must not continue).
//
// Host: every request — only localhost / *.localhost / IP literals (and empty
// Host for HTTP/1.0 clients). DNS names are forbidden so a rebinding name
// cannot read the mirror.
//
// Origin: state-changing methods and WebSocket upgrades — missing Origin is
// allowed (CLI and curl); present Origin must match r.Host exactly (scheme
// http or https).
//
// The upgrade clause is not decoration. A WebSocket handshake is a GET, so
// the method test alone lets it past, and browsers do not apply the
// same-origin policy to WebSocket connections: any page anywhere can open
// ws://gadak.localhost:7777/… and the browser will make the connection. The
// host test does not help, because that is a host this guard allows on
// purpose. The Origin header — which a browser always sends on a handshake
// and cannot be told to omit — is the only thing standing between a hostile
// tab and whatever the socket speaks. Written before the first socket exists
// (the v0.18 terminal pane, GDK-855) rather than after.
func browserGuard(w http.ResponseWriter, r *http.Request, ex GuardExempts) bool {
	if !allowedHost(r.Host) && !anyExempt(ex.Host, r) {
		log.Printf("server: forbidden host %q on %s %s", r.Host, r.Method, r.URL.Path)
		fail(w, http.StatusForbidden, "forbidden_host")
		return false
	}
	if (stateChanging(r.Method) || isWebSocketUpgrade(r)) &&
		!allowedOrigin(r) && !anyExempt(ex.Origin, r) {
		log.Printf("server: forbidden origin %q host %q on %s %s",
			r.Header.Get("Origin"), r.Host, r.Method, r.URL.Path)
		fail(w, http.StatusForbidden, "forbidden_origin")
		return false
	}
	return true
}

func anyExempt(exempts []func(*http.Request) bool, r *http.Request) bool {
	for _, ok := range exempts {
		if ok != nil && ok(r) {
			return true
		}
	}
	return false
}

// isWebSocketUpgrade reports whether r is a WebSocket handshake, so the guard
// can origin-check it even though its method is GET.
//
// Both headers are matched the way RFC 6455 §4.1 and RFC 9110 §7.6.1 say they
// must be: Upgrade is a case-insensitive token, and Connection is a
// comma-separated list that is very often "keep-alive, Upgrade" rather than a
// bare "Upgrade" — matching the whole field value would miss every proxy and
// several browsers.
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, tok := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(tok), "upgrade") {
			return true
		}
	}
	return false
}

func stateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// allowedHost reports whether r.Host is a safe target for this loopback API.
// Empty Host is tolerated (HTTP/1.0, many test harnesses).
func allowedHost(hostport string) bool {
	if hostport == "" {
		return true
	}
	host := stripHostPort(hostport)
	if host == "" {
		return true
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	// Any IP literal is intentional (loopback or --allow-remote LAN). Rebinding
	// needs a DNS name, so accepting IPs does not reopen that attack.
	return net.ParseIP(host) != nil
}

// allowedOrigin is true when Origin is absent or matches r.Host exactly.
func allowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	// Exact host:port match against the request Host (rebinding + CSRF).
	return u.Host == r.Host
}

// stripHostPort returns the host portion of a Host header value, handling
// "name:port", IPv4, and bracketed IPv6 ("[::1]:7777" / "[::1]").
func stripHostPort(hostport string) string {
	// net.SplitHostPort requires a port; it also strips IPv6 brackets.
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	// No port: strip optional IPv6 brackets so ParseIP can see the address.
	if len(hostport) >= 2 && hostport[0] == '[' && hostport[len(hostport)-1] == ']' {
		return hostport[1 : len(hostport)-1]
	}
	return hostport
}
