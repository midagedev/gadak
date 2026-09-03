package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/config/tokencheck"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	gadaksync "github.com/midagedev/gadak/internal/sync"
)

// Version is the gadak release string exposed on GET settings/ under runtime.
// cmd/gadak should assign this from its ldflags version var at startup:
//
//	server.Version = version
//
// Until that wiring lands, the default below is what the UI shows.
var Version = "0.0.0-dev"

// The client falls back to 72 hours when the document omits the threshold, but a
// literal 0 would override that and mark every issue stale, so the document
// always carries a real number.
const defaultStaleHours = 72

// featureNames aliases config.FeatureNames so existing tests in this package
// keep compiling. The names themselves live in the config package.
var featureNames = config.FeatureNames

type webConfigDoc struct {
	APIBase             string                    `json:"apiBase"`
	AuthBase            string                    `json:"authBase"`
	JiraBaseURL         string                    `json:"jiraBaseUrl"`
	QaDashboardURL      string                    `json:"qaDashboardUrl"`
	Projects            []string                  `json:"projects"`
	GroupLabels         map[string]string         `json:"groupLabels"`
	GroupColors         map[string]string         `json:"groupColors"`
	ProductByGroup      map[string]config.Product `json:"productByGroup"`
	StaleThresholdHours int                       `json:"staleThresholdHours"`
	Features            map[string]bool           `json:"features"`
	// ConfluenceEnabled is true when the wiki source is configured (cfg.Confluence
	// non-nil). Boolean only — no site, token, or space list.
	ConfluenceEnabled bool `json:"confluenceEnabled"`
	// Profile is the gadak profile name this document belongs to. Empty and
	// "default" both serialize as "default" (same as `gadak doctor --json`
	// and `gadak profiles`), so two primaries on one origin never share a
	// client cache key.
	Profile string `json:"profile"`
	// OS is the GOOS of the process serving this document — the machine an
	// upgrade would happen on. The UI needs it because upgrade instructions
	// are per-platform: `brew upgrade --cask gadak` is a macOS command, and
	// printing it to a Linux reader is simply wrong. Absent (static export,
	// hosted demo) means "unknown", and the UI then names no command.
	OS string `json:"os,omitempty"`
	// WorkspaceKind is origin.Describe's kind: "connected" or "standalone".
	// Always sent. The UI must not infer this from an empty site URL — a
	// hosted demo and an older document also have no site.
	WorkspaceKind string `json:"workspaceKind"`
	// OriginType and Transport split what WorkspaceKind answered at once
	// (GDK-1278): which tracker the origin is (jira|linear|gadak), and
	// whether it is reached in-process or across a serve API
	// (local|remote). A paired workspace is the case that needed them —
	// WorkspaceKind calls it "connected" while its origin is gadak's own
	// tracker one machine away. Always sent; the UI must not infer either
	// from the site URL or from the presence of a pairing block.
	OriginType string `json:"originType"`
	Transport  string `json:"transport"`
	// OriginWritable mirrors config.HasAtlassianCredential — "can this server
	// reach the Jira-family origin at all" (site+email+token, localOrigin, or
	// a pairing remote). Boolean only; no site, token, or email.
	//
	// It exists so the web does not re-derive that three-way predicate
	// (GDK-1090). Every write path's 409 credential_required comes from this
	// one bool on the server, and a surface that skips a request rather than
	// collecting a 409 has to agree with it exactly. The near-miss it closes:
	// `me.identified` looks like the same question and is not — auth/me
	// answers from cfg.Email, which is empty on both local-origin and paired,
	// so gating on identity would have silenced the very workspaces 0.19
	// targets. Absent (static export, hosted demo) reads as false, which is
	// the truth there.
	OriginWritable bool `json:"originWritable"`
	// UI is the server-merged color/dimension/font override block
	// (GDK-786/791, GDK-842, GDK-896 R4): the final per-palette CSS variable
	// map, data inks, and the palette-agnostic dimension and font overrides,
	// all already validated and filtered, so the web needs no catalog
	// knowledge of its own. Warnings are load-time advisories (a downgrade
	// met a renamed token), not errors.
	UI *uiDoc `json:"ui"`
	// ConfigVersion is the disk identity (mtime.size) of this profile's
	// config.json. The ui-focus poll carries it too; the web refetches
	// config.json when it moves, which is how a CLI `config set` reaches an
	// already-open tab with no reload. Empty only when the serving process
	// has no directory to stat.
	ConfigVersion string `json:"configVersion,omitempty"`
}

