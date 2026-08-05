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
	FieldMap   map[string]string `json:"fieldMap,omitempty"`   // alias -> customfield_xxxxx
	BodyFields []string          `json:"bodyFields,omitempty"` // ADF custom-field ids to fold into FTS

	// Write allowlist: alias -> field id. Empty hides the inline edit UI entirely.
	EditableFields map[string]string `json:"editableFields,omitempty"`

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

// Dir is SCRY_HOME or ~/.scry, plus profiles/<name> when a profile is active.
func Dir() (string, error) {
	base := os.Getenv("SCRY_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".scry")
	}
	if p := Profile(); p != "" {
		return filepath.Join(base, "profiles", p), nil
	}
	return base, nil
}

// DBPath is the default SQLite path for the active profile.
func DBPath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "scry.db"), nil
}

// AttachmentDir is where attachment bytes are cached, next to the mirror it
// belongs to (so a profile keeps its own, and deleting a profile takes its cache
// with it).
func AttachmentDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "attachments"), nil
}

// Profiles lists the configured profile names, excluding the default one.
func Profiles() ([]string, error) {
	base := os.Getenv("SCRY_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(home, ".scry")
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

// Load returns an empty Config when the file does not exist; that is not an error.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &c, nil
}

// Save writes the file atomically with mode 0600.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
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
