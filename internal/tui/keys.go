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
	Help, Feed, Views      key.Binding
	Watch                  key.Binding
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
		Refresh:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh / mark feed read")),
		Comment:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "comment")),
		Transition:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "transition")),
		Assignee:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "assignee")),
		Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Feed:          key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "feed")),
		Views:         key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "saved views")),
		Watch:         key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch")),
		Quit:          key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// helpLines is the ordered list shown by the ? overlay — actual bindings only.
func (k keyMap) helpLines() [][2]string {
	return [][2]string{
		{k.Up.Help().Key, k.Up.Help().Desc},
		{k.Down.Help().Key, k.Down.Help().Desc},
		{k.Top.Help().Key, k.Top.Help().Desc},
		{k.Bottom.Help().Key, k.Bottom.Help().Desc},
		{k.TabAll.Help().Key, k.TabAll.Help().Desc},
		{k.TabOpen.Help().Key, k.TabOpen.Help().Desc},
		{k.TabInProgress.Help().Key, k.TabInProgress.Help().Desc},
		{k.TabDone.Help().Key, k.TabDone.Help().Desc},
		{k.Filter.Help().Key, k.Filter.Help().Desc},
		{k.Enter.Help().Key, k.Enter.Help().Desc},
		{k.Back.Help().Key, k.Back.Help().Desc},
		{k.Comment.Help().Key, k.Comment.Help().Desc},
		{k.Transition.Help().Key, k.Transition.Help().Desc},
		{k.Assignee.Help().Key, k.Assignee.Help().Desc},
		{k.Watch.Help().Key, k.Watch.Help().Desc},
		{k.Feed.Help().Key, k.Feed.Help().Desc},
		{k.Views.Help().Key, k.Views.Help().Desc},
		{k.Refresh.Help().Key, k.Refresh.Help().Desc},
		{k.Help.Help().Key, k.Help.Help().Desc},
		{k.Quit.Help().Key, k.Quit.Help().Desc},
	}
}
