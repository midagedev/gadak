package origin

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/midagedev/gadak/internal/pairing"
)

// handlerTransport sends the Jira client's HTTP requests to an in-process
// http.Handler. Nothing leaves the process: there is no dial, no TLS, and
// no site origin.
//
// internal/jira/write.go MediaRef copies c.HTTP.Transport onto its
// no-follow client, so this injection reaches that path too.
type handlerTransport struct {
	h http.Handler
}

func (t *handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.h == nil {
		return nil, errors.New("origin: nil handler transport")
	}
	if req == nil {
		return nil, errors.New("origin: nil request")
	}
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// serveOriginTransport rewrites a site-relative Jira/Confluence request
// onto a gadak serve's origin passthrough. Inner rt is DefaultTransport
// so this RoundTrip cannot recurse into the same Client.
//
// bearer carries a pairing device token when the target serve has the
// pairing gate on (GDK-433); scheme extends the historical loopback http
// dial to an https endpoint (tailscale serve). Both empty is the
// pre-pairing shape: byte-identical behavior on the disabled path.
type serveOriginTransport struct {
	host   string
	scheme string
	bearer string
	rt     http.RoundTripper
}

func newServeOriginTransport(host string) *serveOriginTransport {
	return &serveOriginTransport{host: host, rt: http.DefaultTransport}
}

// newRemoteOriginTransport builds the passthrough transport for a paired
// remote serve: endpoint is the advertised serve URL (http or https) and
// bearer the device token from the pairing offer. This is the DC-PAT
// shape — Authorization: Bearer — not the Cloud Basic email:token, and it
// replaces whatever Authorization the client set, which never leaves this
// transport anyway.
func newRemoteOriginTransport(endpoint, bearer string) (*serveOriginTransport, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, fmt.Errorf("origin: pairing endpoint: %w", err)
	}
	if u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("origin: pairing endpoint %q must be an http(s) URL", endpoint)
	}
	if strings.TrimPrefix(u.Path, "/") != "" {
		return nil, fmt.Errorf("origin: pairing endpoint %q must not carry a path", endpoint)
	}
	if bearer == "" {
		return nil, errors.New("origin: pairing transport needs a token")
	}
	return &serveOriginTransport{host: u.Host, scheme: u.Scheme, bearer: bearer, rt: http.DefaultTransport}, nil
}

// TransportIsEmbedded reports whether c talks to an in-process issuetap
// handler. Tests use this instead of naming the unexported type.
func TransportIsEmbedded(rt http.RoundTripper) bool {
	_, ok := rt.(*handlerTransport)
	return ok
}

// TransportIsServe reports whether c talks to a live serve passthrough.
func TransportIsServe(rt http.RoundTripper) bool {
	_, ok := rt.(*serveOriginTransport)
	return ok
}

func (t *serveOriginTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.rt == nil {
		return nil, errors.New("origin: nil serve transport")
	}
	if req == nil {
		return nil, errors.New("origin: nil request")
	}
	path := req.URL.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := *req.URL
	u.Scheme = "http"
	if t.scheme != "" {
		u.Scheme = t.scheme
	}
	u.Host = t.host
	u.Path = RESTPrefix + path
	u.RawPath = ""
	if req.URL.RawPath != "" {
		raw := req.URL.RawPath
		if !strings.HasPrefix(raw, "/") {
			raw = "/" + raw
		}
		u.RawPath = RESTPrefix + raw
	}
	u.User = nil
	req2 := req.Clone(req.Context())
	req2.URL = &u
	req2.Host = t.host
	req2.RequestURI = ""
	if req2.Header == nil {
		req2.Header = make(http.Header)
	}
	if t.bearer != "" {
		req2.Header.Set("Authorization", "Bearer "+t.bearer)
	}
	return t.rt.RoundTrip(req2)
}

// localRoutingToken is the device token a same-machine CLI presents when
// it routes through this profile's live serve. The home machine's own
// writes take the passthrough too, and once any token is minted the gate
// has no loopback bypass — so the minting machine stores its own token in
// the pairing credential file and the router picks it up here. Absent
// file (the common case) returns "" and routing is byte-identical to
// before.
func localRoutingToken(cfgDir string) string {
	rem, err := pairing.LoadRemote(cfgDir)
	if err != nil || rem == nil {
		return ""
	}
	return rem.Token
}
