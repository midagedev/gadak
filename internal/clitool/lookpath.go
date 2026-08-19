package clitool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/midagedev/gadak/internal/config"
)

// NPMFallbackPaths is LookPath("npm") then these, first existing+executable
// wins. Same brew locations as contrib/raycast/src/gadak.ts GADAK_CANDIDATES,
// opposite direction (we are finding npm, not gadak).
//
// Owned here so `gadak raycast install` and the desktop catalog cannot drift.
var NPMFallbackPaths = []string{
	"/opt/homebrew/bin/npm",
	"/usr/local/bin/npm",
}

// RaycastExtDirName is the directory under the gadak home that holds the
// unpacked Raycast extension. Profile is ignored: one copy per home.
const RaycastExtDirName = "raycast-extension"

// LookPathThen is LookPath(name), then the first fallback that present
// reports. look and present are injected so tests can assert order without
// touching the host. A nil present falls back to an execute-bit check
// (on Windows, any regular file).
func LookPathThen(name string, fallbacks []string, look func(string) (string, error), present func(string) bool) (string, bool) {
	if look != nil {
		if p, err := look(name); err == nil && p != "" {
			return p, true
		}
	}
	if present == nil {
		present = executable
	}
	for _, c := range fallbacks {
		if present(c) {
			return c, true
		}
	}
	return "", false
}

// ResolveNPM is LookPath("npm"), then NPMFallbackPaths.
func ResolveNPM(look func(string) (string, error), present func(string) bool) (string, bool) {
	return LookPathThen("npm", NPMFallbackPaths, look, present)
}

// NPMNotFoundDetail names every place ResolveNPM looks. Generated from
// NPMFallbackPaths so the wording cannot drift from the table.
func NPMNotFoundDetail() string {
	return "PATH, then " + strings.Join(NPMFallbackPaths, ", ")
}

// RaycastExtDir is $GADAK_HOME/raycast-extension (profile ignored).
// A DirFor error is returned: the install verb must not write to a guessed
// path. Catalog display that cannot fail should fall back at the call site.
func RaycastExtDir() (string, error) {
	base, err := config.DirFor("")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, RaycastExtDirName), nil
}

func executable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode()&0o111 != 0
}
