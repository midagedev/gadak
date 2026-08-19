//go:build !darwin

package main

import (
	"errors"
	"unsafe"
)

// The in-app browse pane is darwin-only (embed_darwin.go). This stub keeps
// the module compiling on the other pack targets (Linux AppImage, Windows
// portable zip — platform table in README.md). Every call reports the pane
// as unavailable.
type stubEmbedder struct{}

func newPlatformEmbedder(func() unsafe.Pointer) embedder {
	return stubEmbedder{}
}

// No embedded webviews off darwin, so there is nothing to relay Escape past.
func installEscapeRelay() {}

func (stubEmbedder) Create(string, frameRect) (unsafe.Pointer, error) {
	return nil, errors.New("in-app browser pane is macOS-only")
}
func (stubEmbedder) SetFrame(unsafe.Pointer, frameRect)   {}
func (stubEmbedder) SetHidden(unsafe.Pointer, bool)       {}
func (stubEmbedder) Close(unsafe.Pointer)                 {}
func (stubEmbedder) Info(unsafe.Pointer) (string, string) { return "", "" }
