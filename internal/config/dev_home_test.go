package config

import (
	"path/filepath"
	"testing"
)

// GDK-1697: a dev build's default home is ~/.gadak-dev, so a checkout build
// and the installed release never share workspace files unless GADAK_HOME
// says so. Off by default — nothing else in this package's tests calls
// SetDevBuild, and they keep ~/.gadak.
func TestDevBuildHomeIsGadakDev(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("GADAK_HOME", "")
	t.Setenv("SCRY_HOME", "")
	t.Cleanup(func() { SetDevBuild(false) })

	SetDevBuild(true)
	got, err := homeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, DevDirName); got != want {
		t.Fatalf("dev build homeRoot() = %q, want %q", got, want)
	}
	if !DevHome() {
		t.Fatal("DevHome() = false under a dev build with no GADAK_HOME")
	}

	// GADAK_HOME wins for both builds — that is how a dev build is pointed
	// at a real home on purpose, and GDK-1687's refusal covers that case.
	override := filepath.Join(root, "elsewhere")
	t.Setenv("GADAK_HOME", override)
	got, err = homeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != override {
		t.Fatalf("dev build with GADAK_HOME: homeRoot() = %q, want %q", got, override)
	}
	if DevHome() {
		t.Fatal("DevHome() = true with GADAK_HOME set — the reason doctor prints would be wrong")
	}

	SetDevBuild(false)
	t.Setenv("GADAK_HOME", "")
	got, err = homeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, DirName); got != want {
		t.Fatalf("release homeRoot() = %q, want %q", got, want)
	}
}
