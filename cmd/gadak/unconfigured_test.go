package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// TestUnconfiguredVerbsShareNotConfiguredSentence is GDK-454 FAIL-first:
// an empty workspace used to describe the same state five different ways
// (sync / writes / open / api / pairing list / status). Every branch must
// print config.ErrNotConfigured; verb-specific addenda may follow it.
func TestUnconfiguredVerbsShareNotConfiguredSentence(t *testing.T) {
	emptyHome(t)
	want := config.ErrNotConfigured.Error()

	t.Run("sync", func(t *testing.T) {
		_, err := capture(t, func() error { return cmdSync(nil) })
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("sync: %v", err)
		}
	})
	t.Run("comment", func(t *testing.T) {
		_, err := capture(t, func() error { return cmdComment([]string{"NMB-1", "-m", "hello"}) })
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("comment: %v", err)
		}
	})
	t.Run("open", func(t *testing.T) {
		_, err := capture(t, func() error { return cmdOpen([]string{"NMB-1"}) })
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("open: %v", err)
		}
		if !strings.Contains(err.Error(), "gadak views open NMB-1") {
			t.Fatalf("open must keep the views-open addendum, got %v", err)
		}
	})
	t.Run("api", func(t *testing.T) {
		_, err := capture(t, func() error { return cmdAPI([]string{"/rest/api/3/myself"}) })
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("api: %v", err)
		}
	})
	t.Run("pairing list", func(t *testing.T) {
		_, _, err := captureErr(t, func() error { return cmdPairing([]string{"list"}) })
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("pairing list: %v", err)
		}
	})
	t.Run("status stderr", func(t *testing.T) {
		stdout, stderr, err := captureErr(t, func() error { return cmdStatus(nil) })
		if err != nil {
			t.Fatalf("status must stay exit 0, got %v", err)
		}
		if !strings.Contains(stderr, want) {
			t.Fatalf("status stderr missing not-configured sentence: %q", stderr)
		}
		if !strings.Contains(stdout, "kind") {
			t.Fatalf("status stdout lost the kind line:\n%s", stdout)
		}
	})
	t.Run("status --json stderr", func(t *testing.T) {
		stdout, stderr, err := captureErr(t, func() error { return cmdStatus([]string{"--json"}) })
		if err != nil {
			t.Fatalf("status --json must stay exit 0, got %v", err)
		}
		if !strings.Contains(stderr, want) {
			t.Fatalf("status --json stderr missing not-configured sentence: %q", stderr)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("status --json stdout: %v\n%s", err, stdout)
		}
		if doc["kind"] == nil {
			t.Fatalf("status --json lost kind: %s", stdout)
		}
	})
}

// TestPairedWorkspaceIsNotUnconfigured is GDK-454 × GDK-449: a paired
// workspace has a credential (remote-origin.json). status and pairing list
// must keep describing the pairing, not the empty-home sentence.
func TestPairedWorkspaceIsNotUnconfigured(t *testing.T) {
	seedPairedProfile(t)
	want := config.ErrNotConfigured.Error()

	_, stderr, err := captureErr(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(stderr, want) {
		t.Fatalf("paired status printed the unconfigured sentence:\n%s", stderr)
	}

	out, _, err := captureErr(t, func() error { return cmdPairing([]string{"list"}) })
	if err != nil {
		t.Fatalf("pairing list: %v\n%s", err, out)
	}
	if strings.Contains(out, want) {
		t.Fatalf("paired pairing list printed the unconfigured sentence:\n%s", out)
	}
	if !strings.Contains(out, `this workspace is paired with "laptop"`) {
		t.Fatalf("paired pairing list lost self-status: %q", out)
	}
}
