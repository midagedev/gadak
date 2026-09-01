package originbind

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

// TestNoDirectKindClearOutsideOriginbind is the structural lock: clearing
// Kind (leaving localOrigin) belongs to ClearLocalOrigin in this package.
// A `.Kind = ""` assignment in production code outside internal/originbind/
// fails this test.
//
// Tests (*_test.go) may still write Kind in fixtures.
// Pattern borrowed from internal/origin/direct_new_gate_test.go.
func TestNoDirectKindClearOutsideOriginbind(t *testing.T) {
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
			switch d.Name() {
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
		if strings.HasPrefix(rel, "internal/originbind/") {
			return nil
		}
		hits = append(hits, findKindClearAssignments(t, path, rel)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("cfg.Kind = \"\" must not be assigned from production code outside internal/originbind (use workspace.ClearLocalOrigin):\n  %s",
			strings.Join(hits, "\n  "))
	}
}

func findKindClearAssignments(t *testing.T, path, rel string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
		return nil
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range as.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Kind" {
				continue
			}
			if i >= len(as.Rhs) {
				continue
			}
			lit, ok := as.Rhs[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if lit.Value != `""` && lit.Value != "``" {
				continue
			}
			pos := fset.Position(as.Pos())
			hits = append(hits, rel+":"+itoa(pos.Line))
		}
		return true
	})
	return hits
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
