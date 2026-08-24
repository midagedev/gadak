package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config/tokencheck"
)

// FeatureNames is every optional-surface flag PUT / gadak config accept.
// Unknown keys in a features map are dropped.
var FeatureNames = []string{"feed", "deploy", "qa", "teamGroups"}

// themeIdentRe is the only shape a theme id may have. Palette names belong to
// the web; the server checks form so a new palette does not need a server
// deploy. system / light / dark always match.
var themeIdentRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// issueTypeIDRe is a Jira Cloud issue type id (digits). Display names are
// rejected so a Korean account cannot store "Task" and silently miss "작업".
var issueTypeIDRe = regexp.MustCompile(`^[0-9]+$`)

// ValidateDefaultIssueTypeID accepts empty (unset) or a Jira issue type id.
func ValidateDefaultIssueTypeID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !issueTypeIDRe.MatchString(s) {
		return "", fmt.Errorf("defaultIssueTypeId must be a Jira issue type id (digits), not a display name (got %q)", s)
	}
	return s, nil
}

// ValidateDefaultIssueType stores an optional display label. Resolution
// never reads this value.
func ValidateDefaultIssueType(s string) (string, error) {
	return strings.TrimSpace(s), nil
}

// ValidateDefaultProject accepts empty (unset) or a project key with no
// whitespace. Membership in Projects is not checked here — that list can
// be empty ("every project") and can change after the default is set.
func ValidateDefaultProject(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.ContainsAny(s, " \t\n\r") {
		return "", fmt.Errorf("defaultProject must be a project key (got %q)", s)
	}
	return s, nil
}

// projectKeyRe is the Jira Cloud project-key shape: 2–10 uppercase
// alphanumeric characters, starting with a letter. IssueKeyLiteral uses the
// same prefix for ABC-123; the length cap is so a multi-kilobyte all-caps
// string is not stored as a "key" (GDK-809).
var projectKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

// LooksLikeProjectKey reports that shape without case-folding. "Fix" (the
// create --help example) is a summary word; "FIX" / "GDK" look like keys.
func LooksLikeProjectKey(s string) bool {
	return projectKeyRe.MatchString(s)
}

// ValidateProjectKeys is the single choke for the `projects` setting (GDK-809).
// Empty means "every project this account can see". Entries are trimmed and
// upper-cased (same as gadak init). Mixed-case values are refused as display
// names. Load does not call this — a file that already contains an unknown
// key keeps working until the next Set.
func ValidateProjectKeys(keys []string) ([]string, error) {
	if keys == nil {
		return []string{}, nil
	}
	out := make([]string, 0, len(keys))
	seen := map[string]bool{}
	var bad []string
	for _, raw := range keys {
		if strings.IndexFunc(raw, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			bad = append(bad, fmt.Sprintf("%q", raw))
			continue
		}
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			bad = append(bad, `""`)
			continue
		}
		upper, lower := strings.ToUpper(trimmed), strings.ToLower(trimmed)
		if trimmed != upper && trimmed != lower {
			bad = append(bad, fmt.Sprintf("%q", raw))
			continue
		}
		if !LooksLikeProjectKey(upper) {
			bad = append(bad, fmt.Sprintf("%q", raw))
			continue
		}
		if seen[upper] {
			continue
		}
		seen[upper] = true
		out = append(out, upper)
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("projects: %s is not a Jira project key (want 2–10 chars A–Z then A–Z0–9, e.g. NMB); empty list mirrors every project", strings.Join(bad, ", "))
	}
	return out, nil
}

// ValidateTheme accepts empty/"system" (stored as "") and any lowercase
// identifier. "system", "light", and "dark" are always valid.
func ValidateTheme(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "system" {
		return "", nil
	}
	if !themeIdentRe.MatchString(s) {
		return "", fmt.Errorf("appearance.theme must be a lowercase identifier [a-z0-9-]{1,32} (got %q)", s)
	}
	return s, nil
}

// localeValues are the locales the issuetap origin serves (its locale
// package). Anything else is rejected loudly rather than silently falling
// back to English.
var localeValues = map[string]bool{"": true, "en": true, "ko": true, "ja": true, "de": true}

