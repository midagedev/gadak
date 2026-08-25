package config

/*
 * GDK-853: the ui.tokens.<axis> subpaths — colors, spacing, layout, type.
 * The whole-object ui.tokens set replaces (a colors-then-spacing sequence
 * dropped colors on the second write — the agent-theme friction this
 * closes); the subpath merges key-wise instead. Every assertion goes
 * through SettingByPath the way `gadak config set` does, so the file
 * compiles against the pre-change source and FAIL-first is a real run,
 * not a compile error:
 *
 *	contract                              assertion
 *	────────────────────────────────────  ─────────────────────────────────────
 *	four axis paths, Root, teaching       TestUITokenAxisSettingsPaths
 *	merge preserves other axes/colors     TestUITokenAxisMergePreservesOtherAxes
 *	key-wise merge within the axis        TestUITokenAxisMergeKeyWise
 *	null deletes; {} / null are no-ops    TestUITokenAxisNullDeletesAndNoOps
 *	refusal leaves config unchanged       TestUITokenAxisRefusalLeavesConfigUnchanged
 *	bad shapes refuse with teaching       TestUITokenAxisBadShapes
 *	read-only siblings unchanged          TestUITokenAxisReadOnlySiblingsUnchanged
 *
 * Fixtures reuse the values the uitokens tests already proved against the
 * catalogs (accent #7a4bd0 passes contrast in every palette, lozenge-red is
 * a free-tier hex, spacing.row 44px is inside 36–56) so nothing here
 * hand-computes color or dimension math.
 */

import (
	"encoding/json"
	"strings"
	"testing"
)

// setJSON runs one catalog Set the way `gadak config set` does: path lookup,
// raw JSON body. A Set error fails the test — refusals have their own test.
func setJSON(t *testing.T, c *Config, path, raw string) {
	t.Helper()
	s, ok := SettingByPath(path)
	if !ok {
		t.Fatalf("%s missing from the settings catalog", path)
	}
	if err := s.Set(c, json.RawMessage(raw)); err != nil {
		t.Fatalf("set %s %s: %v", path, raw, err)
	}
}

// marshalSettingGet renders a Get exactly the way the CLI echoes it.
func marshalSettingGet(t *testing.T, s Setting, c *Config) string {
	t.Helper()
	b, err := json.Marshal(s.Get(c))
	if err != nil {
		t.Fatalf("marshal Get(%s): %v", s.Path, err)
	}
	return string(b)
}

func TestUITokenAxisSettingsPaths(t *testing.T) {
	for _, path := range []string{"ui.tokens.colors", "ui.tokens.spacing", "ui.tokens.layout", "ui.tokens.type"} {
		s, ok := SettingByPath(path)
		if !ok {
			t.Errorf("catalog path %q missing", path)
			continue
		}
		if s.Root != "ui" {
			t.Errorf("%s Root = %q, want ui", path, s.Root)
		}
		// The description is the catalog's own teaching surface: it must
		// say merge (what separates this path from the whole-object
		// ui.tokens set) and the null-deletes contract.
		if !strings.Contains(s.Description, "merge") {
			t.Errorf("%s Description must teach the merge semantics: %q", path, s.Description)
		}
		if !strings.Contains(s.Description, "null") {
			t.Errorf("%s Description must teach that null deletes a key: %q", path, s.Description)
		}
		// Default Get on an empty config prints {}, never null.
		if got := marshalSettingGet(t, s, &Config{}); got != "{}" {
			t.Errorf("%s default Get = %s, want {}", path, got)
		}
	}
	// The whole-object setter must now point at the axis subpaths — the
	// description is where `config list` readers learn the recipe.
	whole, ok := SettingByPath("ui.tokens")
	if !ok {
		t.Fatal("ui.tokens missing from the settings catalog")
	}
	for _, axis := range []string{"ui.tokens.colors", "ui.tokens.spacing", "ui.tokens.layout", "ui.tokens.type"} {
		if !strings.Contains(whole.Description, axis) {
			t.Errorf("ui.tokens Description must point at %s: %q", axis, whole.Description)
		}
	}
}

