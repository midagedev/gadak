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
	hash, at, ok, err := PeekFor(config.Profile())
	if err != nil || !ok || hash != "pj=NMA&sc=inprogress" {
		t.Fatalf("peek %q ok=%v err=%v", hash, ok, err)
	}
	if at == "" {
		t.Fatal("peek must return at")
	}
	// GDK-960: freshness is MaxAge, not consume-on-read. A second client
	// (desktop + serve tab) must see the same hash; the file is not deleted.
	hash2, at2, ok, err := PeekFor(config.Profile())
	if err != nil || !ok || hash2 != hash {
		t.Fatalf("second peek %q ok=%v err=%v, want %q", hash2, ok, err, hash)
	}
	if at2 != at {
		t.Fatalf("second peek at %q, want %q", at2, at)
	}
}

func TestPeekForUsesProfileDir(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := WriteFor("work", "ks=BBB-1"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFor("", "ks=AAA-1"); err != nil {
		t.Fatal(err)
	}
	if hash, _, ok, err := PeekFor(config.Profile()); err != nil || !ok || hash != "ks=AAA-1" {
		t.Fatalf("default peek %q ok=%v err=%v", hash, ok, err)
	}
	hash, at, ok, err := PeekFor("work")
	if err != nil || !ok || hash != "ks=BBB-1" {
		t.Fatalf("work peek %q ok=%v err=%v", hash, ok, err)
	}
	// GDK-960: a second peek of the same profile still returns the hash.
	hash2, at2, ok, err := PeekFor("work")
	if err != nil || !ok || hash2 != hash {
		t.Fatalf("second PeekFor(work) %q ok=%v err=%v, want %q", hash2, ok, err, hash)
	}
	if at2 != at {
		t.Fatalf("second PeekFor(work) at %q, want %q", at2, at)
	}
}

func TestPeekIgnoresStale(t *testing.T) {
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
	if _, _, ok, err := PeekFor(config.Profile()); err != nil || ok {
		t.Fatalf("stale should be ignored ok=%v err=%v", ok, err)
	}
}
