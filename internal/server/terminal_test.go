package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/term"
)

// The terminal gate (GDK-862/GDK-863). Three surfaces now take a Bearer,
// and the third one hands out a shell — so every wrong pairing of scope
// and door is pinned here, in both directions.
//
// These tests run against a real httptest listener rather than a
// ResponseRecorder: a WebSocket needs a hijackable connection, and a
// terminal that only works against a recorder is not a terminal.

// remoteIPHost is what an `--allow-remote` bind looks like from the
// client's side: an IP literal that is not loopback. TEST-NET-1, so
// nothing is ever dialed even if a test regresses into trying.
const remoteIPHost = "192.0.2.7:7777"

// hostRewrite makes a request to the test listener carry a different Host
// header than the address it dials — which is exactly what `tailscale
// serve` does upstream, and the only way to exercise the guard's DNS-name
// branch against a real socket.
type hostRewrite struct {
	host string
	base http.RoundTripper
}

func (h hostRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Host = h.host
	return h.base.RoundTrip(r2)
}

func termClient(host string) *http.Client {
	if host == "" {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: hostRewrite{host: host, base: http.DefaultTransport},
	}
}

// termServer is the API behind a real listener.
func termServer(t *testing.T) (*httptest.Server, *Handler, string) {
	t.Helper()
	h, cfg := standaloneServer(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, h, cfg.Directory()
}

// termRequest sends one REST call with an optional Host override and
// Bearer.
func termRequest(t *testing.T, srv *httptest.Server, method, path, body, host, token string) (int, string) {
	t.Helper()
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := termClient(host).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	return resp.StatusCode, string(buf[:n])
}

// createSession is POST sessions/, returning the new id.
func createSession(t *testing.T, srv *httptest.Server, host, token string) string {
	t.Helper()
	code, body := termRequest(t, srv, http.MethodPost, termBase+"sessions/", `{"cols":90,"rows":30}`, host, token)
	if code != http.StatusOK {
		t.Fatalf("create session: %d %s", code, body)
	}
	var doc terminalSessionDoc
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("create body %s: %v", body, err)
	}
	if doc.ID == "" || doc.Cols != 90 || doc.Rows != 30 {
		t.Fatalf("create doc %+v", doc)
	}
	return doc.ID
}

func dialTerminal(t *testing.T, srv *httptest.Server, id, host, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + termBase + "sessions/" + id + "/ws/"
	opts := &websocket.DialOptions{HTTPHeader: http.Header{}, HTTPClient: termClient(host)}
	if token != "" {
		opts.HTTPHeader.Set("Authorization", "Bearer "+token)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return websocket.Dial(ctx, u, opts)
}

// readUntilMarker collects binary frames until want appears, and fails on
// a control frame arriving first (that means the session died).
func readUntilMarker(t *testing.T, c *websocket.Conn, want string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var got strings.Builder
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v (got %q)", want, err, got.String())
		}
		if typ == websocket.MessageText {
			t.Fatalf("waiting for %q, got control frame %s (output so far %q)", want, data, got.String())
		}
		got.Write(data)
		if strings.Contains(got.String(), want) {
			return got.String()
		}
	}
}

// readControl skips output frames and returns the first JSON control frame.
func readControl(t *testing.T, c *websocket.Conn, within time.Duration) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for a control frame: %v", err)
		}
		if typ != websocket.MessageText {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("control frame %s: %v", data, err)
		}
		return msg
	}
}

