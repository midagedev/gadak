package config

/*
 * Dimension-axis tests (dim-token chunk 1: schema, catalog, validation).
 * Mirrors the color-axis map in uitokens_test.go:
 *
 *	contract                              assertion
 *	────────────────────────────────────  ─────────────────────────────────────
 *	locked docked-min refuses writes      TestApplyUIDimensionsRefusals
 *	bad length / range refused            TestApplyUIDimensionsRefusals
 *	unknown dim names carried + warned    TestApplyUIDimensionsUnknownCarried
 *	dims inside tokensByTheme warn only   TestTokensByThemeDimsWarnNotRefuse
 *	CLI roundtrip via the settings path   TestUIDimensionsSettingsRoundtrip
 *	expansion degrades drift to advisory  TestUIDimensionVarsDegradesToAdvisory
 *	expansion is palette-agnostic flat    TestUIDimensionVarsShape
 */

import (
	"strings"
	"testing"
)

func TestApplyUIDimensionsRefusals(t *testing.T) {
	cases := []struct {
		name    string
		spacing map[string]string
		layout  map[string]string
		typ     map[string]string
		wantErr string
	}{
		{
			name:    "locked docked-min refused",
			layout:  map[string]string{"docked-min": "1200px"},
			wantErr: "locked",
		},
		{
			name:    "missing px unit refused",
			spacing: map[string]string{"row": "44"},
			wantErr: "not a px length",
		},
		{
			name:    "range refused",
			spacing: map[string]string{"row": "60px"},
			wantErr: "outside",
		},
		{
			name:    "relation refused",
			spacing: map[string]string{"control-sm": "34px"},
			wantErr: "control",
		},
		{
			name:    "line-height format refused",
			typ:     map[string]string{"body-line-height": "1.4px"},
			wantErr: "not a unitless",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := &UIConfig{Tokens: &UITokens{Spacing: tc.spacing, Layout: tc.layout, Type: tc.typ}}
			_, err := ValidateUIConfig(ui)
			if err == nil {
				t.Fatalf("ValidateUIConfig accepted %+v", ui)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestApplyUIDimensionsUnknownCarried(t *testing.T) {
	c := &Config{}
	ui := &UIConfig{Tokens: &UITokens{Spacing: map[string]string{
		"row":           "44px",
		"renamed-later": "48px",
	}}}
	warns, err := ValidateUIConfig(ui)
	if err != nil {
		t.Fatalf("unknown dim names must not refuse the save: %v", err)
	}
	saw := false
	for _, w := range warns {
		if w.Rule == "unknown-token" && w.Token == "renamed-later" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("no unknown-token warning for renamed-later: %+v", warns)
	}
	if err := ApplyUIConfig(c, ui); err != nil {
		t.Fatalf("ApplyUIConfig: %v", err)
	}
	if c.UI.Tokens.Spacing["renamed-later"] != "48px" {
		t.Errorf("unknown dim name must be carried for forward compatibility")
	}
}

// A config.json written by hand (or a future build) can carry dimension axes
// inside tokensByTheme; the schema struct has the fields, so the write gate
// warns instead of pretending they render.
func TestTokensByThemeDimsWarnNotRefuse(t *testing.T) {
	ui := &UIConfig{
		TokensByTheme: map[string]*UITokens{
			"dark": {Spacing: map[string]string{"row": "44px"}},
		},
	}
	warns, err := ValidateUIConfig(ui)
	if err != nil {
		t.Fatalf("dims in tokensByTheme must not refuse: %v", err)
	}
	saw := false
	for _, w := range warns {
		if strings.Contains(w.Message, "ui.tokens") && w.Token == "dark" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("no warning for dims under tokensByTheme: %+v", warns)
	}
	vars, _ := UIDimensionVars(ui)
	if len(vars) != 0 {
		t.Errorf("tokensByTheme dims must not expand: %+v", vars)
	}
}

// Round-trip through the settings catalog the way `gadak config set ui.tokens`
// does, without touching a real workspace.
func TestUIDimensionsSettingsRoundtrip(t *testing.T) {
	set, ok := SettingByPath("ui.tokens")
	if !ok {
		t.Fatal("ui.tokens missing from the settings catalog")
	}
	c := &Config{}
	if err := set.Set(c, []byte(`{"spacing":{"row":"44px"},"type":{"body-line-height":"1.5"}}`)); err != nil {
		t.Fatalf("set ui.tokens with dimension axes: %v", err)
	}
	if c.UI.Tokens.Spacing["row"] != "44px" || c.UI.Tokens.Type["body-line-height"] != "1.5" {
		t.Fatalf("dimension axes not stored: %+v", c.UI.Tokens)
	}
	got, ok := set.Get(c).(UITokens)
	if !ok {
		t.Fatalf("Get returned %T, want UITokens", set.Get(c))
	}
	if got.Spacing["row"] != "44px" {
		t.Errorf("Get does not round-trip the spacing axis: %+v", got)
	}
	// A refused set leaves the stored value untouched.
	if err := set.Set(c, []byte(`{"layout":{"sidebar":"200px"}}`)); err == nil {
		t.Fatal("relation-violating sidebar accepted")
	}
	if c.UI.Tokens.Spacing["row"] != "44px" || c.UI.Tokens.Layout != nil {
		t.Errorf("refused set mutated stored tokens: %+v", c.UI.Tokens)
	}
	// Dimensions under tokensByTheme are refused at parse: they apply to
	// every palette, so per-palette copies would be dead weight.
	byTheme, _ := SettingByPath("ui.tokensByTheme")
	if err := byTheme.Set(c, []byte(`{"dark":{"spacing":{"row":"44px"}}}`)); err == nil {
		t.Fatal("spacing under ui.tokensByTheme accepted")
	} else if !strings.Contains(err.Error(), "ui.tokens") {
		t.Fatalf("error should point at ui.tokens: %v", err)
	}
	// Unknown wrapper axes refuse with the valid list — a typo must not
	// silently drop the payload.
	if err := set.Set(c, []byte(`{"radius":{"sm":"4px"}}`)); err == nil {
		t.Fatal("unknown axis accepted")
	} else if !strings.Contains(err.Error(), "colors, spacing, layout, type") {
		t.Fatalf("error should teach the axis list: %v", err)
	}
}

func TestUIDimensionVarsDegradesToAdvisory(t *testing.T) {
	ui := &UIConfig{Tokens: &UITokens{
		Spacing: map[string]string{
			"row":         "44px",
			"no-such-dim": "48px",
			"docked-hack": "48px",
		},
		Layout: map[string]string{
			"docked-min": "1200px", // locked in this build
			"sidebar":    "w280",   // invalid value on disk
		},
	}}
	vars, warns := UIDimensionVars(ui)
	if vars["--spacing-row"] != "44px" {
		t.Errorf("valid override not expanded: %+v", vars)
	}
	for _, banned := range []string{"--spacing-no-such-dim", "--layout-docked-min", "--layout-sidebar"} {
		if _, ok := vars[banned]; ok {
			t.Errorf("%s injected despite drift", banned)
		}
	}
	if len(warns) < 3 {
		t.Errorf("expected advisories for unknown/locked/invalid dims, got %+v", warns)
	}
	var sawLocked, sawUnknown bool
	for _, w := range warns {
		if w.Rule == "locked" {
			sawLocked = true
		}
		if w.Rule == "unknown-token" {
			sawUnknown = true
		}
	}
	if !sawLocked || !sawUnknown {
		t.Errorf("missing locked/unknown advisories: %+v", warns)
	}
	if _, ok := vars["--spacing-docked-hack"]; ok {
		// docked-hack is an unknown spacing name; it must not be injected.
		t.Errorf("unknown spacing name injected")
	}
}

func TestUIDimensionVarsShape(t *testing.T) {
	ui := &UIConfig{Tokens: &UITokens{
		Spacing: map[string]string{"row": "44px", "--spacing-control": "34px"},
		Layout:  map[string]string{"sidebar": "280px"},
		Type:    map[string]string{"body": "14px", "body-line-height": "1.5"},
	}}
	vars, warns := UIDimensionVars(ui)
	want := map[string]string{
		"--spacing-row":            "44px",
		"--spacing-control":        "34px",
		"--layout-sidebar":         "280px",
		"--text-body":              "14px",
		"--text-body--line-height": "1.5",
	}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("vars[%s] = %q, want %q (all: %+v)", k, vars[k], v, vars)
		}
	}
	if len(vars) != len(want) {
		t.Errorf("vars has %d entries, want %d: %+v", len(vars), len(want), vars)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected advisories: %+v", warns)
	}
}
