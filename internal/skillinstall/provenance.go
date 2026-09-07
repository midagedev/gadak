package skillinstall

import (
	"runtime/debug"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Provenance of the binary (GDK-1531)
//
// status.go answers "who wrote this file" from its content hash. This file
// answers the question one level up: *what kind of gadak* wrote it — one cut as
// a release, or one built straight from a checkout whose embedded SKILL.md is
// whatever happened to be in the working tree.
//
// Measured 2026-09-07: a checkout build replaced the developer's real
// ~/.claude/skills/gadak/SKILL.md with an uncommitted copy, and the receipt it
// left recorded only `gadak_version: 0.0.0-dev` — a field nothing read. Two
// things hang off this file so that cannot repeat quietly:
//
//	IsDevBuild     the daily auto-sync asks it before it writes anything
//	SourceFor      every receipt records which kind of binary wrote it, so
//	               `gadak doctor` can say `dev-tree` instead of pretending the
//	               copy is a shipped one
//
// One owner for both: a second copy of the "0.0.0-dev" literal is how the two
// answers drift apart.
// ---------------------------------------------------------------------------

// DevVersion is the version string cmd/gadak carries when the release ldflag
// (-X main.version=…, see .goreleaser.yaml) was not applied.
const DevVersion = "0.0.0-dev"

// The provenance words a receipt records. They are wire format — `gadak doctor
// --json` prints them — so the strings are contract.
const (
	SourceRelease = "release"  // a cut release wrote this copy
	SourceDevTree = "dev-tree" // a binary built from a checkout wrote it
)

// IsDevBuild reports whether version identifies a binary built from a checkout
// rather than cut as a release.
//
// An empty version — a binary carrying no build information at all — counts as
// a dev build. "Unknown provenance" must not be trusted with the developer's
// agent configuration; the failure mode of guessing wrong the other way is
// exactly the incident this closes.
func IsDevBuild(version string) bool {
	v := strings.TrimSpace(version)
	return v == "" || v == DevVersion
}

// SourceFor is the receipt word for a version string.
func SourceFor(version string) string {
	if IsDevBuild(version) {
		return SourceDevTree
	}
	return SourceRelease
}

var (
	buildRevOnce sync.Once
	buildRev     string
)

// BuildRevision is the short git hash this binary was built from, or "" when
// the toolchain stamped none (-buildvcs=false, or a build from outside a
// checkout). A tree with uncommitted changes gets a trailing "+", the same
// convention as `git describe --dirty` — and the copy that started GDK-1531
// came from exactly such a tree, so that marker is the whole point.
func BuildRevision() string {
	buildRevOnce.Do(func() { buildRev = readBuildRevision(debug.ReadBuildInfo) })
	return buildRev
}

// readBuildRevision is BuildRevision with the build-info source injected, so a
// test can assert the shortening and the dirty marker without needing a
// particular checkout state.
func readBuildRevision(read func() (*debug.BuildInfo, bool)) string {
	bi, ok := read()
	if !ok || bi == nil {
		return ""
	}
	rev := ""
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "+"
	}
	return rev
}
