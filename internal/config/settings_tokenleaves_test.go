package config

/*
 * GDK-853: ui.tokens.<axis>.<name> scalar leaves. The axis JSON object is
 * still the merge; these paths are that merge with a one-key object, spelled
 * as a bare scalar so `gadak config set ui.tokens.type.terminal 15px` works
 * the way every other catalog path does. Assertions go through SettingByPath
 * the way `gadak config set` does:
 *
 *	contract                              assertion
 *	────────────────────────────────────  ─────────────────────────────────────
 *	four templates, not a token census    TestUITokenLeafListingIsTemplates
 *	every catalog name resolves           TestUITokenLeafResolvesCatalogNames
 *	scalar set merges one key             TestUITokenLeafSetScalarMerges
 *	null deletes; get is override-or-null TestUITokenLeafGetAndNullDelete
 *	unknown name names the catalog        TestUITokenLeafUnknownNameRefuses
 *	refusal leaves config unchanged       TestUITokenLeafRefusalLeavesConfigUnchanged
 */

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config/tokencheck"
)

func TestUITokenLeafListingIsTemplates(t *testing.T) {
	listed := map[string]bool{}
	var templates []string
	for _, s := range Settings() {
		listed[s.Path] = true
		if strings.HasPrefix(s.Path, "ui.tokens.") && strings.HasSuffix(s.Path, ".<name>") {
			templates = append(templates, s.Path)
			if s.Root != "ui" {
				t.Errorf("%s Root = %q, want ui", s.Path, s.Root)
			}
			if !strings.Contains(s.Description, "scalar") {
				t.Errorf("%s Description must teach the scalar: %q", s.Path, s.Description)
			}
			if !strings.Contains(s.Description, "null") {
				t.Errorf("%s Description must teach that null deletes: %q", s.Path, s.Description)
			}
			if !strings.Contains(s.Description, "gadak config get ui.tokens.") {
				t.Errorf("%s Description must name a discovery command: %q", s.Path, s.Description)
			}
			if got := marshalSettingGet(t, s, &Config{}); got != "null" {
				t.Errorf("%s template Get = %s, want null (the placeholder is not a token)", s.Path, got)
			}
		}
	}
	wantTemplates := []string{
		"ui.tokens.colors.<name>",
		"ui.tokens.spacing.<name>",
		"ui.tokens.layout.<name>",
		"ui.tokens.type.<name>",
	}
	if strings.Join(templates, ",") != strings.Join(wantTemplates, ",") {
		t.Errorf("templates = %v, want %v", templates, wantTemplates)
	}
	// The listing must not grow by the catalog. Concrete names resolve
	// through SettingByPath; they must not appear as rows.
	for _, p := range []string{
		"ui.tokens.type.terminal",
		"ui.tokens.colors.accent",
		"ui.tokens.spacing.row",
		"ui.tokens.layout.sidebar",
	} {
		if listed[p] {
			t.Errorf("catalog listing enumerated %s — that is the census this path exists to avoid", p)
		}
	}
}