func TestUITokenAxisMergePreservesOtherAxes(t *testing.T) {
	c := &Config{}
	setJSON(t, c, "ui.tokens", `{"colors":{"accent":"#7a4bd0"},"layout":{"sidebar":"280px"},"type":{"heading":"24px"}}`)
	setJSON(t, c, "ui.tokensByTheme", `{"dark":{"colors":{"accent":"#9a6be0"}}}`)
	setJSON(t, c, "ui.dataColors", `{"label":{"urgent":"#c03030"}}`)
	// The GDK-853 friction: this write used to drop colors (and everything
	// else) — ui.tokens is a whole-object replace.
	setJSON(t, c, "ui.tokens.spacing", `{"row":"44px"}`)
	tok := c.UI.Tokens
	if tok.Colors["accent"] != "#7a4bd0" {
		t.Errorf("colors lost to the spacing write: %+v", tok)
	}
	if tok.Layout["sidebar"] != "280px" {
		t.Errorf("layout lost to the spacing write: %+v", tok)
	}
	if tok.Type["heading"] != "24px" {
		t.Errorf("type lost to the spacing write: %+v", tok)
	}
	if tok.Spacing["row"] != "44px" {
		t.Errorf("spacing not stored: %+v", tok)
	}
	if c.UI.TokensByTheme["dark"].Colors["accent"] != "#9a6be0" {
		t.Errorf("tokensByTheme lost to the spacing write: %+v", c.UI.TokensByTheme)
	}
	if c.UI.DataColors["label"]["urgent"] != "#c03030" {
		t.Errorf("dataColors lost to the spacing write: %+v", c.UI.DataColors)
	}
	// The colors axis is symmetric: a colors write preserves dimensions.
	setJSON(t, c, "ui.tokens.colors", `{"lozenge-red":"#e05050"}`)
	if c.UI.Tokens.Spacing["row"] != "44px" || c.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Errorf("colors-axis write lost spacing or prior colors: %+v", c.UI.Tokens)
	}
	if c.UI.Tokens.Colors["lozenge-red"] != "#e05050" {
		t.Errorf("colors-axis write not stored: %+v", c.UI.Tokens)
	}
}

func TestUITokenAxisMergeKeyWise(t *testing.T) {
	c := &Config{}
	setJSON(t, c, "ui.tokens.spacing", `{"row":"44px"}`)
	setJSON(t, c, "ui.tokens.spacing", `{"control":"30px"}`)
	if c.UI.Tokens.Spacing["row"] != "44px" || c.UI.Tokens.Spacing["control"] != "30px" {
		t.Fatalf("key-wise merge replaced the axis: %+v", c.UI.Tokens.Spacing)
	}
	// An update overwrites only its own key.
	setJSON(t, c, "ui.tokens.spacing", `{"row":"46px"}`)
	if c.UI.Tokens.Spacing["row"] != "46px" || c.UI.Tokens.Spacing["control"] != "30px" {
		t.Fatalf("update dropped the other key: %+v", c.UI.Tokens.Spacing)
	}
	// The echo carries the whole merged axis, both keys.
	spacing, ok := SettingByPath("ui.tokens.spacing")
	if !ok {
		t.Fatal("ui.tokens.spacing missing from the settings catalog")
	}
	if got := marshalSettingGet(t, spacing, c); got != `{"control":"30px","row":"46px"}` {
		t.Errorf("Get echo = %s, want the merged axis", got)
	}
}

func TestUITokenAxisNullDeletesAndNoOps(t *testing.T) {
	c := &Config{}
	setJSON(t, c, "ui.tokens.colors", `{"accent":"#7a4bd0","lozenge-red":"#e05050"}`)
	setJSON(t, c, "ui.tokens.colors", `{"accent":null}`)
	if _, ok := c.UI.Tokens.Colors["accent"]; ok {
		t.Errorf("null did not delete accent: %+v", c.UI.Tokens.Colors)
	}
	if c.UI.Tokens.Colors["lozenge-red"] != "#e05050" {
		t.Errorf("null delete dropped the sibling key: %+v", c.UI.Tokens.Colors)
	}
	// {} and whole-body null are no-ops — an empty config keeps no ui block.
	fresh := &Config{}
	setJSON(t, fresh, "ui.tokens.spacing", `{}`)
	if fresh.UI != nil {
		t.Errorf("{} created a ui block: %+v", fresh.UI)
	}
	setJSON(t, fresh, "ui.tokens.spacing", `null`)
	if fresh.UI != nil {
		t.Errorf("null created a ui block: %+v", fresh.UI)
	}
	// Deleting the last token clears the tokens block — ui.tokens null parity.
	setJSON(t, fresh, "ui.tokens.colors", `{"accent":"#7a4bd0"}`)
	setJSON(t, fresh, "ui.tokens.colors", `{"accent":null}`)
	if fresh.UI == nil || fresh.UI.Tokens != nil {
		t.Errorf("deleting the last token must clear the tokens block: %+v", fresh.UI)
	}
}

