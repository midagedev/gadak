package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/store"
)

// samplePages builds a two-space tree used by tree / breadcrumb / filter tests:
//
//	ENG
//	  Root (depth 0)
//	    Child (depth 1)
//	    Orphan-parent-in-other-space treated as root: Cross (parent in PROD)
//	  Sibling root
//	PROD
//	  Alone
//	  CycleA ↔ CycleB (cycle → both surface as roots)
func samplePages() []store.PageLite {
	return []store.PageLite{
		{Key: "e-root", Title: "Root Guide", SpaceKey: "ENG", ParentID: "", Author: "Ada", UpdatedAt: "2026-08-01T10:00:00Z"},
		{Key: "e-child", Title: "Child Page", SpaceKey: "ENG", ParentID: "e-root", Author: "Bob", UpdatedAt: "2026-08-02T10:00:00Z"},
		{Key: "e-sib", Title: "Sibling Root", SpaceKey: "ENG", ParentID: "", Author: "Ada", UpdatedAt: "2026-08-01T11:00:00Z"},
		// Parent lives in another space → hang as ENG root.
		{Key: "e-cross", Title: "Cross Ref", SpaceKey: "ENG", ParentID: "p-alone", Author: "Ada", UpdatedAt: "2026-08-01T12:00:00Z"},
		// Missing parent → root.
		{Key: "e-miss", Title: "Missing Parent", SpaceKey: "ENG", ParentID: "no-such", Author: "Ada", UpdatedAt: "2026-08-01T13:00:00Z"},
		{Key: "p-alone", Title: "Prod Alone", SpaceKey: "PROD", ParentID: "", Author: "Lee", UpdatedAt: "2026-08-03T10:00:00Z"},
		// Cycle: a→b→a
		{Key: "p-ca", Title: "Cycle A", SpaceKey: "PROD", ParentID: "p-cb", Author: "Lee", UpdatedAt: "2026-08-03T11:00:00Z"},
		{Key: "p-cb", Title: "Cycle B", SpaceKey: "PROD", ParentID: "p-ca", Author: "Lee", UpdatedAt: "2026-08-03T12:00:00Z"},
	}
}

func seededDocsModel() Model {
	m := newModel(&config.Config{}, nil)
	m.width, m.height = 120, 40
	m.now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	m.pages = samplePages()
	m.docsTab = docsTabUpdated
	m.refilterDocs()
	m.mode = modeDocs
	return m
}

