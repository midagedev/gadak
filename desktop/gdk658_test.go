package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/apprun"
	"github.com/midagedev/gadak/internal/config"
)

// TestGDK658StandaloneOriginAfterApplicationNew is the GDK-658 contract:
// StartOriginPassthrough must run after application.New inside run().
// wails os.Exits a second instance inside New() without running defers
// (v3/pkg/application/application.go alreadyRunningError path), so persist
// taken first is abandoned. A second GUI instance is still SingleInstance
// handoff; WAL allows a second process to embed the same persist.
//
// The sequence owner is internal/apprun; this file pins the desktop caller.
// FAIL-first: with the call still above New() this fails at the index check.
func TestGDK658StandaloneOriginAfterApplicationNew(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var run *ast.FuncDecl
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "run" || fn.Recv != nil {
			continue
		}
		run = fn
		break
	}
	if run == nil || run.Body == nil {
		t.Fatal("func run() not found in main.go")
	}
	calls := topLevelCallNames(run.Body)
	appAt, standAt := -1, -1
	for i, name := range calls {
		switch {
		case name == "application.New":
			if appAt < 0 {
				appAt = i
			}
		case strings.HasSuffix(name, "StartOriginPassthrough"):
			if standAt < 0 {
				standAt = i
			}
		}
	}
	if appAt < 0 {
		t.Fatal("application.New not called from run()")
	}
	if standAt < 0 {
		t.Fatal("StartOriginPassthrough not called from run()")
	}
	if standAt < appAt {
		t.Fatalf("GDK-658: StartOriginPassthrough (call %d) is before application.New (call %d) in run() — second instance takes persist then os.Exit skips defers; sequence=%v",
			standAt, appAt, calls)
	}
}

func topLevelCallNames(body *ast.BlockStmt) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name := exprCallName(call.Fun); name != "" {
			names = append(names, name)
		}
		return true
	})
	return names
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

// TestGDK658SecondProcessStandaloneEmbeds is Q1 after GDK-936: a second
// process may embed the same WAL persist. SingleInstance still owns the
// GUI; this test is the origin session, not the window.
func TestGDK658SecondProcessStandaloneEmbeds(t *testing.T) {
	if os.Getenv("GDK658_CHILD") == "1" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(1)
		}
		_, err = apprun.StartOriginPassthrough(cfg, http.NotFoundHandler())
		if err != nil {
			fmt.Fprintf(os.Stderr, "other: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "GDK658_ACQUIRED")
		os.Exit(0)
	}

	cfg, api := standaloneApp(t)
	stop, err := apprun.StartOriginPassthrough(cfg, api)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	cmd := exec.Command(os.Args[0], "-test.run=^TestGDK658SecondProcessStandaloneEmbeds$", "-test.v=false")
	cmd.Env = append(os.Environ(), "GDK658_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed to embed: err=%v out=%s", err, out)
	}
	if !strings.Contains(string(out), "GDK658_ACQUIRED") {
		t.Fatalf("child want GDK658_ACQUIRED, err=%v out=%s", err, out)
	}
}
