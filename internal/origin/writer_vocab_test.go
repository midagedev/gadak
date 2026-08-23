package origin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

// TestWriterInterfaceOmitsJiraTypes is the GDK-665 vocabulary lock: origin.Writer
// must not name internal/jira types in its signatures. Adapters produce origin
// DTOs; jira stays the HTTP payload package.
//
// FAIL-first on the pre-change writer.go (measured 2026-08-23): eight
// jira. selectors on the interface (CreateMetaProject, FieldMeta, Transition,
// CommentVisibility, Comment, User, NamedID, Attachment).
func TestWriterInterfaceOmitsJiraTypes(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src := filepath.Join(filepath.Dir(thisFile), "writer.go")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		t.Fatalf("parse writer.go: %v", err)
	}

	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name == nil || ts.Name.Name != "Writer" {
			return true
		}
		iface, ok := ts.Type.(*ast.InterfaceType)
		if !ok || iface.Methods == nil {
			return false
		}
		for _, m := range iface.Methods.List {
			ast.Inspect(m, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok || id.Name != "jira" {
					return true
				}
				pos := fset.Position(sel.Pos())
				name := ""
				if len(m.Names) > 0 {
					name = m.Names[0].Name
				}
				hits = append(hits, filepath.Base(pos.Filename)+":"+itoa(pos.Line)+" "+name+" jira."+sel.Sel.Name)
				return true
			})
		}
		return false
	})
	if len(hits) > 0 {
		t.Fatalf("origin.Writer signatures must not name jira types:\n  %s", joinLines(hits))
	}
}

func TestJiraWriterIsNotBareClient(t *testing.T) {
	w := newJiraWriter(nil)
	var wr Writer = w
	if _, ok := wr.(*jira.Client); ok {
		t.Fatal("jiraWriter must not be a *jira.Client")
	}
}

func joinLines(s []string) string {
	out := ""
	for i, line := range s {
		if i > 0 {
			out += "\n  "
		}
		out += line
	}
	return out
}
