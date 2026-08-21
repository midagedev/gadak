//go:build windows

package main

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func testProtocolScheme(t *testing.T) string {
	t.Helper()
	name := strings.ToLower(t.Name())
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, `\`, "-")
	scheme := "gadak-test-" + name
	if scheme == gadakProtocolScheme {
		t.Fatal("test must not use the live gadak scheme")
	}
	return scheme
}

func TestProtocolWindowsRoundTrip(t *testing.T) {
	scheme := testProtocolScheme(t)
	exe := `C:\Temp\gadak-desktop.exe`
	want := protocolCommand(exe)

	t.Cleanup(func() {
		if err := unregisterProtocolScheme(scheme); err != nil {
			t.Errorf("cleanup unregister: %v", err)
		}
	})

	rewrote, err := registerProtocolScheme(scheme, exe)
	if err != nil {
		t.Fatal(err)
	}
	if !rewrote {
		t.Fatal("first register reported already current")
	}
	got, err := readProtocolCommand(scheme)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("command %q, want %q", got, want)
	}

	rewrote, err = registerProtocolScheme(scheme, exe)
	if err != nil {
		t.Fatal(err)
	}
	if rewrote {
		t.Fatal("second register rewrote identical command")
	}

	moved := `D:\Other\gadak-desktop.exe`
	rewrote, err = registerProtocolScheme(scheme, moved)
	if err != nil {
		t.Fatal(err)
	}
	if !rewrote {
		t.Fatal("path change did not rewrite")
	}
	got, err = readProtocolCommand(scheme)
	if err != nil {
		t.Fatal(err)
	}
	if got != protocolCommand(moved) {
		t.Fatalf("after move command %q, want %q", got, protocolCommand(moved))
	}

	if err := unregisterProtocolScheme(scheme); err != nil {
		t.Fatal(err)
	}
	got, err = readProtocolCommand(scheme)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("command still present after unregister: %q", got)
	}
	_, err = registry.OpenKey(registry.CURRENT_USER, protocolClassPath(scheme), registry.QUERY_VALUE)
	if err == nil {
		t.Fatal("scheme key still exists after unregister")
	}
	if !registryMissing(err) {
		t.Fatalf("open after unregister: %v", err)
	}
}
