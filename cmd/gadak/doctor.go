package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/applog"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

// doctorBanner is the first line of every doctor dump so a user pasting into a
// public issue can see what was stripped before they hit submit.
const doctorBanner = "# gadak doctor — safe to paste: counts and versions only, no keys, names, URLs or tokens"

// doctorReport is the redacted diagnostic document. Field names are stable for
// --json consumers; values never carry tokens, hostnames, emails, project
// keys, custom-field names, or raw error strings.
type doctorReport struct {
	GadakVersion    string `json:"gadak_version"`
	GoVersion       string `json:"go_version"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	Profile         string `json:"profile"`
	WorkspaceSource string `json:"workspace_source"`
	WorkspaceKind   string `json:"workspace_kind"`
	Origin          string `json:"origin"`
	OriginOwner     string `json:"origin_owner,omitempty"`
	MirrorPath      string `json:"mirror_path"`
	// HomeLeftover is the abandoned legacy home (~/.scry) when it still exists
	// beside ~/.gadak. The stderr warning fires once per machine (GDK-1072);
	// this is the standing report.
	HomeLeftover string `json:"home_leftover,omitempty"`
	// ProjectsMismatch is the config↔mirror project-scope rename signature
	// (GDK-973), as counts only — the keys themselves stay out of this
	// paste-safe document (the rule the banner states and TestDoctorRedaction
	// enforces). The post-sync warning and `gadak status` name the keys. Nil
	// when the signature is absent.
	ProjectsMismatch *doctorProjectsMismatch `json:"projects_mismatch,omitempty"`
	Mirror           doctorMirror            `json:"mirror"`
	MirrorHolders    *doctorMirrorHolders    `json:"mirror_holders,omitempty"`
	SchemaVersion    *int                    `json:"schema_version"`
	SchemaSinceSync  string                  `json:"schema_since_sync,omitempty"`
	SchemaAudit      *doctorSchemaAudit      `json:"schema_audit,omitempty"`
	Migrations       string                  `json:"migrations"`
	Counts           *doctorCounts           `json:"counts"`
	Confluence       string                  `json:"confluence"`
	CustomFields     doctorCustomFields      `json:"custom_fields"`
	Credential       string                  `json:"credential"`
	Site             string                  `json:"site"`
	Email            string                  `json:"email"`
	Skill            doctorSkill             `json:"skill"`
	MCP              doctorMCP               `json:"mcp"`
	Sync             map[string]doctorSync   `json:"sync"`
	APIUsage         doctorAPIUsage          `json:"api_usage"`
	Workspace        doctorWorkspace         `json:"workspace"`
	Logs             doctorLogs              `json:"logs"`
}

// doctorLogs is the process log file Install opens under the gadak home.
// Path is tilde-abbreviated like mirror_path. Size is omitted when the
// file is absent. Recent is error-ish ring lines, cap 10.
type doctorLogs struct {
	Path    string   `json:"path"`
	Size    *int64   `json:"size,omitempty"`
	Rotated bool     `json:"rotated"`
	Recent  []string `json:"recent,omitempty"`
}

// doctorWorkspace is the one-line consistency view: kind, whether a site
// token is stored (never the token itself), the origin persist path, and
// how many locally originated issues LocalData counts. Inconsistent is
// standalone-with-a-token — a site token on a standalone workspace is
// unused and contradicts Kind (GDK-247).
//
// HasSiteToken is site-token presence, not config.HasCredential (GDK-470).
// Standalone writes work with no site token; this field stays false there.
// doctorCustomFields is the mapping-visibility object (GDK-522). mapped is
// the configured alias count; applied_at is when `gadak fields --apply` last
// succeeded. usage_rows and raw_has_custom need the mirror (0 / false when
// it is missing). The previous JSON value was that same mapped count as an
// int; mapped preserves it.
type doctorCustomFields struct {
	Mapped       int    `json:"mapped"`
	AppliedAt    string `json:"applied_at,omitempty"`
	UsageRows    int    `json:"usage_rows"`
	RawHasCustom bool   `json:"raw_has_custom"`
	// rawScanned is true only when HasCustomFieldKeysInRaw actually ran.
	rawScanned bool
}

type doctorWorkspace struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	HasSiteToken bool   `json:"has_site_token"`
	Persist      string `json:"persist"`
	LocalIssues  int    `json:"local_issues"`
	Inconsistent bool   `json:"inconsistent"`
	Frozen       bool   `json:"frozen"`
}

// doctorSkill answers "is my Claude Code skill current?" without the user
// having to diff files by hand. Identity is the content hash, never mtime.
type doctorSkill struct {
	Status string `json:"status"` // missing | stale | current
	Scope  string `json:"scope"`  // user | project
	Path   string `json:"path"`   // tilde-abbreviated (user) or repo-relative (project)
	// LastAutoCheck is when the daily auto-sync last looked (GDK-996), so
	// "why did/didn't it update?" is one line. Same convention as
	// sync.*.synced_at: "never" before the first check.
	LastAutoCheck string `json:"last_auto_check"`
}

// doctorMCP reports whether gadak is registered as an MCP server. Only the
// config file and the scope name are reported — never the project directory a
// local registration is filed under, which would put the user's working path in
// a document the banner promises is safe to paste.
type doctorMCP struct {
	Status string `json:"status"`          // absent | registered
	Scope  string `json:"scope,omitempty"` // user | local | project | other
	Path   string `json:"path,omitempty"`  // tilde-abbreviated config path
}

// doctorProjectsMismatch is the count-only view of the config↔mirror
// project-scope rename signature (GDK-973): configured keys the Jira mirror
// holds no issues under, beside Jira-mirrored keys the config does not list.
// Which keys those are never enters doctor output — the banner promises a
// paste-safe document; the post-sync warning and `gadak status` name them.
type doctorProjectsMismatch struct {
	ConfiguredNotMirrored       int `json:"configured_not_mirrored"`        // configured keys with zero mirrored issues
	MirroredNotConfigured       int `json:"mirrored_not_configured"`        // Jira-mirrored keys outside the config
	MirroredNotConfiguredIssues int `json:"mirrored_not_configured_issues"` // issues held under those keys
}

type doctorMirror struct {
	Status string `json:"status"`           // present | not_found | open_error | schema_too_new
	Bytes  *int64 `json:"bytes,omitempty"`  // set when the file exists
	Detail string `json:"detail,omitempty"` // redacted path / short reason
	// Version and ChangedAt are the live-update signal, reported so "why is
	// my open board not showing what I just wrote?" is one command instead of
	// a guess (GDK-1170). The ui-focus poll hands Version to the web every
	// 500ms and the board pulls a delta when it moves; ChangedAt (RFC3339,
	// UTC) is when the mirror bytes last moved. A write that leaves ChangedAt
	// in the past is a mirror that never got refreshed — the board is right
	// and the write-through is the suspect. Both are counters and a
	// timestamp: nothing here names a file, a person, or a key.
	Version   string `json:"version,omitempty"`
	ChangedAt string `json:"changed_at,omitempty"`
}

// doctorMirrorHolders is the best-effort list of other processes that have
// this profile's mirror open (GDK-740). Nil means the scan was skipped
// (non-darwin/linux, lsof missing, or it failed). Count 0 is a successful
// scan that found nobody but us.
type doctorMirrorHolders struct {
	Count     int                   `json:"count"`
	Processes []doctorMirrorProcess `json:"processes,omitempty"`
}

type doctorMirrorProcess struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

// doctorSchemaAudit is the user_version-vs-DDL check (GDK-180). Status is
// ok | mismatch | error. Sample is a handful of missing names, never the
// full list; table/column identifiers are schema, not personal data, but
// doctor still keeps the paste short.
type doctorSchemaAudit struct {
	Status    string   `json:"status"`
	Stamp     int      `json:"stamp"`
	Supported int      `json:"supported"`
	Missing   int      `json:"missing"`
	Extra     int      `json:"extra"`
	Sample    []string `json:"sample,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

type doctorCounts struct {
	Items            int `json:"items"`
	Issues           int `json:"issues"`
	Pages            int `json:"pages"`
	Comments         int `json:"comments"`
	Projects         int `json:"projects"`
	StatusCategories int `json:"status_categories"`
	Spaces           int `json:"spaces"`
}

type doctorSync struct {
	SyncedAt   string `json:"synced_at"`  // RFC3339-ish or "never"
	Watermark  string `json:"watermark"`  // present | absent
	LastError  string `json:"last_error"` // classified only
	SyncCount  int64  `json:"sync_count"`
	LastFullAt string `json:"last_full_sync_at"` // or "never"
}

type doctorAPIUsage struct {
	Day       string `json:"day"`
	Requests  int64  `json:"requests"`
	Throttled int64  `json:"throttled"`
	Retries   int64  `json:"retries"`
}

func cmdDoctor(args []string) error {
	fs := newFlagSet("doctor")
	asJSON := fs.Bool("json", false, "emit the same document as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rep := collectDoctor()
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	fmt.Print(formatDoctorText(rep))
	return nil
}

func collectDoctor() doctorReport {
	profile := config.Profile()
	if profile == "" {
		profile = "default"
	}

	rep := doctorReport{
		GadakVersion:    version,
		GoVersion:       runtime.Version(),
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		Profile:         profile,
		WorkspaceSource: workspaceJSONSource(),
		WorkspaceKind:   config.KindConnected,
		Origin:          "jira",
		Confluence:      "inactive",
		Credential:      "absent",
		Site:            "none",
		Email:           "none",
		Sync:            map[string]doctorSync{},
		APIUsage: doctorAPIUsage{
			Day: time.Now().UTC().Format("2006-01-02"),
		},
		Migrations: "unknown",
		Workspace: doctorWorkspace{
			Name: workspaceJSONName(),
			Kind: config.KindConnected,
		},
	}

	if path, err := config.DBPath(); err == nil {
		rep.MirrorPath = tildeHome(path)
	} else {
		rep.MirrorPath = "unknown"
	}
	if prev := config.DualHomeLeftover(); prev != "" {
		rep.HomeLeftover = tildeHome(prev)
	}

	if cfg, err := config.Load(); err == nil && cfg != nil {
		if cfg.Token != "" {
			rep.Credential = "present"
		}
		rep.Site = redactSite(cfg.Site)
		if cfg.Email != "" {
			rep.Email = "configured"
		}
		rep.CustomFields.Mapped = len(cfg.FieldSpecs())
		rep.CustomFields.AppliedAt = cfg.FieldsAppliedAt
		if cfg.Confluence != nil {
			rep.Confluence = "active"
		}
		kind, src := origin.Describe(cfg)
		rep.WorkspaceKind = kind
		if kind == config.KindStandalone {
			// Persist path is the origin; tilde so the account username
			// does not appear (same rule as mirror_path).
			rep.Origin = tildeHome(src)
			rep.OriginOwner = origin.OwnerStatus(cfg)
		} else {
			rep.Origin = src
		}
		n, persist, _ := originbind.LocalData(cfg)
		hasTok := cfg.Token != ""
		if rem, err := origin.PairedStatus(cfg); err == nil && rem != nil {
			// Same redaction as site: no hostname. Label is the pairing
			// identity doctor can name without leaking the endpoint.
			if rem.Label != "" {
				rep.Origin = fmt.Sprintf("paired gadak serve (label %q)", rem.Label)
			} else {
				rep.Origin = "paired gadak serve"
			}
			hasTok = true
			if cfg.Token == "" {
				rep.Credential = "present"
			}
		}
		rep.Workspace = doctorWorkspace{
			Name:         workspaceJSONName(),
			Kind:         cfg.WorkspaceKind(),
			HasSiteToken: hasTok,
			Persist:      tildeHome(persist),
			LocalIssues:  n,
			Inconsistent: cfg.IsStandalone() && hasTok,
			Frozen:       cfg.SyncFrozen(),
		}
	}

	// Agent wiring is independent of the mirror, and the mirror branch below
	// returns early — collect it first so a user with no mirror still gets the
	// answer to "is my skill current?".
	rep.Skill = collectSkillStatus()
	rep.Skill.LastAutoCheck = doctorSkillAutoCheckWord(lastSkillAutoCheck())
	rep.MCP = collectMCPStatus()
	rep.Logs = collectLogs()

	path, err := config.DBPath()
	if err != nil {
		rep.Mirror = doctorMirror{Status: "open_error", Detail: "path unavailable"}
		return rep
	}

	info, statErr := os.Stat(path)
	if os.IsNotExist(statErr) {
		rep.Mirror = doctorMirror{
			Status: "not_found",
			Detail: "not found at " + tildeHome(path),
		}
		return rep
	}
	if statErr != nil {
		rep.Mirror = doctorMirror{
			Status: "open_error",
			Detail: "stat failed",
		}
		return rep
	}
	size := info.Size()
	rep.Mirror = doctorMirror{Status: "present", Bytes: &size}
	// Read before store.Open below: opening mints -wal/-shm when they are
	// absent, which is itself a move. This has to report what an open board's
	// poll would have seen a moment ago, not what doctor just caused.
	rep.Mirror.Version = store.MirrorVersion(path)
	if at := store.MirrorChangedAt(path); !at.IsZero() {
		rep.Mirror.ChangedAt = at.UTC().Format(time.RFC3339)
	}
	rep.MirrorHolders = listMirrorHolders(path)

	db, err := store.Open(path)
	if err != nil {
		// doctor is what someone runs when the mirror has stopped opening, so
		// "open failed" is the one answer it must not give for a cause it can
		// name (GDK-498). The version pair is the whole diagnosis here.
		var tooNew *store.SchemaTooNewError
		if errors.As(err, &tooNew) {
			rep.Mirror.Status = "schema_too_new"
			rep.Mirror.Detail = fmt.Sprintf("written by a newer gadak; this build reads up to %d — run the newer gadak, or set the file aside and re-sync", tooNew.Supported)
			rep.SchemaVersion = &tooNew.Have
			rep.Migrations = "none applied"
			return rep
		}
		rep.Mirror.Status = "open_error"
		rep.Mirror.Detail = "open failed"
		return rep
	}
	defer db.Close()

	sv := db.SchemaVersion()
	rep.SchemaVersion = &sv
	if sv > 0 {
		rep.Migrations = fmt.Sprintf("1..%d", sv)
	} else {
		rep.Migrations = "none"
	}
	rep.SchemaSinceSync = doctorSchemaSinceSync(db, sv)
	rep.SchemaAudit = collectSchemaAudit(db)

	counts := &doctorCounts{}
	if n, err := db.TableCount(context.Background(), "items"); err == nil {
		counts.Items = n
	}
	if n, err := db.TableCount(context.Background(), "issues"); err == nil {
		counts.Issues = n
	}
	if n, err := db.TableCount(context.Background(), "pages"); err == nil {
		counts.Pages = n
	}
	if n, err := db.TableCount(context.Background(), "comments"); err == nil {
		counts.Comments = n
	}
	if n, err := db.DistinctCount(context.Background(), "issues", "project_key"); err == nil {
		counts.Projects = n
	}
	if n, err := db.DistinctCount(context.Background(), "issues", "status_category"); err == nil {
		counts.StatusCategories = n
	}
	// Prefer the spaces catalog; fall back to distinct keys on mirrored pages
	// when the catalog was never filled (older snapshots).
	if n, err := db.TableCount(context.Background(), "spaces"); err == nil && n > 0 {
		counts.Spaces = n
	} else if n, err := db.DistinctCount(context.Background(), "pages", "space_key"); err == nil {
		counts.Spaces = n
	}
	rep.Counts = counts

	if pm := collectProjectsMismatch(db); pm != nil {
		rep.ProjectsMismatch = pm
	}

	for _, src := range []string{"jira", "confluence"} {
		rep.Sync[src] = collectSync(db, src)
	}

	if usage, err := db.APIUsageSummary(context.Background()); err == nil {
		rep.APIUsage = doctorAPIUsage{
			Day:       usage.Today.Day,
			Requests:  usage.Today.Requests,
			Throttled: usage.Today.Throttled,
			Retries:   usage.Today.Retries,
		}
	}

	ctx := context.Background()
	if rows, err := db.FieldUsage(ctx); err == nil {
		rep.CustomFields.UsageRows = len(rows)
	}
	// The raw probe reads whole documents (~12 KB each; 230 MB at 20k issues
	// when no custom field exists — the exact-no case GDK-749). doctor's hint
	// samples instead: a hit is actionable advice either way, and a miss says
	// "none seen", which is all the hint ever claimed. `fields --apply` keeps
	// the exact probe, where a false no would wrongly refuse a sync.
	if rep.CustomFields.Mapped == 0 {
		if has, err := db.HasCustomFieldKeysInRawSampled(ctx, doctorRawSampleLimit); err == nil {
			rep.CustomFields.RawHasCustom = has
			rep.CustomFields.rawScanned = true
		}
	}

	return rep
}

// collectSkillStatus reuses the installer's own classifier (skillDestStatus in
// skill.go), so `gadak doctor` and `gadak skill install --print` can never
// disagree about the same file.
// collectProjectsMismatch reads the config↔mirror rename signature through
// sync.ProjectScopeMismatch — the same verdict the post-Jira-pass warning
// logs — so doctor and sync can never disagree about it. Reads are
// best-effort: an unopenable config or count query reports nothing rather
// than a half verdict.
func collectProjectsMismatch(db *store.DB) *doctorProjectsMismatch {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return nil
	}
	counts, err := db.ProjectIssueCounts(context.Background(), syncer.SourceID)
	if err != nil {
		return nil
	}
	empty, extra := syncer.ProjectScopeMismatch(cfg, counts)
	if len(extra) == 0 {
		return nil
	}
	out := &doctorProjectsMismatch{
		ConfiguredNotMirrored: len(empty),
		MirroredNotConfigured: len(extra),
	}
	for _, k := range extra {
		out.MirroredNotConfiguredIssues += counts[k]
	}
	return out
}

// formatDoctorProjectsMismatch names the verdict and the way out without
// naming keys: `gadak status` prints both scope-mismatch lines (GDK-809).
func formatDoctorProjectsMismatch(m doctorProjectsMismatch) string {
	return fmt.Sprintf("configured_not_mirrored=%d mirrored_not_configured=%d (%d issues) — a renamed Jira project key? `gadak status` lists the keys; fix with: gadak config set projects '[\"…\"]'",
		m.ConfiguredNotMirrored, m.MirroredNotConfigured, m.MirroredNotConfiguredIssues)
}

func collectSkillStatus() doctorSkill {
	content := gadak.SkillMarkdown()

	userDest, userErr := resolveSkillDest(false, "")
	if userErr == nil {
		if status, _, err := skillDestStatus(userDest, content); err == nil && status != "missing" {
			return doctorSkill{Status: doctorSkillWord(status), Scope: "user", Path: tildeHome(userDest)}
		}
	}
	// Nothing at the user scope — a `--project` install still counts. Report it
	// with the literal relative path: printing the working directory would put
	// the user's project name in a report meant to be pasted in public.
	if projDest, err := resolveSkillDest(true, ""); err == nil {
		if status, _, err := skillDestStatus(projDest, content); err == nil && status != "missing" {
			return doctorSkill{
				Status: doctorSkillWord(status),
				Scope:  "project",
				Path:   filepath.Join(".claude", "skills", "gadak", "SKILL.md"),
			}
		}
	}
	path := "unknown"
	if userErr == nil {
		path = tildeHome(userDest)
	}
	return doctorSkill{Status: "missing", Scope: "user", Path: path}
}

// doctorSkillWord maps the installer's four-way classification onto the three
// words doctor reports. "conflict" (a file gadak did not write) reports as
// stale because either way what the agent loads is not this binary's skill;
// `gadak skill install --print` is the command that tells the two apart.
func doctorSkillWord(installStatus string) string {
	if installStatus == "identical" {
		return "current"
	}
	return "stale"
}

// doctorSkillAutoCheckWord maps the empty auto-sync stamp (no check has run
// yet on this machine) to "never".
func doctorSkillAutoCheckWord(stamp string) string {
	if stamp == "" {
		return "never"
	}
	return stamp
}

// claudeMCPConfig is the slice of Claude Code's config that says whether gadak
// is registered. `claude mcp add` (what `gadak mcp install claude` runs) files
// the entry under projects[<cwd>] at its default scope, and under the top-level
// mcpServers at user scope.
type claudeMCPConfig struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	} `json:"projects"`
}

