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

// TestWriterInterfaceOmitsJiraTypes is the GDK-665 vocabulary lock: no
// interface in writer.go — Writer or the optional faces — may name
// internal/jira types in its signatures. Adapters produce origin DTOs; jira
// stays the HTTP payload package.
//
// FAIL-first history: the original lock inspected only `type Writer
// interface` and went green while the optional faces named jira (GDK-687,
// measured 2026-08-24: VersionCatalog.ProjectVersions []jira.Version,
// IssueLinker.IssueLinkTypes []jira.IssueLinkType,
// CreateFieldCatalog.CreateFields []jira.CreateFieldMeta).
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
		if !ok || ts.Name == nil {
			return true
		}
		iface, ok := ts.Type.(*ast.InterfaceType)
		if !ok || iface.Methods == nil {
			return true
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
				hits = append(hits, filepath.Base(pos.Filename)+":"+itoa(pos.Line)+" "+ts.Name.Name+"."+name+" jira."+sel.Sel.Name)
				return true
			})
		}
		return true
	})
	if len(hits) > 0 {
		t.Fatalf("origin interface signatures must not name jira types:\n  %s", joinLines(hits))
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