// uiDoc is the ui slice of config.json. Vars is palette → cssVar → hex with
// tokensByTheme already merged in (theme wins); only overrides appear — base
// values keep coming from app.css. Dims is cssVar → length/line-height
// (GDK-842): palette-agnostic, overrides only, unknown/locked/invalid tokens
// already filtered by UIDimensionVars. Fonts is cssVar → font stack
// (GDK-896 R4): palette-agnostic like dims, but a separate field on purpose —
// the web's dim filters assume numeric values, and mixing kinds would make
// both mirrors' name checks lie. Unknown names and grammar failures are
// already dropped by UIFontVars; a web build that predates the field simply
// ignores it.
type uiDoc struct {
	Vars       map[string]map[string]string `json:"vars"`
	Dims       map[string]string            `json:"dims"`
	Fonts      map[string]string            `json:"fonts"`
	DataColors map[string]map[string]string `json:"dataColors"`
	Warnings   []tokencheck.Violation       `json:"warnings,omitempty"`
}

// webConfig is the credential-free projection of the configuration. Site is the
// only field of the credential block that appears here, as a deep-link base.
func webConfig(cfg *config.Config) webConfigDoc {
	if cfg == nil {
		cfg = &config.Config{}
	}
	stale := cfg.StaleThresholdHours
	if stale <= 0 {
		stale = defaultStaleHours
	}
	kind, _ := origin.Describe(cfg)
	// An in-memory Config (tests, embedded docs) has no directory; fall back
	// to the active profile the way origin's profileDir does — Save() writes
	// there too, so the version still tracks the file.
	dir := cfg.Directory()
	if dir == "" {
		dir, _ = config.DirFor(config.Profile())
	}
	vars, colorWarns := config.UITokenVars(cfg.UI)
	dims, dimWarns := config.UIDimensionVars(cfg.UI)
	warns := append(colorWarns, dimWarns...)
	return webConfigDoc{
		APIBase:             apiBase,
		AuthBase:            authBase,
		JiraBaseURL:         strings.TrimRight(cfg.Site, "/"),
		QaDashboardURL:      cfg.QaDashboardURL,
		Projects:            strs(cfg.Projects),
		GroupLabels:         strMap(cfg.GroupLabels),
		GroupColors:         strMap(cfg.GroupColors),
		ProductByGroup:      products(cfg.ProductByGroup),
		StaleThresholdHours: stale,
		Features:            features(cfg.Features),
		ConfluenceEnabled:   cfg.Confluence != nil,
		Profile:             profileDisplay(config.Profile()),
		OS:                  runtime.GOOS,
		WorkspaceKind:       kind,
		OriginType:          cfg.OriginType(),
		Transport:           cfg.Transport(),
		OriginWritable:      cfg.HasAtlassianCredential(),
		UI: &uiDoc{
			Vars:       vars,
			Dims:       dims,
			Fonts:      config.UIFontVars(cfg.UI),
			DataColors: config.UIDataColors(cfg.UI),
			Warnings:   warns,
		},
		ConfigVersion: config.ConfigVersionOfDir(dir),
	}
}

