package pairing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/atomicfile"
)

// remoteRel is the profile-relative file holding this workspace's stored
// pairing credential: the workspace's origin is a remote gadak serve, and
// this is how to reach and authenticate it.
//
// It sits next to config.json (the credential convention — 0600, atomic
// write, profile-scoped) rather than inside it, for the same reason the
// server's token store is a separate file: config.json is what settings
// surfaces read and rewrite; a credential file has one writer, the pairing
// path. The token is plaintext here because the client must present it.
const remoteRel = "remote-origin.json"

// Remote is the stored client side of a pairing: where the home serve is,
// the device token, and the label the home shows in `pairing list`.
type Remote struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	Label    string `json:"label,omitempty"`
	PairedAt string `json:"pairedAt,omitempty"`
}

// RemotePath is the absolute credential path inside a profile directory.
func RemotePath(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, remoteRel)
}

// LoadRemote reads the stored pairing credential. Missing file is
// (nil, nil): most workspaces are not paired, and that is not an error.
func LoadRemote(dir string) (*Remote, error) {
	p := RemotePath(dir)
	if p == "" {
		return nil, nil
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r Remote
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("pairing: %s: %w", remoteRel, err)
	}
	if strings.TrimSpace(r.Endpoint) == "" || r.Token == "" {
		return nil, fmt.Errorf("pairing: %s is incomplete; re-pair with gadak init --pairing-code", remoteRel)
	}
	return &r, nil
}

// SaveRemote writes the pairing credential atomically at 0600, creating
// the profile directory if needed. Called only after a successful
// verify-before-save round trip — a credential that was never proven good
// must not reach disk.
func SaveRemote(dir string, r Remote) error {
	p := RemotePath(dir)
	if p == "" {
		return errors.New("pairing: no profile directory")
	}
	if strings.TrimSpace(r.Endpoint) == "" || r.Token == "" {
		return errors.New("pairing: remote credential needs endpoint and token")
	}
	if r.PairedAt == "" {
		r.PairedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("pairing: profile dir: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	// Unique staging via atomicfile: a fixed .tmp let two savers truncate
	// each other's staging copy of this credential (GDK-1244, the GDK-1233
	// class outside config).
	if err := atomicfile.WriteFile(p, "remote-origin-*.json", data); err != nil {
		return err
	}
	return nil
}
