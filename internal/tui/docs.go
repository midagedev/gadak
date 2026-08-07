package tui

// Docs view: mirrored wiki pages with three list axes (Updated / By author /
// Spaces tree), plain-text detail, and in-memory title/space/author filter
// with match highlighting on list titles. Spaces tree shows direct-child
// counts (unfiltered total) and keeps path ancestors muted while filtering.
// Updated / By author rows show a muted one-line body excerpt when present
// (Spaces tree does not — web parity). Labels appear on page detail only
// (list chips + dedicated label filter stay web-only — narrow width).
// Issue↔page cross-refs surface on both detail panes. Issue-only write
// actions report unsupported rather than no-op. Full-text search, Viewed
// recency, People axis, and URL deeplinks stay on the web UI / CLI.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/store"
)

// docsLine kinds for the docs list viewport (headers are non-navigable).
const (
	docsLineHeader = iota
	docsLinePage
)

// docsTab is which list axis the docs mode shows. Keys 1/2/3 switch these
// (same physical keys as issue status tabs and feed focus — context-bound).
type docsTab int

const (
	docsTabUpdated  docsTab = iota // 1 — flat, updated_at desc (default)
	docsTabByAuthor                // 2 — author group headers
	docsTabSpaces                  // 3 — space-grouped parent_id tree
)

// docsLine is one screen row in the docs list: a group header or a page.
// space holds the header label (space key or author name) when kind is header.
type docsLine struct {
	kind  int
	space string
	count int // header only: pages in the group (or keep-set size under filter)
	page  store.PageLite
	depth int
	// childCount is direct children in the unfiltered space tree (Spaces tab
	// page rows only). Same meaning as the web doc-tree-count badge.
	childCount int
	// pathOnly marks an ancestor kept only so a filter hit has a path (Spaces
	// tab under filter). Rendered muted; still cursor-addressable.
	pathOnly bool
}

// docsNavItem is one cursor-addressable page in tree order (no headers).
type docsNavItem struct {
	page  store.PageLite
	depth int
}

type pageDetailMsg struct {
	key string
	d   *store.PageDetail
	err error
}

// pageSpaceLabel is the on-screen space name: SpaceName, or SpaceKey when empty
// (web pages.spaceLabel parity).
func pageSpaceLabel(p store.PageLite) string {
	if p.SpaceName != "" {
		return p.SpaceName
	}
	return p.SpaceKey
}

// formatDocsMeta builds the dimmed second clause: "author · age · in space".
// Empty clauses are dropped (web DocRow).
func formatDocsMeta(p store.PageLite, now time.Time) string {
	parts := make([]string, 0, 3)
	if p.Author != "" {
		parts = append(parts, p.Author)
	}
	if age := relativeTime(p.UpdatedAt, now); age != "" {
		parts = append(parts, age)
	}
	if space := pageSpaceLabel(p); space != "" {
		parts = append(parts, "in "+space)
	}
	return strings.Join(parts, " · ")
}

// buildDocsUpdatedLines is a flat list sorted by updated_at desc (ISO lex).
// Empty timestamps sort last — same rule as issue list sort.
func buildDocsUpdatedLines(pages []store.PageLite) []docsLine {
	if len(pages) == 0 {
		return nil
	}
	list := make([]store.PageLite, len(pages))
	copy(list, pages)
	sort.SliceStable(list, func(i, j int) bool {
		return lessStr(list[i].UpdatedAt, list[j].UpdatedAt, false)
	})
	out := make([]docsLine, len(list))
	for i, p := range list {
		out[i] = docsLine{kind: docsLinePage, page: p}
	}
	return out
}

