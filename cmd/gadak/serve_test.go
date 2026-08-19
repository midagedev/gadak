package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

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
		{name: "empty host ok", addr: ":7777"},
		{
			name:    "non-loopback refused",
			addr:    "0.0.0.0:7777",
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

func TestPublishStandaloneOriginWritesAndRemoves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}

	unpublish := publishStandaloneOrigin(cfg, "127.0.0.1:7998")
	p := origin.AdvertisePath(cfg.Directory())
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("advertise missing: %v", err)
	}
	var adv origin.Advertise
	if err := json.Unmarshal(raw, &adv); err != nil {
		t.Fatal(err)
	}
	if adv.Addr != "127.0.0.1:7998" || adv.PID != os.Getpid() {
		t.Fatalf("advertise = %+v", adv)
	}
	unpublish()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("advertise still present: %v", err)
	}
}

func TestPublishStandaloneOriginSkipsConnected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	cfg := &config.Config{Kind: config.KindConnected}
	unpublish := publishStandaloneOrigin(cfg, "127.0.0.1:7998")
	unpublish()
	if _, err := os.Stat(filepath.Join(home, origin.AdvertiseRel)); !os.IsNotExist(err) {
		t.Fatal("connected workspace must not write serve-origin.json")
	}
}
