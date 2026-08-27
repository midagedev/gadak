package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// The terminal behavior block (GDK-896). Behavior (shell, working dir,
// scrollback, cursor blink, renderer) lives here; style (font family,
// size, line height) stays token-owned in ui.tokens.type.terminal.

// terminalPaths is the block's schema: the block object plus its five
// leaves. The coverage test below pins both the set and the Root each
// path maps to on PUT /api/settings.
var terminalPaths = []string{
	"terminal",
	"terminal.shell",
	"terminal.workingDir",
	"terminal.scrollback",
	"terminal.cursorBlink",
	"terminal.renderer",
}

func TestTerminalCatalogPathsAndRoots(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Settings() {
		if strings.HasPrefix(s.Path, "terminal") {
			seen[s.Path] = true
		}
	}
	for _, p := range terminalPaths {
		s, ok := SettingByPath(p)
		if !ok {
			t.Errorf("catalog missing %q", p)
			continue
		}
		if s.Root != "terminal" {
			t.Errorf("%s Root = %q, want %q", p, s.Root, "terminal")
		}
		if s.Description == "" || s.Get == nil || s.Set == nil {
			t.Errorf("%s incomplete entry: %+v", p, s)
		}
	}
	if len(seen) != len(terminalPaths) {
		t.Errorf("catalog carries %d terminal paths (%v), want exactly %d", len(seen), seen, len(terminalPaths))
	}
}

func TestTerminalDefaultsRoundTrip(t *testing.T) {
	c := &Config{}
	block, ok := SettingByPath("terminal")
	if !ok {
		t.Fatal("terminal not in catalog")
	}
	got, ok := block.Get(c).(TerminalConfig)
	if !ok {
		t.Fatalf("terminal Get returned %T", block.Get(c))
	}
	want := TerminalConfig{Scrollback: 5000, Renderer: "ghostty"}
	if got != want {
		t.Fatalf("defaults = %+v, want %+v", got, want)
	}
	for _, leaf := range []struct {
		path string
		want any
	}{
		{"terminal.shell", ""},
		{"terminal.workingDir", ""},
		{"terminal.scrollback", 5000},
		{"terminal.cursorBlink", false},
		{"terminal.renderer", "ghostty"},
	} {
		s, ok := SettingByPath(leaf.path)
		if !ok {
			t.Fatalf("%s not in catalog", leaf.path)
		}
		if got := s.Get(c); got != leaf.want {
			t.Errorf("%s default = %#v, want %#v", leaf.path, got, leaf.want)
		}
	}
	// An all-default block set stores nil: zero-value = defaults, so an
	// untouched config never carries the block.
	if err := block.Set(c, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if c.Terminal != nil {
		t.Fatalf("all-default block stored %+v, want nil", c.Terminal)
	}
}

func TestTerminalValidationRejections(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		raw    string
		wantIn []string
	}{
		{"shell not absolute", "terminal.shell", `"zsh"`, []string{"terminal.shell", "absolute"}},
		{"workingDir not absolute", "terminal.workingDir", `"repo/gadak"`, []string{"terminal.workingDir", "absolute"}},
		{"scrollback below range", "terminal.scrollback", `100`, []string{"terminal.scrollback", "200", "100000"}},
		{"scrollback above range", "terminal.scrollback", `2000000`, []string{"terminal.scrollback", "200", "100000"}},
		{"renderer unknown", "terminal.renderer", `"alacritty"`, []string{"terminal.renderer", "ghostty", "xterm"}},
		{"block shell not absolute", "terminal", `{"shell":"fish"}`, []string{"terminal.shell", "absolute"}},
		{"block renderer unknown", "terminal", `{"renderer":"kitty"}`, []string{"terminal.renderer", "ghostty", "xterm"}},
	}
	for _, tc := range cases {
		s, ok := SettingByPath(tc.path)
		if !ok {
			t.Fatalf("%s not in catalog", tc.path)
		}
		err := s.Set(&Config{}, json.RawMessage(tc.raw))
		if err == nil {
			t.Errorf("%s: %s accepted", tc.name, tc.raw)
			continue
		}
		for _, sub := range tc.wantIn {
			if !strings.Contains(err.Error(), sub) {
				t.Errorf("%s: error %q does not name %q", tc.name, err.Error(), sub)
			}
		}
	}
}

