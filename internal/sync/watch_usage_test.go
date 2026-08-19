package sync

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/atlhttp"
	"github.com/midagedev/gadak/internal/linear"
)

// originUsageStructDirs is every internal/ directory allowed to declare
// `type Usage struct`. There is one owner: the host-neutral HTTP policy
// package both connectors import. Type aliases (`type Usage = httppolicy.Usage`)
// are not declarations and must not appear here.
//
// The list defends itself the same way originClientAuthSentinels does:
// originUsageStructDeclDirs walks internal/ and this test fails both ways —
// a directory that declares a Usage struct without a row, and a row with no
// declaration behind it. A hand-maintained field-name list is not used;
// field identity is reflection over the two public Usage types.
var originUsageStructDirs = []string{"httppolicy"}

// Compile-time unification test: usageTaker is TakeUsage() atlhttp.Usage.
// A distinct linear.Usage (even with the same fields) would not assign.
var _ usageTaker = (*linear.Client)(nil)

func TestUsageStructHasOneOwner(t *testing.T) {
	known := map[string]bool{}
	for _, dir := range originUsageStructDirs {
		known[dir] = true
	}
	found := originUsageStructDeclDirs(t)
	for dir := range found {
		if !known[dir] {
			t.Errorf("internal/%s declares type Usage struct but is not in originUsageStructDirs — a second copy will drift (WaitMS / LastThrottledAt already did)", dir)
		}
	}
	for dir := range known {
		if !found[dir] {
			t.Errorf("originUsageStructDirs lists %s but no type Usage struct was found under internal/%s", dir, dir)
		}
	}
}

func TestUsageTypesIdentical(t *testing.T) {
	// Unification test: the two public Usage types must be the same named
	// type (aliases of httppolicy.Usage). Same field names on two structs
	// is not enough — usageTaker is TakeUsage() atlhttp.Usage, and a
	// conversion would still be required.
	a := reflect.TypeOf(atlhttp.Usage{})
	b := reflect.TypeOf(linear.Usage{})
	if a != b {
		t.Errorf("atlhttp.Usage is %s, linear.Usage is %s — they must be one type so *linear.Client satisfies usageTaker without conversion", a.String(), b.String())
	}
	dumpUsageFields(t, "atlhttp", a)
	dumpUsageFields(t, "linear", b)
	if a.NumField() != b.NumField() {
		t.Errorf("field count atlhttp=%d linear=%d", a.NumField(), b.NumField())
	}
	n := a.NumField()
	if b.NumField() < n {
		n = b.NumField()
	}
	for i := 0; i < n; i++ {
		fa, fb := a.Field(i), b.Field(i)
		if fa.Name != fb.Name || fa.Type != fb.Type {
			t.Errorf("field[%d] atlhttp=%s %s linear=%s %s", i, fa.Name, fa.Type, fb.Name, fb.Type)
		}
	}
}

func TestLinearTakeUsageReturnsSharedUsage(t *testing.T) {
	m, ok := reflect.TypeOf((*linear.Client)(nil)).MethodByName("TakeUsage")
	if !ok {
		t.Fatal("linear.Client has no TakeUsage")
	}
	if m.Type.NumOut() != 1 {
		t.Fatalf("TakeUsage outs = %d, want 1", m.Type.NumOut())
	}
	got := m.Type.Out(0)
	want := reflect.TypeOf(atlhttp.Usage{})
	if got != want {
		t.Errorf("linear.Client.TakeUsage returns %s, want %s (usageTaker is TakeUsage() atlhttp.Usage)", got, want)
	}
}

func TestLinearSourcesDoNotImportAtlhttp(t *testing.T) {
	// The audit's side: Linear has one URL, so atlhttp's host-pinning does
	// not apply. Sharing retry/usage must not drag that package in.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "linear"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	needle := `"github.com/midagedev/gadak/internal/atlhttp"`
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), needle) {
			t.Errorf("internal/linear/%s imports atlhttp — retry/usage belong in httppolicy; path joining stays in atlhttp", e.Name())
		}
	}
}

func dumpUsageFields(t *testing.T, label string, typ reflect.Type) {
	t.Helper()
	var parts []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		parts = append(parts, f.Name+" "+f.Type.String())
	}
	t.Logf("%s.Usage fields: %s", label, strings.Join(parts, ", "))
}

func originUsageStructDeclDirs(t *testing.T) map[string]bool {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	internal := filepath.Join(root, "internal")
	found := map[string]bool{}
	err := filepath.WalkDir(internal, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "dist", "testdata", "scratch":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !srcDeclaresUsageStruct(string(data)) {
			return nil
		}
		rel, err := filepath.Rel(internal, filepath.Dir(path))
		if err != nil {
			return err
		}
		found[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func srcDeclaresUsageStruct(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "//") {
			continue
		}
		if strings.HasPrefix(s, "type Usage struct") {
			return true
		}
		// type ( Usage struct { ... } ) block form
		if s == "Usage struct {" || strings.HasPrefix(s, "Usage struct{") {
			return true
		}
	}
	return false
}
