package main

import (
	"errors"
	"testing"
)

func TestProtocolCommand(t *testing.T) {
	exe := `C:\Program Files\Gadak\gadak-desktop.exe`
	got := protocolCommand(exe)
	want := `"C:\Program Files\Gadak\gadak-desktop.exe" "%1"`
	if got != want {
		t.Fatalf("protocolCommand = %q, want %q", got, want)
	}
}

// GDK-1578: the DefaultIcon spelling, pinned by a table on every GOOS —
// this reference is also what kept protocolDefaultIcon out of staticcheck's
// cross-platform dead-code findings (its only production caller is the
// windows registration).
func TestProtocolDefaultIcon(t *testing.T) {
	table := []struct {
		exe  string
		want string
	}{
		{`C:\Gadak\gadak-desktop.exe`, `"C:\Gadak\gadak-desktop.exe",0`},
		{`C:\Program Files\Gadak\gadak-desktop.exe`, `"C:\Program Files\Gadak\gadak-desktop.exe",0`},
		{`D:\a,b\gadak-desktop.exe`, `"D:\a,b\gadak-desktop.exe",0`},
	}
	for _, tc := range table {
		if got := protocolDefaultIcon(tc.exe); got != tc.want {
			t.Errorf("protocolDefaultIcon(%q) = %q, want %q", tc.exe, got, tc.want)
		}
	}
}

func TestProtocolNeedsRewrite(t *testing.T) {
	want := protocolCommand(`C:\Gadak\gadak-desktop.exe`)
	if protocolNeedsRewrite(want, want) {
		t.Fatal("identical command needs rewrite")
	}
	if !protocolNeedsRewrite("", want) {
		t.Fatal("empty current skipped rewrite")
	}
	moved := protocolCommand(`D:\Gadak\gadak-desktop.exe`)
	if !protocolNeedsRewrite(moved, want) {
		t.Fatal("path change skipped rewrite")
	}
}

// GDK-1582: the registration decision compares all three values a handler
// is made of, not just the command. The rows that used to pass unnoticed
// under the command-only comparison are the point: a stale icon and a
// missing "URL Protocol" must each force the rewrite.
func TestProtocolStateNeedsRewrite(t *testing.T) {
	exe := `C:\Gadak\gadak-desktop.exe`
	want := protocolState{
		URLProtocolSet: true,
		Icon:           protocolDefaultIcon(exe),
		Command:        protocolCommand(exe),
	}
	table := []struct {
		name    string
		current protocolState
		rewrite bool
	}{
		{"identical state", want, false},
		{"fresh machine (nothing registered)", protocolState{}, true},
		{"command moved", func() protocolState {
			s := want
			s.Command = protocolCommand(`D:\Gadak\gadak-desktop.exe`)
			return s
		}(), true},
		// The GDK-1582 rows: command still names this exe, but the other
		// two values drifted. Command-only comparison read this as current.
		{"stale icon", func() protocolState {
			s := want
			s.Icon = protocolDefaultIcon(`D:\Old\gadak-desktop.exe`)
			return s
		}(), true},
		{"missing URL Protocol marker", func() protocolState {
			s := want
			s.URLProtocolSet = false
			return s
		}(), true},
	}
	for _, tc := range table {
		if got := protocolStateNeedsRewrite(tc.current, want); got != tc.rewrite {
			t.Errorf("%s: protocolStateNeedsRewrite = %v, want %v", tc.name, got, tc.rewrite)
		}
	}
}

// fakeProtocolRegistry stands in for the HKCU registry so the whole
// decision — read, compare, write — runs on any GOOS (the windows
// round-trip test covers the real registry).
type fakeProtocolRegistry struct {
	state    protocolState
	readErr  error
	writes   []string
	writeErr error
}

func (f *fakeProtocolRegistry) readProtocolState(scheme string) (protocolState, error) {
	return f.state, f.readErr
}

