package uifocus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

func TestWritePeek(t *testing.T) {
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

// GDK-981: two writes inside one second must stay distinguishable.
//
// The client dedupes a focus payload on `at` so an open tab applies it once
// (GDK-960). That only works while `at` changes per write: stamped at second
// resolution, `gadak views open A && gadak views open B` hands both writes
// the same `at`, and every tab that already applied A drops B in silence.
// Sub-second precision is the contract, not the spelling.
func TestWriteStampsAreDistinguishableWithinOneSecond(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	profile := config.Profile()

	if err := Write("pj=NMA"); err != nil {
		t.Fatal(err)
	}
	_, first, ok, err := PeekFor(profile)
	if err != nil || !ok {
		t.Fatalf("peek A ok=%v err=%v", ok, err)
	}
	if err := Write("pj=NMB"); err != nil {
		t.Fatal(err)
	}
	_, second, ok, err := PeekFor(profile)
	if err != nil || !ok {
		t.Fatalf("peek B ok=%v err=%v", ok, err)
	}

	if !strings.Contains(first, ".") {
		t.Errorf("at = %q, want sub-second precision: two writes in one second are indistinguishable without it", first)
	}
	if first == second {
		t.Errorf("both writes stamped %q — the second focus is dropped by every tab that applied the first", first)
	}
}
