package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/term"
)

// The terminal surface (GDK-862/GDK-863): the PTY sessions `gadak serve`
// runs, and the WebSocket that carries their bytes. internal/term owns the
// shells; this file owns who may reach one.
//
// termBase sits outside apiBase/authBase/dashBase deliberately, so
// serveScopeAdmits (mirror_gate.go) is false for it by construction. A
// phone's serve token cannot open a shell by any path that exists here,
// and it does not get to learn that the path exists: the rebinding guard's
// terminal exemption declines a wrong-scope bearer, so a serve token on a
// DNS Host is answered forbidden_host, the same as before this route was
// written.
const termBase = "/api/v1/terminal/"

// terminalWriteTimeout bounds one socket write. A client whose TCP window
// has closed must not hold the writer loop: past this the attachment ends
// and the session keeps running for the reconnect grace.
const terminalWriteTimeout = 10 * time.Second

// terminalReadLimit is the largest single client→server message. Keyboard
// input is bytes; a paste is not, so the default 32 KiB is too small.
const terminalReadLimit = 1 << 20

// terminalPollInterval is how often the serve re-reads the token store for
// live sessions. See terminalRevokeWatch for why this is a poll.
var terminalPollInterval = 2 * time.Second

// registerTerminal adds the terminal routes to the API mux. Kept out of
// newServer's body because the gate wraps every one of them.
func (s *server) registerTerminal(mux *http.ServeMux) {
	mux.Handle("POST "+termBase+"sessions/{$}", s.terminalRoute(s.handleTerminalCreate))
	mux.Handle("GET "+termBase+"sessions/{$}", s.terminalRoute(s.handleTerminalList))
	mux.Handle("DELETE "+termBase+"sessions/{id}/{$}", s.terminalRoute(s.handleTerminalDelete))
	mux.Handle("GET "+termBase+"sessions/{id}/ws/{$}", s.terminalRoute(s.handleTerminalWS))
}

// terminalManager is this server's session core, built on first use so a
// serve that never opens a terminal pays nothing for it.
func (s *server) terminalManager() *term.Manager {
	s.termMu.Lock()
	defer s.termMu.Unlock()
	if s.termMgr == nil {
		dir := ""
		if cfg := s.config(); cfg != nil {
			dir = cfg.Directory()
		}
		s.termMgr = term.New(term.Config{WorkDir: dir})
	}
	return s.termMgr
}

// Terminals is the session core, for an in-process host that carries the
// terminal over its own transport instead of the WebSocket below.
//
// Gadak.app is that host (GDK-892): it mounts this Handler behind the wails
// asset server, where there is no TCP listener for a ws:// URL to reach, and
// moves the same bytes over a wails GoStream. Everything above the socket —
// create, list, delete, the gate — is the REST surface it already uses.
//
// Lazy exactly as the HTTP path is: a process that never opens a terminal
// still constructs no manager, because this is the same call handleTerminal*
// makes.
func (h *Handler) Terminals() *term.Manager { return h.s.terminalManager() }

// closeTerminals reaps every shell this server started. Shutdown calls it:
// an exiting serve that leaves shells behind is the orphan class
// term.Session.Close exists to close, one level up.
func (s *server) closeTerminals() {
	s.termMu.Lock()
	mgr := s.termMgr // nil when no terminal was ever opened: nothing to reap
	s.termMu.Unlock()
	if mgr != nil {
		mgr.CloseAll()
	}
}

// terminalRoute is the gate every terminal handler sits behind. Handlers
// receive the pairing token id the request authenticated with — empty for
// a local client, which needs none.
func (s *server) terminalRoute(fn func(http.ResponseWriter, *http.Request, string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenID, ok := s.terminalGate(w, r)
		if !ok {
			return
		}
		fn(w, r, tokenID)
	})
}

