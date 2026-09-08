package server

// GDK-1613: the app could not show a single attachment on a built-in
// origin. fetchAttachment built its target by concatenating cfg.Site, and a
// built-in workspace has no site — in-process because the origin is this
// process, paired because the endpoint lives in remote-origin.json. The
// concatenation produced a relative "/rest/api/3/attachment/content/N",
// which http.Client refuses ("unsupported protocol scheme \"\""), so every
// view answered 502. Measured on both transports before the fix.
//
// The seam that answers on every origin already existed: origin.Client.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

func TestBuiltInAttachmentIsServedNotA502(t *testing.T) {
	h, cfg, db := builtInServerDB(t)
	if cfg.Site != "" {
		t.Fatalf("premise: a built-in workspace has no site, got %q", cfg.Site)
	}
	ctx := context.Background()

	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatalf("origin client: %v", err)
	}
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "attachment probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatalf("create on the built-in origin: %v", err)
	}
	atts, err := c.Upload(ctx, key, "probe.txt", strings.NewReader("BUILTINBYTES"))
	if err != nil || len(atts) == 0 {
		t.Fatalf("upload: %v (%d attachments)", err, len(atts))
	}
	externalID := atts[0].ID

	// The route reads the mirror for membership, so the row has to be there
	// — which is exactly the state a synced workspace is in.
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "standalone-jira:10001", SourceID: "jira", Kind: "issue",
				ExternalID: "10001", Key: key, Title: "attachment probe",
				CreatedAt: "2026-09-01T00:00:00.000Z", UpdatedAt: "2026-09-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: origin.DefaultProjectKey, IssueType: "Task",
				IssueTypeID: "10001",
				Status:      "To Do", StatusID: "1", StatusCategory: "new",
			},
			Attachments: []store.Attachment{{
				ID: "standalone-jira:" + externalID, ExternalID: externalID,
				Filename: "probe.txt", MimeType: "text/plain", Size: 12,
				CreatedAt: "2026-09-01T00:00:00.000Z",
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	rec := get(t, h, apiBase+key+"/attachments/"+externalID+"/content/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("built-in attachment → %d %s (502 is the site-string target; 409 is the credential gate reading a site that is not the origin)",
			rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := rec.Body.String(); got != "BUILTINBYTES" {
		t.Fatalf("body = %q, want the uploaded bytes", got)
	}
}
