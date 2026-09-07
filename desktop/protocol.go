package main

import (
	"fmt"
	"os"
	"strings"
)

// protocolCommand is the HKCU shell\open\command default: quoted exe + "%1".
func protocolCommand(exePath string) string {
	return `"` + exePath + `" "%1"`
}

// protocolDefaultIcon is the DefaultIcon default: quoted exe + ",0".
func protocolDefaultIcon(exePath string) string {
	return `"` + exePath + `",0`
}

func protocolNeedsRewrite(current, want string) bool {
	return current != want
}

// protocolState is the three values a working gadak:// handler is made of:
// the "URL Protocol" marker, DefaultIcon, and shell\open\command — the same
// three writeProtocolScheme writes.
type protocolState struct {
	// URLProtocolSet is whether the class key carries the "URL Protocol"
	// value. Its data is always "" — Windows reads the PRESENCE (a class
	// key without the value is not a protocol handler at all), which is
	// why absence cannot fold into "" the way a missing command does.
	URLProtocolSet bool
	Icon           string
	Command        string
}

// protocolRegistry is the registry surface the registration decision needs.
// The real implementation reads and writes HKCU (protocol_windows.go);
// tests fake it, which is what makes the decision testable off Windows.
type protocolRegistry interface {
	readProtocolState(scheme string) (protocolState, error)
	writeProtocolState(scheme, exePath string) error
}

// protocolStateNeedsRewrite compares all three values. The pre-GDK-1582
// decision compared only shell\open\command, so a stale DefaultIcon — or a
// class key missing "URL Protocol" — survived every launch unchanged as
// long as the exe path in the command still matched.
func protocolStateNeedsRewrite(current, want protocolState) bool {
	return current.URLProtocolSet != want.URLProtocolSet ||
		protocolNeedsRewrite(current.Icon, want.Icon) ||
		protocolNeedsRewrite(current.Command, want.Command)
}

// registerProtocolScheme writes the scheme's class key when what is there
// is not exactly what a current handler would be. GOOS-agnostic because it
// only decides; the reads and writes go through the registry parameter
// (windowsRegistryState in production). A missing key is the zero state —
// nothing registered means "write it" — mirroring readProtocolCommand's
// convention.
func registerProtocolScheme(scheme, exePath string, reg protocolRegistry) (rewrote bool, err error) {
	if scheme == "" || strings.ContainsAny(scheme, `\/`) {
		return false, fmt.Errorf("invalid protocol scheme %q", scheme)
	}
	current, err := reg.readProtocolState(scheme)
	if err != nil {
		return false, err
	}
	want := protocolState{
		URLProtocolSet: true,
		Icon:           protocolDefaultIcon(exePath),
		Command:        protocolCommand(exePath),
	}
	if !protocolStateNeedsRewrite(current, want) {
		return false, nil
	}
	if err := reg.writeProtocolState(scheme, exePath); err != nil {
		return false, err
	}
	return true, nil
}

// protocolRegistersFor reports whether this GOOS registers gadak:// in HKCU.
// macOS uses Info.plist (build-app.sh); Linux xdg-mime is a separate track.
func protocolRegistersFor(goos string) bool {
	return goos == "windows"
}

func hasUnregisterGadakProtocolFlag(args []string) bool {
	for _, a := range args {
		if a == "--unregister-gadak-protocol" {
			return true
		}
	}
	return false
}

// unregisterGadakProtocolIfRequested handles --unregister-gadak-protocol
// before the wails app is created. Always returns true when the flag is
// present so main exits 0 without opening a window. A missing key is not
// an error; a real registry error is written to stderr and still returns.
func unregisterGadakProtocolIfRequested(args []string) bool {
	if !hasUnregisterGadakProtocolFlag(args) {
		return false
	}
	if err := unregisterGadakProtocol(); err != nil {
		fmt.Fprintf(os.Stderr, "gadak:// unregister: %v\n", err)
	}
	fmt.Println("unregistered gadak:// protocol handler")
	return true
}
