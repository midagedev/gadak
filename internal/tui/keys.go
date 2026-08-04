package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap documents the navigator bindings. Matching still happens via
// tea.KeyMsg.String() in the model; this type is the single source of truth
// for help text and tests.
type keyMap struct {
	Up, Down, Top, Bottom  key.Binding
	Filter, ClearFilter    key.Binding
	TabAll, TabOpen        key.Binding
	TabInProgress, TabDone key.Binding
	Enter, Back, Refresh   key.Binding
	Comment, Transition    key.Binding
	Assignee, Quit         key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:            key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:          key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Top:           key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:        key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Filter:        key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		ClearFilter:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear/back")),
		TabAll:        key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "all")),
		TabOpen:       key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "open")),
		TabInProgress: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "in progress")),
		TabDone:       key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "done")),
		Enter:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
		Back:          key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc", "back")),
		Refresh:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Comment:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Transition:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
		Assignee:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assignee")),
		Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
