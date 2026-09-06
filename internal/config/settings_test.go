package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateTheme(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"system", "", false},
		{"light", "light", false},
		{"dark", "dark", false},
		{"paper", "paper", false},
		{"slate-2", "slate-2", false},
		{"Dark", "", true},
		{"LIGHT", "", true},
		{"has space", "", true},
		{"under_score", "", true},
		{strings.Repeat("a", 33), "", true},
		{strings.Repeat("a", 32), strings.Repeat("a", 32), false},
		{"UPPER", "", true},
		{"slash/no", "", true},
	}
	for _, tc := range cases {
		got, err := ValidateTheme(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ValidateTheme(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateTheme(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ValidateTheme(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEffectiveThemeEmptyIsSystem(t *testing.T) {
	if got := (*Config)(nil).EffectiveTheme(); got != "system" {
		t.Fatalf("nil: %q", got)
	}
	if got := (&Config{}).EffectiveTheme(); got != "system" {
		t.Fatalf("empty: %q", got)
	}
	if got := (&Config{Appearance: &Appearance{Theme: "dark"}}).EffectiveTheme(); got != "dark" {
		t.Fatalf("dark: %q", got)
	}
}

func TestSettingsCatalogHasRequiredPaths(t *testing.T) {
	paths := map[string]bool{}
	for _, s := range Settings() {
		if s.Path == "" || s.Root == "" || s.Description == "" {
			t.Errorf("incomplete setting %+v", s)
		}
		if s.Get == nil || s.Set == nil {
			t.Errorf("%s missing Get/Set", s.Path)
		}
		if paths[s.Path] {
			t.Errorf("duplicate path %q", s.Path)
		}
		paths[s.Path] = true
	}
	for _, name := range FeatureNames {
		want := "features." + name
		if !paths[want] {
			t.Errorf("catalog missing %q", want)
		}
	}
	for _, want := range []string{
		"appearance.theme",
		"syncIntervalSec",
		"reconcileIntervalSec",
		"staleThresholdHours",
		"projects",
		"notify",
		"updateCheck",
		"devStatus",
		"actor.trailer",
		"retro.sessionGap",
	} {
		if !paths[want] {
			t.Errorf("catalog missing %q", want)
		}
	}
}

func TestSettingGetSetAppearanceTheme(t *testing.T) {
	s, ok := SettingByPath("appearance.theme")
	if !ok {
		t.Fatal("appearance.theme not in catalog")
	}
	c := &Config{}
	if got := s.Get(c); got != "system" {
		t.Fatalf("default get = %#v", got)
	}
	if err := s.Set(c, json.RawMessage(`"dark"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance == nil || c.Appearance.Theme != "dark" {
		t.Fatalf("stored %+v", c.Appearance)
	}
	if got := s.Get(c); got != "dark" {
		t.Fatalf("get after set = %#v", got)
	}
	if err := s.Set(c, json.RawMessage(`"system"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance != nil {
		t.Fatalf("system must store nil, got %+v", c.Appearance)
	}
	if err := s.Set(c, json.RawMessage(`"Nope"`)); err == nil {
		t.Fatal("invalid theme accepted")
	}
}

func TestSettingGetSetDevStatus(t *testing.T) {
	s, ok := SettingByPath("devStatus")
	if !ok {
		t.Fatal("devStatus not in catalog")
	}
	if !strings.Contains(s.Description, "dev-status") {
		t.Errorf("description must name Jira's internal dev-status API, got %q", s.Description)
	}
	if !strings.Contains(s.Description, "per-issue") {
		t.Errorf("description must name the per-issue sync cost, got %q", s.Description)
	}
	c := &Config{}
	if got := s.Get(c); got != false {
		t.Fatalf("default get = %#v, want false", got)
	}
	if err := s.Set(c, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if !c.DevStatus {
		t.Fatal("Set(true) did not store DevStatus")
	}
	if got := s.Get(c); got != true {
		t.Fatalf("get after set = %#v", got)
	}
	if err := s.Set(c, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if c.DevStatus {
		t.Fatal("Set(false) did not clear DevStatus")
	}
}

func TestSettingSetIntervalFloor(t *testing.T) {
	s, ok := SettingByPath("syncIntervalSec")
	if !ok {
		t.Fatal("syncIntervalSec missing")
	}
	c := &Config{}
	if err := s.Set(c, json.RawMessage(`5`)); err == nil {
		t.Fatal("below-floor sync accepted")
	}
	if err := s.Set(c, json.RawMessage(`30`)); err != nil {
		t.Fatal(err)
	}
	if c.SyncIntervalSec != 30 {
		t.Fatalf("got %d", c.SyncIntervalSec)
	}
}

func TestValidateIntervalsMoved(t *testing.T) {
	if err := ValidateIntervals(5, 0); err == nil || !strings.Contains(err.Error(), "syncIntervalSec") {
		t.Fatalf("want sync floor error, got %v", err)
	}
	if err := ValidateIntervals(0, 60); err == nil || !strings.Contains(err.Error(), "reconcileIntervalSec") {
		t.Fatalf("want reconcile floor error, got %v", err)
	}
	if err := ValidateIntervals(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIntervals(15, 300); err != nil {
		t.Fatal(err)
	}
}

func TestSettingSetProjectsRejectsInvalidKeys(t *testing.T) {
	// GDK-809: the projects setter is the single choke for key shape.
	// Site membership is CLI-only (needs an origin); this rejects garbage
	// that would otherwise be stored from config set and PUT settings/.
	s, ok := SettingByPath("projects")
	if !ok {
		t.Fatal("projects not in catalog")
	}
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty entry", `[""]`, "project key"},
		{"whitespace", `["  "]`, "project key"},
		{"display-name words", `["Numbers"]`, "project key"},
		{"control char", string(mustJSONStrings(t, "NMB\n")), "project key"},
		{"too long", `["` + strings.Repeat("A", 11) + `"]`, "project key"},
		{"punctuation", `["DI!"]`, "project key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{}
			err := s.Set(c, json.RawMessage(tc.raw))
			if err == nil {
				t.Fatalf("accepted %s", tc.raw)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q missing %q", err, tc.want)
			}
			if len(c.Projects) != 0 {
				t.Errorf("stored %v after rejection", c.Projects)
			}
		})
	}
}

func TestSettingSetProjectsNormalizesAndAcceptsEmpty(t *testing.T) {
	s, ok := SettingByPath("projects")
	if !ok {
		t.Fatal("projects not in catalog")
	}
	c := &Config{}
	if err := s.Set(c, json.RawMessage(`[]`)); err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if c.Projects == nil || len(c.Projects) != 0 {
		t.Fatalf("empty list stored %+v", c.Projects)
	}
	if err := s.Set(c, json.RawMessage(`["nmb","NMB","d1"]`)); err != nil {
		t.Fatalf("valid keys: %v", err)
	}
	if got := strings.Join(c.Projects, ","); got != "NMB,D1" {
		t.Fatalf("normalized %v", c.Projects)
	}
}

func TestSettingGetNeverConfiguredIsEmpty(t *testing.T) {
	// GDK-1241: a setting the user never configured GETs as [] / {}, not
	// JSON null — sliceOrEmpty/mapOrEmpty are the whole guarantee, and this
	// pins the marshaled shape rather than nil-ness (an any holding a nil
	// slice is not a nil interface).
	for path, want := range map[string]string{
		"projects": "[]",
		"fieldMap": "{}",
	} {
		s, ok := SettingByPath(path)
		if !ok {
			t.Fatalf("%s not in catalog", path)
		}
		got, err := json.Marshal(s.Get(&Config{}))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s GET on never-configured config = %s, want %s", path, got, want)
		}
	}
}

func mustJSONStrings(t *testing.T, keys ...string) []byte {
	t.Helper()
	b, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExistingUnknownProjectsSurviveDecode(t *testing.T) {
	// GDK-809 "손상": a config.json that already contains a site-unknown or
	// ill-shaped key must still Load. Validation is Set-only.
	var c Config
	if err := json.Unmarshal([]byte(`{"projects":["DI","Numbers"]}`), &c); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.Projects, ","); got != "DI,Numbers" {
		t.Fatalf("Load-equivalent decode rewrote stored projects: %v", c.Projects)
	}
}

func TestLooksLikeProjectKey(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"GDK", true},
		{"D1", true},
		{"NMB", true},
		{"CRWN", true},
		{"Fix", false}, // help example first word; do not case-fold
		{"FIX", true},
		{"A", false},
		{"", false},
		{strings.Repeat("A", 11), false},
		{"GDK-1", false},
		{"nmb", false},
	}
	for _, tc := range cases {
		if got := LooksLikeProjectKey(tc.in); got != tc.want {
			t.Errorf("LooksLikeProjectKey(%q)=%v want %v", tc.in, got, tc.want)
		}
	}
}

func TestSettingMemorySpaceRoundtripAndValidation(t *testing.T) {
	s, ok := SettingByPath("memory.space")
	if !ok {
		t.Fatal("memory.space not in catalog")
	}
	c := &Config{}
	if got := s.Get(c); got != "" {
		t.Fatalf("unset must read as empty, got %v", got)
	}
	if err := s.Set(c, json.RawMessage(`"ENG"`)); err != nil {
		t.Fatalf("set ENG: %v", err)
	}
	if c.Memory == nil || c.Memory.Space != "ENG" {
		t.Fatalf("stored %+v, want block {ENG}", c.Memory)
	}
	if got := s.Get(c); got != "ENG" {
		t.Fatalf("get after set = %v", got)
	}
	if got := c.MemorySpace(); got != "ENG" {
		t.Fatalf("MemorySpace() = %q", got)
	}
	// Setting empty clears the block — an unset memory is nil, not an
	// empty block, so config.json stays clean.
	if err := s.Set(c, json.RawMessage(`""`)); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if c.Memory != nil {
		t.Fatalf("cleared memory must be nil, got %+v", c.Memory)
	}
	// Whitespace inside is refused: a space key is one token, and a
	// two-word answer here would silently search nothing. Surrounding
	// whitespace trims away, same as defaultProject.
	for name, raw := range map[string]string{
		"interior space": `"EN G"`,
		"interior tab":   "\"EN\tG\"",
	} {
		bad := &Config{}
		if err := s.Set(bad, json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s (%s)", raw, name)
		}
		if bad.Memory != nil {
			t.Fatalf("%s: stored %+v after rejection", name, bad.Memory)
		}
	}
	trimmed := &Config{}
	if err := s.Set(trimmed, json.RawMessage("\"  ENG  \"")); err != nil {
		t.Fatalf("trim-around: %v", err)
	}
	if trimmed.Memory == nil || trimmed.Memory.Space != "ENG" {
		t.Fatalf("trim-around stored %+v", trimmed.Memory)
	}
}

// GDK-1357: the terminal dock's own appearance. Dark is the default and
// stores as the zero value; the leaf setters start from the stored block so
// `gadak config set appearance.theme ink` cannot drop a stored terminal
// value, nor the other way round.
func TestSettingGetSetAppearanceTerminal(t *testing.T) {
	term, ok := SettingByPath("appearance.terminal")
	if !ok {
		t.Fatal("appearance.terminal not in catalog")
	}
	theme, _ := SettingByPath("appearance.theme")
	c := &Config{}
	if got := term.Get(c); got != "dark" {
		t.Fatalf("default get = %#v, want dark", got)
	}
	if err := term.Set(c, json.RawMessage(`"follow"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance == nil || c.Appearance.Terminal != "follow" || c.Appearance.Theme != "" {
		t.Fatalf("stored %+v", c.Appearance)
	}
	// The sibling leaf keeps it.
	if err := theme.Set(c, json.RawMessage(`"ink"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance.Terminal != "follow" || c.Appearance.Theme != "ink" {
		t.Fatalf("theme leaf dropped terminal: %+v", c.Appearance)
	}
	// And this leaf keeps the theme.
	if err := term.Set(c, json.RawMessage(`"dark"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance == nil || c.Appearance.Theme != "ink" || c.Appearance.Terminal != "" {
		t.Fatalf("dark must store as zero and keep the theme: %+v", c.Appearance)
	}
	if got := term.Get(c); got != "dark" {
		t.Fatalf("get after dark = %#v", got)
	}
	// Both defaults → no block at all.
	if err := theme.Set(c, json.RawMessage(`"system"`)); err != nil {
		t.Fatal(err)
	}
	if c.Appearance != nil {
		t.Fatalf("both defaults must store nil, got %+v", c.Appearance)
	}
	if err := term.Set(c, json.RawMessage(`"light"`)); err == nil {
		t.Fatal("unknown terminal appearance accepted")
	}
}

// ApplyTerminalDisplay is the settings-PUT road into the terminal block: it
// must leave shell and workingDir exactly as stored (GDK-1069).
func TestApplyTerminalDisplayKeepsShellAndDir(t *testing.T) {
	c := &Config{Terminal: &TerminalConfig{Shell: "/bin/zsh", WorkingDir: "/srv/work"}}
	if err := ApplyTerminalDisplay(c, 20000, true); err != nil {
		t.Fatal(err)
	}
	want := TerminalConfig{Shell: "/bin/zsh", WorkingDir: "/srv/work", Scrollback: 20000, CursorBlink: true}
	if c.Terminal == nil || *c.Terminal != want {
		t.Fatalf("got %+v, want %+v", c.Terminal, want)
	}
	if err := ApplyTerminalDisplay(c, 5, false); err == nil {
		t.Fatal("scrollback below the floor accepted")
	}
	// Back to defaults on a config with no paths → the block is dropped.
	c = &Config{Terminal: &TerminalConfig{Scrollback: 900}}
	if err := ApplyTerminalDisplay(c, 0, false); err != nil {
		t.Fatal(err)
	}
	if c.Terminal != nil {
		t.Fatalf("all-default block must store nil, got %+v", c.Terminal)
	}
}

// actor.trailer — contract ↔ assertion (FAIL-first: before the key existed
// SettingByPath missed and both the default-get and the set failed):
// default true; false stores on a block that may not exist yet (the actor
// itself can come from env/auto-detect — empty slug is legal there); true
// back drops an otherwise-empty block instead of persisting {}.
func TestSettingGetSetActorTrailer(t *testing.T) {
	s, ok := SettingByPath("actor.trailer")
	if !ok {
		t.Fatal("actor.trailer not in the settings catalog")
	}
	if !strings.Contains(s.Description, "via gadak") || !strings.Contains(s.Description, "default true") {
		t.Errorf("description must name the trailer line and the default: %q", s.Description)
	}
	c := &Config{}
	if got := s.Get(c); got != true {
		t.Fatalf("default get = %#v, want true", got)
	}
	if err := s.Set(c, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if c.Actor == nil || c.Actor.Trailer == nil || *c.Actor.Trailer != false {
		t.Fatalf("stored %+v", c.Actor)
	}
	if got := s.Get(c); got != false {
		t.Fatalf("get after set = %#v", got)
	}
	// A slug set beside it survives a trailer flip (the slug goes through
	// the actor root path, the switch through this leaf).
	root, ok := SettingByPath("actor")
	if !ok {
		t.Fatal("actor not in the settings catalog")
	}
	if err := root.Set(c, json.RawMessage(`"claude:354bff2b|Claude Code"`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(c, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if c.Actor.Slug != "claude:354bff2b" {
		t.Fatalf("trailer flip dropped the slug: %+v", c.Actor)
	}
	// Back to true: with the slug present the block stays; the switch is nil.
	if err := s.Set(c, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if c.Actor == nil || c.Actor.Trailer != nil || c.Actor.Slug != "claude:354bff2b" {
		t.Fatalf("true must clear the switch, not the block: %+v", c.Actor)
	}
	// And on a block that only ever held the switch, true drops it entirely.
	c = &Config{}
	if err := s.Set(c, json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(c, json.RawMessage(`true`)); err != nil {
		t.Fatal(err)
	}
	if c.Actor != nil {
		t.Fatalf("all-zero block must store nil, got %+v", c.Actor)
	}
}

// retro.sessionGap — contract ↔ assertion (FAIL-first: before the key
// existed the default-get failed and the 45m set errored as unknown path):
// default 30m; 45m stores; storing the default drops the block; the same
// bounds as retro.ParseSessionGap reject outside 5m..24h, naming them.
func TestSettingGetSetRetroSessionGap(t *testing.T) {
	s, ok := SettingByPath("retro.sessionGap")
	if !ok {
		t.Fatal("retro.sessionGap not in the settings catalog")
	}
	if !strings.Contains(s.Description, "5m to 24h") || !strings.Contains(s.Description, "30m") {
		t.Errorf("description must name the bounds and the default: %q", s.Description)
	}
	c := &Config{}
	if got := s.Get(c); got != "30m" {
		t.Fatalf("default get = %#v, want 30m", got)
	}
	if err := s.Set(c, json.RawMessage(`"45m"`)); err != nil {
		t.Fatal(err)
	}
	if c.Retro == nil || c.Retro.SessionGap != "45m" {
		t.Fatalf("stored %+v", c.Retro)
	}
	if got := s.Get(c); got != "45m" {
		t.Fatalf("get after set = %#v", got)
	}
	if got := c.EffectiveRetroSessionGap(); got != "45m" {
		t.Fatalf("effective = %q", got)
	}
	if err := s.Set(c, json.RawMessage(`"30m"`)); err != nil {
		t.Fatal(err)
	}
	if c.Retro != nil {
		t.Fatalf("the default must store nil, got %+v", c.Retro)
	}
	for _, bad := range []string{`"1m"`, `"25h"`, `"banana"`, `"0"`} {
		if err := s.Set(c, json.RawMessage(bad)); err == nil || !strings.Contains(err.Error(), "5m and 24h") {
			t.Fatalf("bad value %s accepted: %v", bad, err)
		}
	}
	// Nil config keeps the default (a failed config.Load reads as unset).
	if got := (*Config)(nil).EffectiveRetroSessionGap(); got != "30m" {
		t.Fatalf("nil config effective = %q", got)
	}
}
