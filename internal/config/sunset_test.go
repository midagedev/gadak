package config

// The scry→gadak rename compatibility ships with a sunset (the
// rename-sunset audit; decision 0007's addendum is the record):
// LegacySunsetRelease names the release that drops it. Until CHANGELOG.md
// carries a release at or past that version this test passes silently —
// the compat is allowed to live. The day the sunset release is cut, it
// fails listing every surviving site, and keeps failing until the drop
// lands. A release must not quietly carry the legacy names past their
// sunset; when the drop happens, this test goes with them (it is part of
// the ~120 lines).

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scryCompatSites is what the drop removes, as (file, case-insensitive
// token) pairs. The tokens are chosen so every current compat line matches
// and nothing else in those files does: identity.go and config.go mention
// scry only as the legacy name; main.go's hint goes through the
// config.LegacyName identifier; storage.ts's migrations all carry the
// scry: prefix. doctor's HomeLeftover is deliberately absent — it reports
// a directory on disk, it does not read it, and "you still have a ~/.scry"
// stays true after the drop.
var scryCompatSites = []struct{ path, token string }{
	{"internal/config/identity.go", "scry"},
	{"internal/config/config.go", "scry"},
	{"cmd/gadak/main.go", "config.legacyname"},
	{"web/src/lib/storage.ts", "scry:"},
}

func TestScryCompatSunsetEnforced(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	if !releasedAtOrPast(string(body), LegacySunsetRelease) {
		return // before the sunset: the compat may live
	}

	var hits []string
	for _, s := range scryCompatSites {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(s.path)))
		if err != nil {
			hits = append(hits, fmt.Sprintf("%s: unreadable (%v) — if the file moved, update scryCompatSites", s.path, err))
			continue
		}
		if strings.Contains(strings.ToLower(string(b)), s.token) {
			hits = append(hits, s.path+": still carries scry compatibility")
		}
	}
	if len(hits) > 0 {
		t.Fatalf("the scry compatibility outlived its sunset (%s is released): drop it — decision 0007's addendum and LegacySunsetRelease list the sites; delete this test with them:\n  %s",
			LegacySunsetRelease, strings.Join(hits, "\n  "))
	}
}

// releasedAtOrPast reports whether changelog text carries a release
// heading (`## vMAJOR.MINOR.PATCH …`) at or past want (`vMAJOR.MINOR.PATCH`).
func releasedAtOrPast(changelog, want string) bool {
	wantV, ok := parseSemver(strings.TrimPrefix(want, "v"))
	if !ok {
		return false // a malformed sunset constant never fires — loud, not silent
	}
	for _, line := range strings.Split(changelog, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "## v") {
			continue
		}
		ver := strings.TrimPrefix(line, "## v")
		if i := strings.IndexAny(ver, " \t"); i >= 0 {
			ver = ver[:i]
		}
		if v, ok := parseSemver(ver); ok && semverAtLeast(v, wantV) {
			return true
		}
	}
	return false
}

func parseSemver(s string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return v, false
			}
			n = n*10 + int(c-'0')
		}
		v[i] = n
	}
	return v, true
}

// semverAtLeast is component-wise major.minor.patch order.
func semverAtLeast(v, want [3]int) bool {
	for i := range v {
		if v[i] != want[i] {
			return v[i] > want[i]
		}
	}
	return true
}