// runtimeInfo is read-only instance facts for the settings UI. Never carry
// secrets (token, email) — only paths and mirror bookkeeping.
type runtimeInfo struct {
	Profile       string  `json:"profile"`
	DBPath        string  `json:"dbPath"`
	DBSizeBytes   int64   `json:"dbSizeBytes"`
	DBSizeHuman   string  `json:"dbSizeHuman"`
	DBModifiedAt  *string `json:"dbModifiedAt,omitempty"`
	ConfigPath    string  `json:"configPath"`
	IssueCount    int     `json:"issueCount"`
	CommentCount  int     `json:"commentCount"`
	SchemaVersion int     `json:"schemaVersion"`
	Watermark     string  `json:"watermark,omitempty"`
	// SyncVersion is sync_state.version — the mirror generation clients poll.
	SyncVersion    int64   `json:"syncVersion"`
	LastFullSyncAt *string `json:"lastFullSyncAt,omitempty"`
	LastError      *string `json:"lastError,omitempty"`
	GadakVersion   string  `json:"gadakVersion"`
	// Defaults the UI shows as placeholders when the stored interval is 0.
	DefaultSyncIntervalSec      int `json:"defaultSyncIntervalSec"`
	DefaultReconcileIntervalSec int `json:"defaultReconcileIntervalSec"`
	// OsNotifySupported is whether this process can fire a real OS desktop
	// notification (macOS osascript, Linux notify-send). Always sent — false
	// is meaningful (Windows no-op) so it must not use omitempty. Owner is
	// sync.OSNotifier.Supported; the settings UI must not re-derive it from GOOS.
	OsNotifySupported bool `json:"osNotifySupported"`
	// ApiUsage is our process's outbound Jira call volume (today + 7-day
	// rollup), not Jira's remaining rate-limit budget. Omitted only if the
	// mirror cannot be read; zeros are still a valid document.
	ApiUsage *store.APIUsageSummary `json:"apiUsage,omitempty"`
}

// settingsConfluenceDoc is the Confluence slice of settings. A pointer on
// settingsDoc so absence on PUT is distinguishable from an empty spaces list
// (empty = "all global spaces" and is a valid save when Confluence is already on).
// Enabled turns the source on/off; Spaces alone still updates scope only when
// the source is already configured (legacy path).
type settingsConfluenceDoc struct {
	Enabled *bool    `json:"enabled,omitempty"`
	Spaces  []string `json:"spaces,omitempty"`
}

// settingsTerminalDoc is the terminal block as the settings document carries
// it: the two display fields, stored values (0 / false = default) — unlike
// appearance, which GET fills with the effective value. Chosen, not
// accidental (GDK-1365): the form round-trips what is stored, so an empty
// scrollback field stays "default" rather than freezing 5000 into the
// config; the default number the placeholder shows is the one duplicate
// that buys (config.DefaultTerminalScrollback). See the
// Terminal field comment for why shell and workingDir are absent. Present
// means the whole display pair is replaced — omit-to-preserve is block-
// granular, like `ui`; a body with only scrollback resets cursorBlink.
type settingsTerminalDoc struct {
	Scrollback  int  `json:"scrollback"`
	CursorBlink bool `json:"cursorBlink"`
}

func terminalDisplay(cfg *config.Config) *settingsTerminalDoc {
	out := &settingsTerminalDoc{}
	if cfg != nil && cfg.Terminal != nil {
		out.Scrollback = cfg.Terminal.Scrollback
		out.CursorBlink = cfg.Terminal.CursorBlink
	}
	return out
}

