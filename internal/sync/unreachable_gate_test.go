//go:build sourcelint

// Repo-wide AST gate (GDK-1144 discipline, GDK-1699 contract): lives behind
// the sourcelint tag so the default `go test ./...` never pays the
// whole-tree parse. Run with `bash tools/sourcelint.sh` — CI's Go-tests step
// runs exactly that.

package sync

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
)

// unreachableFixtureOwner is empty on purpose: no test may stand up an
// httptest server, close it, and then keep using its URL as an
// "unreachable" endpoint. On a busy CI runner another process can bind the
// closed port within microseconds — the request then reaches something, the
// expected failure never happens, and the test flakes (GDK-1699,
// run 34314938549). The reserved-listener helper (unreachableEndpoint in
// each package that needs one) is the only sanctioned shape: a port held
// for the test's lifetime cannot be stolen.
//
// The scan is textual and narrow on purpose — a var bound from
// httptest.NewServer followed within four lines by a bare non-defer
// <var>.Close(). A test that closes a server to exercise reconnect logic
// rebinds or rebuilds it, which this window does not match.
var unreachableFixtureOwner = map[string]string{}

var (
	unreachableNewServer = regexp.MustCompile(`(\w+)\s*:?=\s*httptest\.NewServer\(`)
	unreachableBareClose = regexp.MustCompile(`^\s*(\w+)\.Close\(\)\s*$`)
)

// TestUnreachableFixturesDoNotReuseClosedPorts fails on the closed-httptest
// unreachable fixture anywhere in the tree's tests.
func TestUnreachableFixturesDoNotReuseClosedPorts(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	var hits []string
	err := archlint.Walk(root, func(af *archlint.File) error {
		if !strings.HasSuffix(af.Rel, "_test.go") {
			return nil
		}
		body, err := af.Src()
		if err != nil {
			return err
		}
		lines := strings.Split(string(body), "\n")
		for i, line := range lines {
			m := unreachableNewServer.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			v := m[1]
			for j := i + 1; j < len(lines) && j <= i+4; j++ {
				c := unreachableBareClose.FindStringSubmatch(lines[j])
				if c == nil || c[1] != v {
					continue
				}
				if _, allowed := unreachableFixtureOwner[af.Rel]; allowed {
					continue
				}
				hits = append(hits, af.Rel+":"+strconv.Itoa(j+1)+" closes "+v+" from line "+strconv.Itoa(i+1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("a closed httptest port is OS state — another process can rebind it and the 'unreachable' request then succeeds (GDK-1699). Hold a reserved listener for the test's lifetime instead (unreachableEndpoint helpers):\n  %s",
			strings.Join(hits, "\n  "))
	}
}
