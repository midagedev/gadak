package config

/*
 * GDK-896 R4 — the fonts axis. Four contracts, one per block:
 *
 *   1. the value grammar (validFontStack): injection carriers refuse,
 *      shape violations refuse, real stacks pass;
 *   2. expansion (UIFontVars): only catalogued names with grammar-passing
 *      values reach the web doc, and the load path never dies;
 *   3. the settings paths: whole object, axis merge, scalar leaf (bare and
 *      CSS-var spellings), theme refusal, unknown-axis list;
 *   4. downgrade tolerance (GDK-769 axis 3): a build compiled without the
 *      Fonts field boots on a config that carries it.
 */

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidFontStack(t *testing.T) {
	// A stack at exactly the 256-character cap: 8 families (32+7×31 chars)
	// joined by 7 commas. Judged on a local copy so the test itself proves
	// the length arithmetic.
	atCap := strings.Repeat("a", 32) + "," + strings.Join([]string{
		strings.Repeat("b", 31), strings.Repeat("c", 31), strings.Repeat("d", 31),
		strings.Repeat("e", 31), strings.Repeat("f", 31), strings.Repeat("g", 31),
		strings.Repeat("h", 31),
	}, ",")
	if len(atCap) != 256 {
		t.Fatalf("cap fixture is %d characters, want 256", len(atCap))
	}
	overCap := strings.Repeat("a", 32) + "," + strings.Join([]string{
		strings.Repeat("b", 32), strings.Repeat("c", 32), strings.Repeat("d", 32),
		strings.Repeat("e", 32), strings.Repeat("f", 32), strings.Repeat("g", 32),
		strings.Repeat("h", 32),
	}, ",")
	if len(overCap) != 263 {
		t.Fatalf("over-cap fixture is %d characters, want 263", len(overCap))
	}
	valid := map[string]string{
		"bare identifier":                     "Menlo",
		"hyphenated identifiers":              "ui-monospace, SFMono-Regular, Menlo, monospace",
		"quoted mixed stack":                  "'JetBrains Mono', Menlo, monospace",
		"double quotes":                       "\"SF Mono\", Menlo",
		"quoted digits and underscore":        "'Noto_Sans Mono CJK 2', monospace",
		"untrimmed families":                  "  Menlo ,  monospace  ",
		"exactly 8 families":                  "a, b, c, d, e, f, g, h",
		"exactly 256 characters":              atCap,
		"64-char bare identifier (the cap)":   strings.Repeat("m", 64),
		"64-char quoted inner text (the cap)": "'" + strings.Repeat("m", 64) + "'",
	}
	for name, v := range valid {
		if !validFontStack(v) {
			t.Errorf("%s: %q should pass", name, truncate(v, 48))
		}
	}
	invalid := map[string]string{
		"empty":                     "",
		"spaces only":               "   ",
		"injection: rule close":     "Menlo;}",
		"injection: rule open":      "Menlo}:root{--x:1",
		"injection: function call":  "url(evil)",
		"injection: html tag":       "Menlo, <script>",
		"nine families":             "a, b, c, d, e, f, g, h, i",
		"263 characters":            overCap,
		"leading digit":             "1Menlo",
		"unquoted space":            "Menlo mono",
		"unterminated quote":        "'JetBrains",
		"mismatched quote pair":     "'JetBrains Mono\"",
		"punctuation inside quotes": "'JetBrains Mono!'",
		"empty quoted family":       "'', Menlo",
		"slash in family":           "Menlo/monospace",
		"paren in family":           "Menlo (alt)",
		"colon in family":           "Menlo:root",
		"65-char bare identifier":   strings.Repeat("m", 65),
		"65-char quoted inner":      "'" + strings.Repeat("m", 65) + "'",
	}
	for name, v := range invalid {
		if validFontStack(v) {
			t.Errorf("%s: %q should refuse", name, truncate(v, 48))
		}
	}
}