// claudeConfigMaxBytes caps the read of a config file doctor does not own.
const claudeConfigMaxBytes = 32 << 20

func collectMCPStatus() doctorMCP {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cfgPath := filepath.Join(home, ".claude.json")
		if cfg, ok := readClaudeMCPConfig(cfgPath); ok {
			if _, found := cfg.MCPServers["gadak"]; found {
				return doctorMCP{Status: "registered", Scope: "user", Path: tildeHome(cfgPath)}
			}
			if cwd, err := os.Getwd(); err == nil {
				if _, found := cfg.Projects[cwd].MCPServers["gadak"]; found {
					return doctorMCP{Status: "registered", Scope: "local", Path: tildeHome(cfgPath)}
				}
			}
			for _, p := range cfg.Projects {
				if _, found := p.MCPServers["gadak"]; found {
					// Registered, but filed under some other directory — the
					// directory itself is never printed.
					return doctorMCP{Status: "registered", Scope: "other", Path: tildeHome(cfgPath)}
				}
			}
		}
	}
	// Project-scoped registrations live in a .mcp.json beside the checkout.
	if cfg, ok := readClaudeMCPConfig(".mcp.json"); ok {
		if _, found := cfg.MCPServers["gadak"]; found {
			return doctorMCP{Status: "registered", Scope: "project", Path: ".mcp.json"}
		}
	}
	return doctorMCP{Status: "absent"}
}