// terminalGate is the third gate, the same shape as pairingGate and
// mirrorGate: the guard vouches for the Host, the gate vouches for the
// token, neither trusts the other's half.
//
//	local Host (loopback, *.localhost, empty) → no Bearer, tokenID ""
//	anything else, store unreadable           → 500 internal_error
//	anything else, no active tokens           → 403 forbidden_host
//	active tokens, no/unknown Bearer          → 401 pairing_rejected
//	valid Bearer, any other scope             → 403 scope_rejected
//	terminal-scope Bearer                     → through, tokenID = its hash
//
// The local rule is decision 0003 — the loopback web UI is the same person
// as the CLI user, and the browser guard's Origin check on the upgrade is
// what stands between that surface and a hostile tab (GDK-855).
//
// It is narrower here than on the mirror: terminalLocalHost accepts only
// *loopback* IP literals, where allowedHost accepts every IP literal.
// `--allow-remote` binds a LAN or tailnet address, and a request arriving
// at that address carries an IP-literal Host — under the mirror's rule
// that would be an unauthenticated shell for anyone who can reach the
// port. Publishing the mirror's data that way is a documented sharp edge
// (SECURITY.md); publishing the machine that way is not one anybody
// chose. A non-loopback address therefore needs a terminal token, exactly
// as a DNS name does.
func (s *server) terminalGate(w http.ResponseWriter, r *http.Request) (string, bool) {
	if terminalLocal(r) {
		return "", true
	}
	cfg := s.config()
	if cfg == nil {
		log.Printf("server: terminal gate: no config for %s %s", r.Method, r.URL.Path)
		fail(w, http.StatusForbidden, "forbidden_host")
		return "", false
	}
	verdict, meta, err := pairing.AuthorizeMeta(cfg.Directory(), bearerToken(r), time.Now())
	if err != nil {
		log.Printf("server: terminal gate: %s %s: %v", r.Method, r.URL.Path, err)
		fail(w, http.StatusInternalServerError, "internal_error")
		return "", false
	}
	switch verdict {
	case pairing.VerdictOff:
		// No active token exists, so nothing can vouch for this Host.
		fail(w, http.StatusForbidden, "forbidden_host")
		return "", false
	case pairing.VerdictReject:
		_, reason := pairing.Explain(cfg.Directory(), bearerToken(r), time.Now())
		failPairing(w, reason)
		return "", false
	case pairing.VerdictAccept:
		if !pairing.AdmitsTerminal(meta.Scope) {
			log.Printf("server: terminal gate: label %q scope %q denied on %s %s",
				meta.Label, meta.Scope, r.Method, r.URL.Path)
			fail(w, http.StatusForbidden, "scope_rejected")
			return "", false
		}
	}
	return meta.Hash, true
}

// terminalLocal is "this machine's own user is asking", and it reads both
// halves of the connection: the Host header (what the client typed) and
// the peer address (where the bytes came from). Host alone is not enough —
// under `--allow-remote` a remote peer can send `Host: localhost` and the
// header rule would hand it a shell (GDK-863, 2026-08-25 hardening). The
// mirror accepts that risk for data; the terminal does not for the machine.
// An empty RemoteAddr is an in-process call with no network peer at all.
func terminalLocal(r *http.Request) bool {
	if !terminalLocalHost(r.Host) {
		return false
	}
	if r.RemoteAddr == "" {
		return true
	}
	ip := net.ParseIP(stripHostPort(r.RemoteAddr))
	return ip != nil && ip.IsLoopback()
}

// terminalLocalHost is the Host half of terminalLocal: loopback names and
// loopback IP literals only — see terminalGate for why an `--allow-remote`
// address is not on this list.
func terminalLocalHost(hostport string) bool {
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
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// terminalPathAdmits reports whether a path is on the terminal surface.
// Deliberately a prefix on termBase and nothing wider: the exemption below
// must not become a hole in the rebinding guard for any other route.
func terminalPathAdmits(path string) bool {
	return strings.HasPrefix(path, termBase)
}

// PairedTerminalHostExempt lets GuardBrowser pass a DNS-named Host for
// terminal requests, the third exemption beside the origin and mirror
// ones — but a stricter probe than either.
//
// The other two ask only "does the gate have anything to check" with an
// empty bearer, and let the gate answer everything else. This one also
// refuses a bearer that authenticates for another surface: a serve or
// origin token on the terminal route is not exempted, so it dies at the
// guard as forbidden_host instead of learning that a shell endpoint
// exists. That is the GDK-863 ruling taken literally — a serve token may
// never open a shell, and there is no reason to tell it where the shell
// is. A request with no bearer, or an unknown one, is still exempted, so
// the gate can answer it 401 with a reason (an honest "pair this device"
// for someone who has not yet).
func PairedTerminalHostExempt(dir func() string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		if !terminalPathAdmits(r.URL.Path) {
			return false
		}
		d := dir()
		if d == "" {
			return false
		}
		verdict, meta, err := pairing.AuthorizeMeta(d, bearerToken(r), time.Now())
		if err != nil {
			return false
		}
		switch verdict {
		case pairing.VerdictReject:
			// Tokens exist; this bearer is not one of them (or is absent).
			return true
		case pairing.VerdictAccept:
			return pairing.AdmitsTerminal(meta.Scope)
		}
		return false
	}
}

