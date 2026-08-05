// Package config loads and saves ~/.scry/config.json.
//
// Credentials (email/token) share the file with the site settings, but they
// never reach the database, a log line, or a snapshot (constitution article 8).
// The file is written 0600.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	// dir is the profile directory this Config was loaded from (or will save to).
	// Unexported so it never appears in JSON; set by LoadFor.
	dir string
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

// profile comes from SetProfile (the --profile flag) or SCRY_PROFILE. Empty or
// "default" means the root profile (~/.scry); anything else lives under
// ~/.scry/profiles/<name>. Each profile gets its own config.json and scry.db, so
// two sites can be used side by side without their credentials or mirrors
// colliding.
var profile = os.Getenv("SCRY_PROFILE")

// SetProfile is called by the CLI's --profile flag, which wins over the env var.
func SetProfile(name string) { profile = name }

// Profile returns the active profile name ("" for the default one).
func Profile() string {
	if profile == "default" {
		return ""
	}
	return profile
}

// scryHome is SCRY_HOME or ~/.scry (the root that holds the default profile and
// the profiles/ subdirectory). Shared by DirFor and Profiles.
func scryHome() (string, error) {
	base := os.Getenv("SCRY_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".scry")
	}
	return base, nil
}

// DirFor is the config directory for a named profile. "" or "default" means the
// root (SCRY_HOME / ~/.scry); any other name lives under profiles/<name>.
func DirFor(profile string) (string, error) {
	base, err := scryHome()
	if err != nil {
		return "", err
	}
	if profile == "" || profile == "default" {
		return base, nil
	}
	return filepath.Join(base, "profiles", profile), nil
}

// Dir is SCRY_HOME or ~/.scry, plus profiles/<name> when a profile is active.
func Dir() (string, error) {
	return DirFor(Profile())
}

// DBPathFor is the SQLite path for the named profile.
func DBPathFor(profile string) (string, error) {
	d, err := DirFor(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "scry.db"), nil
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

// Profiles lists the configured profile names, excluding the default one.
func Profiles() ([]string, error) {
	base, err := scryHome()
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
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
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
