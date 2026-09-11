package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// The URL half of `gadak link` (GDK-530): `gadak link KEY <url> [--title]`
// writes a Jira remote issue link through the origin — the same row family
// `gadak ref` writes — so a PR-shaped URL rides the linked_prs surface with
// zero new UI. The harness is ref_test.go's: a real in-process built-in
// tracker, the only origin remote links are writable on (a Cloud site would
// publish a personal pointer to the whole team; internal/origin/writer.go
// owns that gate).

// newBuiltinWorkspace stands up one built-in-tracker workspace, the way
// ref_test.go does, and tears it down with the test.
func newBuiltinWorkspace(t *testing.T, profile string) {
	t.Helper()
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
	config.SetProfile(profile)
	if out, err := capture(t, func() error { return cmdInit([]string{"--local"}) }); err != nil {
		t.Fatalf("init %s: %v\n%s", profile, err, out)
	}
}

func TestLinkURLWritesRemoteLink(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")

	out, err := capture(t, func() error {
		return cmdLink([]string{mine, "https://github.com/midagedev/gadak/pull/50", "--title", "Fix login"})
	})
	if err != nil {
		t.Fatalf("link KEY url: %v\n%s", err, out)
	}

	// The write went through the origin and came back as a mirror row —
	// `gadak ref --list` reads the same rows (JSON: the text TSV shows the
	// title where one is set, not the URL).
	listed, err := capture(t, func() error { return cmdRef([]string{mine, "--list", "--json"}) })
	if err != nil {
		t.Fatalf("ref --list: %v\n%s", err, listed)
	}
	var refListed struct {
		Refs []struct {
			URL          string `json:"url"`
			Title        string `json:"title"`
			Relationship string `json:"relationship"`
		} `json:"refs"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(listed)), &refListed); err != nil {
		t.Fatalf("ref --list json %q: %v", listed, err)
	}
	if len(refListed.Refs) != 1 {
		t.Fatalf("refs = %+v, want the one link", refListed.Refs)
	}
	got := refListed.Refs[0]
	if got.URL != "https://github.com/midagedev/gadak/pull/50" || got.Title != "Fix login" || got.Relationship != "relates to" {
		t.Fatalf("remote link = %+v", got)
	}

	// And it rides linked_prs: `gadak issue` prints the PR section from the
	// same derivation the web detail response uses.
	issue, err := capture(t, func() error { return cmdIssue([]string{mine}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, issue)
	}
	if !strings.Contains(issue, "Linked PRs (1)") || !strings.Contains(issue, "pull/50") {
		t.Fatalf("issue did not derive the PR from the remote link:\n%s", issue)
	}
}

func TestLinkURLDryRunPlansAndWritesNothing(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")

	out, err := capture(t, func() error {
		return cmdLink([]string{mine, "https://github.com/midagedev/gadak/pull/9", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("link --dry-run: %v\n%s", err, out)
	}
	var plan struct {
		Verb    string `json:"verb"`
		Request struct {
			URL          string `json:"url"`
			Title        string `json:"title"`
			Relationship string `json:"relationship"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &plan); err != nil {
		t.Fatalf("plan %q: %v", out, err)
	}
	if plan.Verb != "link" || plan.Request.URL != "https://github.com/midagedev/gadak/pull/9" {
		t.Fatalf("plan = %+v", plan)
	}
	// No --title typed: the plan carries the URL as the title, the default a
	// real write stores.
	if plan.Request.Title != "https://github.com/midagedev/gadak/pull/9" {
		t.Fatalf("default title = %q, want the URL itself", plan.Request.Title)
	}
	if plan.Request.Relationship != "relates to" {
		t.Fatalf("default relationship = %q, want relates to", plan.Request.Relationship)
	}

	listedJSON, err := capture(t, func() error { return cmdRef([]string{mine, "--list", "--json"}) })
	if err != nil || !strings.Contains(listedJSON, `"refs":[]`) {
		t.Fatalf("dry run wrote something: %v\n%s", err, listedJSON)
	}
}

func TestLinkURLWithTypeIsRefused(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")

	_, err := capture(t, func() error {
		return cmdLink([]string{mine, "https://github.com/midagedev/gadak/pull/9", "--type", "blocks"})
	})
	if err == nil {
		t.Fatal("--type with a URL target must be refused — the two link grammars do not mix")
	}
	if !strings.Contains(err.Error(), "--title") {
		t.Fatalf("error should point at the URL form's flags: %v", err)
	}
	listed, err := capture(t, func() error { return cmdRef([]string{mine, "--list"}) })
	if err != nil || !strings.Contains(listed, "no references") {
		t.Fatalf("refused call wrote something: %v\n%s", err, listed)
	}
}

// A URL that is not a pull request is still a valid remote link — it just
// must not become a linked PR.
func TestLinkURLNonPRIsARemoteLinkNotAPR(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")

	out, err := capture(t, func() error {
		return cmdLink([]string{mine, "https://example.com/design", "--title", "design doc"})
	})
	if err != nil {
		t.Fatalf("link KEY url: %v\n%s", err, out)
	}
	listed, err := capture(t, func() error { return cmdRef([]string{mine, "--list", "--json"}) })
	if err != nil || !strings.Contains(listed, "https://example.com/design") {
		t.Fatalf("remote link missing: %v\n%s", err, listed)
	}
	issue, err := capture(t, func() error { return cmdIssue([]string{mine}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, issue)
	}
	if strings.Contains(issue, "Linked PRs") {
		t.Fatalf("a non-PR URL must not light up the PR section:\n%s", issue)
	}
}
