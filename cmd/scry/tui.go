package main

import (
	"fmt"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/tui"
)

// cmdTUI opens the terminal issue navigator against the local mirror.
func cmdTUI(args []string) error {
	_ = args
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return fmt.Errorf("open mirror: %w (run `scry sync` or `scry demo` first)", err)
	}
	defer db.Close()
	return tui.Run(cfg, db)
}
