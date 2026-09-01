package originbind

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

func localOriginHome(t *testing.T) *config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindLocalOrigin
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func seedMirrorPages(t *testing.T, n int) {
	t.Helper()
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "confluence", Kind: "confluence"}); err != nil {
		t.Fatal(err)
	}
	recs := make([]store.PageRecord, n)
	for i := 0; i < n; i++ {
		id := "1000" + string(rune('1'+i))
		recs[i] = store.PageRecord{
			Item: store.Item{
				ID: "confluence:" + id, SourceID: "confluence", Kind: "page",
				ExternalID: id, Key: id, Title: "page " + id,
				CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Page: store.Page{
				SpaceKey: "LOC", Version: 1, Status: "current",
				BodyADF: json.RawMessage(`{"type":"doc","version":1,"content":[]}`),
			},
		}
	}
	if _, err := db.UpsertPages(ctx, recs); err != nil {
		t.Fatal(err)
	}
}

// TestLocalDataCountsPagesWhenNoIssues is GDK-417: a wiki-only local-origin
// workspace is still local data. Docstring previously counted issues only.
func TestLocalDataCountsPagesWhenNoIssues(t *testing.T) {
	cfg := localOriginHome(t)
	seedMirrorPages(t, 2)
	n, persist, err := LocalData(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if persist == "" {
		t.Fatal("persist path empty")
	}
	if n < 2 {
		t.Fatalf("LocalData = %d, want pages to count (2)", n)
	}
}

// TestRefuseReplaceBlocksPagesOnlyWorkspace is GDK-417's convert gate:
// pages without issues must still refuse a silent connected init.
func TestRefuseReplaceBlocksPagesOnlyWorkspace(t *testing.T) {
	cfg := localOriginHome(t)
	seedMirrorPages(t, 2)
	err := RefuseReplace(cfg, false)
	if err == nil {
		t.Fatal("pages-only local-origin must refuse a connected init")
	}
	var refused *ReplaceRefusedError
	if !asReplaceRefused(err, &refused) {
		t.Fatalf("want ReplaceRefusedError, got %T %v", err, err)
	}
	if refused.Issues < 2 {
		t.Fatalf("count = %d, want >= 2 pages", refused.Issues)
	}
	if !strings.Contains(refused.Error(), "page") {
		t.Fatalf("refusal must name pages, got: %s", refused.Error())
	}
}

func asReplaceRefused(err error, target **ReplaceRefusedError) bool {
	if err == nil {
		return false
	}
	e, ok := err.(*ReplaceRefusedError)
	if !ok {
		return false
	}
	*target = e
	return true
}

func TestRefuseReplaceAllowsEmpty(t *testing.T) {
	cfg := localOriginHome(t)
	if err := RefuseReplace(cfg, false); err != nil {
		t.Fatalf("empty local-origin: %v", err)
	}
}

func TestRefuseReplaceOptIn(t *testing.T) {
	cfg := localOriginHome(t)
	seedMirrorPages(t, 1)
	if err := RefuseReplace(cfg, true); err != nil {
		t.Fatalf("opt-in: %v", err)
	}
}

func TestLocalDataCountsLegacyYAMLOrigin(t *testing.T) {
	cfg := localOriginHome(t)
	home := cfg.Directory()
	yamlPath := origin.LegacyYAMLPath(home)
	if err := os.MkdirAll(filepath.Dir(yamlPath), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`projects:
  - id: "10000"
    key: STD
    name: Local-origin
    type: software
    style: classic
issues:
  - key: STD-1
    summary: from legacy yaml
`)
	if err := os.WriteFile(yamlPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	n, persist, err := LocalData(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if persist != origin.PersistPath(home) {
		t.Fatalf("persist = %q, want %s", persist, origin.PersistPath(home))
	}
	if n < 1 {
		t.Fatalf("LocalData = %d, want yaml-origin issue counted", n)
	}
	if err := RefuseReplace(cfg, false); err == nil {
		t.Fatal("yaml-only local-origin with issues must refuse a connected init")
	}
}

func TestLocalDataMissingMirrorIsZero(t *testing.T) {
	cfg := localOriginHome(t)
	n, _, err := LocalData(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("empty = %d", n)
	}
	if _, err := os.Stat(origin.PersistPath(cfg.Directory())); !os.IsNotExist(err) && err != nil {
		t.Fatal(err)
	}
}