func TestUITokenLeafResolvesCatalogNames(t *testing.T) {
	for _, tok := range tokencheck.CatalogTokens() {
		path := "ui.tokens.colors." + tok.Name
		s, ok := SettingByPath(path)
		if !ok {
			t.Errorf("SettingByPath(%s) missed a catalog color", path)
			continue
		}
		if s.Path != path {
			t.Errorf("SettingByPath(%s).Path = %q", path, s.Path)
		}
	}
	for axis, names := range dimDiscoveryNames() {
		for _, name := range names {
			path := "ui.tokens." + axis + "." + name
			s, ok := SettingByPath(path)
			if !ok {
				t.Errorf("SettingByPath(%s) missed a dim token", path)
				continue
			}
			if s.Path != path {
				t.Errorf("SettingByPath(%s).Path = %q", path, s.Path)
			}
		}
	}
	// CSS-variable spelling of a known token is the same leaf.
	s, ok := SettingByPath("ui.tokens.colors.--color-accent")
	if !ok {
		t.Fatal("CSS-var color leaf did not resolve")
	}
	c := &Config{}
	if err := s.Set(c, json.RawMessage(`"#7a4bd0"`)); err != nil {
		t.Fatalf("set --color-accent: %v", err)
	}
	if c.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Errorf("CSS-var set stored under the var name, want bare: %+v", c.UI.Tokens.Colors)
	}
	s, ok = SettingByPath("ui.tokens.type.--text-terminal")
	if !ok {
		t.Fatal("CSS-var type leaf did not resolve")
	}
	if err := s.Set(c, json.RawMessage(`"15px"`)); err != nil {
		t.Fatalf("set --text-terminal: %v", err)
	}
	if c.UI.Tokens.Type["terminal"] != "15px" {
		t.Errorf("CSS-var set stored under the var name, want bare: %+v", c.UI.Tokens.Type)
	}
	// Siblings that share the ui.tokens. prefix stay exact-match catalog rows.
	for _, path := range []string{"ui.tokens", "ui.tokens.colors", "ui.tokens.catalog", "ui.tokens.dim-catalog"} {
		s, ok := SettingByPath(path)
		if !ok || s.Path != path {
			t.Errorf("exact path %s lost to leaf parsing: ok=%v path=%q", path, ok, s.Path)
		}
	}
	if _, ok := SettingByPath("ui.tokens.type."); ok {
		t.Error("empty name must not resolve")
	}
	if _, ok := SettingByPath("ui.tokens.foo.bar"); ok {
		t.Error("unknown axis must not resolve as a leaf")
	}
}

func TestUITokenLeafSetScalarMerges(t *testing.T) {
	c := &Config{}
	setJSON(t, c, "ui.tokens", `{"colors":{"accent":"#7a4bd0"},"layout":{"sidebar":"280px"}}`)
	setJSON(t, c, "ui.tokensByTheme", `{"dark":{"colors":{"accent":"#9a6be0"}}}`)
	setJSON(t, c, "ui.dataColors", `{"label":{"urgent":"#c03030"}}`)
	setJSON(t, c, "ui.tokens.type.terminal", `"15px"`)
	setJSON(t, c, "ui.tokens.spacing.row", `"44px"`)
	tok := c.UI.Tokens
	if tok.Type["terminal"] != "15px" {
		t.Errorf("type.terminal not stored: %+v", tok)
	}
	if tok.Spacing["row"] != "44px" {
		t.Errorf("spacing.row not stored: %+v", tok)
	}
	if tok.Colors["accent"] != "#7a4bd0" {
		t.Errorf("colors lost to a leaf write: %+v", tok)
	}
	if tok.Layout["sidebar"] != "280px" {
		t.Errorf("layout lost to a leaf write: %+v", tok)
	}
	if c.UI.TokensByTheme["dark"].Colors["accent"] != "#9a6be0" {
		t.Errorf("tokensByTheme lost to a leaf write: %+v", c.UI.TokensByTheme)
	}
	if c.UI.DataColors["label"]["urgent"] != "#c03030" {
		t.Errorf("dataColors lost to a leaf write: %+v", c.UI.DataColors)
	}
	// A second leaf on the same axis is key-wise, like the axis object.
	setJSON(t, c, "ui.tokens.spacing.control", `"30px"`)
	if c.UI.Tokens.Spacing["row"] != "44px" || c.UI.Tokens.Spacing["control"] != "30px" {
		t.Fatalf("second leaf replaced the axis: %+v", c.UI.Tokens.Spacing)
	}
	// The leaf Get echoes only that token, not the axis.
	leaf, ok := SettingByPath("ui.tokens.spacing.row")
	if !ok {
		t.Fatal("ui.tokens.spacing.row missing")
	}
	if got := marshalSettingGet(t, leaf, c); got != `"44px"` {
		t.Errorf("leaf Get = %s, want the one scalar", got)
	}
}