func TestTerminalLeafSetPreservesSiblings(t *testing.T) {
	set := func(t *testing.T, c *Config, path, raw string) {
		t.Helper()
		s, ok := SettingByPath(path)
		if !ok {
			t.Fatalf("%s not in catalog", path)
		}
		if err := s.Set(c, json.RawMessage(raw)); err != nil {
			t.Fatalf("set %s = %s: %v", path, raw, err)
		}
	}
	c := &Config{}
	set(t, c, "terminal.shell", `"/bin/zsh"`)
	set(t, c, "terminal.workingDir", `"/tmp"`)
	set(t, c, "terminal.scrollback", `20000`)
	set(t, c, "terminal.cursorBlink", `true`)
	set(t, c, "terminal.renderer", `"xterm"`)
	want := TerminalConfig{
		Shell:       "/bin/zsh",
		WorkingDir:  "/tmp",
		Scrollback:  20000,
		CursorBlink: true,
		Renderer:    "xterm",
	}
	if c.Terminal == nil || *c.Terminal != want {
		t.Fatalf("stored %+v, want %+v", c.Terminal, want)
	}
	// Effective view agrees leaf-for-leaf.
	if got := c.EffectiveTerminal(); got != want {
		t.Fatalf("effective %+v, want %+v", got, want)
	}
	// Resetting every leaf to its default drops the block again.
	set(t, c, "terminal.shell", `""`)
	set(t, c, "terminal.workingDir", `""`)
	set(t, c, "terminal.scrollback", `0`)
	set(t, c, "terminal.cursorBlink", `false`)
	set(t, c, "terminal.renderer", `""`)
	if c.Terminal != nil {
		t.Fatalf("all-default leaves stored %+v, want nil", c.Terminal)
	}
}

func TestTerminalScrollbackZeroIsDefault(t *testing.T) {
	s, ok := SettingByPath("terminal.scrollback")
	if !ok {
		t.Fatal("terminal.scrollback not in catalog")
	}
	c := &Config{}
	if err := s.Set(c, json.RawMessage(`0`)); err != nil {
		t.Fatalf("0 (default) rejected: %v", err)
	}
	if c.Terminal != nil {
		t.Fatalf("default scrollback stored %+v, want nil", c.Terminal)
	}
	boundaries := []int{MinTerminalScrollback, MaxTerminalScrollback}
	for _, n := range boundaries {
		c := &Config{}
		if err := s.Set(c, json.RawMessage(jsonNumber(n))); err != nil {
			t.Fatalf("%d rejected: %v", n, err)
		}
		if c.Terminal == nil || c.Terminal.Scrollback != n {
			t.Fatalf("%d stored %+v", n, c.Terminal)
		}
	}
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// TestTerminalPersistenceShape pins the on-disk contract: the block is
// absent unless something is set, and the stored shape is exactly the
// five leaves.
func TestTerminalPersistenceShape(t *testing.T) {
	b, err := json.Marshal(&Config{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"terminal"`) {
		t.Fatalf("zero-value Config carries a terminal block: %s", b)
	}
	in := []byte(`{"terminal":{"shell":"/bin/zsh","scrollback":20000,"renderer":"xterm"}}`)
	var c Config
	if err := json.Unmarshal(in, &c); err != nil {
		t.Fatal(err)
	}
	want := TerminalConfig{Shell: "/bin/zsh", Scrollback: 20000, Renderer: "xterm"}
	if c.Terminal == nil || *c.Terminal != want {
		t.Fatalf("unmarshaled %+v, want %+v", c.Terminal, want)
	}
	out, err := json.Marshal(&c)
	if err != nil {
		t.Fatal(err)
	}
	var round map[string]any
	if err := json.Unmarshal(out, &round); err != nil {
		t.Fatal(err)
	}
	term, ok := round["terminal"].(map[string]any)
	if !ok {
		t.Fatalf("marshaled shape lost the block: %s", out)
	}
	for _, key := range []string{"shell", "scrollback", "renderer"} {
		if _, ok := term[key]; !ok {
			t.Errorf("marshaled terminal block missing %q: %s", key, out)
		}
	}
	for _, key := range []string{"workingDir", "cursorBlink"} {
		if _, ok := term[key]; ok {
			t.Errorf("marshaled terminal block carries unset %q: %s", key, out)
		}
	}
}

func TestEffectiveTerminalNilConfig(t *testing.T) {
	var c *Config
	got := c.EffectiveTerminal()
	want := TerminalConfig{Scrollback: 5000, Renderer: "ghostty"}
	if got != want {
		t.Fatalf("nil Config effective = %+v, want %+v", got, want)
	}
}
