package store

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// MirrorVersion is the disk identity of a mirror: "did this move?" answered
// for a caller that has to ask twice a second (GDK-1170, the ui-focus poll).
// Only equality is defined — the shape is not a contract, and a client that
// stores one and compares the next is using it correctly.
//
// The technique is config.ConfigVersionOfDir's: mtime+size, no open, no query,
// no read of the bytes. Two os.Stat calls, so a 500ms poll costs nothing.
//
// It keys on the -wal sidecar and not on the database file alone, and that is
// the whole point. store.Open sets journal_mode=WAL, so a write from another
// process (a `gadak claim` in a terminal) lands in gadak.db-wal and leaves
// gadak.db untouched until a checkpoint — and while `gadak serve` holds the
// mirror open there is no checkpoint-on-close to move it. Measured on
// macOS/APFS with serve running: over `gadak claim` and `gadak edit`,
// gadak.db's mtime and size never moved while gadak.db-wal moved on both
// (mtime and size, every write). Keyed on gadak.db alone this would have been
// a version that never changes for exactly the writes it exists to notice.
//
// gadak.db-shm is excluded on purpose: it is the reader's shared index, so
// folding it in would report movement for reads. Measured in the same run —
// a `gadak issue` read moved none of the three.
//
// A missing file is "0", not an error: a mirror that does not exist yet is a
// legitimate state, and it has to compare equal to itself on the next poll.
func MirrorVersion(path string) string {
	v, _ := mirrorState(path)
	return v
}

// MirrorChangedAt is when the mirror bytes last moved — the newest mtime among
// the files MirrorVersion keys on. Zero when path is empty or nothing exists.
// This is the diagnostic half: `gadak doctor` reports it so "why is the board
// not updating?" is answered by looking at one timestamp instead of guessing.
func MirrorChangedAt(path string) time.Time {
	_, at := mirrorState(path)
	return at
}

func mirrorState(path string) (string, time.Time) {
	if path == "" {
		return "", time.Time{}
	}
	var b strings.Builder
	var newest time.Time
	for i, p := range [...]string{path, path + "-wal"} {
		if i > 0 {
			b.WriteByte('/')
		}
		fi, err := os.Stat(p)
		if err != nil {
			b.WriteString("0")
			continue
		}
		fmt.Fprintf(&b, "%d.%d", fi.ModTime().UnixNano(), fi.Size())
		if mt := fi.ModTime(); mt.After(newest) {
			newest = mt
		}
	}
	return b.String(), newest
}

// MirrorVersion is this handle's own file's version. The mirror file is the
// single owner of "the mirror moved": every writer — the web UI's
// write-through, a CLI verb's RefreshIssue, the watch loop — reaches it by
// writing those bytes, so no writer has to remember to announce anything.
func (db *DB) MirrorVersion() string { return MirrorVersion(db.path) }