// buildDocsByAuthorLines groups by author (empty → "(no author)"), pages within
// a group by updated_at desc, groups ordered by their newest page (web byAuthor).
func buildDocsByAuthorLines(pages []store.PageLite) []docsLine {
	if len(pages) == 0 {
		return nil
	}
	type bucket struct {
		label string
		list  []store.PageLite
	}
	byKey := map[string]*bucket{}
	order := make([]string, 0)
	for _, p := range pages {
		key := p.Author
		label := p.Author
		if key == "" {
			label = "(no author)"
		}
		b, ok := byKey[key]
		if !ok {
			b = &bucket{label: label}
			byKey[key] = b
			order = append(order, key)
		}
		b.list = append(b.list, p)
	}
	for _, b := range byKey {
		sort.SliceStable(b.list, func(i, j int) bool {
			return lessStr(b.list[i].UpdatedAt, b.list[j].UpdatedAt, false)
		})
	}
	// Groups by newest edit first (first page after within-group sort).
	sort.SliceStable(order, func(i, j int) bool {
		ai, aj := byKey[order[i]].list, byKey[order[j]].list
		if len(ai) == 0 || len(aj) == 0 {
			return len(ai) > len(aj)
		}
		return lessStr(ai[0].UpdatedAt, aj[0].UpdatedAt, false)
	})

	out := make([]docsLine, 0, len(pages)+len(order))
	for _, k := range order {
		b := byKey[k]
		out = append(out, docsLine{kind: docsLineHeader, space: b.label, count: len(b.list)})
		for _, p := range b.list {
			out = append(out, docsLine{kind: docsLinePage, page: p})
		}
	}
	return out
}

// buildDocsLines groups pages by space (alpha), nests by parent_id, and flattens
// for display. Rules match the web sidebar (pages.svelte.ts treeBySpace):
// missing / other-space parent → root; parent cycles are bounded and residual
// nodes surface as roots rather than vanishing. Direct-child counts ride on
// each parent row (unfiltered total).
func buildDocsLines(pages []store.PageLite) []docsLine {
	return buildDocsTree(pages, nil)
}

// buildDocsTree is the Spaces-tab list builder. When match is non-nil, only
// hits plus their ancestors are kept (web SpaceDocsView treeKeep); non-hit
// ancestors are pathOnly. Child counts always come from the full children map
// over `pages` (pass the unfiltered index for web-parity totals).
func buildDocsTree(pages []store.PageLite, match map[string]bool) []docsLine {
	if len(pages) == 0 {
		return nil
	}
	byKey := make(map[string]store.PageLite, len(pages))
	groups := map[string][]store.PageLite{}
	for _, p := range pages {
		byKey[p.Key] = p
		groups[p.SpaceKey] = append(groups[p.SpaceKey], p)
	}
	spaces := make([]string, 0, len(groups))
	for s := range groups {
		spaces = append(spaces, s)
	}
	sort.Strings(spaces)

	var out []docsLine
	for _, space := range spaces {
		list := groups[space]
		sort.Slice(list, func(i, j int) bool {
			return list[i].Title < list[j].Title
		})

		children := map[string][]store.PageLite{}
		var roots []store.PageLite
		for _, p := range list {
			parentKey := p.ParentID
			parent, ok := byKey[parentKey]
			if parentKey == "" || !ok || parent.SpaceKey != space {
				roots = append(roots, p)
				continue
			}
			children[parentKey] = append(children[parentKey], p)
		}
		// Siblings keep title order already applied on list; children slices
		// inherit that order because we walked list sorted.
		for k := range children {
			sort.Slice(children[k], func(i, j int) bool {
				return children[k][i].Title < children[k][j].Title
			})
		}
		sort.Slice(roots, func(i, j int) bool {
			return roots[i].Title < roots[j].Title
		})

		// While filtering: every hit plus ancestors that say where it lives
		// (web SpaceDocsView treeKeep). Walk parent_id upward — cycle-safe,
		// same keep set as a post-order mark over an acyclic tree.
		var keep map[string]bool
		if match != nil {
			keep = map[string]bool{}
			for _, p := range list {
				if !match[p.Key] {
					continue
				}
				cur := p
				seen := map[string]bool{}
				for {
					if seen[cur.Key] {
						break
					}
					seen[cur.Key] = true
					keep[cur.Key] = true
					parentKey := cur.ParentID
					if parentKey == "" {
						break
					}
					parent, ok := byKey[parentKey]
					if !ok || parent.SpaceKey != space {
						break
					}
					cur = parent
				}
			}
			if len(keep) == 0 {
				continue
			}
		}

		visited := map[string]bool{}
		var walk func(p store.PageLite, depth int)
		walk = func(p store.PageLite, depth int) {
			if keep != nil && !keep[p.Key] {
				return
			}
			visited[p.Key] = true
			pathOnly := match != nil && !match[p.Key]
			out = append(out, docsLine{
				kind:       docsLinePage,
				page:       p,
				depth:      depth,
				childCount: len(children[p.Key]),
				pathOnly:   pathOnly,
			})
			for _, c := range children[p.Key] {
				if visited[c.Key] {
					continue
				}
				walk(c, depth+1)
			}
		}

		hdrCount := len(list)
		if keep != nil {
			hdrCount = 0
			for _, p := range list {
				if keep[p.Key] {
					hdrCount++
				}
			}
		}
		out = append(out, docsLine{kind: docsLineHeader, space: space, count: hdrCount})
		for _, r := range roots {
			if !visited[r.Key] {
				walk(r, 0)
			}
		}
		// Cycle residual: not reachable from any root.
		for _, p := range list {
			if !visited[p.Key] {
				walk(p, 0)
			}
		}
	}
	return out
}

