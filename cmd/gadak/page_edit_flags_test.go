package main

import (
	"strings"
	"testing"
)

// TestCmdPageEditVersionFlagParses is FAIL-first for GDK-408: --version must
// be a registered flag so the error is "nothing to change", not unknown flag.
// Stops before origin (no title / -m / --adf-file).
func TestCmdPageEditVersionFlagParses(t *testing.T) {
	err := cmdPageEdit([]string{"12345", "--version", "3"})
	if err == nil {
		t.Fatal("expected a usage error (nothing to change), got success")
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--version must be registered on page edit, got %v", err)
	}
	if !strings.Contains(err.Error(), "nothing to change") {
		t.Fatalf("got %v, want nothing to change", err)
	}
}

// TestCmdPageEditRequiresPageID is an offline input-validation path: --version
// and --title are not enough without the page id positional.
func TestCmdPageEditRequiresPageID(t *testing.T) {
	err := cmdPageEdit([]string{"--title", "Renamed", "--version", "2"})
	if err == nil {
		t.Fatal("expected exactly-one-id error")
	}
	if !strings.Contains(err.Error(), "exactly one page id") {
		t.Fatalf("got %v, want exactly one page id", err)
	}
}