// settingsDoc is everything the settings UI may read and write. The credential
// block (email, token) is absent by construction, not by filtering.
type settingsDoc struct {
	Projects       []string           `json:"projects"`
	FieldMap       map[string]string  `json:"fieldMap"`
	BodyFields     []string           `json:"bodyFields"`
	EditableFields map[string]string  `json:"editableFields"`
	Members        []config.Member    `json:"members"`
	GroupRules     []config.GroupRule `json:"groupRules"`
	// Pointer so an older PUT that omits the key cannot wipe a stored query.
	GroupQuery           *string                   `json:"groupQuery,omitempty"`
	GroupLabels          map[string]string         `json:"groupLabels"`
	GroupColors          map[string]string         `json:"groupColors"`
	ProductByGroup       map[string]config.Product `json:"productByGroup"`
	Features             map[string]bool           `json:"features"`
	QaDashboardURL       string                    `json:"qaDashboardUrl"`
	StaleThresholdHours  int                       `json:"staleThresholdHours"`
	SyncIntervalSec      int                       `json:"syncIntervalSec"`
	ReconcileIntervalSec int                       `json:"reconcileIntervalSec"`

	// Fields edits the discovered specs. A pointer so absence is distinguishable:
	// an older UI that PUTs without this key must not wipe discovery output.
	// GET never populates it (fieldSpecs below is the read surface), which also
	// keeps old clients from echoing it back.
	Fields *[]config.FieldSpec `json:"fields,omitempty"`

	// Confluence is nil when the source is off. PUT with enabled:true turns it
	// on; enabled:false turns it off; spaces alone still requires it already on.
	Confluence *settingsConfluenceDoc `json:"confluence,omitempty"`

	// Appearance is the UI look. A pointer so older clients that omit the key
	// on PUT cannot wipe a stored theme. GET always populates it (empty
	// stored theme → "system", empty stored terminal → "dark").
	Appearance *config.Appearance `json:"appearance,omitempty"`

	// Terminal is the pane's display behavior — scrollback and cursor blink
	// only. Shell and workingDir are deliberately NOT here (GDK-1069): this
	// endpoint admits serve-scope pairing Bearers, and a field that names the
	// next binary the local user's terminal spawns would let a paired device
	// plant it. Those two stay `gadak config set terminal.shell` — a write
	// from the machine that runs the shell. Pointer for the omit-to-preserve
	// rule; GET always populates it with the stored values (0 = default).
	Terminal *settingsTerminalDoc `json:"terminal,omitempty"`

	// UI is the user color-override block (GDK-786): tokens, per-palette
	// overlays, data inks. Pointer for the same reason as Appearance — an
	// older PUT that omits the key must not wipe stored overrides. GET
	// always populates it (empty object = nothing overridden).
	UI *config.UIConfig `json:"ui,omitempty"`

	// UIWarnings carries the write-time token warnings of THIS PUT back to
	// the caller: judgment violations now save, so the warning is
	// the payload's own diagnostics — why the saved look will render the way
	// it does. Response-only: omitempty keeps GET and clean PUTs unchanged,
	// and a client-supplied value is ignored (decode target, never read).
	UIWarnings []tokencheck.Violation `json:"uiWarnings,omitempty"`

	// Read-only context for the UI. Ignored on PUT — the site and the token are
	// the credential endpoint's business (T4). runtime is assembled per request.
	// fieldSpecs / fieldUsage are discovery output; PUT must not clobber Fields.
	Site          string                    `json:"site"`
	HasCredential bool                      `json:"hasCredential"`
	Runtime       *runtimeInfo              `json:"runtime,omitempty"`
	FieldSpecsOut []config.FieldSpec        `json:"fieldSpecs"` // read-only; PUT ignores
	FieldUsage    map[string]map[string]int `json:"fieldUsage"` // project → alias → filled; read-only
}

func (s *server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsResponse(r.Context(), s.config()))
}

// spaceRow is one Confluence space in the settings picker. selected mirrors
// config.Confluence.Spaces membership; when that list is empty the API leaves
// every selected=false and sets all_global_when_empty so the UI can render
// "all global spaces" without lying about checkboxes.
type spaceRow struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Selected bool   `json:"selected"`
}

