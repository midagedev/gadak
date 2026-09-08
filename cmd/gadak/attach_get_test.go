package main

// GDK-1610: reading an attachment had no verb. `gadak issue KEY` lists them;
// getting the bytes meant parsing issues.raw for a numeric id and assembling
// a REST path for `gadak api`.
//
// Contract ↔ assertion:
//
//  1. bytes land at the default destination (the filename, here)
//     TestAttachGetWritesTheFileHere
//  2. --out takes a directory (keeps the filename) or a file path
//     TestAttachGetOutDirectoryAndFile
//  3. an existing file is refused; --force overwrites
//     TestAttachGetRefusesToOverwrite
//  4. resolution by origin id as well as filename
//     TestAttachGetResolvesByID
//  5. an attachment of another issue is not readable under this key
//     TestAttachGetDoesNotCrossIssues
//  6. a name the issue does not have lists what it does have
//     TestAttachGetUnknownNameListsWhatThereIs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachGetWritesTheFileHere(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.attachmentBytes["10021"] = "PK\x03\x04ok"

	dir := t.TempDir()
	t.Chdir(dir)

	out, err := capture(t, func() error { return cmdAttach([]string{"get", "NMB-1", "trace.har"}) })
	if err != nil {
		t.Fatalf("attach get: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "trace.har"))
	if err != nil {
		t.Fatalf("the default destination is the filename, here: %v", err)
	}
	if string(got) != "PK\x03\x04ok" {
		t.Errorf("bytes = %q", got)
	}
	if !strings.Contains(out, "trace.har") || !strings.Contains(out, "bytes") {
		t.Errorf("the line must say where it went and how much: %q", out)
	}
}

func TestAttachGetOutDirectoryAndFile(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.attachmentBytes["10021"] = "BYTES"

	into := t.TempDir()
	if _, err := capture(t, func() error {
		return cmdAttach([]string{"get", "NMB-1", "trace.har", "--out", into})
	}); err != nil {
		t.Fatalf("--out DIR: %v", err)
	}
	if _, err := os.Stat(filepath.Join(into, "trace.har")); err != nil {
		t.Errorf("a directory keeps the filename inside it: %v", err)
	}

	named := filepath.Join(t.TempDir(), "renamed.bin")
	if _, err := capture(t, func() error {
		return cmdAttach([]string{"get", "NMB-1", "trace.har", "--out", named})
	}); err != nil {
		t.Fatalf("--out FILE: %v", err)
	}
	if got, err := os.ReadFile(named); err != nil || string(got) != "BYTES" {
		t.Errorf("--out FILE = %q, %v", got, err)
	}
}

func TestAttachGetRefusesToOverwrite(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.attachmentBytes["10021"] = "NEW"

	dir := t.TempDir()
	dest := filepath.Join(dir, "trace.har")
	if err := os.WriteFile(dest, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdAttach([]string{"get", "NMB-1", "trace.har", "--out", dest})
	})
	if err == nil {
		t.Fatal("an existing file must not be replaced silently")
	}
	if got, _ := os.ReadFile(dest); string(got) != "OLD" {
		t.Fatalf("the refusal still wrote: %q", got)
	}
	if _, err := capture(t, func() error {
		return cmdAttach([]string{"get", "NMB-1", "trace.har", "--out", dest, "--force"})
	}); err != nil {
		t.Fatalf("--force: %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "NEW" {
		t.Errorf("--force did not overwrite: %q", got)
	}
}

func TestAttachGetResolvesByID(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.attachmentBytes["10021"] = "BYID"

	dest := filepath.Join(t.TempDir(), "out.bin")
	if _, err := capture(t, func() error {
		return cmdAttach([]string{"get", "NMB-1", "10021", "--out", dest})
	}); err != nil {
		t.Fatalf("by origin id: %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "BYID" {
		t.Errorf("bytes = %q", got)
	}
}

// Membership is the query, not a check bolted beside it: an id that belongs
// to another issue is simply not one of this issue's attachments.
func TestAttachGetDoesNotCrossIssues(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.attachmentBytes["70000"] = "SOMEONE ELSE'S"

	_, err := capture(t, func() error {
		return cmdAttach([]string{"get", "NMB-1", "70000", "--out", filepath.Join(t.TempDir(), "x")})
	})
	if err == nil {
		t.Fatal("an id this issue does not carry must not read bytes")
	}
}

func TestAttachGetUnknownNameListsWhatThereIs(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error { return cmdAttach([]string{"get", "NMB-1", "nope.zip"}) })
	if err == nil {
		t.Fatal("an unknown name must fail")
	}
	if !strings.Contains(err.Error(), "trace.har") {
		t.Errorf("the error must name what the issue does have: %v", err)
	}
}