// ① Loopback opens a shell with no Bearer and echoes both ways — decision
// 0003, the local web UI is the same person as the CLI user. Text frames
// carry control; binary frames carry PTY bytes.
func TestTerminalLoopbackOpensAndEchoes(t *testing.T) {
	srv, _, _ := termServer(t)
	id := createSession(t, srv, "", "")
	c, _, err := dialTerminal(t, srv, id, "", "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("echo gadak-ws-ok\n")); err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, "gadak-ws-ok")

	// Resize is a text control frame, and it must reach the child.
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"t":"resize","cols":120,"rows":40}`)); err != nil {
		t.Fatal(err)
	}
	if err := c.Write(ctx, websocket.MessageBinary, []byte("stty size\n")); err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, "40 120")

	// And an exiting shell reports its status as a control frame.
	if err := c.Write(ctx, websocket.MessageBinary, []byte("exit 5\n")); err != nil {
		t.Fatal(err)
	}
	msg := readControl(t, c, 15*time.Second)
	if msg["t"] != "exit" || msg["code"] != float64(5) {
		t.Fatalf("exit frame %+v; want {t:exit code:5}", msg)
	}
}

// ② The session list is metadata, never output — and it is what a future
// `gadak terminal list` reads.
func TestTerminalListIsMetadataOnly(t *testing.T) {
	srv, _, _ := termServer(t)
	id := createSession(t, srv, "", "")
	code, body := termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", "", "")
	if code != http.StatusOK {
		t.Fatalf("list: %d %s", code, body)
	}
	var doc struct {
		Sessions []term.Info `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Sessions) != 1 || doc.Sessions[0].ID != id {
		t.Fatalf("list %s", body)
	}
	if doc.Sessions[0].PID <= 0 || doc.Sessions[0].Cols != 90 {
		t.Fatalf("list row %+v", doc.Sessions[0])
	}
	// DELETE reaps it.
	code, body = termRequest(t, srv, http.MethodDelete, termBase+"sessions/"+id+"/", "", "", "")
	if code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, body)
	}
	code, body = termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", "", "")
	if code != http.StatusOK || !strings.Contains(body, `"sessions":[]`) {
		t.Fatalf("list after delete: %d %s", code, body)
	}
	// A session id that never existed is a 404, not a 500.
	code, _ = termRequest(t, srv, http.MethodDelete, termBase+"sessions/deadbeef/", "", "", "")
	if code != http.StatusNotFound {
		t.Fatalf("delete unknown id: %d; want 404", code)
	}
}

// ③ A DNS-named Host with no Bearer is told to pair — not silently passed,
// and not answered as if the surface did not exist.
func TestTerminalDNSHostDemandsBearer(t *testing.T) {
	srv, _, dir := termServer(t)
	seedStore(t, dir, seedToken{"pane", pairing.ScopeTerminal})
	code, body := termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", mirrorHost, "")
	if code != http.StatusUnauthorized || !strings.Contains(body, "pairing_rejected") {
		t.Fatalf("no bearer on a DNS host: %d %s; want 401 pairing_rejected", code, body)
	}
	// An unknown bearer is the same answer — missing and wrong must not be
	// distinguishable.
	code, body = termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", mirrorHost, "guessed-token")
	if code != http.StatusUnauthorized || !strings.Contains(body, "pairing_rejected") {
		t.Fatalf("unknown bearer: %d %s; want 401 pairing_rejected", code, body)
	}
}

// ④ The one-way doors, all four wrong pairings.
//
// A serve or origin token on a DNS Host does not even reach the gate: the
// terminal exemption declines a bearer minted for another surface, so the
// rebinding guard answers forbidden_host and a leaked phone token is not
// told where the shell is. The named scope_rejected answer exists for the
// hosts the guard already admits — an `--allow-remote` IP literal, ⑦.
func TestTerminalScopeIsAOneWayDoor(t *testing.T) {
	srv, _, dir := termServer(t)
	toks := seedStore(t, dir,
		seedToken{"pane", pairing.ScopeTerminal},
		seedToken{"phone", pairing.ScopeServe},
		seedToken{"laptop", pairing.ScopeOrigin})
	termTok, serveTok, originTok := toks[0], toks[1], toks[2]

	// serve → terminal route: forbidden_host, from the guard.
	code, body := termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", mirrorHost, serveTok)
	if code != http.StatusForbidden || !strings.Contains(body, "forbidden_host") {
		t.Fatalf("serve token on the terminal route: %d %s; want 403 forbidden_host", code, body)
	}
	// origin → terminal route: same.
	code, body = termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", mirrorHost, originTok)
	if code != http.StatusForbidden || !strings.Contains(body, "forbidden_host") {
		t.Fatalf("origin token on the terminal route: %d %s; want 403 forbidden_host", code, body)
	}
	// terminal → mirror REST: authenticated, not authorized.
	code, body = termRequest(t, srv, http.MethodGet, "/api/v1/issues/bootstrap/", "", mirrorHost, termTok)
	if code != http.StatusForbidden || !strings.Contains(body, "scope_rejected") {
		t.Fatalf("terminal token on the mirror REST: %d %s; want 403 scope_rejected", code, body)
	}
	// terminal → origin passthrough (loopback; that gate has never had a
	// loopback bypass).
	code, body = termRequest(t, srv, http.MethodGet, origin.RESTPrefix+"/rest/api/3/myself", "", "", termTok)
	if code != http.StatusForbidden || !strings.Contains(body, "scope_rejected") {
		t.Fatalf("terminal token on the origin passthrough: %d %s; want 403 scope_rejected", code, body)
	}
}

