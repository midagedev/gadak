//go:build sourcelint

// Repo-wide AST gate (GDK-1316): lives behind the sourcelint tag so the
// default `go test ./...` never pays the whole-tree parse. Run with
// `bash tools/sourcelint.sh` — CI's Go-tests step runs exactly that.

package reflink

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

// TestSchemeLiteralHasOneOwner is the structural lock on the ref grammar
// (GDK-1316): the exact "gadak://" literal had two private owners kept in
// step by hand (cmd/gadak's refScheme and internal/server's), each with its
// own parser copies. The grammar now lives in this package; a new string
// literal exactly equal to the scheme in production code outside
// internal/reflink/ is a second owner being born and fails here.
//
// Scope notes, so the rule stays narrow on purpose:
//   - Longer literals that merely mention the scheme (help text, error
//     messages, doc comments) do not match — this gate hunts owners, not
//     mentions.
//   - internal/deeplink owns a different grammar on the same scheme
//     (gadak://<action>?…) and builds it as Scheme + "://", so it has no
//     exact literal today; if it ever grows one, extend exempt below as a
//     conscious decision, not by widening the match.
//   - web/ composes gadak://view links in TypeScript (view-link.ts) — the
//     deeplink grammar, outside this Go gate's reach.
func TestSchemeLiteralHasOneOwner(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	exempt := func(rel string) bool {
		return strings.HasPrefix(rel, "internal/reflink/")
	}

	var hits []string
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") || exempt(af.Rel) {
			return nil
		}
		f, fset, err := af.AST()
		if err != nil {
			t.Fatalf("parse %s: %v", af.Rel, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if lit.Value != `"gadak://"` {
				return true
			}
			pos := fset.Position(lit.Pos())
			hits = append(hits, af.Rel+":"+strconv.Itoa(pos.Line))
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf(`the "gadak://" ref scheme literal must appear only in internal/reflink (GDK-1316) — use reflink.Scheme/Parse/Compose instead of a second copy:
  %s`,
			strings.Join(hits, "\n  "))
	}
}
