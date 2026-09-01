package apprun

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
)

func localOriginRuntime(t *testing.T) (*Runtime, *config.Config) {
	t.Helper()
	testHome(t)
	cfg := saveLocalOrigin(t)
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	api := server.New(db, cfg)
	t.Cleanup(func() { _ = api.Close() })
	rt := &Runtime{Cfg: cfg, DB: db, API: api, log: func(string) {}}
	return rt, cfg
}

func TestStartOriginPassthroughEmbedsWithoutAdvertise(t *testing.T) {
	_, cfg := localOriginRuntime(t)
	stop, err := StartOriginPassthrough(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	if _, err := os.Stat(filepath.Join(cfg.Directory(), "serve-origin.json")); !os.IsNotExist(err) {
		t.Fatal("StartOriginPassthrough must not write serve-origin.json")
	}
	if !origin.IsInProcess(cfg) {
		t.Fatal("local-origin StartOriginPassthrough must mark in-process")
	}
	c, err := origin.Client(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !origin.TransportIsEmbedded(c.HTTP.Transport) {
		t.Fatalf("transport %T, want embedded", c.HTTP.Transport)
	}
}

func TestStartOriginPassthroughConnectedNoListener(t *testing.T) {
	testHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	stop, err := StartOriginPassthrough(cfg)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := os.Stat(filepath.Join(os.Getenv("GADAK_HOME"), "serve-origin.json")); !os.IsNotExist(err) {
		t.Fatal("connected workspace wrote serve-origin.json")
	}
}

// TestSecondProcessLocalOriginEmbeds is Q1 after GDK-936: a second process
// may embed the same WAL persist. The parent holds a session; the child
// must acquire, not fail busy.
func TestSecondProcessLocalOriginEmbeds(t *testing.T) {
	if os.Getenv("GDK658_APPRUN_CHILD") == "1" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(1)
		}
		_, err = StartOriginPassthrough(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "other: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "GDK658_ACQUIRED")
		os.Exit(0)
	}

	_, cfg := localOriginRuntime(t)
	stop, err := StartOriginPassthrough(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	cmd := exec.Command(os.Args[0], "-test.run=^TestSecondProcessLocalOriginEmbeds$", "-test.v=false")
	cmd.Env = append(os.Environ(), "GDK658_APPRUN_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed to embed: err=%v out=%s", err, out)
	}
	if !strings.Contains(string(out), "GDK658_ACQUIRED") {
		t.Fatalf("child want GDK658_ACQUIRED, err=%v out=%s", err, out)
	}
}

// TestDesktopRunStartsOriginAfterApplicationNew is the GDK-658 contract
// on the desktop caller: StartOriginPassthrough must run after
// application.New inside run(). wails os.Exits a second instance inside
// New() without running defers, so persist taken first is abandoned.
//
// FAIL-first: with the call still above New() this fails at the index check.
func TestDesktopRunStartsOriginAfterApplicationNew(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "desktop", "main.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
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
		t.Fatal("func run() not found in desktop/main.go")
	}
	appAt, standAt := -1, -1
	i := 0
	ast.Inspect(run.Body, func(n ast.Node) bool {
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
		name := exprCallName(call.Fun)
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
		if name != "" {
			i++
		}
		return true
	})
	if appAt < 0 {
		t.Fatal("application.New not called from run()")
	}
	if standAt < 0 {
		t.Fatal("StartOriginPassthrough not called from run()")
	}
	if standAt < appAt {
		t.Fatalf("GDK-658: StartOriginPassthrough (call %d) is before application.New (call %d) in run()", standAt, appAt)
	}
}
