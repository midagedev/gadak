//go:build sourcelint

// Repo-wide AST gate (GDK-1315): lives behind the sourcelint tag so the
// default `go test ./...` never pays the whole-tree parse. Run with
// `bash tools/sourcelint.sh` — CI's Go-tests step runs exactly that.

package statuscat

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

// TestCloudKeyFoldHasOneOwner is the structural lock on the category
// fold (GDK-1315): the mapping between gadak's tokens (new|inprogress|done)
// and the Cloud fixture keys (new|indeterminate|done) is owned by
// CategoryKey in this package. internal/migrate used to carry a private
// map{"new":…, "inprogress":"indeterminate", "done":…} — a second table
// one quiet edit away from disagreeing with the owner. A production file
// outside internal/statuscat that spells the Cloud key "indeterminate" as
// a string literal is a second table being born and fails here; go through
// CategoryKey/KnownCategory instead.
//
// Comments and longer literals (error text, doc prose) are not BasicLits
// of exactly this value and do not match.
func TestCloudKeyFoldHasOneOwner(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	var hits []string
	err := archlint.Walk(root, func(af *archlint.File) error {
		if strings.HasSuffix(af.Rel, "_test.go") ||
			strings.HasPrefix(af.Rel, "internal/statuscat/") {
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
			if lit.Value != `"indeterminate"` {
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
		t.Fatalf(`the Cloud statusCategory key "indeterminate" must appear only in internal/statuscat (GDK-1315) — fold through statuscat.CategoryKey/KnownCategory instead of a second table:
  %s`,
			strings.Join(hits, "\n  "))
	}
}
