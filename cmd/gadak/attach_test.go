package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// Contract ↔ assertion:
//
//  1. Happy path: two files uploaded in order; re-read row; `+ filename` lines
//     TestAttachHappyPathTwoFiles
//  2. JSON carries attached ids/filenames
//     TestAttachJSONIncludesAttached
//  3. Missing path: no upload, error names the path
//     TestAttachMissingPathMakesNoUpload
//  4. Mid-list 500: error names landed vs not, non-zero
//     TestAttachMidListFailureReportsLandedAndNot
//  5. No credential → shared errNoCredential
//     TestWritesRefuseToRunWithoutACredential (agent_test.go),
//     TestAttachRefusesWithoutCredential
//  6. Dispatch + help (image example, Writing-through line)
//     TestAttachIsRegisteredAndHelpMentionsImage
//  7. Self-review: directory rejected; symlink-to-file accepted via Stat
//     TestAttachRejectsDirectory, TestAttachAcceptsSymlinkToRegularFile

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAttachHappyPathTwoFiles(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	dir := t.TempDir()
	a := writeTempFile(t, dir, "shot.png", "PNGDATA")
	b := writeTempFile(t, dir, "trace.log", "LOGDATA")

	out, err := capture(t, func() error {
		return cmdAttach([]string{"NMB-1", a, b})
	})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if len(f.uploads) != 2 {
		t.Fatalf("uploads %d, want 2; calls %v", len(f.uploads), f.calls)
	}
	if f.uploads[0].Filename != "shot.png" || f.uploads[0].Content != "PNGDATA" {
		t.Errorf("first upload %+v", f.uploads[0])
	}
	if f.uploads[1].Filename != "trace.log" || f.uploads[1].Content != "LOGDATA" {
		t.Errorf("second upload %+v", f.uploads[1])
	}
	if f.uploads[0].Key != "NMB-1" || f.uploads[1].Key != "NMB-1" {
		t.Errorf("keys %q %q", f.uploads[0].Key, f.uploads[1].Key)
	}
	if f.uploads[0].Token != "no-check" {
		t.Errorf("X-Atlassian-Token %q", f.uploads[0].Token)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want summary + 2 plus-lines, got %q", out)
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) != 4 || fields[0] != "NMB-1" || fields[1] != "완료" {
		t.Fatalf("reread line %q", lines[0])
	}
	if lines[1] != "  + shot.png" || lines[2] != "  + trace.log" {
		t.Fatalf("plus lines %q / %q", lines[1], lines[2])
	}
}

