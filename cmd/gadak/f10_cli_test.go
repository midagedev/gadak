package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
	"github.com/midagedev/gadak/internal/views"
)

func TestUnknownCommandError(t *testing.T) {
	err := unknownCommandError("foobar")
	if err == nil || err.Error() != `unknown command "foobar" — see gadak --help` {
		t.Fatalf("err = %v", err)
	}
	if got := exitStatus(err); got != 64 {
		t.Fatalf("exitStatus=%d want 64", got)
	}
}

func TestViewsJSONEmptyIsArray(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error { return cmdViews([]string{"--json"}) })
	if err != nil {
		t.Fatalf("views --json: %v\n%s", err, out)
	}
	if strings.Contains(out, `"views":null`) {
		t.Fatalf("empty views encoded as null: %s", out)
	}
	var doc struct {
		Views []views.ListedView `json:"views"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if doc.Views == nil {
		t.Fatal("views must be [] not null")
	}
	if len(doc.Views) != 0 {
		t.Fatalf("views = %+v", doc.Views)
	}
}

func TestSearchZeroMatchesTTYWritesStderr(t *testing.T) {
	mirror(t, "https://unused.example.com")
	saved := searchIsTTY
	searchIsTTY = func() bool { return true }
	t.Cleanup(func() { searchIsTTY = saved })

	stdout, stderr, err := captureErr(t, func() error { return cmdSearch([]string{"zzznotatoken"}) })
	if err != nil {
		t.Fatalf("search: %v\nstdout=%q stderr=%q", err, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("pipe/TTY stdout must stay empty on 0 hits, got %q", stdout)
	}
	if !strings.Contains(stderr, "0 matches") {
		t.Fatalf("TTY stderr missing 0 matches: %q", stderr)
	}
}

func TestSearchZeroMatchesPipeStaysSilent(t *testing.T) {
	mirror(t, "https://unused.example.com")
	saved := searchIsTTY
	searchIsTTY = func() bool { return false }
	t.Cleanup(func() { searchIsTTY = saved })

	stdout, stderr, err := captureErr(t, func() error { return cmdSearch([]string{"zzznotatoken"}) })
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if stdout != "" {
		t.Fatalf("pipe stdout must stay empty, got %q", stdout)
	}
	if strings.Contains(stderr, "0 matches") {
		t.Fatalf("pipe must not print 0 matches, stderr=%q", stderr)
	}
}

func TestSearchTTYPrintsShortHeader(t *testing.T) {
	mirror(t, "https://unused.example.com")
	saved := searchIsTTY
	searchIsTTY = func() bool { return true }
	t.Cleanup(func() { searchIsTTY = saved })

	out, err := capture(t, func() error { return cmdSearch([]string{"idempotency"}) })
	if err != nil {
		t.Fatalf("search: %v\n%s", err, out)
	}
	first := strings.Split(out, "\n")[0]
	if first != searchTSVHeader {
		t.Fatalf("TTY header = %q, want %q", first, searchTSVHeader)
	}
}

func TestInitJSONDefaultProfileAndEmptyProjects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	out, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) })
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if doc["profile"] != "default" {
		t.Fatalf("profile = %v, want default", doc["profile"])
	}
	if doc["projects"] == nil {
		t.Fatalf("projects encoded as null: %s", out)
	}
}

func TestDoctorStandaloneHasSiteTokenNotHasCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) }); err != nil {
		t.Fatal(err)
	}
	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, raw)
	}
	if strings.Contains(raw, `"has_credential"`) {
		t.Fatalf("old field still present:\n%s", raw)
	}
	var doc struct {
		Workspace struct {
			HasSiteToken bool   `json:"has_site_token"`
			Kind         string `json:"kind"`
		} `json:"workspace"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, raw)
	}
	if doc.Workspace.Kind != config.KindStandalone {
		t.Fatalf("kind %q", doc.Workspace.Kind)
	}
	if doc.Workspace.HasSiteToken {
		t.Fatal("standalone with no site token must report has_site_token=false")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasCredential() {
		t.Fatal("config.HasCredential must stay true for standalone")
	}
}

