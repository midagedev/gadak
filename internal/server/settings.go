package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/confluence"
	"github.com/midagedev/gadak/internal/store"
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

// Every optional surface a config flag can switch on. Unknown keys in the
// configuration are dropped: a flag nobody reads is a flag that does nothing.
var featureNames = []string{"feed", "push", "deploy", "qa", "teamGroups"}

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

// settingsDoc is everything the settings UI may read and write. The credential
// block (email, token) is absent by construction, not by filtering.
type settingsDoc struct {
	Projects             []string                  `json:"projects"`
	FieldMap             map[string]string         `json:"fieldMap"`
	BodyFields           []string                  `json:"bodyFields"`
	EditableFields       map[string]string         `json:"editableFields"`
	Members              []config.Member           `json:"members"`
	GroupRules           []config.GroupRule        `json:"groupRules"`
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
	writeJSON(w, http.StatusOK, s.settingsResponse(s.config()))
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
	c := confluence.New(cfg.Site, cfg.Email, cfg.Token)
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
	// global first, then name (case-insensitive) within each group.
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := out[i].Type == "global", out[j].Type == "global"
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
	if err := validateIntervals(in.SyncIntervalSec, in.ReconcileIntervalSec); err != nil {
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
		switch {
		case in.Confluence.Enabled != nil && !*in.Confluence.Enabled:
			next.Confluence = nil
		case in.Confluence.Enabled != nil && *in.Confluence.Enabled:
			// Always assign a fresh struct — never mutate a shared nested pointer.
			next.Confluence = &config.ConfluenceConfig{Spaces: strs(in.Confluence.Spaces)}
		default:
			// enabled omitted: spaces-only update, same as before.
			if next.Confluence == nil {
				fail(w, http.StatusBadRequest, "confluence_not_configured")
				return
			}
			// Copy the section so we do not mutate the shared atomic config
			// pointer's nested struct before Save succeeds.
			cc := *next.Confluence
			cc.Spaces = strs(in.Confluence.Spaces)
			next.Confluence = &cc
		}
	}
	next.BodyFields = in.BodyFields
	next.EditableFields = in.EditableFields
	next.Members = in.Members
	next.GroupRules = in.GroupRules
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
	if scopeChanged(prev, &next) && next.HasCredential() {
		_ = s.startSyncJob(&next, true)
	}
	writeJSON(w, http.StatusOK, s.settingsResponse(&next))
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
	ma, mb := stringSet(a), stringSet(b)
	if len(ma) != len(mb) {
		return false
	}
	for k := range ma {
		if _, ok := mb[k]; !ok {
			return false
		}
	}
	return true
}

func stringSet(v []string) map[string]struct{} {
	out := make(map[string]struct{}, len(v))
	for _, s := range v {
		out[s] = struct{}{}
	}
	return out
}

func validateIntervals(syncSec, reconcileSec int) error {
	if syncSec < 0 {
		return fmt.Errorf("syncIntervalSec must be 0 (default) or a positive number of seconds")
	}
	if syncSec > 0 && syncSec < config.MinSyncIntervalSec {
		return fmt.Errorf("syncIntervalSec must be 0 (default) or >= %d (got %d)", config.MinSyncIntervalSec, syncSec)
	}
	if reconcileSec < 0 {
		return fmt.Errorf("reconcileIntervalSec must be 0 (default) or a positive number of seconds")
	}
	if reconcileSec > 0 && reconcileSec < config.MinReconcileIntervalSec {
		return fmt.Errorf("reconcileIntervalSec must be 0 (default) or >= %d (got %d)", config.MinReconcileIntervalSec, reconcileSec)
	}
	return nil
}

func settings(cfg *config.Config) settingsDoc {
	doc := settingsDoc{
		Projects:             strs(cfg.Projects),
		FieldMap:             strMap(cfg.FieldMap),
		BodyFields:           strs(cfg.BodyFields),
		EditableFields:       strMap(cfg.EditableFields),
		Members:              members(cfg.Members),
		GroupRules:           rules(cfg.GroupRules),
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
	}
	if cfg.Confluence != nil {
		doc.Confluence = &settingsConfluenceDoc{Spaces: strs(cfg.Confluence.Spaces)}
	}
	return doc
}

func (s *server) settingsResponse(cfg *config.Config) settingsDoc {
	doc := settings(cfg)
	doc.Runtime = s.runtimeInfo()
	// Read-only discovery surfaces — never written by handlePutSettings.
	specs := cfg.FieldSpecs()
	if specs == nil {
		doc.FieldSpecsOut = []config.FieldSpec{}
	} else {
		doc.FieldSpecsOut = specs
	}
	doc.FieldUsage = map[string]map[string]int{}
	if s.db != nil {
		if rows, err := s.db.FieldUsage(context.Background()); err == nil {
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

func (s *server) runtimeInfo() *runtimeInfo {
	info := &runtimeInfo{
		Profile:                     profileDisplay(s.profile),
		GadakVersion:                Version,
		DefaultSyncIntervalSec:      config.DefaultSyncIntervalSec,
		DefaultReconcileIntervalSec: config.DefaultReconcileIntervalSec,
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
		if lites, err := s.db.IssueLites(context.Background()); err == nil {
			info.IssueCount = len(lites)
			comments := 0
			for _, l := range lites {
				comments += l.CommentCount
			}
			info.CommentCount = comments
		}
		info.SchemaVersion = s.db.SchemaVersion()
		if st, err := s.db.SyncState(context.Background(), sourceID); err == nil {
			info.Watermark = st.Watermark
			info.SyncVersion = st.Version
			info.LastFullSyncAt = st.LastFullSyncAt
			info.LastError = st.LastError
			if st.SchemaVersion > 0 {
				info.SchemaVersion = st.SchemaVersion
			}
		}
		if summary, err := s.db.APIUsageSummary(context.Background()); err == nil {
			info.ApiUsage = &summary
		}
	}
	return info
}

// profileDisplay returns the UI name for a profile ("" / "default" → "default").
func profileDisplay(profile string) string {
	if profile == "" || profile == "default" {
		return "default"
	}
	return profile
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
	out := make(map[string]bool, len(featureNames))
	for _, name := range featureNames {
		if set != nil {
			if v, ok := set[name]; ok {
				out[name] = v
				continue
			}
		}
		out[name] = name == "feed"
	}
	return out
}

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
