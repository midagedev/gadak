package main

import (
	"encoding/json"
	"fmt"
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

// GDK-1816 — the two front doors over the one remote-link write.
//
// `gadak link KEY <url>` and `gadak ref KEY <url>` both reach addRemoteLink.
// Before this round they were not equivalent: --dry-run existed on link only,
// --title on link only, --as on ref only. The two tests below are the
// recurrence gate — a capability that lands on one door and not the other is
// red here rather than discovered by a person typing the other spelling.

// remoteLinkDoorOptions reads the option names a verb's --help advertises.
// The help Options block is rendered straight off the verb's FlagSet
// (optionLines/VisitAll), so this reads the flag set itself, not prose.
func remoteLinkDoorOptions(t *testing.T, verb string, run func([]string) error) map[string]bool {
	t.Helper()
	out, err := capture(t, func() error { return run([]string{"--help"}) })
	if err != nil {
		t.Fatalf("%s --help: %v\n%s", verb, err, out)
	}
	opts := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "  --") {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(line), "--")
		if i := strings.IndexAny(name, " \t"); i >= 0 {
			name = name[:i]
		}
		opts[name] = true
	}
	if len(opts) == 0 {
		t.Fatalf("%s --help listed no options:\n%s", verb, out)
	}
	return opts
}

// TestRemoteLinkDoorsOfferTheSameOptions: link and ref are one write with two
// spellings, so their option sets may differ only by the grammar each door
// owns — --type is link's issue-link half, --list/--rm are ref's read and
// delete halves. Everything else must be on both doors.
func TestRemoteLinkDoorsOfferTheSameOptions(t *testing.T) {
	linkOnly := map[string]bool{"type": true}
	refOnly := map[string]bool{"list": true, "rm": true}

	linkOpts := remoteLinkDoorOptions(t, "link", cmdLink)
	refOpts := remoteLinkDoorOptions(t, "ref", cmdRef)

	for name := range linkOpts {
		if linkOnly[name] || refOpts[name] {
			continue
		}
		t.Errorf("gadak link offers --%s and gadak ref does not — the same remote-link write must be callable the same way through both doors", name)
	}
	for name := range refOpts {
		if refOnly[name] || linkOpts[name] {
			continue
		}
		t.Errorf("gadak ref offers --%s and gadak link does not — the same remote-link write must be callable the same way through both doors", name)
	}
	// The ones the audit found missing, named so a regression reads plainly.
	for _, want := range []string{"title", "as", "dry-run", "json"} {
		if !linkOpts[want] {
			t.Errorf("gadak link --help does not offer --%s", want)
		}
		if !refOpts[want] {
			t.Errorf("gadak ref --help does not offer --%s", want)
		}
	}
}

// TestRemoteLinkDoorsPlanTheSameWrite: given the same options, the plan the
// two doors print is identical apart from the verb naming the door the person
// typed. That is what makes --dry-run answer "are these two commands the same
// write?" — the answer is the JSON, not the source.
func TestRemoteLinkDoorsPlanTheSameWrite(t *testing.T) {
	newBuiltinWorkspace(t, "plan")
	mine := createIssue(t, "the issue")

	plan := func(verb string, run func([]string) error, args []string) map[string]any {
		t.Helper()
		out, err := capture(t, func() error { return run(args) })
		if err != nil {
			t.Fatalf("%s --dry-run: %v\n%s", verb, err, out)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &body); err != nil {
			t.Fatalf("%s plan %q: %v", verb, out, err)
		}
		if body["verb"] != verb {
			t.Errorf("%s plan named verb %v", verb, body["verb"])
		}
		if body["key"] != mine {
			t.Errorf("%s plan named key %v, want %s", verb, body["key"], mine)
		}
		req, ok := body["request"].(map[string]any)
		if !ok {
			t.Fatalf("%s plan carries no request: %v", verb, body)
		}
		return req
	}

	url := "https://github.com/midagedev/gadak/pull/77"
	linkReq := plan("link", cmdLink, []string{mine, url, "--title", "Fix login", "--as", "blocked by", "--dry-run"})
	refReq := plan("ref", cmdRef, []string{mine, url, "--title", "Fix login", "--as", "blocked by", "--dry-run"})

	// Logged so `go test -v` answers the question this gate exists for
	// without anyone re-deriving it from the source.
	t.Logf("link plans %v", linkReq)
	t.Logf(" ref plans %v", refReq)
	if fmt.Sprint(linkReq) != fmt.Sprint(refReq) {
		t.Fatalf("the two doors plan different writes:\n link: %v\n  ref: %v", linkReq, refReq)
	}
	// And both carry the chosen title *and* the chosen relationship — the
	// pair addRemoteLink always supported and neither door could pass.
	if linkReq["title"] != "Fix login" || linkReq["relationship"] != "blocked by" {
		t.Fatalf("--title and --as did not both reach the write: %v", linkReq)
	}

	// Neither plan wrote anything.
	listed, err := capture(t, func() error { return cmdRef([]string{mine, "--list", "--json"}) })
	if err != nil || !strings.Contains(listed, `"refs":[]`) {
		t.Fatalf("a dry run wrote something: %v\n%s", err, listed)
	}
}
