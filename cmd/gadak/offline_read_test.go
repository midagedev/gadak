package main

// Promise 12 in docs/PROMISES.md: a read verb answers from the cache on disk,
// opens no socket, and starts no child process. `tools/check-promises.sh` runs
// TestReadVerbsAnswerWithoutNetwork and TestPromiseReadVerbsAreClassified from
// the fence in that file.
//
// Two axes, because either one alone is decoration:
//
//	(a) no dial — the verb runs in-process with http.DefaultTransport and
//	    net.DefaultResolver replaced by hooks that fail the test and name the
//	    address. Counting `http.NewRequest` in the source instead would miss a
//	    new client package and miss a helper that dials without building a
//	    request.
//	(b) no child — a detached `gadak sync` spawned from a read path has its
//	    own address space and its own transport, so (a) stays green while the
//	    promise is false. spawn.go owns every child in this package and
//	    records argv while armed.
//
// Context, deliberately not taken sides on in the prose: GDK-599 proposes
// letting a read verb kick off a background sync. This test is green while
// that is off and red the moment it is on, which forces the promise text and
// SECURITY.md to be edited in the same commit as the behaviour.

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// coveredReadVerbs is the promise's subject: verbs that must answer from the
// cache with no socket and no child. The value is the argv the block runs
// them with; the fixture is examples/demo.db in a throwaway GADAK_HOME.
//
// The set is not typed from memory — TestPromiseReadVerbsAreClassified derives
// the read verbs from the "Reading the mirror" section of main.go's usage
// string and fails if any of them is missing here and from excludedReadVerbs.
var coveredReadVerbs = map[string][]string{
	"issue":      {"NMB-130"},
	"show":       {"NMB-130"},
	"search":     {"login"},
	"list":       {"--limit", "3"},
	"ready":      {"--limit", "3"},
	"recents":    nil,
	"recent":     nil,
	"retro":      nil,
	"sql":        {"select count(*) from issues"},
	"next":       nil,
	"pick":       nil,
	"recipes":    nil,
	"views":      nil,
	"dashboards": nil,
}

// excludedReadVerbs are the verbs in that section the promise does not cover,
// each with the reason. An entry here is a claim a reader can check, so the
// reason says what the verb does, not that it is inconvenient to test.
//
// `fields` is not in this table because it is not in that section: it reads
// the Jira field catalogue over REST by design (cmd/gadak/fields.go's own
// doc comment, and its not-configured error says "this command queries Jira,
// not only the mirror"). It is a read verb that is honestly not offline.
var excludedReadVerbs = map[string]string{
	"open":     "launches the browser at the origin URL — a child process is the whole point of the verb",
	"snapshot": "takes an output path and writes a database file; reads the cache, but answers no question on stdout",
	"backup":   "copies the built-in origin's persist file to a path you name",
	"export":   "writes your local state to a JSON file you name",
	"import":   "mutates local state from a file",
	"mcp":      "a long-lived server on stdio, not a verb that returns",
	"skill":    "installs a file into an agent host's config directory",
	"raycast":  "installs a Raycast extension, npm and all",
}

// usageReadVerbs returns the verbs listed under the "Reading the mirror"
// heading of the usage string in main.go — the registry-adjacent inventory
// main actually prints. Deriving from it rather than typing a literal list is
// what makes a read verb added later unable to escape the promise quietly:
// the new verb lands in that section, lands in neither table here, and
// TestPromiseReadVerbsAreClassified goes red naming it.
func usageReadVerbs(t *testing.T) []string {
	t.Helper()
	const head = "Reading the mirror (no network; see docs/MIRROR.md):"
	_, after, ok := strings.Cut(usage, head+"\n")
	if !ok {
		t.Fatalf("usage has no %q section — if it was renamed, rename it here and in docs/PROMISES.md promise 12", head)
	}
	entry := regexp.MustCompile(`^  ([a-z][a-z-]*)\s`)
	var verbs []string
	for _, line := range strings.Split(after, "\n") {
		if strings.TrimSpace(line) == "" {
			break // a blank line ends the section
		}
		m := entry.FindStringSubmatch(line)
		if m == nil {
			continue // continuation line (flags, wrapped prose)
		}
		verbs = append(verbs, m[1])
	}
	if len(verbs) < 10 {
		t.Fatalf("parsed only %d verbs from the %q section (%v) — the parser and the usage layout disagree", len(verbs), head, verbs)
	}
	return verbs
}

func TestPromiseReadVerbsAreClassified(t *testing.T) {
	verbs := usageReadVerbs(t)
	for _, v := range verbs {
		if _, ok := commands[v]; !ok {
			t.Errorf("usage lists read verb %q but commands has no such entry", v)
		}
		_, covered := coveredReadVerbs[v]
		reason, excluded := excludedReadVerbs[v]
		switch {
		case covered && excluded:
			t.Errorf("read verb %q is both covered by promise 12 and excluded from it (%s)", v, reason)
		case !covered && !excluded:
			t.Errorf("read verb %q is new to the usage section and classified nowhere. "+
				"Promise 12 in docs/PROMISES.md says read verbs open no socket: either add %q to "+
				"coveredReadVerbs with the args to run it, or to excludedReadVerbs with the reason it is not offline.", v, v)
		}
	}
	for v := range coveredReadVerbs {
		if !slices.Contains(verbs, v) {
			t.Errorf("coveredReadVerbs has %q, which the usage read section no longer lists — stale entry", v)
		}
	}
	for v := range excludedReadVerbs {
		if !slices.Contains(verbs, v) {
			t.Errorf("excludedReadVerbs has %q, which the usage read section no longer lists — stale entry", v)
		}
	}
}

