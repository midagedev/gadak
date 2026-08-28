package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/midagedev/gadak/internal/apprun"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

func standaloneApp(t *testing.T) (*config.Config, *server.Handler) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		origin.ResetInProcess()
		config.SetProfile("")
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kind = config.KindStandalone
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(home, "mirror.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	api := server.New(db, cfg)
	t.Cleanup(func() { _ = api.Close() })
	return cfg, api
}

func TestGDK340StandaloneAppEmbedsWithoutAdvertise(t *testing.T) {
	cfg, _ := standaloneApp(t)

	stop, err := apprun.StartOriginPassthrough(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if _, err := os.Stat(filepath.Join(cfg.Directory(), "serve-origin.json")); !os.IsNotExist(err) {
		t.Fatal("desktop must not write serve-origin.json")
	}
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("transport %T, want embedded", c.HTTP.Transport)
	}
}

func TestGDK340ConnectedAppNoAdvertise(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	stop, err := apprun.StartOriginPassthrough(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := os.Stat(filepath.Join(home, "serve-origin.json")); !os.IsNotExist(err) {
		t.Fatalf("connected workspace wrote serve-origin.json: %v", err)
	}
}
