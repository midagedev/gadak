// Package config loads and saves ~/.gadak/config.json.
//
// Credentials (email/token) share the file with the site settings, but they
// never reach the database, a log line, or a snapshot (constitution article 8).
// The file is written 0600.
package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Member is one entry of the static member directory injected through settings.
// It merges into bootstrap's members[], which is what gives an avatar its ring,
// tooltip, and team preset.
type Member struct {
	Email         string `json:"email"`
	Name          string `json:"name,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	Group         string `json:"group,omitempty"`
	Department    string `json:"department,omitempty"`
	JobRole       string `json:"job_role,omitempty"`
	JiraAccountID string `json:"jira_account_id,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
}

// GroupRule classifies an issue into a group. Rules are read top-down and the
// first match wins. Conditions AND together; the list inside one condition ORs.
// An empty condition is always true.
type GroupRule struct {
	Group      string   `json:"group"`
	Projects   []string `json:"projects,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Components []string `json:"components,omitempty"`
}

// Product is the product bucket a group maps to.
type Product struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Config struct {
	// The credential and what it connects to. Token is never copied out of this file.
	Site     string   `json:"site,omitempty"` // https://your-site.atlassian.net
	Email    string   `json:"email,omitempty"`
	Token    string   `json:"token,omitempty"`
	Projects []string `json:"projects,omitempty"`

	// Result of verifying the credential: when `PUT credential/` last confirmed it
	// against /myself, and who owns it. Unlike the token itself, both may be
	// returned in a response.
	TokenVerifiedAt string `json:"tokenVerifiedAt,omitempty"`
	TokenOwner      string `json:"tokenOwner,omitempty"`
	// AccountID is the Jira accountId returned by /myself. Used for feed
	// relevance (assignee/reporter/mention) and self-action filtering. Empty
	// when the credential was never verified against a live site.
	AccountID string `json:"account_id,omitempty"`

	// Sync field mapping (contracts/sync.md, "Field mapping").
	// Fields is the sole truth when present. FieldMap/EditableFields remain for
	// legacy configs; FieldSpecs() synthesizes them into one shape.
	Fields         []FieldSpec       `json:"fields,omitempty"`
	FieldMap       map[string]string `json:"fieldMap,omitempty"`       // alias -> customfield_xxxxx (legacy)
	BodyFields     []string          `json:"bodyFields,omitempty"`     // ADF custom-field ids to fold into FTS
	EditableFields map[string]string `json:"editableFields,omitempty"` // alias -> field id (legacy write allowlist)

	// Optional surfaces carried over from the tool this was extracted from.
	Members        []Member           `json:"members,omitempty"`
	GroupRules     []GroupRule        `json:"groupRules,omitempty"`
	GroupLabels    map[string]string  `json:"groupLabels,omitempty"`
	GroupColors    map[string]string  `json:"groupColors,omitempty"`
	ProductByGroup map[string]Product `json:"productByGroup,omitempty"`
	Features       map[string]bool    `json:"features,omitempty"` // feed/push/deploy/qa/teamGroups
	QaDashboardURL string             `json:"qaDashboardUrl,omitempty"`

	StaleThresholdHours int `json:"staleThresholdHours,omitempty"` // 0 = the client default (72)
	// AttachmentCacheMB caps the on-disk attachment byte cache. 0 = package
	// default (512 MB); a negative value is treated as 0.
	AttachmentCacheMB int `json:"attachmentCacheMB,omitempty"`

	// Sync periods in seconds. 0 means use DefaultSyncIntervalSec /
	// DefaultReconcileIntervalSec. serve's Watch loop reads these once at start,
	// so a change only takes effect after the process restarts.
	SyncIntervalSec      int `json:"syncIntervalSec,omitempty"`
	ReconcileIntervalSec int `json:"reconcileIntervalSec,omitempty"`

	// Notify enables OS desktop notifications from the sync watch loop after
	// new personal-feed events. Default true when absent; set false to opt out.
	// Pointer so omitempty can distinguish "unset" from explicit false.
	Notify *bool `json:"notify,omitempty"`

	// UpdateCheck enables the once-per-day GitHub release lookup that surfaces
	// a newer version on sync/status/serve bootstrap. Default true when absent;
	// set false to opt out (restores the prior "outbound is only Jira" model).
	UpdateCheck *bool `json:"updateCheck,omitempty"`

	// Confluence, when non-nil, enables the wiki-page mirror (second source).
	// Spaces empty means every *global* space — not every space the account can
	// see, which is what this comment used to claim and what a warning written
	// from it went on to tell users. Cloud gives each person a personal space,
	// so an unfiltered listing is mostly noise; personal spaces are mirrored
	// only when named here. The rule itself lives in internal/sync/confluence.go.
	Confluence *ConfluenceConfig `json:"confluence,omitempty"`

	// dir is the profile directory this Config was loaded from (or will save to).
	// Unexported so it never appears in JSON; set by LoadFor.
	dir string
}

// ConfluenceConfig is the optional wiki-page source. Presence (non-nil) is the
// on switch; same site/email/token as Jira, REST base under /wiki.
type ConfluenceConfig struct {
	Spaces []string `json:"spaces,omitempty"`
}

// FieldSpec is one logical custom field. Jira creates a separate field id per
// board template for the same concept, so one spec can carry several ids; the
// sync coalesces the first filled value (measured fact: 57 of 353 custom field
// names on one large site map to 2+ ids).
type FieldSpec struct {
	Alias string   `json:"alias"`          // stable key: ascii slug of the name, else cf_<id>
	Label string   `json:"label"`          // Jira display name, in the account's language
	IDs   []string `json:"ids"`            // all field ids sharing the name, most-filled first
	Role  string   `json:"role"`           // body | facet | user | plain
	Kind  string   `json:"kind,omitempty"` // editor: option | multi_option | user | version_array | ""
	Auto  bool     `json:"auto,omitempty"` // discovery-owned; regenerated on re-apply
}

// Sync loop defaults and floors. Zero in the file means "use default".
// Floors reject values that would thrash Jira or busy-loop the local process.
const (
	DefaultSyncIntervalSec      = 60   // 1 minute
	DefaultReconcileIntervalSec = 3600 // 1 hour
	MinSyncIntervalSec          = 15   // seconds
	MinReconcileIntervalSec     = 300  // 5 minutes
)

// EffectiveSyncIntervalSec returns the interval Watch should use.
func (c *Config) EffectiveSyncIntervalSec() int {
	if c == nil || c.SyncIntervalSec <= 0 {
		return DefaultSyncIntervalSec
	}
	return c.SyncIntervalSec
}

// EffectiveReconcileIntervalSec returns the reconcile interval Watch should use.
func (c *Config) EffectiveReconcileIntervalSec() int {
	if c == nil || c.ReconcileIntervalSec <= 0 {
		return DefaultReconcileIntervalSec
	}
	return c.ReconcileIntervalSec
}

// profile comes from SetProfile (the --profile flag) or GADAK_PROFILE
// (SCRY_PROFILE still works). Empty or "default" means the root profile
// (~/.gadak); anything else lives under ~/.gadak/profiles/<name>. Each profile
// gets its own config.json and gadak.db, so two sites can be used side by side
// without their credentials or mirrors colliding.
var profile = Env("PROFILE")

// SetProfile is called by the CLI's --profile flag, which wins over the env var.
func SetProfile(name string) { profile = name }

// Profile returns the active profile name ("" for the default one).
func Profile() string {
	if profile == "default" {
		return ""
	}
	return profile
}

// homeRoot is GADAK_HOME, else SCRY_HOME, else ~/.gadak. An existing ~/.scry
// directory is renamed to ~/.gadak on first use so a pre-rename install keeps
// its mirror. Shared by DirFor and Profiles.
func homeRoot() (string, error) {
	if base := Env("HOME"); base != "" {
		return base, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	next := filepath.Join(home, DirName)
	prev := filepath.Join(home, LegacyDirName)
	if err := migratePath(prev, next); err != nil {
		if _, statErr := os.Stat(next); statErr != nil {
			if _, oldErr := os.Stat(prev); oldErr == nil {
				return prev, nil
			}
		}
	}
	warnIfDualHome(prev, next)
	return next, nil
}

// dualHomeWarnOnce keeps the leftover-home warning to one line per process
// (homeRoot is called several times per command).
var dualHomeWarnOnce sync.Once

// warnIfDualHome is the single owner of the D1 surface: when both the
// legacy ~/.scry tree and ~/.gadak exist, migratePath is a no-op and the
// old mirror is silently abandoned. We do not merge (data-loss risk);
// we name both paths on stderr so the leftover is visible.
func warnIfDualHome(prev, next string) {
	if prev == "" || next == "" || prev == next {
		return
	}
	if _, err := os.Stat(prev); err != nil {
		return
	}
	if _, err := os.Stat(next); err != nil {
		return
	}
	dualHomeWarnOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "gadak: leftover data at %s is being ignored; using %s\n", prev, next)
	})
}

// migratePath renames oldPath → newPath when newPath is absent and oldPath
// exists. No-op if they are the same, if the destination already exists, or if
// the source is missing.
func migratePath(oldPath, newPath string) error {
	if oldPath == "" || newPath == "" || oldPath == newPath {
		return nil
	}
	if _, err := os.Stat(newPath); err == nil {
		return nil
	}
	if _, err := os.Stat(oldPath); err != nil {
		return nil
	}
	return os.Rename(oldPath, newPath)
}

func migrateDB(oldPath, newPath string) {
	_ = migratePath(oldPath, newPath)
	_ = migratePath(oldPath+"-wal", newPath+"-wal")
	_ = migratePath(oldPath+"-shm", newPath+"-shm")
}

// maxProfileNameLen is the longest allowed named-profile directory name.
const maxProfileNameLen = 64

// profileNameRe is the only allowed shape for a named profile. Empty and
// "default" are handled separately (they mean the root, not a directory name).
var profileNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// validProfileName is the single owner of profile-name checks. Every path
// that turns a name into a directory goes through DirFor, which calls this.
func validProfileName(name string) error {
	if name == "" || name == "default" {
		return nil
	}
	switch {
	case name == "." || name == "..":
		return fmt.Errorf("invalid profile name %q: cannot be %q", name, name)
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("invalid profile name %q: contains a null byte", name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("invalid profile name %q: contains a path separator", name)
	case len(name) > maxProfileNameLen:
		return fmt.Errorf("invalid profile name %q: longer than %d characters", name, maxProfileNameLen)
	case !profileNameRe.MatchString(name):
		return fmt.Errorf("invalid profile name %q: must start with a letter or digit and contain only letters, digits, '.', '_' or '-'", name)
	}
	return nil
}

// DirFor is the config directory for a named profile. "" or "default" means the
// root (GADAK_HOME / ~/.gadak); any other name lives under profiles/<name>.
// Names that fail validProfileName return an error and no path.
func DirFor(profile string) (string, error) {
	if err := validProfileName(profile); err != nil {
		return "", err
	}
	base, err := homeRoot()
	if err != nil {
		return "", err
	}
	if profile == "" || profile == "default" {
		return base, nil
	}
	return filepath.Join(base, "profiles", profile), nil
}

// Dir is GADAK_HOME or ~/.gadak, plus profiles/<name> when a profile is active.
func Dir() (string, error) {
	return DirFor(Profile())
}

// DBPathFor is the SQLite path for the named profile.
func DBPathFor(profile string) (string, error) {
	d, err := DirFor(profile)
	if err != nil {
		return "", err
	}
	next := filepath.Join(d, DBFile)
	migrateDB(filepath.Join(d, LegacyDBFile), next)
	return next, nil
}

// DBPath is the default SQLite path for the active profile.
func DBPath() (string, error) {
	return DBPathFor(Profile())
}

// AttachmentDirFor is where attachment bytes are cached for the named profile.
func AttachmentDirFor(profile string) (string, error) {
	d, err := DirFor(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "attachments"), nil
}

// AttachmentDir is where attachment bytes are cached, next to the mirror it
// belongs to (so a profile keeps its own, and deleting a profile takes its cache
// with it).
func AttachmentDir() (string, error) {
	return AttachmentDirFor(Profile())
}

// RequireExistingProfile is the single owner of "may this named profile be
// used without creating it?". The default profile (empty / "default") is
// always allowed so first-run can mint ~/.gadak. A named profile whose
// directory does not exist is an error; names that do exist are listed so
// a typo is obvious.
func RequireExistingProfile() error {
	name := Profile()
	if name == "" {
		return nil
	}
	d, err := DirFor(name)
	if err != nil {
		return err
	}
	fi, err := os.Stat(d)
	if err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("profile %q is not a directory (%s)", name, d)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	names, listErr := Profiles()
	if listErr != nil {
		names = nil
	}
	sort.Strings(names)
	msg := fmt.Sprintf("profile %q not found; run gadak init --profile %q", name, name)
	if len(names) > 0 {
		msg += fmt.Sprintf(" (available: %s)", strings.Join(names, ", "))
	}
	return fmt.Errorf("%s", msg)
}

// Profiles lists the configured profile names, excluding the default one.
func Profiles() ([]string, error) {
	base, err := homeRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(base, "profiles"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.json"), nil
}

// LoadFor reads config.json for the named profile. Missing file returns an empty
// Config with dir set (not an error), matching Load's convention.
func LoadFor(profile string) (*Config, error) {
	d, err := DirFor(profile)
	if err != nil {
		return nil, err
	}
	p := filepath.Join(d, "config.json")
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{dir: d}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	c.dir = d
	return &c, nil
}

// Load returns an empty Config when the file does not exist; that is not an error.
func Load() (*Config, error) {
	return LoadFor(Profile())
}

// Save writes the file atomically with mode 0600. When c.dir is set (LoadFor),
// the write goes to that profile's config.json; otherwise the active Path().
// The profile directory (and ~/.gadak / GADAK_HOME when writing the default
// profile) is created and tightened to 0700; chmod failures are logged only.
func (c *Config) Save() error {
	var p string
	if c != nil && c.dir != "" {
		p = filepath.Join(c.dir, "config.json")
	} else {
		var err error
		p, err = Path()
		if err != nil {
			return err
		}
	}
	if err := ensurePrivateDir(filepath.Dir(p)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ensurePrivateDir creates dir at 0700 and chmods an existing one to 0700 so
// older installs left at 0755 are quietly tightened (migration of H-2).
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		log.Printf("config: chmod %s: %v", dir, err)
	}
	return nil
}

// HasCredential reports whether writes and the attachment proxy are possible.
func (c *Config) HasCredential() bool {
	return c.Site != "" && c.Email != "" && c.Token != ""
}

// NotifyEnabled is true unless the user set notify: false. Absent means on.
func (c *Config) NotifyEnabled() bool {
	if c == nil || c.Notify == nil {
		return true
	}
	return *c.Notify
}

// UpdateCheckEnabled is true unless the user set updateCheck: false. Absent means on.
func (c *Config) UpdateCheckEnabled() bool {
	if c == nil || c.UpdateCheck == nil {
		return true
	}
	return *c.UpdateCheck
}

// FieldSpecs returns the effective field specs. Legacy configs that predate
// Fields carry FieldMap/EditableFields instead; synthesize specs from them so
// every consumer reads one shape. Fields, when present, is the sole truth.
func (c *Config) FieldSpecs() []FieldSpec {
	if c == nil {
		return nil
	}
	if len(c.Fields) > 0 {
		return c.Fields
	}
	if len(c.FieldMap) == 0 {
		return nil
	}
	out := make([]FieldSpec, 0, len(c.FieldMap))
	// Stable order for tests and logs.
	aliases := make([]string, 0, len(c.FieldMap))
	for a := range c.FieldMap {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		id := c.FieldMap[alias]
		if id == "" {
			continue
		}
		out = append(out, FieldSpec{
			Alias: alias,
			Label: alias,
			IDs:   []string{id},
			Role:  "facet",
			Auto:  false,
		})
	}
	return out
}
