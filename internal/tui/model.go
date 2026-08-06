package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/selfupdate"
	"github.com/midagedev/scry/internal/store"
)

// mode is the top-level UI state.
type mode int

const (
	modeList mode = iota
	modeFilter
	modeDetail
	modeForm
	modeFeed
	modeViews
	modeDocs
	modeDocDetail
)

// Model is the bubbletea model for the issue navigator.
type Model struct {
	cfg *config.Config
	db  *store.DB
	// version is the running build; updateNotice is set when a newer release
	// is published (checked async at start, cached daily, silent on failure).
	version      string
	updateNotice string

	all     []row
	visible []int // indices into all
	cursor  int
	offset  int // scroll top for the list viewport

	filter    string
	tab       Tab
	mode      mode
	detail    *store.Detail
	detailKey string
	// detailLite is the list row shown in the detail header (status, assignee…).
	detailLite *store.IssueLite
	// detailFrom remembers which mode opened detail (list or feed) for Esc.
	detailFrom mode

	form       *huh.Form
	formSubmit func() tea.Cmd
	formTitle  string

	width, height int
	toast         string
	toastErr      bool
	toastAt       time.Time
	loadErr       error
	syncedLabel   string
	now           time.Time // injectable for tests
	keys          keyMap

	// Visual-only progress: spinner runs while loading (refresh) or busy (write).
	spin    spinner.Model
	loading bool
	busy    bool

	// Help overlay (?).
	showHelp bool

	// Personal feed (F).
	feedItems  []store.FeedItem
	feedFocus  store.FeedFocus
	feedCounts store.FeedUnreadCounts
	feedCursor int
	feedOffset int

	// Saved views (v).
	savedViews  []store.SavedView
	viewCursor  int
	viewName    string
	viewNote    string
	extraFilter listFilter // from saved view: categories, unassigned, assignee email
	listSort    listSort   // from saved view display.sort / dir
	groupBy     string     // from saved view display.group_by

	// Watches (w) — issue keys currently watched.
	watches map[string]bool

	// Ambient animation. animOn is decided once at startup (Run sets it from
	// animEnabled(); tests leave it false so views render static). animPhase
	// advances ~0.045 per 110ms tick and drives every shimmer/pulse/glow.
	animOn    bool
	animPhase float64

	// Command palette (ctrl+k). Nil when closed.
	pal *paletteState

	// Mirrored wiki pages (docs view, D).
	pages         []store.PageLite
	docsLines     []docsLine
	docsNav       []docsNavItem
	docsCursor    int
	docsOffset    int
	pageDetail    *store.PageDetail
	pageDetailKey string
	// filterFrom remembers which mode opened / so Esc/Enter return there.
	filterFrom mode
}

// newModel builds a model without loading data (tests inject rows).
func newModel(cfg *config.Config, db *store.DB) Model {
	if cfg == nil {
		cfg = &config.Config{}
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleSpinner
	return Model{
		cfg:       cfg,
		db:        db,
		tab:       TabAll,
		mode:      modeList,
		now:       time.Now(),
		keys:      defaultKeys(),
		spin:      sp,
		loading:   db != nil, // initial mirror load; tests inject rows with db==nil
		watches:   map[string]bool{},
		feedFocus: store.FeedFocusAll,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.reloadCmd(), m.checkUpdateCmd()}
	if m.loading {
		cmds = append(cmds, m.spin.Tick)
	}
	if m.animOn {
		cmds = append(cmds, animTickCmd())
	}
	return tea.Batch(cmds...)
}

// animTickMsg is the global ambient-animation heartbeat (~9 fps). It keeps
// firing while the program runs so the surface breathes even when idle.
type animTickMsg struct{}

func animTickCmd() tea.Cmd {
	return tea.Tick(110*time.Millisecond, func(time.Time) tea.Msg { return animTickMsg{} })
}

type updateNoticeMsg struct{ latest string }

// checkUpdateCmd asks selfupdate (daily cache) whether a newer release exists.
// Runs off the UI loop; failures and opt-out yield no message at all.
func (m Model) checkUpdateCmd() tea.Cmd {
	cfg, version := m.cfg, m.version
	return func() tea.Msg {
		if cfg == nil || !cfg.UpdateCheckEnabled() {
			return nil
		}
		dir, err := config.Dir()
		if err != nil {
			return nil
		}
		info, ok := selfupdate.Check(context.Background(), dir, version, true)
		if !ok || !selfupdate.Newer(version, info.Latest) {
			return nil
		}
		return updateNoticeMsg{latest: info.Latest}
	}
}

type loadedMsg struct {
	rows        []row
	pages       []store.PageLite
	syncedLabel string
	watches     map[string]bool
	feedUnread  int
	err         error
}

type detailMsg struct {
	key  string
	lite store.IssueLite
	d    *store.Detail
	err  error
}

type watchToggledMsg struct {
	key string
	on  bool
	err error
}

func (m Model) reloadCmd() tea.Cmd {
	db := m.db
	me := m.feedMe()
	return func() tea.Msg {
		if db == nil {
			return loadedMsg{err: fmt.Errorf("no database")}
		}
		lites, err := db.IssueLites()
		if err != nil {
			return loadedMsg{err: err}
		}
		pages, err := db.PageLites()
		if err != nil {
			return loadedMsg{err: err}
		}
		label := "never synced"
		if st, err := db.SyncState("jira"); err == nil {
			if st.SyncedAt != nil && *st.SyncedAt != "" {
				label = "synced " + relativeTime(*st.SyncedAt, time.Now())
			} else if st.Watermark != "" {
				label = "watermark " + relativeTime(st.Watermark, time.Now())
			}
		}
		watchMap := map[string]bool{}
		if keys, err := db.Watches(); err == nil {
			for _, k := range keys {
				watchMap[k] = true
			}
		}
		feedUnread := 0
		if res, err := db.Feed(store.FeedOpts{Me: me, Limit: 1}); err == nil {
			// UnreadCounts is over the full event set, not the limit slice.
			feedUnread = res.UnreadCounts.All
		}
		return loadedMsg{
			rows:        buildRows(lites),
			pages:       pages,
			syncedLabel: label,
			watches:     watchMap,
			feedUnread:  feedUnread,
		}
	}
}

func (m Model) loadDetailCmd(key string, lite store.IssueLite) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if db == nil {
			return detailMsg{key: key, lite: lite, err: fmt.Errorf("no database")}
		}
		d, err := db.Detail(key)
		return detailMsg{key: key, lite: lite, d: d, err: err}
	}
}

