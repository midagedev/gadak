package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func recopyDemoDB(t *testing.T) {
	t.Helper()
	home := os.Getenv("GADAK_HOME")
	if home == "" {
		t.Fatal("GADAK_HOME unset")
	}
	src := filepath.Join("..", "..", "examples", "demo.db")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	for _, name := range []string{"gadak.db", "gadak.db-wal", "gadak.db-shm"} {
		_ = os.Remove(filepath.Join(home, name))
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db"), raw, 0o600); err != nil {
		t.Fatalf("recopy demo.db: %v", err)
	}
}

// TestExportImportRoundTripDemoDB is the T1.4 / kepano gate: a saved view
// (plus a watch and a favorite) survive export → delete mirror → recopy
// demo.db → import. Distinctive site/token strings in config.json must not
// appear in the file.
func TestExportImportRoundTripDemoDB(t *testing.T) {
	sqlDemoHome(t)
	cfg := &config.Config{
		Site:  "https://no-export.example.test",
		Email: "secret-owner@example.test",
		Token: "tok-must-not-appear-in-export",
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	view := store.SavedView{
		ID:     "t14-night",
		Name:   "Night triage",
		Config: json.RawMessage(`{"filters":{},"display":{}}`),
	}
	if err := db.PutSavedView(context.Background(), view); err != nil {
		t.Fatal(err)
	}
	if err := db.SetWatch(context.Background(), "NMB-1", true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetFavorite(context.Background(), "NMB-2", true); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(t.TempDir(), "personal.json")
	if _, err := capture(t, func() error {
		return cmdExport([]string{"--out", outPath})
	}); err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"gadak_export"`) {
		t.Fatalf("missing gadak_export field:\n%s", body)
	}
	if !strings.Contains(body, "Night triage") {
		t.Fatalf("saved view missing from export:\n%s", body)
	}
	if !strings.Contains(body, "NMB-1") || !strings.Contains(body, "NMB-2") {
		t.Fatalf("watch/favorite missing from export:\n%s", body)
	}
	for _, secret := range []string{cfg.Site, cfg.Email, cfg.Token} {
		if strings.Contains(body, secret) {
			t.Fatalf("export leaked %q:\n%s", secret, body)
		}
	}

	recopyDemoDB(t)

	db, err = openStore()
	if err != nil {
		t.Fatal(err)
	}
	views, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		if v.Name == "Night triage" {
			t.Fatal("view survived recopy; cannot test restore")
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := capture(t, func() error {
		return cmdImport([]string{outPath})
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	db, err = openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	views, err = db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range views {
		if v.ID == "t14-night" && v.Name == "Night triage" {
			found = true
		}
	}
	if !found {
		t.Fatalf("view not restored: %+v", views)
	}
	watches, err := db.Watches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(watches, "NMB-1") {
		t.Fatalf("watch not restored: %v", watches)
	}
	favs, err := db.Favorites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(favs, "NMB-2") {
		t.Fatalf("favorite not restored: %v", favs)
	}
}

func TestImportRejectsVersionMismatch(t *testing.T) {
	sqlDemoHome(t)
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte(`{"gadak_export":99,"views":[],"watches":[],"favorites":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error { return cmdImport([]string{p}) })
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("want unsupported version error, got %v", err)
	}
}

func TestImportRejectsMissingVersion(t *testing.T) {
	sqlDemoHome(t)
	p := filepath.Join(t.TempDir(), "no-ver.json")
	if err := os.WriteFile(p, []byte(`{"views":[],"watches":[],"favorites":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error { return cmdImport([]string{p}) })
	if err == nil || !strings.Contains(err.Error(), "gadak_export") {
		t.Fatalf("want missing gadak_export error, got %v", err)
	}
}

func TestImportWarnsUnknownTopLevelKey(t *testing.T) {
	sqlDemoHome(t)
	p := filepath.Join(t.TempDir(), "fwd.json")
	body := `{"gadak_export":1,"exported_at":"2026-08-15T00:00:00Z","views":[],"watches":[],"favorites":[],"future_field":true}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := captureErr(t, func() error { return cmdImport([]string{p}) })
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !strings.Contains(stderr, "future_field") {
		t.Fatalf("want unknown-key warning mentioning future_field, stderr=%q", stderr)
	}
}

func TestImportFileWinsNameConflict(t *testing.T) {
	sqlDemoHome(t)
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PutSavedView(context.Background(), store.SavedView{
		ID:     "local-id",
		Name:   "Night triage",
		Config: json.RawMessage(`{"filters":{"old":true}}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	p := filepath.Join(t.TempDir(), "win.json")
	body := `{"gadak_export":1,"views":[{"id":"file-id","name":"Night triage","config":{"filters":{"new":true}}}],"watches":[],"favorites":[]}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := capture(t, func() error { return cmdImport([]string{p}) }); err != nil {
		t.Fatalf("import: %v", err)
	}

	db, err = openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	views, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].ID != "file-id" || views[0].Name != "Night triage" {
		t.Fatalf("file should replace same-named view, got %+v", views)
	}
	if !strings.Contains(string(views[0].Config), `"new"`) {
		t.Fatalf("file config did not win: %s", views[0].Config)
	}
}

func TestExportHelpDistinctFromExportStatic(t *testing.T) {
	ex := formatHelp("export", nil)
	st := formatHelp("export-static", nil)
	if !strings.Contains(ex, "views") || !strings.Contains(ex, "watches") || !strings.Contains(ex, "favorites") {
		t.Fatalf("export help must name the three personal tables:\n%s", ex)
	}
	if strings.Contains(ex, "hosted demo") || strings.Contains(strings.ToLower(ex), "static json") {
		t.Fatalf("export help collides with export-static:\n%s", ex)
	}
	if strings.Contains(st, "watches") || strings.Contains(st, "favorites") {
		t.Fatalf("export-static help collides with export:\n%s", st)
	}
}

func containsKey(keys []string, want string) bool {
	for _, k := range keys {
		if k == want {
			return true
		}
	}
	return false
}
