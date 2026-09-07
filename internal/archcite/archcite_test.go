package archcite

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCitations(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{"symbol citation in parens", "The session rule is retro's (internal/retro/retro.go SessionGap):", []string{"internal/retro/retro.go"}},
		{"multi-dot extension", "lockstep with web/src/lib/done-words.test.ts)", []string{"web/src/lib/done-words.test.ts"}},
		{"trailing line number stops the path", "--open flag (cmd/gadak/retro.go:23 pre-move)", []string{"cmd/gadak/retro.go"}},
		{"extensionless web path", "imports web/src/lib/view-config directly", []string{"web/src/lib/view-config"}},
		{"dead path is still extracted — the gate flags it", "see internal/nope/missing.go for why", []string{"internal/nope/missing.go"}},
		{"import path: slash before root blocks", `"github.com/midagedev/gadak/internal/jql"`, nil},
		{"word ending in cmd does not match", "recmd/gadak internal/store", []string{"internal/store"}},
		{"module path before root blocks", "v3/internal/webview2 errors", nil},
		{"root alone", "in internal/store", []string{"internal/store"}},
	}
	for _, tc := range cases {
		got := Citations(tc.text, 1)
		var paths []string
		for _, c := range got {
			paths = append(paths, c.Path)
		}
		if strings.Join(paths, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: Citations(%q) = %v; want %v", tc.name, tc.text, paths, tc.want)
		}
	}
}

func TestCitationsLineNumbers(t *testing.T) {
	text := "first line cites internal/retro/retro.go\nsecond line cites nothing\nthird cites internal/store/flow.go"
	got := Citations(text, 10)
	if len(got) != 2 {
		t.Fatalf("got %d citations; want 2", len(got))
	}
	if got[0].Path != "internal/retro/retro.go" || got[0].Line != 10 {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Path != "internal/store/flow.go" || got[1].Line != 12 {
		t.Errorf("second: %+v", got[1])
	}
}

// TestCitedExists exercises the gate's resolution rules against a synthetic
// tree, including the rows the gate flags: a file citation whose file is not
// there is dead no matter what directory the trailing dot-stripping would
// find, and a symbol citation needs its package directory to exist.
func TestCitedExists(t *testing.T) {
	root := t.TempDir()
	mkdir := func(p string) {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(p)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	touch := func(p string) {
		mkdir(filepath.Dir(p))
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(p)), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("internal/pairing")
	mkdir("web/src/lib")
	touch("internal/retro/retro.go")
	touch("web/src/lib/view-config.ts")

	cases := []struct {
		cite string
		want bool
	}{
		{"internal/retro/retro.go", true},
		{"internal/retro/parse.go", false}, // dead file — the GDK-1569 class
		{"internal/pairing.Offer", true},   // pkg.Symbol: dir exists
		{"internal/pairing.Gone", true},    // symbol existence is out of scope: the package is there
		{"web/src/lib/view-config", true},  // extensionless: +.ts exists
		{"web/src/lib/absent", false},      // no +.ts/.js/.svelte
		{"internal/jql/jql.go", false},     // .go means file: never symbol-stripped
		{"internal/retro", true},           // directory citation
	}
	for _, tc := range cases {
		if got := citedExists(root, tc.cite); got != tc.want {
			t.Errorf("citedExists(%q) = %v; want %v", tc.cite, got, tc.want)
		}
	}
}

// fileExts are the extensions a citation's last segment is read as a file
// by. Anything else with a dot is a Go symbol tail (internal/pairing.Offer)
// whose base should be a package directory.
var fileExts = map[string]bool{
	"go": true, "ts": true, "js": true, "svelte": true,
	"md": true, "json": true, "sql": true,
}

// citedExists reports whether cite names something in the tree at root:
// the path as written, a package directory plus symbol, or an extensionless
// web import with a .ts/.js/.svelte twin.
func citedExists(root, cite string) bool {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(cite))); err == nil {
		return true
	}
	last := cite[strings.LastIndex(cite, "/")+1:]
	dot := strings.LastIndex(last, ".")
	if dot > 0 {
		if fileExts[last[dot+1:]] {
			return false // a file citation; the file is not there
		}
		base := cite[:len(cite)-len(last)+dot]
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(base)))
		return err == nil
	}
	for _, ext := range []string{".ts", ".js", ".svelte"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(cite+ext))); err == nil {
			return true
		}
	}
	return false
}

// crossRepo holds citations of another repository's internal/ tree. These
// are real pointers — issuetap's and wails' own files — that this tree's
// existence check cannot see. New entries need the same one-line reason;
// anything else must exist here or the gate is red.
var crossRepo = map[string]string{
	"internal/fixtures.Doc":                       "issuetap's package (internal/migrate cites issuetap's YAML contract)",
	"internal/jql/jql.go":                         "issuetap's file (internal/sync watermark test cites issuetap's jql.go; gadak's internal/jql has no jql.go)",
	"internal/webview2":                           "wails/v3's package (desktop/main.go: unexported wails errors)",
	"internal/assetserver/assetserver_webview.go": "wails/v3's file (desktop/main.go: the synthetic TEST-NET peer)",
}

// TestCitedRepoPathsExist is the gate: every code-comment citation of a
// cmd/, internal/ or web/src/ path names a path that exists in this tree.
// Go comments are read through go/parser, so strings never match; web,
// e2e and mobile sources are read line-by-line, a comment being a line
// whose trimmed form starts with //, * or /*.
//
// FAIL-first note, stated rather than shown: this gate's red state is the
// renamed-or-deleted-file class, which the tree currently has none of —
// the four dead citations of GDK-1569 pointed at files that still exist
// and were fixed as part of that issue. TestCitedExists's false rows are
// the demonstration that the checker flags the class it claims to.
func TestCitedRepoPathsExist(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: no file")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // internal/archcite → repo root

	skip := map[string]bool{
		".git": true, ".claude": true, ".tmp": true, "vendor": true,
		"node_modules": true, "dist": true, "build": true, "testdata": true,
		"scratch": true, "coverage": true, "playwright-report": true, "test-results": true,
	}
	var missing []string
	visit := func(path string, line int, text string) {
		for _, c := range Citations(text, line) {
			if citedExists(root, c.Path) {
				continue
			}
			if _, ok := crossRepo[c.Path]; ok {
				continue
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				rel = path
			}
			missing = append(missing, fmt.Sprintf("%s:%d: cites %s — %s", filepath.ToSlash(rel), c.Line, c.Path, c.Text))
		}
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		switch {
		case strings.HasSuffix(name, ".go"):
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			for _, group := range f.Comments {
				for _, cm := range group.List {
					visit(path, fset.Position(cm.Pos()).Line, cm.Text)
				}
			}
		case strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".svelte"):
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, l := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(l)
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
					visit(path, i+1, l)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("dead repo-path citations in code comments (fix the comment, or add a crossRepo entry with a reason):\n%s",
			strings.Join(missing, "\n"))
	}
}
