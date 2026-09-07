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

// TestNoDirectJiraNewOutsideOrigin is the structural lock: "this workspace's
// Jira client" is built in this package only. A new jira.New( in production
// code (outside internal/jira and this package) fails this test.
//
// Tests (*_test.go) may still call jira.New to stand up httptest servers.
func TestNoDirectJiraNewOutsideOrigin(t *testing.T) {
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
		if allowedJiraNewFile(af.Rel) {
			return nil
		}
		hits = append(hits, findJiraNewCalls(t, af)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("jira.New must not be called from production code outside internal/origin (and its definition in internal/jira):\n  %s",
			strings.Join(hits, "\n  "))
	}
}

func allowedJiraNewFile(rel string) bool {
	if strings.HasPrefix(rel, "internal/origin/") {
		return true
	}
	// The constructor itself.
	if rel == "internal/jira/client.go" {
		return true
	}
	return false
}

func findJiraNewCalls(t *testing.T, af *archlint.File) []string {
	t.Helper()
	f, fset, err := af.AST()
	if err != nil {
		t.Fatalf("parse %s: %v", af.Rel, err)
		return nil
	}
	var jiraName string
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p != "github.com/midagedev/gadak/internal/jira" {
			continue
		}
		if imp.Name != nil {
			jiraName = imp.Name.Name
		} else {
			jiraName = "jira"
		}
	}
	if jiraName == "" || jiraName == "_" {
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
		if !ok || id.Name != jiraName {
			return true
		}
		pos := fset.Position(call.Pos())
		hits = append(hits, filepath.ToSlash(af.Rel)+":"+strconv.Itoa(pos.Line))
		return true
	})
	return hits
}
