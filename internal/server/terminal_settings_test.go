package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/midagedev/gadak/internal/config"
)

// The terminal settings block (GDK-896) as the create path consumes it.
// Shell and workingDir carry no term.Info today, so those assertions ride
// the product path — what the shell reports about itself ($0, pwd) —
// rather than a new seam. Scrollback and cursorBlink (R2) have a seam of
// their own: the create response.

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
	h, cfg := localOriginServer(t)
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
// configured path out loud. The session then starts in the unconfigured
// default — the user's home (GDK-995; it was the workspace directory, the
// serve process's cwd).
func TestTerminalCreateWorkingDirMissingFallsBack(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	h, cfg := localOriginServer(t)
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
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("os.UserHomeDir: %v — fallback destination unpinned here", err)
	}
	// The kernel resolves symlinks on chdir, so the shell's pwd reports the
	// evaluated path (same note as TestTerminalCreateUsesConfiguredShellAndDir).
	want, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	readUntilMarker(t, c, want)
}

// TestTerminalCreateResponseCarriesBehavior (GDK-896 R2): the create
// response is the one surface terminal behavior reaches the pane on — the
// settings dialog does not carry terminal keys (GDK-1069), so there is no
// second road. The same body must never echo shell or workingDir: both are
// server-only, and a response carrying them would hand a paired remote
// client the machine's shell paths — the exact shape GDK-1069 rejected.
func TestTerminalCreateResponseCarriesBehavior(t *testing.T) {
	h, cfg := localOriginServer(t)
	setTerminalLeaf(t, cfg, "terminal.scrollback", `9000`)
	setTerminalLeaf(t, cfg, "terminal.cursorBlink", `true`)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	code, body := termRequest(t, srv, http.MethodPost, termBase+"sessions/", `{"cols":90,"rows":30}`, "", "")
	if code != http.StatusOK {
		t.Fatalf("create session: %d %s", code, body)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("create body %s: %v", body, err)
	}
	if got := doc["scrollback"]; got != float64(9000) {
		t.Errorf("scrollback = %v (%T), want 9000 — body %s", got, got, body)
	}
	if got := doc["cursorBlink"]; got != true {
		t.Errorf("cursorBlink = %v (%T), want true — body %s", got, got, body)
	}
	for _, key := range []string{"shell", "workingDir", "renderer"} {
		if _, ok := doc[key]; ok {
			t.Errorf("create response carries %q — server-only and removed keys must not reach a client (GDK-1069/1078): %s", key, body)
		}
	}
}

// TestTerminalCreateResponseBehaviorDefaults: an untouched config answers
// the pane with the effective defaults — the same 5000/false the pane
// hardcoded before the block existed, now named by the server instead of
// reinvented on the client.
func TestTerminalCreateResponseBehaviorDefaults(t *testing.T) {
	h, _ := localOriginServer(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	code, body := termRequest(t, srv, http.MethodPost, termBase+"sessions/", `{"cols":90,"rows":30}`, "", "")
	if code != http.StatusOK {
		t.Fatalf("create session: %d %s", code, body)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("create body %s: %v", body, err)
	}
	if got := doc["scrollback"]; got != float64(config.DefaultTerminalScrollback) {
		t.Errorf("scrollback = %v, want %d — body %s", got, config.DefaultTerminalScrollback, body)
	}
	if got := doc["cursorBlink"]; got != false {
		t.Errorf("cursorBlink = %v, want false — body %s", got, body)
	}
}
