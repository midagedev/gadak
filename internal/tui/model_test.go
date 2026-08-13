package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// feed applies a message and returns the updated model (discards cmds).
func feed(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func feedKey(m Model, key string) Model {
	return feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func feedSpecial(m Model, t tea.KeyType) Model {
	return feed(m, tea.KeyMsg{Type: t})
}

func seededModel() Model {
	m := newModel(&config.Config{}, nil)
	m.width, m.height = 120, 40
	m.now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	m.all = sampleRows()
	m.refilter()
	return m
}

func TestTabKeys(t *testing.T) {
	m := seededModel()
	if len(m.visible) != 4 {
		t.Fatalf("start all: %d", len(m.visible))
	}

	m = feedKey(m, "3") // in progress
	if m.tab != TabInProgress || len(m.visible) != 1 {
		t.Fatalf("tab 3: tab=%v n=%d", m.tab, len(m.visible))
	}
	if m.all[m.visible[0]].lite.IssueKey != "AAA-2" {
		t.Fatalf("expected AAA-2, got %s", m.all[m.visible[0]].lite.IssueKey)
	}

	m = feedKey(m, "4") // done
	if m.tab != TabDone || len(m.visible) != 1 {
		t.Fatalf("tab 4: tab=%v n=%d", m.tab, len(m.visible))
	}

	m = feedKey(m, "2") // open
	if m.tab != TabOpen || len(m.visible) != 3 {
		t.Fatalf("tab 2: tab=%v n=%d", m.tab, len(m.visible))
	}

	m = feedKey(m, "1") // all
	if m.tab != TabAll || len(m.visible) != 4 {
		t.Fatalf("tab 1: tab=%v n=%d", m.tab, len(m.visible))
	}
}

func TestFilterKeystroke(t *testing.T) {
	m := seededModel()
	m = feedKey(m, "/")
	if m.mode != modeFilter {
		t.Fatalf("expected filter mode, got %v", m.mode)
	}
	// type "bob"
	for _, ch := range "bob" {
		m = feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if m.filter != "bob" {
		t.Fatalf("filter=%q", m.filter)
	}
	if len(m.visible) != 1 || m.all[m.visible[0]].lite.IssueKey != "AAA-2" {
		t.Fatalf("filtered keys: %v", visibleKeys(m))
	}
	m = feedSpecial(m, tea.KeyEnter)
	if m.mode != modeList {
		t.Fatalf("enter should leave filter mode")
	}
	// esc clears filter from list mode
	m = feedSpecial(m, tea.KeyEscape)
	if m.filter != "" || len(m.visible) != 4 {
		t.Fatalf("esc clear: filter=%q n=%d", m.filter, len(m.visible))
	}
}

func TestCursorKeys(t *testing.T) {
	m := seededModel()
	if m.issuesPane.cursor != 0 {
		t.Fatalf("start cursor %d", m.issuesPane.cursor)
	}
	m = feedKey(m, "j")
	if m.issuesPane.cursor != 1 {
		t.Fatalf("j → %d", m.issuesPane.cursor)
	}
	m = feedKey(m, "k")
	if m.issuesPane.cursor != 0 {
		t.Fatalf("k → %d", m.issuesPane.cursor)
	}
	m = feedKey(m, "G")
	if m.issuesPane.cursor != len(m.visible)-1 {
		t.Fatalf("G → %d want %d", m.issuesPane.cursor, len(m.visible)-1)
	}
	m = feedKey(m, "g")
	if m.issuesPane.cursor != 0 {
		t.Fatalf("g → %d", m.issuesPane.cursor)
	}
}

func TestWriteKeysWithoutCredential(t *testing.T) {
	m := seededModel()
	// c / t / a should toast, not panic, when no credential
	for _, key := range []string{"c", "t", "a"} {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("%s: expected toast cmd", key)
		}
		toast, ok := findToast(cmd)
		if !ok {
			t.Fatalf("%s: no toastMsg in cmd tree", key)
		}
		if !toast.err || !strings.Contains(toast.text, "credential") {
			t.Fatalf("%s toast: %+v", key, toast)
		}
	}
}

// findToast walks a tea.Cmd (including BatchMsg trees) for the first toastMsg.
func findToast(cmd tea.Cmd) (toastMsg, bool) {
	if cmd == nil {
		return toastMsg{}, false
	}
	msg := cmd()
	switch m := msg.(type) {
	case toastMsg:
		return m, true
	case tea.BatchMsg:
		for _, c := range m {
			if t, ok := findToast(c); ok {
				return t, true
			}
		}
	}
	return toastMsg{}, false
}

func TestRenderSmoke(t *testing.T) {
	m := seededModel()
	m = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	view := m.View()
	if view == "" {
		t.Fatal("empty view")
	}
	// Strip ANSI so assertions stay stable under style changes.
	plain := stripANSI(view)
	for _, want := range []string{"gadak", "AAA-1", "all", "open"} {
		if !strings.Contains(plain, want) {
			t.Errorf("view missing %q\n%s", want, plain)
		}
	}

	// Detail path without a real DB: inject detail and render.
	m.detailKey = "AAA-1"
	lite := m.all[0].lite
	m.detailLite = &lite
	m.detail = &store.Detail{
		IssueKey: "AAA-1",
		Comments: []store.DetailComment{
			{Author: "Ada", Body: "Looks good", CreatedAt: "2026-08-03T10:00:00Z"},
		},
		History: []store.DetailChange{
			{At: "2026-08-02T10:00:00Z", Author: "Bob", Field: "status", FromValue: "Backlog", ToValue: "Done"},
		},
	}
	m.mode = modeDetail
	dview := stripANSI(m.View())
	for _, want := range []string{"AAA-1", "Description", "Comments", "History", "Ada", "status"} {
		if !strings.Contains(dview, want) {
			t.Errorf("detail view missing %q\n%s", want, dview)
		}
	}

	// Esc returns to list
	m = feedSpecial(m, tea.KeyEscape)
	if m.mode != modeList {
		t.Fatalf("esc → mode %v", m.mode)
	}
}

// stripANSI removes CSI sequences so View() smoke tests can match plain text
// even when lipgloss wraps every glyph in colour codes.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			// CSI: ESC [ ... final byte in @-~
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && (s[i] < '@' || s[i] > '~') {
					i++
				}
				continue
			}
			// OSC / other: skip until BEL or ST
			i++
			for i < len(s) && s[i] != '\x07' && s[i] != '\x1b' {
				i++
			}
			if i < len(s) && s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func visibleKeys(m Model) []string {
	out := make([]string, len(m.visible))
	for i, idx := range m.visible {
		out[i] = m.all[idx].lite.IssueKey
	}
	return out
}

func TestHelpToggle(t *testing.T) {
	m := seededModel()
	m = feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.showHelp {
		t.Fatal("? should open help")
	}
	view := stripANSI(m.View())
	for _, want := range []string{"Keys", "j/↓", "filter", "feed", "quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("help missing %q\n%s", want, view)
		}
	}
	// Only real bindings from keys.go
	if strings.Contains(view, "bulk") {
		t.Error("help should not invent bulk shortcuts")
	}
	m = feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if m.showHelp {
		t.Fatal("? should close help")
	}
	// esc also closes
	m.showHelp = true
	m = feedSpecial(m, tea.KeyEscape)
	if m.showHelp {
		t.Fatal("esc should close help")
	}
}