// handleSettingsSpaces lists live Confluence spaces for the settings picker.
// Discovery only needs a credential — Confluence may be off (enabled:false);
// selected flags and all_global_when_empty then stay false so "off" is never
// read as "all global spaces".
func (s *server) handleSettingsSpaces(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	if !cfg.HasCredential() {
		fail(w, http.StatusConflict, "credential_required")
		return
	}
	c, err := origin.Wiki(cfg)
	if err != nil {
		failOriginClient(w, err)
		return
	}
	listed, err := c.Spaces(r.Context())
	if err != nil {
		if errors.Is(err, confluence.ErrAuth) {
			fail(w, http.StatusConflict, "credential_rejected")
			return
		}
		log.Printf("server: %s %s: %v", r.Method, r.URL.Path, err)
		fail(w, http.StatusBadGateway, "confluence_unavailable")
		return
	}

	enabled := cfg.Confluence != nil
	selected := map[string]bool{}
	empty := false
	if enabled {
		for _, k := range cfg.Confluence.Spaces {
			selected[k] = true
		}
		// Empty Spaces means "all global" only while the source is on.
		empty = len(cfg.Confluence.Spaces) == 0
	}

	out := make([]spaceRow, 0, len(listed))
	for _, sp := range listed {
		out = append(out, spaceRow{
			Key:  sp.Key,
			Name: sp.Name,
			Type: sp.Type,
			// Empty config means "all global" at sync time — do not pre-check
			// every row; the flag below tells the UI how to present that.
			// When off, every selected is false (off ≠ all-global).
			Selected: enabled && !empty && selected[sp.Key],
		})
	}
	// team spaces first (anything but personal — GDK-1302), then name
	// (case-insensitive) within each group.
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := out[i].Type != "personal", out[j].Type != "personal"
		if gi != gj {
			return gi
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"spaces":                out,
		"all_global_when_empty": empty,
		"enabled":               enabled,
	})
}