func (m Model) toggleWatchCmd(issueKey string, on bool) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if db == nil {
			return watchToggledMsg{key: issueKey, err: fmt.Errorf("no database")}
		}
		if err := db.SetWatch(issueKey, on); err != nil {
			return watchToggledMsg{key: issueKey, err: err}
		}
		return watchToggledMsg{key: issueKey, on: on}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case updateNoticeMsg:
		m.updateNotice = msg.latest
		return m, nil

	case loadedMsg:
		m.loading = false
		m.loadErr = msg.err
		if msg.err == nil {
			m.all = msg.rows
			if msg.pages != nil {
				m.pages = msg.pages
			} else {
				m.pages = []store.PageLite{}
			}
			m.syncedLabel = msg.syncedLabel
			if msg.watches != nil {
				m.watches = msg.watches
			}
			// List status chip uses All only; other focus counts fill when feed opens.
			m.feedCounts.All = msg.feedUnread
			m.refilter()
			if m.mode == modeDocs || m.mode == modeDocDetail {
				m.refilterDocs()
			}
		}
		return m, nil

	case pageDetailMsg:
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
			m.toastAt = m.clock()
			m.mode = modeDocs
			return m, nil
		}
		m.pageDetail = msg.d
		m.pageDetailKey = msg.key
		m.mode = modeDocDetail
		return m, nil

	case feedLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
			m.toastAt = m.clock()
			m.mode = modeList
			return m, nil
		}
		m.feedItems = msg.items
		if m.feedItems == nil {
			m.feedItems = []store.FeedItem{}
		}
		m.feedCounts = msg.counts
		if m.feedCursor >= len(m.feedItems) {
			m.feedCursor = max(0, len(m.feedItems)-1)
		}
		m.ensureFeedVisible()
		m.mode = modeFeed
		return m, nil

	case feedMarkedMsg:
		m.loading = false
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
			m.toastAt = m.clock()
			return m, nil
		}
		m.feedCounts = msg.counts
		m.toast = "feed marked read"
		m.toastErr = false
		m.toastAt = m.clock()
		// Reload items so read_at stamps refresh.
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.loadFeedCmd())

	case viewsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
			m.toastAt = m.clock()
			m.mode = modeList
			return m, nil
		}
		m.savedViews = msg.views
		if m.savedViews == nil {
			m.savedViews = []store.SavedView{}
		}
		m.viewCursor = 0
		m.mode = modeViews
		return m, nil

	case watchToggledMsg:
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
			m.toastAt = m.clock()
			return m, nil
		}
		if m.watches == nil {
			m.watches = map[string]bool{}
		}
		if msg.on {
			m.watches[msg.key] = true
			m.toast = msg.key + " watching"
		} else {
			delete(m.watches, msg.key)
			m.toast = msg.key + " unwatched"
		}
		m.toastErr = false
		m.toastAt = m.clock()
		return m, nil

	case detailMsg:
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
			m.toastAt = m.clock()
			if m.detailFrom == modeFeed {
				m.mode = modeFeed
			} else {
				m.mode = modeList
			}
			return m, nil
		}
		m.detail = msg.d
		m.detailKey = msg.key
		lite := msg.lite
		m.detailLite = &lite
		m.mode = modeDetail
		return m, nil

	case openFormMsg:
		m.busy = false
		m.form = msg.form.WithWidth(min(m.width-4, 72)).WithHeight(min(m.height-6, 16))
		m.formSubmit = msg.submit
		m.formTitle = msg.title
		m.mode = modeForm
		return m, m.form.Init()

	case formResultMsg:
		m.busy = false
		m.mode = modeList
		m.form = nil
		m.formSubmit = nil
		if msg.err != nil {
			m.toast = msg.err.Error()
			m.toastErr = true
		} else if msg.note != "" {
			m.toast = msg.note
			m.toastErr = false
		}
		m.toastAt = m.clock()
		if msg.key != "" {
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.reloadCmd())
		}
		return m, nil

	case toastMsg:
		m.busy = false
		m.toast = msg.text
		m.toastErr = msg.err
		m.toastAt = m.clock()
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.busy {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case animTickMsg:
		if !m.animOn {
			return m, nil
		}
		m.animPhase += 0.045
		return m, animTickCmd()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward other messages to an active huh form (e.g. blur events).
	if m.mode == modeForm && m.form != nil {
		return m.updateForm(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeForm && m.form != nil {
		return m.updateForm(msg)
	}

	keyStr := msg.String()

	// Global quit always works outside forms (form uses ctrl+c via huh).
	if keyStr == "ctrl+c" {
		return m, tea.Quit
	}

	// Command palette: ctrl+k toggles from anywhere outside forms; while open
	// it consumes every key.
	if m.pal != nil {
		return m.handlePaletteKey(msg)
	}
	if keyStr == "ctrl+k" && m.mode != modeFilter {
		m.openPalette()
		return m, nil
	}

	// Help overlay: ? toggles; esc/? closes; other keys ignored while open
	// except quit which already handled.
	if m.showHelp {
		k := m.keys
		if key.Matches(msg, k.Help) || key.Matches(msg, k.Back) || keyStr == "esc" {
			m.showHelp = false
			return m, nil
		}
		if key.Matches(msg, k.Quit) {
			return m, tea.Quit
		}
		return m, nil
	}

	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modeDetail:
		return m.handleDetailKey(msg)
	case modeFeed:
		return m.handleFeedKey(msg)
	case modeViews:
		return m.handleViewsKey(msg)
	case modeDocs:
		return m.handleDocsKey(msg)
	case modeDocDetail:
		return m.handleDocDetailKey(msg)
	default:
		return m.handleListKey(msg)
	}
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp = true
		return m, nil
	case key.Matches(msg, k.Feed):
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.loadFeedCmd())
	case key.Matches(msg, k.Docs):
		m.enterDocs()
		return m, nil
	case key.Matches(msg, k.Views):
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.loadViewsCmd())
	case key.Matches(msg, k.Watch):
		return m, m.toggleWatchSelected()
	case key.Matches(msg, k.Refresh):
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.reloadCmd())
	case key.Matches(msg, k.Filter):
		m.filterFrom = modeList
		m.mode = modeFilter
		return m, nil
	case key.Matches(msg, k.ClearFilter):
		if m.filter != "" || m.hasSavedViewState() {
			m.filter = ""
			m.clearSavedViewFilters()
			m.refilter()
		}
		return m, nil
	case key.Matches(msg, k.Down):
		m.moveCursor(1)
		return m, nil
	case key.Matches(msg, k.Up):
		m.moveCursor(-1)
		return m, nil
	case key.Matches(msg, k.Top):
		m.cursor = 0
		m.ensureVisible()
		return m, nil
	case key.Matches(msg, k.Bottom):
		if n := len(m.visible); n > 0 {
			m.cursor = n - 1
			m.ensureVisible()
		}
		return m, nil
	case key.Matches(msg, k.PageDown):
		m.moveCursor(m.pageSize())
		return m, nil
	case key.Matches(msg, k.PageUp):
		m.moveCursor(-m.pageSize())
		return m, nil
	case key.Matches(msg, k.TabAll):
		m.setTab(TabAll)
		return m, nil
	case key.Matches(msg, k.TabOpen):
		m.setTab(TabOpen)
		return m, nil
	case key.Matches(msg, k.TabInProgress):
		m.setTab(TabInProgress)
		return m, nil
	case key.Matches(msg, k.TabDone):
		m.setTab(TabDone)
		return m, nil
	case key.Matches(msg, k.Enter):
		issueKey, ok := m.selectedKey()
		if !ok {
			return m, nil
		}
		lite := m.all[m.visible[m.cursor]].lite
		m.detailFrom = modeList
		return m, m.loadDetailCmd(issueKey, lite)
	case key.Matches(msg, k.Comment):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startComment())
	case key.Matches(msg, k.Transition):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startTransition())
	case key.Matches(msg, k.Assignee):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startAssignee())
	case key.Matches(msg, k.Edit):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startFieldEdit())
	}
	return m, nil
}

