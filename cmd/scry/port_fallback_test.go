package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDecidePortBusy_SameProfileScry(t *testing.T) {
	probe := func(port string) scryProbe {
		return scryProbe{IsScry: true, Profile: "work"}
	}
	got := decidePortBusy("127.0.0.1:7777", false, "work", probe)
	if got.Action != portActionOpenExisting {
		t.Fatalf("action = %v, want openExisting", got.Action)
	}
	if !strings.Contains(got.ExistingURL, "7777") {
		t.Fatalf("ExistingURL = %q, want port 7777", got.ExistingURL)
	}
}

func TestDecidePortBusy_OtherProfileOrProcess(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		probe   scryProbe
		wantSub string // substring of occupant reason
	}{
		{
			name:    "other scry profile",
			profile: "default",
			probe:   scryProbe{IsScry: true, Profile: "work"},
			wantSub: `scry profile "work"`,
		},
		{
			name:    "empty vs named",
			profile: "",
			probe:   scryProbe{IsScry: true, Profile: "demo"},
			wantSub: `scry profile "demo"`,
		},
		{
			name:    "not scry",
			profile: "work",
			probe:   scryProbe{},
			wantSub: "another process",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decidePortBusy("127.0.0.1:7777", false, tc.profile, func(string) scryProbe {
				return tc.probe
			})
			if got.Action != portActionFallback {
				t.Fatalf("action = %v, want fallback", got.Action)
			}
			if !strings.Contains(got.Occupant, tc.wantSub) {
				t.Fatalf("Occupant = %q, want substring %q", got.Occupant, tc.wantSub)
			}
		})
	}
}

func TestDecidePortBusy_PinnedAddrFails(t *testing.T) {
	// Explicit --addr: never fall back; surface scry identity when present.
	got := decidePortBusy("127.0.0.1:9001", true, "work", func(string) scryProbe {
		return scryProbe{IsScry: true, Profile: "other"}
	})
	if got.Action != portActionFail {
		t.Fatalf("action = %v, want fail", got.Action)
	}
	if !strings.Contains(got.ErrDetail, "scry") {
		t.Fatalf("ErrDetail = %q, want scry mention", got.ErrDetail)
	}

	got2 := decidePortBusy("127.0.0.1:9001", true, "work", func(string) scryProbe {
		return scryProbe{}
	})
	if got2.Action != portActionFail {
		t.Fatalf("action = %v, want fail", got2.Action)
	}
}

func TestDecidePortBusy_PinnedSameProfileStillFails(t *testing.T) {
	// Pin means "this address or error" — even same-profile scry must not
	// silently open-existing when the user forced the port? Spec rule 3:
	// "명시 addr였으면 폴백하지 않는다" and "점유자가 scry로 식별되면 에러 메시지에 부가".
	// Open-existing (rule 1) only applies to default addr. Pinned → fail.
	got := decidePortBusy("127.0.0.1:7777", true, "work", func(string) scryProbe {
		return scryProbe{IsScry: true, Profile: "work"}
	})
	if got.Action != portActionFail {
		t.Fatalf("action = %v, want fail (pinned never open-existing)", got.Action)
	}
	if !strings.Contains(got.ErrDetail, "scry") {
		t.Fatalf("ErrDetail = %q, want scry mention", got.ErrDetail)
	}
}

func TestBindListen_FallbackOnEADDRINUSE(t *testing.T) {
	// Real sockets: hold a port, then bind with fallback must land on the next free one.
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	_, portStr, err := net.SplitHostPort(hold.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	// Also hold port+1 so fallback must skip at least one.
	hold2, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port+1))
	if err != nil {
		t.Fatalf("hold second port: %v", err)
	}
	defer hold2.Close()

	want := fmt.Sprintf("127.0.0.1:%d", port)
	ln, bound, existing, err := bindListen(want, false, "work",
		func(string) scryProbe { return scryProbe{} }, // not scry
		net.Listen,
	)
	if err != nil {
		t.Fatalf("bindListen: %v", err)
	}
	if existing != "" {
		t.Fatalf("unexpected existing URL %q", existing)
	}
	if ln == nil {
		t.Fatal("nil listener")
	}
	defer ln.Close()
	_, gotPort, err := net.SplitHostPort(bound)
	if err != nil {
		t.Fatal(err)
	}
	gotN, _ := strconv.Atoi(gotPort)
	if gotN != port+2 {
		t.Fatalf("bound port = %d, want %d (skipped busy %d and %d)", gotN, port+2, port, port+1)
	}
}

func TestBindListen_OpenExistingSameProfile(t *testing.T) {
	// A real HTTP server that answers with identity headers; bind must not
	// take another port when profile matches.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/issues/sync/progress/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Scry", "1")
		w.Header().Set("X-Scry-Profile", "work")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"phase":"idle"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// httptest binds 127.0.0.1:port — parse it.
	u := strings.TrimPrefix(srv.URL, "http://")
	host, port, err := net.SplitHostPort(u)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		// Some environments use localhost; force loopback probe path via real listen.
		t.Skipf("httptest host %q not 127.0.0.1", host)
	}
	addr := net.JoinHostPort(host, port)

	// Occupy the same port with a plain listener so bind fails with EADDRINUSE.
	// The httptest server already holds it.
	ln, bound, existing, err := bindListen(addr, false, "work",
		func(p string) scryProbe {
			return probeScryOnPort(p, 700*time.Millisecond)
		},
		net.Listen,
	)
	if err != nil {
		t.Fatalf("bindListen: %v", err)
	}
	if ln != nil {
		ln.Close()
		t.Fatal("expected nil listener for open-existing")
	}
	if bound != "" {
		t.Fatalf("bound = %q, want empty", bound)
	}
	if existing == "" || !strings.Contains(existing, port) {
		t.Fatalf("existing = %q, want URL with port %s", existing, port)
	}
}

func TestIsAddrInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_, err = net.Listen("tcp", addr)
	if err == nil {
		t.Fatal("expected second listen to fail")
	}
	if !isAddrInUse(err) {
		t.Fatalf("isAddrInUse(%v) = false", err)
	}
	ln.Close()
	if isAddrInUse(errors.New("something else")) {
		t.Fatal("plain error should not match")
	}
	if isAddrInUse(syscall.EADDRINUSE) {
		// direct sentinel should match via errors.Is
	} else {
		// wrap it
		if !isAddrInUse(fmt.Errorf("wrap: %w", syscall.EADDRINUSE)) {
			t.Fatal("wrapped EADDRINUSE should match")
		}
	}
}

func TestShiftPort(t *testing.T) {
	got, err := shiftPort("127.0.0.1:7777", 4)
	if err != nil {
		t.Fatal(err)
	}
	if got != "127.0.0.1:7781" {
		t.Fatalf("got %q", got)
	}
}
