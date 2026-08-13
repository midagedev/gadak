package main

// gadak mcp — stdio MCP server for clients without a shell.
// See docs/MCP.md and specs/000-product/contracts/agent.md.

import (
	"fmt"
	"os"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/mcp"
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
	// stdout is the JSON-RPC stream only. log.Fatalf and friends must not write
	// there while Serve runs; mcp.Logf goes to stderr.
	if err := srv.Serve(os.Stdin, os.Stdout); err != nil {
		return fmt.Errorf("mcp session: %w", err)
	}
	return nil
}
