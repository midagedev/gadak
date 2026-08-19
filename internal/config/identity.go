package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

const (
	// Name is the CLI binary, desktop app, and user-facing product name.
	Name = "gadak"
	// DirName is the directory under $HOME that holds the default profile.
	DirName = ".gadak"
	// DBFile is the SQLite filename inside a profile directory.
	DBFile = "gadak.db"
	// EnvPrefix is prepended to HOME, PROFILE, TOKEN, SITE, EMAIL, PROJECTS.
	EnvPrefix = "GADAK_"

	// Legacy names from the 2026-08 rename (scry → gadak). Still accepted so
	// an existing install keeps working until the user next launches gadak.
	LegacyName      = "scry"
	LegacyDirName   = ".scry"
	LegacyDBFile    = "scry.db"
	LegacyEnvPrefix = "SCRY_"
)

// envSuffixes is every suffix production currently passes to Env. Env itself
// does not consult this map — a new call site works immediately. The map is
// only the runtime allowlist for warnUnknownGADAK. A hardcoded census is
// unavoidable here: Env is a generic constructor, the suffixes live at call
// sites across packages, and the process cannot scan its own source. It is
// not left to rot: tools/doc-checks.sh check 18 derives the same set from
// the Go source and fails when this map falls behind.
var envSuffixes = map[string]struct{}{
	"HOME": {}, "PROFILE": {}, "TOKEN": {}, "SITE": {}, "EMAIL": {}, "PROJECTS": {},
}

// envLiterals are GADAK_* names production reads via os.Getenv, not Env.
// cmd/gadak/views.go (GADAK_NO_OPEN) and desktop/integrations.go
// (GADAK_DESKTOP_CLI). Check 18 covers these too.
var envLiterals = map[string]struct{}{
	"GADAK_NO_OPEN":     {},
	"GADAK_DESKTOP_CLI": {},
}

// envHarness are names this repository's own harness exports into the
// environment a gadak process then runs in — `make media` sets GADAK_MEDIA
// before Playwright starts e2e/serve.sh, which runs `gadak status`. The
// binary does not read them, but warning about them would be pure noise in
// the project's own workflows, so they are silent. This map is the one
// place staleness is harmless: a missing name costs a spurious line, never
// a ghost that stays quiet, so check 18 deliberately does not police it.
var envHarness = map[string]struct{}{
	"GADAK_MEDIA":     {},
	"GADAK_PERF":      {},
	"GADAK_FRESHEN":   {},
	"GADAK_BASE_PATH": {},
}

// Env returns GADAK_<suffix>, then SCRY_<suffix> if the new name is unset
// or empty. An empty GADAK_* value is treated as unset so a blank export
// cannot hide a real SCRY_* fallback (decision 0007: read SCRY_* when
// GADAK_* is unset).
func Env(suffix string) string {
	if v := os.Getenv(EnvPrefix + suffix); v != "" {
		return v
	}
	return os.Getenv(LegacyEnvPrefix + suffix)
}

var unknownEnvWarnOnce sync.Once

func knownGADAK(name string) bool {
	if !strings.HasPrefix(name, EnvPrefix) {
		return true
	}
	suffix := strings.TrimPrefix(name, EnvPrefix)
	if _, ok := envSuffixes[suffix]; ok {
		return true
	}
	if _, ok := envLiterals[name]; ok {
		return true
	}
	_, ok := envHarness[name]
	return ok
}

// warnUnknownGADAK prints one stderr line when a GADAK_* variable is set
// that this process does not read. Empty values are unset (same as Env).
// Called from DBPath — that is the moment a ghost like GADAK_DB would have
// been believed to relocate the mirror. stdout is untouched (gadak sql
// contract).
func warnUnknownGADAK() {
	unknownEnvWarnOnce.Do(func() {
		var unknown []string
		for _, kv := range os.Environ() {
			name, val, ok := strings.Cut(kv, "=")
			if !ok || val == "" {
				continue
			}
			if !strings.HasPrefix(name, EnvPrefix) {
				continue
			}
			if knownGADAK(name) {
				continue
			}
			unknown = append(unknown, name)
		}
		if len(unknown) == 0 {
			return
		}
		sort.Strings(unknown)
		fmt.Fprintf(os.Stderr, "gadak: ignoring unrecognised %s (not read; home is GADAK_HOME, profile is GADAK_PROFILE)\n", strings.Join(unknown, ", "))
	})
}