// readClaudeMCPConfig is best-effort: a missing, oversized, or unparsable file
// is simply "no registration found". doctor never writes to it.
func readClaudeMCPConfig(path string) (claudeMCPConfig, bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() || fi.Size() > claudeConfigMaxBytes {
		return claudeMCPConfig{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return claudeMCPConfig{}, false
	}
	var cfg claudeMCPConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return claudeMCPConfig{}, false
	}
	return cfg, true
}

const schemaAuditSampleCap = 3

// doctorRawSampleLimit bounds doctor's raw customfield probe (GDK-749).
const doctorRawSampleLimit = 500

func collectSchemaAudit(db *store.DB) *doctorSchemaAudit {
	got, err := db.SchemaAudit(context.Background())
	if err != nil {
		return &doctorSchemaAudit{Status: "error", Detail: "audit failed"}
	}
	out := &doctorSchemaAudit{
		Status:    "ok",
		Stamp:     got.Stamp,
		Supported: got.Supported,
		Missing:   len(got.Missing),
		Extra:     len(got.Extra),
	}
	if len(got.Missing) > 0 {
		out.Status = "mismatch"
		out.Sample = schemaAuditSample(got.Missing, schemaAuditSampleCap)
	}
	return out
}

func schemaAuditSample(names []string, cap int) []string {
	if len(names) <= cap {
		return append([]string(nil), names...)
	}
	return append([]string(nil), names[:cap]...)
}

