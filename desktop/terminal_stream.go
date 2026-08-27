package main

// The terminal's transport inside Gadak.app (GDK-892).
//
// `gadak serve` carries PTY bytes over a WebSocket (internal/server/
// terminal.go). This app cannot: it mounts the same http.Handler behind the
// wails asset server — a WKURLSchemeHandler on macOS — and there is no TCP
// listener, by design (see the package comment in main.go). A browser's
// network stack owns ws:// and never consults a custom-scheme handler, so the
// pane's socket could not open here and the pane said "unavailable".
//
// wails v3 ships a transport for exactly this position: GoStream, a held poll
// plus a POST, dispatched by wails before any user handler
// (pkg/application/application.go:134), so gadak's own asset handler never
// sees those requests. `Stream(name)` on the frontend hands back a WailsSocket
// with the WebSocket shape. This file is the Go end of that socket.
//
// Framing, symmetrical in both directions, because a WailsSocket delivers
// every message as an ArrayBuffer and has no text-vs-binary distinction to
// carry the control channel on:
//
//	byte 0 = 0x00 → the rest of the frame is raw PTY bytes
//	byte 0 = 0x01 → the rest of the frame is UTF-8 JSON control
//
// The control vocabulary reuses the strings the WebSocket already sends
// (term.Reason*, {"t":"exit"}, {"t":"dropped"}), so the pane's status
// rendering needs no new case for the desktop.

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/term"
)

// terminalStreamName is the stream the pane connects to. One name, one
// meaning: a connection is one attachment to one session.
const terminalStreamName = "terminal"

// Frame tags. Defined once here and mirrored by web/src/lib/terminal/
// wails-stream.ts, which is the only other speaker of this protocol.
const (
	termFrameData byte = 0x00
	termFrameCtrl byte = 0x01
)

// Control message types, client → Go.
const (
	termMsgAttach = "attach"
	termMsgResize = "resize"
)

// Error codes, Go → client. Deliberately the vocabulary the REST surface
// already uses for the same conditions.
const (
	// termErrProtocol: the first frame was not a valid attach.
	termErrProtocol = "protocol"
	// termErrNotFound: no such session, or its shell is already gone. Same
	// answer handleTerminalWS gives (a 404 on the upgrade).
	termErrNotFound = "not_found"
	// termErrUnsupported: this platform has no PTY. Unreachable through
	// Get/Attach today — term.ErrUnsupportedPlatform is a Create answer, and
	// the pane creates over REST — but the mapping belongs with the others
	// rather than in whatever code first needs it.
	termErrUnsupported = "unsupported"
)

