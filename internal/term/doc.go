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
//     (Setsid + Setctty) so the child is a session leader and this PTY is
//     its controlling terminal. A job-control shell puts background jobs
//     in a new process group, so a signal to -pgid does not reach them.
//     cwd is the workspace directory unless Options.Dir names another.
//     Env is the parent's plus TERM=xterm-256color and GADAK_TERMINAL=1.
//
//   - Close. SIGHUP every process on the shell's controlling terminal,
//     then wait; a SIGKILL to whoever is still on that terminal after
//     CloseGrace (2s). No zombies and no orphaned grandchildren —
//     TestCloseKillsHUPImmuneGrandchild pins a grandchild that ignores
//     SIGHUP, which bash's own HUP-to-jobs cannot hide.
//
//   - Ring. Each session keeps the last DefaultRingBytes (256 KiB) of
//     output. Attach replays that buffer as the first chunk a reader sees,
//     then live bytes: a client that reconnects inside the grace picks up
//     its scrollback instead of a blank screen.
//
//   - Backpressure. Every attachment carries its own backlog, bounded in
//     bytes (DefaultAttachBytes, 4 MiB), not in chunks: pending chunks
//     are coalesced into it, because a terminal stream is a byte stream
//     and N pending chunks are exactly their concatenation (GDK-1042 —
//     the chunk-count bound this replaces dropped a client that was 256
//     tiny reads behind, all of 59 KB). When the backlog would exceed
//     the bound the attachment is dropped — closed with a reason — and
//     the PTY read loop keeps going. A slow client never stalls the
//     shell and never delays another client. The one thing that is never
//     dropped is the PTY.
//
//   - Reconnect. A session survives its last attachment leaving for
//     DefaultGrace (60s). Reattaching by session id inside the grace
//     cancels the reap and replays the ring. A session that still has an
//     attachment never reaps, and an attached idle session is not timed
//     out.
//
//   - Idle, and what it is not. When the grace elapses, Session
//     .idleForReap decides — one owner, three signals: an attachment,
//     output since the grace was armed, or a process besides the shell on
//     this controlling terminal. Any of them and the grace re-arms
//     instead of reaping, because a shell with an agent or a build under
//     it is not idle just because the tab closed. Attachment count alone
//     was the whole answer until GDK-994, and it killed running work
//     60 seconds after a phone went to the background. The ceiling is
//     DefaultMaxDetachedLife (24h) measured from the last detach: past it
//     an unattended session is closed however busy it looks. Info carries
//     DetachedAt and GraceExtensions so the decision is visible from
//     `gadak terminal list` rather than inferred.
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
// Snapshot() is the debug surface: per-session id, pid, pids (every
// process on the session's controlling terminal), size, attachment
// count, createdAt, lastOutputAt, bytesOut, droppedAttachments, and —
// since GDK-1042 — backlogMaxBytes and coalescedChunks, which say how
// close a session came to the drop bound and how often coalescing did
// work. It carries no output bytes and no token id, so it is safe to
// serve.
package term
