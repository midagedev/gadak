package originbind

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

// TestNoDirectKindClearOutsideOriginbind is the structural lock: clearing
// Kind (leaving builtIn) belongs to ClearBuiltIn in this package.
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
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") {
			return nil
		}
		if strings.HasPrefix(af.Rel, "internal/originbind/") {
			return nil
		}
		hits = append(hits, findKindClearAssignments(t, af)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("cfg.Kind = \"\" must not be assigned from production code outside internal/originbind (use workspace.ClearBuiltIn):\n  %s",
			strings.Join(hits, "\n  "))
	}
}

func findKindClearAssignments(t *testing.T, af *archlint.File) []string {
	t.Helper()
	f, fset, err := af.AST()
	if err != nil {
		t.Fatalf("parse %s: %v", af.Rel, err)
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
			hits = append(hits, af.Rel+":"+strconv.Itoa(pos.Line))
		}
		return true
	})
	return hits
}
