package apprun

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
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

func standaloneRuntime(t *testing.T) (*Runtime, *config.Config) {
	t.Helper()
	testHome(t)
	cfg := saveStandalone(t)
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

func TestStartOriginPassthroughAdvertisesAndProbes(t *testing.T) {
	rt, cfg := standaloneRuntime(t)
	origin.SetInProcess(true)
	stop, err := StartOriginPassthrough(cfg, rt.API)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	advPath := origin.AdvertisePath(cfg.Directory())
	data, err := os.ReadFile(advPath)
	if err != nil {
		t.Fatalf("FAIL-first GDK-340: advertise file missing: %v", err)
	}
	var adv origin.Advertise
	if err := json.Unmarshal(data, &adv); err != nil {
		t.Fatal(err)
	}
	if adv.Addr == "" || adv.PID != os.Getpid() {
		t.Fatalf("advertise doc %+v", adv)
	}

	res, err := http.Get("http://" + adv.Addr + origin.ProbePath)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	res.Body.Close()
	if res.Header.Get("X-Gadak") != "1" {
		t.Fatalf("probe response lacks X-Gadak (status %d)", res.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, "http://"+adv.Addr+origin.RESTPrefix+"/rest/api/3/myself", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("standalone:standalone")))
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("passthrough /rest/api/3/myself = %d", res.StatusCode)
	}

	stop()
	if _, err := os.Stat(advPath); !os.IsNotExist(err) {
		t.Fatal("advertise file still present after stop")
	}
}

func TestStartOriginPassthroughServesOnlyOriginPaths(t *testing.T) {
	rt, cfg := standaloneRuntime(t)
	origin.SetInProcess(true)
	stop, err := StartOriginPassthrough(cfg, rt.API)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	data, err := os.ReadFile(origin.AdvertisePath(cfg.Directory()))
	if err != nil {
		t.Fatal(err)
	}
	var adv origin.Advertise
	if err := json.Unmarshal(data, &adv); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"/",
		"/api/v1/bootstrap/",
		"/healthz",
		"/api/v1/issues/sync/progress/deeper",
	} {
		res, err := http.Get("http://" + adv.Addr + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", p, res.StatusCode)
		}
	}
}

func TestStartOriginPassthroughConnectedNoListener(t *testing.T) {
	testHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	stop, err := StartOriginPassthrough(cfg, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := os.Stat(origin.AdvertisePath(os.Getenv("GADAK_HOME"))); !os.IsNotExist(err) {
		t.Fatal("connected workspace wrote advertise")
	}
}

// TestSecondProcessStandaloneListenerBusy is Q1 as a real second process:
// with the parent holding the persist lock, StartOriginPassthrough must
// return ErrWorkspaceBusy and must not write advertise.
func TestSecondProcessStandaloneListenerBusy(t *testing.T) {
	if os.Getenv("GDK658_APPRUN_CHILD") == "1" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			os.Exit(1)
		}
		_, err = StartOriginPassthrough(cfg, http.NotFoundHandler())
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

	rt, cfg := standaloneRuntime(t)
	origin.SetInProcess(true)
	stop, err := StartOriginPassthrough(cfg, rt.API)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	advPath := origin.AdvertisePath(cfg.Directory())
	before, err := os.ReadFile(advPath)
	if err != nil {
		t.Fatalf("parent advertise: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestSecondProcessStandaloneListenerBusy$", "-test.v=false")
	cmd.Env = append(os.Environ(), "GDK658_APPRUN_CHILD=1")
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
