package main

import (
	"errors"
	"sort"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// browseWindows tracks the in-app browser tabs: Atlassian pages opened as
// native macOS tab windows so edits happen next to the mirror instead of in a
// pile of browser tabs. The registry only holds ids — the SPA polls
// GET /desktop/browse/state, diffs against the tabs it opened, and treats a
// missing id as "tab closed, resync that item" (web/src/lib/browse.svelte.ts).
//
// spawn is bound to the real window constructor once the application exists
// (same late binding as openURL in run()); handler tests substitute a fake.
type browseWindows struct {
	mu    sync.Mutex
	seq   int
	open  map[string]struct{}
	spawn func(url, title string, onClosing func()) error
}

func newBrowseWindows() *browseWindows {
	return &browseWindows{open: map[string]struct{}{}}
}

// bindApp wires spawn to real webview windows. Called after application.New;
// until then Open fails and the route answers 503. The returned func closes
// the focused window if it is a browse tab — the Close Tab menu item (⌘W)
// wants exactly that and must never close the main window.
func (b *browseWindows) bindApp(app *application.App) (closeCurrentTab func()) {
	var mu sync.Mutex
	wins := map[uint]*application.WebviewWindow{}
	b.spawn = func(url, title string, onClosing func()) error {
		win := app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title:     title,
			Width:     1200,
			Height:    850,
			MinWidth:  640,
			MinHeight: 480,
			URL:       url,
			Mac: application.MacWindow{
				// Preferred, not Automatic: every browse window asks to join a
				// native tab group, and since the main Scry window keeps the
				// wails default (tabbing disallowed), the only group they can
				// form is with each other — one tabbed browser window.
				TabbingMode: application.MacWindowTabbingModePreferred,
			},
		})
		mu.Lock()
		wins[win.ID()] = win
		mu.Unlock()
		// Fires for ⌘W on a tab as well as the close button. Window methods
		// marshal to the main thread themselves, so no InvokeSync here.
		win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			mu.Lock()
			delete(wins, win.ID())
			mu.Unlock()
			onClosing()
		})
		return nil
	}
	return func() {
		cur := app.Window.Current()
		if cur == nil {
			return
		}
		mu.Lock()
		win := wins[cur.ID()]
		mu.Unlock()
		if win != nil {
			win.Close()
		}
	}
}

var errBrowseUnavailable = errors.New("browse windows not ready")

func (b *browseWindows) Open(url, title string) (string, error) {
	b.mu.Lock()
	spawn := b.spawn
	b.seq++
	id := strconv.Itoa(b.seq)
	b.open[id] = struct{}{}
	b.mu.Unlock()
	if spawn == nil {
		b.remove(id)
		return "", errBrowseUnavailable
	}
	if err := spawn(url, title, func() { b.remove(id) }); err != nil {
		b.remove(id)
		return "", err
	}
	return id, nil
}

func (b *browseWindows) remove(id string) {
	b.mu.Lock()
	delete(b.open, id)
	b.mu.Unlock()
}

// OpenIDs answers the state poll. Sorted numerically so responses are stable.
func (b *browseWindows) OpenIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]string, 0, len(b.open))
	for id := range b.open {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, _ := strconv.Atoi(ids[i])
		z, _ := strconv.Atoi(ids[j])
		return a < z
	})
	return ids
}
