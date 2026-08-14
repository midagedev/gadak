package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
)

func writeShotManifest(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), []byte("PNG"), 0o600); err != nil {
		t.Fatal(err)
	}
	man := `{"attachments":[{"id":"` + id + `","file":"shot.png","filename":"shot.png","content_type":"image/png"}]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(man), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestImportAttachmentsIntoUsesReaderKey(t *testing.T) {
	cfg := mirror(t, "https://nimbus.example.com")
	home, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	man := t.TempDir()
	writeShotManifest(t, man, "10021")
	cacheDir := filepath.Join(t.TempDir(), "attachments")
	dbPath := filepath.Join(home, "gadak.db")

	stats, err := importAttachmentsInto(man, cacheDir, cfg.Site, config.Profile(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Seeded != 1 || len(stats.SkippedIDs) != 0 {
		t.Fatalf("stats %+v", stats)
	}

	cache, err := attachcache.New(cacheDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := attachcache.Key(cfg.Site, config.Profile(), "NMB-1", "10021")
	if !cache.Has(want) {
		t.Fatal("import did not write the key the reader uses")
	}
	if cache.Has("10021") {
		t.Fatal("import also wrote the raw id")
	}
}

func TestImportAttachmentsIntoSkipsUnknownID(t *testing.T) {
	cfg := mirror(t, "https://nimbus.example.com")
	home, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	man := t.TempDir()
	writeShotManifest(t, man, "99999")
	stats, err := importAttachmentsInto(man, filepath.Join(t.TempDir(), "attachments"),
		cfg.Site, config.Profile(), filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Seeded != 0 || len(stats.SkippedIDs) != 1 || stats.SkippedIDs[0] != "99999" {
		t.Fatalf("stats %+v", stats)
	}
}

func TestAttachmentIssueKeysMapsExternalAndStoreID(t *testing.T) {
	_ = mirror(t, "https://nimbus.example.com")
	home, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := attachmentIssueKeys(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if got["10021"] != "NMB-1" {
		t.Fatalf("external_id 10021 → %q", got["10021"])
	}
	if got["jira:a-1"] != "NMB-1" {
		t.Fatalf("store id jira:a-1 → %q", got["jira:a-1"])
	}
}

func TestImportAttachmentsIntoEmptySiteKeepsLegacyKey(t *testing.T) {
	_ = mirror(t, "")
	home, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	man := t.TempDir()
	writeShotManifest(t, man, "10021")
	cacheDir := filepath.Join(t.TempDir(), "attachments")
	stats, err := importAttachmentsInto(man, cacheDir, "", "", filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Seeded != 1 {
		t.Fatalf("stats %+v", stats)
	}
	cache, err := attachcache.New(cacheDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !cache.Has("10021") {
		t.Fatal("empty site must keep the legacy id-only key")
	}
}
