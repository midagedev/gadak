//go:build !windows

package main

// gadak:// registration is Windows HKCU only. macOS uses Info.plist
// (build-app.sh); Linux xdg-mime is a separate track.
func registerGadakProtocol(string) (string, error) { return "", nil }

func unregisterGadakProtocol() error { return nil }