// filterPages keeps pages whose title, space key/name, page key, or author
// contains needle (case-insensitive). Haystack matches the web docs filter
// (title + space + author); key is kept for keystroke jumps by id.
func filterPages(pages []store.PageLite, needle string) []store.PageLite {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return pages
	}
	out := make([]store.PageLite, 0, len(pages))
	for _, p := range pages {
		hay := strings.ToLower(p.Title + " " + p.SpaceKey + " " + pageSpaceLabel(p) + " " + p.Key + " " + p.Author)
		if strings.Contains(hay, needle) {
			out = append(out, p)
		}
	}
	return out
}

// docsBreadcrumb builds "SPACE › ancestor… › title" for key. Ancestors walk
// parent_id via the full index (any space), cycle-defended — same as web.
func docsBreadcrumb(pages []store.PageLite, key string) string {
	byKey := make(map[string]store.PageLite, len(pages))
	for _, p := range pages {
		byKey[p.Key] = p
	}
	cur, ok := byKey[key]
	if !ok {
		return key
	}
	// Ancestors outermost first.
	var trail []string
	seen := map[string]bool{key: true}
	walk := cur
	for walk.ParentID != "" {
		parent, ok := byKey[walk.ParentID]
		if !ok || seen[parent.Key] {
			break
		}
		seen[parent.Key] = true
		trail = append([]string{parent.Title}, trail...)
		walk = parent
	}
	parts := make([]string, 0, 2+len(trail))
	if cur.SpaceKey != "" {
		parts = append(parts, cur.SpaceKey)
	}
	parts = append(parts, trail...)
	parts = append(parts, cur.Title)
	return strings.Join(parts, " › ")
}

func (m *Model) refilterDocs() {
	filtered := filterPages(m.pages, m.filter)
	var lines []docsLine
	switch m.docsTab {
	case docsTabByAuthor:
		lines = buildDocsByAuthorLines(filtered)
	case docsTabSpaces:
		// Tree child counts and path ancestors need the full index; match set
		// narrows which rows stay visible (web SpaceDocsView treeKeep).
		if strings.TrimSpace(m.filter) == "" {
			lines = buildDocsTree(m.pages, nil)
		} else if len(filtered) == 0 {
			lines = nil
		} else {
			match := make(map[string]bool, len(filtered))
			for _, p := range filtered {
				match[p.Key] = true
			}
			lines = buildDocsTree(m.pages, match)
		}
	default:
		lines = buildDocsUpdatedLines(filtered)
	}
	m.docsLines = lines
	nav := make([]docsNavItem, 0, len(lines))
	for _, ln := range lines {
		if ln.kind == docsLinePage {
			nav = append(nav, docsNavItem{page: ln.page, depth: ln.depth})
		}
	}
	m.docsNav = nav
	if m.docsCursor >= len(m.docsNav) {
		m.docsCursor = max(0, len(m.docsNav)-1)
	}
	if m.docsCursor < 0 {
		m.docsCursor = 0
	}
	m.ensureDocsVisible()
}

