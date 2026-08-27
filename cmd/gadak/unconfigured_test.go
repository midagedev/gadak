package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// TestWriteVerbsRefuseNotConfiguredInEmptyHome is GDK-943 FAIL-first: the
// same empty home used to reach each write verb in its own dialect — pages
// quoted origin's errNeedCredential ("site, email and token are required"),
// project create and the dev verbs diagnosed a "connected workspace" that
// does not exist. The refusal is one decision: every write verb answers
// config.ErrNotConfigured here (errors.Is, so verb addenda may follow).
// A write verb born later joins this table the day it is born.
func TestWriteVerbsRefuseNotConfiguredInEmptyHome(t *testing.T) {
	emptyHome(t)
	verbs := []struct {
		name string
		run  func() error
	}{
		{"comment", func() error { return cmdComment([]string{"NMB-1", "-m", "hello"}) }},
		{"transition", func() error { return cmdTransition([]string{"NMB-1", "Done"}) }},
		{"assign", func() error { return cmdAssign([]string{"NMB-1", "someone@example.com"}) }},
		{"edit", func() error { return cmdEdit([]string{"NMB-1", "--summary", "new"}) }},
		{"page create", func() error { return cmdPageCreate([]string{"--space", "LOC", "--title", "T", "-m", "body"}) }},
		{"page edit", func() error { return cmdPageEdit([]string{"NMB-1", "--title", "T"}) }},
		{"page comment", func() error { return cmdPageComment([]string{"NMB-1", "-m", "hello"}) }},
		{"project create", func() error { return cmdProjectCreate([]string{"NMBKEY"}) }},
		{"dev link", func() error {
			return cmdDevLink([]string{"NMB-1", "--pr", "https://example.com/pr/1", "--branch", "main"})
		}},
		{"dev deploy", func() error { return cmdDevDeploy([]string{"NMB-1", "--env", "production", "--state", "successful"}) }},
	}
	for _, v := range verbs {
		t.Run(v.name, func(t *testing.T) {
			_, err := capture(t, v.run)
			if err == nil {
				t.Fatalf("%s: empty home must refuse, got success", v.name)
			}
			if !errors.Is(err, config.ErrNotConfigured) {
				t.Fatalf("%s: refusal must wrap config.ErrNotConfigured, got: %v", v.name, err)
			}
		})
	}
}

// TestConnectedNoTokenPageWriteKeepsCredentialRefusal is GDK-943's other
// half: an origin that exists but whose credential is incomplete (site and
// email set, token cleared) must keep origin's connected refusal for pages
// — errNeedCredential — not the empty-home sentence. The no-origin fold
// must not swallow the connected dialect.
func TestConnectedNoTokenPageWriteKeepsCredentialRefusal(t *testing.T) {
	dir := emptyHome(t)
	cfgJSON := `{"site": "https://conf-token-cleared.example.com", "email": "u@example.com"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfgJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdPageCreate([]string{"--space", "LOC", "--title", "T", "-m", "body"})
	})
	if err == nil || !strings.Contains(err.Error(), "origin: site, email and token are required") {
		t.Fatalf("connected workspace without token must keep errNeedCredential, got: %v", err)
	}
	if errors.Is(err, config.ErrNotConfigured) {
		t.Fatalf("a workspace with a site must not read as unconfigured: %v", err)
	}
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
