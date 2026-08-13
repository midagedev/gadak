package main

import (
	"fmt"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/tui"
)

// cmdTUI opens the terminal issue navigator against the local mirror.
func cmdTUI(args []string) error {
	if wantsHelp(args) {
		printHelp("tui")
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return fmt.Errorf("open mirror: %w (run `gadak sync` or `gadak demo` first)", err)
	}
	defer db.Close()
	return tui.Run(cfg, db, version)
}