func TestUITokenAxisRefusalLeavesConfigUnchanged(t *testing.T) {
	c := &Config{}
	setJSON(t, c, "ui.tokens", `{"colors":{"accent":"#7a4bd0"},"spacing":{"row":"44px"}}`)
	// An out-of-range dimension (row is 36–56) and a locked color: both
	// refuse, and neither may leak into the stored config.
	for path, raw := range map[string]string{
		"ui.tokens.spacing": `{"row":"90px"}`,
		"ui.tokens.colors":  `{"bg-base":"#000000"}`,
	} {
		s, ok := SettingByPath(path)
		if !ok {
			t.Fatalf("%s missing from the settings catalog", path)
		}
		if err := s.Set(c, json.RawMessage(raw)); err == nil {
			t.Fatalf("%s accepted %s", path, raw)
		}
		if c.UI.Tokens.Colors["accent"] != "#7a4bd0" || c.UI.Tokens.Spacing["row"] != "44px" {
			t.Fatalf("refused %s write mutated the config: %+v", path, c.UI.Tokens)
		}
	}
	// The patch is all-or-nothing: its valid half must not land either.
	spacing, ok := SettingByPath("ui.tokens.spacing")
	if !ok {
		t.Fatal("ui.tokens.spacing missing from the settings catalog")
	}
	if err := spacing.Set(c, json.RawMessage(`{"row":"46px","control":"90px"}`)); err == nil {
		t.Fatal("mixed patch accepted (control 90px is outside 28–40)")
	}
	if c.UI.Tokens.Spacing["row"] != "44px" {
		t.Fatalf("refused mixed patch leaked its valid half: %+v", c.UI.Tokens.Spacing)
	}
}

func TestUITokenAxisBadShapes(t *testing.T) {
	cases := []struct {
		path    string
		raw     string
		wantErr string
	}{
		{"ui.tokens.spacing", `[]`, "must be a JSON object"},
		{"ui.tokens.spacing", `"44px"`, "must be a JSON object"},
		{"ui.tokens.spacing", `{"row": 44}`, "must be a JSON object"},
		// The wrapper shape is ui.tokens itself — sending it to an axis
		// subpath means a key whose value is an object, refused with the
		// shape teaching rather than silently stored as garbage.
		{"ui.tokens.colors", `{"colors":{"accent":"#7a4bd0"}}`, "must be a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.path+" "+tc.raw, func(t *testing.T) {
			s, ok := SettingByPath(tc.path)
			if !ok {
				t.Fatalf("%s missing from the settings catalog", tc.path)
			}
			c := &Config{}
			err := s.Set(c, json.RawMessage(tc.raw))
			if err == nil {
				t.Fatalf("%s accepted %s", tc.path, tc.raw)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
			if c.UI != nil {
				t.Errorf("refused shape left a ui block behind: %+v", c.UI)
			}
		})
	}
}

func TestUITokenAxisReadOnlySiblingsUnchanged(t *testing.T) {
	// The read-only discovery siblings must refuse exactly as before the
	// subpaths landed: same wording, same pointer at the writable path.
	for _, path := range []string{"ui.tokens.catalog", "ui.tokens.dim-catalog"} {
		s, ok := SettingByPath(path)
		if !ok {
			t.Fatalf("%s missing from the settings catalog", path)
		}
		err := s.Set(&Config{}, json.RawMessage(`[]`))
		if err == nil || !strings.Contains(err.Error(), "read-only") || !strings.Contains(err.Error(), "set ui.tokens instead") {
			t.Fatalf("%s Set error = %v, want the read-only refusal pointing at ui.tokens", path, err)
		}
	}
}
