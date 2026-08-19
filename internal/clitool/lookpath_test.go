package clitool

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

func TestResolveNPMLookupOrder(t *testing.T) {
	var order []string
	look := func(name string) (string, error) {
		order = append(order, "path:"+name)
		return "", errors.New("not on PATH")
	}
	present := func(p string) bool {
		order = append(order, p)
		return false
	}
	if got, ok := ResolveNPM(look, present); ok {
		t.Fatalf("expected miss, got %q", got)
	}
	want := append([]string{"path:npm"}, NPMFallbackPaths...)
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, want %v", order, want)
	}
}

func TestResolveNPMPathWins(t *testing.T) {
	look := func(name string) (string, error) {
		if name != "npm" {
			t.Errorf("LookPath(%q)", name)
		}
		return "/custom/bin/npm", nil
	}
	present := func(p string) bool {
		t.Errorf("fallback should not run after PATH hit; present(%s)", p)
		return false
	}
	got, ok := ResolveNPM(look, present)
	if !ok || got != "/custom/bin/npm" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestResolveNPMSecondFallback(t *testing.T) {
	look := func(string) (string, error) { return "", errors.New("no") }
	present := func(p string) bool { return p == "/usr/local/bin/npm" }
	got, ok := ResolveNPM(look, present)
	if !ok || got != "/usr/local/bin/npm" {
		t.Fatalf("got %q ok=%v, want /usr/local/bin/npm", got, ok)
	}
}

func TestNPMNotFoundDetailListsTable(t *testing.T) {
	detail := NPMNotFoundDetail()
	if !strings.HasPrefix(detail, "PATH, then ") {
		t.Fatalf("detail %q must start with PATH, then", detail)
	}
	for _, p := range NPMFallbackPaths {
		if !strings.Contains(detail, p) {
			t.Errorf("detail %q missing table entry %s", detail, p)
		}
	}
}

func TestRaycastExtDirHonorsGADAKHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("work")

	got, err := RaycastExtDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, RaycastExtDirName)
	if got != want {
		t.Errorf("RaycastExtDir() = %q, want %q (profile must not nest it)", got, want)
	}
}

func TestLookPathThenEmptyLookIsMiss(t *testing.T) {
	look := func(string) (string, error) { return "", nil }
	got, ok := LookPathThen("gadak", []string{"/usr/local/bin/gadak"}, look, func(p string) bool {
		return p == "/usr/local/bin/gadak"
	})
	if !ok || got != "/usr/local/bin/gadak" {
		t.Fatalf("empty LookPath must fall through: got %q ok=%v", got, ok)
	}
}

func TestExecutableRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows treats a regular file as executable")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "tool")
	if err := os.WriteFile(bin, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if executable(bin) {
		t.Fatal("0644 regular file must not count as executable on unix")
	}
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if !executable(bin) {
		t.Fatal("0755 regular file must count as executable")
	}
}