func TestUIFontVarsExpansion(t *testing.T) {
	ui := &UIConfig{Tokens: &UITokens{Fonts: map[string]string{
		"mono-terminal": "'JetBrains Mono', Menlo, monospace",
		"no-such-font":  "Menlo",   // unknown name: dropped, no advisory
		"serif":         "Menlo;}", // grammar failure: dropped
	}}}
	vars := UIFontVars(ui)
	if vars["--font-mono-terminal"] != "'JetBrains Mono', Menlo, monospace" {
		t.Errorf("valid stack not expanded: %+v", vars)
	}
	if len(vars) != 1 {
		t.Errorf("unknown/invalid entries must drop: %+v", vars)
	}
	// The CSS-variable spelling resolves to the same slot.
	varSpelled := UIFontVars(&UIConfig{Tokens: &UITokens{Fonts: map[string]string{
		"--font-mono-terminal": "Inconsolata, Menlo, monospace",
	}}})
	if varSpelled["--font-mono-terminal"] != "Inconsolata, Menlo, monospace" {
		t.Errorf("CSS-var key not resolved: %+v", varSpelled)
	}
	// Nil-safety: empty map, never nil — the web binds to it like dims.
	if got := UIFontVars(nil); got == nil || len(got) != 0 {
		t.Errorf("nil config must yield an empty map, got %+v", got)
	}
}

func TestUIFontsSettingsPaths(t *testing.T) {
	c := &Config{}
	setJSON(t, c, "ui.tokens", `{"colors":{"accent":"#7a4bd0"},"fonts":{"mono-terminal":"Menlo, monospace"}}`)
	if c.UI.Tokens.Colors["accent"] != "#7a4bd0" || c.UI.Tokens.Fonts["mono-terminal"] != "Menlo, monospace" {
		t.Fatalf("whole-object set dropped an axis: %+v", c.UI.Tokens)
	}
	// Scalar leaf, bare spelling — merges one key, keeps the rest.
	setJSON(t, c, "ui.tokens.fonts.mono-terminal", `"'JetBrains Mono', Menlo"`)
	if c.UI.Tokens.Fonts["mono-terminal"] != "'JetBrains Mono', Menlo" {
		t.Fatalf("leaf set not stored: %+v", c.UI.Tokens.Fonts)
	}
	if c.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Fatalf("leaf set dropped colors: %+v", c.UI.Tokens)
	}
	// CSS-variable spelling of the same leaf resolves to the bare key.
	setJSON(t, c, "ui.tokens.fonts.--font-mono-terminal", `"Inconsolata, Menlo, monospace"`)
	if c.UI.Tokens.Fonts["mono-terminal"] != "Inconsolata, Menlo, monospace" {
		t.Fatalf("CSS-var leaf stored under the wrong key: %+v", c.UI.Tokens.Fonts)
	}
	// A refused value leaves the stored stack untouched.
	leaf, ok := SettingByPath("ui.tokens.fonts.mono-terminal")
	if !ok {
		t.Fatal("fonts leaf missing from the settings catalog")
	}
	if err := leaf.Set(c, json.RawMessage(`"Menlo;}"`)); err == nil {
		t.Fatal("injection value accepted")
	} else if !strings.Contains(err.Error(), "not a font stack") {
		t.Fatalf("refusal should teach the grammar: %v", err)
	}
	if c.UI.Tokens.Fonts["mono-terminal"] != "Inconsolata, Menlo, monospace" {
		t.Fatalf("refused set mutated stored fonts: %+v", c.UI.Tokens.Fonts)
	}
	// An unknown font token name saves with a warning, exactly like the
	// other axes (GDK-913 semantics; forward-compat, GDK-769 axis 3).
	setJSON(t, c, "ui.tokens.fonts.serif", `"Georgia, serif"`)
	if c.UI.Tokens.Fonts["serif"] != "Georgia, serif" {
		t.Fatalf("unknown token name must warn and save: %+v", c.UI.Tokens.Fonts)
	}
	// Null deletes the key; deleting the last key clears nothing else.
	setJSON(t, c, "ui.tokens.fonts.mono-terminal", `null`)
	if _, ok := c.UI.Tokens.Fonts["mono-terminal"]; ok {
		t.Fatalf("null must delete the key: %+v", c.UI.Tokens.Fonts)
	}
	if c.UI.Tokens.Fonts["serif"] != "Georgia, serif" || c.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Fatalf("null deleted siblings: %+v", c.UI.Tokens)
	}
}

