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
	mirrorAttachment(t, db, key, externalID, "probe.txt", "text/plain", 12)

	rec := get(t, h, apiBase+key+"/attachments/"+externalID+"/content/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("built-in attachment → %d %s (502 is the site-string target; 409 is the credential gate reading a site that is not the origin)",
			rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := rec.Body.String(); got != "BUILTINBYTES" {
		t.Fatalf("body = %q, want the uploaded bytes", got)
	}
}

// TestBuiltInAttachmentSeeksWithRange is GDK-1617's app-side half. The
// origin answers Range now, but the proxy in front of it did not forward
// the header and mapped anything but 200 to an error — so a <video> on a
// built-in workspace could play from the start and never seek, on the path
// that does not go through the byte cache (a file too large for it, or no
// cache at all).
func TestBuiltInAttachmentSeeksWithRange(t *testing.T) {
	h, cfg, db := builtInServerDB(t)
	ctx := context.Background()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "range probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const body = "0123456789abcdef"
	atts, err := c.Upload(ctx, key, "clip.bin", strings.NewReader(body))
	if err != nil || len(atts) == 0 {
		t.Fatalf("upload: %v", err)
	}
	externalID := atts[0].ID
	mirrorAttachment(t, db, key, externalID, "clip.bin", "application/octet-stream", int64(len(body)))

	rec := get(t, h, apiBase+key+"/attachments/"+externalID+"/content/",
		map[string]string{"Range": "bytes=4-8"})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("Range request → %d, want 206: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := rec.Body.String(); got != "45678" {
		t.Fatalf("body = %q, want the requested slice %q", got, "45678")
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 4-8/16" {
		t.Errorf("Content-Range = %q, want bytes 4-8/16", cr)
	}
}

// mirrorAttachment puts the one row the attachment route needs for its
// membership check: a synced workspace already has it.
func mirrorAttachment(t *testing.T, db *store.DB, key, externalID, filename, mime string, size int64) {
	t.Helper()
	ctx := context.Background()
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
				Filename: filename, MimeType: mime, Size: size,
				CreatedAt: "2026-09-01T00:00:00.000Z",
			}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// TestBuiltInAttachmentRevalidatesWithoutA502 is GDK-1617 review finding 3.
// Forwarding If-None-Match upstream is right — the origin stores bytes
// under their content hash, so its ETag is exact — but mapAttachmentStatus
// accepted only 200 and 206, and a 304 became "upstream status 304" and
// then a 502. This lands on the second view of anything the byte cache
// will not keep, which is precisely the large videos this round is about.
func TestBuiltInAttachmentRevalidatesWithoutA502(t *testing.T) {
	h, cfg, db := builtInServerDB(t)
	ctx := context.Background()
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key, err := c.CreateIssue(ctx, map[string]any{
		"project":   map[string]any{"key": origin.DefaultProjectKey},
		"summary":   "revalidation probe",
		"issuetype": map[string]any{"name": "Task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const body = "bytes a browser will ask about twice"
	atts, err := c.Upload(ctx, key, "clip.bin", strings.NewReader(body))
	if err != nil || len(atts) == 0 {
		t.Fatalf("upload: %v", err)
	}
	id := atts[0].ID
	mirrorAttachment(t, db, key, id, "clip.bin", "application/octet-stream", int64(len(body)))

	path := apiBase + key + "/attachments/" + id + "/content/"
	first := get(t, h, path, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first view → %d", first.Code)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the streaming path, so a browser can never revalidate")
	}

	again := get(t, h, path, map[string]string{"If-None-Match": etag})
	if again.Code == http.StatusBadGateway {
		t.Fatalf("a revalidating browser got a 502: %s", strings.TrimSpace(again.Body.String()))
	}
	if again.Code != http.StatusNotModified {
		t.Fatalf("revalidation → %d, want 304", again.Code)
	}
	if again.Body.Len() != 0 {
		t.Errorf("304 carries a %d-byte body", again.Body.Len())
	}
}
