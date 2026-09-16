package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestMigrateAttachmentBytesFromCache is GDK-1960's gate: a source whose
// origin cannot be reached still migrates attachment bytes when the source
// workspace's own cache holds them. The measured shape was the cutover
// walk-through — freeze the source (or have its credential revoked), run
// `migrate`, and be refused with "site, email and token are required" even
// though every attachment the user had opened was on this disk. The refusal
// was right (GDK-1275); the premise — bytes live only at the origin — was
// not.
//
// The source is the mirror() fixture (NMB-1 with attachment 10021) with its
// credential scrubbed, and its cache is seeded exactly the way a snapshot
// import seeds one (importAttachmentsInto, the `gadak demo` path), so the
// key the migration reads is the key that path wrote.
//
// FAIL-first: on the pre-change tree the run dies on the refusal before any
// byte moves.
func TestMigrateAttachmentBytesFromCache(t *testing.T) {
	cfg := mirror(t, "https://nimbus.example.com")
	t.Setenv("HOME", cfg.Directory())
	clearCredentialEnv(t)
	// The revoked-credential shape: mirror and cached bytes on disk, no way
	// to reach the origin.
	cfg.Email, cfg.Token = "", ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	man := t.TempDir()
	body := []byte("har trace bytes that live only in the cache\n")
	if err := os.WriteFile(filepath.Join(man, "trace.har"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"attachments":[{"id":"10021","file":"trace.har","filename":"trace.har","content_type":"application/json"}]}`
	if err := os.WriteFile(filepath.Join(man, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	home, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := config.AttachmentDirFor("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importAttachmentsInto(man, cacheDir, cfg.Site, "", filepath.Join(home, "gadak.db")); err != nil {
		t.Fatal(err)
	}

	allowProfileCreate = true
	t.Cleanup(func() {
		allowProfileCreate = false
		_ = origin.Close()
		config.SetProfile("")
	})
	config.SetProfile("dst")
	report, err := capture(t, func() error { return cmdMigrate([]string{"--from", "default"}) })
	if err != nil {
		t.Fatalf("migrate with the cache as the only byte source: %v\n%s", err, report)
	}
	if strings.Contains(report, "MISMATCH") {
		t.Fatalf("verification mismatch:\n%s", report)
	}
	// The count line must say where each byte came from — that is the
	// question the line exists to answer (GDK-1960 ③).
	if !strings.Contains(report, "origin 0") || !strings.Contains(report, "local cache 1") {
		t.Fatalf("report does not name the byte source:\n%s", report)
	}

	// The bytes themselves, served by the new workspace: same id, same
	// content, no origin involved anywhere in between.
	id := strings.TrimSpace(dstSQL(t,
		"select coalesce(nullif(external_id,''), id) from attachments a join issues_full i on i.item_id = a.item_id where a.filename = 'trace.har'"))
	if id == "" {
		t.Fatalf("attachment missing from target mirror:\n%s", report)
	}
	dstCfg, err := config.LoadFor("dst")
	if err != nil {
		t.Fatal(err)
	}
	client, err := origin.Client(dstCfg)
	if err != nil {
		t.Fatalf("target origin: %v", err)
	}
	status, got, err := client.Raw(context.Background(), "GET",
		"/rest/api/3/attachment/content/"+url.PathEscape(id), nil, false)
	if err != nil || status != 200 {
		t.Fatalf("attachment content: status=%d err=%v", status, err)
	}
	if string(got) != string(body) {
		t.Fatalf("attachment bytes differ: got %q want %q", got, body)
	}
}