func (f *fakeProtocolRegistry) writeProtocolState(scheme, exePath string) error {
	f.writes = append(f.writes, scheme+`|`+exePath)
	return f.writeErr
}

// GDK-1582: registerProtocolScheme through the fake registry — the stale
// icon and missing-marker rows must actually reach the write, not just
// flip a comparison in isolation.
func TestRegisterProtocolSchemeRewritesForAllThreeValues(t *testing.T) {
	exe := `C:\Gadak\gadak-desktop.exe`
	current := protocolState{
		URLProtocolSet: true,
		Icon:           protocolDefaultIcon(exe),
		Command:        protocolCommand(exe),
	}
	reg := &fakeProtocolRegistry{state: current}
	rewrote, err := registerProtocolScheme("gadak-test", exe, reg)
	if err != nil {
		t.Fatal(err)
	}
	if rewrote || len(reg.writes) != 0 {
		t.Fatalf("identical state rewrote (rewrote=%v writes=%v)", rewrote, reg.writes)
	}

	stale := current
	stale.Icon = protocolDefaultIcon(`D:\Old\gadak-desktop.exe`)
	reg = &fakeProtocolRegistry{state: stale}
	rewrote, err = registerProtocolScheme("gadak-test", exe, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !rewrote || len(reg.writes) != 1 {
		t.Fatalf("stale icon must rewrite (rewrote=%v writes=%v)", rewrote, reg.writes)
	}

	noMarker := current
	noMarker.URLProtocolSet = false
	reg = &fakeProtocolRegistry{state: noMarker}
	rewrote, err = registerProtocolScheme("gadak-test", exe, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !rewrote || len(reg.writes) != 1 {
		t.Fatalf("missing URL Protocol must rewrite (rewrote=%v writes=%v)", rewrote, reg.writes)
	}
}

func TestRegisterProtocolSchemeSurfacesRegistryErrors(t *testing.T) {
	boom := errors.New("registry read failed")
	reg := &fakeProtocolRegistry{readErr: boom}
	if _, err := registerProtocolScheme("gadak-test", `C:\Gadak\gadak-desktop.exe`, reg); !errors.Is(err, boom) {
		t.Fatalf("read error %v, want the registry error verbatim", err)
	}
	wboom := errors.New("registry write failed")
	reg = &fakeProtocolRegistry{writeErr: wboom}
	if _, err := registerProtocolScheme("gadak-test", `C:\Gadak\gadak-desktop.exe`, reg); !errors.Is(err, wboom) {
		t.Fatalf("write error %v, want the registry error verbatim", err)
	}
	// The scheme guard runs before any registry touch.
	if _, err := registerProtocolScheme(`bad/scheme`, `C:\Gadak\gadak-desktop.exe`, reg); err == nil {
		t.Fatal("scheme with a separator must be rejected")
	}
}

func TestProtocolRegistersFor(t *testing.T) {
	table := []struct {
		goos string
		want bool
	}{
		{"windows", true},
		{"darwin", false},
		{"linux", false},
		{"freebsd", false},
		{"js", false},
		{"", false},
	}
	for _, tc := range table {
		if got := protocolRegistersFor(tc.goos); got != tc.want {
			t.Errorf("protocolRegistersFor(%q) = %v, want %v", tc.goos, got, tc.want)
		}
	}
}

func TestUnregisterGadakProtocolFlag(t *testing.T) {
	if hasUnregisterGadakProtocolFlag(nil) {
		t.Fatal("empty args should not unregister")
	}
	if hasUnregisterGadakProtocolFlag([]string{"gadak://view"}) {
		t.Fatal("deeplink is not the unregister flag")
	}
	if hasUnregisterGadakProtocolFlag([]string{"--print-window-chrome"}) {
		t.Fatal("chrome flag is not the unregister flag")
	}
	if !hasUnregisterGadakProtocolFlag([]string{"--unregister-gadak-protocol"}) {
		t.Fatal("flag not recognized")
	}
}