func formatDoctorSchemaAudit(a doctorSchemaAudit) string {
	switch a.Status {
	case "ok":
		return "ok"
	case "mismatch":
		sample := strings.Join(a.Sample, ", ")
		if a.Missing > len(a.Sample) && sample != "" {
			sample += ", ..."
		}
		if sample == "" {
			return fmt.Sprintf("mismatch (%d missing) stamp=%d this_build=%d — mirror is damaged; delete the mirror file and run gadak sync",
				a.Missing, a.Stamp, a.Supported)
		}
		return fmt.Sprintf("mismatch (%d missing: %s) stamp=%d this_build=%d — mirror is damaged; delete the mirror file and run gadak sync",
			a.Missing, sample, a.Stamp, a.Supported)
	default:
		if a.Detail != "" {
			return a.Status + " (" + a.Detail + ")"
		}
		return a.Status
	}
}

// doctorSchemaSinceSync names a lag between PRAGMA user_version and the
// sync_state.schema_version column. The column is only rewritten when a
// migration actually runs, so a later Open can leave it behind (GDK-526).
func doctorSchemaSinceSync(db *store.DB, live int) string {
	seen := map[int]struct{}{}
	var stale []int
	for _, src := range []string{"jira", "confluence"} {
		ss, err := db.SyncState(context.Background(), src)
		if err != nil {
			continue
		}
		row := ss.SchemaVersionRow
		if row <= 0 || row == live {
			continue
		}
		if _, ok := seen[row]; ok {
			continue
		}
		seen[row] = struct{}{}
		stale = append(stale, row)
	}
	if len(stale) == 0 {
		return ""
	}
	parts := make([]string, len(stale))
	for i, n := range stale {
		parts[i] = strconv.Itoa(n)
	}
	return "migrated since last sync (sync_state has " + strings.Join(parts, ", ") + ")"
}