// ⑤ A terminal-scope Bearer on a DNS Host opens a shell and speaks to it —
// the paired phone's whole reason for existing.
func TestTerminalTokenOpensShellOverDNSHost(t *testing.T) {
	srv, _, dir := termServer(t)
	tok := seedStore(t, dir, seedToken{"pane", pairing.ScopeTerminal})[0]
	id := createSession(t, srv, mirrorHost, tok)
	c, _, err := dialTerminal(t, srv, id, mirrorHost, tok)
	if err != nil {
		t.Fatalf("dial with a terminal token: %v", err)
	}
	defer c.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("echo gadak-paired-shell\n")); err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, "gadak-paired-shell")
}

// ⑥ A paired caller reaches only what its own token opened; a local caller
// is this machine's user and reaches everything, which is how a phone's
// shell gets killed from the keyboard in front of it.
func TestTerminalSessionsAreScopedToTheirToken(t *testing.T) {
	srv, _, dir := termServer(t)
	toks := seedStore(t, dir,
		seedToken{"pane-a", pairing.ScopeTerminal},
		seedToken{"pane-b", pairing.ScopeTerminal})
	a, b := toks[0], toks[1]
	idA := createSession(t, srv, mirrorHost, a)

	if code, _ := termRequest(t, srv, http.MethodDelete, termBase+"sessions/"+idA+"/", "", mirrorHost, b); code != http.StatusNotFound {
		t.Fatalf("another token's session was reachable: %d; want 404", code)
	}
	if _, _, err := dialTerminal(t, srv, idA, mirrorHost, b); err == nil {
		t.Fatal("another token attached to a session it did not open")
	}
	code, body := termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", mirrorHost, b)
	if code != http.StatusOK || !strings.Contains(body, `"sessions":[]`) {
		t.Fatalf("list for another token: %d %s; want an empty list", code, body)
	}
	// Loopback sees and can reap it.
	if code, body := termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", "", ""); code != http.StatusOK || !strings.Contains(body, idA) {
		t.Fatalf("loopback list: %d %s", code, body)
	}
	if code, _ := termRequest(t, srv, http.MethodDelete, termBase+"sessions/"+idA+"/", "", "", ""); code != http.StatusNoContent {
		t.Fatalf("loopback could not reap a paired session: %d", code)
	}
}

// ⑦ `--allow-remote` does not hand out a shell.
//
// 2026-08-25 — GDK-862, implementer's narrowing of the spec's "IP-literal
// Host needs no Bearer": binding a non-loopback address (LAN or tailnet IP)
// makes every IP-literal Host reachable by other machines, and the mirror's
// rule would make that an unauthenticated shell for anyone who can reach
// the port. Publishing the mirror's *data* that way is a documented choice
// (SECURITY.md); publishing the machine is not one anybody made. Only
// loopback is local here.
func TestTerminalAllowRemoteAddressNeedsAToken(t *testing.T) {
	srv, _, dir := termServer(t)
	// Unpaired: a non-loopback address gets nothing at all.
	code, body := termRequest(t, srv, http.MethodPost, termBase+"sessions/", "", remoteIPHost, "")
	if code != http.StatusForbidden || !strings.Contains(body, "forbidden_host") {
		t.Fatalf("unpaired --allow-remote address: %d %s; want 403 forbidden_host", code, body)
	}
	toks := seedStore(t, dir,
		seedToken{"pane", pairing.ScopeTerminal},
		seedToken{"phone", pairing.ScopeServe})
	termTok, serveTok := toks[0], toks[1]

	// Paired, no bearer: pair this device.
	code, body = termRequest(t, srv, http.MethodPost, termBase+"sessions/", "", remoteIPHost, "")
	if code != http.StatusUnauthorized || !strings.Contains(body, "pairing_rejected") {
		t.Fatalf("--allow-remote address, no bearer: %d %s; want 401 pairing_rejected", code, body)
	}
	// Paired, wrong scope: the named answer, because the guard admits IP
	// literals and the gate is what speaks here.
	code, body = termRequest(t, srv, http.MethodPost, termBase+"sessions/", "", remoteIPHost, serveTok)
	if code != http.StatusForbidden || !strings.Contains(body, "scope_rejected") {
		t.Fatalf("serve token on an --allow-remote address: %d %s; want 403 scope_rejected", code, body)
	}
	// Right scope: through.
	if code, body := termRequest(t, srv, http.MethodPost, termBase+"sessions/", "", remoteIPHost, termTok); code != http.StatusOK {
		t.Fatalf("terminal token on an --allow-remote address: %d %s; want 200", code, body)
	}
	// Loopback is still unauthenticated (decision 0003), tokens or not.
	if code, body := termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", "", ""); code != http.StatusOK {
		t.Fatalf("loopback with tokens minted: %d %s; want 200", code, body)
	}
}

