package server

// GDK-1617: the settings panel's size number was the mirror's, and the
// mirror is a cache. On a built-in workspace the record is the persist
// database plus the attachment bytes beside it — and the attachments are
// usually the larger of the two by an order of magnitude, so the one number
// the panel showed had almost nothing to do with what the workspace costs
// on disk. Moving the bytes into their own directory made that worse: they
// left the only file the panel knew about.

import (
	"context"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/origin"
)

func TestRuntimeInfoReportsOriginAndAttachmentBytes(t *testing.T) {
	h, cfg, _ := builtInServerDB(t)
	ctx := context.Background()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "disk usage probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("x", 300_000)
	if _, err := c.Upload(ctx, key, "big.bin", strings.NewReader(body)); err != nil {
		t.Fatalf("upload: %v", err)
	}

	doc := decode[struct {
		Runtime runtimeInfo `json:"runtime"`
	}](t, get(t, h, apiBase+"settings/", nil))
	info := doc.Runtime
	if info.OriginPath == "" {
		t.Fatal("the panel does not name the built-in tracker's own database — only the mirror, which is a cache")
	}
	if info.AttachmentsPath == "" {
		t.Fatal("the panel does not name the attachment directory; those bytes are invisible to the user")
	}
	if int64(len(body)) > info.AttachmentsBytes {
		t.Errorf("attachment bytes reported as %d, but %d were uploaded", info.AttachmentsBytes, len(body))
	}
	if info.AttachmentsFileCount != 1 || info.AttachmentCount != 1 {
		t.Errorf("files=%d attachments=%d, want 1 and 1", info.AttachmentsFileCount, info.AttachmentCount)
	}
	if info.AttachmentsOldestAt == "" {
		t.Error("no date on the attachments — the origin's created_at is not reaching the panel")
	}
	if info.AttachmentsHuman == "" || info.OriginSizeHuman == "" {
		t.Errorf("sizes have no human form: %+v", info)
	}
}
