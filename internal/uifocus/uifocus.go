// Package uifocus is the handoff from the CLI to a running UI.
// The CLI writes a hash; every polling UI on that profile peeks the same
// payload while it is fresh (MaxAge). The file is not deleted on read —
// one-shot apply belongs to each client, keyed on the write timestamp.
// The file lives next to the profile's config — not in SQLite — so a sync
// cannot wipe a pending focus and a snapshot cannot carry one.
package uifocus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

const fileName = "ui-focus.json"

// stampLayout is RFC 3339 with a fixed-width nanosecond field.
//
// The client dedupes a focus payload on this stamp, so two writes inside one
// second must not carry the same one: at second resolution `gadak views open
// A && gadak views open B` stamped both alike, and every tab that had applied
// A dropped B in silence (GDK-981). time.RFC3339Nano is not enough here — it
// trims trailing zeros, so a whole-second instant formats without any
// fractional part at all. Readers are unaffected either way: fractional
// seconds are RFC 3339, and time.Parse(time.RFC3339, …) accepts them.
const stampLayout = "2006-01-02T15:04:05.000000000Z07:00"

// MaxAge is how long a focus request stays valid. Older files are ignored
// so a leftover from yesterday cannot yank the list when the app next opens.
const MaxAge = 2 * time.Minute

type request struct {
	Hash string `json:"hash"`
	At   string `json:"at"`
}

// PathFor is the focus file for a named profile ("" / "default" = root).
func PathFor(profile string) (string, error) {
	d, err := config.DirFor(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, fileName), nil
}

// Write records a view hash (query string, no #/?) for the running UI to apply.
func Write(hash string) error {
	return WriteFor(config.Profile(), hash)
}

// WriteFor records a view hash for the named profile's UI.
func WriteFor(profile, hash string) error {
	p, err := PathFor(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(request{Hash: hash, At: time.Now().UTC().Format(stampLayout)})
	if err != nil {
		return err
	}
	return os.WriteFile(p, body, 0o600)
}

// PeekFor returns a still-fresh hash and its write timestamp without deleting
// the file. ok is false when there is nothing to apply (missing, empty, or
// stale). Workspace mounts pass the /w/<name> segment so they read their own
// file, not the process-primary one.
func PeekFor(profile string) (hash, at string, ok bool, err error) {
	p, err := PathFor(profile)
	if err != nil {
		return "", "", false, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	var req request
	if err := json.Unmarshal(raw, &req); err != nil || req.Hash == "" {
		return "", "", false, nil
	}
	parsed, err := time.Parse(time.RFC3339, req.At)
	if err != nil || time.Since(parsed) > MaxAge {
		return "", "", false, nil
	}
	return req.Hash, req.At, true, nil
}
