package origin

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