func (m *Model) setDocsTab(t docsTab) {
	if m.docsTab == t {
		return
	}
	m.docsTab = t
	m.docsCursor = 0
	m.docsOffset = 0
	m.refilterDocs()
}

func (m *Model) enterDocs() {
	m.mode = modeDocs
	m.filter = ""
	m.docsTab = docsTabUpdated // docs mode default: Updated (not Spaces tree)
	m.docsCursor = 0
	m.docsOffset = 0
	m.refilterDocs()
}

func (m *Model) leaveDocs() {
	m.mode = modeList
	m.filter = ""
	m.pageDetail = nil
	m.pageDetailKey = ""
}

func (m *Model) moveDocsCursor(delta int) {
	n := len(m.docsNav)
	if n == 0 {
		m.docsCursor = 0
		return
	}
	m.docsCursor += delta
	if m.docsCursor < 0 {
		m.docsCursor = 0
	}
	if m.docsCursor >= n {
		m.docsCursor = n - 1
	}
	m.ensureDocsVisible()
}

// docsShowExcerpt reports whether the docs tab shows a body excerpt under each
// page row. Matches the web UI: Updated / By author only — Spaces tree and the
// web-only Viewed tab never show previews.
func docsShowExcerpt(tab docsTab) bool {
	return tab == docsTabUpdated || tab == docsTabByAuthor
}

// docsLineScreenHeight is how many terminal rows a docs list entry occupies.
// Pages with a non-empty excerpt take two rows on Updated / By author tabs.
func docsLineScreenHeight(ln docsLine, showExcerpt bool) int {
	if ln.kind != docsLinePage || !showExcerpt {
		return 1
	}
	if strings.TrimSpace(ln.page.Excerpt) == "" {
		return 1
	}
	return 2
}

// docsCursorScreenLine maps docsCursor onto docsLines (headers shift indices).
func (m Model) docsCursorScreenLine() int {
	if len(m.docsNav) == 0 || m.docsCursor < 0 || m.docsCursor >= len(m.docsNav) {
		return 0
	}
	key := m.docsNav[m.docsCursor].page.Key
	for i, ln := range m.docsLines {
		if ln.kind == docsLinePage && ln.page.Key == key {
			return i
		}
	}
	return 0
}

func (m *Model) ensureDocsVisible() {
	h := m.listHeight()
	if h < 1 {
		h = 1
	}
	line := m.docsCursorScreenLine()
	if line < m.docsOffset {
		m.docsOffset = line
	}
	// docsOffset is a docsLines index; account for multi-row excerpt lines so
	// the cursor page (title + optional excerpt) fully fits in the viewport.
	show := docsShowExcerpt(m.docsTab)
	for m.docsOffset <= line {
		rows := 0
		for i := m.docsOffset; i <= line && i < len(m.docsLines); i++ {
			rows += docsLineScreenHeight(m.docsLines[i], show)
		}
		if rows <= h {
			break
		}
		if m.docsOffset >= line {
			break
		}
		m.docsOffset++
	}
	if m.docsOffset < 0 {
		m.docsOffset = 0
	}
}

// formatDocsExcerpt is the muted second line under a page row: whitespace-
// trimmed body preview, cut to maxW terminal cells (CJK-safe via truncate).
// Empty excerpt → empty string (caller omits the line — web parity).
func formatDocsExcerpt(excerpt string, maxW int) string {
	s := strings.TrimSpace(excerpt)
	if s == "" || maxW <= 0 {
		return ""
	}
	return truncate(s, maxW)
}

func (m Model) selectedDocKey() (string, bool) {
	if m.mode == modeDocDetail && m.pageDetailKey != "" {
		return m.pageDetailKey, true
	}
	if len(m.docsNav) == 0 || m.docsCursor < 0 || m.docsCursor >= len(m.docsNav) {
		return "", false
	}
	return m.docsNav[m.docsCursor].page.Key, true
}

