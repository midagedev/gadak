// Package tui is the terminal issue navigator for scry.
//
// It reads the local mirror only for list/detail; write actions (comment,
// transition, assignee) call Jira and re-mirror via sync.SyncIssue when a
// credential is configured.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// Run starts the issue navigator. It blocks until the user quits.
func Run(cfg *config.Config, db *store.DB) error {
	if db == nil {
		return fmt.Errorf("tui: database is required")
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	m := newModel(cfg, db)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