func (m Model) toggleWatchSelected() tea.Cmd {
	key, ok := m.selectedKey()
	if !ok {
		return toast("no issue selected", true)
	}
	if m.db == nil {
		return toast("no database", true)
	}
	on := !m.watches[key]
	return m.toggleWatchCmd(key, on)
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	leave := func() mode {
		if m.filterFrom == modeDocs {
			return modeDocs
		}
		return modeList
	}
	switch msg.String() {
	case "esc", "enter":
		m.mode = leave()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if m.filter != "" {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			if m.filterFrom == modeDocs {
				m.refilterDocs()
			} else {
				m.refilter()
			}
		}
		return m, nil
	case "ctrl+u":
		m.filter = ""
		if m.filterFrom == modeDocs {
			m.refilterDocs()
		} else {
			m.refilter()
		}
		return m, nil
	default:
		// Printable runes only — ignore chords.
		if msg.Type == tea.KeyRunes {
			m.filter += string(msg.Runes)
			if m.filterFrom == modeDocs {
				m.refilterDocs()
			} else {
				m.refilter()
			}
		}
		return m, nil
	}
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.showHelp = true
		return m, nil
	case key.Matches(msg, k.Watch):
		if m.detailKey == "" {
			return m, toast("no issue selected", true)
		}
		if m.db == nil {
			return m, toast("no database", true)
		}
		on := !m.watches[m.detailKey]
		return m, m.toggleWatchCmd(m.detailKey, on)
	case key.Matches(msg, k.Back), msg.String() == "esc":
		if m.detailFrom == modeFeed {
			m.mode = modeFeed
		} else {
			m.mode = modeList
		}
		m.detail = nil
		m.detailKey = ""
		m.detailLite = nil
		return m, nil
	case key.Matches(msg, k.Comment):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startComment())
	case key.Matches(msg, k.Transition):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startTransition())
	case key.Matches(msg, k.Assignee):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startAssignee())
	case key.Matches(msg, k.Edit):
		m.busy = true
		return m, tea.Batch(m.spin.Tick, m.startFieldEdit())
	case key.Matches(msg, k.Refresh):
		if m.detailKey != "" && m.detailLite != nil {
			return m, m.loadDetailCmd(m.detailKey, *m.detailLite)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}
	switch m.form.State {
	case huh.StateCompleted:
		submit := m.formSubmit
		m.form = nil
		m.formSubmit = nil
		m.mode = modeList
		m.busy = true
		if submit != nil {
			return m, tea.Batch(m.spin.Tick, submit())
		}
		m.busy = false
		return m, nil
	case huh.StateAborted:
		m.form = nil
		m.formSubmit = nil
		m.mode = modeList
		m.busy = false
		m.toast = "cancelled"
		m.toastErr = false
		m.toastAt = m.clock()
		return m, nil
	}
	return m, cmd
}

