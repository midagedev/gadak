package sync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// standaloneWikiCfg is a standalone workspace under GADAK_HOME, isolated
// from the user's profile. Pattern follows internal/origin/wiki_test.go
// standaloneHome (that helper is not exported; this package has no prior
// standalone origin helper).
func standaloneWikiCfg(t *testing.T, withConfluence bool) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	if withConfluence {
		cfg.Confluence = origin.DefaultConfluenceConfig()
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func wikiADF(text string) string {
	b, err := json.Marshal(map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": text},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestStandaloneWikiPagesLandInMirror is the GDK-267 evidence: a page created
// on the in-process origin is visible in the SQLite mirror after RunConfluence.
// No network — GADAK_HOME is a temp dir.
func TestStandaloneWikiPagesLandInMirror(t *testing.T) {
	cfg := standaloneWikiCfg(t, true)
	w, err := origin.Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	created, err := w.CreatePage(context.Background(), origin.DefaultSpaceKey, "Standalone wiki note", wikiADF("hello from standalone wiki"), "")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreatePage returned empty id")
	}

	db := newMirror(t)
	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{Full: true})
	if err != nil {
		t.Fatalf("RunConfluence: %v", err)
	}
	if res.Fetched < 1 {
		t.Fatalf("fetched = %d, want >= 1", res.Fetched)
	}

	d, err := db.PageDetail(context.Background(), created.ID)
	if err != nil || d == nil {
		t.Fatalf("PageDetail(%s): %v %#v", created.ID, err, d)
	}
	if d.Title != "Standalone wiki note" {
		t.Errorf("title %q", d.Title)
	}
	if d.SpaceKey != origin.DefaultSpaceKey {
		t.Errorf("space %q, want %s", d.SpaceKey, origin.DefaultSpaceKey)
	}

	n, err := db.TableCount(context.Background(), "pages")
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("pages count = %d", n)
	}
}

// TestStandaloneWikiPassOffWhenConfluenceNil is the existing-workspace case:
// cfg.Confluence == nil keeps RunConfluence from mirroring, even though
// origin.Wiki still returns a client. No config migration this round.
func TestStandaloneWikiPassOffWhenConfluenceNil(t *testing.T) {
	cfg := standaloneWikiCfg(t, false)
	if cfg.Confluence != nil {
		t.Fatal("precondition: Confluence must be nil")
	}
	w, err := origin.Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreatePage(context.Background(), origin.DefaultSpaceKey, "Unmirrored", wikiADF("stays in origin"), ""); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	db := newMirror(t)
	_, err = RunConfluence(context.Background(), cfg, db.DB, Options{Full: true})
	if err == nil || !strings.Contains(err.Error(), "confluence is not configured") {
		t.Fatalf("RunConfluence err = %v, want confluence is not configured", err)
	}
	n, err := db.TableCount(context.Background(), "pages")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("pages = %d, want 0 when the pass is off", n)
	}
}

// TestWikiAcquisitionFailureSkipsPass: origin.Wiki failing must not become
// a hard RunConfluence error (degrade-don't-break). Issue sync is a different
// function; this asserts the wiki pass itself returns success-and-skip.
func TestWikiAcquisitionFailureSkipsPass(t *testing.T) {
	home := t.TempDir()
	blocked := filepath.Join(home, "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GADAK_HOME", blocked)
	t.Setenv("HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	cfg := &config.Config{
		Kind:       config.KindStandalone,
		Confluence: origin.DefaultConfluenceConfig(),
	}
	var logs []string
	db := newMirror(t)
	res, err := RunConfluence(context.Background(), cfg, db.DB, Options{
		Full: true,
		Log:  func(s string) { logs = append(logs, s) },
	})
	if err != nil {
		t.Fatalf("Wiki failure must skip, not fail RunConfluence: %v", err)
	}
	if res.Fetched != 0 {
		t.Fatalf("fetched = %d after skip", res.Fetched)
	}
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "skip wiki pass") {
		t.Fatalf("missing skip log, got %q", joined)
	}
}