// termClientMsg is every control frame the client may send. One struct
// because the wire has one shape and `t` selects the meaning.
type termClientMsg struct {
	T    string `json:"t"`
	ID   string `json:"id"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type termAttachedMsg struct {
	T string `json:"t"`
}

type termExitMsg struct {
	T    string `json:"t"`
	Code int    `json:"code"`
}

type termDroppedMsg struct {
	T      string `json:"t"`
	Reason string `json:"reason"`
}

type termErrorMsg struct {
	T    string `json:"t"`
	Code string `json:"code"`
}

// termStreamConn is the part of *application.StreamConn this protocol uses.
// Narrow on purpose: the loops below are the thing worth testing, and a live
// StreamConn needs a webview and a held poll to exist.
type termStreamConn interface {
	Send([]byte) error
	Receive() ([]byte, error)
	Context() context.Context
}

var _ termStreamConn = (*application.StreamConn)(nil)

// registerTerminalStream wires the pane's transport to this process's session
// core. The manager is the same one the REST handlers build lazily, so a
// session created over REST is the session this stream attaches to.
func registerTerminalStream(app *application.App, api *server.Handler) {
	if app == nil || api == nil {
		return
	}
	app.HandleStream(terminalStreamName, func(c *application.StreamConn) {
		// The connection is live exactly as long as this handler runs
		// (pkg/application/stream.go:162), so returning is how it closes.
		defer func() { _ = c.Close() }()
		serveTerminalStream(c, api.Terminals())
	})
}

// serveTerminalStream is one connection: one attachment to one session,
// mirroring handleTerminalWS.
//
// Backpressure has one owner and it is internal/term, which drops a slow
// attachment rather than stalling the PTY. Send blocks like a socket write,
// so a frontend that stops collecting parks this loop, grows the
// attachment's pending backlog past AttachBytes, and term drops it with
// slow_client — the cascade the WebSocket path already gets. Nothing here
// adds a second policy.
func serveTerminalStream(c termStreamConn, mgr *term.Manager) {
	first, err := c.Receive()
	if err != nil {
		return
	}
	msg, ok := decodeTermControl(first)
	if !ok || msg.T != termMsgAttach || msg.ID == "" {
		sendTermJSON(c, termErrorMsg{T: "error", Code: termErrProtocol})
		return
	}
	sess, err := mgr.Get(msg.ID)
	if err != nil {
		sendTermJSON(c, termErrorMsg{T: "error", Code: termErrCode(err)})
		return
	}
	att, err := sess.Attach()
	if err != nil {
		sendTermJSON(c, termErrorMsg{T: "error", Code: termErrCode(err)})
		return
	}
	// Detach, never sess.Close(): closing the pane must leave the shell
	// running for the reconnect grace so a reopen replays the ring, exactly
	// as in the browser. Detach is safe on every path, including after the
	// attachment already ended (term.Session.detach ignores a stale one).
	defer att.Detach()

	// Written here and by the reader goroutine, and read by the deferred log
	// below while that reader may still be parked in Receive — three accesses
	// across two goroutines, so it is atomic rather than a plain string. A
	// plain one is a data race that `go test -race ./desktop/...` reports.
	var reason atomic.Value
	reason.Store("context done")
	log.Printf("desktop: terminal stream: attached session %s", msg.ID)
	defer func() {
		log.Printf("desktop: terminal stream: session %s ended: %s", msg.ID, reason.Load())
	}()

	if err := sendTermJSON(c, termAttachedMsg{T: "attached"}); err != nil {
		reason.Store("receive error")
		return
	}

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	go terminalStreamRead(ctx, cancel, c, sess, &reason)

	for {
		select {
		case <-att.Wake():
			if chunk := att.Take(); len(chunk) > 0 {
				if err := c.Send(termDataFrame(chunk)); err != nil {
					reason.Store("receive error")
					return
				}
			}
		case <-att.Done():
			terminalStreamFlush(c, att)
			sendTermEnd(c, att.End())
			return
		case <-ctx.Done():
			// The frontend went away: a page reload, a closed pane, or the
			// app shutting down.
			return
		}
	}
}

// terminalStreamRead is client → Go: keystrokes and resizes. It owns cancel,
// so a dead connection ends the writer too.
func terminalStreamRead(ctx context.Context, cancel context.CancelFunc, c termStreamConn, sess *term.Session, reason *atomic.Value) {
	defer cancel()
	for {
		frame, err := c.Receive()
		if err != nil {
			if reason != nil {
				reason.Store("receive error")
			}
			return
		}
		if len(frame) == 0 {
			continue
		}
		switch frame[0] {
		case termFrameData:
			if _, err := sess.Write(frame[1:]); err != nil {
				return
			}
		case termFrameCtrl:
			msg, ok := decodeTermControl(frame)
			if !ok {
				continue
			}
			if msg.T == termMsgResize {
				// A bad size is the client's bug, not a reason to drop the
				// shell it is attached to.
				_ = sess.Resize(msg.Cols, msg.Rows)
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// terminalStreamFlush writes whatever the attachment still had pending
// when it ended, so a shell's last line arrives before the exit frame.
// Take() returns the whole backlog in one slice — a backlog pending at the
// end stays readable (internal/term), which is what makes this drain safe.
func terminalStreamFlush(c termStreamConn, att *term.Attachment) {
	if chunk := att.Take(); len(chunk) > 0 {
		_ = c.Send(termDataFrame(chunk))
	}
}

// sendTermEnd is the last control frame of an attachment: why it stopped.
// EndDetached says nothing to the client — it is this end letting go.
func sendTermEnd(c termStreamConn, end term.End) {
	switch end.Kind {
	case term.EndExited:
		sendTermJSON(c, termExitMsg{T: "exit", Code: end.Code})
	case term.EndDropped, term.EndClosed:
		reason := end.Reason
		if reason == "" {
			reason = term.ReasonClosed
		}
		sendTermJSON(c, termDroppedMsg{T: "dropped", Reason: reason})
	}
}

// termDataFrame tags PTY bytes. A fresh slice every call: Send queues the
// frame rather than copying it (pkg/application/stream.go:225-232), so the
// caller may not hand it a buffer it still owns. Take() does hand its
// backlog over outright, but the frame needs a type byte in front of it,
// so a fresh slice is what there is to send either way.
func termDataFrame(chunk []byte) []byte {
	frame := make([]byte, 0, len(chunk)+1)
	frame = append(frame, termFrameData)
	return append(frame, chunk...)
}

// decodeTermControl reads a control frame. It reports false for anything that
// is not one, so a caller can tell "not control" from "control I don't know".
func decodeTermControl(frame []byte) (termClientMsg, bool) {
	if len(frame) < 1 || frame[0] != termFrameCtrl {
		return termClientMsg{}, false
	}
	var msg termClientMsg
	if err := json.Unmarshal(frame[1:], &msg); err != nil {
		return termClientMsg{}, false
	}
	return msg, true
}

func sendTermJSON(c termStreamConn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		// Only reachable if one of the structs above grows an unmarshalable
		// field; worth a line rather than a silent drop.
		log.Printf("desktop: terminal stream: marshal %T: %v", v, err)
		return err
	}
	frame := make([]byte, 0, len(data)+1)
	frame = append(frame, termFrameCtrl)
	return c.Send(append(frame, data...))
}

// termErrCode maps a manager/session error onto the client's vocabulary. A
// session that exists but whose shell is gone is not distinguished from one
// that never existed — the pane's answer to both is to make a new one.
func termErrCode(err error) string {
	if errors.Is(err, term.ErrUnsupportedPlatform) {
		return termErrUnsupported
	}
	return termErrNotFound
}
