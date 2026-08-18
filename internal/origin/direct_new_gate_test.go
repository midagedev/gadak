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
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			switch name {
			case ".git", "vendor", "node_modules", "dist", "testdata", "scratch":
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
		// Windows separators would not appear here; still normalize.
		rel = filepath.ToSlash(rel)
		if allowedJiraNewFile(rel) {
			return nil
		}
		hits = append(hits, findJiraNewCalls(t, path, rel)...)
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

func findJiraNewCalls(t *testing.T, path, rel string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
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
		hits = append(hits, filepath.ToSlash(rel)+":"+itoa(pos.Line))
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