func collectSync(db *store.DB, sourceID string) doctorSync {
	out := doctorSync{
		SyncedAt:   "never",
		Watermark:  "absent",
		LastError:  "none",
		LastFullAt: "never",
	}
	ss, err := db.SyncState(context.Background(), sourceID)
	if err != nil {
		out.LastError = "error"
		return out
	}
	if ss.SyncedAt != nil && *ss.SyncedAt != "" {
		out.SyncedAt = *ss.SyncedAt
	}
	if ss.Watermark != "" {
		out.Watermark = "present"
	}
	if ss.LastError != nil && *ss.LastError != "" {
		out.LastError = classifyLastError(*ss.LastError)
	}
	if ss.LastFullSyncAt != nil && *ss.LastFullSyncAt != "" {
		out.LastFullAt = *ss.LastFullSyncAt
	}
	out.SyncCount = ss.SyncCount
	return out
}

func formatDoctorText(r doctorReport) string {
	var b strings.Builder
	b.WriteString(doctorBanner)
	b.WriteByte('\n')
	b.WriteByte('\n')

	line := func(k, v string) {
		fmt.Fprintf(&b, "%-22s %s\n", k+":", v)
	}

	line("gadak_version", r.GadakVersion)
	line("go_version", r.GoVersion)
	line("os", r.OS+"/"+r.Arch)
	line("profile", r.Profile)
	line("workspace_kind", r.WorkspaceKind)
	line("origin", r.Origin)
	if r.OriginOwner != "" {
		line("origin owner", r.OriginOwner)
	}
	line("workspace", formatDoctorWorkspace(r.Workspace))
	line("mirror_path", r.MirrorPath)
	if r.HomeLeftover != "" {
		line("home_leftover", r.HomeLeftover+" (ignored legacy home — delete it, or move anything you still need into the active home)")
	}
	if r.MirrorHolders != nil {
		line("mirror_holders", formatDoctorMirrorHolders(*r.MirrorHolders))
	}

	switch r.Mirror.Status {
	case "present":
		if r.Mirror.Bytes != nil {
			line("mirror", fmt.Sprintf("present (%s)", formatBytes(*r.Mirror.Bytes)))
		} else {
			line("mirror", "present")
		}
	case "not_found":
		line("mirror", r.Mirror.Detail)
	default:
		if r.Mirror.Detail != "" {
			line("mirror", r.Mirror.Status+" ("+r.Mirror.Detail+")")
		} else {
			line("mirror", r.Mirror.Status)
		}
	}
	if r.Mirror.Version != "" {
		line("mirror_version", formatDoctorMirrorVersion(r.Mirror))
	}

	if r.SchemaVersion != nil {
		line("schema_version", strconv.Itoa(*r.SchemaVersion))
	} else {
		line("schema_version", "unknown")
	}
	if r.SchemaSinceSync != "" {
		line("schema_since_sync", r.SchemaSinceSync)
	}
	if r.SchemaAudit != nil {
		line("schema_audit", formatDoctorSchemaAudit(*r.SchemaAudit))
	}
	line("migrations", r.Migrations)

	if r.Counts != nil {
		line("items", strconv.Itoa(r.Counts.Items))
		line("issues", strconv.Itoa(r.Counts.Issues))
		line("pages", strconv.Itoa(r.Counts.Pages))
		line("comments", strconv.Itoa(r.Counts.Comments))
		line("projects", strconv.Itoa(r.Counts.Projects))
		line("status_categories", strconv.Itoa(r.Counts.StatusCategories))
		line("spaces", strconv.Itoa(r.Counts.Spaces))
	} else {
		line("items", "n/a")
		line("issues", "n/a")
		line("pages", "n/a")
		line("comments", "n/a")
		line("projects", "n/a")
		line("status_categories", "n/a")
		line("spaces", "n/a")
	}

	if r.ProjectsMismatch != nil {
		line("projects_mismatch", formatDoctorProjectsMismatch(*r.ProjectsMismatch))
	}

	line("custom_fields", formatDoctorCustomFields(r.CustomFields))
	line("confluence", r.Confluence)
	line("credential", r.Credential)
	line("site", r.Site)
	line("email", r.Email)

	if r.Skill.Path != "" {
		line("skill", r.Skill.Status+" ("+r.Skill.Path+")")
	} else {
		line("skill", r.Skill.Status)
	}
	if r.Skill.LastAutoCheck != "" {
		line("skill.last_auto_check", r.Skill.LastAutoCheck)
	}
	if r.MCP.Path != "" {
		line("mcp", r.MCP.Status+" ("+r.MCP.Path+", "+r.MCP.Scope+")")
	} else {
		line("mcp", r.MCP.Status)
	}

	for _, src := range []string{"jira", "confluence"} {
		s, ok := r.Sync[src]
		if !ok {
			continue
		}
		line("sync."+src+".synced_at", s.SyncedAt)
		line("sync."+src+".watermark", s.Watermark)
		line("sync."+src+".last_error", s.LastError)
		line("sync."+src+".sync_count", strconv.FormatInt(s.SyncCount, 10))
		line("sync."+src+".last_full_sync_at", s.LastFullAt)
	}

	line("api_usage.day", r.APIUsage.Day)
	line("api_usage.requests", strconv.FormatInt(r.APIUsage.Requests, 10))
	line("api_usage.throttled", strconv.FormatInt(r.APIUsage.Throttled, 10))
	line("api_usage.retries", strconv.FormatInt(r.APIUsage.Retries, 10))

	if r.Logs.Path != "" {
		line("logs.path", r.Logs.Path)
		if r.Logs.Size != nil {
			line("logs.size", formatBytes(*r.Logs.Size))
		} else {
			line("logs.size", "not found")
		}
		rotated := "no"
		if r.Logs.Rotated {
			rotated = "yes"
		}
		line("logs.rotated", rotated)
		if len(r.Logs.Recent) > 0 {
			line("logs.recent", strings.Join(r.Logs.Recent, " | "))
		}
	}

	return b.String()
}

