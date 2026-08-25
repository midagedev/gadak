package config

// GDK-786 ui.* schema tests. The contract↔assertion map:
//
//	contract                              assertion
//	────────────────────────────────────  ─────────────────────────────────────
//	locked tokens warn and save (858)     TestApplyUIConfigTiers
//	validated rules judge per palette     TestApplyUIConfigPerPaletteJudging
//	judgment violations store + stderr    TestJudgmentViolationsSaveWithWarnings
//	warnings fold, name failing palettes  TestJudgmentWarningNamesFailingPalettes
//	free tokens need hex only             TestApplyUIConfigTiers (lozenge)
//	unknown token carried + warned        TestApplyUIConfigUnknownTokenCarried
//	unknown palette carried + warned      TestApplyUIConfigUnknownTokenCarried
//	dataColors key rules (3 families)     TestValidateDataColorsKeys
//	hex gate on every value               TestValidateDataColorsKeys
//	oversized/corrupt input refuses       TestValidateDataColorsKeys (huge key)
//	tokensByTheme judged in its palette   TestApplyUIConfigPerPaletteJudging
//	settings catalog exposes map paths    TestUISettingsPaths
//	expansion filters drift to advisories TestUITokenVarsDegradesToAdvisory
//	configVersion moves with the file     TestConfigVersionMoves
//
// GDK-858 (user decision 2026-08-25): judgment violations — locked tiers,
// contrast floors — warn and SAVE; only parse/shape/derived refuse. The
// revised assertions below fail against the pre-GDK-858 source (run output
// in the round report).
//
// Fixtures that depend on color math are derived from the catalog at run time
// (a token's own catalog value passes its palette by construction; a ground
// color used as an ink fails the floor by construction) so these tests never
// hand-compute contrast.

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config/tokencheck"
)

