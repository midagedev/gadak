package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/config"
)

func configHome(t *testing.T) {
	t.Helper()
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")
}

func TestConfigCommandRegistered(t *testing.T) {
	run, ok := commands["config"]
	if !ok || run == nil {
		t.Fatal("gadak config is not registered")
	}
	if _, ok := helps["config"]; !ok {
		t.Fatal("helps has no config entry")
	}
}

func TestConfigListCoversCatalog(t *testing.T) {
	configHome(t)
	out, err := capture(t, func() error {
		return cmdConfig([]string{"list"})
	})
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	for _, s := range config.Settings() {
		if !strings.Contains(out, s.Path) {
			t.Errorf("list missing path %q", s.Path)
		}
	}
	if !strings.Contains(out, "gadak init") {
		t.Errorf("list must mention gadak init for credentials:\n%s", out)
	}
}

func TestConfigListJSON(t *testing.T) {
	configHome(t)
	out, err := capture(t, func() error {
		return cmdConfig([]string{"list", "--json"})
	})
	if err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	var doc struct {
		Settings []struct {
			Path        string `json:"path"`
			Value       any    `json:"value"`
			Description string `json:"description"`
		} `json:"settings"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(doc.Settings) == 0 {
		t.Fatal("empty settings")
	}
	if !strings.Contains(doc.Note, "gadak init") {
		t.Errorf("note %q", doc.Note)
	}
	seen := map[string]bool{}
	for _, row := range doc.Settings {
		seen[row.Path] = true
	}
	if !seen["appearance.theme"] {
		t.Fatal("json list missing appearance.theme")
	}
}

func TestConfigGetSetAppearanceTheme(t *testing.T) {
	configHome(t)
	cfg := &config.Config{Site: "https://x.example", Email: "a@b.c", Token: "secret-token"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdConfig([]string{"get", "appearance.theme"})
	})
	if err != nil {
		t.Fatalf("get default: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != `"system"` {
		t.Fatalf("default theme %q", out)
	}

	out, err = capture(t, func() error {
		return cmdConfig([]string{"set", "appearance.theme", "dark"})
	})
	if err != nil {
		t.Fatalf("set dark: %v\n%s", err, out)
	}

	saved, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Appearance == nil || saved.Appearance.Theme != "dark" {
		t.Fatalf("disk theme %+v", saved.Appearance)
	}
	if saved.Token != "secret-token" || saved.Email != "a@b.c" {
		t.Fatalf("credential lost: email=%q token=%q", saved.Email, saved.Token)
	}

	out, err = capture(t, func() error {
		return cmdConfig([]string{"get", "appearance.theme", "--json"})
	})
	if err != nil {
		t.Fatalf("get --json: %v\n%s", err, out)
	}
	var got struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Path != "appearance.theme" || got.Value != "dark" {
		t.Fatalf("get json %+v", got)
	}

	if _, err := capture(t, func() error {
		return cmdConfig([]string{"set", "appearance.theme", "system"})
	}); err != nil {
		t.Fatal(err)
	}
	saved, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Appearance != nil {
		t.Fatalf("system must not persist, got %+v", saved.Appearance)
	}
	raw, err := os.ReadFile(mustConfigPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"appearance"`)) {
		t.Fatalf("default appearance persisted:\n%s", raw)
	}
}

func TestConfigGetSetSyncInterval(t *testing.T) {
	configHome(t)
	if _, err := capture(t, func() error {
		return cmdConfig([]string{"set", "syncIntervalSec", "30"})
	}); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error {
		return cmdConfig([]string{"get", "syncIntervalSec"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "30" {
		t.Fatalf("get = %q", out)
	}
	if _, err := capture(t, func() error {
		return cmdConfig([]string{"set", "syncIntervalSec", "5"})
	}); err == nil {
		t.Fatal("below-floor interval accepted")
	}
}

func TestConfigUnknownPathExit64(t *testing.T) {
	configHome(t)
	_, err := capture(t, func() error {
		return cmdConfig([]string{"get", "no.such.path"})
	})
	if err == nil {
		t.Fatal("unknown path succeeded")
	}
	if got := exitStatus(err); got != 64 {
		t.Fatalf("exitStatus=%d want 64; err=%v", got, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "no.such.path") {
		t.Errorf("error missing path: %s", msg)
	}
	if !strings.Contains(msg, "appearance.theme") {
		t.Errorf("error missing valid path list: %s", msg)
	}
}

func TestConfigUnknownFlag(t *testing.T) {
	configHome(t)
	err := cmdConfig([]string{"list", "--pretty"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
		t.Fatalf("got %v", err)
	}
}

func TestSkillMentionsConfig(t *testing.T) {
	skill := gadak.SkillMarkdown()
	if !bytes.Contains(skill, []byte("gadak config")) {
		t.Fatal("skills/gadak/SKILL.md must document gadak config")
	}
	for _, frag := range []string{"config list", "config get", "config set", "appearance.theme"} {
		if !bytes.Contains(skill, []byte(frag)) {
			t.Errorf("skill missing %q", frag)
		}
	}
}

func mustConfigPath(t *testing.T) string {
	t.Helper()
	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
