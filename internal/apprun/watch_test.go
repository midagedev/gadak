package apprun

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStartWatchUsesWatchLoop is the GDK-663 contract, promoted from the
// two mains: StartWatch must re-enter Watch through syncer.WatchLoop. A
// direct syncer.Watch call returns on fatal auth and leaves the process
// alive with sync permanently stopped.
//
// FAIL-first: with Watch in StartWatch this fails at the direct-call check.
func TestStartWatchUsesWatchLoop(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "watch.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var watch, loop int
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch exprCallName(call.Fun) {
		case "syncer.Watch":
			watch++
		case "syncer.WatchLoop":
			loop++
		}
		return true
	})
	if watch != 0 {
		t.Fatalf("GDK-663: apprun StartWatch calls syncer.Watch %d time(s) — Watch returns on fatal auth and never re-enters; use syncer.WatchLoop", watch)
	}
	if loop == 0 {
		t.Fatal("GDK-663: apprun StartWatch does not call syncer.WatchLoop")
	}
}

// TestMainsEnterWatchThroughApprun: after the move, neither main may call
// syncer.Watch (or WatchLoop) itself. Both must go through StartWatch.
func TestMainsEnterWatchThroughApprun(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("cmd", "gadak", "serve.go"),
		filepath.Join("desktop", "main.go"),
	} {
		path := filepath.Join(root, rel)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body := string(src)
		if strings.Contains(body, "syncer.Watch(") || strings.Contains(body, "syncer.WatchLoop(") {
			t.Fatalf("%s still calls syncer.Watch/WatchLoop — use apprun.StartWatch (GDK-663)", rel)
		}
		if !strings.Contains(body, "StartWatch(") {
			t.Fatalf("%s does not call StartWatch", rel)
		}
	}
}

func TestMainsCallSelectWorkspace(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("cmd", "gadak", "main.go"),
		filepath.Join("desktop", "main.go"),
	} {
		path := filepath.Join(root, rel)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(src), "apprun.SelectWorkspace()") {
			t.Fatalf("%s does not call apprun.SelectWorkspace (GDK-644)", rel)
		}
	}
}

func exprCallName(fun ast.Expr) string {
	switch x := fun.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			return id.Name + "." + x.Sel.Name
		}
	}
	return ""
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// tests run with cwd = this package.
	return filepath.Abs(filepath.Join(wd, "..", ".."))
}
