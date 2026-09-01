package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func TestMCPSyncLoopEnabled(t *testing.T) {
	cred := &config.Config{
		Site:  "https://example.atlassian.net",
		Email: "a@b.c",
		Token: "t",
	}
	localOrigin := &config.Config{Kind: config.KindLocalOrigin}
	cases := []struct {
		name   string
		cfg    *config.Config
		noSync bool
		want   bool
		reason string
	}{
		{name: "connected credential", cfg: cred, want: true},
		{name: "standalone", cfg: localOrigin, want: true},
		{name: "nil config", cfg: nil, reason: "no credential"},
		{name: "empty config", cfg: &config.Config{}, reason: "no credential"},
		{name: "connected missing token", cfg: &config.Config{Site: cred.Site, Email: cred.Email}, reason: "no credential"},
		{name: "no-sync connected", cfg: cred, noSync: true, reason: "no-sync"},
		{name: "no-sync local-origin", cfg: localOrigin, noSync: true, reason: "no-sync"},
		{name: "frozen connected", cfg: &config.Config{Site: cred.Site, Email: cred.Email, Token: cred.Token, Frozen: true}, reason: "frozen"},
		{name: "frozen local-origin", cfg: &config.Config{Kind: config.KindLocalOrigin, Frozen: true}, reason: "frozen"},
		{name: "no-sync wins over frozen", cfg: &config.Config{Site: cred.Site, Email: cred.Email, Token: cred.Token, Frozen: true}, noSync: true, reason: "no-sync"},
		{name: "no-sync wins over no credential", cfg: nil, noSync: true, reason: "no-sync"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := mcpSyncLoopEnabled(tc.cfg, tc.noSync)
			if got != tc.want {
				t.Errorf("enabled = %v, want %v", got, tc.want)
			}
			if reason != tc.reason {
				t.Errorf("reason = %q, want %q", reason, tc.reason)
			}
		})
	}
}

func TestMCPWatchLogGoesToStderrOnly(t *testing.T) {
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	mcpWatchLog("hello-sync")
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = wOut.Close()
	_ = wErr.Close()
	out, _ := io.ReadAll(rOut)
	errb, _ := io.ReadAll(rErr)
	if len(out) != 0 {
		t.Fatalf("stdout polluted: %q", out)
	}
	if !bytes.Contains(errb, []byte("hello-sync")) {
		t.Fatalf("stderr missing log: %q", errb)
	}
	if bytes.Contains(out, []byte("hello-sync")) {
		t.Fatalf("log leaked to stdout")
	}
}

func TestParseMCPOpts(t *testing.T) {
	got, err := parseMCPOpts(nil)
	if err != nil || got {
		t.Fatalf("nil args: noSync=%v err=%v", got, err)
	}
	got, err = parseMCPOpts([]string{"--no-sync"})
	if err != nil || !got {
		t.Fatalf("--no-sync: noSync=%v err=%v", got, err)
	}
	_, err = parseMCPOpts([]string{"--pretty"})
	if err == nil {
		t.Fatal("unknown flag --pretty must fail")
	}
	if !strings.Contains(err.Error(), "--pretty") {
		t.Fatalf("unknown flag error: %v", err)
	}
	_, err = parseMCPOpts([]string{"install"})
	if err == nil {
		t.Fatal("leftover install positional must fail (install is handled before parse)")
	}
}

func TestMCPHelpNamesNoSync(t *testing.T) {
	out := formatHelp("mcp", nil)
	if !strings.Contains(out, "--no-sync") {
		t.Fatalf("mcp help missing --no-sync:\n%s", out)
	}
	if !strings.Contains(out, "do not run the incremental sync loop") {
		t.Fatalf("mcp help --no-sync usage drifted from serve:\n%s", out)
	}
	if !strings.Contains(helps["mcp"].usage, "--no-sync") {
		t.Fatalf("mcp usage missing [--no-sync]: %s", helps["mcp"].usage)
	}
}

func TestMCPNoSyncFlagUsageMatchesServe(t *testing.T) {
	const want = "do not run the incremental sync loop"
	fs := flag.NewFlagSet("probe-mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	noSync := fs.Bool("no-sync", false, want)
	if _, err := parseAround(fs, []string{"--no-sync"}); err != nil {
		t.Fatal(err)
	}
	if !*noSync {
		t.Fatal("parseAround did not set --no-sync")
	}
	got := ""
	fs.VisitAll(func(f *flag.Flag) {
		if f.Name == "no-sync" {
			got = f.Usage
		}
	})
	if got != want {
		t.Fatalf("mcp --no-sync usage = %q, want %q (same as serve.go parseServeOpts)", got, want)
	}
	serveHelp := formatHelp("serve", nil)
	if !strings.Contains(helps["serve"].usage, "--no-sync") {
		t.Fatalf("serve usage dropped --no-sync: %s", helps["serve"].usage)
	}
	_ = serveHelp
}

func TestMCPInstallStillDispatched(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdMCP([]string{"install", "json"})
	})
	if err != nil {
		t.Fatalf("mcp install json: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"mcpServers"`) {
		t.Fatalf("install json lost mcpServers:\n%s", out)
	}
}

func TestMCPSessionStdoutIsJSONRPCOnly(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = inR, outW, errW
	done := make(chan error, 1)
	go func() { done <- cmdMCP(nil) }()
	if _, err := io.WriteString(inW, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	_ = inW.Close()
	cmdErr := <-done
	os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr
	_ = outW.Close()
	_ = errW.Close()
	_ = inR.Close()
	stdout, _ := io.ReadAll(outR)
	stderr, _ := io.ReadAll(errR)
	if cmdErr != nil {
		t.Fatalf("cmdMCP: %v\nstderr:\n%s", cmdErr, stderr)
	}
	if !bytes.Contains(stderr, []byte("sync loop off — no credential")) {
		t.Fatalf("stderr missing loop-off diagnostic:\n%s", stderr)
	}
	dec := json.NewDecoder(bytes.NewReader(stdout))
	var n int
	for {
		var msg map[string]any
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("stdout is not JSON-RPC: %v\n%s", err, stdout)
		}
		n++
		if msg["jsonrpc"] != "2.0" {
			t.Fatalf("stdout frame missing jsonrpc: %v", msg)
		}
	}
	if n == 0 {
		t.Fatalf("stdout empty, want at least the ping response:\n%s", stdout)
	}
	if bytes.Contains(stdout, []byte("sync loop")) || bytes.Contains(stdout, []byte("gadak mcp:")) {
		t.Fatalf("log leaked onto stdout:\n%s", stdout)
	}
}

func TestMCPNoSyncSkipsLoopWithCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg := &config.Config{
		Site:  "https://example.atlassian.net",
		Email: "a@b.c",
		Token: "t",
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = inR, outW, errW
	done := make(chan error, 1)
	go func() { done <- cmdMCP([]string{"--no-sync"}) }()
	if _, err := io.WriteString(inW, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	_ = inW.Close()
	cmdErr := <-done
	os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr
	_ = outW.Close()
	_ = errW.Close()
	_ = inR.Close()
	stdout, _ := io.ReadAll(outR)
	stderr, _ := io.ReadAll(errR)
	if cmdErr != nil {
		t.Fatalf("cmdMCP --no-sync: %v\nstderr:\n%s", cmdErr, stderr)
	}
	if !bytes.Contains(stderr, []byte("sync loop off — no-sync")) {
		t.Fatalf("stderr missing --no-sync diagnostic:\n%s", stderr)
	}
	if bytes.Contains(stdout, []byte("gadak mcp:")) {
		t.Fatalf("log leaked onto stdout:\n%s", stdout)
	}
}
