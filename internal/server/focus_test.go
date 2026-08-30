package server

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/uifocus"
)

func TestUIFocusPeekReturnsHashAndAtTwice(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := uifocus.Write("pj=NMA&sc=inprogress"); err != nil {
		t.Fatal(err)
	}
	h := New(nil, nil)
	type body struct {
		Hash string `json:"hash"`
		At   string `json:"at"`
	}
	first := decode[body](t, get(t, h, apiBase+"ui-focus/", nil))
	if first.Hash != "pj=NMA&sc=inprogress" {
		t.Fatalf("first hash %q", first.Hash)
	}
	if first.At == "" {
		t.Fatal("first poll must carry at")
	}
	rec := get(t, h, apiBase+"ui-focus/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("second poll %d", rec.Code)
	}
	second := decode[body](t, rec)
	if second.Hash != first.Hash {
		t.Fatalf("second hash %q, want %q", second.Hash, first.Hash)
	}
	if second.At != first.At {
		t.Fatalf("second at %q, want %q", second.At, first.At)
	}
}

// The handler logs a payload once per process so the 500ms poll does not
// spam the log. Two workspace mounts each holding a fresh payload poll in
// turn, so a single remembered slot would evict on every alternation and log
// every time — the spam the once-per-payload rule exists to prevent.
//
// FAIL-first: with lastFocusLog as one (profile, at) struct, this counts 4.
func TestUIFocusLogsOncePerProfileNotPerPoll(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	for _, p := range []string{"work", "oss"} {
		if err := uifocus.WriteFor(p, "ks=AAA-1"); err != nil {
			t.Fatal(err)
		}
	}
	// The remembered map is process-global; another test in this package may
	// have already logged one of these profiles.
	focusLogMu.Lock()
	lastFocusLog = map[string]string{}
	focusLogMu.Unlock()

	var buf bytes.Buffer
	flags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(flags) })

	mounts := map[string]http.Handler{
		"work": NewWorkspace(nil, nil, nil, "work"),
		"oss":  NewWorkspace(nil, nil, nil, "oss"),
	}
	for range 2 {
		for _, p := range []string{"work", "oss"} {
			get(t, mounts[p], apiBase+"ui-focus/", nil)
		}
	}
	if n := strings.Count(buf.String(), "ui-focus: profile="); n != 2 {
		t.Fatalf("logged %d lines over 4 polls of 2 profiles, want 2:\n%s", n, buf.String())
	}
}

func TestWorkspaceFocusTakesProfileFile(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := uifocus.WriteFor("work", "ks=BBB-1"); err != nil {
		t.Fatal(err)
	}
	if err := uifocus.WriteFor("", "ks=AAA-1"); err != nil {
		t.Fatal(err)
	}
	h := NewWorkspace(nil, nil, nil, "work")
	got := decode[struct {
		Hash string `json:"hash"`
	}](t, get(t, h, apiBase+"ui-focus/", nil))
	if got.Hash != "ks=BBB-1" {
		t.Fatalf("workspace took %q, want ks=BBB-1", got.Hash)
	}
	hash, _, ok, err := uifocus.PeekFor("")
	if err != nil || !ok || hash != "ks=AAA-1" {
		t.Fatalf("default file should be untouched: %q ok=%v err=%v", hash, ok, err)
	}
}

// GDK-1170: the poll carries the mirror's identity, and it moves exactly when
// the mirror does. This is the whole signal — the board pulls a delta because
// this value changed, so a value that sits still through a write is the defect
// (and under WAL, keying on gadak.db alone is precisely that value).
//
// FAIL-first: before the handler carried the field, both polls decoded
// mirrorVersion as "" and this stopped on "poll must carry mirrorVersion".
func TestUIFocusCarriesMirrorVersionThatMovesWithTheMirror(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	db, cfg := fixture(t)
	h := New(db, cfg)
	type body struct {
		MirrorVersion string `json:"mirrorVersion"`
		ConfigVersion string `json:"configVersion"`
	}

	first := decode[body](t, get(t, h, apiBase+"ui-focus/", nil))
	if first.MirrorVersion == "" {
		t.Fatal("poll must carry mirrorVersion")
	}
	// An untouched mirror polled twice is the same value, or every 500ms tick
	// would pull a delta.
	if same := decode[body](t, get(t, h, apiBase+"ui-focus/", nil)); same.MirrorVersion != first.MirrorVersion {
		t.Fatalf("mirrorVersion moved with no write: %q -> %q", first.MirrorVersion, same.MirrorVersion)
	}

	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:9001", SourceID: "jira", ExternalID: "9001", Key: "NMB-9001",
				Title:     "written after the first poll",
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-02T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "1",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	after := decode[body](t, get(t, h, apiBase+"ui-focus/", nil))
	if after.MirrorVersion == first.MirrorVersion {
		t.Fatalf("mirrorVersion did not move after a mirror write: %q", after.MirrorVersion)
	}
}

// The mirror half must never take the focus half down with it. A handler with
// no mirror to stat still answers 200 with the payload the CLI left.
func TestUIFocusWithoutAMirrorStillServesFocus(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := uifocus.Write("pj=NMA"); err != nil {
		t.Fatal(err)
	}
	h := New(nil, nil)
	rec := get(t, h, apiBase+"ui-focus/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("poll without a mirror answered %d", rec.Code)
	}
	got := decode[struct {
		Hash          string `json:"hash"`
		MirrorVersion string `json:"mirrorVersion"`
	}](t, rec)
	if got.Hash != "pj=NMA" {
		t.Fatalf("focus payload lost: %q", got.Hash)
	}
	if got.MirrorVersion != "" {
		t.Fatalf("no mirror must report no version, got %q", got.MirrorVersion)
	}
}

func TestPrimaryFocusTakesProcessProfileFile(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := uifocus.Write("pj=NMA"); err != nil {
		t.Fatal(err)
	}
	h := New(nil, nil)
	got := decode[struct {
		Hash string `json:"hash"`
	}](t, get(t, h, apiBase+"ui-focus/", nil))
	if got.Hash != "pj=NMA" {
		t.Fatalf("primary took %q", got.Hash)
	}
}