func TestAttachJSONIncludesAttached(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	dir := t.TempDir()
	a := writeTempFile(t, dir, "shot.png", "PNGDATA")
	b := writeTempFile(t, dir, "trace.log", "LOGDATA")

	out, err := capture(t, func() error {
		return cmdAttach([]string{"nmb-1", a, b, "--json"})
	})
	if err != nil {
		t.Fatalf("attach --json: %v\n%s", err, out)
	}
	var res struct {
		Issue    store.IssueLite `json:"issue"`
		Attached []struct {
			ID       string `json:"id"`
			Filename string `json:"filename"`
		} `json:"attached"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Issue.IssueKey != "NMB-1" || res.Issue.Status != "완료" {
		t.Fatalf("json issue is not the re-read row: %+v", res.Issue)
	}
	if len(res.Attached) != 2 || res.Attached[0].ID != "20001" || res.Attached[0].Filename != "shot.png" {
		t.Fatalf("attached %+v", res.Attached)
	}
	if res.Attached[1].ID != "20002" || res.Attached[1].Filename != "trace.log" {
		t.Fatalf("attached[1] %+v", res.Attached[1])
	}
	if strings.Contains(out, "  + ") {
		t.Errorf("json must not print plus-lines: %s", out)
	}
}

func TestAttachMissingPathMakesNoUpload(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	dir := t.TempDir()
	good := writeTempFile(t, dir, "ok.png", "OK")
	missing := filepath.Join(dir, "typo.png")

	_, err := capture(t, func() error {
		return cmdAttach([]string{"NMB-1", good, missing})
	})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing path: %v", err)
	}
	if len(f.uploads) != 0 {
		t.Fatalf("uploaded despite typo: %+v", f.uploads)
	}
	if f.called("POST /issue/NMB-1/attachments") {
		t.Fatalf("upload called: %v", f.calls)
	}
}

func TestAttachMidListFailureReportsLandedAndNot(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.failNthAttach = 2
	dir := t.TempDir()
	a := writeTempFile(t, dir, "one.png", "ONE")
	b := writeTempFile(t, dir, "two.png", "TWO")
	c := writeTempFile(t, dir, "three.png", "THREE")

	_, err := capture(t, func() error {
		return cmdAttach([]string{"NMB-1", a, b, c})
	})
	if err == nil {
		t.Fatal("expected mid-list failure")
	}
	msg := err.Error()
	for _, want := range []string{"one.png", "two.png", "three.png"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if !strings.Contains(msg, "landed") || !strings.Contains(msg, "not attached") {
		t.Errorf("error must name landed vs not: %q", msg)
	}
	if len(f.uploads) != 2 {
		t.Fatalf("want 2 attempts (2nd failed), got %d: %+v", len(f.uploads), f.uploads)
	}
}

func TestAttachRefusesWithoutCredential(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Token = ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	p := writeTempFile(t, t.TempDir(), "a.png", "x")
	_, err := capture(t, func() error {
		return cmdAttach([]string{"NMB-1", p})
	})
	if err == nil || !strings.Contains(err.Error(), "gadak init") {
		t.Fatalf("no credential: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("called Jira anyway: %v", f.calls)
	}
}

func TestAttachRejectsDirectory(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	dir := t.TempDir()

	_, err := capture(t, func() error {
		return cmdAttach([]string{"NMB-1", dir})
	})
	if err == nil || !strings.Contains(err.Error(), dir) || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory: %v", err)
	}
	if len(f.uploads) != 0 {
		t.Fatalf("uploaded a directory: %+v", f.uploads)
	}
}

func TestAttachAcceptsSymlinkToRegularFile(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	dir := t.TempDir()
	target := writeTempFile(t, dir, "real.png", "REAL")
	link := filepath.Join(dir, "alias.png")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdAttach([]string{"NMB-1", link})
	})
	if err != nil {
		t.Fatalf("symlink attach: %v", err)
	}
	if len(f.uploads) != 1 || f.uploads[0].Filename != "alias.png" || f.uploads[0].Content != "REAL" {
		t.Fatalf("uploads %+v", f.uploads)
	}
	if !strings.Contains(out, "  + alias.png") {
		t.Fatalf("plus line %q", out)
	}
}

func TestAttachIsRegisteredAndHelpMentionsImage(t *testing.T) {
	run, ok := commands["attach"]
	if !ok || run == nil {
		t.Fatal("attach missing from dispatch map")
	}
	h, ok := helps["attach"]
	if !ok {
		t.Fatal("attach missing from helps")
	}
	if !strings.Contains(h.usage, "gadak [--profile <name>] attach") {
		t.Errorf("usage: %s", h.usage)
	}
	joined := strings.Join(h.examples, "\n")
	if !strings.Contains(joined, ".png") && !strings.Contains(joined, "screenshot") {
		t.Errorf("examples missing an image:\n%s", joined)
	}
	if !strings.Contains(usage, "attach     attach files") {
		t.Errorf("top-level Writing-through block missing attach:\n%s", usage)
	}
}

func TestAttachRequiresKeyAndFile(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error { return cmdAttach(nil) })
	if err == nil || !strings.Contains(err.Error(), "usage: gadak attach") {
		t.Fatalf("no args: %v", err)
	}
	_, err = capture(t, func() error { return cmdAttach([]string{"NMB-1"}) })
	if err == nil || !strings.Contains(err.Error(), "usage: gadak attach") {
		t.Fatalf("no files: %v", err)
	}
	if len(f.uploads) != 0 {
		t.Fatalf("usage error reached Jira: %v", f.calls)
	}
}
