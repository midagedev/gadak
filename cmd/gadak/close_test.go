package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

func TestCloseIsDoneTransition(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error { return cmdClose([]string{"NMB-1"}) }); err != nil {
		t.Fatalf("close: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "31" {
		t.Fatal("close must fire the done-category transition")
	}
}

func TestCloseAlreadyDoneIsNoop(t *testing.T) {
	f := alreadyDoneOrigin(t)
	out, err := capture(t, func() error {
		return cmdClose([]string{"NMB-1", "-m", "retry"})
	})
	if err != nil {
		t.Fatalf("close already done: %v\n%s", err, out)
	}
	mustNotTransition(t, f, "NMB-1")
	if !strings.Contains(out, "already done — nothing to do") {
		t.Fatalf("human no-op missing: %q", out)
	}
	if !strings.Contains(out, "comment not posted") {
		t.Fatalf("must say the comment was not posted: %q", out)
	}
}

func TestCloseHelp(t *testing.T) {
	out, err := capture(t, func() error { return cmdClose([]string{"--help"}) })
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{
		"status category done",
		"already done is a no-op",
		"gadak close NMB-140",
		"gadak transition NMB-140 inprogress",
		"there is no gadak reopen",
		"--resolution",
		"--field",
		"-m",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q\n%s", want, out)
		}
	}
}

func TestCloseNoKeyIsUsage(t *testing.T) {
	_, err := capture(t, func() error { return cmdClose(nil) })
	if err == nil {
		t.Fatal("missing key must be a usage error")
	}
	if !strings.Contains(err.Error(), "usage: gadak close <KEY>") {
		t.Errorf("usage %q", err)
	}
}

func TestCloseExtraArgIsUsage(t *testing.T) {
	_, err := capture(t, func() error { return cmdClose([]string{"NMB-1", "done"}) })
	if err == nil {
		t.Fatal("extra positional must be usage")
	}
	if !strings.Contains(err.Error(), "usage: gadak close <KEY>") {
		t.Errorf("usage %q", err)
	}
}

// TestCloseStandaloneRoundtrip is the GDK-500 origin check: close posts
// transition+comment in one write, and a second close on an already-done
// issue is exit 0 / changed:false with no extra comment.
func TestCloseStandaloneRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("GADAK_ACTOR", "agent-a|Agent A")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	created, err := capture(t, func() error { return cmdCreate([]string{"close roundtrip"}) })
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	key := strings.Split(strings.TrimSpace(strings.Split(created, "\n")[0]), "\t")[0]

	out, err := capture(t, func() error {
		return cmdClose([]string{key, "-m", "closing out"})
	})
	if err != nil {
		t.Fatalf("close: %v\n%s", err, out)
	}

	issue, err := capture(t, func() error { return cmdIssue([]string{key, "--json"}) })
	if err != nil {
		t.Fatalf("issue after close: %v\n%s", err, issue)
	}
	var doc struct {
		Issue struct {
			StatusCategory string `json:"status_category"`
		} `json:"issue"`
		Comments []struct {
			Body string `json:"body"`
		} `json:"comments"`
	}
	if err := json.Unmarshal([]byte(issue), &doc); err != nil {
		t.Fatalf("decode issue json: %v\n%s", err, issue)
	}
	if doc.Issue.StatusCategory != "done" {
		t.Fatalf("status_category %q, want done", doc.Issue.StatusCategory)
	}
	if len(doc.Comments) != 1 || !strings.Contains(doc.Comments[0].Body, "closing out") {
		t.Fatalf("issuetap did not keep the transition comment: %+v", doc.Comments)
	}

	again, err := capture(t, func() error {
		return cmdClose([]string{key, "-m", "retry", "--json"})
	})
	if err != nil {
		t.Fatalf("second close: %v\n%s", err, again)
	}
	var wrap struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal([]byte(again), &wrap); err != nil {
		t.Fatalf("decode second close: %v\n%s", err, again)
	}
	if wrap.Changed {
		t.Fatalf("second close must be changed=false: %s", again)
	}

	issue2, err := capture(t, func() error { return cmdIssue([]string{key, "--json"}) })
	if err != nil {
		t.Fatalf("issue after second close: %v\n%s", err, issue2)
	}
	if err := json.Unmarshal([]byte(issue2), &doc); err != nil {
		t.Fatalf("decode issue json: %v\n%s", err, issue2)
	}
	if len(doc.Comments) != 1 {
		t.Fatalf("second close posted a comment: %+v", doc.Comments)
	}
}
