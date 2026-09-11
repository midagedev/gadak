package main

import (
	"context"
	"os/exec"
	"sync/atomic"
)

// Child processes have one owner in this package (GDK-1792).
//
// Promise 12 in docs/PROMISES.md says a read verb answers from the cache on
// disk and moves no network. Replacing the process-wide HTTP transport proves
// the verb itself dialled nothing, but it proves nothing about a child: a
// detached `gadak sync` started from a read path gets its own address space
// and its own transport, so the transport hook stays green while the promise
// is false. The counter below is the second axis. Every spawn in this package
// goes through execCommand / execCommandContext, TestReadVerbsSpawnNoChild
// arms the ledger around each read verb, and TestSpawnHasOneOwner fails on a
// raw exec.Command anywhere else in package main — so new spawn code cannot
// route around the count without turning that test red first.
//
// The count is on construction, not on Start: conservative on purpose. A read
// verb that builds an *exec.Cmd it never runs is already doing something the
// promise does not describe, and construction is the one point every start
// path (Run, Start, Output, CombinedOutput) shares.

// spawnLedger records what was constructed while armed. Off by default: a
// nil ledger costs one atomic load per spawn and nothing else. The argv is
// kept because "a child was spawned" is not a debuggable failure message —
// "read verb `list` spawned [open https://…]" is.
var spawnLedger atomic.Pointer[[]string]

func recordSpawn(name string, arg []string) {
	p := spawnLedger.Load()
	if p == nil {
		return
	}
	argv := append([]string{name}, arg...)
	next := append(append([]string{}, *p...), joinArgv(argv))
	spawnLedger.Store(&next)
}

func joinArgv(argv []string) string {
	out := ""
	for i, a := range argv {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

// execCommand is exec.Command with the ledger hook.
func execCommand(name string, arg ...string) *exec.Cmd {
	recordSpawn(name, arg)
	return exec.Command(name, arg...)
}

// execCommandContext is exec.CommandContext with the ledger hook.
func execCommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	recordSpawn(name, arg)
	return exec.CommandContext(ctx, name, arg...)
}
