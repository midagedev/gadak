package origin

// The open marker answers one question and only one: is this standalone
// workspace open in some other process right now?
//
// That question used to be answered by a side effect. Before GDK-936 a
// flock sidecar arbitrated *writes*, and callers who wanted to know whether
// anyone had the workspace open asked the lock, because the lock happened to
// know. GDK-936 proved the write arbitration unnecessary — issuetap's WAL
// already lets two sessions share one persist, measured — and deleted the
// sidecar. The write question was answered correctly; the open question lost
// its answer with it (GDK-971: `gadak init --replace-standalone` stopped
// refusing while Gadak.app held the workspace, because the app listens on no
// port and so is invisible to serveaddr discovery).
//
// So this file restores the second question alone, and is deliberately not a
// lock:
//
//	Nothing waits on it and nothing is refused a write because of it. The
//	only caller is the standalone→connected conversion, which is a
//	destructive migration rather than concurrent access — a workspace is
//	bound to one origin (see CLAUDE.md), so changing it under a live holder
//	is the "quietly points at a different tracker" defect.
//
//	It never fails an open. A read-only home is a supported state; a marker
//	that cannot be written costs a refusal we would not have made anyway.
//
//	A stale marker never blocks. The kernel used to release the flock when
//	the holder died; here nothing does, so a marker whose PID is gone is
//	ignored and removed on sight.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// openMarkSuffix sits beside the persist it describes. SQLite ignores
// siblings it did not create, and keying by persist path matches `live`.
const openMarkSuffix = ".open"

type openMark struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
}

func openMarkPath(persist string) string {
	if persist == "" {
		return ""
	}
	return persist + openMarkSuffix
}

// markOpen records this process as a holder of persist. Best effort: every
// failure path leaves no marker, which reads as "not open" — the safe
// direction, since the alternative is refusing a conversion nobody is
// holding.
func markOpen(persist string) {
	p := openMarkPath(persist)
	if p == "" {
		return
	}
	body, err := json.Marshal(openMark{
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return
	}
	// Temp + rename so a concurrent reader never sees a half-written file.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".open-*")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return
	}
	if err := os.Chmod(name, 0o600); err != nil {
		_ = os.Remove(name)
		return
	}
	if err := os.Rename(name, p); err != nil {
		_ = os.Remove(name)
	}
}

// clearOpen drops this process's marker. Removing another process's marker
// would be wrong, so the PID is checked first — two holders of one persist
// is a supported state under WAL, and the last one out should not have been
// erased by the first.
func clearOpen(persist string) {
	p := openMarkPath(persist)
	if p == "" {
		return
	}
	m, ok := readOpenMark(p)
	if ok && m.PID != os.Getpid() {
		return
	}
	_ = os.Remove(p)
}

func readOpenMark(p string) (openMark, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return openMark{}, false
	}
	var m openMark
	if json.Unmarshal(b, &m) != nil || m.PID <= 0 {
		return openMark{}, false
	}
	return m, true
}

// OpenHolder reports the PID of another live process holding this persist,
// or 0. This process's own mark is not a holder — the caller is asking
// whether it must stand aside for someone else.
//
// A marker naming a dead PID is removed here rather than reported: the
// alternative is a workspace that refuses conversion forever after one
// crash, with nothing on screen explaining why.
func OpenHolder(persist string) int {
	p := openMarkPath(persist)
	if p == "" {
		return 0
	}
	m, ok := readOpenMark(p)
	if !ok {
		return 0
	}
	if m.PID == os.Getpid() {
		return 0
	}
	if !processAlive(m.PID) {
		_ = os.Remove(p)
		return 0
	}
	return m.PID
}