func (m *Model) setTab(t Tab) {
	if m.tab == t && len(m.extraFilter.statusCategories) == 0 {
		return
	}
	m.tab = t
	// Manual tab change clears precise category list from a saved view.
	m.extraFilter.statusCategories = nil
	m.refilter()
}

func (m *Model) refilter() {
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	f := m.extraFilter
	f.tab = m.tab
	// Free-text from / merges with any text baked into the saved view.
	if needle != "" {
		f.text = needle
	}
	m.visible = applyListFilter(m.all, f)
	if m.listSort.key != "" {
		sortVisible(m.all, m.visible, m.listSort)
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.ensureVisible()
}

// hasSavedViewState reports whether esc should clear view-applied state.
func (m Model) hasSavedViewState() bool {
	return m.extraFilter.statusCategories != nil ||
		m.extraFilter.unassigned ||
		m.extraFilter.assigneeEmail != "" ||
		m.viewName != "" ||
		m.listSort.key != "" ||
		m.groupBy != ""
}

// listLines is the screen-line expansion of the current visible set, or nil
// when no grouping is active. nil means "screen line == visible index", which
// is the hot path: every keystroke re-renders, and expanding 10k rows into a
// parallel slice on each one buys nothing when there are no headers to insert.
func (m Model) listLines() []listLine {
	if m.groupBy == "" {
		return nil
	}
	return buildListLines(m.all, m.visible, m.groupBy)
}

// cursorScreenLine returns the screen-line index of the selected issue.
func (m Model) cursorScreenLine() int {
	if m.groupBy == "" {
		return m.cursor
	}
	for i, ln := range m.listLines() {
		if ln.kind == lineKindIssue && ln.visIdx == m.cursor {
			return i
		}
	}
	return 0
}

func (m *Model) moveCursor(delta int) {
	n := len(m.visible)
	if n == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	m.ensureVisible()
}

func (m *Model) pageSize() int {
	h := m.listHeight()
	if h < 1 {
		return 1
	}
	return h
}

func (m *Model) listHeight() int {
	// header(1) + tabs(1) + status(1) ≈ 3 chrome lines
	h := m.height - 3
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) ensureVisible() {
	h := m.listHeight()
	// offset is a screen-line index (headers count when grouping is on).
	line := m.cursorScreenLine()
	if line < m.offset {
		m.offset = line
	}
	if line >= m.offset+h {
		m.offset = line - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) clock() time.Time {
	if !m.now.IsZero() {
		return m.now
	}
	return time.Now()
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		m.width = 80
	}
	if m.height == 0 {
		m.height = 24
	}

	if m.pal != nil {
		return m.viewPalette()
	}
	var frame string
	switch {
	case m.mode == modeForm && m.form != nil:
		frame = m.viewForm()
	case m.mode == modeDetail:
		frame = m.viewDetail()
	case m.mode == modeDocDetail:
		frame = m.viewDocDetail()
	case m.mode == modeFeed:
		frame = m.viewFeed()
	case m.mode == modeViews:
		frame = m.viewViews()
	case m.mode == modeDocs:
		frame = m.viewDocs()
	case m.mode == modeFilter && m.filterFrom == modeDocs:
		// Filter overlays the docs list chrome (same as issue list).
		frame = m.viewDocs()
	default:
		frame = m.viewList()
	}
	if m.showHelp && m.mode != modeFeed {
		// feed view already overlays help itself
		return m.overlayHelp(frame)
	}
	return frame
}

func (m Model) viewList() string {
	var b strings.Builder
	w := m.width

	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')
	b.WriteString(fitWidth(m.renderTabs(), w))
	b.WriteByte('\n')

	listH := m.listHeight()
	if len(m.visible) == 0 {
		empty := "  no issues — press / to filter · r to reload"
		if m.filter != "" || m.tab != TabAll || m.extraFilter.unassigned ||
			m.extraFilter.assigneeEmail != "" || len(m.extraFilter.statusCategories) > 0 {
			empty = "  no matches — press / to filter · r to reload"
		}
		for i := 0; i < listH; i++ {
			if i == 0 {
				b.WriteString(fitWidth(styleMuted.Render(empty), w))
			}
			b.WriteByte('\n')
		}
	} else {
		// lines is nil unless a saved view asked for grouping; then screen lines
		// and visible indices diverge by the header rows.
		lines := m.listLines()
		total := len(m.visible)
		if lines != nil {
			total = len(lines)
		}
		end := m.offset + listH
		if end > total {
			end = total
		}
		for i := m.offset; i < end; i++ {
			if lines == nil {
				b.WriteString(m.renderRowAt(m.all[m.visible[i]], i == m.cursor, w, i-m.offset))
				b.WriteByte('\n')
				continue
			}
			ln := lines[i]
			if ln.kind == lineKindHeader {
				hdr := fmt.Sprintf(" ▸ %s (%d)", ln.label, ln.count)
				b.WriteString(fitWidth(styleMuted.Render(hdr), w))
			} else {
				r := m.all[m.visible[ln.visIdx]]
				b.WriteString(m.renderRowAt(r, ln.visIdx == m.cursor, w, i-m.offset))
			}
			b.WriteByte('\n')
		}
		for i := end - m.offset; i < listH; i++ {
			b.WriteByte('\n')
		}
	}

	b.WriteString(m.renderStatusBar(w))
	return b.String()
}

// renderHeader: app name + profile chip + watermark/sync age on one bar.
func (m Model) renderHeader(w int) string {
	left := styleBrand.Render(" scry ")
	if m.animOn {
		// Brand wordmark shimmers with the global phase — the one place the
		// gradient runs at full strength.
		left = " " + shimmer("scry", m.animPhase) + " "
	}
	if p := config.Profile(); p != "" {
		left += styleChip.Render(p)
	}
	right := styleMuted.Render(m.syncedLabel)
	if m.updateNotice != "" {
		right = styleBrand.Render("v"+m.updateNotice+" available") + " " + right
	}
	if m.loadErr != nil {
		right = styleToastErr.Render(m.loadErr.Error())
	}
	return styleHeaderBar.Render(joinBar(left, right, max(1, w-2)))
}

// renderTabLabel is the plain text of one tab ("1 All (34)") — shared with
// the mouse hit-testing in tabAt, which must reproduce the same widths.
func renderTabLabel(i int, t Tab, n int) string {
	return fmt.Sprintf("%d %s (%d)", i+1, t.Label(), n)
}

func (m Model) renderTabs() string {
	tabs := []struct {
		t Tab
		n int
	}{
		{TabAll, 0},
		{TabOpen, 0},
		{TabInProgress, 0},
		{TabDone, 0},
	}
	// Counts ignore the text filter so the tab bar stays stable while typing.
	for _, r := range m.all {
		for i := range tabs {
			if matchTab(tabs[i].t, r.lite.StatusCategory) {
				tabs[i].n++
			}
		}
	}
	parts := make([]string, 0, 4)
	for i, t := range tabs {
		label := renderTabLabel(i, t.t, t.n)
		if t.t == m.tab {
			active := styleTabActive
			if m.animOn {
				// The active tab breathes with the global pulse.
				active = active.Background(pulseColor(m.animPhase)).Foreground(colSelFg)
			}
			parts = append(parts, active.Render(label))
		} else {
			parts = append(parts, styleTabInactive.Render(label))
		}
	}
	s := " " + strings.Join(parts, " ")
	if m.viewName != "" {
		s += " " + styleChip.Render(m.viewName)
	}
	if chip := m.listSort.chip(); chip != "" && m.width >= 40 {
		s += " " + styleChip.Render(chip)
	}
	return s
}

func (m Model) renderRow(r row, selected bool, w int) string {
	return m.renderRowAt(r, selected, w, 0)
}

// renderRowAt renders one list row. screenRow feeds the ambient wave so each
// line sits at a slightly different phase; selection gets the breathing glow.
func (m Model) renderRowAt(r row, selected bool, w, screenRow int) string {
	// Animated backgrounds: selection glows, idle rows ride a very dark wave.
	// sep carries the row background across the gaps between segments —
	// unstyled spaces after an inner ANSI reset would show the terminal's own
	// background as dark slivers.
	styleSel, styleSelKey, styleSelMuted := styleSel, styleSelKey, styleSelMuted
	stylePrimary, styleDim, styleMuted, styleKey := stylePrimary, styleDim, styleMuted, styleKey
	rowFill := func(s string) string { return fitWidth(s, w) }
	sep := " "
	matchBg := styleHighlight
	if selected {
		matchBg = matchBg.Background(colAccentBg)
	}
	if m.animOn {
		if selected {
			g := glowColor(m.animPhase)
			styleSel = styleSel.Background(g)
			styleSelKey = styleSelKey.Background(g)
			styleSelMuted = styleSelMuted.Background(g)
			matchBg = matchBg.Background(g)
		} else if amb, ok := ambientBg(m.animPhase, screenRow); ok {
			stylePrimary = stylePrimary.Background(amb)
			styleDim = styleDim.Background(amb)
			styleMuted = styleMuted.Background(amb)
			styleKey = styleKey.Background(amb)
			matchBg = matchBg.Background(amb)
			ambStyle := lipgloss.NewStyle().Background(amb)
			sep = ambStyle.Render(" ")
			rowFill = func(s string) string {
				pad := w - lipgloss.Width(s)
				if pad < 0 {
					pad = 0
				}
				return s + ambStyle.Render(spaces(pad))
			}
		}
	}
	if selected {
		sep = styleSel.Render(" ")
	}
	hl := func(plain string, base lipgloss.Style) string {
		if m.filter == "" {
			return base.Render(plain)
		}
		return highlightMatch(plain, m.filter, base, matchBg)
	}

	// Narrow terminal: key + summary only.
	if w < 40 {
		const keyW = 12
		sumW := w - keyW - 3
		if sumW < 4 {
			sumW = 4
		}
		keyPlain := padRight(r.lite.IssueKey, min(keyW, max(4, w/3)))
		summaryPlain := truncate(r.lite.Summary, sumW)
		watch := ""
		if m.watches[r.lite.IssueKey] {
			watch = "★"
		}
		if selected {
			inner := sep + hl(keyPlain, styleSelKey) + sep + hl(summaryPlain, styleSel) + watch
			return styleSel.Width(w).MaxWidth(w).Render(inner)
		}
		line := sep + hl(keyPlain, styleKey) + sep + hl(summaryPlain, stylePrimary)
		if watch != "" {
			line += styleBrand.Render(watch)
		}
		return rowFill(line)
	}

	// Layout: " ● KEY········ status····· summary… [label] assignee age"
	// Fixed budget leaves the rest for the summary.
	const (
		keyW      = 12
		statusW   = 12
		assigneeW = 12
		ageW      = 9
		// " " + "●" + " " + key + " " + status + " " + ... + " " + assignee + " " + age
		// prefix dots/spaces ≈ 5; chip is optional and short.
		overhead = 5 + keyW + 1 + statusW + 1 + 1 + assigneeW + 1 + ageW
	)
	chip := ""
	if len(r.lite.Labels) > 0 {
		chip = truncate(r.lite.Labels[0], 10)
	}
	if m.watches[r.lite.IssueKey] {
		if chip != "" {
			chip = "★" + chip
		} else {
			chip = "★"
		}
	}
	// The summary absorbs exactly what the chip leaves: a wide chip once made
	// the selected row overflow w and wrap, scrolling the header off-screen.
	chipW := 0
	if chip != "" {
		chipW = runewidth.StringWidth(chip) + 1
	}
	sumW := w - overhead - chipW
	if sumW < 8 {
		sumW = 8
	}

	dotStyle := statusStyle(r.lite.StatusCategory, r.lite.ReopenCount)
	keyPlain := padRight(r.lite.IssueKey, keyW)
	statusPlain := padRight(truncate(r.lite.Status, statusW), statusW)
	summaryPlain := padRight(truncate(r.lite.Summary, sumW), sumW)
	assigneePlain := padRight(truncate(deref(r.lite.Assignee), assigneeW), assigneeW)
	agePlain := padRight(relativeTime(deref(r.lite.UpdatedAt), m.clock()), ageW)

	if selected {
		// Solid glowing row; keep the status dot coloured on top of the bg.
		dotBg := colAccentBg
		dot := dotStyle.Background(dotBg)
		if m.animOn {
			dot = dotStyle.Background(glowColor(m.animPhase))
		}
		inner := sep +
			dot.Render("●") + sep +
			hl(keyPlain, styleSelKey) + sep +
			hl(statusPlain, styleSel) + sep +
			hl(summaryPlain, styleSel)
		if chip != "" {
			inner += sep + styleSelMuted.Render(chip)
		}
		inner += sep + hl(assigneePlain, styleSelMuted) + sep + styleSelMuted.Render(agePlain)
		return styleSel.MaxWidth(w).Render(inner)
	}

	dot := dotStyle
	if m.animOn {
		if amb, ok := ambientBg(m.animPhase, screenRow); ok {
			dot = dot.Background(amb)
		}
	}
	line := sep +
		dot.Render("●") + sep +
		hl(keyPlain, styleKey) + sep +
		hl(statusPlain, styleDim) + sep +
		hl(summaryPlain, stylePrimary)
	if chip != "" {
		line += sep + styleChip.Render(chip)
	}
	line += sep + hl(assigneePlain, styleDim) + sep + styleMuted.Render(agePlain)
	return rowFill(line)
}

// renderStatusBar: left = key hints (+ spinner/toast/filter), right = sync + count.
func (m Model) renderStatusBar(w int) string {
	leftParts := make([]string, 0, 4)
	if m.loading || m.busy {
		leftParts = append(leftParts, m.spin.View())
	}
	if w < 40 {
		leftParts = append(leftParts, styleHelp.Render("j/k / enter ? q"))
	} else {
		leftParts = append(leftParts, styleHelp.Render("j/k · / · 1-4 · enter · c/t/a · w · F · v · ? · r · q"))
	}
	if m.filter != "" || m.mode == modeFilter {
		f := m.filter
		if m.mode == modeFilter {
			f += "▌"
		}
		leftParts = append(leftParts, styleFilter.Render("/"+f))
	}
	if m.feedCounts.All > 0 {
		leftParts = append(leftParts, styleChip.Render(fmt.Sprintf("feed %d", m.feedCounts.All)))
	}
	if m.toast != "" && m.clock().Sub(m.toastAt) < 8*time.Second {
		if m.toastErr {
			leftParts = append(leftParts, styleToastErr.Render(m.toast))
		} else {
			leftParts = append(leftParts, styleToastOK.Render(m.toast))
		}
	}
	if m.cfg != nil && !m.cfg.HasCredential() {
		leftParts = append(leftParts, styleMuted.Render("read-only"))
	}
	left := strings.Join(leftParts, "  ")

	count := fmt.Sprintf("%d/%d", 0, len(m.visible))
	if len(m.visible) > 0 {
		count = fmt.Sprintf("%d/%d", m.cursor+1, len(m.visible))
	}
	right := styleMuted.Render(m.syncedLabel + "  ·  " + count)
	return joinBar(left, right, w)
}

func (m Model) viewForm() string {
	var b strings.Builder
	title := styleBrand.Render(" " + m.formTitle + " ")
	if m.busy {
		title = m.spin.View() + title
	}
	b.WriteString(fitWidth(title, m.width))
	b.WriteString("\n\n")
	b.WriteString(m.form.View())
	return b.String()
}

func (m Model) viewDetail() string {
	var b strings.Builder
	w := m.width
	key := m.detailKey
	lite := m.detailLite

	// Outer chrome: header bar + footer hints. Body sits in a rounded card.
	b.WriteString(m.renderHeader(w))
	b.WriteByte('\n')

	innerW := w - 4
	if innerW < 20 {
		innerW = 20
	}

	var body strings.Builder
	// Title row: key + status dot + summary
	title := styleKey.Render(key)
	if lite != nil {
		dot := statusStyle(lite.StatusCategory, lite.ReopenCount).Render("●")
		title += "  " + dot + " " + styleDim.Render(lite.Status)
		if m.watches[key] {
			title += " " + styleBrand.Render("★")
		}
		title += "\n" + stylePrimary.Bold(true).Render(lite.Summary)
	}
	body.WriteString(title)
	body.WriteByte('\n')

	if lite != nil {
		body.WriteString(m.renderDetailField("assignee", orDash(deref(lite.Assignee))))
		body.WriteByte('\n')
		labelVal := "—"
		if len(lite.Labels) > 0 {
			chips := make([]string, 0, len(lite.Labels))
			for _, l := range lite.Labels {
				chips = append(chips, styleChip.Render(l))
			}
			labelVal = strings.Join(chips, " ")
		}
		body.WriteString(styleDetailLabel.Render("labels") + " " + labelVal)
		body.WriteByte('\n')
		body.WriteString(m.renderDetailField("updated", orDash(relativeTime(deref(lite.UpdatedAt), m.clock()))))
		body.WriteByte('\n')
	}
	body.WriteByte('\n')

	if m.detail == nil {
		body.WriteString(styleMuted.Render("loading…"))
	} else {
		// Discovered custom fields — only the ones this issue fills (boards
		// differ; an empty row is noise). Same source of truth as the web:
		// config field specs over issues.custom.
		for _, spec := range m.cfg.FieldSpecs() {
			if spec.Role == "body" {
				continue // rendered as a section below, like Description
			}
			val := customDisplay(m.detail.Custom[spec.Alias])
			if val == "" {
				continue
			}
			body.WriteString(styleCustomLabel.Render(truncateCells(spec.Label, 17)) + " " + stylePrimary.Render(val))
			body.WriteByte('\n')
		}

		// Description
		body.WriteString(styleSection.Render("Description"))
		body.WriteByte('\n')
		desc := jira.PlainText(m.detail.DescriptionADF)
		if desc == "" {
			body.WriteString(styleMuted.Render("  (empty)"))
			body.WriteByte('\n')
		} else {
			for _, line := range wrapLines(desc, innerW-2, 12) {
				body.WriteString("  ")
				body.WriteString(line)
				body.WriteByte('\n')
			}
		}
		body.WriteByte('\n')

		// Body-role custom fields (repro steps, QA notes, …) as prose sections.
		for _, spec := range m.cfg.FieldSpecs() {
			if spec.Role != "body" {
				continue
			}
			text := bodyText(m.detail.Custom[spec.Alias])
			if text == "" {
				continue
			}
			body.WriteString(styleSection.Render(spec.Label))
			body.WriteByte('\n')
			for _, line := range wrapLines(text, innerW-2, 12) {
				body.WriteString("  ")
				body.WriteString(line)
				body.WriteByte('\n')
			}
			body.WriteByte('\n')
		}

		// Comments (last 5)
		body.WriteString(styleSection.Render("Comments"))
		body.WriteByte('\n')
		comments := m.detail.Comments
		if len(comments) == 0 {
			body.WriteString(styleMuted.Render("  (none)"))
			body.WriteByte('\n')
		} else {
			start := 0
			if len(comments) > 5 {
				start = len(comments) - 5
			}
			for _, c := range comments[start:] {
				who := c.Author
				if who == "" {
					who = "?"
				}
				when := relativeTime(c.CreatedAt, m.clock())
				body.WriteString(styleDim.Render(fmt.Sprintf("  %s · %s", who, when)))
				body.WriteByte('\n')
				text := c.Body
				if text == "" {
					text = jira.PlainText(c.BodyADF)
				}
				for _, line := range wrapLines(text, innerW-4, 4) {
					body.WriteString("    ")
					body.WriteString(line)
					body.WriteByte('\n')
				}
			}
		}
		body.WriteByte('\n')

		// History (last 5)
		body.WriteString(styleSection.Render("History"))
		body.WriteByte('\n')
		hist := m.detail.History
		if len(hist) == 0 {
			body.WriteString(styleMuted.Render("  (none)"))
			body.WriteByte('\n')
		} else {
			start := 0
			if len(hist) > 5 {
				start = len(hist) - 5
			}
			for _, h := range hist[start:] {
				line := fmt.Sprintf("  %s  %s: %s → %s  (%s)",
					relativeTime(h.At, m.clock()),
					h.Field,
					orDash(h.FromValue),
					orDash(h.ToValue),
					orDash(h.Author),
				)
				body.WriteString(styleDim.Render(line))
				body.WriteByte('\n')
			}
		}
	}

	panel := styleDetailPanel.Width(w - 2).Render(strings.TrimRight(body.String(), "\n"))
	b.WriteString(panel)
	b.WriteByte('\n')

	// Footer: hints left, toast right/appended
	footer := styleHelp.Render("esc back  c comment  t transition  a assignee  w watch  r refresh  ?")
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

func (m Model) renderDetailField(label, value string) string {
	return styleDetailLabel.Render(label) + " " + stylePrimary.Render(value)
}

// truncateCells caps s at max display cells (CJK-aware), appending … when cut.
func truncateCells(s string, max int) string {
	if runewidth.StringWidth(s) <= max {
		return s
	}
	return runewidth.Truncate(s, max, "…")
}

// customDisplay renders a flattened custom value (string or []string; sync
// stores non-body values display-ready) as one chip-ish line.
func customDisplay(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, el := range t {
			if s, ok := el.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return ""
	}
}

// bodyText extracts prose from a body-role custom value: ADF documents render
// through the same plain-text path as descriptions; plain strings pass through.
func bodyText(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		raw, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(jira.PlainText(raw))
	default:
		return ""
	}
}

func wrapLines(s string, width, maxLines int) []string {
	if width < 8 {
		width = 8
	}
	raw := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var out []string
	for _, line := range raw {
		line = strings.TrimRight(line, " ")
		if line == "" {
			out = append(out, "")
			if maxLines > 0 && len(out) >= maxLines {
				break
			}
			continue
		}
		r := []rune(line)
		for len(r) > 0 {
			n := width
			if n > len(r) {
				n = len(r)
			}
			out = append(out, string(r[:n]))
			r = r[n:]
			if maxLines > 0 && len(out) >= maxLines {
				return out
			}
		}
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
