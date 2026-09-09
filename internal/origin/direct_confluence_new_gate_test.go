//go:build sourcelint

// Repo-wide AST gate (GDK-1144): lives behind the sourcelint tag so the
// default `go test ./...` never pays the whole-tree parse. Run with
// `bash tools/sourcelint.sh` — CI's Go-tests step runs exactly that.

package origin

import (
	"go/ast"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

// TestNoDirectConfluenceNewOutsideOrigin is the structural lock: "this
// workspace's Confluence client" is built in this package only. A new
// confluence.New( in production code (outside internal/confluence and this
// package) fails this test.
//
// 2026-08-18 GDK-267: FAIL-first ran against the four production callers
// (cmd/gadak/api.go, internal/server/settings.go, internal/sync/confluence.go,
// internal/sync/one.go) before they were rewritten to origin.Wiki. Tests
// (*_test.go) may still call confluence.New to stand up httptest servers.
func TestNoDirectConfluenceNewOutsideOrigin(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	var hits []string
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") {
			return nil
		}
		if allowedConfluenceNewFile(af.Rel) {
			return nil
		}
		hits = append(hits, findConfluenceNewCalls(t, af)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("confluence.New must not be called from production code outside internal/origin (and its definition in internal/confluence):\n  %s",
			strings.Join(hits, "\n  "))
	}
}

func allowedConfluenceNewFile(rel string) bool {
	if strings.HasPrefix(rel, "internal/origin/") {
		return true
	}
	// The constructor itself (and any other file in that package).
	if strings.HasPrefix(rel, "internal/confluence/") {
		return true
	}
	return false
}

func findConfluenceNewCalls(t *testing.T, af *archlint.File) []string {
	t.Helper()
	f, fset, err := af.AST()
	if err != nil {
		t.Fatalf("parse %s: %v", af.Rel, err)
		return nil
	}
	var confName string
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p != "github.com/midagedev/gadak/internal/confluence" {
			continue
		}
		if imp.Name != nil {
			confName = imp.Name.Name
		} else {
			confName = "confluence"
		}
	}
	if confName == "" || confName == "_" {
		return nil
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "New" {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != confName {
			return true
		}
		pos := fset.Position(call.Pos())
		hits = append(hits, filepath.ToSlash(af.Rel)+":"+strconv.Itoa(pos.Line))
		return true
	})
	return hits
}
