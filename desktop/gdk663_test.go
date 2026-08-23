package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestGDK663StartWatchUsesWatchLoop is the GDK-663 contract on the desktop
// caller: run() must enter Watch through apprun.StartWatch, not a direct
// syncer.Watch. WatchLoop itself is pinned in internal/apprun.
//
// FAIL-first: with a direct syncer.Watch call in startWatch this fails
// at the direct-call check. After the move, a missing StartWatch call
// fails the same way.
func TestGDK663StartWatchUsesWatchLoop(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var watch, loop, start int
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := exprCallName(call.Fun)
		switch {
		case name == "syncer.Watch":
			watch++
		case name == "syncer.WatchLoop":
			loop++
		case strings.HasSuffix(name, "StartWatch"):
			start++
		}
		return true
	})
	if watch != 0 {
		t.Fatalf("GDK-663: desktop/main.go calls syncer.Watch %d time(s) — Watch returns on fatal auth and never re-enters; use apprun.StartWatch", watch)
	}
	if loop != 0 {
		t.Fatalf("GDK-663: desktop/main.go calls syncer.WatchLoop %d time(s) — WatchLoop lives in apprun; the caller is StartWatch", loop)
	}
	if start == 0 {
		t.Fatal("GDK-663: desktop/main.go does not call StartWatch")
	}
}