// ValidateLocale accepts "", en, ko, ja, de (GDK-597). Empty stores as the
// zero value so the default is not persisted.
func ValidateLocale(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !localeValues[s] {
		return "", fmt.Errorf("locale must be one of en, ko, ja, de (or empty for English), not %q", s)
	}
	return s, nil
}

// ApplyAppearance writes a validated theme onto c. "system" and empty store
// as the zero value so the default is not persisted.
func ApplyAppearance(c *Config, a Appearance) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	theme, err := ValidateTheme(a.Theme)
	if err != nil {
		return err
	}
	if theme == "" {
		c.Appearance = nil
		return nil
	}
	c.Appearance = &Appearance{Theme: theme}
	return nil
}

// ValidateIntervals is the PUT / gadak config rule for the two watch periods.
// 0 means "use the package default". A positive value below the floor is
// rejected so a typo cannot thrash Jira.
func ValidateIntervals(syncSec, reconcileSec int) error {
	if err := validateInterval("syncIntervalSec", syncSec, MinSyncIntervalSec); err != nil {
		return err
	}
	return validateInterval("reconcileIntervalSec", reconcileSec, MinReconcileIntervalSec)
}

func validateInterval(name string, sec, min int) error {
	if sec < 0 {
		return fmt.Errorf("%s must be 0 (default) or a positive number of seconds", name)
	}
	if sec > 0 && sec < min {
		return fmt.Errorf("%s must be 0 (default) or >= %d (got %d)", name, min, sec)
	}
	return nil
}

// NormalizeFeatures projects the optional-surface flags. An explicit key wins;
// missing keys stay off except feed, which defaults on.
func NormalizeFeatures(set map[string]bool) map[string]bool {
	out := make(map[string]bool, len(FeatureNames))
	for _, name := range FeatureNames {
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

// ApplyConfluence is the PUT settings/ confluence rule, shared with
// `gadak config set confluence*`. enabled:false turns the source off;
// enabled:true creates/replaces the block; spaces alone requires it on.
func ApplyConfluence(c *Config, enabled *bool, spaces []string) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	if spaces == nil {
		spaces = []string{}
	}
	switch {
	case enabled != nil && !*enabled:
		c.Confluence = nil
		return nil
	case enabled != nil && *enabled:
		c.Confluence = &ConfluenceConfig{Spaces: spaces}
		return nil
	default:
		if c.Confluence == nil {
			return fmt.Errorf("confluence_not_configured")
		}
		cc := *c.Confluence
		cc.Spaces = spaces
		c.Confluence = &cc
		return nil
	}
}

// Setting is one dotted path `gadak config` may get or set. Root is the
// matching PUT /api/settings JSON key so a coverage test can keep the two
// surfaces on one schema.
type Setting struct {
	Path        string
	Root        string
	Description string
	Get         func(*Config) any
	Set         func(*Config, json.RawMessage) error
}

// Settings is the editable-path catalog. Credentials are not on it.
func Settings() []Setting {
	return buildSettings()
}

// SettingByPath looks up one catalog entry.
func SettingByPath(path string) (Setting, bool) {
	for _, s := range buildSettings() {
		if s.Path == path {
			return s, true
		}
	}
	return Setting{}, false
}

// SettingPaths returns catalog paths in catalog order.
func SettingPaths() []string {
	all := buildSettings()
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.Path
	}
	return out
}

