//go:build sourcelint

// Repo-wide AST gate (GDK-1753): lives behind the sourcelint tag so the
// default `go test ./...` never pays the whole-tree parse. Run with
// `bash tools/sourcelint.sh` — CI's Go-tests step runs exactly that.

package store

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

/*
 * TestReopenVerdictHasOneOwner is the structural lock on the reopen
 * predicate (GDK-1753). Before it, four surfaces spelled their own:
 * internal/store's reopenTransition (in-progress → new counts), the web's
 * view-config isReopen (done-only), the CLI --derive listing (done-only),
 * and two retro paths (done-only) — so one changelog row could be a red
 * feed event and reopen_count=1 in SQL while its timeline dot stayed grey.
 *
 * The rules:
 *
 *   R1  ReopenTransition is declared exactly once in the whole tree, in
 *       internal/store/derive.go. A second definition is a second owner.
 *   R2  No production file outside internal/store spells the predicate
 *       inline — the fingerprint is a conjunction where the from-side
 *       compares == done and the to-side != done, or from == in-progress
 *       && to == new. Route through store.ReopenTransition instead. The
 *       entered-done / entered-progress directions (resolved, started,
 *       closed — where the *to* side is the == done one) are different
 *       metrics and are not flagged.
 *   R3  internal/store/feed.go, whose "reopened" events key on ReopenedAt
 *       (the column only Derive's verdict sets), stays free of category
 *       comparisons — its path to the verdict is the derived column, and a
 *       direct comparison there would be a second rule being born.
 *
 * FAIL-first: against the pre-fix tree this flagged the printReopenReason site (now cmd/gadak/agent_print.go) and
 * internal/retro/materials.go (twice) — the copies this gate exists to
 * keep from coming back.
 */
func TestReopenVerdictHasOneOwner(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	var defs, inline []string
	feedHasCategory := false
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") {
			return nil
		}
		inStore := strings.HasPrefix(af.Rel, "internal/store/")
		f, fset, err := af.AST()
		if err != nil {
			t.Fatalf("parse %s: %v", af.Rel, err)
		}
		pos := func(n ast.Node) string {
			p := fset.Position(n.Pos())
			return af.Rel + ":" + strconv.Itoa(p.Line)
		}
		// R1: single declaration, in the owner's file.
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "ReopenTransition" {
				defs = append(defs, pos(d))
			}
		}
		// R3: feed.go reaches the verdict only through the derived column.
		if af.Rel == "internal/store/feed.go" {
			ast.Inspect(f, func(n ast.Node) bool {
				if categoryNameOf(n) != "" {
					feedHasCategory = true
				}
				return true
			})
		}
		// R2 runs on every expression; the owner's package is exempt (its
		// other metrics — resolved_at, started_at — compare categories
		// legitimately).
		if inStore {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			b, ok := n.(*ast.BinaryExpr)
			if !ok || b.Op != token.LAND {
				return true
			}
			if isReopenFingerprint(b.X, b.Y) || isReopenFingerprint(b.Y, b.X) {
				inline = append(inline, pos(b))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || !strings.HasPrefix(defs[0], "internal/store/derive.go:") {
		t.Fatalf("ReopenTransition must be declared exactly once, in internal/store/derive.go (GDK-1753); declarations: %v", defs)
	}
	if len(inline) > 0 {
		t.Fatalf(`an inline copy of the reopen predicate (GDK-1753) — route it through store.ReopenTransition:
  %s`, strings.Join(inline, "\n  "))
	}
	if feedHasCategory {
		t.Fatal("internal/store/feed.go compares status categories directly — its reopened events must keep keying on ReopenedAt, the column only Derive's verdict sets (GDK-1753)")
	}
}

// categoryConstNames are the tokens the predicate can be spelled with.
var categoryConstNames = map[string]bool{"CategoryDone": true, "CategoryInProgress": true, "categoryNew": true}

// categoryNameOf names the category constant n references, or "".
func categoryNameOf(n ast.Node) string {
	switch e := n.(type) {
	case *ast.Ident:
		if categoryConstNames[e.Name] {
			return e.Name
		}
	case *ast.SelectorExpr:
		if categoryConstNames[e.Sel.Name] {
			return e.Sel.Name
		}
	}
	return ""
}

// side is one operand of the conjunction: which category constant it
// compares against, with which operator, and which end of the move the
// compared expression names.
type side struct {
	cat    string
	eq     bool
	isFrom bool
	isTo   bool
}

func classifySide(e ast.Expr) (side, bool) {
	b, ok := e.(*ast.BinaryExpr)
	if !ok || (b.Op != token.EQL && b.Op != token.NEQ) {
		return side{}, false
	}
	s := side{eq: b.Op == token.EQL}
	for _, cand := range []ast.Expr{b.X, b.Y} {
		if name := categoryNameOf(cand); name != "" {
			s.cat = name
		}
	}
	if s.cat == "" {
		return side{}, false
	}
	// Which end of the move does the compared expression name? fromID /
	// from / r.fromID versus toID / to / r.toID — by identifier text, over
	// both operands. The constant operand is skipped whole: its package
	// qualifier (store) contains the letters "to" and would poison the
	// attribution.
	for _, cand := range []ast.Expr{b.X, b.Y} {
		if categoryNameOf(cand) != "" {
			continue
		}
		ast.Inspect(cand, func(m ast.Node) bool {
			if id, ok := m.(*ast.Ident); ok {
				l := strings.ToLower(id.Name)
				if strings.Contains(l, "from") {
					s.isFrom = true
				}
				if strings.Contains(l, "to") {
					s.isTo = true
				}
			}
			return true
		})
	}
	return s, true
}

// isReopenFingerprint reports whether a && b (in that order) spells the
// reopen predicate: from == done && to != done, or from == in-progress &&
// to == new. An operand naming both ends (a helper call move(from, to))
// classifies as neither and does not trip — the fence prefers a miss over
// a false accusation.
func isReopenFingerprint(a, b ast.Expr) bool {
	sa, ok := classifySide(a)
	if !ok {
		return false
	}
	sb, ok := classifySide(b)
	if !ok {
		return false
	}
	if !sa.isFrom || sa.isTo || !sb.isTo || sb.isFrom {
		return false
	}
	if sa.cat == "CategoryDone" && sb.cat == "CategoryDone" && sa.eq && !sb.eq {
		return true
	}
	return sa.cat == "CategoryInProgress" && sb.cat == "categoryNew" && sa.eq && sb.eq
}