func TestUIFontsRefusedPerTheme(t *testing.T) {
	c := &Config{}
	byTheme, ok := SettingByPath("ui.tokensByTheme")
	if !ok {
		t.Fatal("ui.tokensByTheme missing from the catalog")
	}
	if err := byTheme.Set(c, json.RawMessage(`{"dark":{"fonts":{"mono-terminal":"Menlo"}}}`)); err == nil {
		t.Fatal("fonts under ui.tokensByTheme accepted")
	} else if !strings.Contains(err.Error(), "apply to every palette") {
		t.Fatalf("error should teach the scoping fix: %v", err)
	}
	// Colors still ride the overlay — only the palette-agnostic axes refuse.
	if err := byTheme.Set(c, json.RawMessage(`{"dark":{"colors":{"accent":"#9a6be0"}}}`)); err != nil {
		t.Fatalf("colors per theme must keep working: %v", err)
	}
	// The write gate warns (not refuses) when the same config reaches it by
	// other roads — hand edits and newer builds (the PUT path decodes
	// straight into UIConfig, so this is the gate it hits).
	warns, err := ValidateUIConfig(&UIConfig{TokensByTheme: map[string]*UITokens{
		"dark": {Fonts: map[string]string{"mono-terminal": "Menlo"}},
	}})
	if err != nil {
		t.Fatalf("theme-carried fonts must warn, not refuse: %v", err)
	}
	found := false
	for _, w := range warns {
		if w.Rule == "axes-not-per-theme" && strings.Contains(w.Message, "fonts") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing the per-theme advisory for fonts: %+v", warns)
	}
}

func TestUIFontsUnknownTokenWarnsAndCarries(t *testing.T) {
	ui := &UIConfig{Tokens: &UITokens{Fonts: map[string]string{
		"serif": "Georgia, serif",
	}}}
	warns, err := ValidateUIConfig(ui)
	if err != nil {
		t.Fatalf("unknown font name must not refuse: %v", err)
	}
	found := false
	for _, w := range warns {
		if w.Rule == "unknown-token" && strings.Contains(w.Message, "fonts catalog") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing unknown-token advisory: %+v", warns)
	}
}

func TestUITokensFontsDowngradeTolerance(t *testing.T) {
	// A build compiled before the fonts axis (GDK-896 R4) reads a config
	// written with it: json.Unmarshal drops the unknown key silently and the
	// old binary boots (GDK-769 axis 3). The struct below is that old build,
	// spelled locally so this test cannot drift with the real one.
	type legacyUITokens struct {
		Colors  map[string]string `json:"colors,omitempty"`
		Spacing map[string]string `json:"spacing,omitempty"`
		Layout  map[string]string `json:"layout,omitempty"`
		Type    map[string]string `json:"type,omitempty"`
	}
	var legacy struct {
		Tokens *legacyUITokens `json:"tokens,omitempty"`
	}
	in := []byte(`{"tokens":{"colors":{"accent":"#7a4bd0"},"fonts":{"mono-terminal":"Menlo, monospace"}}}`)
	if err := json.Unmarshal(in, &legacy); err != nil {
		t.Fatalf("a fonts-bearing config must load on the old struct: %v", err)
	}
	if legacy.Tokens == nil || legacy.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Fatalf("sibling axes lost to the unknown key: %+v", legacy.Tokens)
	}
}