func collectLogs() doctorLogs {
	dir, err := config.DirFor("")
	if err != nil {
		return doctorLogs{Path: "unknown"}
	}
	p := applog.Path(dir)
	out := doctorLogs{Path: tildeHome(p)}
	if fi, err := os.Stat(p); err == nil {
		sz := fi.Size()
		out.Size = &sz
	}
	if _, err := os.Stat(p + ".1"); err == nil {
		out.Rotated = true
	}
	// The ring only holds what this process logged, and doctor is usually a
	// fresh process that has logged nothing — so the file is the real source
	// here and the ring is the fallback for a long-lived one (serve).
	if lines := errorishLogLines(applog.Tail(dir, 2000), 10); len(lines) > 0 {
		out.Recent = lines
	} else {
		out.Recent = errorishLogLines(applog.Recent(500), 10)
	}
	return out
}

func errorishLogLines(lines []string, capN int) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < capN; i-- {
		if isErrorishLog(lines[i]) {
			out = append(out, lines[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func isErrorishLog(line string) bool {
	lower := strings.ToLower(line)
	for _, k := range []string{"error", "failed", "denied", "refused", "panic"} {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func formatDoctorCustomFields(cf doctorCustomFields) string {
	if cf.Mapped > 0 {
		word := "aliases"
		if cf.Mapped == 1 {
			word = "alias"
		}
		applied := ""
		if cf.AppliedAt != "" {
			applied = " (applied " + cf.AppliedAt + ")"
		}
		return fmt.Sprintf("%d %s mapped%s, usage rows %d", cf.Mapped, word, applied, cf.UsageRows)
	}
	if cf.rawScanned && cf.RawHasCustom {
		return "none mapped — raw carries customfield keys; run gadak fields --apply"
	}
	if cf.rawScanned {
		return "none mapped (none seen in raw)"
	}
	return "none mapped"
}

// formatDoctorMirrorVersion renders the live-update signal as the one line
// that answers "is my open board going to notice a write?" — the identity the
// ui-focus poll compares, plus how long ago it last moved. A version that is
// hours old right after a `gadak claim` says the mirror never got refreshed;
// a version that moves while the board sits still says the web half is where
// to look (GDK-1170).
func formatDoctorMirrorVersion(m doctorMirror) string {
	if m.ChangedAt == "" {
		return m.Version
	}
	at, err := time.Parse(time.RFC3339, m.ChangedAt)
	if err != nil {
		return m.Version + " (moved " + m.ChangedAt + ")"
	}
	return fmt.Sprintf("%s (moved %s ago, %s)", m.Version, formatDoctorAge(time.Since(at)), m.ChangedAt)
}

// formatDoctorAge is a coarse duration for a human reading one line. Sub-second
// reads as "0s" on purpose: what matters is "just now" vs "not since I wrote".
func formatDoctorAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours())/24)
}

func formatDoctorMirrorHolders(h doctorMirrorHolders) string {
	if h.Count == 0 {
		return "none"
	}
	parts := make([]string, 0, len(h.Processes))
	for _, p := range h.Processes {
		cmd := p.Command
		if cmd == "" {
			cmd = "?"
		}
		parts = append(parts, fmt.Sprintf("pid %d %s", p.PID, cmd))
	}
	if len(parts) == 0 {
		return strconv.Itoa(h.Count)
	}
	return fmt.Sprintf("%d (%s)", h.Count, strings.Join(parts, ", "))
}

// listMirrorHolders is best-effort: darwin and linux run lsof on the mirror
// and its WAL/SHM sidecars, drop this process, and return count+pid/command.
// A missing binary, timeout, or other failure returns nil so doctor omits
// the section rather than guessing.
func listMirrorHolders(mirrorPath string) *doctorMirrorHolders {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return nil
	}
	if mirrorPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{"-F", "pc", "--", mirrorPath, mirrorPath + "-wal", mirrorPath + "-shm"}
	cmd := exec.CommandContext(ctx, "lsof", args...)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return nil
		}
	}
	procs := parseLsofHolders(out, os.Getpid())
	return &doctorMirrorHolders{Count: len(procs), Processes: procs}
}

