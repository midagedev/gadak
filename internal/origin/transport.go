package origin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
// onto a live gadak serve's origin passthrough. Inner rt is DefaultTransport
// so this RoundTrip cannot recurse into the same Client.
type serveOriginTransport struct {
	host string
	rt   http.RoundTripper
}

func newServeOriginTransport(host string) *serveOriginTransport {
	return &serveOriginTransport{host: host, rt: http.DefaultTransport}
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
	return t.rt.RoundTrip(req2)
}