func TestUITokenLeafGetAndNullDelete(t *testing.T) {
	c := &Config{}
	leaf, ok := SettingByPath("ui.tokens.type.terminal")
	if !ok {
		t.Fatal("ui.tokens.type.terminal missing")
	}
	if got := marshalSettingGet(t, leaf, c); got != "null" {
		t.Errorf("unset Get = %s, want null (do not invent the catalog default)", got)
	}
	setJSON(t, c, "ui.tokens.type.terminal", `"15px"`)
	setJSON(t, c, "ui.tokens.colors.accent", `"#7a4bd0"`)
	if got := marshalSettingGet(t, leaf, c); got != `"15px"` {
		t.Errorf("set Get = %s, want \"15px\"", got)
	}
	setJSON(t, c, "ui.tokens.type.terminal", `null`)
	if _, ok := c.UI.Tokens.Type["terminal"]; ok {
		t.Errorf("null did not delete terminal: %+v", c.UI.Tokens.Type)
	}
	if c.UI.Tokens.Colors["accent"] != "#7a4bd0" {
		t.Errorf("null delete dropped a sibling axis: %+v", c.UI.Tokens)
	}
	if got := marshalSettingGet(t, leaf, c); got != "null" {
		t.Errorf("after delete Get = %s, want null", got)
	}
	// Deleting the last token clears the tokens block — axis-path parity.
	setJSON(t, c, "ui.tokens.colors.accent", `null`)
	if c.UI == nil || c.UI.Tokens != nil {
		t.Errorf("deleting the last token must clear the tokens block: %+v", c.UI)
	}
}

func TestUITokenLeafUnknownNameRefuses(t *testing.T) {
	for _, tc := range []struct {
		path, raw, discovery string
	}{
		{"ui.tokens.type.not-a-token", `"15px"`, "ui.tokens.dim-catalog"},
		{"ui.tokens.colors.not-a-token", `"#7a4bd0"`, "ui.tokens.catalog"},
		{"ui.tokens.spacing.<name>", `"44px"`, "ui.tokens.dim-catalog"},
	} {
		s, ok := SettingByPath(tc.path)
		if !ok {
			t.Fatalf("%s must resolve so Set can name the catalog (got unknown path)", tc.path)
		}
		c := &Config{}
		err := s.Set(c, json.RawMessage(tc.raw))
		if err == nil {
			t.Fatalf("%s accepted an unknown token", tc.path)
		}
		if !strings.Contains(err.Error(), tc.discovery) {
			t.Errorf("%s error = %q, want it to name %s", tc.path, err.Error(), tc.discovery)
		}
		if !strings.Contains(err.Error(), "discover names") {
			t.Errorf("%s error = %q, want the discovery teaching", tc.path, err.Error())
		}
		if c.UI != nil {
			t.Errorf("unknown token left a ui block: %+v", c.UI)
		}
	}
}

func TestUITokenLeafRefusalLeavesConfigUnchanged(t *testing.T) {
	c := &Config{}
	setJSON(t, c, "ui.tokens", `{"colors":{"accent":"#7a4bd0"},"spacing":{"row":"44px"}}`)
	for _, tc := range []struct{ path, raw string }{
		{"ui.tokens.spacing.row", `"wide"`},
		{"ui.tokens.colors.accent", `"not-a-color"`},
		{"ui.tokens.layout.docked-min", `"1200px"`},
		{"ui.tokens.type.terminal", `15`},
		{"ui.tokens.type.terminal", `{"terminal":"15px"}`},
	} {
		s, ok := SettingByPath(tc.path)
		if !ok {
			t.Fatalf("%s missing", tc.path)
		}
		if err := s.Set(c, json.RawMessage(tc.raw)); err == nil {
			t.Fatalf("%s accepted %s", tc.path, tc.raw)
		}
		if c.UI.Tokens.Colors["accent"] != "#7a4bd0" || c.UI.Tokens.Spacing["row"] != "44px" {
			t.Fatalf("refused %s %s mutated the config: %+v", tc.path, tc.raw, c.UI.Tokens)
		}
	}
}