// parseLsofHolders reads `lsof -F pc` output: one process per pid, command
// name only, self excluded, no paths. Empty input is a successful empty scan.
func parseLsofHolders(out []byte, selfPID int) []doctorMirrorProcess {
	byPID := map[int]string{}
	var curPID int
	var curCmd string
	flush := func() {
		if curPID != 0 && curPID != selfPID && curCmd != "" {
			byPID[curPID] = curCmd
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			flush()
			n, convErr := strconv.Atoi(line[1:])
			if convErr != nil {
				curPID, curCmd = 0, ""
				continue
			}
			curPID, curCmd = n, ""
		case 'c':
			curCmd = line[1:]
		}
	}
	flush()
	pids := make([]int, 0, len(byPID))
	for pid := range byPID {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	procs := make([]doctorMirrorProcess, 0, len(pids))
	for _, pid := range pids {
		procs = append(procs, doctorMirrorProcess{PID: pid, Command: byPID[pid]})
	}
	return procs
}

func formatDoctorWorkspace(w doctorWorkspace) string {
	tok := "no"
	if w.HasSiteToken {
		tok = "yes"
	}
	persist := w.Persist
	if persist == "" {
		persist = "none"
	}
	frozen := "no"
	if w.Frozen {
		frozen = "yes"
	}
	s := fmt.Sprintf("kind=%s site_token=%s persist=%s issues=%d frozen=%s",
		w.Kind, tok, persist, w.LocalIssues, frozen)
	if w.Inconsistent {
		s += " inconsistent"
	}
	return s
}

// tildeHome shortens an absolute path under the user's home to ~/… so doctor
// output never embeds the account username. It is clitool.TildeHome — the one
// implementation, kept behind this name so doctor's many call sites stay short.
// GADAK_HOME may sit outside $HOME (tests, CI); such a path is returned cleaned
// but otherwise as-is, since it carries no home prefix to strip.
func tildeHome(path string) string {
	return clitool.TildeHome(path)
}

// redactSite never returns a hostname. Atlassian Cloud sites collapse to a
// fixed pattern; anything else is "configured (cloud)" or "none".
func redactSite(site string) string {
	site = strings.TrimSpace(site)
	if site == "" {
		return "none"
	}
	// Avoid net/url import of full URL when the value is a bare host, but still
	// never echo any host component back.
	lower := strings.ToLower(site)
	if strings.Contains(lower, "atlassian.net") {
		return "<redacted>.atlassian.net"
	}
	return "configured (cloud)"
}

// jiraStatusRe matches the store's usual last_error shape from jira.APIError:
// "GET /rest/…: jira: 403: …" or bare "jira: 429".
var jiraStatusRe = regexp.MustCompile(`(?i)jira:\s*(\d{3})\b`)

// httpStatusRe is a fallback for "status 403", "HTTP 429", bare 4xx/5xx.
var httpStatusRe = regexp.MustCompile(`(?i)(?:status|http)\s*(\d{3})\b`)

// bareStatusRe catches a leading or mid-string 4xx/5xx that is not a year.
var bareStatusRe = regexp.MustCompile(`\b([45]\d{2})\b`)

// classifyLastError turns a raw sync error into "http NNN (kind)" or a short
// kind with no message text. Raw strings often embed URLs, issue keys, and
// field names — none of that leaves this function.
func classifyLastError(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "none"
	}
	code := 0
	if m := jiraStatusRe.FindStringSubmatch(raw); len(m) == 2 {
		code, _ = strconv.Atoi(m[1])
	} else if m := httpStatusRe.FindStringSubmatch(raw); len(m) == 2 {
		code, _ = strconv.Atoi(m[1])
	} else if m := bareStatusRe.FindStringSubmatch(raw); len(m) == 2 {
		code, _ = strconv.Atoi(m[1])
	}
	kind := errorKind(code, raw)
	if code > 0 {
		return fmt.Sprintf("http %d (%s)", code, kind)
	}
	return kind
}

func errorKind(code int, raw string) string {
	switch code {
	case 401, 403:
		return "auth"
	case 429:
		return "throttled"
	case 404:
		return "not_found"
	case 400, 409, 422:
		return "client"
	}
	if code >= 500 && code <= 599 {
		return "server"
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "timeout"),
		strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "i/o timeout"):
		return "timeout"
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "network is unreachable"),
		strings.Contains(lower, "tls:"),
		strings.Contains(lower, "x509:"):
		return "network"
	case strings.Contains(lower, "credential rejected"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"):
		return "auth"
	case strings.Contains(lower, "throttl"),
		strings.Contains(lower, "rate limit"):
		return "throttled"
	default:
		return "error"
	}
}
