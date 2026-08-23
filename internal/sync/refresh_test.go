package sync

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/store"
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
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "dist", "testdata", "scratch", ".claude":
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
		rel = filepath.ToSlash(rel)
		if allowedRefreshOwnerFile(rel) {
			return nil
		}
		hits = append(hits, findWriteThroughTailCopies(t, path, rel)...)
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

func TestRefreshIssueRoutesLinear(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	var db *store.DB
	lc := &linear.Client{}
	var gotCfg *config.Config
	var gotDB *store.DB
	var gotKey string
	var gotClient *linear.Client
	var issueN int
	err := refreshIssue(ctx, cfg, db, "LIN-1", LinearSourceID, refreshPaths{
		linear: func(c *config.Config) (*linear.Client, error) {
			gotCfg = c
			return lc, nil
		},
		syncLinear: func(_ context.Context, d *store.DB, c *linear.Client, key string) error {
			gotDB = d
			gotClient = c
			gotKey = key
			return nil
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			issueN++
			return errors.New("jira path must not run")
		},
	})
	if err != nil {
		t.Fatalf("linear path: %v", err)
	}
	if gotCfg != cfg {
		t.Fatal("linear factory must receive the same cfg")
	}
	if gotDB != db || gotClient != lc || gotKey != "LIN-1" {
		t.Fatalf("syncLinear db=%v client=%v key=%q", gotDB, gotClient, gotKey)
	}
	if issueN != 0 {
		t.Fatalf("SyncIssue calls %d, want 0", issueN)
	}
}

func TestRefreshIssueRoutesNonLinear(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	var db *store.DB
	for _, src := range []string{"", "jira", "LINEAR"} {
		var gotCfg *config.Config
		var gotDB *store.DB
		var gotKey string
		var gotOpts Options
		var linearN int
		err := refreshIssue(ctx, cfg, db, "NMB-1", src, refreshPaths{
			linear: func(*config.Config) (*linear.Client, error) {
				linearN++
				return nil, errors.New("linear factory must not run")
			},
			syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
				return errors.New("linear path must not run")
			},
			syncIssue: func(_ context.Context, c *config.Config, d *store.DB, key string, opts Options) error {
				gotCfg = c
				gotDB = d
				gotKey = key
				gotOpts = opts
				return nil
			},
		})
		if err != nil {
			t.Fatalf("src %q: %v", src, err)
		}
		if linearN != 0 {
			t.Fatalf("src %q: linear factory calls %d, want 0", src, linearN)
		}
		if gotCfg != cfg || gotDB != db || gotKey != "NMB-1" {
			t.Fatalf("src %q: cfg/db/key mismatch", src)
		}
		if !reflect.DeepEqual(gotOpts, Options{}) {
			t.Fatalf("src %q: Options = %+v, want zero", src, gotOpts)
		}
	}
}

func TestRefreshIssuePassesErrorsThrough(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	want := errors.New("origin: linear is not configured")
	err := refreshIssue(ctx, cfg, nil, "LIN-1", LinearSourceID, refreshPaths{
		linear: func(*config.Config) (*linear.Client, error) {
			return nil, want
		},
		syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
			return errors.New("syncLinear must not run after factory error")
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			return errors.New("jira path must not run")
		},
	})
	if err != want {
		t.Fatalf("linear factory err = %v, want passthrough", err)
	}

	want = errors.New("linear re-read failed")
	err = refreshIssue(ctx, cfg, nil, "LIN-1", LinearSourceID, refreshPaths{
		linear: func(*config.Config) (*linear.Client, error) {
			return &linear.Client{}, nil
		},
		syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
			return want
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			return errors.New("jira path must not run")
		},
	})
	if err != want {
		t.Fatalf("syncLinear err = %v, want passthrough", err)
	}

	want = errors.New("sync: site, email and token are required")
	err = refreshIssue(ctx, cfg, nil, "NMB-1", "", refreshPaths{
		linear: func(*config.Config) (*linear.Client, error) {
			return nil, errors.New("linear factory must not run")
		},
		syncLinear: func(context.Context, *store.DB, *linear.Client, string) error {
			return errors.New("linear path must not run")
		},
		syncIssue: func(context.Context, *config.Config, *store.DB, string, Options) error {
			return want
		},
	})
	if err != want {
		t.Fatalf("syncIssue err = %v, want passthrough", err)
	}
}

func TestRefreshIssuePublicWiresRealPaths(t *testing.T) {
	ctx := context.Background()
	err := RefreshIssue(ctx, &config.Config{}, nil, "LIN-1", LinearSourceID)
	if err == nil || err.Error() != "origin: linear is not configured" {
		t.Fatalf("linear src without credential: %v", err)
	}
	err = RefreshIssue(ctx, &config.Config{}, nil, "NMB-1", "")
	if err == nil || err.Error() != "sync: site, email and token are required" {
		t.Fatalf("jira src without credential: %v", err)
	}
}

func TestRefreshIssuePublicWrapsMirrorStale(t *testing.T) {
	ctx := context.Background()
	err := RefreshIssue(ctx, &config.Config{}, nil, "LIN-1", LinearSourceID)
	if !errors.Is(err, ErrMirrorStale) {
		t.Fatalf("linear factory err must be ErrMirrorStale: %v", err)
	}
	if err.Error() != "origin: linear is not configured" {
		t.Fatalf("Error() must stay the inner sentence: %v", err)
	}
	err = RefreshIssue(ctx, &config.Config{}, nil, "NMB-1", "")
	if !errors.Is(err, ErrMirrorStale) {
		t.Fatalf("syncIssue err must be ErrMirrorStale: %v", err)
	}
	if err.Error() != "sync: site, email and token are required" {
		t.Fatalf("Error() must stay the inner sentence: %v", err)
	}
}

func allowedRefreshOwnerFile(rel string) bool {
	return rel == "internal/sync/refresh.go"
}

func findWriteThroughTailCopies(t *testing.T, path, rel string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
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
		hits = append(hits, filepath.ToSlash(rel)+":"+strconv.Itoa(pos.Line)+" "+name)
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
