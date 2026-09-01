package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/serveaddr"
)

func TestServeScopeLogLocalOriginOmitsAccount(t *testing.T) {
	// GDK-464
	st := serveScopeLog(&config.Config{Kind: config.KindLocalOrigin, DefaultProject: origin.DefaultProjectKey})
	if strings.Contains(st, "this account") {
		t.Fatalf("local-origin serve line still mentions an account: %q", st)
	}
	if st != "syncing "+origin.DefaultProjectKey {
		t.Fatalf("local-origin serve line = %q", st)
	}
	connected := serveScopeLog(&config.Config{
		Site: "https://example.atlassian.net", Email: "a@b.c", Token: "t",
	})
	if connected != "no project filter — syncing everything this account can see" {
		t.Fatalf("connected empty-projects line = %q", connected)
	}
	filtered := serveScopeLog(&config.Config{
		Site: "https://example.atlassian.net", Email: "a@b.c", Token: "t",
		Projects: []string{"NMB"},
	})
	if filtered != "" {
		t.Fatalf("connected with projects must omit the filter line, got %q", filtered)
	}
}

func TestParseServeOpts_AddrPinned(t *testing.T) {
	// Default addr is not a pin — fallback may run on conflict.
	opts, err := parseServeOpts(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.addrPinned {
		t.Fatal("default addr must not set addrPinned")
	}
	if opts.addr != "127.0.0.1:7777" {
		t.Fatalf("addr = %q", opts.addr)
	}

	// Explicit --addr is a pin (rule 3: no fallback).
	opts, err = parseServeOpts([]string{"--addr", "127.0.0.1:9001", "--no-open"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.addrPinned {
		t.Fatal("explicit --addr must set addrPinned")
	}
	if opts.addr != "127.0.0.1:9001" {
		t.Fatalf("addr = %q", opts.addr)
	}
	if !opts.noOpen {
		t.Fatal("noOpen should be true")
	}
}

func TestCheckServeAddr(t *testing.T) {
	cases := []struct {
		name        string
		addr        string
		allowRemote bool
		wantErr     string // substring; empty = ok
	}{
		{name: "loopback default", addr: "127.0.0.1:7777"},
		{name: "localhost", addr: "localhost:7777"},
		{name: "ipv6 loopback", addr: "[::1]:7777"},
		{
			name:    "empty host refused",
			addr:    ":7777",
			wantErr: "without --allow-remote",
		},
		{
			name:    "non-loopback refused",
			addr:    "0.0.0.0:7777",
			wantErr: "without --allow-remote",
		},
		{
			name:    "unspecified ipv6 refused",
			addr:    "[::]:7777",
			wantErr: "without --allow-remote",
		},
		{
			name:    "LAN refused",
			addr:    "192.168.1.10:7777",
			wantErr: "192.168.1.10",
		},
		{
			name:        "non-loopback with allow-remote",
			addr:        "0.0.0.0:7777",
			allowRemote: true,
		},
		{
			name:        "empty host with allow-remote",
			addr:        ":7777",
			allowRemote: true,
		},
		{
			name:        "unspecified ipv6 with allow-remote",
			addr:        "[::]:7777",
			allowRemote: true,
		},
		{
			name:    "bad addr",
			addr:    "not-a-hostport",
			wantErr: "bad --addr",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkServeAddr(tc.addr, tc.allowRemote)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestIsLoopback(t *testing.T) {
	if !isLoopback("127.0.0.1") || !isLoopback("::1") || !isLoopback("localhost") {
		t.Fatal("loopback hosts should be true")
	}
	if isLoopback("0.0.0.0") || isLoopback("192.168.0.1") || isLoopback("example.com") {
		t.Fatal("non-loopback hosts should be false")
	}
}

func TestLocalOriginConfigDoesNotNeedAdvertiseFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Directory(), "serve-origin.json")); !os.IsNotExist(err) {
		t.Fatal("local-origin config must not write serve-origin.json")
	}
}

func TestPublishServeAddrWritesAndRemoves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("oss")
	t.Cleanup(func() { config.SetProfile("") })

	unpublish := publishServeAddr("127.0.0.1:7891")
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	recs := serveaddr.List(dir)
	if len(recs) != 1 || recs[0].Addr != "127.0.0.1:7891" || recs[0].Profile != "oss" {
		t.Fatalf("run file = %+v", recs)
	}
	unpublish()
	if recs := serveaddr.List(dir); len(recs) != 0 {
		t.Fatalf("run file still present: %+v", recs)
	}
}

func TestPublishServeAddrWriteFailureIsNonFatal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	if err := os.WriteFile(filepath.Join(home, serveaddr.Rel), []byte("not-a-dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := publishServeAddr("127.0.0.1:9")
	stop()
}

func TestRunServeHTTPRecordsBoundAddress(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- runServeHTTP(ctx, mux, "127.0.0.1:0", true, true, &config.Config{})
	}()

	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var recs []serveaddr.Record
	for time.Now().Before(deadline) {
		recs = serveaddr.List(dir)
		if len(recs) > 0 {
			break
		}
		select {
		case err := <-errCh:
			t.Fatalf("runServeHTTP exited before advertising: %v", err)
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(recs) != 1 {
		t.Fatalf("run files = %d, want 1", len(recs))
	}
	_, port, err := net.SplitHostPort(recs[0].Addr)
	if err != nil {
		t.Fatal(err)
	}
	if port == "0" {
		t.Fatalf("recorded preferred :0 rather than the bound port: %s", recs[0].Addr)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runServeHTTP: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runServeHTTP did not stop")
	}
	if recs := serveaddr.List(dir); len(recs) != 0 {
		t.Fatalf("run file still present after shutdown: %+v", recs)
	}
}
