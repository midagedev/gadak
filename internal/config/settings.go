package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
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

// ValidateMemorySpace accepts empty (unset) or one space key with no
// whitespace. Existence is not checked here — the origin answers that on
// the next write (the space-catalog error names what is actually
// available), and the mirrored-spaces warning lives in the catalog
// description instead.
func ValidateMemorySpace(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.ContainsAny(s, " \t\n\r") {
		return "", fmt.Errorf("memory.space must be one space key (got %q)", s)
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

// The terminal dock's appearance values (GDK-1357). Dark is the default and
// stores as the zero value; follow makes the dock take the app palette.
const (
	TerminalAppearanceDark   = "dark"
	TerminalAppearanceFollow = "follow"
)

// ValidateTerminalAppearance accepts empty or "dark" (both store as the zero
// value — dark is the default) and "follow". Anything else is refused by
// name: a stored value the web does not know would leave the dock in
// whatever the CSS default is, silently.
func ValidateTerminalAppearance(s string) (string, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "", TerminalAppearanceDark:
		return "", nil
	case TerminalAppearanceFollow:
		return s, nil
	}
	return "", fmt.Errorf("appearance.terminal must be %q (default) or %q (got %q)",
		TerminalAppearanceDark, TerminalAppearanceFollow, s)
}

// ApplyAppearance writes a validated look block onto c. "system" theme and
// "dark" terminal are the defaults and store as zero values, so an untouched
// look never persists a block at all.
func ApplyAppearance(c *Config, a Appearance) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	theme, err := ValidateTheme(a.Theme)
	if err != nil {
		return err
	}
	terminal, err := ValidateTerminalAppearance(a.Terminal)
	if err != nil {
		return err
	}
	a = Appearance{Theme: theme, Terminal: terminal}
	if a == (Appearance{}) {
		c.Appearance = nil
		return nil
	}
	c.Appearance = &a
	return nil
}

// appearanceOrZero is the current look block by value, so a leaf setter
// (`appearance.theme`, `appearance.terminal`) starts from what is stored and
// changes one field — not from an empty block that would drop the other.
func (c *Config) appearanceOrZero() Appearance {
	if c == nil || c.Appearance == nil {
		return Appearance{}
	}
	return *c.Appearance
}

// The terminal behavior block (GDK-896). Defaults named once so the
// catalog descriptions, the validators' messages, and EffectiveTerminal
// cannot drift apart.
const (
	// DefaultTerminalScrollback is the pane scrollback in lines.
	DefaultTerminalScrollback = 5000
	// MinTerminalScrollback / MaxTerminalScrollback bound a stored value.
	MinTerminalScrollback = 200
	MaxTerminalScrollback = 100000
)

// ValidateTerminalShell accepts empty (unset: $SHELL, else /bin/sh) or an
// absolute path. Existence is not checked — the config may be edited on
// another machine or before the shell is installed; a missing shell
// surfaces at create time, not here.
func ValidateTerminalShell(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !filepath.IsAbs(s) {
		return "", fmt.Errorf("terminal.shell must be empty or an absolute path (got %q)", s)
	}
	return s, nil
}

// ValidateTerminalWorkingDir accepts empty (unset: the workspace work dir)
// or an absolute path. Existence is not checked here; create falls back to
// the default with a log line when the directory is missing.
func ValidateTerminalWorkingDir(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !filepath.IsAbs(s) {
		return "", fmt.Errorf("terminal.workingDir must be empty or an absolute path (got %q)", s)
	}
	return s, nil
}

// ValidateTerminalScrollback accepts 0 (default) or [200, 100000] lines.
func ValidateTerminalScrollback(n int) (int, error) {
	if n == 0 {
		return 0, nil
	}
	if n < MinTerminalScrollback || n > MaxTerminalScrollback {
		return 0, fmt.Errorf("terminal.scrollback must be 0 (default %d) or between %d and %d (got %d)",
			DefaultTerminalScrollback, MinTerminalScrollback, MaxTerminalScrollback, n)
	}
	return n, nil
}