func TestServeStartHintsUnconfigured(t *testing.T) {
	lines := serveStartHints(&config.Config{})
	found := false
	for _, line := range lines {
		if line == config.ErrNotConfigured.Error() {
			found = true
		}
	}
	if !found {
		t.Fatalf("unconfigured serve must print ErrNotConfigured, got %q", lines)
	}
	st := serveStartHints(&config.Config{Kind: config.KindStandalone})
	for _, line := range st {
		if line == config.ErrNotConfigured.Error() {
			t.Fatalf("standalone must not print init hint: %q", st)
		}
	}
}

func TestCmdServeHandsOffLiveSameProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) }); err != nil {
		t.Fatal(err)
	}
	occupyPreferredServePort(t)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	if err := cmdServe([]string{"--no-open", "--no-sync"}); err != nil {
		t.Fatalf("live serve must hand off, got %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "already serving at") {
		t.Fatalf("handoff log = %q", buf.String())
	}
}

func occupyPreferredServePort(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:7777")
	if err != nil {
		t.Fatalf("listen 127.0.0.1:7777: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	mux := http.NewServeMux()
	mux.HandleFunc(origin.ProbePath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Gadak", "1")
		w.Header().Set("X-Gadak-Profile", config.Profile())
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

func TestCmdServeStaleLockFileDoesNotBusy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) }); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := origin.Client(cfg); err != nil {
		t.Fatal(err)
	}
	origin.ForgetLive()
	if err := os.WriteFile(origin.PersistPath(cfg.Directory())+".lock", []byte("99999"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := origin.Client(cfg); err != nil {
		t.Fatalf("leftover persist.lock must not busy: %v", err)
	}
}

func TestCreateStandaloneUnknownProjectListsAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) }); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdCreate([]string{"hello", "--project", "NOPE", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("unknown project must fail")
	}
	msg := err.Error()
	for _, want := range []string{"does not exist in this workspace", "available:", origin.DefaultProjectKey} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestPageCreateUnknownSpaceListsAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) }); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdPage([]string{"create", "--space", "ENG", "--title", "T", "-m", "hi"})
	})
	if err == nil {
		t.Fatal("unknown space must fail")
	}
	msg := err.Error()
	if strings.Contains(msg, `"statusCode"`) || strings.Contains(msg, `"message"`) {
		t.Fatalf("raw Confluence JSON leaked: %v", err)
	}
	for _, want := range []string{`no space matching "ENG"`, "available:", origin.DefaultSpaceKey} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestPairingRevokeLastWithoutHomeOpensGate(t *testing.T) {
	dir := pairingHome(t)
	if _, _, err := pairing.Mint(dir, "laptop", time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := captureErr(t, func() error {
		return cmdPairing([]string{"revoke", "laptop"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "no active tokens remain — the gate is open again; stop the serve to cut access") {
		t.Fatalf("last-token revoke stderr = %q", stderr)
	}
	out, stderr, err := captureErr(t, func() error { return cmdPairing([]string{"list"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "the gate is open again") {
		t.Fatalf("list after last revoke stderr = %q\nstdout=%s", stderr, out)
	}
}

func TestPairingRevokeLastDeviceKeepsHomeGateClosed(t *testing.T) {
	pairingHome(t)
	if _, _, err := captureErr(t, func() error {
		return cmdPairing([]string{"mint", "--label", "laptop", "--ttl", "1h", "--endpoint", "http://127.0.0.1:9"})
	}); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := captureErr(t, func() error {
		return cmdPairing([]string{"revoke", "laptop"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr, "gate is open") {
		t.Fatalf("_home still holds the gate, but revoke claimed it opened: %q", stderr)
	}
}