func (m Model) loadPageDetailCmd(key string) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if db == nil {
			return pageDetailMsg{key: key, err: fmt.Errorf("no database")}
		}
		d, err := db.PageDetail(key)
		if err != nil {
			return pageDetailMsg{key: key, err: err}
		}
		if d == nil {
			return pageDetailMsg{key: key, err: fmt.Errorf("page not found")}
		}
		return pageDetailMsg{key: key, d: d}
	}
}

func (m Model) docsUnsupported(action string) tea.Cmd {
	return toast("unsupported in docs view: "+action, true)
}

func (m Model) handleDocsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp = true
		return m, nil
	case key.Matches(msg, k.Docs):
		m.leaveDocs()
		return m, nil
	case key.Matches(msg, k.Back), msg.String() == "esc":
		if m.filter != "" {
			m.filter = ""
			m.refilterDocs()
			return m, nil
		}
		m.leaveDocs()
		return m, nil
	case key.Matches(msg, k.Filter):
		m.filterFrom = modeDocs
		m.mode = modeFilter
		return m, nil
	case key.Matches(msg, k.Down):
		m.moveDocsCursor(1)
		return m, nil
	case key.Matches(msg, k.Up):
		m.moveDocsCursor(-1)
		return m, nil
	case key.Matches(msg, k.Top):
		m.docsCursor = 0
		m.ensureDocsVisible()
		return m, nil
	case key.Matches(msg, k.Bottom):
		if n := len(m.docsNav); n > 0 {
			m.docsCursor = n - 1
			m.ensureDocsVisible()
		}
		return m, nil
	case key.Matches(msg, k.PageDown):
		m.moveDocsCursor(m.pageSize())
		return m, nil
	case key.Matches(msg, k.PageUp):
		m.moveDocsCursor(-m.pageSize())
		return m, nil
	// 1/2/3 reuse issue/feed tab keys; in docs they mean Updated / By author / Spaces.
	case key.Matches(msg, k.TabAll):
		m.setDocsTab(docsTabUpdated)
		return m, nil
	case key.Matches(msg, k.TabOpen):
		m.setDocsTab(docsTabByAuthor)
		return m, nil
	case key.Matches(msg, k.TabInProgress):
		m.setDocsTab(docsTabSpaces)
		return m, nil
	case key.Matches(msg, k.Enter):
		key, ok := m.selectedDocKey()
		if !ok {
			return m, nil
		}
		if m.db == nil {
			// Tests inject pageDetail; without DB stay put unless already set.
			return m, nil
		}
		return m, m.loadPageDetailCmd(key)
	case key.Matches(msg, k.Refresh):
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.reloadCmd())
	case key.Matches(msg, k.Comment):
		return m, m.docsUnsupported("comment")
	case key.Matches(msg, k.Transition):
		return m, m.docsUnsupported("transition")
	case key.Matches(msg, k.Assignee):
		return m, m.docsUnsupported("assignee")
	case key.Matches(msg, k.Edit):
		return m, m.docsUnsupported("edit")
	case key.Matches(msg, k.Watch):
		return m, m.docsUnsupported("watch")
	case key.Matches(msg, k.Feed):
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.loadFeedCmd())
	case key.Matches(msg, k.Views):
		return m, m.docsUnsupported("saved views")
	}
	return m, nil
}

func (m Model) handleDocDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp = true
		return m, nil
	case key.Matches(msg, k.Back), msg.String() == "esc":
		m.mode = modeDocs
		m.pageDetail = nil
		m.pageDetailKey = ""
		return m, nil
	case key.Matches(msg, k.Docs):
		m.leaveDocs()
		return m, nil
	case key.Matches(msg, k.Refresh):
		if m.pageDetailKey != "" && m.db != nil {
			return m, m.loadPageDetailCmd(m.pageDetailKey)
		}
		return m, nil
	case key.Matches(msg, k.Comment):
		return m, m.docsUnsupported("comment")
	case key.Matches(msg, k.Transition):
		return m, m.docsUnsupported("transition")
	case key.Matches(msg, k.Assignee):
		return m, m.docsUnsupported("assignee")
	case key.Matches(msg, k.Edit):
		return m, m.docsUnsupported("edit")
	case key.Matches(msg, k.Watch):
		return m, m.docsUnsupported("watch")
	}
	return m, nil
}