// terminalSessionDoc is the create/list row: identity and geometry, never
// output. term.Info carries no token id, which is why it can be served.
type terminalSessionDoc struct {
	ID   string `json:"id"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type terminalCreateReq struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (s *server) handleTerminalCreate(w http.ResponseWriter, r *http.Request, tokenID string) {
	var req terminalCreateReq
	if body, err := io.ReadAll(io.LimitReader(r.Body, 4<<10)); err == nil && len(body) > 0 {
		// A body is optional: the pane may not know its size yet.
		_ = json.Unmarshal(body, &req)
	}
	sess, err := s.terminalManager().Create(term.Options{
		Cols:    req.Cols,
		Rows:    req.Rows,
		TokenID: tokenID,
	})
	if err != nil {
		if errors.Is(err, term.ErrUnsupportedPlatform) {
			// Honest, and named: the pane can say "not on Windows yet".
			failMsg(w, http.StatusNotImplemented, "terminal_unsupported", err.Error())
			return
		}
		log.Printf("server: terminal create: %v", err)
		failMsg(w, http.StatusInternalServerError, "terminal_failed", err.Error())
		return
	}
	if tokenID != "" {
		s.startTerminalRevokeWatch()
	}
	info := sess.Info()
	log.Printf("server: terminal create: session %s", info.ID)
	writeJSON(w, http.StatusOK, terminalSessionDoc{ID: info.ID, Cols: info.Cols, Rows: info.Rows})
}

func (s *server) handleTerminalList(w http.ResponseWriter, r *http.Request, tokenID string) {
	_ = r
	all := s.terminalManager().Snapshot()
	out := make([]term.Info, 0, len(all))
	for _, info := range all {
		if !s.terminalVisible(info.ID, tokenID) {
			continue
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, struct {
		Sessions []term.Info `json:"sessions"`
	}{out})
}

func (s *server) handleTerminalDelete(w http.ResponseWriter, r *http.Request, tokenID string) {
	sess, err := s.terminalManager().Get(r.PathValue("id"))
	if err != nil || !terminalOwns(sess.TokenID(), tokenID) {
		// A session another credential opened is not distinguished from
		// one that does not exist.
		handleNotFound(w, r)
		return
	}
	_ = sess.Close()
	w.WriteHeader(http.StatusNoContent)
}

// terminalOwns decides whether a caller may act on a session. A local
// caller is this machine's user and may reach every session — including
// killing a phone's shell, which is the whole point of having the machine
// in front of you. A paired caller reaches only what its own token opened.
func terminalOwns(sessionToken, callerToken string) bool {
	return callerToken == "" || sessionToken == callerToken
}

func (s *server) terminalVisible(id, callerToken string) bool {
	if callerToken == "" {
		return true
	}
	sess, err := s.terminalManager().Get(id)
	return err == nil && sess.TokenID() == callerToken
}

// handleTerminalWS is the socket. Framing, kept as small as it can be:
//
//	binary frame, either way → PTY bytes
//	text frame, client→server → {"t":"resize","cols":N,"rows":N}
//	text frame, server→client → {"t":"exit","code":N}
//	                            {"t":"dropped","reason":"…"}
//
// The renderer adds nothing to this. Every other question a pane might ask
// — what sessions exist, how big they are — is a REST call above.
func (s *server) handleTerminalWS(w http.ResponseWriter, r *http.Request, tokenID string) {
	sess, err := s.terminalManager().Get(r.PathValue("id"))
	if err != nil || !terminalOwns(sess.TokenID(), tokenID) {
		handleNotFound(w, r)
		return
	}
	att, err := sess.Attach()
	if err != nil {
		handleNotFound(w, r)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// browserGuard already ran the Origin check on this upgrade, and
		// it is stricter than OriginPatterns: exact host:port equality
		// against r.Host, with `null` refused (browser_guard.go). A
		// second, looser check here could only disagree with it.
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		att.Detach()
		log.Printf("server: terminal upgrade: %v", err)
		return
	}
	c.SetReadLimit(terminalReadLimit)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go terminalReadLoop(ctx, cancel, c, sess)

	for {
		select {
		case <-att.Wake():
			if chunk := att.Take(); len(chunk) > 0 {
				if err := terminalWrite(ctx, c, websocket.MessageBinary, chunk); err != nil {
					att.Detach()
					_ = c.CloseNow()
					return
				}
			}
		case <-att.Done():
			terminalFlush(ctx, c, att)
			terminalSendEnd(ctx, c, att.End())
			_ = c.Close(websocket.StatusNormalClosure, "")
			return
		case <-ctx.Done():
			// The client went away (read error or close frame).
			att.Detach()
			_ = c.CloseNow()
			return
		}
	}
}

// terminalReadLoop is client→server: keystrokes and resizes. It owns
// cancel, so a dead socket ends the writer too.
func terminalReadLoop(ctx context.Context, cancel context.CancelFunc, c *websocket.Conn, sess *term.Session) {
	defer cancel()
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			if _, err := sess.Write(data); err != nil {
				return
			}
		case websocket.MessageText:
			var msg struct {
				T    string `json:"t"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if msg.T == "resize" {
				// A bad size is the client's bug, not a reason to drop
				// the shell it is attached to.
				_ = sess.Resize(msg.Cols, msg.Rows)
			}
		}
	}
}