// ApplyTerminal is the PUT settings / `gadak config set terminal*` rule.
// Every field is validated; the block is stored nil when every field is
// its default, so an untouched config never carries it (zero-value =
// defaults, no migration).
func ApplyTerminal(c *Config, t TerminalConfig) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	shell, err := ValidateTerminalShell(t.Shell)
	if err != nil {
		return err
	}
	dir, err := ValidateTerminalWorkingDir(t.WorkingDir)
	if err != nil {
		return err
	}
	scrollback, err := ValidateTerminalScrollback(t.Scrollback)
	if err != nil {
		return err
	}
	t = TerminalConfig{
		Shell:       shell,
		WorkingDir:  dir,
		Scrollback:  scrollback,
		CursorBlink: t.CursorBlink,
	}
	if t == (TerminalConfig{}) {
		c.Terminal = nil
		return nil
	}
	c.Terminal = &t
	return nil
}

// ApplyTerminalDisplay sets the two display fields of the terminal block
// and leaves shell and workingDir as stored. It is the only terminal write
// PUT settings/ performs (GDK-1069: that endpoint admits paired serve-scope
// Bearers, so the fields that name the next spawned binary are not on it).
func ApplyTerminalDisplay(c *Config, scrollback int, cursorBlink bool) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	next := c.terminalOrZero()
	next.Scrollback = scrollback
	next.CursorBlink = cursorBlink
	return ApplyTerminal(c, next)
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

// SettingByPath looks up one catalog entry. Concrete ui.tokens.<axis>.<name>
// leaves are not enumerated in the catalog listing (that would add one row
// per token and bury the rest of the table); they resolve here from the
// four ui.tokens.<axis>.<name> templates.
func SettingByPath(path string) (Setting, bool) {
	if canonical, ok := settingAliases[path]; ok {
		path = canonical
	}
	for _, s := range buildSettings() {
		if s.Path == path {
			return s, true
		}
	}
	if axis, name, ok := parseUITokenLeafPath(path); ok {
		return uiTokenLeafSetting(axis, name), true
	}
	return Setting{}, false
}

// settingAliases are second spellings of a stored path. The wiki source is
// keyed `confluence` because that is the file's field and the Atlassian
// product it started as — but the Built-in tracker's wiki is not
// Confluence, and every other surface (Settings → Sources, `gadak page`,
// `gadak wiki`, the skill) calls it the wiki. A user on a paired workspace
// typed `wiki` and `spaces` first and got "unknown path" twice (GDK-1289).
// The stored key does not move: aliases resolve here, the echo prints the
// canonical path, and `config list` keeps one row per fact.
var settingAliases = map[string]string{
	"wiki":         "confluence",
	"wiki.enabled": "confluence.enabled",
	"wiki.spaces":  "confluence.spaces",
}

