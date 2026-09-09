package main

// GDK-1615: attachments uploaded under the built-in origin's old 8 MiB cap
// were cut off silently. The bytes are unrecoverable, so the product's job
// is to stop presenting the remainder as a whole file — in doctor, as a
// count for the workspace, and on the attachment line of `gadak issue KEY`.
//
// The signal is a size of exactly 8 MiB, which is a suspicion: a file can
// really be that long. Both surfaces say "may".
//
// It is only ever true of a built-in-origin workspace. A Jira or Linear
// workspace never went through that cap, so the same 8 MiB attachment there
// must produce no line at all — a truncation warning about a file Jira
// holds intact is a false statement about someone's data.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/attachaudit"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// seedAttachmentWorkspace writes a mirror holding one issue with one
// attachment of the given size, and a config of the given kind.
func seedAttachmentWorkspace(t *testing.T, kind string, size int64) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "GDK-1", Title: "clip attached",
				CreatedAt: "2026-09-01T00:00:00.000Z", UpdatedAt: "2026-09-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "GDK", IssueType: "Task", IssueTypeID: "1",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
			Attachments: []store.Attachment{{
				ID: "jira:9001", ExternalID: "9001", Filename: "clip.mp4",
				MimeType: "video/mp4", Size: size,
				CreatedAt: "2026-09-01T00:00:00.000Z",
			}},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Kind: kind}
	if kind != config.KindStandalone && kind != config.OriginGadak {
		cfg.Site = "https://example.atlassian.net"
		cfg.Email = "someone@example.com"
		cfg.Token = "token"
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorFlagsMaybeTruncatedAttachments(t *testing.T) {
	seedAttachmentWorkspace(t, config.KindStandalone, attachaudit.TruncatedSize)
	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "attachments_maybe_truncated:") {
		t.Fatalf("no attachments_maybe_truncated line on a built-in workspace holding an 8 MiB attachment:\n%s", out)
	}
	if !strings.Contains(out, "may have been cut short") {
		t.Errorf("the line states truncation as fact instead of a suspicion:\n%s", out)
	}
	if strings.Contains(out, "clip.mp4") || strings.Contains(out, "GDK-1/") {
		t.Errorf("filenames belong in the sql one-liner, not in the paste-safe document:\n%s", out)
	}
}

func TestDoctorSaysNothingWithoutTheSignal(t *testing.T) {
	seedAttachmentWorkspace(t, config.KindStandalone, attachaudit.TruncatedSize-1)
	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "attachments_maybe_truncated") {
		t.Fatalf("a 8388607-byte attachment produced a truncation line:\n%s", out)
	}
}

func TestDoctorSaysNothingOnAJiraWorkspace(t *testing.T) {
	seedAttachmentWorkspace(t, config.OriginJira, attachaudit.TruncatedSize)
	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "attachments_maybe_truncated") {
		t.Fatalf("a Jira workspace never went through the built-in cap, so this line is false there:\n%s", out)
	}
}

func TestIssueMarksMaybeTruncatedAttachment(t *testing.T) {
	seedAttachmentWorkspace(t, config.KindStandalone, attachaudit.TruncatedSize)
	out, err := capture(t, func() error { return cmdIssue([]string{"GDK-1"}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, out)
	}
	if !strings.Contains(out, attachaudit.Mark) {
		t.Fatalf("the attachment line does not say the bytes may be short:\n%s", out)
	}
}

func TestIssueLeavesAnIntactAttachmentAlone(t *testing.T) {
	seedAttachmentWorkspace(t, config.KindStandalone, attachaudit.TruncatedSize+1)
	out, err := capture(t, func() error { return cmdIssue([]string{"GDK-1"}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, out)
	}
	if strings.Contains(out, "truncated") {
		t.Fatalf("an 8388609-byte attachment was marked:\n%s", out)
	}
}

func TestIssueOnAJiraWorkspaceIsNotMarked(t *testing.T) {
	seedAttachmentWorkspace(t, config.OriginJira, attachaudit.TruncatedSize)
	out, err := capture(t, func() error { return cmdIssue([]string{"GDK-1"}) })
	if err != nil {
		t.Fatalf("issue: %v\n%s", err, out)
	}
	if strings.Contains(out, "truncated") {
		t.Fatalf("Jira never had the built-in cap, so its 8 MiB file is not suspect:\n%s", out)
	}
}

// The one-command answer to "what state are this workspace's attachment
// bytes in": mirrored rows and how many are local (GDK-1616 layer 3).
func TestDoctorReportsAttachmentState(t *testing.T) {
	seedAttachmentWorkspace(t, config.KindStandalone, 1234)
	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "attachments:") {
		t.Fatalf("no attachments line:\n%s", out)
	}
	if !strings.Contains(out, "1 mirrored, 0 cached") {
		t.Fatalf("attachments line does not count the mirror and the cache:\n%s", out)
	}
}