func TestBuildDocsTreeIndentAndOrder(t *testing.T) {
	// Space order alphabetical: ENG before PROD.
	// Within ENG, roots sorted by title: Cross Ref, Missing Parent, Root Guide, Sibling Root.
	// Child of Root Guide at depth 1, indented under Root Guide.
	lines := buildDocsLines(samplePages())

	// Collect (space header | page key@depth) sequence.
	var got []string
	for _, ln := range lines {
		if ln.kind == docsLineHeader {
			got = append(got, "H:"+ln.space)
			continue
		}
		got = append(got, "P:"+ln.page.Key+"@"+itoa(ln.depth))
	}

	// ENG header first, then its pages in tree order.
	if len(got) < 2 || got[0] != "H:ENG" {
		t.Fatalf("first line = %v, want H:ENG", got)
	}
	// Find Root Guide then Child at depth 1 immediately after (tree walk).
	rootIdx := -1
	for i, s := range got {
		if s == "P:e-root@0" {
			rootIdx = i
			break
		}
	}
	if rootIdx < 0 {
		t.Fatalf("missing e-root in %v", got)
	}
	if rootIdx+1 >= len(got) || got[rootIdx+1] != "P:e-child@1" {
		t.Fatalf("child not indented under root: around %v", got[rootIdx:])
	}
	// Cross-space parent and missing parent are depth 0 roots.
	for _, key := range []string{"e-cross@0", "e-miss@0"} {
		found := false
		for _, s := range got {
			if s == "P:"+key {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("want root %s in %v", key, got)
		}
	}
	// PROD after ENG; cycle pages both appear (not dropped).
	prodIdx := -1
	for i, s := range got {
		if s == "H:PROD" {
			prodIdx = i
			break
		}
	}
	if prodIdx < 0 {
		t.Fatalf("missing PROD header in %v", got)
	}
	if prodIdx < rootIdx {
		t.Fatalf("PROD before ENG content: %v", got)
	}
	// Cycle residual surfaces (not dropped). One is walked as root, the other
	// as its child — same as web treeBySpace (first residual owns the walk).
	var cycleSeen int
	for _, s := range got {
		if strings.HasPrefix(s, "P:p-ca@") || strings.HasPrefix(s, "P:p-cb@") {
			cycleSeen++
		}
	}
	if cycleSeen != 2 {
		t.Fatalf("cycle pages seen=%d want 2 in %v", cycleSeen, got)
	}
}

func TestDocsBreadcrumb(t *testing.T) {
	pages := samplePages()
	// Root: space › title
	if got := docsBreadcrumb(pages, "e-root"); got != "ENG › Root Guide" {
		t.Fatalf("root breadcrumb = %q", got)
	}
	// Child: space › ancestor… › title
	if got := docsBreadcrumb(pages, "e-child"); got != "ENG › Root Guide › Child Page" {
		t.Fatalf("child breadcrumb = %q", got)
	}
	// Cross-space parent: parent not in same chain for breadcrumb (parent is PROD).
	// ancestors only walks parent_id while parent exists in index — web walks regardless of space.
	// Match web: ancestors() does not check space, only byKey.
	if got := docsBreadcrumb(pages, "e-cross"); got != "ENG › Prod Alone › Cross Ref" {
		t.Fatalf("cross breadcrumb = %q", got)
	}
}

func TestDocsEmptyState(t *testing.T) {
	m := newModel(&config.Config{}, nil)
	m.width, m.height = 100, 30
	m.now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	m.pages = nil
	m.refilterDocs()
	m.mode = modeDocs
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "no documents mirrored") {
		t.Fatalf("empty docs view missing empty state:\n%s", plain)
	}
}

func TestDocsToggleAndTreeRender(t *testing.T) {
	m := seededModel() // issue list
	m.pages = samplePages()
	// D enters docs (default Updated); 3 switches to Spaces tree.
	m = feedKey(m, "D")
	if m.mode != modeDocs {
		t.Fatalf("D → mode %v want modeDocs", m.mode)
	}
	m = feedKey(m, "3")
	if m.docsTab != docsTabSpaces {
		t.Fatalf("3 → docsTab %v want spaces", m.docsTab)
	}
	plain := stripANSI(m.View())
	for _, want := range []string{"ENG", "PROD", "Root Guide", "Child Page"} {
		if !strings.Contains(plain, want) {
			t.Errorf("docs list missing %q\n%s", want, plain)
		}
	}
	// Indent: child line should carry deeper leading spaces than root in plain render.
	// Cursor on first navigable page; j/k moves among pages only.
	if m.docsPane.cursor != 0 {
		t.Fatalf("docs cursor start %d", m.docsPane.cursor)
	}
	m = feedKey(m, "j")
	if m.docsPane.cursor != 1 {
		t.Fatalf("j → docsCursor %d", m.docsPane.cursor)
	}
	// D again returns to issues
	m = feedKey(m, "D")
	if m.mode != modeList {
		t.Fatalf("D toggle back → mode %v", m.mode)
	}
}