// SettingAliases returns "alias → canonical" pairs in a stable order, for
// the unknown-path listing.
func SettingAliases() []string {
	keys := make([]string, 0, len(settingAliases))
	for k := range settingAliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = k + " → " + settingAliases[k]
	}
	return out
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
			Description: "UI look block {theme, terminal}; empty theme means system, empty terminal means dark",
			Get: func(c *Config) any {
				return Appearance{Theme: c.EffectiveTheme(), Terminal: c.EffectiveTerminalAppearance()}
			},
			Set: func(c *Config, raw json.RawMessage) error {
				var a Appearance
				if err := json.Unmarshal(raw, &a); err != nil {
					return fmt.Errorf("appearance must be an object {\"theme\": \"…\", \"terminal\": \"dark|follow\"}")
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
				next := c.appearanceOrZero()
				next.Theme = s
				return ApplyAppearance(c, next)
			},
		},
		{
			Path: "appearance.terminal",
			Root: "appearance",
			Description: "terminal dock appearance: dark (default — the dock paints the dark palette " +
				"under every app theme) or follow (the dock takes the app palette)",
			Get: func(c *Config) any { return c.EffectiveTerminalAppearance() },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "appearance.terminal")
				if err != nil {
					return err
				}
				next := c.appearanceOrZero()
				next.Terminal = s
				return ApplyAppearance(c, next)
			},
		},
		{
			Path: "ui.tokens",
			Root: "ui",
			Description: "token overrides: colors {\"accent\": \"#7a4bd0\"} for every palette; " +
				"dimensions {\"spacing\": {\"row\": \"44px\"}, \"layout\": {\"sidebar\": \"280px\"}, " +
				"\"type\": {\"heading\": \"24px\"}} and fonts {\"mono-terminal\": \"Menlo, monospace\"} " +
				"apply to every palette — a set replaces the " +
				"whole object; to update one axis only, set ui.tokens.colors / ui.tokens.spacing / " +
				"ui.tokens.layout / ui.tokens.type / ui.tokens.fonts (key-wise merge); to update one token, set " +
				"ui.tokens.<axis>.<name> to a scalar (unparseable values and the " +
				"derived layout.docked-min refuse; locked tiers and contrast/range/relation " +
				"judgments warn and save; discover color " +
				"names with `gadak config get ui.tokens.catalog`, dimension names with " +
				"`gadak config get ui.tokens.dim-catalog`, font tokens: mono-terminal)",
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
		uiTokenAxisSetting("colors"),
		uiTokenLeafTemplate("colors"),
		uiTokenAxisSetting("spacing"),
		uiTokenLeafTemplate("spacing"),
		uiTokenAxisSetting("layout"),
		uiTokenLeafTemplate("layout"),
		uiTokenAxisSetting("type"),
		uiTokenLeafTemplate("type"),
		uiTokenAxisSetting("fonts"),
		uiTokenLeafTemplate("fonts"),
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
				palettes := slices.Sorted(maps.Keys(in))
				for _, p := range palettes {
					t, err := parseThemeTokenOverlay("ui.tokensByTheme."+p, in[p])
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
		{
			Path: "ui.tokens.dim-catalog",
			Root: "ui",
			Description: "read-only dimension-token catalog (axis, name, cssVar, tier, default, range, " +
				"relations) — the discovery path for ui.tokens spacing/layout/type keys",
			Get: func(c *Config) any { return dimCatalogEntries() },
			Set: func(*Config, json.RawMessage) error {
				return fmt.Errorf("ui.tokens.dim-catalog is read-only — it ships with the binary; set ui.tokens instead")
			},
		},
		{
			Path: "terminal",
			Root: "terminal",
			Description: "terminal behavior block: {shell, workingDir, scrollback, cursorBlink}; " +
				"a set replaces the whole object — omitted fields return to their defaults " +
				"(empty shell = $SHELL else /bin/sh, empty workingDir = the workspace dir, " +
				"scrollback 0 = 5000). Style (font, size, line height) " +
				"lives in ui.tokens.type.terminal, not here",
			Get: func(c *Config) any { return c.EffectiveTerminal() },
			Set: func(c *Config, raw json.RawMessage) error {
				var t TerminalConfig
				if err := json.Unmarshal(raw, &t); err != nil {
					return fmt.Errorf(`terminal must be an object {"shell": "/bin/zsh", "workingDir": "/tmp", "scrollback": 5000, "cursorBlink": false}`)
				}
				return ApplyTerminal(c, t)
			},
		},
		{
			Path: "terminal.shell",
			Root: "terminal",
			Description: "absolute shell path for new terminal sessions (empty = $SHELL, else /bin/sh; " +
				"existence is not checked here — a missing shell fails at create, by name)",
			Get: func(c *Config) any { return c.EffectiveTerminal().Shell },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "terminal.shell")
				if err != nil {
					return err
				}
				v, err := ValidateTerminalShell(s)
				if err != nil {
					return err
				}
				next := c.terminalOrZero()
				next.Shell = v
				return ApplyTerminal(c, next)
			},
		},
		{
			Path: "terminal.workingDir",
			Root: "terminal",
			Description: "absolute starting directory for new terminal sessions (empty = the workspace " +
				"dir; a directory missing at create time falls back to it with a log line)",
			Get: func(c *Config) any { return c.EffectiveTerminal().WorkingDir },
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "terminal.workingDir")
				if err != nil {
					return err
				}
				v, err := ValidateTerminalWorkingDir(s)
				if err != nil {
					return err
				}
				next := c.terminalOrZero()
				next.WorkingDir = v
				return ApplyTerminal(c, next)
			},
		},
		intSetting("terminal.scrollback", "terminal",
			"scrollback lines a terminal pane keeps (0 = default 5000; 200–100000 when set)",
			func(c *Config) int { return c.EffectiveTerminal().Scrollback },
			func(c *Config, n int) error {
				v, err := ValidateTerminalScrollback(n)
				if err != nil {
					return err
				}
				next := c.terminalOrZero()
				next.Scrollback = v
				return ApplyTerminal(c, next)
			},
		),
		{
			Path:        "terminal.cursorBlink",
			Root:        "terminal",
			Description: "cursor blink in the terminal pane (default false)",
			Get:         func(c *Config) any { return c.EffectiveTerminal().CursorBlink },
			Set: func(c *Config, raw json.RawMessage) error {
				b, err := decodeBool(raw, "terminal.cursorBlink")
				if err != nil {
					return err
				}
				next := c.terminalOrZero()
				next.CursorBlink = b
				return ApplyTerminal(c, next)
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
			Description: "workspace-default acting identity for writes to a local-origin/paired origin " +
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
			Description: "display-name language of a local-origin workspace's origin: \"\", en, ko, ja, de " +
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
			Get:         func(c *Config) any { return sliceOrEmpty(c.Projects) },
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
			Description: "wiki-page source: {enabled, spaces}; empty spaces = every team space (personal excluded); alias: wiki",
			Get:         getConfluence,
			Set:         setConfluence,
		},
		{
			Path:        "confluence.enabled",
			Root:        "confluence",
			Description: "wiki-page mirror on/off; alias: wiki.enabled",
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
			Description: "mirrored wiki space keys (rejected while the source is off); alias: wiki.spaces",
			Get: func(c *Config) any {
				if c.Confluence == nil {
					return []string{}
				}
				return sliceOrEmpty(c.Confluence.Spaces)
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
			Path:        "memory.space",
			Root:        "memory",
			Description: "space key the memory verbs own (empty = local-origin's seeded space; connected refuses until set; the sync joins it into scope even outside confluence.spaces, so a full sync keeps its pages)",
			Get: func(c *Config) any {
				if c.Memory == nil {
					return ""
				}
				return c.Memory.Space
			},
			Set: func(c *Config, raw json.RawMessage) error {
				s, err := decodeString(raw, "memory.space")
				if err != nil {
					return err
				}
				v, err := ValidateMemorySpace(s)
				if err != nil {
					return err
				}
				if v == "" {
					c.Memory = nil
					return nil
				}
				c.Memory = &MemoryConfig{Space: v}
				return nil
			},
		},
		{
			Path:        "fields",
			Root:        "fields",
			Description: "discovered/pinned custom-field specs (alias, ids, role, kind)",
			Get:         func(c *Config) any { return sliceOrEmpty(c.Fields) },
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
			Get:         func(c *Config) any { return mapOrEmpty(c.FieldMap) },
			Set: func(*Config, json.RawMessage) error {
				return fmt.Errorf(`use "fields" instead — fieldMap is a legacy shape that is migrated away on the next load`)
			},
		},
		{
			Path:        "bodyFields",
			Root:        "bodyFields",
			Description: "ADF custom-field ids folded into full-text search",
			Get:         func(c *Config) any { return sliceOrEmpty(c.BodyFields) },
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
			Get:         func(c *Config) any { return mapOrEmpty(c.EditableFields) },
			Set: func(*Config, json.RawMessage) error {
				return fmt.Errorf(`use "fields" instead — editableFields is a legacy shape that is migrated away on the next load`)
			},
		},
		{
			Path:        "members",
			Root:        "members",
			Description: "static member directory (email, name, group, …)",
			Get:         func(c *Config) any { return sliceOrEmpty(c.Members) },
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
			Get:         func(c *Config) any { return sliceOrEmpty(c.GroupRules) },
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
			Get:         func(c *Config) any { return mapOrEmpty(c.GroupLabels) },
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
			Get:         func(c *Config) any { return mapOrEmpty(c.GroupColors) },
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
			Get:         func(c *Config) any { return mapOrEmpty(c.ProductByGroup) },
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

// ── ui.tokens.<axis> subpaths and ui.tokens.<axis>.<name> leaves (GDK-853) ──

// uiTokenAxisExamples is the example body each axis subpath's description
// teaches. Every value is one a real set accepts alone (body "14px" is not:
// the type-step relation holds title at its default 15px, which is < 14+2).
var uiTokenAxisExamples = map[string]string{
	"colors":  `{"accent": "#7a4bd0"}`,
	"spacing": `{"row": "44px"}`,
	"layout":  `{"sidebar": "280px"}`,
	"type":    `{"heading": "24px"}`,
	"fonts":   `{"mono-terminal": "Menlo, monospace"}`,
}

// uiTokenAxisSetting builds one ui.tokens.<axis> entry. The whole-object
// ui.tokens set replaces — an agent that stores colors and then sets
// spacing used to drop colors on the second write (the GDK-853 friction).
// The subpath merges key-wise instead: named keys update, everything else
// (other keys of the axis, other axes, tokensByTheme, dataColors) survives,
// a null value deletes its key, and {} (or null) is a no-op. Validation
// still judges the merged whole through ApplyUIConfig, so a refused write
// leaves the config untouched exactly like every other set.
func uiTokenAxisSetting(axis string) Setting {
	path := "ui.tokens." + axis
	discovery := uiTokenDiscovery(axis)
	rules := uiTokenRules(axis)
	return Setting{
		Path: path,
		Root: "ui",
		Description: fmt.Sprintf("the %s axis of ui.tokens as a key-wise merge: %s updates the named "+
			"keys only — other keys, the other axes and ui.tokensByTheme are preserved; "+
			"a null value deletes its key, {} is a no-op; one token: ui.tokens.%s.<name> with a scalar "+
			"(token names: %s; %s)",
			axis, uiTokenAxisExamples[axis], axis, discovery, rules),
		Get: func(c *Config) any {
			return mapOrEmpty(uiTokenAxisMap(uiTokensOf(c), axis))
		},
		Set: func(c *Config, raw json.RawMessage) error {
			patch, err := parseUITokenAxisPatch(path, raw)
			if err != nil {
				return err
			}
			if len(patch) == 0 {
				return nil // {} (and null): merge nothing, touch nothing
			}
			next := cloneUIConfig(c.UI)
			next.Tokens = mergeUITokenAxis(uiTokensOf(c), axis, patch)
			return ApplyUIConfig(c, next)
		},
	}
}

// uiTokensOf reads the token block nil-safely (a catalog Get runs on configs
// that have no ui block at all).
func uiTokensOf(c *Config) *UITokens {
	if c == nil || c.UI == nil {
		return nil
	}
	return c.UI.Tokens
}

// uiTokenAxisMap reads one axis off a UITokens; nil-safe like the Get that
// calls it.
func uiTokenAxisMap(t *UITokens, axis string) map[string]string {
	if t == nil {
		return nil
	}
	switch axis {
	case "colors":
		return t.Colors
	case "spacing":
		return t.Spacing
	case "layout":
		return t.Layout
	case "fonts":
		return t.Fonts
	default: // "type"
		return t.Type
	}
}

// parseUITokenAxisPatch decodes one axis-set body: token name → value, where
// a JSON null deletes that key. Values must be strings (or null) — an object
// under a key means the caller sent the ui.tokens wrapper shape to a
// one-axis path.
func parseUITokenAxisPatch(path string, raw json.RawMessage) (map[string]*string, error) {
	var in map[string]*string
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object of token→value with string values (null deletes the key) — the {\"colors\": …} wrapper shape is ui.tokens itself", path)
	}
	return in, nil
}

// mergeUITokenAxis stages the post-merge token block: cur copied, one axis
// rebuilt from a fresh map with the patch applied. Only the patched axis is
// rebuilt — a refused write must leave every live map untouched (the
// cloneUIConfig guarantee), so the other axes ride as shared read-only maps
// exactly like the parsed replacements the whole-object setters install.
// Deleting the last token anywhere clears the block, matching what
// `ui.tokens null` does.
func mergeUITokenAxis(cur *UITokens, axis string, patch map[string]*string) *UITokens {
	next := &UITokens{}
	if cur != nil {
		*next = *cur
	}
	merged := map[string]string{}
	for k, v := range uiTokenAxisMap(next, axis) {
		merged[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(merged, k)
			continue
		}
		merged[k] = *v
	}
	if len(merged) == 0 {
		merged = nil
	}
	switch axis {
	case "colors":
		next.Colors = merged
	case "spacing":
		next.Spacing = merged
	case "layout":
		next.Layout = merged
	case "fonts":
		next.Fonts = merged
	default:
		next.Type = merged
	}
	if len(next.Colors)+len(next.Spacing)+len(next.Layout)+len(next.Type)+len(next.Fonts) == 0 {
		return nil
	}
	return next
}

// uiTokenAxisNames is the closed set of ui.tokens.<axis> keys. Leaf paths
// ui.tokens.<axis>.<name> resolve against this list so the catalog listing
// stays one template row per axis instead of one row per token (GDK-853).
var uiTokenAxisNames = []string{"colors", "spacing", "layout", "type", "fonts"}

// uiTokenLeafExamples is the scalar body each axis leaf template teaches.
// Values are ones a real set accepts alone (same source as uiTokenAxisExamples,
// plus type.terminal — the GDK-853 motivating path, independent of the type ladder).
var uiTokenLeafExamples = map[string]struct{ name, value string }{
	"colors":  {"accent", "#7a4bd0"},
	"spacing": {"row", "44px"},
	"layout":  {"sidebar", "280px"},
	"type":    {"terminal", "15px"},
	"fonts":   {"mono-terminal", "Menlo, monospace"},
}

func uiTokenLeafTemplate(axis string) Setting {
	return uiTokenLeafSetting(axis, "<name>")
}

// parseUITokenLeafPath splits ui.tokens.<axis>.<name>. Exact catalog paths
// (ui.tokens.type, ui.tokens.catalog) do not match: they have no name segment.
func parseUITokenLeafPath(path string) (axis, name string, ok bool) {
	const prefix = "ui.tokens."
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	rest := path[len(prefix):]
	for _, ax := range uiTokenAxisNames {
		p := ax + "."
		if strings.HasPrefix(rest, p) {
			name = rest[len(p):]
			if name == "" {
				return "", "", false
			}
			return ax, name, true
		}
	}
	return "", "", false
}

func uiTokenDiscovery(axis string) string {
	if axis == "colors" {
		return "`gadak config get ui.tokens.catalog`"
	}
	if axis == "fonts" {
		return "`gadak config get ui.tokens.fonts` (one token: mono-terminal, the terminal pane stack)"
	}
	return "`gadak config get ui.tokens.dim-catalog`"
}

func uiTokenRules(axis string) string {
	if axis == "colors" {
		return "unparseable values refuse; locked tiers and contrast/ΔEok judgments warn and save"
	}
	if axis == "fonts" {
		return "values that do not parse as a font stack refuse — comma-separated families (1-8, at most 256 characters total), each a bare identifier (Menlo) or a quoted name ('JetBrains Mono'); no warn tier exists"
	}
	return "unparseable lengths and the derived docked-min refuse; range and relation judgments warn and save"
}

// knownUITokenName reports the catalog's bare name for a leaf segment.
// Bare names ("terminal", "accent") and CSS-variable spellings
// ("--text-terminal", "--color-accent") both resolve; the stored key is the
// bare name so a leaf set matches an axis JSON set of the same token.
func knownUITokenName(axis, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if axis == "colors" {
		if _, ok := tokencheck.TierOf(name); !ok {
			return "", false
		}
		return strings.TrimPrefix(name, "--color-"), true
	}
	if axis == "fonts" {
		bare, _, ok := uiFontToken(name)
		return bare, ok
	}
	want, ok := tokencheck.DimTokenOf(axis, name)
	if !ok {
		return "", false
	}
	for _, n := range dimDiscoveryNames()[axis] {
		tok, _ := tokencheck.DimTokenOf(axis, n)
		if n == name || tok.CSSVar == want.CSSVar {
			return n, true
		}
	}
	return "", false
}

// uiTokenLeafSetting is one ui.tokens.<axis>.<name> entry. The axis subpath
// merges a JSON object; this merges a single scalar the same way
// `gadak config set locale ko` does — no quotes, no one-key wrapper.
// Validation still judges the merged whole through ApplyUIConfig.
func uiTokenLeafSetting(axis, name string) Setting {
	path := "ui.tokens." + axis + "." + name
	ex := uiTokenLeafExamples[axis]
	discovery := uiTokenDiscovery(axis)
	rules := uiTokenRules(axis)
	return Setting{
		Path: path,
		Root: "ui",
		Description: fmt.Sprintf("one %s token as a scalar: ui.tokens.%s.%s %s merges that key — "+
			"other keys, the other axes and ui.tokensByTheme are preserved; "+
			"null deletes it (token names: %s; %s)",
			axis, axis, ex.name, ex.value, discovery, rules),
		Get: func(c *Config) any {
			// A known name resolves to its catalog key; an unknown one reads
			// under the name as given, the same key its own set stored. Both
			// the ui.tokens object and the ui.tokens.<axis> merge already work
			// this way (GDK-913).
			key := name
			if canonical, ok := knownUITokenName(axis, name); ok {
				key = canonical
			}
			m := uiTokenAxisMap(uiTokensOf(c), axis)
			if v, ok := m[key]; ok {
				return v
			}
			return nil
		},
		Set: func(c *Config, raw json.RawMessage) error {
			// The name is not a second gate. ui.tokens (whole object) and
			// ui.tokens.<axis> (merge) both warn-and-save an unknown token
			// name — ApplyUIConfig / ValidateUIConfig documents that as
			// forward-compat, not an error (GDK-769). This leaf path once
			// rejected it (GDK-853, for discoverability), so the same token
			// set as a scalar and set as a JSON blob reached different
			// products. Route through the one gate the other two use: a known
			// name resolves to its catalog key, an unknown one passes through
			// as given and ApplyUIConfig carries the discovery hint as a
			// warning (GDK-913).
			key := name
			if canonical, ok := knownUITokenName(axis, name); ok {
				key = canonical
			}
			var patch map[string]*string
			if string(raw) == "null" {
				patch = map[string]*string{key: nil}
			} else {
				s, err := decodeString(raw, path)
				if err != nil {
					return err
				}
				patch = map[string]*string{key: &s}
			}
			next := cloneUIConfig(c.UI)
			next.Tokens = mergeUITokenAxis(uiTokensOf(c), axis, patch)
			return ApplyUIConfig(c, next)
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
	return map[string]any{"enabled": true, "spaces": sliceOrEmpty(c.Confluence.Spaces)}
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

// sliceOrEmpty and mapOrEmpty are the GET-side nil→empty pair: a setting the
// user never configured must marshal as [] / {}, not null.
func sliceOrEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func mapOrEmpty[K comparable, V any](v map[K]V) map[K]V {
	if v == nil {
		return map[K]V{}
	}
	return v
}

// ── ui.tokens.dim-catalog (read-only dimension-token discovery, GDK-852) ──

// tokencheck owns the dimension catalog but exports lookup (DimAxes,
// DimTokenOf), not enumeration — and the discovery path must list every
// name without a second hand-maintained table. The same generated file
// tokencheck embeds is embedded here once more and parsed for the name
// list only: every field served to the user comes from tokencheck, and
// settings_dimcatalog_test.go pins the two reads of the one file together
// (it reads it off disk, so an embed/parse drift fails the test build).
//
//go:embed tokencheck/dim-catalog.json
var dimDiscoveryNamesJSON []byte

// dimDiscoveryNames parses the embedded file into axis → sorted token
// names. A parse failure is a build defect (the file is generated and
// committed); the zero result lists nothing, which the enumeration test
// fails loudly — the same stance as tokencheck's own embed.
func dimDiscoveryNames() map[string][]string {
	var file struct {
		Axes []struct {
			ID     string              `json:"id"`
			Tokens map[string]struct{} `json:"tokens"`
		} `json:"axes"`
	}
	out := map[string][]string{}
	if err := json.Unmarshal(dimDiscoveryNamesJSON, &file); err != nil {
		return out
	}
	for _, ax := range file.Axes {
		out[ax.ID] = slices.Sorted(maps.Keys(ax.Tokens))
	}
	return out
}

// DimCatalogEntry is one row of the read-only dimension-token discovery
// catalog — the sibling of tokencheck.CatalogToken on the color side.
// Min/Max read null where a relation owns the bound (row-excerpt,
// detail-max) or the token is locked, mirroring the embedded DimToken;
// Relations is always present, [] when the token stands alone, like the
// color catalog's rules field. Locked tokens (docked-min) stay in the
// same list, tier-marked — the color catalog's locked notation.
type DimCatalogEntry struct {
	Axis      string   `json:"axis"`
	Name      string   `json:"name"`
	CSSVar    string   `json:"cssVar"`
	Tier      string   `json:"tier"`
	Unit      string   `json:"unit"`
	Default   string   `json:"default"`
	Min       *float64 `json:"min"`
	Max       *float64 `json:"max"`
	Relations []string `json:"relations"`
}

// dimRelationSpec mirrors tokencheck's unexported dimRelations so the
// discovery output can state each cross-token rule as a sentence — the
// prose lives here because tokencheck never exports it. The sync is
// behavioral, not textual: TestDimCatalogRelationsMatchValidation fires
// every rule valid input can violate against ValidateDimensions, so the
// two tables cannot drift apart silently (overlay-max ≤ shell-max is
// pinned here but structurally unfirable — its ranges never overlap).
var dimRelationSpecs = []struct {
	axis    string
	a, b    string
	kind    string // "le" | "ge" | "ge_add"
	add     float64
	because string
}{
	{"spacing", "control-sm", "control", "le", 0, "the small control rides inside the regular one"},
	{"spacing", "row-excerpt", "row", "ge_add", 8, "a row carrying a preview line needs headroom"},
	{"layout", "detail-max", "detail-min", "ge", 0, "the docked panel cannot be narrower than its floor"},
	{"layout", "overlay-max", "shell-max", "le", 0, "the overlay panel cannot outgrow the shell"},
	{"layout", "sidebar-narrow", "sidebar", "le", 0, "the narrow sidebar step must not exceed the default"},
	{"type", "body", "micro", "ge_add", 2, "type steps closer than 2px read as noise, not hierarchy"},
	{"type", "title", "body", "ge_add", 2, "type steps closer than 2px read as noise, not hierarchy"},
	{"type", "heading", "title", "ge_add", 2, "type steps closer than 2px read as noise, not hierarchy"},
}

// dimRelationSentence renders one rule the way a write-time warning would
// state it, so the discovery output and the warning read the same.
func dimRelationSentence(axis, a, b, kind string, add float64, because string) string {
	tokA, _ := tokencheck.DimTokenOf(axis, a)
	tokB, _ := tokencheck.DimTokenOf(axis, b)
	switch kind {
	case "le":
		return fmt.Sprintf("%s must stay ≤ %s (%s)", tokA.CSSVar, tokB.CSSVar, because)
	case "ge_add":
		return fmt.Sprintf("%s must be ≥ %s + %s (%s)", tokA.CSSVar, tokB.CSSVar, dimNumber(tokB.Unit, add), because)
	default: // "ge"
		return fmt.Sprintf("%s must be ≥ %s (%s)", tokA.CSSVar, tokB.CSSVar, because)
	}
}

// dimNumber renders a bound in the token's unit (line-heights are unitless).
func dimNumber(unit string, v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if unit == "none" {
		return s
	}
	return s + "px"
}

// dimCatalogEntries builds the discovery output: axes in DimAxes() order,
// names sorted within an axis, every field from tokencheck.DimTokenOf —
// the embedded name list contributes names only. Each relation sentence
// rides on both participants: overriding either side can break the pair.
func dimCatalogEntries() []DimCatalogEntry {
	relations := map[string][]string{}
	for _, r := range dimRelationSpecs {
		s := dimRelationSentence(r.axis, r.a, r.b, r.kind, r.add, r.because)
		relations[r.axis+"\x00"+r.a] = append(relations[r.axis+"\x00"+r.a], s)
		relations[r.axis+"\x00"+r.b] = append(relations[r.axis+"\x00"+r.b], s)
	}
	names := dimDiscoveryNames()
	out := []DimCatalogEntry{}
	for _, axis := range tokencheck.DimAxes() {
		for _, name := range names[axis] {
			tok, ok := tokencheck.DimTokenOf(axis, name)
			if !ok {
				// Same file, same build; the enumeration test fails
				// loudly long before this could read as a short list.
				continue
			}
			rel := relations[axis+"\x00"+name]
			if rel == nil {
				rel = []string{}
			}
			out = append(out, DimCatalogEntry{
				Axis:      axis,
				Name:      name,
				CSSVar:    tok.CSSVar,
				Tier:      tok.Tier,
				Unit:      tok.Unit,
				Default:   tok.Default,
				Min:       tok.Min,
				Max:       tok.Max,
				Relations: rel,
			})
		}
	}
	return out
}
