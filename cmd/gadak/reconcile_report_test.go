package main

// GDK-1400 — the two surfaces that answer "does this mirror hold what its
// origin holds?". The field case had neither: the short host reported a
// healthy sync on every tick, and learning the origin's real count meant
// pairing a second machine to the same origin.
//
// Its own file rather than status_test.go / doctor_test.go because both of
// those are being edited by a concurrent round; the contract is the same
// either way.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// TestFormatReconcileLineOmitsWhatNeverRan: a tally of zeroes reads as
// "checked, all level", which is the opposite of "never checked". The line
// must be absent until a reconcile has actually recorded one.
func TestFormatReconcileLineOmitsWhatNeverRan(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if got := formatReconcileLine(store.ReconcileStats{}, now); got != "" {
		t.Errorf("line for a mirror that never reconciled = %q, want empty", got)
	}
	if got := formatReconcileLine(store.ReconcileStats{UpstreamKeys: 0}, now); got != "" {
		t.Errorf("a zero tally with no timestamp still means never: %q", got)
	}

	got := formatReconcileLine(store.ReconcileStats{
		At:             "2026-09-07T10:00:00Z",
		UpstreamKeys:   1399,
		MissingFetched: 1265,
	}, now)
	for _, want := range []string{"1,399 upstream", "1,265 fetched missing", "0 deleted", "2h ago"} {
		if !strings.Contains(got, want) {
			t.Errorf("reconcile line %q is missing %q", got, want)
		}
	}
}

// TestDoctorNamesAShortMirror: the mirror holds fewer issues in scope than the
// origin reported at the last reconcile. Counts only — no keys enter a
// document the doctor banner promises is safe to paste.
func TestDoctorNamesAShortMirror(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	ctx := context.Background()
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"3": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "GDK-1", Title: "the one row this mirror got",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "GDK", IssueType: "Bug", IssueTypeID: "1",
				Status: "To Do", StatusID: "3", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// A watermark is what makes this the issue source status reads.
	if err := db.RecordSync(ctx, "jira", store.SyncResult{Watermark: "2026-09-03T11:17:35.422Z"}); err != nil {
		t.Fatalf("record sync: %v", err)
	}
	if err := db.RecordReconcile(ctx, "jira", store.ReconcileStats{
		At: "2026-09-07T10:00:00Z", UpstreamKeys: 1399,
	}); err != nil {
		t.Fatalf("record reconcile: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Site: "https://example.atlassian.net", Email: "someone@example.com", Token: "token",
		Projects: []string{"GDK"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "mirror_short") {
		t.Fatalf("doctor does not name a mirror holding 1 of its origin's 1,399 issues:\n%s", out)
	}
	if !strings.Contains(out, "mirror=1") || !strings.Contains(out, "upstream=1399") {
		t.Errorf("mirror_short line does not carry both counts:\n%s", out)
	}
	if strings.Contains(out, "GDK-1 ") {
		t.Errorf("doctor leaked an issue key into the paste-safe report:\n%s", out)
	}
}

// TestDoctorSaysNothingWhenTheMirrorIsLevel: the check must be silent on a
// healthy mirror, or it is noise that gets trained away.
func TestDoctorSaysNothingWhenTheMirrorIsLevel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	ctx := context.Background()
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"3": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "GDK-1", Title: "level",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "GDK", IssueType: "Bug", IssueTypeID: "1",
				Status: "To Do", StatusID: "3", StatusCategory: "new",
			},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.RecordSync(ctx, "jira", store.SyncResult{Watermark: "2026-09-03T11:17:35.422Z"}); err != nil {
		t.Fatalf("record sync: %v", err)
	}
	if err := db.RecordReconcile(ctx, "jira", store.ReconcileStats{At: "2026-09-07T10:00:00Z", UpstreamKeys: 1}); err != nil {
		t.Fatalf("record reconcile: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Site: "https://example.atlassian.net", Email: "someone@example.com", Token: "token",
		Projects: []string{"GDK"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "mirror_short") {
		t.Fatalf("a level mirror must produce no finding:\n%s", out)
	}
}

// TestCountIssuesInScopeHonoursTheProjectScope is the mirror half of the
// divergence probe: it must count what the origin's reconcile JQL counts, or
// the comparison escalates forever on a scoped workspace.
func TestCountIssuesInScopeHonoursTheProjectScope(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if err := db.UpsertSource(ctx, store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	recs := []struct{ id, key, project, source string }{
		{"jira:1", "GDK-1", "GDK", "jira"},
		{"jira:2", "GDK-2", "GDK", "jira"},
		{"jira:3", "LOC-1", "LOC", "jira"},
		{"linear:1", "FIX-1", "FIX", "linear"},
	}
	for _, r := range recs {
		if _, err := db.UpsertIssues(ctx, store.Batch{
			Categories: map[string]string{"3": "new"},
			Records: []store.IssueRecord{{
				Item: store.Item{
					ID: r.id, SourceID: r.source, Kind: "issue", ExternalID: r.id,
					Key: r.key, Title: r.key,
					CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: r.project, IssueType: "Bug", IssueTypeID: "1",
					Status: "To Do", StatusID: "3", StatusCategory: "new",
				},
			}},
		}); err != nil {
			t.Fatalf("seed %s: %v", r.key, err)
		}
	}
	for _, tc := range []struct {
		name     string
		source   string
		projects []string
		want     int
	}{
		{"scoped to one project", "jira", []string{"GDK"}, 2},
		{"scoped to two", "jira", []string{"GDK", "LOC"}, 3},
		{"case does not matter", "jira", []string{"gdk"}, 2},
		{"blank entries are not a scope", "jira", []string{"GDK", "  "}, 2},
		{"unscoped is the whole source", "jira", nil, 3},
		{"the other source is not counted", "linear", nil, 1},
	} {
		got, err := db.CountIssuesInScope(ctx, tc.source, tc.projects)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: count = %d, want %d", tc.name, got, tc.want)
		}
	}
}
