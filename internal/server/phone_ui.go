package server

import (
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// The phone bundle at /m/ (GDK-1966): the phone opens the serve in its
// browser — same origin, same guard — and gets a small SPA built from
// mobile/ (`make phone`, which writes dist/phone with base /m/). There is
// no pairing and no second app: the address is the credential, exactly as
// for the desktop web UI.

// phonePrefix is the mount this handler serves on the top-level serve mux.
const phonePrefix = "/m/"

// PhoneUIHandler serves the phone bundle. get returns the embedded bundle
// (embed.go's PhoneUI); ok=false means this build shipped without one —
// `make phone` was never run — and every /m/ path answers 503 with the
// fix as the hint, not a bare 404 that reads as "wrong address".
func PhoneUIHandler(get func() (fs.FS, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ui, ok := get()
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "phone_ui_missing",
				"hint":  "make phone",
			})
			return
		}
		servePhoneFS(ui, w, r)
	})
}

// servePhoneFS is the /m/ serving core, the spaHandlerFS shape with two
// differences the phone app needs: paths clean into the mount (never out
// of it), and a missing extension-less path is the SPA shell rather than a
// 404, so a reload mid-view survives client-side routing. A missing path
// WITH an extension still 404s — a typo'd asset URL must fail loudly, not
// boot the app.
func servePhoneFS(ui fs.FS, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		fail(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	// Clean the whole URL before touching the bundle: "/m/../config.json"
	// cleans to "/config.json", which is outside this mount — and outside
	// the mount answers as the mount root (the shell), never as a sibling
	// route's bytes and never as disk. This is the clamp that keeps
	// traversal inside the bundle.
	full := path.Clean("/" + r.URL.Path)
	if full != "/m" && !strings.HasPrefix(full, phonePrefix) {
		full = phonePrefix
	}
	name := strings.TrimSuffix(strings.TrimPrefix(full, phonePrefix), "/")

	if name != "" {
		if st, err := fs.Stat(ui, name); err == nil && !st.IsDir() {
			// Serve real files through http.FileServer for its headers
			// (Content-Type by extension, ranges) — on a request whose
			// path is the in-bundle name, since the mount prefix is not
			// part of the bundle.
			files := http.FileServer(http.FS(ui))
			r2 := r.Clone(r.Context())
			r2.URL = &url.URL{Path: "/" + name, RawQuery: r.URL.RawQuery}
			files.ServeHTTP(w, r2)
			return
		}
		if path.Ext(name) != "" {
			fail(w, http.StatusNotFound, "not_found")
			return
		}
	}
	// The SPA shell: the mount root, an app route, or a directory.
	shell, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(shell)
}

// PhoneURLs is the address list config.json carries as phone_urls (the
// web Devices tab reads it). Every --public-url origin is listed — someone
// pointed the serve at that name on purpose, whatever it cost to get
// there. The listen address contributes one entry only when a phone could
// actually dial it: a non-loopback bind, addressed by the Tailscale
// MagicDNS name when the probe found one (a name survives IP churn and is
// the form the runbook teaches), by a concrete address otherwise. A
// wildcard bind without a name prints nothing — "0.0.0.0" is not an
// address anyone can open.
func PhoneURLs(listenAddr string, publicURLs []string, tsName string) []string {
	out := make([]string, 0, len(publicURLs)+1)
	seen := make(map[string]bool, len(publicURLs)+1)
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	for _, u := range publicURLs {
		add(strings.TrimRight(strings.TrimSpace(u), "/") + phonePrefix)
	}
	if listenAddr == "" {
		return out
	}
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return out
	}
	host = strings.TrimSpace(host)
	tsHost, tsOK := policyHostName(tsName)
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		// Loopback: nothing for a phone to dial. Wildcard: no name of its
		// own — only the Tailscale probe can say what to open.
		if ip.IsUnspecified() && tsOK {
			add(phoneURL(tsHost, port))
		}
		return out
	}
	if tsOK {
		add(phoneURL(tsHost, port))
		return out
	}
	add(phoneURL(host, port))
	return out
}

// phoneURL builds one address list entry, bracketing an IPv6 literal so
// the port separator stays readable.
func phoneURL(host, port string) string {
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		return "http://" + host + ":" + port + phonePrefix
	}
	return "http://" + host + phonePrefix
}