// ⑧ Revoking the token cuts the shell it opened, without waiting for the
// next request. The socket is told why, the process group is killed, and
// the session leaves the manager.
//
// The poll interval is shortened here; the production value is 2s and the
// contract is "gone within 3s".
func TestTerminalRevokeCutsLiveSession(t *testing.T) {
	prev := terminalPollInterval
	terminalPollInterval = 50 * time.Millisecond
	t.Cleanup(func() { terminalPollInterval = prev })

	srv, _, dir := termServer(t)
	// A real mint, not a seeded file: revoke has to find the same row the
	// gate authenticated against, and `pairing revoke` selects by label.
	tok, meta, err := pairing.MintScoped(dir, "pane-live", pairing.ScopeTerminal, time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Scope != pairing.ScopeTerminal {
		t.Fatalf("minted scope %q", meta.Scope)
	}
	id := createSession(t, srv, mirrorHost, tok)
	c, _, err := dialTerminal(t, srv, id, mirrorHost, tok)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("echo gadak-live-before-revoke\n")); err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, "gadak-live-before-revoke")

	if _, err := pairing.Revoke(dir, "pane-live", time.Now()); err != nil {
		t.Fatal(err)
	}
	msg := readControl(t, c, 3*time.Second)
	if msg["t"] != "dropped" || msg["reason"] != term.ReasonRevoked {
		t.Fatalf("control frame %+v; want {t:dropped reason:%s}", msg, term.ReasonRevoked)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		code, body := termRequest(t, srv, http.MethodGet, termBase+"sessions/", "", "", "")
		if code == http.StatusOK && strings.Contains(body, `"sessions":[]`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session survived its token's revoke: %d %s", code, body)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ⑨ Shutdown reaps the shells. A serve that exits leaving a shell behind
// is the orphan class one level up from term.Session.Close.
func TestTerminalShutdownReapsSessions(t *testing.T) {
	srv, h, _ := termServer(t)
	id := createSession(t, srv, "", "")
	if _, err := h.s.terminalManager().Get(id); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if n := len(h.s.terminalManager().Snapshot()); n != 0 {
		t.Fatalf("%d session(s) survived shutdown", n)
	}
}

// ⑩ The terminal path is outside the mirror REST by construction, so the
// mirror's own allowlist can never grow a shell (mirror_gate.go says so in
// a comment; this says it in a test).
func TestTerminalPathIsNotMirrorREST(t *testing.T) {
	for _, path := range []string{
		termBase,
		termBase + "sessions/",
		termBase + "sessions/abc/ws/",
	} {
		if serveScopeAdmits(http.MethodGet, path) {
			t.Errorf("serveScopeAdmits(%q) is true — a serve token would reach a shell", path)
		}
		if !terminalPathAdmits(path) {
			t.Errorf("terminalPathAdmits(%q) is false", path)
		}
	}
	for _, path := range []string{"/api/v1/issues/bootstrap/", "/api/v1/auth/me/", "/", "/api/v1/terminals/"} {
		if terminalPathAdmits(path) {
			t.Errorf("terminalPathAdmits(%q) is true — the exemption is wider than the surface", path)
		}
	}
}

// ⑪ terminalLocalHost is the loopback rule itself, in one table.
func TestTerminalLocalHostIsLoopbackOnly(t *testing.T) {
	for host, want := range map[string]bool{
		"":                              true,
		"localhost:7777":                true,
		"gadak.localhost:7777":          true,
		"127.0.0.1:7777":                true,
		"[::1]:7777":                    true,
		"::1":                           true,
		"192.0.2.7:7777":                false,
		"198.51.100.1:7777":             false,
		"home.tailnet.example.com:8443": false,
	} {
		if got := terminalLocalHost(host); got != want {
			t.Errorf("terminalLocalHost(%q) = %v, want %v", host, got, want)
		}
	}
}

// ⑫ The loopback rule reads the connection, not just the header. Host is
// whatever the client typed: with `--allow-remote`, a remote peer sending
// `Host: localhost` must not become this machine's user.
//
// 2026-08-25 — GDK-863 (lead, post-round hardening): FAIL-first — before
// terminalLocal checked RemoteAddr, this table's remote-peer rows passed.
func TestTerminalLocalNeedsLoopbackPeer(t *testing.T) {
	for _, tc := range []struct {
		host, peer string
		want       bool
	}{
		{"localhost:7877", "127.0.0.1:50000", true},
		{"127.0.0.1:7877", "[::1]:50000", true},
		{"localhost:7877", "", true}, // in-process, no network peer
		{"localhost:7877", "192.0.2.9:50000", false},
		{"127.0.0.1:7877", "198.51.100.1:50000", false},
		{"192.0.2.7:7877", "127.0.0.1:50000", false},
	} {
		r := httptest.NewRequest(http.MethodGet, termBase+"sessions/", nil)
		r.Host = tc.host
		r.RemoteAddr = tc.peer
		if got := terminalLocal(r); got != tc.want {
			t.Errorf("terminalLocal(host=%q peer=%q) = %v, want %v", tc.host, tc.peer, got, tc.want)
		}
	}
}

// ⑨ An app webview may not open this socket, and that is why the phone's
// transport is native.
//
// The mobile companion (GDK-865) runs inside a WKWebView whose origin is a
// custom scheme, and a webview WebSocket can neither omit that Origin nor
// set an Authorization header. Both halves are refused here: the custom
// scheme dies at the guard's Origin check, and a socket with no bearer at
// all dies at the gate. The phone therefore dials natively — no Origin, a
// real header — which is ⑤ above.
//
// This is pinned because the cheap "fix" for a desktop app is to teach
// allowedOrigin about custom schemes, and that change would quietly hand a
// webview — anyone's webview — a shell on this machine. Measured against a
// live serve on a LAN address before it was written: 101 for the native
// dial, 403 forbidden_origin with `Origin: tauri://localhost`.
func TestTerminalWebviewOriginCannotOpenTheSocket(t *testing.T) {
	srv, _, dir := termServer(t)
	tok := seedStore(t, dir, seedToken{"phone", pairing.ScopeTerminal})[0]
	id := createSession(t, srv, mirrorHost, tok)

	u := "ws" + strings.TrimPrefix(srv.URL, "http") + termBase + "sessions/" + id + "/ws/"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The third case is the one that isolates the scheme rule: its host is
	// the request's own Host, so only "a custom scheme is not http(s)" can
	// refuse it. Drop that clause from allowedOrigin and this row alone
	// turns green — which is what "teach it about custom schemes" means.
	for _, origin := range []string{
		"tauri://localhost",
		"capacitor://localhost",
		"null",
		"tauri://" + mirrorHost,
	} {
		hdr := http.Header{}
		hdr.Set("Authorization", "Bearer "+tok)
		hdr.Set("Origin", origin)
		c, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{
			HTTPHeader: hdr,
			HTTPClient: termClient(mirrorHost),
		})
		if err == nil {
			c.CloseNow()
			t.Fatalf("Origin %q opened the terminal socket; want it refused", origin)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			code := 0
			if resp != nil {
				code = resp.StatusCode
			}
			t.Fatalf("Origin %q: status %d (%v); want 403", origin, code, err)
		}
	}
}