func TestApplyUIConfigTiers(t *testing.T) {
	cases := []struct {
		name    string
		ui      *UIConfig
		wantErr string // substring of the refusal; empty means accept
	}{
		{
			// GDK-858: tier is a judgment — the write warns and stores; the
			// dedicated warn+save test below carries the assertions.
			name: "locked token warned and stored",
			ui:   &UIConfig{Tokens: &UITokens{Colors: map[string]string{"bg-base": "#000000"}}},
		},
		{
			name:    "invalid hex refused",
			ui:      &UIConfig{Tokens: &UITokens{Colors: map[string]string{"accent": "not-a-color"}}},
			wantErr: "not a #rgb or #rrggbb hex color",
		},
		{
			name: "free lozenge any-hex accepted",
			ui:   &UIConfig{Tokens: &UITokens{Colors: map[string]string{"lozenge-red": "#c0ffee"}}},
		},
		{
			name: "css-variable key form accepted",
			ui:   &UIConfig{Tokens: &UITokens{Colors: map[string]string{"--color-accent": "#7a4bd0"}}},
		},
		{
			name: "empty colors map accepted",
			ui:   &UIConfig{Tokens: &UITokens{Colors: map[string]string{}}},
		},
		{
			name:    "nil ui is a valid clear",
			ui:      nil,
			wantErr: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{}
			err := ApplyUIConfig(c, tc.ui)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ApplyUIConfig: unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ApplyUIConfig accepted %v", tc.ui)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ApplyUIConfig error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// Each palette's overlay is judged against that palette's own catalog base:
// the own-palette catalog value passes, and using a palette's ground color as
// a status ink (contrast 1.0 against itself) warns naming that palette
// (GDK-858: warns and saves — the palette attribution is what the assertion
// pins; FAIL-first against the pre-GDK-858 source, which refused here).
func TestApplyUIConfigPerPaletteJudging(t *testing.T) {
	byTheme := map[string]*UITokens{}
	for _, p := range tokencheck.CatalogPalettes() {
		v, ok := tokencheck.CatalogValue("status-reopen", p)
		if !ok {
			t.Fatalf("catalog missing status-reopen/%s", p)
		}
		byTheme[p] = &UITokens{Colors: map[string]string{"status-reopen": v}}
	}
	if _, err := ValidateUIConfig(&UIConfig{TokensByTheme: byTheme}); err != nil {
		t.Fatalf("own-catalog values must pass in their own palette: %v", err)
	}

	// The dark entry is the broken one: dark's bg-base as a status ink is
	// contrast 1.0 on dark grounds. The light entry (light's own good ink)
	// must not have been judged against dark's base, or the warning would
	// name the wrong palette — and a fixed-base implementation fails one of
	// the two directions.
	badInk, _ := tokencheck.CatalogValue("bg-base", "dark")
	goodInk, _ := tokencheck.CatalogValue("status-reopen", "light")
	mixed := &UIConfig{TokensByTheme: map[string]*UITokens{
		"dark":  {Colors: map[string]string{"status-reopen": badInk}},
		"light": {Colors: map[string]string{"status-reopen": goodInk}},
	}}
	warns, err := ValidateUIConfig(mixed)
	if err != nil {
		t.Fatalf("judgment violations must warn and save (GDK-858), got refusal: %v", err)
	}
	var floor *tokencheck.Violation
	for i, w := range warns {
		if w.Rule == "status-role-floor" {
			floor = &warns[i]
		}
	}
	if floor == nil {
		t.Fatal("ground-color ink must warn with the role floor")
	}
	if !strings.Contains(floor.Message, "palette dark") {
		t.Fatalf("warning must name the palette whose rules failed: %q", floor.Message)
	}
	if !strings.Contains(floor.Message, "contrast") {
		t.Fatalf("warning must carry the measured contrast: %q", floor.Message)
	}
}

// The merge semantics the web relies on: tokensByTheme wins over tokens for
// its palette, leaves other palettes alone, and tolerates nil blocks.
func TestEffectiveTokenColorsMerge(t *testing.T) {
	var nilUI *UIConfig
	if got := nilUI.EffectiveTokenColors("light"); got != nil {
		t.Errorf("nil config must yield nil, got %v", got)
	}
	u := &UIConfig{
		Tokens: &UITokens{Colors: map[string]string{"accent": "#7a4bd0", "lozenge-red": "#e05050"}},
		TokensByTheme: map[string]*UITokens{
			"dark": {Colors: map[string]string{"accent": "#9a6be0"}},
		},
	}
	light := u.EffectiveTokenColors("light")
	if light["accent"] != "#7a4bd0" || light["lozenge-red"] != "#e05050" {
		t.Errorf("light merge wrong: %v", light)
	}
	dark := u.EffectiveTokenColors("dark")
	if dark["accent"] != "#9a6be0" {
		t.Errorf("theme overlay must win: %v", dark)
	}
	if dark["lozenge-red"] != "#e05050" {
		t.Errorf("palette-agnostic token must carry into themed palette: %v", dark)
	}
	ember := u.EffectiveTokenColors("ember")
	if ember["accent"] != "#7a4bd0" {
		t.Errorf("untouched palette must see base overrides: %v", ember)
	}
	noTokens := &UIConfig{TokensByTheme: map[string]*UITokens{"dark": {Colors: map[string]string{"accent": "#9a6be0"}}}}
	if got := noTokens.EffectiveTokenColors("light"); len(got) != 0 {
		t.Errorf("nil Tokens must not break merge: %v", got)
	}
}

func TestApplyUIConfigUnknownTokenCarried(t *testing.T) {
	c := &Config{}
	ui := &UIConfig{
		Tokens: &UITokens{Colors: map[string]string{
			"accent":            "#7a4bd0",
			"renamed-in-future": "#111111",
		}},
		TokensByTheme: map[string]*UITokens{
			"solarized": {Colors: map[string]string{"accent": "#002b36"}},
		},
	}
	warns, err := ValidateUIConfig(ui)
	if err != nil {
		t.Fatalf("unknown names must not refuse the save: %v", err)
	}
	var sawUnknownToken, sawUnknownPalette bool
	for _, w := range warns {
		if w.Rule == "unknown-token" && w.Token == "renamed-in-future" {
			sawUnknownToken = true
		}
		if w.Rule == "unknown-palette" && w.Token == "solarized" {
			sawUnknownPalette = true
		}
	}
	if !sawUnknownToken {
		t.Errorf("no unknown-token warning for renamed-in-future: %+v", warns)
	}
	if !sawUnknownPalette {
		t.Errorf("no unknown-palette warning for solarized: %+v", warns)
	}
	if err := ApplyUIConfig(c, ui); err != nil {
		t.Fatalf("ApplyUIConfig: %v", err)
	}
	if c.UI.Tokens.Colors["renamed-in-future"] != "#111111" {
		t.Errorf("unknown token must be carried for forward compatibility")
	}
	if c.UI.TokensByTheme["solarized"] == nil {
		t.Errorf("unknown palette must be carried for forward compatibility")
	}
}

// The GDK-858 write contract, end to end: a judgment violation (locked
// tier, contrast floor) SAVES — the value lands in the config a CLI write
// would persist — and the warning reaches stderr. Deliberately rides the
// long-standing ApplyUIConfig signature so the file compiles against the
// pre-change source and the FAIL-first run is a real run: the old source
// refuses both writes (locked + floor are rejects), stores nothing, and
// prints nothing.
func TestJudgmentViolationsSaveWithWarnings(t *testing.T) {
	ink, ok := tokencheck.CatalogValue("bg-base", "dark")
	if !ok {
		t.Fatal("catalog missing bg-base/dark")
	}
	ui := &UIConfig{Tokens: &UITokens{Colors: map[string]string{
		"bg-base":    "#000000", // locked tier: judgment, warn+save
		"status-new": ink,       // dark ground as an all-palette ink: floors fail somewhere
	}}}
	var c Config
	stderr := captureStderr(t, func() {
		if err := ApplyUIConfig(&c, ui); err != nil {
			t.Fatalf("judgment violations must not refuse the save: %v", err)
		}
	})
	if c.UI == nil || c.UI.Tokens.Colors["bg-base"] != "#000000" || c.UI.Tokens.Colors["status-new"] != ink {
		t.Fatalf("judgment-violating values not stored: %+v", c.UI)
	}
	if !strings.Contains(stderr, "gadak: ui:") {
		t.Errorf("warnings did not reach stderr: %q", stderr)
	}
	if !strings.Contains(stderr, "locked") {
		t.Errorf("no locked warning on stderr: %q", stderr)
	}
	if !strings.Contains(stderr, "status-role-floor") && !strings.Contains(stderr, "contrast") {
		t.Errorf("no contrast-floor warning on stderr: %q", stderr)
	}
	// The refusal tone is gone: a warning says the value applied, not "pick a
	// darker ink" alone.
	if !strings.Contains(stderr, "applied") {
		t.Errorf("warnings must say the value applied (GDK-858 tone): %q", stderr)
	}
}

// The fold: one warning line per (rule, token) no matter how many palettes
// repeat it, and palette-judged warnings name the failing palettes plus the
// ui.tokensByTheme scoping fix — the GDK-856 teaching the user decision
// asked for.
func TestJudgmentWarningNamesFailingPalettes(t *testing.T) {
	// All-palette write of dark's ground as a status ink: the floor fails in
	// some palettes and passes in others; one folded line names the set.
	ink, _ := tokencheck.CatalogValue("bg-base", "dark")
	warns, err := ValidateUIConfig(&UIConfig{Tokens: &UITokens{Colors: map[string]string{"status-new": ink}}})
	if err != nil {
		t.Fatalf("judgment write refused: %v", err)
	}
	count := 0
	for _, w := range warns {
		if w.Rule == "status-role-floor" {
			count++
		}
	}
	if count == 0 {
		t.Fatalf("ground ink produced no floor warning: %+v", warns)
	}
	if count > 1 {
		t.Errorf("floor warning printed %d times — the fold must emit one line per rule+token: %+v", count, warns)
	}
	for _, w := range warns {
		if w.Rule == "status-role-floor" {
			if !strings.Contains(w.Message, "palette ") && !strings.Contains(w.Message, "palettes ") {
				t.Errorf("folded warning does not name the failing palette(s): %q", w.Message)
			}
			if !strings.Contains(w.Message, "ui.tokensByTheme") {
				t.Errorf("folded warning does not teach the per-palette scoping fix: %q", w.Message)
			}
		}
	}
}

// captureStderr for this file is the package-wide helper in config_test.go.

func TestValidateDataColorsKeys(t *testing.T) {
	cases := []struct {
		name    string
		dc      map[string]map[string]string
		wantErr string
	}{
		{
			name: "label is free text",
			dc:   map[string]map[string]string{"label": {"urgent": "#c03030", "고객문의": "#30c030"}},
		},
		{
			name:    "type key must be an issue type id",
			dc:      map[string]map[string]string{"type": {"Task": "#d07020"}},
			wantErr: "issue type ids",
		},
		{
			name: "type id digits accepted",
			dc:   map[string]map[string]string{"type": {"10007": "#d07020"}},
		},
		{
			name:    "status key must be a category",
			dc:      map[string]map[string]string{"status": {"In Progress": "#7e5904"}},
			wantErr: "status_category",
		},
		{
			name: "status category accepted",
			dc:   map[string]map[string]string{"status": {"inprogress": "#7e5904"}},
		},
		{
			name:    "unknown family refused with the valid list",
			dc:      map[string]map[string]string{"priority": {"1": "#ff0000"}},
			wantErr: "valid families are label, type, status",
		},
		{
			name:    "value must be hex",
			dc:      map[string]map[string]string{"label": {"urgent": "red"}},
			wantErr: "not a #rgb or #rrggbb hex color",
		},
		{
			name:    "paste-bomb key refused",
			dc:      map[string]map[string]string{"label": {strings.Repeat("x", 257): "#000000"}},
			wantErr: "longer than 256",
		},
		{
			name:    "empty key refused",
			dc:      map[string]map[string]string{"label": {"  ": "#000000"}},
			wantErr: "must not be empty",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDataColors(tc.dc)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateDataColors: unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateDataColors accepted %v", tc.dc)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestUISettingsPaths(t *testing.T) {
	for _, path := range []string{"ui.tokens", "ui.tokensByTheme", "ui.dataColors", "ui.tokens.catalog", "ui.tokens.dim-catalog"} {
		s, ok := SettingByPath(path)
		if !ok {
			t.Errorf("catalog path %q missing", path)
			continue
		}
		if path != "ui.tokens.catalog" && s.Root != "ui" {
			t.Errorf("%s Root = %q, want ui", path, s.Root)
		}
	}
	// Round-trip through the catalog the way `gadak config set` does.
	c := &Config{}
	set, ok := SettingByPath("ui.dataColors")
	if !ok {
		t.Fatal("ui.dataColors missing")
	}
	if err := set.Set(c, []byte(`{"label":{"urgent":"#c03030"},"status":{"done":"#1c4c31"}}`)); err != nil {
		t.Fatalf("set ui.dataColors: %v", err)
	}
	if c.UI.DataColors["label"]["urgent"] != "#c03030" {
		t.Errorf("label ink not stored: %+v", c.UI)
	}
	// A second set replaces the map (groupColors semantics), and a bad key
	// refuses without mutating the stored value.
	if err := set.Set(c, []byte(`{"label":{"urgent":"#000000"},"type":{"10007":"#d07020"}}`)); err != nil {
		t.Fatalf("replace ui.dataColors: %v", err)
	}
	if len(c.UI.DataColors["label"]) != 1 || c.UI.DataColors["label"]["urgent"] != "#000000" {
		t.Errorf("replace semantics broken: %+v", c.UI.DataColors)
	}
	if err := set.Set(c, []byte(`{"label":{"x":"nothex"}}`)); err == nil {
		t.Fatal("invalid hex accepted")
	}
	if c.UI.DataColors["label"]["urgent"] != "#000000" {
		t.Errorf("refused set mutated stored value: %+v", c.UI.DataColors)
	}
}

func TestUITokensFlatAndWrappedShapes(t *testing.T) {
	c := &Config{}
	set, _ := SettingByPath("ui.tokens")
	if err := set.Set(c, []byte(`{"accent":"#7a4bd0"}`)); err != nil {
		t.Fatalf("flat map: %v", err)
	}
	if err := set.Set(c, []byte(`{"colors":{"lozenge-red":"#e05050"}}`)); err != nil {
		t.Fatalf("wrapped map: %v", err)
	}
	if c.UI.Tokens.Colors["lozenge-red"] != "#e05050" || len(c.UI.Tokens.Colors) != 1 {
		t.Errorf("wrapped shape stored wrong: %+v", c.UI.Tokens)
	}
	byTheme, _ := SettingByPath("ui.tokensByTheme")
	if err := byTheme.Set(c, []byte(`{"dark":{"accent":"#9a6be0"}}`)); err != nil {
		t.Fatalf("set ui.tokensByTheme: %v", err)
	}
	if c.UI.TokensByTheme["dark"].Colors["accent"] != "#9a6be0" {
		t.Errorf("theme overlay not stored: %+v", c.UI.TokensByTheme)
	}
	// The overlay is judged in its own palette: light's ground as a status
	// ink warns (and stores — GDK-858), naming palette light.
	if err := byTheme.Set(c, []byte(`{"light":{"status-new":"#ffffff"}}`)); err != nil {
		t.Fatalf("judgment-breaking overlay must save with a warning (GDK-858): %v", err)
	}
	if c.UI.TokensByTheme["light"].Colors["status-new"] != "#ffffff" {
		t.Errorf("judgment-breaking overlay not stored: %+v", c.UI.TokensByTheme)
	}
	warns, err := ValidateUIConfig(c.UI)
	if err != nil {
		t.Fatalf("stored overlay revalidated with an error: %v", err)
	}
	sawFloor := false
	for _, w := range warns {
		if w.Rule == "status-role-floor" && strings.Contains(w.Message, "palette light") {
			sawFloor = true
		}
	}
	if !sawFloor {
		t.Errorf("no role-floor warning naming palette light: %+v", warns)
	}
}

// The stale-schema defense: a config that names a token this build does not
// know must degrade to an advisory, not break expansion — this is the load
// path a downgrade takes. Locked colors are NOT drift anymore (GDK-858:
// they warn at write time and are legitimate overrides), so they expand; a
// tier that some future build re-locks is that build's call to re-filter.
func TestUITokenVarsDegradesToAdvisory(t *testing.T) {
	ui := &UIConfig{Tokens: &UITokens{Colors: map[string]string{
		"accent":        "#7a4bd0",
		"no-such-token": "#111111",
		"bg-base":       "#000000", // locked tier — warn+save+render (GDK-858)
		"lozenge-red":   "red",     // invalid value on disk
	}}}
	vars, warns := UITokenVars(ui)
	if vars["light"]["--color-accent"] != "#7a4bd0" {
		t.Errorf("valid override not expanded: %+v", vars)
	}
	if len(vars["dark"]) == 0 || vars["dark"]["--color-accent"] != "#7a4bd0" {
		t.Errorf("palette-agnostic override missing from dark: %+v", vars)
	}
	for _, p := range tokencheck.CatalogPalettes() {
		if _, ok := vars[p]["--color-no-such-token"]; ok {
			t.Errorf("unknown token injected for %s", p)
		}
		if vars[p]["--color-bg-base"] != "#000000" {
			t.Errorf("locked override must render after its write-time warning (GDK-858), %s: %+v", p, vars[p])
		}
		if _, ok := vars[p]["--color-lozenge-red"]; ok {
			t.Errorf("invalid value injected for %s", p)
		}
	}
	if len(warns) == 0 {
		t.Fatal("no advisories for unknown/invalid entries")
	}
	// dataColors values that reached disk without validation are filtered,
	// valid ones pass through trimmed.
	dc := UIDataColors(&UIConfig{DataColors: map[string]map[string]string{
		"label":               {"urgent": "#c03030", "broken": "nothex"},
		"family-not-rendered": {"x": "#000000"},
	}})
	if dc["label"]["urgent"] != "#c03030" || len(dc["label"]) != 1 {
		t.Errorf("dataColors filter wrong: %+v", dc)
	}
}

func TestConfigVersionMoves(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	d, err := DirFor("")
	if err != nil {
		t.Fatal(err)
	}
	before := ConfigVersionOfDir(d)
	if before == "" {
		t.Fatal("version must be non-empty even before the file exists")
	}
	c := &Config{}
	if err := ApplyUIConfig(c, &UIConfig{Tokens: &UITokens{Colors: map[string]string{"accent": "#7a4bd0"}}}); err != nil {
		t.Fatal(err)
	}
	c.dir = d
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	after := ConfigVersionOfDir(d)
	if after == before {
		t.Fatalf("configVersion did not move on save: %q", after)
	}
	// Same file, same answer — the poll compares for equality.
	if ConfigVersionOfDir(d) != after {
		t.Fatal("configVersion not stable across reads of the same file")
	}
	if ConfigVersionOfDir("") != "" {
		t.Fatal("empty dir must yield empty version")
	}
}