// terminalFlush writes whatever the attachment still had pending when it
// ended, so a shell's last line arrives before the exit frame. Take()
// returns the whole backlog in one slice — a backlog that was pending at
// the end stays readable (internal/term), which is what makes this drain
// safe.
func terminalFlush(ctx context.Context, c *websocket.Conn, att *term.Attachment) {
	if chunk := att.Take(); len(chunk) > 0 {
		_ = terminalWrite(ctx, c, websocket.MessageBinary, chunk)
	}
}

func terminalSendEnd(ctx context.Context, c *websocket.Conn, end term.End) {
	var payload any
	switch end.Kind {
	case term.EndExited:
		payload = struct {
			T    string `json:"t"`
			Code int    `json:"code"`
		}{"exit", end.Code}
	case term.EndDropped, term.EndClosed:
		reason := end.Reason
		if reason == "" {
			reason = term.ReasonClosed
		}
		payload = struct {
			T      string `json:"t"`
			Reason string `json:"reason"`
		}{"dropped", reason}
	default:
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = terminalWrite(ctx, c, websocket.MessageText, data)
}

func terminalWrite(ctx context.Context, c *websocket.Conn, typ websocket.MessageType, p []byte) error {
	wctx, cancel := context.WithTimeout(ctx, terminalWriteTimeout)
	defer cancel()
	return c.Write(wctx, typ, p)
}

// startTerminalRevokeWatch runs the revoke watchdog once per server, from
// the first session that was opened with a pairing token.
//
// This is a poll, not a callback, and the reason is process boundaries:
// `gadak pairing revoke` runs in a *different* process from `gadak serve`
// and can only edit pairing.json. A hook inside internal/pairing would
// fire in the CLI, where no session lives. The store is already re-read by
// stat on every gated request (loadCached), so this costs one stat every
// terminalPollInterval, and only while a token-bound shell is open.
func (s *server) startTerminalRevokeWatch() {
	s.termWatchOnce.Do(func() {
		s.jobsWG.Go(func() { s.terminalRevokeWatch(s.jobsCtx) })
	})
}

func (s *server) terminalRevokeWatch(ctx context.Context) {
	t := time.NewTicker(terminalPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		mgr := s.terminalManager()
		ids := mgr.TokenIDs()
		if len(ids) == 0 {
			continue
		}
		cfg := s.config()
		if cfg == nil {
			continue
		}
		dir := cfg.Directory()
		now := time.Now()
		for _, id := range ids {
			if pairing.TokenActive(dir, id, now) {
				continue
			}
			// Revoked or expired: the shell goes with the token. The id
			// is a SHA-256 hash, never the token itself.
			if n := mgr.CloseByToken(id); n > 0 {
				log.Printf("server: terminal: token %s… is no longer active; closed %d session(s)", id[:8], n)
			}
		}
	}
}