func buildSettings() []Setting {
	out := []Setting{
		{
			Path:        "appearance",
			Root:        "appearance",
			Description: "UI look block; empty theme means system",
			Get:         func(c *Config) any { return Appearance{Theme: c.EffectiveTheme()} },
			Set: func(c *Config, raw json.RawMessage) error {
				var a Appearance
				if err := json.Unmarshal(raw, &a); err != nil {
					return fmt.Errorf("appearance must be an object {\"theme\": \"…\"}")
				}
				return ApplyAppearance(c, a)
			},
		},
		{
			Path:        "appearance.theme",
			Root:        "appearance",
			Description: "UI theme: system, light, dark, or a lowercase palette id",
			Get:         func(c *Config) any { return c.EffectiveTheme() },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "appearance.theme")
				if err != nil {
					return err
				}
				return ApplyAppearance(c, Appearance{Theme: s})
			},
		},
		{
			Path: "ui.tokens",
			Root: "ui",
			Description: "color-token overrides for every palette: {\"accent\": \"#7a4bd0\", …} " +
				"(locked tokens refused; validated tokens must pass the contrast rules; " +
				"discover names with `gadak config get ui.tokens.catalog`)",
			Get: func(c *Config) any {
				if c.UI == nil || c.UI.Tokens == nil {
					return UITokens{}
				}
				return *c.UI.Tokens
			},
			Set: func(c *Config, raw json.RawMessage) error {
				tokens, err := parseUITokens("ui.tokens", raw)
				if err != nil {
					return err
				}
				next := cloneUIConfig(c.UI)
				next.Tokens = tokens
				return ApplyUIConfig(c, next)
			},
		},
		{
			Path: "ui.tokensByTheme",
			Root: "ui",
			Description: "per-palette token overlays: {\"dark\": {\"accent\": \"#9a6be0\"}} — " +
				"each named palette is validated against that palette's rules",
			Get: func(c *Config) any {
				out := map[string]UITokens{}
				if c.UI != nil {
					for p, t := range c.UI.TokensByTheme {
						if t != nil {
							out[p] = *t
						}
					}
				}
				return out
			},
			Set: func(c *Config, raw json.RawMessage) error {
				var in map[string]json.RawMessage
				if err := json.Unmarshal(raw, &in); err != nil {
					return fmt.Errorf("ui.tokensByTheme must be an object of palette→token map")
				}
				byTheme := map[string]*UITokens{}
				palettes := make([]string, 0, len(in))
				for p := range in {
					palettes = append(palettes, p)
				}
				sort.Strings(palettes)
				for _, p := range palettes {
					t, err := parseUITokens("ui.tokensByTheme."+p, in[p])
					if err != nil {
						return err
					}
					byTheme[p] = t
				}
				next := cloneUIConfig(c.UI)
				next.TokensByTheme = byTheme
				return ApplyUIConfig(c, next)
			},
		},
		{
			Path: "ui.dataColors",
			Root: "ui",
			Description: "per-data inks: {\"label\": {\"urgent\": \"#c03030\"}, \"type\": {\"10007\": \"#d07020\"}, " +
				"\"status\": {\"inprogress\": \"#7e5904\"}} — type keys are issue type ids, status keys are " +
				"status categories (new|inprogress|done); display names are refused",
			Get: func(c *Config) any {
				out := map[string]map[string]string{}
				if c.UI != nil {
					for _, family := range uiDataFamilies {
						if len(c.UI.DataColors[family]) > 0 {
							out[family] = c.UI.DataColors[family]
						}
					}
				}
				return out
			},
			Set: func(c *Config, raw json.RawMessage) error {
				var in map[string]map[string]string
				if err := json.Unmarshal(raw, &in); err != nil {
					return fmt.Errorf("ui.dataColors must be an object of family→key→hex")
				}
				next := cloneUIConfig(c.UI)
				next.DataColors = in
				return ApplyUIConfig(c, next)
			},
		},
		{
			Path: "ui.tokens.catalog",
			Root: "ui",
			Description: "read-only color-token catalog (name, cssVar, tier, rules, per-palette values) — " +
				"the discovery path for ui.tokens keys",
			Get: func(c *Config) any { return tokencheck.CatalogTokens() },
			Set: func(*Config, json.RawMessage) error {
				return fmt.Errorf("ui.tokens.catalog is read-only — it ships with the binary; set ui.tokens instead")
			},
		},
		intSetting("syncIntervalSec", "syncIntervalSec",
			"incremental sync period in seconds (0 = default 60; min 15 when set)",
			func(c *Config) int { return c.SyncIntervalSec },
			func(c *Config, n int) error {
				if err := ValidateIntervals(n, 0); err != nil {
					return err
				}
				if n < 0 {
					n = 0
				}
				c.SyncIntervalSec = n
				return nil
			},
		),
		intSetting("reconcileIntervalSec", "reconcileIntervalSec",
			"deletion-reconcile period in seconds (0 = default 3600; min 300 when set)",
			func(c *Config) int { return c.ReconcileIntervalSec },
			func(c *Config, n int) error {
				if err := ValidateIntervals(0, n); err != nil {
					return err
				}
				if n < 0 {
					n = 0
				}
				c.ReconcileIntervalSec = n
				return nil
			},
		),
		intSetting("staleThresholdHours", "staleThresholdHours",
			"hours in the current status before an unresolved issue counts as stale (0 = client default 72)",
			func(c *Config) int { return c.StaleThresholdHours },
			func(c *Config, n int) error {
				if n < 0 {
					n = 0
				}
				c.StaleThresholdHours = n
				return nil
			},
		),
		boolDefaultTrue("notify", "notify",
			"OS desktop notifications from the watch loop (default true)",
			func(c *Config) bool { return c.NotifyEnabled() },
			func(c *Config, b bool) {
				if b {
					c.Notify = nil
					return
				}
				f := false
				c.Notify = &f
			},
		),
		boolDefaultTrue("updateCheck", "updateCheck",
			"once-per-day GitHub release lookup (default true)",
			func(c *Config) bool { return c.UpdateCheckEnabled() },
			func(c *Config, b bool) {
				if b {
					c.UpdateCheck = nil
					return
				}
				f := false
				c.UpdateCheck = &f
			},
		),
		{
			Path: "devStatus",
			Root: "devStatus",
			Description: "mirror Jira's internal development-status (dev-status) API into dev_links; " +
				"adds a per-issue request to each sync (default false)",
			Get: func(c *Config) any {
				return c != nil && c.DevStatus
			},
			Set: func(c *Config, raw json.RawMessage) error {
				b, err := decodeBool(raw, "devStatus")
				if err != nil {
					return err
				}
				c.DevStatus = b
				return nil
			},
		},
		{
			Path: "actor",
			Root: "actor",
			Description: "workspace-default acting identity for writes to a standalone/paired origin " +
				"(GDK-586): {\"slug\", \"name\"}; env GADAK_ACTOR wins over it and Claude Code is " +
				"auto-detected when neither is set (never sent to a connected Cloud site)",
			Get: func(c *Config) any {
				if c == nil || c.Actor == nil {
					return ActorConfig{}
				}
				return *c.Actor
			},
			Set: func(c *Config, raw json.RawMessage) error {
				// Object form {"slug","name"}; the "slug|name" shorthand
				// (the GADAK_ACTOR shape) parses through the same owner.
				var in ActorConfig
				if err := json.Unmarshal(raw, &in); err != nil {
					s, serr := decodeString(raw, "actor")
					if serr != nil {
						return fmt.Errorf("actor must be an object {\"slug\": \"claude:354bff2b\", \"name\": \"Claude Code\"} or the shorthand string \"slug|name\"")
					}
					in.Slug, in.Name = ParseActorShorthand(s)
				}
				v, err := ValidateActor(in.Slug, in.Name)
				if err != nil {
					return err
				}
				c.Actor = v
				return nil
			},
		},
		{
			Path: "locale",
			Root: "locale",
			Description: "display-name language of a standalone workspace's origin: \"\", en, ko, ja, de " +
				"(empty = English). Status / issue-type / field names and agent aliases follow it; " +
				"priority names stay English, like a live Cloud site (GDK-597). Changing it rebuilds " +
				"the mirror on the next sync — display names are cached. A connected workspace " +
				"ignores it: its language is the Atlassian account's",
			Get: func(c *Config) any { return c.Locale },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "locale")
				if err != nil {
					return err
				}
				v, err := ValidateLocale(s)
				if err != nil {
					return err
				}
				c.Locale = v
				return nil
			},
		},
		{
			Path: "frozen",
			Root: "frozen",
			Description: "freeze this workspace: no request leaves for the origin — pulls and " +
				"writes alike (demo / scrubbed-fixture latch, GDK-181/GDK-507); mirror reads still work",
			Get: func(c *Config) any {
				return c.SyncFrozen()
			},
			Set: func(c *Config, raw json.RawMessage) error {
				b, err := decodeBool(raw, "frozen")
				if err != nil {
					return err
				}
				c.Frozen = b
				return nil
			},
		},
		intSetting("attachmentCacheMB", "attachmentCacheMB",
			"on-disk attachment cache cap in megabytes (0 = package default 512)",
			func(c *Config) int { return c.AttachmentCacheMB },
			func(c *Config, n int) error {
				if n < 0 {
					n = 0
				}
				c.AttachmentCacheMB = n
				return nil
			},
		),
		{
			Path:        "features",
			Root:        "features",
			Description: "optional-surface flags (feed, deploy, qa, teamGroups)",
			Get:         func(c *Config) any { return NormalizeFeatures(c.Features) },
			Set: func(c *Config, raw json.RawMessage) error {
				var m map[string]bool
				if err := json.Unmarshal(raw, &m); err != nil {
					return fmt.Errorf("features must be an object of bools")
				}
				c.Features = NormalizeFeatures(m)
				return nil
			},
		},
		featureLeaf("feed", "personal activity feed"),
		featureLeaf("deploy", "deploy column and filters"),
		featureLeaf("qa", "QA column, filters, and inline field edit"),
		featureLeaf("teamGroups", "team/group taxonomy surfaces"),
		{
			Path:        "projects",
			Root:        "projects",
			Description: "Jira project keys to mirror (empty = every project this account can see)",
			Get:         func(c *Config) any { return strsOrEmpty(c.Projects) },
			Set: func(c *Config, raw json.RawMessage) error {
				v, err := decodeStrings(raw, "projects")
				if err != nil {
					return err
				}
				v, err = ValidateProjectKeys(v)
				if err != nil {
					return err
				}
				c.Projects = v
				return nil
			},
		},
		{
			Path:        "defaultProject",
			Root:        "defaultProject",
			Description: "project key used when create omits --project / project_key (empty = unset)",
			Get:         func(c *Config) any { return c.DefaultProject },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "defaultProject")
				if err != nil {
					return err
				}
				v, err := ValidateDefaultProject(s)
				if err != nil {
					return err
				}
				c.DefaultProject = v
				return nil
			},
		},
		{
			Path:        "defaultIssueTypeId",
			Root:        "defaultIssueTypeId",
			Description: "Jira issue type id used when create omits --type / issue_type (empty = unset; not a display name)",
			Get:         func(c *Config) any { return c.DefaultIssueTypeID },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "defaultIssueTypeId")
				if err != nil {
					return err
				}
				v, err := ValidateDefaultIssueTypeID(s)
				if err != nil {
					return err
				}
				c.DefaultIssueTypeID = v
				return nil
			},
		},
		{
			Path:        "defaultIssueType",
			Root:        "defaultIssueType",
			Description: "optional display label for defaultIssueTypeId; create never resolves against this name",
			Get:         func(c *Config) any { return c.DefaultIssueType },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "defaultIssueType")
				if err != nil {
					return err
				}
				v, err := ValidateDefaultIssueType(s)
				if err != nil {
					return err
				}
				c.DefaultIssueType = v
				return nil
			},
		},
		{
			Path:        "qaDashboardUrl",
			Root:        "qaDashboardUrl",
			Description: "external QA dashboard origin for issue-to-test-run links",
			Get:         func(c *Config) any { return c.QaDashboardURL },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "qaDashboardUrl")
				if err != nil {
					return err
				}
				c.QaDashboardURL = s
				return nil
			},
		},
		{
			Path:        "confluence",
			Root:        "confluence",
			Description: "wiki-page source: {enabled, spaces}; empty spaces = every global space",
			Get:         getConfluence,
			Set:         setConfluence,
		},
		{
			Path:        "confluence.enabled",
			Root:        "confluence",
			Description: "wiki-page mirror on/off",
			Get:         func(c *Config) any { return c.Confluence != nil },
			Set: func(c *Config, raw json.RawMessage) error {
				b, err := decodeBool(raw, "confluence.enabled")
				if err != nil {
					return err
				}
				var spaces []string
				if c.Confluence != nil {
					spaces = c.Confluence.Spaces
				}
				return ApplyConfluence(c, &b, spaces)
			},
		},
		{
			Path:        "confluence.spaces",
			Root:        "confluence",
			Description: "mirrored Confluence space keys (rejected while the source is off)",
			Get: func(c *Config) any {
				if c.Confluence == nil {
					return []string{}
				}
				return strsOrEmpty(c.Confluence.Spaces)
			},
			Set: func(c *Config, raw json.RawMessage) error {
				v, err := decodeStrings(raw, "confluence.spaces")
				if err != nil {
					return err
				}
				return ApplyConfluence(c, nil, v)
			},
		},
		{
			Path:        "fields",
			Root:        "fields",
			Description: "discovered/pinned custom-field specs (alias, ids, role, kind)",
			Get:         func(c *Config) any { return fieldSpecsOrEmpty(c.Fields) },
			Set: func(c *Config, raw json.RawMessage) error {
				var v []FieldSpec
				if err := json.Unmarshal(raw, &v); err != nil {
					return fmt.Errorf("fields must be an array of field specs")
				}
				c.Fields = v
				return nil
			},
		},
		{
			Path:        "fieldMap",
			Root:        "fieldMap",
			Description: "legacy alias→custom-field id map; LoadFor synthesizes into fields and clears it (set refuses — use fields)",
			Get:         func(c *Config) any { return stringMapOrEmpty(c.FieldMap) },
			Set: func(*Config, json.RawMessage) error {
				return fmt.Errorf(`use "fields" instead — fieldMap is a legacy shape that is migrated away on the next load`)
			},
		},
		{
			Path:        "bodyFields",
			Root:        "bodyFields",
			Description: "ADF custom-field ids folded into full-text search",
			Get:         func(c *Config) any { return strsOrEmpty(c.BodyFields) },
			Set: func(c *Config, raw json.RawMessage) error {
				v, err := decodeStrings(raw, "bodyFields")
				if err != nil {
					return err
				}
				c.BodyFields = v
				return nil
			},
		},
		{
			Path:        "editableFields",
			Root:        "editableFields",
			Description: "legacy alias→field id write allowlist; LoadFor overlays onto fields and clears it (set refuses — use fields)",
			Get:         func(c *Config) any { return stringMapOrEmpty(c.EditableFields) },
			Set: func(*Config, json.RawMessage) error {
				return fmt.Errorf(`use "fields" instead — editableFields is a legacy shape that is migrated away on the next load`)
			},
		},
		{
			Path:        "members",
			Root:        "members",
			Description: "static member directory (email, name, group, …)",
			Get:         func(c *Config) any { return membersOrEmpty(c.Members) },
			Set: func(c *Config, raw json.RawMessage) error {
				var v []Member
				if err := json.Unmarshal(raw, &v); err != nil {
					return fmt.Errorf("members must be an array of member objects")
				}
				c.Members = v
				return nil
			},
		},
		{
			Path:        "groupRules",
			Root:        "groupRules",
			Description: "first-match group classification rules",
			Get:         func(c *Config) any { return rulesOrEmpty(c.GroupRules) },
			Set: func(c *Config, raw json.RawMessage) error {
				var v []GroupRule
				if err := json.Unmarshal(raw, &v); err != nil {
					return fmt.Errorf("groupRules must be an array of rule objects")
				}
				c.GroupRules = v
				return nil
			},
		},
		{
			Path:        "groupQuery",
			Root:        "groupQuery",
			Description: "optional SELECT/WITH (key, group) that classifies issues; empty string = fall through",
			Get:         func(c *Config) any { return c.GroupQuery },
			Set: func(c *Config, raw json.RawMessage) error {
				var v string
				if err := json.Unmarshal(raw, &v); err != nil {
					return fmt.Errorf("groupQuery must be a string")
				}
				v = strings.TrimSpace(v)
				if err := ValidateGroupQuery(v); err != nil {
					return err
				}
				c.GroupQuery = v
				return nil
			},
		},
		{
			Path:        "groupLabels",
			Root:        "groupLabels",
			Description: "group key → display label",
			Get:         func(c *Config) any { return stringMapOrEmpty(c.GroupLabels) },
			Set: func(c *Config, raw json.RawMessage) error {
				v, err := decodeStringMap(raw, "groupLabels")
				if err != nil {
					return err
				}
				c.GroupLabels = v
				return nil
			},
		},
		{
			Path:        "groupColors",
			Root:        "groupColors",
			Description: "group key → hex color",
			Get:         func(c *Config) any { return stringMapOrEmpty(c.GroupColors) },
			Set: func(c *Config, raw json.RawMessage) error {
				v, err := decodeStringMap(raw, "groupColors")
				if err != nil {
					return err
				}
				c.GroupColors = v
				return nil
			},
		},
		{
			Path:        "productByGroup",
			Root:        "productByGroup",
			Description: "group key → {key, label} product bucket",
			Get:         func(c *Config) any { return productsOrEmpty(c.ProductByGroup) },
			Set: func(c *Config, raw json.RawMessage) error {
				var v map[string]Product
				if err := json.Unmarshal(raw, &v); err != nil {
					return fmt.Errorf("productByGroup must be an object of {key, label}")
				}
				c.ProductByGroup = v
				return nil
			},
		},
	}
	return out
}