func TestDocsFilter(t *testing.T) {
	m := seededDocsModel()
	m = feedKey(m, "/")
	if m.mode != modeFilter {
		t.Fatalf("expected filter mode, got %v", m.mode)
	}
	for _, ch := range "child" {
		m = feed(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if m.filter != "child" {
		t.Fatalf("filter=%q", m.filter)
	}
	// Only Child Page remains navigable (title match).
	if n := len(m.docsNav); n != 1 {
		t.Fatalf("filtered nav n=%d keys=%v", n, docsNavKeys(m))
	}
	if m.docsNav[0].page.Key != "e-child" {
		t.Fatalf("want e-child, got %v", docsNavKeys(m))
	}
	// Space match: filter "prod"
	m.filter = ""
	m.refilterDocs()
	for _, ch := range "prod" {
		m.filter += string(ch)
	}
	// refilter after setting filter like key handler does
	m.refilterDocs()
	// PROD pages + ENG Cross? "prod" matches space PROD and title "Prod Alone"
	for _, p := range m.docsNav {
		hay := strings.ToLower(p.page.Title + " " + p.page.SpaceKey)
		if !strings.Contains(hay, "prod") {
			t.Errorf("unmatched page in filter: %s %s", p.page.Key, hay)
		}
	}
	if len(m.docsNav) == 0 {
		t.Fatal("prod filter emptied nav")
	}
	// Enter leaves filter back to docs, not issue list.
	m.mode = modeFilter
	m = feedSpecial(m, tea.KeyEnter)
	if m.mode != modeDocs {
		t.Fatalf("enter from docs filter → mode %v want modeDocs", m.mode)
	}
}

func TestDocsUnsupportedActions(t *testing.T) {
	m := seededDocsModel()
	// Issue-only actions must report unsupported, not silent no-op / credential toast.
	for _, key := range []string{"c", "t", "a", "e", "w"} {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("%s: expected toast cmd", key)
		}
		toast, ok := findToast(cmd)
		if !ok {
			t.Fatalf("%s: no toastMsg", key)
		}
		if !strings.Contains(strings.ToLower(toast.text), "unsupported") {
			t.Fatalf("%s toast=%q want unsupported", key, toast.text)
		}
	}
}

func TestDocsDetailBreadcrumbAndBody(t *testing.T) {
	m := seededDocsModel()
	// Inject detail without DB.
	bodyADF, _ := json.Marshal(map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Login guide body text"},
				},
			},
		},
	})
	m.pageDetail = &store.PageDetail{
		PageLite: store.PageLite{
			Key: "e-child", Title: "Child Page", SpaceKey: "ENG", ParentID: "e-root",
		},
		BodyADF: bodyADF,
		Comments: []store.PageComment{
			{Author: "Lee", CreatedAt: "2026-08-03T10:00:00Z", BodyText: "Helpful note"},
		},
	}
	m.pageDetailKey = "e-child"
	m.mode = modeDocDetail
	plain := stripANSI(m.View())
	for _, want := range []string{
		"ENG › Root Guide › Child Page",
		"Login guide body text",
		"Lee",
		"Helpful note",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("doc detail missing %q\n%s", want, plain)
		}
	}
	// Esc back to docs list
	m = feedSpecial(m, tea.KeyEscape)
	if m.mode != modeDocs {
		t.Fatalf("esc → mode %v want modeDocs", m.mode)
	}
}

func TestDocsHelpMentionsFTSHonesty(t *testing.T) {
	m := seededDocsModel()
	m.showHelp = true
	plain := stripANSI(m.View())
	// Help lists the docs key and the FTS honesty note.
	if !strings.Contains(plain, "docs") && !strings.Contains(plain, "documents") {
		t.Errorf("help missing docs binding:\n%s", plain)
	}
	if !strings.Contains(strings.ToLower(plain), "full-text") &&
		!strings.Contains(strings.ToLower(plain), "full text") {
		// Accept either phrasing for the web/CLI pointer.
		if !strings.Contains(plain, "web/CLI") && !strings.Contains(plain, "web or CLI") {
			t.Errorf("help missing full-text search honesty note:\n%s", plain)
		}
	}
}

