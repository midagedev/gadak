package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func teamImportFile(t *testing.T, dir, name string) string {
	t.Helper()
	body := `{
  "gadak_team_config": 1,
  "exported_at": "2026-08-14T00:00:00Z",
  "settings": {"groupLabels": {"platform": "Platform"}},
  "views": [{"name": "Night triage", "config": {"filters": {}, "display": {}}}]
}`
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTeamImportSaveFailureLeavesViewsUnwritten(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	cfg := &config.Config{Site: "https://example.atlassian.net", Email: "a@b.c", Token: "tok"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// Ensure the mirror exists so openStore succeeds.
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	saved := saveConfig
	t.Cleanup(func() { saveConfig = saved })
	saveConfig = func(*config.Config) error { return errors.New("injected save failure") }

	in := teamImportFile(t, home, "team.json")
	_, err = capture(t, func() error {
		return cmdTeamImport([]string{in})
	})
	if err == nil {
		t.Fatal("expected Save failure to surface")
	}
	if !strings.Contains(err.Error(), "injected save failure") {
		t.Fatalf("want injected save error, got %v", err)
	}

	db, err = store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	views, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("Save failure must not persist views, got %+v", views)
	}
}

func TestTeamImportAppliesSettingsAndViews(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	cfg := &config.Config{Site: "https://example.atlassian.net", Email: "a@b.c", Token: "tok"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	in := teamImportFile(t, home, "team.json")
	out, err := capture(t, func() error {
		return cmdTeamImport([]string{in})
	})
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Site != cfg.Site || got.Email != cfg.Email || got.Token != cfg.Token {
		t.Fatalf("credentials changed: %+v", got)
	}
	if got.GroupLabels["platform"] != "Platform" {
		t.Fatalf("settings not saved: %+v", got.GroupLabels)
	}

	db, err = store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	views, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Name != "Night triage" {
		t.Fatalf("views: %+v", views)
	}
}
