package pairing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// ServerVersion is the home serve's gadak version as its own response
	// header stated it during the verify round trip (GDK-1273) — the
	// pair-time record `gadak status` and `doctor` read for the skew line.
	// Empty on credentials written before the header existed or served by
	// a gadak that predates it.
	ServerVersion string `json:"serverVersion,omitempty"`
}

// VersionSkew classifies the home serve's recorded version against this
// client's (GDK-1273): "same", "home-older", "home-newer", or "unknown".
// Comparison is component-wise over the leading dotted numbers, with a
// `-suffix` ignored (a dev build rides its release line); a version with no
// parseable number prefix can only be compared for equality, and either
// side empty is "unknown" — an absent record must not read as "same".
func VersionSkew(home, client string) string {
	if home == "" || client == "" {
		return "unknown"
	}
	if home == client {
		return "same"
	}
	h, ok1 := parseVersionNumbers(home)
	c, ok2 := parseVersionNumbers(client)
	if !ok1 || !ok2 {
		return "unknown"
	}
	n := len(h)
	if len(c) > n {
		n = len(c)
	}
	for i := 0; i < n; i++ {
		var hv, cv int
		if i < len(h) {
			hv = h[i]
		}
		if i < len(c) {
			cv = c[i]
		}
		if hv != cv {
			if hv < cv {
				return "home-older"
			}
			return "home-newer"
		}
	}
	return "same"
}

// parseVersionNumbers reads the leading dot-separated numeric components,
// stopping at the first non-numeric one ("0.22.0-dev" → [0 22 0]). ok is
// false when not even one number is there.
func parseVersionNumbers(v string) ([]int, bool) {
	var nums []int
	for _, part := range strings.Split(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			// A non-numeric component ends the numeric prefix.
			break
		}
		nums = append(nums, n)
	}
	return nums, len(nums) > 0
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