// TestBuildDocsUpdatedOrder: flat list, updated_at desc (newest first).
// Empty timestamps sort last (same as issue list sort).
func TestBuildDocsUpdatedOrder(t *testing.T) {
	pages := []store.PageLite{
		{Key: "old", Title: "Old", SpaceKey: "A", UpdatedAt: "2026-08-01T10:00:00Z"},
		{Key: "new", Title: "New", SpaceKey: "B", UpdatedAt: "2026-08-03T10:00:00Z"},
		{Key: "mid", Title: "Mid", SpaceKey: "A", UpdatedAt: "2026-08-02T10:00:00Z"},
		{Key: "empty", Title: "No stamp", SpaceKey: "C", UpdatedAt: ""},
	}
	lines := buildDocsUpdatedLines(pages)
	var keys []string
	for _, ln := range lines {
		if ln.kind != docsLinePage {
			t.Fatalf("updated view must be flat (no headers), got kind=%d", ln.kind)
		}
		keys = append(keys, ln.page.Key)
	}
	want := []string{"new", "mid", "old", "empty"}
	if len(keys) != len(want) {
		t.Fatalf("keys=%v want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys=%v want %v", keys, want)
		}
	}
}

// TestBuildDocsByAuthor: group headers (issue header grammar), empty author
// bucket is "(no author)", within-group updated_at desc, groups by newest first.
func TestBuildDocsByAuthor(t *testing.T) {
	pages := []store.PageLite{
		{Key: "a1", Title: "Ada old", Author: "Ada", UpdatedAt: "2026-08-01T10:00:00Z"},
		{Key: "a2", Title: "Ada new", Author: "Ada", UpdatedAt: "2026-08-03T10:00:00Z"},
		{Key: "b1", Title: "Bob only", Author: "Bob", UpdatedAt: "2026-08-02T10:00:00Z"},
		{Key: "n1", Title: "Anon", Author: "", UpdatedAt: "2026-08-04T10:00:00Z"},
	}
	lines := buildDocsByAuthorLines(pages)
	var got []string
	for _, ln := range lines {
		if ln.kind == docsLineHeader {
			got = append(got, "H:"+ln.space)
			continue
		}
		got = append(got, "P:"+ln.page.Key)
	}
	// n1 is newest overall → "(no author)" group first; then Ada (a2=Aug3);
	// then Bob (Aug2). Within Ada: a2 before a1.
	want := []string{
		"H:(no author)", "P:n1",
		"H:Ada", "P:a2", "P:a1",
		"H:Bob", "P:b1",
	}
	if len(got) != len(want) {
		t.Fatalf("got=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want %v", got, want)
		}
	}
}

// TestPageSpaceLabel: SpaceName when set, else SpaceKey (web spaceLabel parity).
func TestPageSpaceLabel(t *testing.T) {
	if got := pageSpaceLabel(store.PageLite{SpaceKey: "ENG", SpaceName: "Engineering"}); got != "Engineering" {
		t.Errorf("named = %q", got)
	}
	if got := pageSpaceLabel(store.PageLite{SpaceKey: "ENG", SpaceName: ""}); got != "ENG" {
		t.Errorf("fallback = %q want ENG", got)
	}
}

// TestFormatDocsMeta: "author · age · in space" with space-name fallback.
func TestFormatDocsMeta(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	p := store.PageLite{
		Author: "Ada", SpaceKey: "ENG", SpaceName: "",
		UpdatedAt: "2026-08-03T12:00:00Z",
	}
	got := formatDocsMeta(p, now)
	if !strings.Contains(got, "Ada") || !strings.Contains(got, "in ENG") {
		t.Errorf("meta=%q want author and in ENG", got)
	}
	if !strings.Contains(got, "·") {
		t.Errorf("meta=%q want · separators", got)
	}
	p.SpaceName = "Engineering"
	got = formatDocsMeta(p, now)
	if !strings.Contains(got, "in Engineering") || strings.Contains(got, "in ENG") {
		t.Errorf("meta=%q want in Engineering (not key)", got)
	}
	// Empty author drops the author clause (web DocRow).
	p.Author = ""
	got = formatDocsMeta(p, now)
	if strings.HasPrefix(got, " ·") || strings.Contains(got, "(no author)") {
		t.Errorf("empty author should drop clause, got %q", got)
	}
}

