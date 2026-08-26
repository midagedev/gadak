package origin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
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
	// actor/actorName stamp X-Issuetap-Actor/-Name on requests bound for
	// the embedded issuetap origin (GDK-586). Empty keeps the identity the
	// origin already infers from the in-process Basic credential.
	actor     string
	actorName string
}

func (t *handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.h == nil {
		return nil, errors.New("origin: nil handler transport")
	}
	if req == nil {
		return nil, errors.New("origin: nil request")
	}
	if t.actor != "" {
		// The request never leaves the process, and the jira client builds
		// one request per call, so stamping in place cannot race a reuse.
		req.Header.Set("X-Issuetap-Actor", t.actor)
		if t.actorName != "" {
			req.Header.Set("X-Issuetap-Actor-Name", t.actorName)
		}
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
	// actor/actorName stamp X-Issuetap-Actor/-Name on the rewritten request
	// (GDK-586). The serve passthrough forwards them, so a CLI routing
	// through its live serve attributes to its own agent, not the serve's
	// identity. Empty sends nothing.
	actor     string
	actorName string
	rt        http.RoundTripper
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
	if t.actor != "" {
		req2.Header.Set("X-Issuetap-Actor", t.actor)
		if t.actorName != "" {
			req2.Header.Set("X-Issuetap-Actor-Name", t.actorName)
		}
	}
	resp, err := t.rt.RoundTrip(req2)
	// Local serve routing (scheme empty) stays the pre-pairing shape —
	// folding is only for a remote paired endpoint (GDK-453).
	if t.scheme == "" {
		return resp, err
	}
	return foldPairedRoundTrip(t, resp, err)
}

// PairingError is the remote-device first line for a failed home-serve
// round trip: cause and next action, no REST method/path.
type PairingError struct {
	msg    string
	reason string
}

func (e *PairingError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

func unreachableError(endpoint string) *PairingError {
	return &PairingError{
		msg:    fmt.Sprintf("pairing: cannot reach the home serve at %s — is 'gadak serve' running on the home machine?", endpoint),
		reason: "unreachable",
	}
}

func refusedError(reason string) *PairingError {
	switch reason {
	case string(pairing.ReasonRevoked):
		return &PairingError{
			msg:    "pairing: this device's token was revoked on the home machine — pair again with a fresh offer (home: gadak pairing mint --label <new-device>)",
			reason: reason,
		}
	case string(pairing.ReasonExpired):
		return &PairingError{
			msg:    "pairing: this device's token expired — ask the home machine for a fresh offer (gadak pairing mint)",
			reason: reason,
		}
	default:
		return &PairingError{
			msg:    "pairing: the home serve refused this device's token — mint a fresh offer on the home machine",
			reason: string(pairing.ReasonUnknown),
		}
	}
}

type pairedReject struct {
	endpoint string
	reason   string
}

var lastPairedReject atomic.Pointer[pairedReject]

func rememberPairedReject(endpoint, reason string) {
	lastPairedReject.Store(&pairedReject{endpoint: strings.TrimRight(endpoint, "/"), reason: reason})
}

func (t *serveOriginTransport) endpointURL() string {
	scheme := t.scheme
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + t.host
}

func foldPairedRoundTrip(t *serveOriginTransport, resp *http.Response, err error) (*http.Response, error) {
	ep := t.endpointURL()
	if err != nil {
		if isUnreachable(err) {
			return nil, unreachableError(ep)
		}
		return nil, err
	}
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		rememberPairedReject(ep, peekPairingReason(resp))
	}
	return resp, nil
}

func peekPairingReason(resp *http.Response) string {
	if resp == nil {
		return string(pairing.ReasonUnknown)
	}
	if h := strings.TrimSpace(resp.Header.Get("X-Gadak-Pairing")); h != "" {
		return h
	}
	if resp.Body == nil {
		return string(pairing.ReasonUnknown)
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(data))
	if readErr != nil {
		return string(pairing.ReasonUnknown)
	}
	var doc struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return string(pairing.ReasonUnknown)
	}
	if strings.TrimSpace(doc.Reason) != "" {
		return doc.Reason
	}
	return string(pairing.ReasonUnknown)
}

func isUnreachable(err error) bool {
	if err == nil {
		return false
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return true
	}
	s := strings.ToLower(err.Error())
	for _, n := range []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"no such host",
		"network is unreachable",
		"server misbehaving",
	} {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func isAuth(err error) bool {
	return errors.Is(err, jira.ErrAuth)
}

// IsPairingFailure reports a pairing/dial/auth failure that create must
// not relabel as a missing --project flag.
func IsPairingFailure(err error) bool {
	if err == nil {
		return false
	}
	var p *PairingError
	if errors.As(err, &p) {
		return true
	}
	return isAuth(err) || isUnreachable(err)
}

// FoldPairedError is the single owner of the remote-device pairing
// sentence. On a paired workspace, 401 and dial failures become a
// PairingError whose Error() is the first line a person should see.
// Other workspaces pass err through.
func FoldPairedError(cfg *config.Config, err error) error {
	if err == nil {
		return nil
	}
	rem, rerr := pairedRemote(cfg)
	if rerr != nil || rem == nil {
		return err
	}
	var p *PairingError
	if errors.As(err, &p) {
		return p
	}
	if isAuth(err) {
		reason := string(pairing.ReasonUnknown)
		if got := lastPairedReject.Load(); got != nil && strings.TrimRight(got.endpoint, "/") == strings.TrimRight(rem.Endpoint, "/") {
			reason = got.reason
		}
		return refusedError(reason)
	}
	if isUnreachable(err) {
		return unreachableError(strings.TrimRight(rem.Endpoint, "/"))
	}
	return err
}
