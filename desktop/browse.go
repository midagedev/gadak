package main

import (
	"errors"
	"strconv"
	"sync"
	"unsafe"
)

// The in-app browser pane: Atlassian pages render in native WKWebViews layered
// over the SPA (Atlassian forbids iframes, so this is the only way to put a
// page inside the app's own layout). The SPA owns everything visual — the tab
// strip, which tab is active, and the rectangle the pane occupies — and drives
// this registry through the /desktop/browse routes. This side only bookkeeps
// ids and forwards to the platform embedder.
//
// One frame is shared by every tab: tabs are alternative contents of the same
// pane, never side by side.

// frameRect is the pane's rectangle in the SPA's coordinate space: CSS px,
// y from the top. The platform layer flips it into native coordinates.
type frameRect struct {
	X, Y, W, H float64
}

// embedder is the platform seam (macEmbedder on darwin; tests use a fake).
type embedder interface {
	Create(url string, f frameRect) (unsafe.Pointer, error)
	SetFrame(wv unsafe.Pointer, f frameRect)
	SetHidden(wv unsafe.Pointer, hidden bool)
	Close(wv unsafe.Pointer)
	Info(wv unsafe.Pointer) (title, url string)
}

type browseTab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type browseTabs struct {
	mu     sync.Mutex
	emb    embedder // nil until bind; routes answer 503 before that
	seq    int
	tabs   map[string]unsafe.Pointer
	order  []string // insertion order, what the tab strip shows
	active string   // "" = no tab visible (the SPA is showing itself)
	frame  frameRect
}

var errBrowseUnavailable = errors.New("browse pane not ready")

func newBrowseTabs() *browseTabs {
	return &browseTabs{tabs: map[string]unsafe.Pointer{}}
}

// bind installs the platform embedder once the application (and main window)
// exist. Until then Open fails and the routes answer 503.
func (b *browseTabs) bind(emb embedder) {
	b.mu.Lock()
	b.emb = emb
	b.mu.Unlock()
}

// Open creates a tab over the current pane rect and makes it the visible one.
func (b *browseTabs) Open(url string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.emb == nil {
		return "", errBrowseUnavailable
	}
	wv, err := b.emb.Create(url, b.frame)
	if err != nil {
		return "", err
	}
	b.seq++
	id := strconv.Itoa(b.seq)
	b.tabs[id] = wv
	b.order = append(b.order, id)
	b.activateLocked(id)
	return id, nil
}

// Activate shows one tab and hides the rest; "" hides them all. The SPA also
// uses "" while one of its own overlays (palette, media viewer) is up — a
// native view otherwise draws over every SPA pixel in its rect.
func (b *browseTabs) Activate(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.emb == nil {
		return errBrowseUnavailable
	}
	if id != "" {
		if _, ok := b.tabs[id]; !ok {
			return errors.New("no such tab")
		}
	}
	b.activateLocked(id)
	return nil
}

func (b *browseTabs) activateLocked(id string) {
	for tid, wv := range b.tabs {
		b.emb.SetHidden(wv, tid != id)
	}
	b.active = id
}

// CloseTab tears the webview down. The next active tab is the SPA's decision,
// not ours: active just clears when the visible tab closes.
func (b *browseTabs) CloseTab(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.emb == nil {
		return errBrowseUnavailable
	}
	wv, ok := b.tabs[id]
	if !ok {
		return errors.New("no such tab")
	}
	b.emb.Close(wv)
	delete(b.tabs, id)
	for i, tid := range b.order {
		if tid == id {
			b.order = append(b.order[:i], b.order[i+1:]...)
			break
		}
	}
	if b.active == id {
		b.active = ""
	}
	return nil
}

// CloseActive is the Close Tab menu item (⌘W). A no-op when the SPA is
// frontmost — the stock CloseWindow role would quit the app instead.
func (b *browseTabs) CloseActive() {
	b.mu.Lock()
	id := b.active
	b.mu.Unlock()
	if id != "" {
		_ = b.CloseTab(id)
	}
}

// SetFrame moves the pane. Applies to every tab — hidden ones must be in
// place before they are shown. Window resizes are handled natively
// (autoresizing margins); this is for the SPA's own layout changes.
func (b *browseTabs) SetFrame(f frameRect) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.frame = f
	if b.emb == nil {
		return
	}
	for _, wv := range b.tabs {
		b.emb.SetFrame(wv, f)
	}
}

// State answers the SPA's poll: open tabs in strip order, with live
// title/URL off each webview, plus which one is visible.
func (b *browseTabs) State() (open []browseTab, active string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	open = []browseTab{}
	for _, id := range b.order {
		title, url := "", ""
		if b.emb != nil {
			title, url = b.emb.Info(b.tabs[id])
		}
		open = append(open, browseTab{ID: id, Title: title, URL: url})
	}
	return open, b.active
}