// TestDocsDefaultTabUpdated: entering docs mode lands on Updated (flat), not Spaces.
func TestDocsDefaultTabUpdated(t *testing.T) {
	m := seededModel()
	m.pages = samplePages()
	m = feedKey(m, "D")
	if m.mode != modeDocs {
		t.Fatalf("mode=%v", m.mode)
	}
	if m.docsTab != docsTabUpdated {
		t.Fatalf("docsTab=%v want docsTabUpdated", m.docsTab)
	}
	// Flat: no space headers in nav-driving lines; first nav is newest (p-cb).
	for _, ln := range m.docsLines {
		if ln.kind == docsLineHeader {
			t.Fatalf("Updated default should be flat, saw header %q", ln.space)
		}
	}
	if len(m.docsNav) == 0 || m.docsNav[0].page.Key != "p-cb" {
		t.Fatalf("newest first: nav[0]=%v", docsNavKeys(m))
	}
}

// TestDocsTabKeys: 1/2/3 switch Updated / By author / Spaces inside docs mode.
func TestDocsTabKeys(t *testing.T) {
	m := seededDocsModel()
	// seededDocsModel refilters without setting tab; pin Updated then switch.
	m.docsTab = docsTabUpdated
	m.refilterDocs()

	m = feedKey(m, "2")
	if m.docsTab != docsTabByAuthor {
		t.Fatalf("2 → docsTab=%v want by author", m.docsTab)
	}
	var sawAuthorHeader bool
	for _, ln := range m.docsLines {
		if ln.kind == docsLineHeader {
			sawAuthorHeader = true
			break
		}
	}
	if !sawAuthorHeader {
		t.Fatal("by author should insert group headers")
	}

	m = feedKey(m, "3")
	if m.docsTab != docsTabSpaces {
		t.Fatalf("3 → docsTab=%v want spaces", m.docsTab)
	}
	// Spaces tree: ENG then PROD headers (existing buildDocsLines).
	if len(m.docsLines) == 0 || m.docsLines[0].kind != docsLineHeader || m.docsLines[0].space != "ENG" {
		t.Fatalf("spaces first line = %+v", m.docsLines)
	}

	m = feedKey(m, "1")
	if m.docsTab != docsTabUpdated {
		t.Fatalf("1 → docsTab=%v want updated", m.docsTab)
	}
}

// TestDocsHelpMentionsViewedUnsupported: honest parity note for Viewed recency.
func TestDocsHelpMentionsViewedUnsupported(t *testing.T) {
	m := seededDocsModel()
	m.showHelp = true
	plain := stripANSI(m.View())
	// Exact sentence from the parity brief (or a close substring).
	if !strings.Contains(plain, "Viewed recency") &&
		!strings.Contains(strings.ToLower(plain), "does not track visits") {
		t.Errorf("help missing Viewed unsupported honesty:\n%s", plain)
	}
}

// TestFormatDocsExcerpt: empty → omit; present → kept; CJK cut by display width.
func TestFormatDocsExcerpt(t *testing.T) {
	if got := formatDocsExcerpt("", 40); got != "" {
		t.Fatalf("empty excerpt = %q want empty", got)
	}
	if got := formatDocsExcerpt("  hello world  ", 40); got != "hello world" {
		t.Fatalf("trim = %q", got)
	}
	// CJK: each Hangul is 2 cells; maxW 5 → at most 2 chars + ellipsis (or 2 chars).
	long := "한글요약이아주길다"
	got := formatDocsExcerpt(long, 5)
	if got == "" {
		t.Fatal("CJK excerpt should not be empty at maxW=5")
	}
	if w := runewidth.StringWidth(got); w > 5 {
		t.Fatalf("CJK truncate width=%d want <=5 got %q", w, got)
	}
	// ASCII cut with ellipsis when over width.
	got = formatDocsExcerpt("abcdefghijklmnop", 8)
	if runewidth.StringWidth(got) > 8 {
		t.Fatalf("ascii truncate width=%d want <=8 got %q", runewidth.StringWidth(got), got)
	}
}