func TestFeedViewRender(t *testing.T) {
	m := seededModel()
	at := "2026-08-04T10:00:00Z"
	m.feedItems = []store.FeedItem{
		{
			EventID:    "cm:1",
			IssueKey:   "AAA-1",
			Summary:    "Fix login timeout",
			EventType:  "comment_added",
			OccurredAt: &at,
			ActorName:  "Other",
		},
		{
			EventID:   "cl:2",
			IssueKey:  "AAA-2",
			Summary:   "Ship dashboard",
			EventType: "status_changed",
			ActorName: "Bob",
			ReadAt:    &at, // read
		},
	}
	m.feedCounts = store.FeedUnreadCounts{All: 1, Assignee: 1}
	m.mode = modeFeed
	m = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	plain := stripANSI(m.View())
	for _, want := range []string{"all", "assignee", "reporter", "mention", "AAA-1", "comment_added", "Other", "AAA-2", "status_changed", "1 unread"} {
		if !strings.Contains(plain, want) {
			t.Errorf("feed view missing %q\n%s", want, plain)
		}
	}
	// Status bar hints for feed mode
	if !strings.Contains(plain, "mark-read") {
		t.Errorf("feed status missing mark-read hint\n%s", plain)
	}

	// F leaves feed
	m = feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("F")})
	if m.mode != modeList {
		t.Fatalf("F should leave feed, mode=%v", m.mode)
	}
}

func TestFeedUnreadInListStatus(t *testing.T) {
	m := seededModel()
	m.feedCounts.All = 3
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "feed 3") {
		t.Fatalf("list status should show feed unread badge\n%s", plain)
	}
}

