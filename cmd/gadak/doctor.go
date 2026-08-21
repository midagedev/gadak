package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/originbind"
	"github.com/midagedev/gadak/internal/store"
)

// doctorBanner is the first line of every doctor dump so a user pasting into a
// public issue can see what was stripped before they hit submit.
const doctorBanner = "# gadak doctor — safe to paste: counts and versions only, no keys, names, URLs or tokens"

// doctorReport is the redacted diagnostic document. Field names are stable for
// --json consumers; values never carry tokens, hostnames, emails, project
// keys, custom-field names, or raw error strings.
type doctorReport struct {
	GadakVersion    string                `json:"gadak_version"`
	GoVersion       string                `json:"go_version"`
	OS              string                `json:"os"`
	Arch            string                `json:"arch"`
	Profile         string                `json:"profile"`
	WorkspaceSource string                `json:"workspace_source"`
	WorkspaceKind   string                `json:"workspace_kind"`
	Origin          string                `json:"origin"`
	OriginOwner     string                `json:"origin_owner,omitempty"`
	MirrorPath      string                `json:"mirror_path"`
	Mirror          doctorMirror          `json:"mirror"`
	SchemaVersion   *int                  `json:"schema_version"`
	SchemaSinceSync string                `json:"schema_since_sync,omitempty"`
	Migrations      string                `json:"migrations"`
	Counts          *doctorCounts         `json:"counts"`
	Confluence      string                `json:"confluence"`
	CustomFields    doctorCustomFields    `json:"custom_fields"`
	Credential      string                `json:"credential"`
	Site            string                `json:"site"`
	Email           string                `json:"email"`
	Skill           doctorSkill           `json:"skill"`
	MCP             doctorMCP             `json:"mcp"`
	Sync            map[string]doctorSync `json:"sync"`
	APIUsage        doctorAPIUsage        `json:"api_usage"`
	Workspace       doctorWorkspace       `json:"workspace"`
}

// doctorWorkspace is the one-line consistency view: kind, whether a site
// token is stored (never the token itself), the origin persist path, and
// how many locally originated issues LocalData counts. Inconsistent is
// standalone-with-a-token — the state GDK-247 closes on the write path.
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

type doctorMirror struct {
	Status string `json:"status"`           // present | not_found | open_error | schema_too_new
	Bytes  *int64 `json:"bytes,omitempty"`  // set when the file exists
	Detail string `json:"detail,omitempty"` // redacted path / short reason
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
	rep.MCP = collectMCPStatus()

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
	if has, err := db.HasCustomFieldKeysInRaw(ctx); err == nil {
		rep.CustomFields.RawHasCustom = has
		rep.CustomFields.rawScanned = true
	}

	return rep
}

// collectSkillStatus reuses the installer's own classifier (skillDestStatus in
// skill.go), so `gadak doctor` and `gadak skill install --print` can never
// disagree about the same file.
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

	if r.SchemaVersion != nil {
		line("schema_version", strconv.Itoa(*r.SchemaVersion))
	} else {
		line("schema_version", "unknown")
	}
	if r.SchemaSinceSync != "" {
		line("schema_since_sync", r.SchemaSinceSync)
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

	return b.String()
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
