package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestGDK658StandaloneOriginAfterApplicationNew is the GDK-658 contract:
// startStandaloneOriginListener must run after application.New inside run().
// wails os.Exits a second instance inside New() without running defers
// (v3/pkg/application/application.go alreadyRunningError path), so persist
// taken first is abandoned — and a second launch dies on ErrWorkspaceBusy
// instead of handing off to the first window.
//
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
		switch name {
		case "application.New":
			if appAt < 0 {
				appAt = i
			}
		case "startStandaloneOriginListener":
			if standAt < 0 {
				standAt = i
			}
		}
	}
	if appAt < 0 {
		t.Fatal("application.New not called from run()")
	}
	if standAt < 0 {
		t.Fatal("startStandaloneOriginListener not called from run()")
	}
	if standAt < appAt {
		t.Fatalf("GDK-658: startStandaloneOriginListener (call %d) is before application.New (call %d) in run() — second instance takes persist then os.Exit skips defers; sequence=%v",
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

// TestGDK658SecondProcessStandaloneListenerBusy is Q1 as a real second
// process: with the parent holding the persist lock, startStandaloneOriginListener
// must return ErrWorkspaceBusy and must not write advertise. This is why a
// second desktop launch never reached application.New before GDK-658.
func TestGDK658SecondProcessStandaloneListenerBusy(t *testing.T) {
	if os.Getenv("GDK658_CHILD") == "1" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(1)
		}
		_, err = startStandaloneOriginListener(cfg, http.NotFoundHandler())
		if err == nil {
			fmt.Fprintln(os.Stderr, "GDK658_ACQUIRED")
			os.Exit(0)
		}
		if errors.Is(err, origin.ErrWorkspaceBusy) {
			fmt.Fprintln(os.Stderr, "GDK658_BUSY")
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "other: %v\n", err)
		os.Exit(1)
	}

	cfg, api := standaloneApp(t)
	origin.SetInProcess(true)
	stop, err := startStandaloneOriginListener(cfg, api)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	advPath := origin.AdvertisePath(cfg.Directory())
	before, err := os.ReadFile(advPath)
	if err != nil {
		t.Fatalf("parent advertise: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestGDK658SecondProcessStandaloneListenerBusy$", "-test.v=false")
	cmd.Env = append(os.Environ(), "GDK658_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child acquired standalone origin while parent held the lock; out=%s", out)
	}
	if !strings.Contains(string(out), "GDK658_BUSY") {
		t.Fatalf("child want GDK658_BUSY, err=%v out=%s", err, out)
	}

	after, err := os.ReadFile(advPath)
	if err != nil {
		t.Fatalf("advertise missing after child: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("child overwrote advertise:\n before=%s\n after=%s", before, after)
	}
}