// netLedger records every address the process tried to reach while armed.
type netLedger struct {
	mu   sync.Mutex
	hits []string
}

func (l *netLedger) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = append(l.hits, s)
}

func (l *netLedger) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string{}, l.hits...)
}

type refusingTransport struct{ led *netLedger }

func (r refusingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.led.add("http " + req.Method + " " + req.URL.String())
	return nil, errors.New("promise 12: a read verb must not dial")
}

// armNoNetwork replaces the two process-wide paths every outbound call in this
// module reaches: http.DefaultTransport (no client in the tree installs its
// own — TestOutboundHTTPHasNoPrivateTransport pins that) and the default
// resolver, so a name lookup counts as a dial too. Restored by t.Cleanup.
func armNoNetwork(t *testing.T) *netLedger {
	t.Helper()
	led := &netLedger{}
	savedTransport, savedResolver := http.DefaultTransport, net.DefaultResolver
	http.DefaultTransport = refusingTransport{led: led}
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(_ context.Context, network, address string) (net.Conn, error) {
			led.add("dns " + network + " " + address)
			return nil, errors.New("promise 12: a read verb must not resolve a name")
		},
	}
	t.Cleanup(func() {
		http.DefaultTransport = savedTransport
		net.DefaultResolver = savedResolver
	})
	return led
}

// armSpawnLedger turns on spawn.go's argv recorder for the duration of one verb.
func armSpawnLedger(t *testing.T) func() []string {
	t.Helper()
	empty := []string{}
	spawnLedger.Store(&empty)
	t.Cleanup(func() { spawnLedger.Store(nil) })
	return func() []string {
		p := spawnLedger.Load()
		if p == nil {
			return nil
		}
		return *p
	}
}

func TestReadVerbsAnswerWithoutNetwork(t *testing.T) {
	verbs := make([]string, 0, len(coveredReadVerbs))
	for v := range coveredReadVerbs {
		verbs = append(verbs, v)
	}
	sort.Strings(verbs)
	for _, v := range verbs {
		args := coveredReadVerbs[v]
		t.Run(v, func(t *testing.T) {
			sqlDemoHome(t)
			dialed := armNoNetwork(t)
			spawned := armSpawnLedger(t)
			out, err := capture(t, func() error { return commands[v](args) })
			if err != nil {
				t.Fatalf("gadak %s %v failed offline: %v\nstdout:\n%s", v, args, err, out)
			}
			if strings.TrimSpace(out) == "" && v != "dashboards" {
				t.Errorf("gadak %s %v printed nothing — the block cannot tell a cached answer from a silent one", v, args)
			}
			if hits := dialed.list(); len(hits) > 0 {
				t.Errorf("gadak %s %v opened a socket: %v", v, args, hits)
			}
			if kids := spawned(); len(kids) > 0 {
				t.Errorf("gadak %s %v spawned a child process: %v", v, args, kids)
			}
		})
	}
	t.Run("sql answers from the cached fixture", func(t *testing.T) {
		sqlDemoHome(t)
		armNoNetwork(t)
		armSpawnLedger(t)
		out, err := capture(t, func() error { return cmdSQL([]string{"--no-header", "select count(*) from issues"}) })
		if err != nil {
			t.Fatalf("sql offline: %v", err)
		}
		if strings.TrimSpace(out) != "534" {
			t.Errorf("sql count = %q, want 534 rows out of the cache on disk", strings.TrimSpace(out))
		}
	})
}

// goSources returns the non-test .go files directly in dir.
func goSources(t *testing.T, dir string) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, n := range names {
		if !strings.HasSuffix(n, "_test.go") {
			out = append(out, n)
		}
	}
	return out
}

// TestSpawnHasOneOwner keeps axis (b) load-bearing: a spawn that does not go
// through spawn.go is invisible to the ledger, so a new one must turn this red
// before it can turn the promise false quietly.
func TestSpawnHasOneOwner(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range goSources(t, ".") {
		if filepath.Base(path) == "spawn.go" {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "exec" {
				return true
			}
			if sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext" {
				t.Errorf("%s: raw exec.%s — child processes in package main go through execCommand/execCommandContext in spawn.go, "+
					"so promise 12's no-child axis can see them", fset.Position(sel.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}

// TestOutboundHTTPHasNoPrivateTransport keeps axis (a) load-bearing. The hook
// replaces http.DefaultTransport, which every outbound client in this module
// resolves to today because none of them sets Transport. A client that brought
// its own would dial straight past the hook, so it has to be declared here.
func TestOutboundHTTPHasNoPrivateTransport(t *testing.T) {
	// The one declared exception: origin's serve passthrough wraps
	// http.DefaultTransport rather than replacing it, so the hook still sees
	// the round trip.
	allowed := map[string]bool{
		filepath.Join("..", "..", "internal", "origin", "transport.go"): true,
	}
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "dist", "dashvendor", "desktop", "mobile", "e2e":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || allowed[path] {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil // generated or build-tagged files are not this test's business
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "http" || sel.Sel.Name != "Client" {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Transport" {
					t.Errorf("%s: http.Client sets its own Transport. Promise 12's no-dial axis hooks "+
						"http.DefaultTransport; a private transport dials past it. Either route it through "+
						"http.DefaultTransport (as internal/origin/transport.go does) or add the file to this "+
						"test's allowlist and say why it cannot reach a read verb.", fset.Position(kv.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
