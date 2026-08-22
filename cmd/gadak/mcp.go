package main

// gadak mcp — stdio MCP server for clients without a shell.
// See docs/MCP.md and specs/000-product/contracts/agent.md.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/mcp"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

func cmdMCP(args []string) error {
	// Subcommand: pin the current profile into an MCP host registration.
	// Must run before the bare-server path so `gadak mcp install` never opens stdio.
	if len(args) > 0 && args[0] == "install" {
		return cmdMCPInstall(args[1:])
	}
	// No flags: profile comes from the global --profile / GADAK_PROFILE, and the
	// mirror path from the active profile (or GADAK_HOME / GADAK_DB is not used —
	// same DBPath as every other command).
	if wantsHelp(args) {
		printHelp("mcp")
		return nil
	}
	noSync, err := parseMCPOpts(args)
	if err != nil {
		return err
	}
	path, err := config.DBPath()
	if err != nil {
		return err
	}
	// Missing mirror: still start the protocol so tools/list works, and tool
	// calls return a clear isError. Mention setup here only if the user expects
	// an immediate failure for a typo'd home — but initialize must succeed.
	if _, err := os.Stat(path); err != nil {
		mcp.Logf("no mirror at %s — tools will error until you run `gadak init && gadak sync`", path)
	}
	srv := mcp.New(path, config.Profile(), version)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	db := startMCPSyncLoop(ctx, &wg, path, noSync)
	defer func() {
		cancel()
		wg.Wait()
		if db != nil {
			_ = db.Close()
		}
	}()
	// stdout is the JSON-RPC stream only. log.Fatalf and friends must not write
	// there while Serve runs; mcp.Logf goes to stderr.
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		return fmt.Errorf("mcp session: %w", err)
	}
	return nil
}

// parseMCPOpts is the bare-server flag surface. install is dispatched before
// this runs so `gadak mcp install` never lands here. ContinueOnError + discarded
// output keep a parse failure off stdout (JSON-RPC).
func parseMCPOpts(args []string) (bool, error) {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	noSync := fs.Bool("no-sync", false, "do not run the incremental sync loop")
	rest, err := parseAround(fs, args)
	if err != nil {
		return false, err
	}
	if len(rest) > 0 {
		return false, usageError("mcp", "usage: gadak mcp [--no-sync]")
	}
	return *noSync, nil
}

// mcpSyncLoopEnabled is the start gate for the MCP process watch loop.
// Same HasCredential / frozen / --no-sync shape as serve's startServeLoops.
func mcpSyncLoopEnabled(cfg *config.Config, noSync bool) (bool, string) {
	if noSync {
		return false, "no-sync"
	}
	if cfg == nil || !cfg.HasCredential() {
		return false, "no credential"
	}
	if cfg.SyncFrozen() {
		return false, "frozen"
	}
	return true, ""
}

// mcpWatchLog is syncer.Options.Log for the MCP process. mcp.Logf writes
// stderr only — a stdout writer here would corrupt the JSON-RPC stream.
func mcpWatchLog(s string) {
	mcp.Logf("%s", s)
}

// startMCPSyncLoop starts runWatchLoop when the gate allows it and the mirror
// file exists. A missing mirror still serves the protocol; the loop is not
// forced on that path. The returned DB is owned by the caller (close after Wait).
func startMCPSyncLoop(ctx context.Context, wg *sync.WaitGroup, path string, noSync bool) *store.DB {
	cfg, err := config.Load()
	if err != nil {
		mcp.Logf("sync loop off — %v", err)
		return nil
	}
	ok, reason := mcpSyncLoopEnabled(cfg, noSync)
	if !ok {
		mcp.Logf("sync loop off — %s", reason)
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	db, err := store.Open(path)
	if err != nil {
		mcp.Logf("sync loop off — open mirror: %v", err)
		return nil
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		runWatchLoop(ctx, cfg, db, syncer.Options{
			Log:    mcpWatchLog,
			Reload: config.Load,
		})
	}()
	return db
}