// TestDocsExcerptRender: Updated/By author show excerpt when present; empty
// omits the second line; Spaces tree never shows excerpts (web parity).
func TestDocsExcerptRender(t *testing.T) {
	pages := []store.PageLite{
		{
			Key: "with", Title: "Has Excerpt", SpaceKey: "ENG", Author: "Ada",
			UpdatedAt: "2026-08-04T10:00:00Z",
			Excerpt:   "Body preview line for the page",
		},
		{
			Key: "empty", Title: "No Excerpt", SpaceKey: "ENG", Author: "Bob",
			UpdatedAt: "2026-08-03T10:00:00Z",
			Excerpt:   "",
		},
	}
	m := newModel(&config.Config{}, nil)
	m.width, m.height = 100, 40
	m.now = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m.pages = pages
	m.mode = modeDocs
	m.docsTab = docsTabUpdated
	m.refilterDocs()

	// Updated: excerpt present under Has Excerpt, absent for empty-excerpt page.
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Has Excerpt") {
		t.Fatalf("missing title:\n%s", plain)
	}
	if !strings.Contains(plain, "Body preview line for the page") {
		t.Fatalf("Updated tab should show excerpt:\n%s", plain)
	}
	// Empty-excerpt page: title appears; no spurious blank excerpt marker.
	if !strings.Contains(plain, "No Excerpt") {
		t.Fatalf("missing empty-excerpt title:\n%s", plain)
	}

	// By author also shows excerpts.
	m.docsTab = docsTabByAuthor
	m.refilterDocs()
	plain = stripANSI(m.View())
	if !strings.Contains(plain, "Body preview line for the page") {
		t.Fatalf("By author tab should show excerpt:\n%s", plain)
	}

	// Spaces tree: no excerpt lines (discovery surface only).
	m.docsTab = docsTabSpaces
	m.refilterDocs()
	plain = stripANSI(m.View())
	if strings.Contains(plain, "Body preview line for the page") {
		t.Fatalf("Spaces tree must not show excerpt:\n%s", plain)
	}
	if docsShowExcerpt(docsTabSpaces) {
		t.Fatal("docsShowExcerpt(spaces) should be false")
	}
	if !docsShowExcerpt(docsTabUpdated) || !docsShowExcerpt(docsTabByAuthor) {
		t.Fatal("docsShowExcerpt should be true for updated/by author")
	}
}

// TestDocsExcerptScreenHeight: empty excerpt is 1 row; non-empty is 2 on
// excerpt tabs; always 1 on Spaces.
func TestDocsExcerptScreenHeight(t *testing.T) {
	with := docsLine{kind: docsLinePage, page: store.PageLite{Excerpt: "preview"}}
	empty := docsLine{kind: docsLinePage, page: store.PageLite{Excerpt: ""}}
	hdr := docsLine{kind: docsLineHeader, space: "ENG", count: 1}
	if docsLineScreenHeight(with, true) != 2 {
		t.Fatal("non-empty excerpt should be 2 rows")
	}
	if docsLineScreenHeight(empty, true) != 1 {
		t.Fatal("empty excerpt should be 1 row")
	}
	if docsLineScreenHeight(with, false) != 1 {
		t.Fatal("spaces tab: always 1 row")
	}
	if docsLineScreenHeight(hdr, true) != 1 {
		t.Fatal("header is always 1 row")
	}
}

// TestDocsExcerptCJKInView: rendered excerpt is cut to terminal cell width.
func TestDocsExcerptCJKInView(t *testing.T) {
	// Narrow width so truncate must fire on a long CJK excerpt.
	pages := []store.PageLite{
		{
			Key: "cjk", Title: "CJK", SpaceKey: "K", Author: "Lee",
			UpdatedAt: "2026-08-04T10:00:00Z",
			Excerpt:   "한글요약이아주아주아주길어서잘려야한다",
		},
	}
	m := newModel(&config.Config{}, nil)
	m.width, m.height = 30, 20
	m.now = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	m.pages = pages
	m.mode = modeDocs
	m.docsTab = docsTabUpdated
	m.refilterDocs()
	plain := stripANSI(m.View())
	// Full uncut excerpt should not appear when terminal is 30 cells wide
	// (indent 4 + long CJK exceeds remaining width).
	full := "한글요약이아주아주아주길어서잘려야한다"
	if strings.Contains(plain, full) {
		t.Fatalf("expected CJK excerpt to be truncated at width=30:\n%s", plain)
	}
	// Some Hangul prefix should still appear.
	if !strings.Contains(plain, "한글") {
		t.Fatalf("expected partial CJK excerpt:\n%s", plain)
	}
}

