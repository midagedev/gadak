package main

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestRefCrossWorkspaceHydrates is the GDK-1032 round-trip: a standalone
// issue points at an issue in another workspace, the pointer lands on this
// origin (never on the target's), and the list shows the target's current
// state read out of that workspace's own mirror.
func TestRefCrossWorkspaceHydrates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	allowProfileCreate = true
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})

	// The "team" workspace stands in for a mirrored Jira: another workspace
	// with its own issues, which this one only ever reads.
	config.SetProfile("team")
	if out, err := capture(t, func() error { return cmdInit([]string{"--standalone"}) }); err != nil {
		t.Fatalf("init team: %v\n%s", err, out)
	}
	teamKey := createIssue(t, "the team's issue")
	if out, err := capture(t, func() error { return cmdTransition([]string{teamKey, "In Progress"}) }); err != nil {
		t.Fatalf("transition: %v\n%s", err, out)
	}
	if out, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync team: %v\n%s", err, out)
	}

	// The personal workspace points at it.
	config.SetProfile("plan")
	if out, err := capture(t, func() error { return cmdInit([]string{"--standalone"}) }); err != nil {
		t.Fatalf("init plan: %v\n%s", err, out)
	}
	mine := createIssue(t, "my own note")
	if out, err := capture(t, func() error {
		return cmdRef([]string{mine, "team/" + teamKey, "--as", "blocked by"})
	}); err != nil {
		t.Fatalf("ref: %v\n%s", err, out)
	}

	listed, err := capture(t, func() error { return cmdRef([]string{mine, "--list"}) })
	if err != nil {
		t.Fatalf("ref --list: %v\n%s", err, listed)
	}
	if !strings.Contains(listed, "team/"+teamKey) || !strings.Contains(listed, "blocked by") {
		t.Fatalf("list missing the pointer:\n%s", listed)
	}
	// Hydration: the target's live status, read from the team mirror.
	if !strings.Contains(listed, "In Progress") {
		t.Fatalf("list did not hydrate the target's status:\n%s", listed)
	}
	if !strings.Contains(listed, "the team's issue") {
		t.Fatalf("list did not hydrate the target's summary:\n%s", listed)
	}

	// The pointer survives a sync (the mirror rewrite reads it back from
	// the origin, so a dropped sync path would empty the list).
	if out, err := capture(t, func() error { return cmdSync(nil) }); err != nil {
		t.Fatalf("sync plan: %v\n%s", err, out)
	}
	after, err := capture(t, func() error { return cmdRef([]string{mine, "--list"}) })
	if err != nil || !strings.Contains(after, "team/"+teamKey) {
		t.Fatalf("pointer lost across sync: %v\n%s", err, after)
	}

	// Nothing was written to the team workspace — a personal note about
	// someone else's issue stays personal.
	config.SetProfile("team")
	back, err := capture(t, func() error { return cmdRef([]string{teamKey, "--list"}) })
	if err != nil {
		t.Fatalf("team ref --list: %v\n%s", err, back)
	}
	if !strings.Contains(back, "no references") {
		t.Fatalf("the target workspace was written to:\n%s", back)
	}

	// Removing it takes the row back out.
	config.SetProfile("plan")
	id := strings.SplitN(strings.TrimSpace(strings.Split(after, "\n")[0]), "\t", 2)[0]
	if out, err := capture(t, func() error { return cmdRef([]string{mine, "--rm", id}) }); err != nil {
		t.Fatalf("ref --rm: %v\n%s", err, out)
	}
	gone, err := capture(t, func() error { return cmdRef([]string{mine, "--list"}) })
	if err != nil || !strings.Contains(gone, "no references") {
		t.Fatalf("ref --rm did not remove it: %v\n%s", err, gone)
	}
}

// TestRefUnhydratedTargetIsNotAnError: pointing at a workspace this machine
// does not mirror is a valid pointer with no live state, never a failure.
func TestRefUnhydratedTargetIsNotAnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	allowProfileCreate = true
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})

	config.SetProfile("plan")
	if out, err := capture(t, func() error { return cmdInit([]string{"--standalone"}) }); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	mine := createIssue(t, "note")
	if out, err := capture(t, func() error { return cmdRef([]string{mine, "elsewhere/ABC-1"}) }); err != nil {
		t.Fatalf("ref at an unmirrored workspace must work: %v\n%s", err, out)
	}
	listed, err := capture(t, func() error { return cmdRef([]string{mine, "--list"}) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, listed)
	}
	if !strings.Contains(listed, "elsewhere/ABC-1") || !strings.Contains(listed, "not mirrored here") {
		t.Fatalf("unhydrated pointer should say so:\n%s", listed)
	}

	// A plain URL is a pointer too.
	if out, err := capture(t, func() error {
		return cmdRef([]string{mine, "https://example.com/thing"})
	}); err != nil {
		t.Fatalf("url ref: %v\n%s", err, out)
	}

	// Garbage is refused before any write.
	if _, err := capture(t, func() error { return cmdRef([]string{mine, "not-a-target"}) }); err == nil {
		t.Fatal("a bare token that is neither <workspace>/<KEY> nor a URL must be refused")
	}
}
