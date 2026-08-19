package origin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			switch name {
			case ".git", "vendor", "node_modules", "dist", "testdata", "scratch", ".claude":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if allowedConfluenceNewFile(rel) {
			return nil
		}
		hits = append(hits, findConfluenceNewCalls(t, path, rel)...)
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

func findConfluenceNewCalls(t *testing.T, path, rel string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
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
		hits = append(hits, filepath.ToSlash(rel)+":"+itoa(pos.Line))
		return true
	})
	return hits
}
