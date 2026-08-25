// Package term owns the PTY sessions `gadak serve` runs next to the mirror
// — one session core for all three surfaces (the web pane, the desktop
// app, and a phone over the paired network). Only renderers differ; the
// shell, its lifetime, and its byte pump live here (GDK-862). Nothing in
// this package speaks HTTP: the WebSocket that carries these bytes is
// internal/server's, and the VT/render half is web-side.
//
// The contracts, in one place, because every one of them is pinned by a
// test in this package:
//
//   - Shell. $SHELL, else /bin/sh, started under a PTY in its own session
//     (Setsid + Setctty) so the child is a process-group leader and a
//     signal to -pgid reaches everything it spawned. cwd is the workspace
//     directory unless Options.Dir names another. Env is the parent's plus
//     TERM=xterm-256color and GADAK_TERMINAL=1.
//
//   - Close. SIGHUP to the process group, then wait; a SIGKILL to the same
//     group after CloseGrace (2s) if the pump has not finished. No zombies
//     and no orphaned grandchildren — TestCloseKillsProcessGroup pins the
//     grandchild.
//
//   - Ring. Each session keeps the last DefaultRingBytes (256 KiB) of
//     output. Attach replays that buffer as the first chunk a reader sees,
//     then live bytes: a client that reconnects inside the grace picks up
//     its scrollback instead of a blank screen.
//
//   - Backpressure. Every attachment has its own bounded channel
//     (DefaultAttachBuffer chunks). When it is full the attachment is
//     dropped — closed with a reason — and the PTY read loop keeps going.
//     A slow client never stalls the shell and never delays another
//     client. The one thing that is never dropped is the PTY.
//
//   - Reconnect. A session survives its last attachment leaving for
//     DefaultGrace (60s), then is reaped (Close). Reattaching by session
//     id inside the grace cancels the reap and replays the ring. A session
//     that still has an attachment never reaps, and an attached idle
//     session is not timed out in v0.18.
//
//   - Ids are 128 bits of crypto/rand, hex. Never sequential: a session id
//     is the only thing a socket URL carries.
//
//   - Revocation. Sessions record the pairing token id they were opened
//     with (empty for a loopback client, which needs no token). Manager
//     .CloseByToken is how `gadak pairing revoke` reaches a live shell —
//     see internal/server's watchdog for who calls it.
//
//   - Windows returns ErrUnsupportedPlatform from Create, naming GDK-861
//     (the ConPTY shape). An honest stub beats a silent one.
//
// Snapshot() is the debug surface: per-session id, pid, size, attachment
// count, createdAt, lastOutputAt, bytesOut, droppedAttachments. It carries
// no output bytes and no token id, so it is safe to serve.
package term
