// Package serveaddr owns the home-root run directory that a live `gadak serve`
// writes so other processes can find it without guessing ports.
//
// This is not origin.AdvertiseRel (serve-origin.json). That file means "who
// owns the standalone persist"; a connected workspace's origin is Jira, so
// overloading it would give the file a second meaning. These files answer a
// different question: which loopback ports currently serve the UI.
package serveaddr

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fsperm"
)

// Rel is the directory name under the gadak home root.
const Rel = "run"

// Dir is <gadak-home>/run, shared across workspaces. findServeTarget prefers
// a same-profile hit and falls back to any other live serve, so discovery
// must see every profile's serve — not only the caller's profile directory
// (config.Dir()). config.DirFor("") is the home root.
func Dir() (string, error) {
	root, err := config.DirFor("")
	if err != nil {
		return "", err
	}
	return filepath.Join(root, Rel), nil
}

// Record is one live-serve file. Addr is the bound listen address (after
// port fallback), not the preferred --addr. Port is the file key (from the
// filename) and is not stored in the JSON.
type Record struct {
	Addr      string `json:"addr"`
	Profile   string `json:"profile"`
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	Port      string `json:"-"`
}

// Path is the runtime file for one port inside dir.
func Path(dir, port string) string {
	if dir == "" || !validPort(port) {
		return ""
	}
	return filepath.Join(dir, port+".json")
}

// Write publishes the bound listen address. Atomic write (temp + rename) at
// 0600, same as origin.WriteAdvertise / config.Save. The filename is the
// port so two serves cannot collide.
func Write(dir, addr, profile string) error {
	if dir == "" || addr == "" {
		return fmt.Errorf("serveaddr: dir and addr are required")
	}
	port, err := portOf(addr)
	if err != nil {
		return err
	}
	if err := fsperm.EnsurePrivateDir(dir); err != nil {
		if errors.Is(err, fsperm.ErrChmod) {
			log.Printf("serveaddr: %v", err)
		} else {
			return fmt.Errorf("serveaddr: run dir: %w", err)
		}
	}
	doc := Record{
		Addr:      addr,
		Profile:   profile,
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	p := Path(dir, port)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Remove deletes the runtime file for port. Missing is not an error — a
// crash leftover is the other path, and the next probe treats a dead file
// as "no live serve".
func Remove(dir, port string) error {
	p := Path(dir, port)
	if p == "" {
		return nil
	}
	err := os.Remove(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// List reads every port-named file in dir. Missing or unreadable dir is
// empty, not an error: discovery then falls back to the port sweep.
func List(dir string) []Record {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		port, ok := filePort(e.Name())
		if !ok {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var rec Record
		if err := json.Unmarshal(raw, &rec); err != nil || rec.Addr == "" {
			continue
		}
		rec.Port = port
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(out[i].Port)
		b, _ := strconv.Atoi(out[j].Port)
		return a < b
	})
	return out
}

// Publish writes the bound address into Dir and returns a cleanup that
// removes that file. Callers that must not take the server down on a write
// failure log err and continue.
func Publish(addr, profile string) (func(), error) {
	nop := func() {}
	dir, err := Dir()
	if err != nil {
		return nop, err
	}
	if err := Write(dir, addr, profile); err != nil {
		return nop, err
	}
	port, err := portOf(addr)
	if err != nil {
		return nop, err
	}
	return func() { _ = Remove(dir, port) }, nil
}

func portOf(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("serveaddr: addr %q: %w", addr, err)
	}
	if !validPort(port) {
		return "", fmt.Errorf("serveaddr: invalid port in %q", addr)
	}
	return port, nil
}

func filePort(name string) (string, bool) {
	port, ok := strings.CutSuffix(name, ".json")
	if !ok {
		return "", false
	}
	return port, validPort(port)
}

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}