func TestFeedFocusKeys(t *testing.T) {
	m := seededModel()
	m.mode = modeFeed
	m.feedFocus = store.FeedFocusAll
	m.feedPane.cursor = 2
	m.feedPane.offset = 1
	m.feedItems = []store.FeedItem{
		{EventID: "1", IssueKey: "A-1"},
		{EventID: "2", IssueKey: "A-2"},
		{EventID: "3", IssueKey: "A-3"},
	}
	m.feedCounts = store.FeedUnreadCounts{All: 3, Assignee: 1, Reporter: 0, Mention: 2}

	// 2 → assignee; cursor/offset reset; load cmd issued (db nil → error msg if run).
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = next.(Model)
	if m.feedFocus != store.FeedFocusAssignee {
		t.Fatalf("focus=%q want assignee", m.feedFocus)
	}
	if m.feedPane.cursor != 0 || m.feedPane.offset != 0 {
		t.Fatalf("cursor=%d offset=%d want 0,0", m.feedPane.cursor, m.feedPane.offset)
	}
	if cmd == nil {
		t.Fatal("expected reload cmd")
	}
	// Same focus again is a no-op (no cmd).
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("same focus should not reload")
	}

	// 3 → reporter, 4 → mention, 1 → all
	for _, tc := range []struct {
		key  string
		want store.FeedFocus
	}{
		{"3", store.FeedFocusReporter},
		{"4", store.FeedFocusMention},
		{"1", store.FeedFocusAll},
	} {
		m.feedPane.cursor = 1
		next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		m = next.(Model)
		if m.feedFocus != tc.want {
			t.Fatalf("key %s: focus=%q want %q", tc.key, m.feedFocus, tc.want)
		}
		if m.feedPane.cursor != 0 {
			t.Fatalf("key %s: cursor not reset", tc.key)
		}
		if cmd == nil {
			t.Fatalf("key %s: expected reload", tc.key)
		}
	}

	// loadFeedCmd closes over current focus (model unit; db nil returns err).
	m.feedFocus = store.FeedFocusMention
	msg := m.loadFeedCmd()()
	loaded, ok := msg.(feedLoadedMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if loaded.err == nil {
		t.Fatal("nil db should error")
	}
}

func TestGroupedListCursorSkipsHeaders(t *testing.T) {
	m := seededModel()
	// Three categories → three headers; sampleRows has new×2, inprogress×1, done×1.
	m.groupBy = "status_category"
	m.listSort = listSort{key: "updated", dir: "desc"}
	m.refilter()
	lines := m.listLines()
	if len(lines) != len(m.visible)+3 {
		t.Fatalf("screen lines=%d visible=%d (want +3 headers)", len(lines), len(m.visible))
	}
	if m.issuesPane.cursor != 0 {
		t.Fatalf("start cursor %d", m.issuesPane.cursor)
	}
	// j through entire list — cursor always indexes an issue in visible.
	for i := 0; i < len(m.visible)+2; i++ {
		m = feedKey(m, "j")
		if m.issuesPane.cursor < 0 || m.issuesPane.cursor >= len(m.visible) {
			t.Fatalf("after j×%d cursor=%d visible=%d", i+1, m.issuesPane.cursor, len(m.visible))
		}
	}
	if m.issuesPane.cursor != len(m.visible)-1 {
		t.Fatalf("cursor should clamp at last issue: %d want %d", m.issuesPane.cursor, len(m.visible)-1)
	}
	// Headers are not selectable: every listLines issue line maps to a unique visIdx.
	seen := map[int]bool{}
	for _, ln := range m.listLines() {
		if ln.kind == lineKindIssue {
			if seen[ln.visIdx] {
				t.Fatalf("duplicate visIdx %d", ln.visIdx)
			}
			seen[ln.visIdx] = true
		}
	}
	if len(seen) != len(m.visible) {
		t.Fatalf("issue lines=%d visible=%d", len(seen), len(m.visible))
	}

	// Ungrouped path: no line expansion at all. nil is the contract — the
	// renderer then indexes m.visible directly instead of allocating a parallel
	// slice on every keystroke — and the cursor is already a screen line.
	m.groupBy = ""
	m.refilter()
	if lines := m.listLines(); lines != nil {
		t.Fatalf("ungrouped listLines should be nil, got %d lines", len(lines))
	}
	m.issuesPane.cursor = len(m.visible) - 1
	if got := m.cursorScreenLine(); got != m.issuesPane.cursor {
		t.Fatalf("ungrouped cursorScreenLine=%d, want %d", got, m.issuesPane.cursor)
	}
}

func TestClearSavedViewResetsSortGroup(t *testing.T) {
	m := seededModel()
	m.listSort = listSort{key: "created", dir: "asc"}
	m.groupBy = "assignee"
	m.viewName = "Mine"
	m.refilter()
	m = feedSpecial(m, tea.KeyEscape)
	if m.listSort.key != "" || m.groupBy != "" || m.viewName != "" {
		t.Fatalf("esc should clear sort/group/name: sort=%+v group=%q name=%q", m.listSort, m.groupBy, m.viewName)
	}
}

