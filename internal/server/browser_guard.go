package server

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// browserGuard rejects browser-origin CSRF and DNS-rebinding before the mux.
// Returns false when the response was already written (caller must not continue).
//
// Host: every request — only localhost / *.localhost / IP literals (and empty
// Host for HTTP/1.0 clients). DNS names are forbidden so a rebinding name
// cannot read the mirror.
//
// Origin: state-changing methods only — missing Origin is allowed (CLI/TUI/
// curl); present Origin must match r.Host exactly (scheme http or https).
func browserGuard(w http.ResponseWriter, r *http.Request) bool {
	if !allowedHost(r.Host) {
		log.Printf("server: forbidden host %q on %s %s", r.Host, r.Method, r.URL.Path)
		fail(w, http.StatusForbidden, "forbidden_host")
		return false
	}
	if stateChanging(r.Method) && !allowedOrigin(r) {
		log.Printf("server: forbidden origin %q host %q on %s %s",
			r.Header.Get("Origin"), r.Host, r.Method, r.URL.Path)
		fail(w, http.StatusForbidden, "forbidden_origin")
		return false
	}
	return true
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