func (m Model) viewDocs() string {
	var b strings.Builder
	w := m.width

	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')
	b.WriteString(fitWidth(m.renderDocsTabs(), w))
	b.WriteByte('\n')

	listH := m.listHeight()
	if len(m.pages) == 0 {
		empty := styleMuted.Render("  no documents mirrored")
		for i := 0; i < listH; i++ {
			if i == 0 {
				b.WriteString(fitWidth(empty, w))
			}
			b.WriteByte('\n')
		}
	} else if len(m.docsNav) == 0 {
		empty := styleMuted.Render("  no matches — press / to filter · D to leave")
		for i := 0; i < listH; i++ {
			if i == 0 {
				b.WriteString(fitWidth(empty, w))
			}
			b.WriteByte('\n')
		}
	} else {
		showEx := docsShowExcerpt(m.docsTab)
		selKey := ""
		if m.docsCursor >= 0 && m.docsCursor < len(m.docsNav) {
			selKey = m.docsNav[m.docsCursor].page.Key
		}
		// Fill listH terminal rows. Excerpt pages cost two rows; stop before
		// starting a row that cannot fully fit (except when the viewport is
		// shorter than the first line alone — draw the title only then).
		used := 0
		for i := m.docsOffset; i < len(m.docsLines) && used < listH; i++ {
			ln := m.docsLines[i]
			lh := docsLineScreenHeight(ln, showEx)
			if used+lh > listH && used > 0 {
				break
			}
			if ln.kind == docsLineHeader {
				// Same grammar as issue group headers / spaces tree.
				hdr := fmt.Sprintf(" ▸ %s (%d)", ln.space, ln.count)
				b.WriteString(fitWidth(styleMuted.Render(hdr), w))
				b.WriteByte('\n')
				used++
				continue
			}
			// Indent by depth (Spaces tree only); title + optional child count +
			// dimmed meta. Filter matches highlight the title (styleHighlight —
			// same palette as the issue list; NO_COLOR drops colour safely).
			indent := strings.Repeat("  ", ln.depth)
			title := ln.page.Title
			if title == "" {
				title = ln.page.Key
			}
			prefix := "  " + indent
			meta := formatDocsMeta(ln.page, m.clock())
			if meta != "" {
				meta = "  " + meta
			}
			selected := ln.page.Key == selKey
			// Path ancestors under filter recede (web data-hit=false); hits and
			// unfiltered rows stay primary weight.
			pathMuted := ln.pathOnly && !selected

			matchBg := styleHighlight
			if selected {
				matchBg = matchBg.Background(colAccentBg)
			}
			hl := func(plain string, base lipgloss.Style) string {
				if m.filter == "" || pathMuted {
					return base.Render(plain)
				}
				return highlightMatch(plain, m.filter, base, matchBg)
			}

			var titlePart, countPart, metaPart string
			if selected {
				titlePart = hl(title, styleSel)
				if ln.childCount > 0 {
					countPart = styleSelMuted.Render(fmt.Sprintf(" %d", ln.childCount))
				}
				metaPart = styleSelMuted.Render(meta)
				inner := styleSel.Render(prefix) + titlePart + countPart + metaPart
				b.WriteString(styleSel.Width(w).MaxWidth(w).Render(inner))
			} else if pathMuted {
				// Whole path row muted — location context, not an answer.
				label := prefix + title
				if ln.childCount > 0 {
					label += fmt.Sprintf(" %d", ln.childCount)
				}
				b.WriteString(fitWidth(styleMuted.Render(label+meta), w))
			} else {
				titlePart = hl(title, stylePrimary)
				if ln.childCount > 0 {
					countPart = styleMuted.Render(fmt.Sprintf(" %d", ln.childCount))
				}
				metaPart = styleMuted.Render(meta)
				line := stylePrimary.Render(prefix) + titlePart + countPart + metaPart
				b.WriteString(fitWidth(line, w))
			}
			b.WriteByte('\n')
			used++
			// Optional muted excerpt under Updated / By author page rows.
			if showEx && used < listH {
				exW := w - 4
				if exW < 1 {
					exW = 1
				}
				ex := formatDocsExcerpt(ln.page.Excerpt, exW)
				if ex != "" {
					exLabel := "    " + ex
					if selected {
						b.WriteString(styleSel.Width(w).MaxWidth(w).Render(styleSelMuted.Render(exLabel)))
					} else {
						b.WriteString(fitWidth(styleMuted.Render(exLabel), w))
					}
					b.WriteByte('\n')
					used++
				}
			}
		}
		for used < listH {
			b.WriteByte('\n')
			used++
		}
	}

	b.WriteString(m.renderDocsStatusBar(w))
	return b.String()
}

