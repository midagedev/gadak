//go:build sourcelint

// Repo-wide AST gate (GDK-1144 discipline, GDK-1129 contract): lives behind
// the sourcelint tag so the default `go test ./...` never pays the
// whole-tree parse. Run with `bash tools/sourcelint.sh` — CI's Go-tests step
// runs exactly that.

package fields

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

// canonicalKeyOwner is the only production file allowed to spell the
// trim+case-fold key-normalization composition. CanonicalKey (GDK-1129) is
// the owner; everything else composes through it.
var canonicalKeyOwner = map[string]string{
	"internal/fields/editable.go": "CanonicalKey is the single owner (GDK-1129)",
}

// pendingCanonicalKeyFile lists production files that still hand-roll the
// composition. The 0.19 audit measured eleven key sites; they live in
// packages owned by other tracks of the same release audit, so each entry
// names why it is still pending. The ratchet only moves one way: a migrated
// site makes its entry stale (this test fails until the entry is dropped),
// and a brand-new hand-rolled site fails outright.
var pendingCanonicalKeyFile = map[string]string{
	// Files outside this round's boundary (cmd/, jql, server, mcp are other
	// tracks of the same audit). Each migrates to CanonicalKey/IsIssueKey in
	// its own round; when it does, this gate goes stale and forces the entry
	// out — the ratchet cannot slip backwards.
	"cmd/gadak/agent.go":         "normalizeKey is this exact function; fold to fields.CanonicalKey and delete it (GDK-1129 named it for immediate deletion)",
	"cmd/gadak/views.go":         "looksLikeIssueKey = fields.IsIssueKey; fold (GDK-1129 named it for immediate deletion)",
	"cmd/gadak/fields.go":        "project filter canonicalization, cmd track",
	"cmd/gadak/create.go":        "issue-key positional canonicalization, cmd track",
	"cmd/gadak/config.go":        "project-key canonicalization, cmd track",
	"internal/jql/compile.go":    "mergeUniqueUpper keys — also holds the two-standards defect the issue names (dst ToUpper-only, src ToUpper+Trim)",
	"internal/server/link.go":    "issue-key canonicalization, server track",
	"internal/server/write.go":   "parent-key comparison, server track",
	"internal/mcp/tools.go":      "three issue-key sites (show/edit/comment keys), mcp track",
	"internal/jira/devstatus.go": "not a key: dev status / gh-state name canonicalization — tracked here only so the idiom's file count cannot grow silently",
}

// TestKeyNormalizationHasOneOwner fails on any production file spelling
// strings.ToUpper(strings.TrimSpace( that is neither the owner nor a listed
// pending site, and on stale owner/pending entries.
func TestKeyNormalizationHasOneOwner(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	const idiom = "strings.ToUpper(strings.TrimSpace("
	hits := map[string]bool{}
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") {
			return nil
		}
		body, err := af.Src()
		if err != nil {
			return err
		}
		if strings.Contains(string(body), idiom) {
			hits[af.Rel] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var extra []string
	for rel := range hits {
		if _, ok := canonicalKeyOwner[rel]; ok {
			continue
		}
		if _, ok := pendingCanonicalKeyFile[rel]; ok {
			continue
		}
		extra = append(extra, rel)
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		t.Errorf("hand-rolled key normalization (strings.ToUpper(strings.TrimSpace()) must go through fields.CanonicalKey (GDK-1129); new sites:\n  %s",
			strings.Join(extra, "\n  "))
	}
	for rel, reason := range canonicalKeyOwner {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("owner %q has no reason", rel)
		}
		if !hits[rel] {
			t.Errorf("owner %q is stale — it no longer contains the composition", rel)
		}
	}
	for rel, reason := range pendingCanonicalKeyFile {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("pending %q has no reason", rel)
		}
		if !hits[rel] {
			t.Errorf("pending %q is stale — the idiom is gone; migrate the call to fields.CanonicalKey and drop the entry", rel)
		}
	}
}
