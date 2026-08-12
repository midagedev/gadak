//go:build !darwin

package main

import (
	"errors"
	"unsafe"
)

// The desktop app ships for macOS only today; this stub keeps the module
// compiling elsewhere (same arrangement as install_cli). Every call reports
// the pane as unavailable.
type stubEmbedder struct{}

func newPlatformEmbedder(func() unsafe.Pointer) embedder {
	return stubEmbedder{}
}

func (stubEmbedder) Create(string, frameRect) (unsafe.Pointer, error) {
	return nil, errors.New("in-app browser pane is macOS-only")
}
func (stubEmbedder) SetFrame(unsafe.Pointer, frameRect)   {}
func (stubEmbedder) SetHidden(unsafe.Pointer, bool)       {}
func (stubEmbedder) Close(unsafe.Pointer)                 {}
func (stubEmbedder) Info(unsafe.Pointer) (string, string) { return "", "" }
