package uifocus

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

func TestWriteTake(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := Write("pj=NMA&sc=inprogress"); err != nil {
		t.Fatal(err)
	}
	hash, ok, err := TakeFor(config.Profile())
	if err != nil || !ok || hash != "pj=NMA&sc=inprogress" {
		t.Fatalf("take %q ok=%v err=%v", hash, ok, err)
	}
	if _, ok, err := TakeFor(config.Profile()); err != nil || ok {
		t.Fatal("second take should be empty")
	}
}

func TestTakeForUsesProfileDir(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := WriteFor("work", "ks=BBB-1"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFor("", "ks=AAA-1"); err != nil {
		t.Fatal(err)
	}
	if hash, ok, err := TakeFor(config.Profile()); err != nil || !ok || hash != "ks=AAA-1" {
		t.Fatalf("default take %q ok=%v err=%v", hash, ok, err)
	}
	if hash, ok, err := TakeFor("work"); err != nil || !ok || hash != "ks=BBB-1" {
		t.Fatalf("work take %q ok=%v err=%v", hash, ok, err)
	}
	if _, ok, err := TakeFor("work"); err != nil || ok {
		t.Fatal("second TakeFor(work) should be empty")
	}
}

func TestTakeIgnoresStale(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	p, err := PathFor(config.Profile())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"hash":"pj=NMA","at":"` + time.Now().UTC().Add(-time.Hour).Format(time.RFC3339) + `"}`
	if err := os.WriteFile(p, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := TakeFor(config.Profile()); err != nil || ok {
		t.Fatalf("stale should be ignored ok=%v err=%v", ok, err)
	}
}
