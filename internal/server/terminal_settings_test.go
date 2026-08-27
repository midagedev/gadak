package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/midagedev/gadak/internal/config"
)

// The terminal settings block (GDK-896) as the create path consumes it.
// Options carry no Shell/Dir today and term.Info never exposes them, so
// the assertions ride the product path — what the shell reports about
// itself ($0, pwd) — rather than a new seam.

// setTerminalLeaf writes one terminal.* leaf through the catalog — the
// same choke `gadak config set` rides, so the test config carries exactly
// what a user's config.json would.
func setTerminalLeaf(t *testing.T, cfg *config.Config, path, raw string) {
	t.Helper()
	s, ok := config.SettingByPath(path)
	if !ok {
		t.Fatalf("%s not in catalog", path)
	}
	if err := s.Set(cfg, json.RawMessage(raw)); err != nil {
		t.Fatalf("set %s = %s: %v", path, raw, err)
	}
}

// TestTerminalCreateUsesConfiguredShellAndDir: a session created while
// terminal.shell and terminal.workingDir are set runs that shell in that
// directory. $SHELL points at a path that does not exist, so a create
// that ignored terminal.shell could not have started a shell at all.
func TestTerminalCreateUsesConfiguredShellAndDir(t *testing.T) {
	h, cfg := standaloneServer(t)
	t.Setenv("SHELL", "/nonexistent-shell-gdk896")
	setTerminalLeaf(t, cfg, "terminal.shell", `"/bin/sh"`)
	dir := t.TempDir()
	setTerminalLeaf(t, cfg, "terminal.workingDir", `"`+dir+`"`)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	id := createSession(t, srv, "", "")
	c, _, err := dialTerminal(t, srv, id, "", "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("printf '%s\\n' \"$0\"\n")); err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, "/bin/sh")
	// The kernel resolves symlinks on chdir (macOS tempdirs live behind
	// /var → /private/var), so the shell's pwd reports the evaluated path.
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(ctx, websocket.MessageBinary, []byte("pwd\n")); err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, real)
}

// TestTerminalCreateWorkingDirMissingFallsBack: a typo in workingDir must
// not make the terminal unopenable, and the fallback must say the
// configured path out loud. The session then starts where it always did:
// the workspace directory.
func TestTerminalCreateWorkingDirMissingFallsBack(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	h, cfg := standaloneServer(t)
	missing := filepath.Join(t.TempDir(), "gone")
	setTerminalLeaf(t, cfg, "terminal.workingDir", `"`+missing+`"`)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	id := createSession(t, srv, "", "")
	if !strings.Contains(buf.String(), missing) {
		t.Fatalf("fallback log does not name the configured path %q; log so far: %s", missing, buf.String())
	}
	c, _, err := dialTerminal(t, srv, id, "", "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, []byte("pwd\n")); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(cfg.Directory())
	if err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, want)
}