// renderDocsTabs draws the three docs list axes (reuses list tab styles).
func (m Model) renderDocsTabs() string {
	tabs := []struct {
		tab   docsTab
		label string
	}{
		{docsTabUpdated, "updated"},
		{docsTabByAuthor, "by author"},
		{docsTabSpaces, "spaces"},
	}
	parts := make([]string, 0, 3)
	for i, t := range tabs {
		label := fmt.Sprintf("%d %s", i+1, t.label)
		if t.tab == m.docsTab {
			parts = append(parts, styleTabActive.Render(label))
		} else {
			parts = append(parts, styleTabInactive.Render(label))
		}
	}
	s := " " + strings.Join(parts, " ")
	if n := len(m.pages); n > 0 {
		s += " " + styleMuted.Render(fmt.Sprintf("%d mirrored", n))
	}
	return s
}

func (m Model) renderDocsStatusBar(w int) string {
	leftParts := make([]string, 0, 4)
	if m.loading || m.busy {
		leftParts = append(leftParts, m.spin.View())
	}
	if w < 40 {
		leftParts = append(leftParts, styleHelp.Render("j/k enter / D q"))
	} else {
		leftParts = append(leftParts, styleHelp.Render("j/k · / filter · enter · D leave · ? · q"))
	}
	if m.filter != "" || m.mode == modeFilter {
		f := m.filter
		if m.mode == modeFilter {
			f += "▌"
		}
		leftParts = append(leftParts, styleFilter.Render("/"+f))
	}
	if m.toast != "" && m.clock().Sub(m.toastAt) < 8*time.Second {
		if m.toastErr {
			leftParts = append(leftParts, styleToastErr.Render(m.toast))
		} else {
			leftParts = append(leftParts, styleToastOK.Render(m.toast))
		}
	}
	left := strings.Join(leftParts, "  ")

	count := "0/0"
	if n := len(m.docsNav); n > 0 {
		count = fmt.Sprintf("%d/%d", m.docsCursor+1, n)
	}
	right := styleMuted.Render(m.syncedLabel + "  ·  " + count)
	return joinBar(left, right, w)
}

