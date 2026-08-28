package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/midagedev/gadak/internal/config"
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

// registerTerminal mounts the terminal surface on the API mux: one
// sub-mux owns every route under termBase, and that sub-mux is mounted
// behind a single gate wrap. The gate is a value in the request path
// (GDK-912), the same shape as Handler.guarded — not a convention each
// registration has to remember. There is no second registration surface:
// a route added to the sub-mux inherits the gate, and a termBase path no
// route claims is answered by the gate first, so its 404 comes from
// behind the gate, never from the outer mux's catch-all.
func (s *server) registerTerminal(mux *http.ServeMux) {
	// The four patterns are byte-identical to the direct registrations
	// this replaces. termBase ends in "/", so the parent pattern below is
	// the subtree they all lived in.
	termMux := http.NewServeMux()
	termMux.HandleFunc("POST "+termBase+"sessions/{$}", s.handleTerminalCreate)
	termMux.HandleFunc("GET "+termBase+"sessions/{$}", s.handleTerminalList)
	termMux.HandleFunc("DELETE "+termBase+"sessions/{id}/{$}", s.handleTerminalDelete)
	termMux.HandleFunc("GET "+termBase+"sessions/{id}/ws/{$}", s.handleTerminalWS)
	// The outer mux ends with the same catch-all (server.go): an unknown
	// terminal path keeps the {"error":"not_found"} body the UI parses —
	// only now the gate has already spoken by the time it is served.
	termMux.HandleFunc("/", handleNotFound)
	mux.Handle(termBase, s.terminalRoute(termMux))
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

// terminalRoute is the gate every terminal handler sits behind, mounted
// once over the whole terminal sub-mux. It stores the pairing token id
// the gate authenticated into the request context; handlers read it with
// terminalTokenID instead of a positional argument a registration could
// forget to pass.
func (s *server) terminalRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenID, ok := s.terminalGate(w, r)
		if !ok {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), terminalTokenCtx{}, tokenID)))
	})
}

// terminalTokenCtx is the context key carrying the pairing token id a
// terminal request authenticated with. Unexported, and written in exactly
// one place — terminalRoute — so no handler can read a token id the gate
// did not vouch for.
type terminalTokenCtx struct{}

// terminalTokenID returns the pairing token id the gate authenticated
// this request with. Empty means local root (decision 0003: a loopback
// caller is this machine's user and needs no token), and the value is set
// only by the gate — never by a handler or a client.
func terminalTokenID(r *http.Request) string {
	id, _ := r.Context().Value(terminalTokenCtx{}).(string)
	return id
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

// terminalSessionDoc is the create response: identity, geometry, and the
// behavior the pane applies to its renderer (GDK-896 R2) — never output.
// term.Info carries no token id, which is why it can be served. Shell and
// workingDir stay server-only: this body reaches paired remote clients,
// and one that echoed them would publish the machine's shell paths — the
// shape GDK-1069 rejected. The settings dialog is not a second road for
// these values either; the create response is where the pane learns them.
type terminalSessionDoc struct {
	ID          string `json:"id"`
	Cols        uint16 `json:"cols"`
	Rows        uint16 `json:"rows"`
	Scrollback  int    `json:"scrollback"`
	CursorBlink bool   `json:"cursorBlink"`
}

type terminalCreateReq struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (s *server) handleTerminalCreate(w http.ResponseWriter, r *http.Request) {
	tokenID := terminalTokenID(r)
	var req terminalCreateReq
	if body, err := io.ReadAll(io.LimitReader(r.Body, 4<<10)); err == nil && len(body) > 0 {
		// A body is optional: the pane may not know its size yet.
		_ = json.Unmarshal(body, &req)
	}
	// The settings block owns behavior (GDK-896): shell and working dir
	// come from the config, everything else stays as it was.
	tc := s.config().EffectiveTerminal()
	opts := term.Options{
		Cols:    req.Cols,
		Rows:    req.Rows,
		TokenID: tokenID,
		Shell:   tc.Shell,
	}
	// Name this window's workspace to the shell it starts. The session
	// inherits the serve process's whole environment, and workspace mode
	// runs one server per profile in the same process — so an inherited
	// GADAK_WORKSPACE, if any, belongs to some other window. Setting it
	// unconditionally is what keeps the pane's bare `gadak` on the board the
	// pane lives in; root serializes as "default", which canonProfile maps
	// back to root when the child reads it. Options.Env is appended after
	// the inherited environment, so this always wins.
	opts.Env = append(opts.Env, "GADAK_WORKSPACE="+config.NormalizeProfile(s.profile))
	// A configured workingDir that is missing at create time must not make
	// the terminal unopenable, and the fallback must not hide the typo.
	if tc.WorkingDir != "" {
		if info, err := os.Stat(tc.WorkingDir); err != nil || !info.IsDir() {
			log.Printf("server: terminal workingDir %q not found; session starts in the default directory", tc.WorkingDir)
		} else {
			opts.Dir = tc.WorkingDir
		}
	}
	// An unconfigured workingDir starts the session in the user's home, not
	// the serve process's cwd — which is the profile state dir, gadak's own
	// internals and nobody's workplace. A home that cannot be resolved
	// leaves Dir empty, and the manager's fallback applies: the old
	// behavior, not a failed create.
	if opts.Dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			opts.Dir = home
		}
	}
	sess, err := s.terminalManager().Create(opts)
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
	writeJSON(w, http.StatusOK, terminalSessionDoc{
		ID:          info.ID,
		Cols:        info.Cols,
		Rows:        info.Rows,
		Scrollback:  tc.Scrollback,
		CursorBlink: tc.CursorBlink,
	})
}

func (s *server) handleTerminalList(w http.ResponseWriter, r *http.Request) {
	tokenID := terminalTokenID(r)
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

func (s *server) handleTerminalDelete(w http.ResponseWriter, r *http.Request) {
	tokenID := terminalTokenID(r)
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
func (s *server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	tokenID := terminalTokenID(r)
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
	// Parent on jobsCtx, not context.Background() (GDK-915). Shutdown cancels
	// jobsCtx (server.go), so both this socket's blocking calls join the
	// cancel tree that actually stops them: the server→client end-frame write
	// and the client→server Read. Rooted at Background they leaked — a stalled
	// client (attached, not draining) left terminalSendEnd's write and the
	// Read parked with no deadline, so the two goroutines outlived Shutdown
	// even after closeTerminals had reaped the shell. Ordering holds: Shutdown
	// runs closeTerminals (unblocking a wedged sess.Write by closing the PTY)
	// before jobsCancel, so a write blocked in terminalReadLoop is freed too.
	// The one residual jobsCtx cannot reach is a client that vanishes (no
	// Shutdown) while a sess.Write is wedged AND another attachment keeps the
	// session alive; closing that needs per-session write serialization — a
	// shared multi-attach PTY fd cannot be safely deadlined per-client.
	ctx, cancel := context.WithCancel(s.jobsCtx)
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
				// the shell it is attached to — but a lost resize must
				// name itself (GDK-1102: a swallowed error here reads as
				// "the child ignores winsize" one flake later).
				if err := sess.Resize(msg.Cols, msg.Rows); err != nil {
					log.Printf("server: terminal: resize %dx%d not applied: %v", msg.Cols, msg.Rows, err)
				}
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
