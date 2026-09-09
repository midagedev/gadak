package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// GDK-1446: --dry-run on every write verb — "a Jira-workspace agent prints
// the exact planned write to a person". Each test runs the verb, asserts the
// plan landed on stdout as JSON, and asserts the fake saw no write method.
// `transition --batch --dry-run` already existed; these are the rest.

// decodeDryRun unmarshals one --dry-run plan document.
func decodeDryRun(t *testing.T, out string) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("dry-run stdout is not one JSON object: %v\n%s", err, out)
	}
	if body["dry_run"] != true {
		t.Fatalf("dry_run marker missing:\n%s", out)
	}
	return body
}

func TestCreateDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdCreate([]string{"dry run probe", "--project", "NMB", "--type", "Task", "--label", "gap", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("create --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	req, _ := body["request"].(map[string]any)
	fields, _ := req["fields"].(map[string]any)
	if fields["summary"] != "dry run probe" {
		t.Fatalf("plan fields: %v", req)
	}
	if f.called("POST /issue") {
		t.Fatalf("dry run created an issue: %v", f.calls)
	}
}

func TestCreateBatchDryRunPerLine(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t,
		`{"summary":"first"}`+"\n"+
			`{"summary":"second"}`+"\n")

	out, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("create --batch --dry-run: %v\n%s", err, out)
	}
	if got := strings.Count(out, `"dry_run":true`); got != 2 {
		t.Fatalf("want one plan per line, got %d:\n%s", got, out)
	}
	if f.called("POST /issue") {
		t.Fatalf("dry run created issues: %v", f.calls)
	}
}

func TestEditDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--summary", "dry run summary", "--label", "+gap", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("edit --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	if body["key"] != "NMB-1" {
		t.Fatalf("plan key: %v", body)
	}
	req, _ := body["request"].(map[string]any)
	fields, _ := req["fields"].(map[string]any)
	if fields["summary"] != "dry run summary" {
		t.Fatalf("plan fields: %v", req)
	}
	if len(f.calls) > 0 && strings.HasPrefix(f.calls[len(f.calls)-1], "PUT") {
		t.Fatalf("dry run edited: %v", f.calls)
	}
	noWriteTag(t, f)
}

func TestCommentDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "dry run body", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("comment --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	if body["key"] != "NMB-1" {
		t.Fatalf("plan key: %v", body)
	}
	req, _ := body["request"].(map[string]any)
	// The plan carries the typed text (the person's words), not the ADF the
	// origin would wrap them in.
	if req["body"] != "dry run body" {
		t.Fatalf("plan body: %v", req)
	}
	noWriteTag(t, f)
}

func TestCommentRmDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdCommentRm([]string{"NMB-1", "91653", "--yes", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("comment rm --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	if body["key"] != "NMB-1" {
		t.Fatalf("plan key: %v", body)
	}
	if len(f.deletedComments) != 0 {
		t.Fatalf("dry run deleted comments: %v", f.deletedComments)
	}
	noWriteTag(t, f)
}

func TestTransitionDryRunSingleKey(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	// The fixture offers 31 Close → done; Preview must resolve it without
	// the transition POST.
	out, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("transition --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	if body["key"] != "NMB-1" {
		t.Fatalf("plan key: %v", body)
	}
	req, _ := body["request"].(map[string]any)
	if req["transition_id"] != "31" {
		t.Fatalf("plan transition: %v", req)
	}
	noWriteTag(t, f)
}

func TestCloseDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdClose([]string{"NMB-1", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("close --dry-run: %v", err)
	}
	decodeDryRun(t, out)
	noWriteTag(t, f)
}

func TestAssignDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdAssign([]string{"NMB-1", "Marco Reyes", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("assign --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	req, _ := body["request"].(map[string]any)
	if req["assignee"] != "acc-mr" {
		t.Fatalf("plan assignee: %v", req)
	}
	noWriteTag(t, f)
}

func TestClaimDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdClaim([]string{"NMB-1", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("claim --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	if body["key"] != "NMB-1" {
		t.Fatalf("plan key: %v", body)
	}
	req, _ := body["request"].(map[string]any)
	if req["assignee"] != "acc-me" {
		t.Fatalf("plan claim: %v", req)
	}
	noWriteTag(t, f)
}

func TestLinkDryRun(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdLink([]string{"NMB-1", "NMB-2", "--type", "Blocks", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("link --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	req, _ := body["request"].(map[string]any)
	if req["type_id"] != "10000" {
		t.Fatalf("plan link: %v", req)
	}
	noWriteTag(t, f)
}

func TestUnlinkDryRun(t *testing.T) {
	f := newFakeJira(t)
	f.issueStatusJSON = unlinkLinksJSON
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdUnlink([]string{"NMB-1", "NMB-2", "--type", "blocks", "--dry-run"})
	})
	if err != nil {
		t.Fatalf("unlink --dry-run: %v", err)
	}
	body := decodeDryRun(t, out)
	req, _ := body["request"].(map[string]any)
	if req["link_id"] != "10500" {
		t.Fatalf("plan unlink: %v", req)
	}
	if len(f.deletedLinks) != 0 {
		t.Fatalf("dry run deleted links: %v", f.deletedLinks)
	}
	noWriteTag(t, f)
}
