package archlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWalkSkipsUnionList builds a fake tree shaped like the drift incident:
// a .claude/worktrees copy of the repo, plus every other skip name, must
// never reach the rules, while production and test .go files both do.
func TestWalkSkipsUnionList(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"a.go",
		"sub/b.go",
		"sub/c_test.go", // test files are delivered; rules filter
		"ignored.go",    // //go:build-constrained files are source too
		".claude/worktrees/agent-x/copy.go",
		".git/hooks/g.go",
		".hidden/h.go",
		"e2e/.tmp/served/g.go",
		"vendor/v.go",
		"node_modules/n.go",
		"dist/d.go",
		"testdata/td.go",
		"scratch/s.go",
		"examples/demo.go",
		"e2e/spec.ts", // not Go
	}
	for _, rel := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "package p\n"
		if rel == "ignored.go" {
			body = "//go:build ignore\n\npackage p\n"
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	err := Walk(root, func(f *File) error {
		got = append(got, f.Rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.go", "ignored.go", "sub/b.go", "sub/c_test.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Walk delivered %v, want %v", got, want)
	}
	for _, rel := range got {
		if rel != filepath.ToSlash(rel) || strings.Contains(rel, "\\") {
			t.Fatalf("Rel must be slash-separated, got %q", rel)
		}
	}
}

// TestFileASTParsesAndCaches locks the in-process parse cache: a second
// Walk in the same test binary must reuse the first parse (pointer
// identity), which is where the gate pairs inside one package get their
// walk savings.
func TestFileASTParsesAndCaches(t *testing.T) {
	root := t.TempDir()
	src := "package p\n\nconst answer = 42\n"
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var visited *File
	if err := Walk(root, func(f *File) error { visited = f; return nil }); err != nil {
		t.Fatal(err)
	}
	file, fset, err := visited.AST()
	if err != nil {
		t.Fatal(err)
	}
	if file == nil || file.Name.Name != "p" || fset == nil {
		t.Fatalf("AST parse got file=%v fset=%v", file, fset)
	}
	got, err := visited.Src()
	if err != nil || string(got) != src {
		t.Fatalf("Src = %q, %v", got, err)
	}
	if err := Walk(root, func(f *File) error { visited = f; return nil }); err != nil {
		t.Fatal(err)
	}
	again, _, err := visited.AST()
	if err != nil {
		t.Fatal(err)
	}
	if again != file {
		t.Fatal("second AST() in one process must reuse the cached parse")
	}
}
