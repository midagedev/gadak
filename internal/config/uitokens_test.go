package config

// GDK-786 ui.* schema tests. The contract↔assertion map:
//
//	contract                              assertion
//	────────────────────────────────────  ─────────────────────────────────────
//	locked tokens refuse writes           TestApplyUIConfigTiers
//	validated tokens pass rules           TestApplyUIConfigPerPaletteJudging
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
			name:    "locked token refused",
			ui:      &UIConfig{Tokens: &UITokens{Colors: map[string]string{"bg-base": "#000000"}}},
			wantErr: "locked",
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
// a status ink (contrast 1.0 against itself) is refused naming that palette.
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
	// must not have been judged against dark's base, or the refusal would
	// name the wrong palette — and a fixed-base implementation fails one of
	// the two directions.
	badInk, _ := tokencheck.CatalogValue("bg-base", "dark")
	goodInk, _ := tokencheck.CatalogValue("status-reopen", "light")
	mixed := &UIConfig{TokensByTheme: map[string]*UITokens{
		"dark":  {Colors: map[string]string{"status-reopen": badInk}},
		"light": {Colors: map[string]string{"status-reopen": goodInk}},
	}}
	_, err := ValidateUIConfig(mixed)
	if err == nil {
		t.Fatal("ground-color ink must be refused")
	}
	if !strings.Contains(err.Error(), "palette dark") {
		t.Fatalf("refusal must name the palette whose rules failed: %v", err)
	}
	if !strings.Contains(err.Error(), "contrast") {
		t.Fatalf("refusal must carry the measured contrast: %v", err)
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
	// The overlay is judged in its own palette: dark's ground as a status ink
	// must refuse even though tokens (all-palette) is empty.
	if err := byTheme.Set(c, []byte(`{"light":{"status-new":"#ffffff"}}`)); err == nil {
		t.Fatal("unchecked overlay accepted")
	}
}

// The stale-schema defense: a config that names a token this build does not
// know (or a token that became locked) must degrade to an advisory, not break
// expansion — this is the load path a downgrade takes.
func TestUITokenVarsDegradesToAdvisory(t *testing.T) {
	ui := &UIConfig{Tokens: &UITokens{Colors: map[string]string{
		"accent":        "#7a4bd0",
		"no-such-token": "#111111",
		"bg-base":       "#000000", // locked in this build
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
		if _, ok := vars[p]["--color-bg-base"]; ok {
			t.Errorf("locked token injected for %s", p)
		}
		if _, ok := vars[p]["--color-lozenge-red"]; ok {
			t.Errorf("invalid value injected for %s", p)
		}
	}
	if len(warns) == 0 {
		t.Fatal("no advisories for unknown/locked/invalid entries")
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