func docsNavKeys(m Model) []string {
	out := make([]string, len(m.docsNav))
	for i, n := range m.docsNav {
		out[i] = n.page.Key
	}
	return out
}

// --- v0.10 document-wave parity ---

// TestFilterPagesAuthor: haystack includes author (web title+space+author).
func TestFilterPagesAuthor(t *testing.T) {
	pages := samplePages()
	// Bob only authors e-child.
	got := filterPages(pages, "bob")
	if len(got) != 1 || got[0].Key != "e-child" {
		keys := make([]string, len(got))
		for i, p := range got {
			keys[i] = p.Key
		}
		t.Fatalf("author filter bob → %v want [e-child]", keys)
	}
	// Case-insensitive.
	got = filterPages(pages, "ADA")
	if len(got) < 3 {
		t.Fatalf("author ADA should hit Ada's pages, got %d", len(got))
	}
	for _, p := range got {
		if !strings.EqualFold(p.Author, "Ada") {
			t.Errorf("unexpected author match: %s author=%q", p.Key, p.Author)
		}
	}
}

// TestDocsTreeChildCount: parent rows carry unfiltered direct-child counts.
func TestDocsTreeChildCount(t *testing.T) {
	lines := buildDocsLines(samplePages())
	// e-root has one direct child (e-child). e-sib has none.
	var rootCount, sibCount int
	var sawRoot, sawSib bool
	for _, ln := range lines {
		if ln.kind != docsLinePage {
			continue
		}
		switch ln.page.Key {
		case "e-root":
			sawRoot = true
			rootCount = ln.childCount
		case "e-sib":
			sawSib = true
			sibCount = ln.childCount
		case "e-child":
			if ln.childCount != 0 {
				t.Errorf("leaf e-child childCount=%d want 0", ln.childCount)
			}
		}
	}
	if !sawRoot || !sawSib {
		t.Fatalf("missing root/sib in tree lines")
	}
	if rootCount != 1 {
		t.Fatalf("e-root childCount=%d want 1", rootCount)
	}
	if sibCount != 0 {
		t.Fatalf("e-sib childCount=%d want 0", sibCount)
	}
	// Render surfaces the count next to the parent title.
	m := seededDocsModel()
	m.docsTab = docsTabSpaces
	m.refilterDocs()
	plain := stripANSI(m.View())
	// "Root Guide 1" — count rides after the title (web doc-tree-count).
	if !strings.Contains(plain, "Root Guide 1") {
		t.Fatalf("spaces tree missing parent child count:\n%s", plain)
	}
}

// TestDocsTreeFilterPathAncestors: filter keeps ancestors as pathOnly rows;
// child counts stay unfiltered totals.
func TestDocsTreeFilterPathAncestors(t *testing.T) {
	m := seededDocsModel()
	m.docsTab = docsTabSpaces
	m.filter = "child"
	m.refilterDocs()

	var pathRoot, hitChild bool
	var rootChildCount int
	for _, ln := range m.docsLines {
		if ln.kind != docsLinePage {
			continue
		}
		switch ln.page.Key {
		case "e-root":
			if !ln.pathOnly {
				t.Error("e-root should be pathOnly under filter 'child'")
			}
			pathRoot = true
			rootChildCount = ln.childCount
		case "e-child":
			if ln.pathOnly {
				t.Error("e-child is the hit — must not be pathOnly")
			}
			hitChild = true
		case "e-sib", "e-cross", "e-miss", "p-alone", "p-ca", "p-cb":
			t.Errorf("unexpected page under filter: %s", ln.page.Key)
		}
	}
	if !pathRoot || !hitChild {
		t.Fatalf("want path root + hit child; pathRoot=%v hitChild=%v nav=%v",
			pathRoot, hitChild, docsNavKeys(m))
	}
	// Unfiltered total: e-root still has 1 child even though only the hit shows.
	if rootChildCount != 1 {
		t.Fatalf("path parent childCount=%d want unfiltered 1", rootChildCount)
	}
	// Both rows remain navigable (path is not a header).
	if len(m.docsNav) != 2 {
		t.Fatalf("nav n=%d want 2 (path+hit) keys=%v", len(m.docsNav), docsNavKeys(m))
	}
}

