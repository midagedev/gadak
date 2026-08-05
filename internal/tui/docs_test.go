package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	// D enters docs
	m = feedKey(m, "D")
	if m.mode != modeDocs {
		t.Fatalf("D → mode %v want modeDocs", m.mode)
	}
	plain := stripANSI(m.View())
	for _, want := range []string{"ENG", "PROD", "Root Guide", "Child Page"} {
		if !strings.Contains(plain, want) {
			t.Errorf("docs list missing %q\n%s", want, plain)
		}
	}
	// Indent: child line should carry deeper leading spaces than root in plain render.
	// Cursor on first navigable page; j/k moves among pages only.
	if m.docsCursor != 0 {
		t.Fatalf("docs cursor start %d", m.docsCursor)
	}
	m = feedKey(m, "j")
	if m.docsCursor != 1 {
		t.Fatalf("j → docsCursor %d", m.docsCursor)
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

func docsNavKeys(m Model) []string {
	out := make([]string, len(m.docsNav))
	for i, n := range m.docsNav {
		out[i] = n.page.Key
	}
	return out
}
