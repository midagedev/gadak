//go:build sourcelint

// Repo-wide AST gate (GDK-1144): lives behind the sourcelint tag so the
// default `go test ./...` never pays the whole-tree parse. Run with
// `bash tools/sourcelint.sh` — CI's Go-tests step runs exactly that.

package sync

import (
	"go/ast"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

// TestRefreshIssueOwnerIsUnique is the structural lock: the write-through
// tail (origin.Linear then SyncLinearIssue in one function) lives only in
// RefreshIssue. A third copy in CLI or REST is how the two surfaces drifted
// before GDK-642.
//
// Tests (*_test.go) may still call both to stand up fixtures.
func TestRefreshIssueOwnerIsUnique(t *testing.T) {
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
		if allowedRefreshOwnerFile(af.Rel) {
			return nil
		}
		hits = append(hits, findWriteThroughTailCopies(t, af)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("write-through tail (origin.Linear + SyncLinearIssue) must live only in RefreshIssue:\n  %s",
			strings.Join(hits, "\n  "))
	}
}

func allowedRefreshOwnerFile(rel string) bool {
	return rel == "internal/sync/refresh.go"
}

func findWriteThroughTailCopies(t *testing.T, af *archlint.File) []string {
	t.Helper()
	f, fset, err := af.AST()
	if err != nil {
		t.Fatalf("parse %s: %v", af.Rel, err)
		return nil
	}
	originName, syncName := importNames(f)
	var hits []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if !funcCallsWriteThroughTail(fn, originName, syncName) {
			continue
		}
		pos := fset.Position(fn.Pos())
		name := fn.Name.Name
		if fn.Recv != nil {
			name = recvName(fn) + "." + name
		}
		hits = append(hits, filepath.ToSlash(af.Rel)+":"+strconv.Itoa(pos.Line)+" "+name)
	}
	return hits
}

func importNames(f *ast.File) (originName, syncName string) {
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch p {
		case "github.com/midagedev/gadak/internal/origin":
			if name == "" {
				name = "origin"
			}
			if name != "_" {
				originName = name
			}
		case "github.com/midagedev/gadak/internal/sync":
			if name == "" {
				name = "sync"
			}
			if name != "_" {
				syncName = name
			}
		}
	}
	return originName, syncName
}

func funcCallsWriteThroughTail(fn *ast.FuncDecl, originName, syncName string) bool {
	var hasLinear, hasSyncLinear bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "SyncLinearIssue" {
				hasSyncLinear = true
			}
			if fun.Name == "Linear" {
				hasLinear = true
			}
		case *ast.SelectorExpr:
			id, ok := fun.X.(*ast.Ident)
			if !ok || fun.Sel == nil {
				return true
			}
			if originName != "" && id.Name == originName && fun.Sel.Name == "Linear" {
				hasLinear = true
			}
			if syncName != "" && id.Name == syncName && fun.Sel.Name == "SyncLinearIssue" {
				hasSyncLinear = true
			}
		}
		return true
	})
	return hasLinear && hasSyncLinear
}

func recvName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch t := fn.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	}
	return ""
}