func (m Model) viewDocDetail() string {
	var b strings.Builder
	w := m.width
	key := m.pageDetailKey

	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')

	innerW := w - 4
	if innerW < 20 {
		innerW = 20
	}

	var body strings.Builder
	// Breadcrumb: space › ancestors › title
	crumb := docsBreadcrumb(m.pages, key)
	if crumb == "" || crumb == key {
		if m.pageDetail != nil {
			crumb = docsBreadcrumb([]store.PageLite{m.pageDetail.PageLite}, key)
			if m.pageDetail.SpaceKey != "" && m.pageDetail.Title != "" && !strings.Contains(crumb, "›") {
				crumb = m.pageDetail.SpaceKey + " › " + m.pageDetail.Title
			}
		}
	}
	body.WriteString(stylePrimary.Bold(true).Render(crumb))
	body.WriteByte('\n')

	if m.pageDetail != nil {
		body.WriteString(m.renderDetailField("author", orDash(m.pageDetail.Author)))
		body.WriteByte('\n')
		labelVal := "—"
		if len(m.pageDetail.Labels) > 0 {
			chips := make([]string, 0, len(m.pageDetail.Labels))
			for _, l := range m.pageDetail.Labels {
				chips = append(chips, styleChip.Render(l))
			}
			labelVal = strings.Join(chips, " ")
		}
		body.WriteString(styleDetailLabel.Render("labels") + " " + labelVal)
		body.WriteByte('\n')
		body.WriteString(m.renderDetailField("updated", orDash(relativeTime(m.pageDetail.UpdatedAt, m.clock()))))
		body.WriteByte('\n')
		body.WriteByte('\n')

		body.WriteString(styleSection.Render("Body"))
		body.WriteByte('\n')
		// Store exposes body_adf for detail; flatten like issue Description.
		// (items.body_text is FTS-only and not on PageDetail.)
		text := jira.PlainText(m.pageDetail.BodyADF)
		if text == "" {
			body.WriteString(styleMuted.Render("  (empty)"))
			body.WriteByte('\n')
		} else {
			for _, line := range wrapLines(text, innerW-2, 40) {
				body.WriteString("  ")
				body.WriteString(line)
				body.WriteByte('\n')
			}
		}
		body.WriteByte('\n')

		body.WriteString(styleSection.Render("Comments"))
		body.WriteByte('\n')
		comments := m.pageDetail.Comments
		if len(comments) == 0 {
			body.WriteString(styleMuted.Render("  (none)"))
			body.WriteByte('\n')
		} else {
			for _, c := range comments {
				who := c.Author
				if who == "" {
					who = "?"
				}
				when := relativeTime(c.CreatedAt, m.clock())
				body.WriteString(styleDim.Render(fmt.Sprintf("  %s · %s", who, when)))
				body.WriteByte('\n')
				ct := c.BodyText
				if ct == "" {
					ct = jira.PlainText(c.BodyADF)
				}
				if ct == "" {
					body.WriteString(styleMuted.Render("    (empty)"))
					body.WriteByte('\n')
					continue
				}
				for _, line := range wrapLines(ct, innerW-4, 8) {
					body.WriteString("    ")
					body.WriteString(line)
					body.WriteByte('\n')
				}
			}
		}

		// Issue keys this page mentions / that mention this page (item_refs).
		// Omit empty sections — same omitempty spirit as the store JSON.
		if keys := m.pageDetail.RefIssueKeys; len(keys) > 0 {
			body.WriteByte('\n')
			body.WriteString(styleSection.Render("Related issues"))
			body.WriteByte('\n')
			for _, k := range keys {
				body.WriteString("  ")
				body.WriteString(styleKey.Render(k))
				body.WriteByte('\n')
			}
		}
		if keys := m.pageDetail.BacklinkIssueKeys; len(keys) > 0 {
			body.WriteByte('\n')
			body.WriteString(styleSection.Render("Mentioned from"))
			body.WriteByte('\n')
			for _, k := range keys {
				body.WriteString("  ")
				body.WriteString(styleKey.Render(k))
				body.WriteByte('\n')
			}
		}
	} else {
		body.WriteString(styleMuted.Render("loading…"))
	}

	panel := styleDetailPanel.Width(w - 2).Render(strings.TrimRight(body.String(), "\n"))
	b.WriteString(panel)
	b.WriteByte('\n')

	footer := styleHelp.Render("esc back  D docs/list  ? help  q")
	if m.toast != "" && m.clock().Sub(m.toastAt) < 8*time.Second {
		var t string
		if m.toastErr {
			t = styleToastErr.Render(m.toast)
		} else {
			t = styleToastOK.Render(m.toast)
		}
		footer = joinBar(footer, t, w)
	}
	b.WriteString(fitWidth(footer, w))
	return b.String()
}
