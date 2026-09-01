package origin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
)

func localOriginHome(t *testing.T) (*config.Config, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = Close()
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
	return cfg, home
}

func testADF(text string) string {
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

func TestDefaultConfluenceConfigNamesSpaceKey(t *testing.T) {
	got := DefaultConfluenceConfig()
	if got == nil {
		t.Fatal("DefaultConfluenceConfig is nil — wiki pass would stay off")
	}
	if len(got.Spaces) != 1 || got.Spaces[0] != DefaultSpaceKey {
		t.Fatalf("Spaces = %v, want [%s]", got.Spaces, DefaultSpaceKey)
	}
}

func TestLocalOriginOriginServesSpace(t *testing.T) {
	cfg, _ := localOriginHome(t)
	w, err := Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	spaces, err := w.Spaces(context.Background())
	if err != nil {
		t.Fatalf("Spaces: %v", err)
	}
	found := false
	for _, s := range spaces {
		if s.Key == DefaultSpaceKey {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("local-origin origin spaces = %+v, want key %s", spaces, DefaultSpaceKey)
	}
}

func TestCreatePageVersion1AndReadPath(t *testing.T) {
	cfg, _ := localOriginHome(t)
	w, err := Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := w.CreatePage(ctx, DefaultSpaceKey, "First note", testADF("hello from create"), "")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreatePage returned empty id")
	}
	if created.Version.Number != 1 {
		t.Fatalf("create version = %d, want 1", created.Version.Number)
	}

	got, err := w.Page(ctx, created.ID)
	if err != nil {
		t.Fatalf("read path Page: %v", err)
	}
	if got.ID != created.ID || got.Title != "First note" {
		t.Fatalf("read path page = %+v", got)
	}
	if got.Version.Number != 1 {
		t.Fatalf("read path version = %d, want 1", got.Version.Number)
	}
	if got.Body.AtlasDocFormat == nil || !strings.Contains(got.Body.AtlasDocFormat.Value, "hello from create") {
		t.Fatalf("read path body = %+v", got.Body)
	}
}

func TestUpdatePageVersion2AndHistory(t *testing.T) {
	cfg, _ := localOriginHome(t)
	w, err := Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := w.CreatePage(ctx, DefaultSpaceKey, "Edits", testADF("v1"), "")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	updated, err := w.UpdatePage(ctx, created.ID, "Edits", testADF("v2"), created.Version.Number+1)
	if err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	if updated.Version.Number != 2 {
		t.Fatalf("update version = %d, want 2", updated.Version.Number)
	}
	hist, err := w.PageVersions(ctx, created.ID)
	if err != nil {
		t.Fatalf("PageVersions: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2; %+v", len(hist), hist)
	}
	if hist[0].Number != 1 || hist[1].Number != 2 {
		t.Fatalf("history numbers = %d,%d want 1,2", hist[0].Number, hist[1].Number)
	}
}

func TestUpdateStaleVersionIsConflict(t *testing.T) {
	cfg, _ := localOriginHome(t)
	w, err := Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := w.CreatePage(ctx, DefaultSpaceKey, "Race", testADF("one"), "")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	// current is 1; next must be 2. Sending 1 is stale.
	_, err = w.UpdatePage(ctx, created.ID, "Race", testADF("stale"), 1)
	if err == nil {
		t.Fatal("stale UpdatePage succeeded")
	}
	if !errors.Is(err, confluence.ErrConflict) {
		t.Fatalf("errors.Is(err, ErrConflict) = false; err = %v", err)
	}
	var apiErr *confluence.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As(APIError) = false; err = %v", err)
	}
	if apiErr.Status != 409 {
		t.Fatalf("APIError.Status = %d, want 409", apiErr.Status)
	}
}

func TestCreatedPageSurvivesPersistReload(t *testing.T) {
	cfg, home := localOriginHome(t)
	w, err := Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := w.CreatePage(ctx, DefaultSpaceKey, "Persisted note", testADF("survive me"), "")
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	id := created.ID
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	persist := PersistPath(home)
	if _, err := os.Stat(persist); err != nil {
		t.Fatalf("persist file missing at %s: %v", persist, err)
	}
	if !strings.HasPrefix(persist, home+string(filepath.Separator)) {
		t.Fatalf("persist %q not under %q", persist, home)
	}

	w2, err := Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w2.Page(ctx, id)
	if err != nil {
		t.Fatalf("Page after reload: %v", err)
	}
	if got.Title != "Persisted note" || got.Version.Number != 1 {
		t.Fatalf("reloaded page = title %q version %d", got.Title, got.Version.Number)
	}
	if got.Body.AtlasDocFormat == nil || !strings.Contains(got.Body.AtlasDocFormat.Value, "survive me") {
		t.Fatalf("reloaded body = %+v", got.Body)
	}

	w3, err := Wiki(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if w2 != w3 {
		t.Fatal("Wiki should reuse the live local-origin session")
	}
}

func TestWikiNilAndConnectedMissingCreds(t *testing.T) {
	if _, err := Wiki(nil); err == nil {
		t.Fatal("Wiki(nil) succeeded")
	}
	if _, err := Wiki(&config.Config{}); err == nil {
		t.Fatal("Wiki(empty connected) succeeded")
	}
	if _, err := Wiki(&config.Config{Site: "https://x.atlassian.net", Email: "a@b.c"}); err == nil {
		t.Fatal("Wiki(no token) succeeded")
	}
}