func featureLeaf(name, desc string) Setting {
	path := "features." + name
	return Setting{
		Path:        path,
		Root:        "features",
		Description: desc,
		Get: func(c *Config) any {
			return NormalizeFeatures(c.Features)[name]
		},
		Set: func(c *Config, raw json.RawMessage) error {
			b, err := decodeBool(raw, path)
			if err != nil {
				return err
			}
			if c.Features == nil {
				c.Features = map[string]bool{}
			}
			c.Features[name] = b
			return nil
		},
	}
}

func intSetting(path, root, desc string, get func(*Config) int, set func(*Config, int) error) Setting {
	return Setting{
		Path:        path,
		Root:        root,
		Description: desc,
		Get:         func(c *Config) any { return get(c) },
		Set: func(c *Config, raw json.RawMessage) error {
			n, err := decodeInt(raw, path)
			if err != nil {
				return err
			}
			return set(c, n)
		},
	}
}

func boolDefaultTrue(path, root, desc string, get func(*Config) bool, set func(*Config, bool)) Setting {
	return Setting{
		Path:        path,
		Root:        root,
		Description: desc,
		Get:         func(c *Config) any { return get(c) },
		Set: func(c *Config, raw json.RawMessage) error {
			b, err := decodeBool(raw, path)
			if err != nil {
				return err
			}
			set(c, b)
			return nil
		},
	}
}

