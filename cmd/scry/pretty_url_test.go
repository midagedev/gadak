package main

import (
	"context"
	"errors"
	"testing"
)

func TestPrettyOpenURL(t *testing.T) {
	loopbackLookup := func(context.Context, string) ([]string, error) {
		return []string{"127.0.0.1", "::1"}, nil
	}
	failLookup := func(context.Context, string) ([]string, error) {
		return nil, errors.New("no such host")
	}
	poisonedLookup := func(context.Context, string) ([]string, error) {
		// Contaminated resolver: includes a non-loopback address.
		return []string{"127.0.0.1", "8.8.8.8"}, nil
	}
	emptyLookup := func(context.Context, string) ([]string, error) {
		return nil, nil
	}

	cases := []struct {
		name     string
		bindHost string
		port     string
		lookup   hostLookup
		want     string
	}{
		{
			name:     "loopback bind + loopback resolve → scry.localhost",
			bindHost: "127.0.0.1",
			port:     "7777",
			lookup:   loopbackLookup,
			want:     "http://scry.localhost:7777",
		},
		{
			name:     "localhost bind + loopback resolve → scry.localhost",
			bindHost: "localhost",
			port:     "7777",
			lookup:   loopbackLookup,
			want:     "http://scry.localhost:7777",
		},
		{
			name:     "::1 bind + loopback resolve → scry.localhost",
			bindHost: "::1",
			port:     "7777",
			lookup:   loopbackLookup,
			want:     "http://scry.localhost:7777",
		},
		{
			name:     "empty bind host + loopback resolve → scry.localhost",
			bindHost: "",
			port:     "7777",
			lookup:   loopbackLookup,
			want:     "http://scry.localhost:7777",
		},
		{
			name:     "loopback bind + lookup failure → fallback",
			bindHost: "127.0.0.1",
			port:     "7777",
			lookup:   failLookup,
			want:     "http://127.0.0.1:7777",
		},
		{
			name:     "loopback bind + empty lookup → fallback",
			bindHost: "127.0.0.1",
			port:     "7777",
			lookup:   emptyLookup,
			want:     "http://127.0.0.1:7777",
		},
		{
			name:     "loopback bind + poisoned resolve → fallback",
			bindHost: "127.0.0.1",
			port:     "7777",
			lookup:   poisonedLookup,
			want:     "http://127.0.0.1:7777",
		},
		{
			name:     "non-loopback bind → keep LAN host (no pretty)",
			bindHost: "192.168.1.10",
			port:     "7777",
			lookup:   loopbackLookup,
			want:     "http://192.168.1.10:7777",
		},
		{
			name:     "localhost bind + lookup failure → localhost fallback",
			bindHost: "localhost",
			port:     "7878",
			lookup:   failLookup,
			want:     "http://localhost:7878",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := prettyOpenURL(tc.bindHost, tc.port, tc.lookup)
			if got != tc.want {
				t.Fatalf("prettyOpenURL(%q, %q) = %q, want %q", tc.bindHost, tc.port, got, tc.want)
			}
		})
	}
}

func TestBrowseAddrUsesPrettyHost(t *testing.T) {
	// Integration with browseAddr: only asserts shape when the real resolver
	// is healthy; injected path is covered above. When scry.localhost is not
	// all-loopback, browseAddr must still return a usable URL.
	got := browseAddr("127.0.0.1:7777")
	if got != "http://scry.localhost:7777" && got != "http://127.0.0.1:7777" {
		t.Fatalf("browseAddr = %q, want scry.localhost or 127.0.0.1 form", got)
	}
}
