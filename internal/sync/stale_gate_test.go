package sync

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

// mirrorStaleSentenceOwner is the only production file allowed to contain
// the CLI "did not refresh" sentence. GDK-740: every other site classifies
// with errors.Is(ErrMirrorStale) and calls this author.
//
// 2026-08-23: one owner so a fourteenth write verb cannot hand-write a
// failure for a write that already landed.
var mirrorStaleSentenceOwner = map[string]string{
	"cmd/gadak/agent.go": "writeAppliedMirrorStaleMessage authors the CLI warning",
}

// mirrorStaleWireOwner is the only production file allowed to decide the
// write_applied_mirror_stale wire code. REST status and code stay
// byte-identical (contracts/api.md); this round only routes through one
// helper. handleResync still uses failJira (re-read only, no preceding
// write) and must not appear here.
var mirrorStaleWireOwner = map[string]string{
	"internal/server/write.go": "failMirrorStale emits 502 write_applied_mirror_stale",
}

func TestMirrorStaleClassHasOneOwner(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	sentenceHits := map[string][]string{}
	wireHits := map[string][]string{}
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") {
			return nil
		}
		f, fset, err := af.AST()
		if err != nil {
			t.Fatalf("parse %s: %v", af.Rel, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			pos := fset.Position(lit.Pos())
			loc := af.Rel + ":" + strconv.Itoa(pos.Line)
			if strings.Contains(s, "did not refresh") {
				sentenceHits[af.Rel] = append(sentenceHits[af.Rel], loc)
			}
			if strings.Contains(s, "write_applied_mirror_stale") {
				wireHits[af.Rel] = append(wireHits[af.Rel], loc)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	checkOwner(t, "CLI sentence \"did not refresh\"", sentenceHits, mirrorStaleSentenceOwner)
	checkOwner(t, "REST wire code write_applied_mirror_stale", wireHits, mirrorStaleWireOwner)
}

func checkOwner(t *testing.T, label string, hits map[string][]string, owner map[string]string) {
	t.Helper()
	var extra []string
	for rel, locs := range hits {
		if _, ok := owner[rel]; ok {
			continue
		}
		extra = append(extra, locs...)
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		t.Errorf("%s must live only in the owner file(s); extra:\n  %s", label, strings.Join(extra, "\n  "))
	}
	for rel, reason := range owner {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("owner %q has no reason", rel)
		}
		if _, ok := hits[rel]; !ok {
			t.Errorf("owner %q is stale — %s no longer contains the %s literal", rel, rel, label)
		}
	}
}
