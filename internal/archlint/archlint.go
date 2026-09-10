// Package archlint owns the tree walk shared by the architecture-lint gate
// tests. Seven gates used to carry private copies of the same WalkDir loop,
// and the copies drifted: six skipped {".git", "vendor", "node_modules",
// "dist", "testdata", "scratch", ".claude"} while the store gate skipped
// every dot-prefixed directory plus "examples" and kept scanning testdata.
// This package is the union of those lists, as one table:
//
//	skip (directory name)   why
//	----------------------------------------------------------------------------
//	any "."-prefixed name   .git, .claude (agent worktrees), e2e/.tmp
//	vendor, node_modules    dependency trees
//	dist                    build output
//	testdata                per-package fixtures
//	scratch                 gitignored working notes
//	examples                shipped fixtures (demo.db and friends), never code
//	"test-results" prefix   Playwright artifacts, wiped and recreated by
//	                        parallel e2e runs — racing a wipe is the GDK-1486
//	                        flake; no .go file ever lives there anyway
//
// Rules stay in their own packages, next to the code they lock; this package
// owns only the walk, the skip list, slash-relative paths, parsing, and the
// in-process parse cache. The cache lives inside one test binary: `go test
// ./...` runs each package as its own process, so only gate pairs inside one
// package (internal/origin, internal/sync) share it — there is no
// cross-package caching.
package archlint

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// File is one .go file discovered by Walk. AST and Src are lazy, so a rule
// that filters files out by suffix or prefix never pays for what it skips.
type File struct {
	Path string // filesystem path as walked (joined onto Walk's root)
	Rel  string // slash-separated path relative to Walk's root
}

// parseCache memoizes AST() by cleaned path for the lifetime of one test
// binary, so a second Walk in the same process does not reparse.
var parseCache sync.Map // string -> *parseResult

type parseResult struct {
	fset *token.FileSet
	file *ast.File
	err  error
}

// AST parses the file (parser mode 0) or returns the cached parse from an
// earlier Walk in the same process. Build-constrained files (//go:build
// ignore) are delivered and parsed like any other file: the gates are
// source scanners, not build participants.
func (f *File) AST() (*ast.File, *token.FileSet, error) {
	key := filepath.Clean(f.Path)
	if v, ok := parseCache.Load(key); ok {
		r := v.(*parseResult)
		return r.file, r.fset, r.err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, f.Path, nil, 0)
	parseCache.Store(key, &parseResult{fset: fset, file: file, err: err})
	return file, fset, err
}

// Src reads the raw file bytes, for rules that match on text instead of the
// syntax tree.
func (f *File) Src() ([]byte, error) {
	return os.ReadFile(f.Path)
}

// Walk visits every .go file under root that survives the shared skip list,
// in filepath.WalkDir order (lexical within each directory). Production and
// test files are both delivered; each rule filters by suffix and prefix
// itself. Rel is slash-separated on every platform. An error returned by
// visit aborts the walk and is returned unchanged, as are walk errors —
// except ENOENT: a directory another process removed mid-walk (a parallel
// Playwright run wiping test-results/, GDK-1486) skips quietly instead of
// failing the whole gate over a path that never held source.
func Walk(root string, visit func(f *File) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return visit(&File{Path: path, Rel: filepath.ToSlash(rel)})
	})
}

// skipDir is the union table from the package comment. Dot-prefixed names
// come first because two of the drifted copies disagreed on which dot
// directories (.git/.claude only vs. all) — the wider rule is the union.
func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if strings.HasPrefix(name, "test-results") {
		return true
	}
	switch name {
	case "vendor", "node_modules", "dist", "testdata", "scratch", "examples":
		return true
	}
	return false
}
