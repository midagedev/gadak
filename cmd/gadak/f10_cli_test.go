package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"syscall"
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
	out, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) })
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

func TestDoctorLocalOriginHasSiteTokenNotHasCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
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
	if doc.Workspace.Kind != config.KindLocalOrigin {
		t.Fatalf("kind %q", doc.Workspace.Kind)
	}
	if doc.Workspace.HasSiteToken {
		t.Fatal("local-origin with no site token must report has_site_token=false")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasCredential() {
		t.Fatal("config.HasCredential must stay true for local-origin")
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
	st := serveStartHints(&config.Config{Kind: config.KindLocalOrigin})
	for _, line := range st {
		if line == config.ErrNotConfigured.Error() {
			t.Fatalf("local-origin must not print init hint: %q", st)
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
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
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

// occupyPreferredServePort stands a fake gadak on serve's default address so
// the handoff path has something live to find.
//
// It has to be that exact address — the point of the test is what serve does
// when its *preferred* port is already gadak — which puts it head-on with
// anyone running `gadak serve` on this machine. That is not a hypothetical
// here: this repo is developed by dogfooding gadak, and a serve left running
// turned `go test ./cmd/gadak` into "bind: address already in use", a failure
// that names a port but not the reason. CI never sees it, so it stayed local
// noise (GDK-982). A busy port means the test cannot run, not that anything
// is broken: skip, and say what to do about it.
func occupyPreferredServePort(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:7777")
	if errors.Is(err, syscall.EADDRINUSE) {
		t.Skipf("127.0.0.1:7777 is taken — this test needs serve's preferred port free. "+
			"A `gadak serve` of your own is the usual holder; stop it and re-run. (%v)", err)
	}
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
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
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

func TestCreateLocalOriginUnknownProjectListsAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
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
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
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

func TestUnknownCommandSuggestsSynonym(t *testing.T) {
	// GDK-1015: the curated map answers the words sessions reach for that are
	// not typos of anything. None of these keys may become real commands —
	// a collision would make the entry dead code.
	for _, tc := range []struct{ in, want string }{
		{"get", "show"},
		{"read", "show"},
		{"ls", "list"},
		{"issues", "list"},
		{"backlog", "list"},
		{"history", "recents"},
		{"finish", "done"},
		{"complete", "done"},
	} {
		if _, ok := commands[tc.in]; ok {
			t.Errorf("%q became a real command; the synonym entry is dead code", tc.in)
		}
		err := unknownCommandError(tc.in)
		if err == nil || err.Error() != fmt.Sprintf("unknown command %q — did you mean \"gadak %s\"? (see gadak --help)", tc.in, tc.want) {
			t.Errorf("%q: err = %v, want suggestion %q", tc.in, err, tc.want)
		}
		if got := exitStatus(err); got != 64 {
			t.Errorf("%q: exitStatus=%d want 64", tc.in, got)
		}
	}
}

func TestUnknownCommandSuggestsNearTypo(t *testing.T) {
	// GDK-1015: fallback edit distance ≤2 over the dispatch names. "et" ties
	// dev/edit/next at 2 and "dn" ties dev/done at 2 — sorted iteration keeps
	// the first strictly-smaller distance, so the lexicographically first
	// candidate wins ("dev" in both). "shw" is distance 1 from show and 2
	// from sql; nearest wins.
	for _, tc := range []struct{ in, want string }{
		{"shw", "show"},
		{"lst", "list"},
		{"et", "dev"},
		{"dn", "dev"},
		{"done2", "done"},
	} {
		if got := suggestCommand(tc.in); got != tc.want {
			t.Errorf("suggestCommand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUnknownCommandDistantNameStaysPlain(t *testing.T) {
	// A bad guess is worse than none: everything is farther than 2 from
	// "xyzzy", so the refusal keeps its bare form (TestUnknownCommandError
	// pins "foobar" byte-for-byte already).
	if got := suggestCommand("xyzzy"); got != "" {
		t.Errorf("suggestCommand(xyzzy) = %q, want none", got)
	}
	err := unknownCommandError("xyzzy")
	if err == nil || err.Error() != `unknown command "xyzzy" — see gadak --help` {
		t.Fatalf("err = %v", err)
	}
	if got := exitStatus(err); got != 64 {
		t.Fatalf("exitStatus=%d want 64", got)
	}
}

func TestBlindSessionVerbAliases(t *testing.T) {
	// show/done/recent/wiki dispatch at the same func as
	// their canonical verb — an alias that drifts to its own entry is a
	// second implementation, not an alias.
	for alias, canonical := range map[string]string{
		"show":   "issue",
		"done":   "close",
		"recent": "recents",
		"wiki":   "page",
	} {
		a, ok := commands[alias]
		if !ok {
			t.Errorf("commands[%q] missing", alias)
			continue
		}
		if reflect.ValueOf(a).Pointer() != reflect.ValueOf(commands[canonical]).Pointer() {
			t.Errorf("commands[%q] is not commands[%q]", alias, canonical)
		}
	}
}

func TestViewIssueKeyAppendsIssueHint(t *testing.T) {
	// GDK-1015: `gadak view GDK-377` is an issue key read as a view name.
	// The FindView refusal stays byte-identical; the CLI layer appends the
	// next step it knows.
	mirror(t, "https://unused.example.com")
	_, _, err := captureErr(t, func() error { return cmdViews([]string{"NMB-140"}) })
	if err == nil {
		t.Fatal("an issue key is not a view name; must fail")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, `no view matching "NMB-140"`) {
		t.Fatalf("first sentence changed: %v", msg)
	}
	if !strings.Contains(msg, "(an issue? try: gadak issue NMB-140)") {
		t.Fatalf("missing issue hint: %v", msg)
	}
}

func TestViewPlainNameKeepsPlainRefusal(t *testing.T) {
	mirror(t, "https://unused.example.com")
	_, _, err := captureErr(t, func() error { return cmdViews([]string{"zzz"}) })
	if err == nil {
		t.Fatal("unknown view must fail")
	}
	if strings.Contains(err.Error(), "an issue? try") {
		t.Fatalf("non-key name must not get the issue hint: %v", err)
	}
}
