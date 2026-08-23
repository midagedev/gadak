package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestGDK663StartWatchUsesWatchLoop is the GDK-663 contract: desktop must
// re-enter Watch through syncer.WatchLoop. A direct syncer.Watch call
// returns on fatal auth and leaves the process alive with sync permanently
// stopped.
//
// FAIL-first: with the current syncer.Watch call in startWatch this fails
// at the direct-call check.
func TestGDK663StartWatchUsesWatchLoop(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var watch, loop int
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch exprCallName(call.Fun) {
		case "syncer.Watch":
			watch++
		case "syncer.WatchLoop":
			loop++
		}
		return true
	})
	if watch != 0 {
		t.Fatalf("GDK-663: desktop/main.go calls syncer.Watch %d time(s) — Watch returns on fatal auth and never re-enters; use syncer.WatchLoop", watch)
	}
	if loop == 0 {
		t.Fatal("GDK-663: desktop/main.go does not call syncer.WatchLoop")
	}
}