type confluenceValue struct {
	Enabled *bool    `json:"enabled,omitempty"`
	Spaces  []string `json:"spaces,omitempty"`
}

func getConfluence(c *Config) any {
	if c.Confluence == nil {
		return map[string]any{"enabled": false, "spaces": []string{}}
	}
	return map[string]any{"enabled": true, "spaces": strsOrEmpty(c.Confluence.Spaces)}
}

func setConfluence(c *Config, raw json.RawMessage) error {
	var in confluenceValue
	if err := json.Unmarshal(raw, &in); err != nil {
		return fmt.Errorf("confluence must be an object {\"enabled\": bool, \"spaces\": […]}")
	}
	return ApplyConfluence(c, in.Enabled, in.Spaces)
}

func decodeString(raw json.RawMessage, path string) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("%s must be a string", path)
	}
	return s, nil
}

func decodeBool(raw json.RawMessage, path string) (bool, error) {
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, fmt.Errorf("%s must be true or false", path)
	}
	return b, nil
}

func decodeInt(raw json.RawMessage, path string) (int, error) {
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("%s must be a number", path)
	}
	return n, nil
}

func decodeStrings(raw json.RawMessage, path string) ([]string, error) {
	var v []string
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s must be a JSON array of strings", path)
	}
	if v == nil {
		return []string{}, nil
	}
	return v, nil
}

func decodeStringMap(raw json.RawMessage, path string) (map[string]string, error) {
	var v map[string]string
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object of strings", path)
	}
	return v, nil
}

func strsOrEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func stringMapOrEmpty(v map[string]string) map[string]string {
	if v == nil {
		return map[string]string{}
	}
	return v
}

func fieldSpecsOrEmpty(v []FieldSpec) []FieldSpec {
	if v == nil {
		return []FieldSpec{}
	}
	return v
}

func membersOrEmpty(v []Member) []Member {
	if v == nil {
		return []Member{}
	}
	return v
}

func rulesOrEmpty(v []GroupRule) []GroupRule {
	if v == nil {
		return []GroupRule{}
	}
	return v
}

func productsOrEmpty(v map[string]Product) map[string]Product {
	if v == nil {
		return map[string]Product{}
	}
	return v
}
