package server

// The request path has one clock: s.now() (GDK-1975). A static fixture
// served past its own dates used to move with the wall — Sprint 43 began
// 2026-09-17, and from that 00:00Z e2e/retro.spec.ts saw three by-sprint
// columns where the fixture promises two. The fix is a single owner, and
// this gate is its recurrence layer: it fails on any time.Now() in a
// non-test file of this package that the allowlist below does not own.
// FAIL-first 2026-09-17: on the unmodified tree it named retro.go:97,
// read.go:179/286/655, personal.go:275 and jql.go:47.
//
// Plain `go test` on purpose (not behind tools/sourcelint.sh's build tag):
// the class it guards is e2e-visible on every `go test ./internal/server`,
// and the file-level parse needs no harness.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// wallClockAllowlist is every (file, top-level function) still allowed to
// read the wall directly, each with the one-line reason it must not follow
// the fixture clock. Pairing and token lifetimes are real security
// boundaries in real time: freezing them on a pinned clock would keep a
// revoked or expired token valid for as long as the fixture is served, and
// hide a Jira token that actually expires. The interface-list cache is the
// host's real state, which keeps changing whatever instant the fixture
// pretends to be.
//
// An entry whose function no longer calls time.Now() also fails the gate —
// the reasons stay attached to live code, not to history.
var wallClockAllowlist = map[string]map[string]string{
	"server.go": {
		// The clock's own three owners: the resolver that maps GADAK_CLOCK to
		// the wall or a frozen instant, the request-path method's default when
		// no clock is installed, and healthz's report of the live wall
		// instant. Everything else on the request path goes through s.now().
		"parseClock": "the clock resolver itself — empty or unparseable GADAK_CLOCK means the wall is the clock",
		"now":        "the owner method's default: with no clock installed, the request path's clock is the wall",
	},
	"health.go": {
		"health":       "token expiry is real time; a pinned clock would hide a token that actually expires while the fixture is served",
		"HealthzClock": "reports the wall's live instant when the source is wall — healthz must not freeze its own report",
	},
	"mirror_gate.go": {
		"PairedMirrorHostExempt": "pairing token validity is real time",
		"mirrorGate":             "pairing token validity is real time",
	},
	"terminal.go": {
		"terminalGate":             "pairing token validity is real time",
		"PairedTerminalHostExempt": "pairing token validity is real time",
		"terminalRevokeWatch":      "background watchdog reaps shells of revoked tokens on the wall",
	},
	"origin_rest.go": {
		"PairedOriginHostExempt": "pairing token validity is real time",
		"PairedAppOriginExempt":  "pairing token validity is real time",
		"pairingGate":            "pairing token validity is real time",
	},
	"viewer.go": {
		"isLocalInterfaceIP": "the host's interface list is real state; its TTL cache must expire in real time, not the fixture's",
	},
}

// TestRequestPathWallClockAllowlist parses every non-test *.go of this
// package and fails on any time.Now reference the allowlist above does not
// own — call or bare value, `clock: time.Now` is the same wall read as
// `time.Now()`. Attribution walks top-level declarations only: every read
// inside a FuncDecl (closures included — the gate pairs live at PairedXxx
// level) belongs to that function's name, and a read outside any function
// reports <package scope>.
func TestRequestPathWallClockAllowlist(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob *.go: %v (%d files)", err, len(files))
	}
	fset := token.NewFileSet()
	seen := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, decl := range f.Decls {
			owner := "<package scope>"
			if fd, ok := decl.(*ast.FuncDecl); ok {
				owner = fd.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || !isTimeNow(sel) {
					return true
				}
				if _, allowed := wallClockAllowlist[file][owner]; !allowed {
					t.Errorf("%s: %s reads the wall clock (time.Now) — route it through s.now(), or add it to wallClockAllowlist in clock_gate_test.go with its reason",
						fset.Position(sel.Pos()), owner)
				} else {
					seen[file+":"+owner] = true
				}
				return true
			})
		}
	}
	for file, fns := range wallClockAllowlist {
		for fn := range fns {
			if !seen[file+":"+fn] {
				t.Errorf("wallClockAllowlist names %s: %s but that function no longer reads time.Now() — remove the stale entry", file, fn)
			}
		}
	}
}

// isTimeNow reports whether e is a reference to time.Now (called or not).
func isTimeNow(e *ast.SelectorExpr) bool {
	ident, ok := e.X.(*ast.Ident)
	return ok && ident.Name == "time" && e.Sel.Name == "Now"
}

// TestServerClockPinning is the behavior half of GDK-1975: the gate above
// keeps time.Now off the request path, this proves what replaces it.
// serverClock itself resolves once per process, so the env path is
// exercised through parseClock (the parse serverClock memoizes) and the
// fallback through a bare server — the shape every zero-value test
// constructs.
func TestServerClockPinning(t *testing.T) {
	fn, at := parseClock("")
	if at != nil || fn == nil {
		t.Fatalf("empty GADAK_CLOCK must mean the wall (at=%v, fn installed=%v)", at, fn != nil)
	}
	if got := (&server{}).now(); time.Since(got) > time.Minute {
		t.Fatalf("now() with no clock installed is not the wall: %v", got)
	}

	fn, at = parseClock("2026-09-10T00:00:00.143Z")
	if at == nil {
		t.Fatal("a valid RFC3339 value must pin")
	}
	if one, two := fn(), fn(); !one.Equal(two) || !one.Equal(*at) {
		t.Fatalf("a pinned clock moved: %v vs %v (pin %v)", one, two, *at)
	}

	fn, at = parseClock("yesterday-ish")
	if at != nil {
		t.Fatalf("an unparseable value must not pin: at=%v", *at)
	}
	if got := fn(); time.Since(got) > time.Minute {
		t.Fatalf("an unparseable value must fall back to the wall: %v", got)
	}
}
