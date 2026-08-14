// Package uifocus is the one-shot handoff from the CLI to a running UI.
// The CLI writes a hash; the desktop app or a serve tab reads and deletes it.
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

// MaxAge is how long a focus request stays valid. Older files are ignored
// so a leftover from yesterday cannot yank the list when the app next opens.
const MaxAge = 2 * time.Minute

type request struct {
	Hash string `json:"hash"`
	At   string `json:"at"`
}

// Path is the focus file for the active profile.
func Path() (string, error) {
	return PathFor(config.Profile())
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
	body, err := json.Marshal(request{Hash: hash, At: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	return os.WriteFile(p, body, 0o600)
}

// Take returns a still-fresh hash and deletes the file. ok is false when
// there is nothing to apply (missing, empty, or stale).
func Take() (hash string, ok bool, err error) {
	return TakeFor(config.Profile())
}

// TakeFor is Take for a named profile. Workspace mounts pass the /w/<name>
// segment so they do not consume the process-primary file.
func TakeFor(profile string) (hash string, ok bool, err error) {
	p, err := PathFor(profile)
	if err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	_ = os.Remove(p)
	var req request
	if err := json.Unmarshal(raw, &req); err != nil || req.Hash == "" {
		return "", false, nil
	}
	at, err := time.Parse(time.RFC3339, req.At)
	if err != nil || time.Since(at) > MaxAge {
		return "", false, nil
	}
	return req.Hash, true, nil
}