func TestEmptyListCopy(t *testing.T) {
	m := seededModel()
	m.all = nil
	m.refilter()
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "no issues — press / to filter · r to reload") {
		t.Fatalf("empty copy missing:\n%s", plain)
	}
}

func TestNarrowLayout(t *testing.T) {
	m := seededModel()
	m = feed(m, tea.WindowSizeMsg{Width: 35, Height: 20})
	plain := stripANSI(m.View())
	// Key present; full status column chrome not required to assert — just that it renders.
	if !strings.Contains(plain, "AAA-1") {
		t.Fatalf("narrow view missing key:\n%s", plain)
	}
	if !strings.Contains(plain, "j/k") {
		t.Fatalf("narrow status missing compact help:\n%s", plain)
	}
}

func TestWatchToggle(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir + "/tui-watch.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	m := newModel(&config.Config{}, db)
	m.width, m.height = 100, 30
	m.now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	m.all = sampleRows()
	m.watches = map[string]bool{}
	m.refilter()
	// Cursor on AAA-1
	if m.all[m.visible[0]].lite.IssueKey != "AAA-1" {
		t.Fatalf("expected AAA-1 first, got %s", m.all[m.visible[0]].lite.IssueKey)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("w should produce a command")
	}
	// Drain the async SetWatch result.
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	if !m.watches["AAA-1"] {
		t.Fatalf("expected AAA-1 watched after w, watches=%v toast=%q", m.watches, m.toast)
	}
	if !strings.Contains(m.toast, "watching") {
		t.Fatalf("toast=%q", m.toast)
	}
	// Persist check
	keys, err := db.Watches(context.Background())
	if err != nil || len(keys) != 1 || keys[0] != "AAA-1" {
		t.Fatalf("db watches=%v err=%v", keys, err)
	}
	// Toggle off
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	m = next.(Model)
	msg = cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.watches["AAA-1"] {
		t.Fatal("expected unwatched")
	}
	keys, _ = db.Watches(context.Background())
	if len(keys) != 0 {
		t.Fatalf("db still has %v", keys)
	}
}

func TestKeyMapNewBindings(t *testing.T) {
	km := defaultKeys()
	if !keyMatches(km.Help, "?") {
		t.Error("? should match Help")
	}
	if !keyMatches(km.Feed, "F") {
		t.Error("F should match Feed")
	}
	if !keyMatches(km.Views, "v") {
		t.Error("v should match Views")
	}
	if !keyMatches(km.Watch, "w") {
		t.Error("w should match Watch")
	}
	if !keyMatches(km.Edit, "e") {
		t.Error("e should match Edit")
	}
	lines := km.helpLines()
	if len(lines) < 15 {
		t.Fatalf("help lines too short: %d", len(lines))
	}
	found := false
	for _, pair := range lines {
		if pair[0] == "e" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("helpLines missing e")
	}
}

// TestDetailRendersDiscoveredFields proves the TUI detail shows discovered
// custom fields from config specs over Detail.Custom: non-body values as
// label/value rows, body-role values as prose sections, empty values omitted.
func TestDetailRendersDiscoveredFields(t *testing.T) {
	m := newModel(&config.Config{Fields: []config.FieldSpec{
		{Alias: "severity_level", Label: "Severity Level", IDs: []string{"customfield_10"}, Role: "facet"},
		{Alias: "repro_steps", Label: "Repro Steps", IDs: []string{"customfield_30"}, Role: "body"},
		{Alias: "never_filled", Label: "Never Filled", IDs: []string{"customfield_40"}, Role: "facet"},
	}}, nil)
	m.width, m.height = 100, 40
	m.all = []row{{lite: store.IssueLite{IssueKey: "AAA-1", Summary: "s", Status: "Done", StatusCategory: "done"}}}
	m.detailKey = "AAA-1"
	lite := m.all[0].lite
	m.detailLite = &lite
	m.detail = &store.Detail{
		IssueKey: "AAA-1",
		Custom: map[string]any{
			"severity_level": "High",
			"repro_steps": map[string]any{
				"type":    "doc",
				"version": float64(1),
				"content": []any{map[string]any{
					"type":    "paragraph",
					"content": []any{map[string]any{"type": "text", "text": "open the editor twice"}},
				}},
			},
		},
	}
	m.mode = modeDetail
	view := stripANSI(m.View())
	for _, want := range []string{"Severity Level", "High", "Repro Steps", "open the editor twice"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "Never Filled") {
		t.Error("empty field must not render")
	}
}
