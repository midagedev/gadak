// Package tui is the terminal issue navigator for gadak.
//
// It reads the local mirror only for list/detail; write actions (comment,
// transition, assignee) call Jira and re-mirror via sync.SyncIssue when a
// credential is configured.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// Run starts the issue navigator. It blocks until the user quits. version is
// the running build ("0.0.0-dev" disables the update notice).
func Run(cfg *config.Config, db *store.DB, version string) error {
	if db == nil {
		return fmt.Errorf("tui: database is required")
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	m := newModel(cfg, db)
	m.version = version
	// Ambient neon runs only in a real session: GADAK_NO_ANIM / NO_COLOR turn
	// it off, and tests never set animOn at all.
	m.animOn = animEnabled()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
