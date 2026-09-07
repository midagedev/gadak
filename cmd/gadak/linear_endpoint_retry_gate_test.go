package main

import (
	"go/ast"
	"go/token"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/archlint"
	"github.com/midagedev/gadak/internal/httppolicy"
)

// TestLinearEndpointStubsDoNotReturnRetryableStatus is the GDK-719 class
// lock: origin.Linear() always builds linear.New with production
// httppolicy.DefaultRetries/DefaultBackoff (5, 1s), and the LinearEndpoint
// test seam only overrides the URL. A stub that answers a retryable status
// therefore sleeps 1+2+4+8s — TestCreateLinearOnlyDoesNotAskForInit did,
// measured 15.03s.
//
// Tests that construct a *linear.Client themselves can set Retries/Backoff
// (internal/linear.testClient, testLinearWriter). Tests that go through
// LinearEndpoint cannot, so they must not return a retryable status.
//
// FAIL-first (unmodified linear_workspace_test.go): this test failed on
// TestCreateLinearOnlyDoesNotAskForInit writing HTTP 500.
func TestLinearEndpointStubsDoNotReturnRetryableStatus(t *testing.T) {
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
		if !strings.HasPrefix(af.Rel, "cmd/gadak/") && !strings.HasPrefix(af.Rel, "internal/") {
			return nil
		}
		hits = append(hits, findLinearEndpointRetryableStubs(t, af)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("LinearEndpoint stubs must not return httppolicy.IsRetryable statuses — origin.Linear() uses production retries and the seam cannot lower them:\n  %s",
			strings.Join(hits, "\n  "))
	}
}

// TestGateNamedRetryableStatusesMatchPolicy fails if httppolicy.IsRetryable
// grows a code this scanner cannot resolve from an http.Status* selector.
func TestGateNamedRetryableStatusesMatchPolicy(t *testing.T) {
	known := map[int]string{}
	for name, code := range retryableHTTPStatusName {
		if !httppolicy.IsRetryable(code) {
			t.Errorf("retryableHTTPStatusName[%q]=%d is not httppolicy.IsRetryable", name, code)
		}
		known[code] = name
	}
	for code := 100; code < 600; code++ {
		if httppolicy.IsRetryable(code) && known[code] == "" {
			t.Errorf("httppolicy.IsRetryable(%d) has no http.Status* name in retryableHTTPStatusName", code)
		}
	}
}

// retryableHTTPStatusName is the net/http identifiers for the codes
// httppolicy.IsRetryable currently accepts. Values come from net/http, not a
// hand-built table; TestGateNamedRetryableStatusesMatchPolicy locks the set
// to the policy.
var retryableHTTPStatusName = map[string]int{
	"StatusTooManyRequests":     http.StatusTooManyRequests,
	"StatusInternalServerError": http.StatusInternalServerError,
	"StatusBadGateway":          http.StatusBadGateway,
	"StatusServiceUnavailable":  http.StatusServiceUnavailable,
	"StatusGatewayTimeout":      http.StatusGatewayTimeout,
}

func findLinearEndpointRetryableStubs(t *testing.T, af *archlint.File) []string {
	t.Helper()
	f, fset, err := af.AST()
	if err != nil {
		t.Fatalf("parse %s: %v", af.Rel, err)
		return nil
	}
	var hits []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		setsEndpoint := false
		var retryAt []string
		ast.Inspect(fn, func(n ast.Node) bool {
			if assignsLinearEndpoint(n) {
				setsEndpoint = true
			}
			if code, ok := retryableStatusCall(n); ok {
				pos := fset.Position(n.Pos())
				name := fn.Name.Name
				retryAt = append(retryAt, af.Rel+":"+strconv.Itoa(pos.Line)+" "+name+" HTTP "+strconv.Itoa(code))
			}
			return true
		})
		if setsEndpoint {
			hits = append(hits, retryAt...)
		}
	}
	return hits
}

func assignsLinearEndpoint(n ast.Node) bool {
	as, ok := n.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, lhs := range as.Lhs {
		switch x := lhs.(type) {
		case *ast.Ident:
			if x.Name == "LinearEndpoint" {
				return true
			}
		case *ast.SelectorExpr:
			if x.Sel != nil && x.Sel.Name == "LinearEndpoint" {
				return true
			}
		}
	}
	return false
}

func retryableStatusCall(n ast.Node) (int, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return 0, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return 0, false
	}
	var arg ast.Expr
	switch sel.Sel.Name {
	case "WriteHeader":
		if len(call.Args) != 1 {
			return 0, false
		}
		arg = call.Args[0]
	case "Error":
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "http" || len(call.Args) != 3 {
			return 0, false
		}
		arg = call.Args[2]
	default:
		return 0, false
	}
	code, ok := httpStatusExpr(arg)
	if !ok || !httppolicy.IsRetryable(code) {
		return 0, false
	}
	return code, true
}

func httpStatusExpr(e ast.Expr) (int, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.INT {
			return 0, false
		}
		n, err := strconv.Atoi(x.Value)
		return n, err == nil
	case *ast.Ident:
		code, ok := retryableHTTPStatusName[x.Name]
		return code, ok
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		if !ok || pkg.Name != "http" || x.Sel == nil {
			return 0, false
		}
		code, ok := retryableHTTPStatusName[x.Sel.Name]
		return code, ok
	}
	return 0, false
}
