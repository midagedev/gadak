package main

import (
	"fmt"
	"os"
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