func (s *server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in settingsDoc
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body")
		return
	}
	if err := config.ValidateIntervals(in.SyncIntervalSec, in.ReconcileIntervalSec); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// Snapshot before we build next so scope comparison sees the pre-write config.
	prev := s.config()
	// Copy the live config so the credential block survives a settings write.
	// site / email / token / tokenVerifiedAt / tokenOwner / tokenExpiresAt /
	// tokenExpirySource are never taken from the PUT body — those stay the
	// credential endpoint's job.
	next := *prev
	next.Projects = in.Projects
	next.FieldMap = in.FieldMap
	// Field specs change only when the client sent the key. Hand edits are the
	// user overriding discovery, so they arrive pinned (auto:false) from the UI;
	// discovery regenerates only auto:true specs.
	if in.Fields != nil {
		next.Fields = *in.Fields
	}
	// Confluence: key absent → leave alone.
	// enabled:false → turn off (nil). Does not delete mirrored pages — they
	// stay readable on disk; wiping is a separate, explicit user action.
	// enabled:true → create/replace the block with Spaces (empty = all global).
	// spaces alone (enabled omitted) → replace Spaces only when already on;
	// still 400 confluence_not_configured when off (legacy path).
	if in.Confluence != nil {
		if err := config.ApplyConfluence(&next, in.Confluence.Enabled, in.Confluence.Spaces); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if in.Appearance != nil {
		if err := config.ApplyAppearance(&next, *in.Appearance); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// Merge onto the stored block: shell and workingDir are not in the
	// document, so they must survive a PUT untouched (GDK-1069).
	if in.Terminal != nil {
		if err := config.ApplyTerminalDisplay(&next, in.Terminal.Scrollback, in.Terminal.CursorBlink); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	// Same omit-to-preserve contract: ui absent leaves stored overrides
	// alone; present means full replacement of the block (the UI edits the
	// whole object it GETs). Refusals carry the validator's reason; judgment
	// warnings ride the response as uiWarnings.
	var uiWarns []tokencheck.Violation
	if in.UI != nil {
		var err error
		uiWarns, err = config.ApplyUIConfigWithWarnings(&next, in.UI)
		if err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	next.BodyFields = in.BodyFields
	next.EditableFields = in.EditableFields
	next.Members = in.Members
	next.GroupRules = in.GroupRules
	// Omitted groupQuery leaves the stored query alone so an older settings
	// client cannot wipe it. Send "" to clear.
	if in.GroupQuery != nil {
		q := strings.TrimSpace(*in.GroupQuery)
		if err := config.ValidateGroupQuery(q); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		next.GroupQuery = q
	}
	next.GroupLabels = in.GroupLabels
	next.GroupColors = in.GroupColors
	next.ProductByGroup = in.ProductByGroup
	next.Features = features(in.Features)
	next.QaDashboardURL = in.QaDashboardURL
	next.StaleThresholdHours = max(in.StaleThresholdHours, 0)
	next.SyncIntervalSec = max(in.SyncIntervalSec, 0)
	next.ReconcileIntervalSec = max(in.ReconcileIntervalSec, 0)

	if err := next.Save(); err != nil {
		serverError(w, r, err)
		return
	}
	s.cfg.Store(&next)
	// Members and group rules feed the cached projection; it has to be rebuilt.
	s.gen.Add(1)
	// Scope change with a credential: kick a full sync. full=true because a
	// newly added project's past issues never arrive on a watermark-based
	// incremental — changing scope is exactly the backfill case. If a job is
	// already running we skip (not an error); the response stays 200 + settings.
	if scopeChanged(prev, &next) && next.HasCredential() && !next.SyncFrozen() {
		_ = s.startSyncJob(&next, true)
	}
	doc := s.settingsResponse(r.Context(), &next)
	doc.UIWarnings = uiWarns // response-only; GET builds it without
	writeJSON(w, http.StatusOK, doc)
}

// scopeChanged reports whether the mirrored source set differs between two
// configs. Order of project/space keys does not count; only set membership and
// Confluence on/off do.
func scopeChanged(prev, next *config.Config) bool {
	if prev == nil {
		prev = &config.Config{}
	}
	if next == nil {
		next = &config.Config{}
	}
	if !sameStringSet(prev.Projects, next.Projects) {
		return true
	}
	prevOn, nextOn := prev.Confluence != nil, next.Confluence != nil
	if prevOn != nextOn {
		return true
	}
	if !prevOn {
		return false
	}
	return !sameStringSet(prev.Confluence.Spaces, next.Confluence.Spaces)
}

func sameStringSet(a, b []string) bool {
	return maps.Equal(stringSet(a), stringSet(b))
}

func stringSet(v []string) map[string]struct{} {
	out := make(map[string]struct{}, len(v))
	for _, s := range v {
		out[s] = struct{}{}
	}
	return out
}

func settings(cfg *config.Config) settingsDoc {
	doc := settingsDoc{
		Projects:             strs(cfg.Projects),
		FieldMap:             strMap(cfg.FieldMap),
		BodyFields:           strs(cfg.BodyFields),
		EditableFields:       strMap(cfg.EditableFields),
		Members:              members(cfg.Members),
		GroupRules:           rules(cfg.GroupRules),
		GroupQuery:           strPtr(cfg.GroupQuery),
		GroupLabels:          strMap(cfg.GroupLabels),
		GroupColors:          strMap(cfg.GroupColors),
		ProductByGroup:       products(cfg.ProductByGroup),
		Features:             features(cfg.Features),
		QaDashboardURL:       cfg.QaDashboardURL,
		StaleThresholdHours:  cfg.StaleThresholdHours,
		SyncIntervalSec:      cfg.SyncIntervalSec,
		ReconcileIntervalSec: cfg.ReconcileIntervalSec,
		Site:                 cfg.Site,
		HasCredential:        cfg.HasCredential(),
		Appearance: &config.Appearance{
			Theme:    cfg.EffectiveTheme(),
			Terminal: cfg.EffectiveTerminalAppearance(),
		},
		Terminal: terminalDisplay(cfg),
		UI:       uiOrEmpty(cfg.UI),
	}
	if cfg.Confluence != nil {
		doc.Confluence = &settingsConfluenceDoc{Spaces: strs(cfg.Confluence.Spaces)}
	}
	return doc
}

func (s *server) settingsResponse(ctx context.Context, cfg *config.Config) settingsDoc {
	doc := settings(cfg)
	doc.Runtime = s.runtimeInfo(ctx)
	// Read-only discovery surfaces — never written by handlePutSettings.
	specs := cfg.FieldSpecs()
	if specs == nil {
		doc.FieldSpecsOut = []config.FieldSpec{}
	} else {
		doc.FieldSpecsOut = specs
	}
	doc.FieldUsage = map[string]map[string]int{}
	if s.db != nil {
		if rows, err := s.db.FieldUsage(ctx); err == nil {
			for _, r := range rows {
				m := doc.FieldUsage[r.ProjectKey]
				if m == nil {
					m = map[string]int{}
					doc.FieldUsage[r.ProjectKey] = m
				}
				m[r.Alias] = r.Filled
			}
		}
	}
	return doc
}

// runtimeInfo is assembled per request; ctx is the request's context so the
// mirror reads stop when the caller is gone (GDK-610).
func (s *server) runtimeInfo(ctx context.Context) *runtimeInfo {
	info := &runtimeInfo{
		Profile:                     profileDisplay(s.profile),
		GadakVersion:                Version,
		DefaultSyncIntervalSec:      config.DefaultSyncIntervalSec,
		DefaultReconcileIntervalSec: config.DefaultReconcileIntervalSec,
		OsNotifySupported:           gadaksync.OSNotifier{}.Supported(),
	}
	if d, err := config.DirFor(s.profile); err == nil {
		info.ConfigPath = filepath.Join(d, "config.json")
		dbPath := filepath.Join(d, "gadak.db")
		info.DBPath = dbPath
		if st, err := os.Stat(dbPath); err == nil {
			info.DBSizeBytes = st.Size()
			info.DBSizeHuman = humanBytes(st.Size())
			mod := st.ModTime().UTC().Format(time.RFC3339)
			info.DBModifiedAt = &mod
		} else {
			info.DBSizeHuman = "—"
		}
	}
	if s.db != nil {
		// Aggregates, not a full IssueLites materialization: counts never
		// needed the rows (GDK-610). CommentCount is issue comments only —
		// IssueCommentCount, not TableCount("comments"), because page
		// comments share the comments table.
		if n, err := s.db.TableCount(ctx, "issues"); err == nil {
			info.IssueCount = n
		}
		if n, err := s.db.IssueCommentCount(ctx); err == nil {
			info.CommentCount = n
		}
		info.SchemaVersion = s.db.SchemaVersion()
		if st, err := s.db.SyncState(ctx, sourceID); err == nil {
			info.Watermark = st.Watermark
			info.SyncVersion = st.Version
			info.LastFullSyncAt = st.LastFullSyncAt
			info.LastError = st.LastError
			if st.SchemaVersion > 0 {
				info.SchemaVersion = st.SchemaVersion
			}
		}
		if summary, err := s.db.APIUsageSummary(ctx); err == nil {
			info.ApiUsage = &summary
		}
	}
	return info
}

// profileDisplay returns the UI name for a profile ("" / "default" → "default").
func profileDisplay(profile string) string {
	return config.NormalizeProfile(profile)
}

// uiOrEmpty keeps the settings GET bindable when nothing is overridden — the
// UI edits and PUTs back the object it read, so an absent key must mean "old
// client", never "empty".
func uiOrEmpty(u *config.UIConfig) *config.UIConfig {
	if u == nil {
		return &config.UIConfig{}
	}
	return u
}

func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// The helpers below only replace nil with empty, so the documents carry `[]` and
// `{}` instead of `null` and the UI can bind to them without guards.

// features projects the optional-surface flags. An explicit key in the config
// wins; missing keys stay off except feed, which defaults on so the personal
// feed surface is available without a config edit (still overridable to false).
func features(set map[string]bool) map[string]bool {
	return config.NormalizeFeatures(set)
}

func strPtr(s string) *string { return &s }

func strs(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func strMap(v map[string]string) map[string]string {
	if v == nil {
		return map[string]string{}
	}
	return v
}

func products(v map[string]config.Product) map[string]config.Product {
	if v == nil {
		return map[string]config.Product{}
	}
	return v
}

func members(v []config.Member) []config.Member {
	if v == nil {
		return []config.Member{}
	}
	return v
}

func rules(v []config.GroupRule) []config.GroupRule {
	if v == nil {
		return []config.GroupRule{}
	}
	return v
}