// TestDocsFilterMatchHighlight: match rows stay fully legible under NO_COLOR
// (TUI_PRINCIPLES §5 — highlight is optional; the row is the match signal).
// highlightMatch itself is covered in neon_test; here we only assert the docs
// list still shows the full title when a filter is active.
func TestDocsFilterMatchHighlight(t *testing.T) {
	m := seededDocsModel()
	m.docsTab = docsTabUpdated
	m.filter = "Child"
	m.refilterDocs()
	if len(m.docsNav) != 1 || m.docsNav[0].page.Key != "e-child" {
		t.Fatalf("want single Child match, nav=%v", docsNavKeys(m))
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "Child Page") {
		t.Fatalf("match row missing after strip (NO_COLOR must not drop title):\n%s", plain)
	}
	// highlightMatch must not alter the visible characters of a docs title.
	got := highlightMatch("Child Page", "Child", stylePrimary, styleHighlight)
	if stripANSI(got) != "Child Page" {
		t.Fatalf("highlight altered title text: %q", stripANSI(got))
	}
}

// TestDocsPageDetailLabelsAndRefs: labels + related/backlink issue keys on
// page detail.
func TestDocsPageDetailLabelsAndRefs(t *testing.T) {
	m := seededDocsModel()
	m.pageDetail = &store.PageDetail{
		PageLite: store.PageLite{
			Key: "e-child", Title: "Child Page", SpaceKey: "ENG",
			Author: "Bob", Labels: []string{"runbook", "ops"},
		},
		BodyADF:           nil,
		RefIssueKeys:      []string{"NMB-1", "NMB-2"},
		BacklinkIssueKeys: []string{"NMB-9"},
	}
	m.pageDetailKey = "e-child"
	m.mode = modeDocDetail
	plain := stripANSI(m.View())
	for _, want := range []string{
		"labels", "runbook", "ops",
		"Related issues", "NMB-1", "NMB-2",
		"Mentioned from", "NMB-9",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("doc detail missing %q\n%s", want, plain)
		}
	}
}

// TestIssueDetailPageRefs: issue detail surfaces RefPages / BacklinkPages.
func TestIssueDetailPageRefs(t *testing.T) {
	m := seededModel()
	m.width, m.height = 100, 40
	m.mode = modeDetail
	m.detailKey = "NMB-1"
	summary := "Sample bug"
	m.detailLite = &store.IssueLite{IssueKey: "NMB-1", Summary: summary}
	m.detail = &store.Detail{
		IssueKey: "NMB-1",
		RefPages: []store.PageLite{
			{Key: "e-root", Title: "Root Guide", SpaceKey: "ENG", SpaceName: "Engineering"},
		},
		BacklinkPages: []store.PageLite{
			{Key: "e-child", Title: "Child Page", SpaceKey: "ENG"},
		},
	}
	plain := stripANSI(m.View())
	for _, want := range []string{
		"Related pages", "e-root", "Root Guide", "Engineering",
		"Mentioned in", "e-child", "Child Page",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("issue detail missing %q\n%s", want, plain)
		}
	}
}

// TestDocsHelpMentionsAuthorFilter: help parity note updated for author haystack.
func TestDocsHelpMentionsAuthorFilter(t *testing.T) {
	m := seededDocsModel()
	m.showHelp = true
	plain := stripANSI(m.View())
	if !strings.Contains(strings.ToLower(plain), "author") {
		t.Errorf("help should mention author in docs filter note:\n%s", plain)
	}
	if !strings.Contains(strings.ToLower(plain), "label") {
		t.Errorf("help should mention labels honesty:\n%s", plain)
	}
}
