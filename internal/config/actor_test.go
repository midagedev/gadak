package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// The actor ladder (GDK-586): env GADAK_ACTOR > config.json actor >
// Claude Code auto-detection > none. Every case pins the ambient Claude
// Code marker so the outcome is the test's, not the invoking agent's —
// go test under Claude Code inherits CLAUDECODE=1; CI does not.
func TestResolveActorLadder(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("AI_AGENT", "")

	// ④ nothing set → no actor (writes keep the origin's default identity).
	if a, ok := ResolveActor(nil); ok {
		t.Fatalf("empty environment resolved %+v, want none", a)
	}
	if a, ok := ResolveActor(&Config{}); ok {
		t.Fatalf("empty config resolved %+v, want none", a)
	}

	// ③ CLAUDECODE=1 derives claude:<first 8 of the session id>.
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "e3e6a49a-1382-4502-9381-2c89d3234d74")
	a, ok := ResolveActor(&Config{})
	if !ok || a.Slug != "claude:e3e6a49a" || a.Name != "Claude Code" || a.Source != ActorSourceAuto {
		t.Fatalf("auto = %+v ok=%v, want claude:e3e6a49a / Claude Code / auto", a, ok)
	}

	// AI_AGENT does not feed the slug: as measured it carries the harness
	// version, which would mint a new identity per upgrade.
	t.Setenv("AI_AGENT", "claude-code_2-1-239_agent")
	if a, _ := ResolveActor(&Config{}); a.Slug != "claude:e3e6a49a" {
		t.Fatalf("AI_AGENT changed the slug: %+v", a)
	}

	// ③ with no session id the bare marker is still an agent of its own.
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	if a, ok := ResolveActor(&Config{}); !ok || a.Slug != "claude" {
		t.Fatalf("no-session auto = %+v ok=%v, want slug claude", a, ok)
	}

	// A marker other than "1" does not fire the Claude Code rung.
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDECODE", "yes")
	if a, ok := ResolveActor(&Config{}); ok {
		t.Fatalf("CLAUDECODE=yes resolved %+v; the rung keys on =1", a)
	}

	// ② config is the workspace default; env wins over it.
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	cfg := &Config{Actor: &ActorConfig{Slug: "claude:9c01ab", Name: "Nightly"}}
	if a, ok := ResolveActor(cfg); !ok || a.Slug != "claude:9c01ab" || a.Source != ActorSourceConfig {
		t.Fatalf("config rung = %+v ok=%v", a, ok)
	}
	t.Setenv("GADAK_ACTOR", "grok:aa11|Grok")
	if a, ok := ResolveActor(cfg); !ok || a.Slug != "grok:aa11" || a.Name != "Grok" || a.Source != ActorSourceEnv {
		t.Fatalf("env must win over config: %+v ok=%v", a, ok)
	}

	// ① the env forms: bare slug, slug|name, and empty-slug fallthrough.
	for _, tc := range []struct {
		env      string
		wantSlug string
		wantName string
		wantOK   bool
	}{
		{env: "codex:77", wantSlug: "codex:77", wantOK: true},
		{env: "  codex:77 | Sam  ", wantSlug: "codex:77", wantName: "Sam", wantOK: true},
		{env: "   ", wantSlug: "claude:9c01ab", wantName: "Nightly", wantOK: true}, // whitespace-only is unset → config rung answers
	} {
		t.Setenv("GADAK_ACTOR", tc.env)
		a, ok := ResolveActor(cfg)
		if ok != tc.wantOK || (ok && (a.Slug != tc.wantSlug || a.Name != tc.wantName)) {
			t.Fatalf("GADAK_ACTOR=%q → %+v ok=%v, want slug %q name %q ok=%v",
				tc.env, a, ok, tc.wantSlug, tc.wantName, tc.wantOK)
		}
	}
	// An empty slug ("|name") is unset, not a failure: the ladder answers
	// from the next rung.
	t.Setenv("GADAK_ACTOR", "|JustAName")
	if a, ok := ResolveActor(cfg); !ok || a.Slug != "claude:9c01ab" || a.Source != ActorSourceConfig {
		t.Fatalf("empty-slug env did not fall through to config: %+v ok=%v", a, ok)
	}

	// A config block with an empty slug is unset, not an actor with no name.
	if a, ok := ResolveActor(&Config{Actor: &ActorConfig{Name: "Ghost"}}); ok {
		t.Fatalf("slugless config block resolved %+v", a)
	}
}

func TestParseActorShorthand(t *testing.T) {
	for _, tc := range []struct{ in, wantSlug, wantName string }{
		{"claude:354bff2b", "claude:354bff2b", ""},
		{"claude:354bff2b|Claude Code", "claude:354bff2b", "Claude Code"},
		{" claude:x | Pad Left ", "claude:x", "Pad Left"},
		{"|name-only", "", "name-only"},
		{"", "", ""},
	} {
		slug, name := ParseActorShorthand(tc.in)
		if slug != tc.wantSlug || name != tc.wantName {
			t.Errorf("ParseActorShorthand(%q) = %q,%q want %q,%q", tc.in, slug, name, tc.wantSlug, tc.wantName)
		}
	}
}

func TestValidateActor(t *testing.T) {
	if v, err := ValidateActor("", ""); err != nil || v != nil {
		t.Fatalf("clear = %+v %v, want nil,nil", v, err)
	}
	if _, err := ValidateActor("", "JustAName"); err == nil {
		t.Fatal("name without slug accepted")
	}
	if _, err := ValidateActor("  claude:354bff2b  ", " Claude "); err != nil {
		t.Fatalf("trim: %v", err)
	}
	if _, err := ValidateActor("Claude Code", ""); err == nil || !strings.Contains(err.Error(), "actor.name") {
		t.Fatalf("display-name slug accepted: %v", err)
	}
	long := strings.Repeat("a", 129)
	if _, err := ValidateActor(long, ""); err == nil || !strings.Contains(err.Error(), "128") {
		t.Fatalf("oversized slug accepted: %v", err)
	}
	if _, err := ValidateActor(strings.Repeat("a", 128), ""); err != nil {
		t.Fatalf("128-char slug rejected: %v", err)
	}
}

func TestSettingGetSetActor(t *testing.T) {
	s, ok := SettingByPath("actor")
	if !ok {
		t.Fatal("actor not in the settings catalog")
	}
	c := &Config{}
	if got := s.Get(c); got != (ActorConfig{}) {
		t.Fatalf("default get = %#v", got)
	}
	// Object form.
	if err := s.Set(c, json.RawMessage(`{"slug":"claude:354bff2b","name":"Claude Code"}`)); err != nil {
		t.Fatal(err)
	}
	if c.Actor == nil || c.Actor.Slug != "claude:354bff2b" || c.Actor.Name != "Claude Code" {
		t.Fatalf("stored %+v", c.Actor)
	}
	if got := s.Get(c); got != (ActorConfig{Slug: "claude:354bff2b", Name: "Claude Code"}) {
		t.Fatalf("get after set = %#v", got)
	}
	// Shorthand form, shared with GADAK_ACTOR.
	if err := s.Set(c, json.RawMessage(`"grok:aa11|Grok"`)); err != nil {
		t.Fatal(err)
	}
	if c.Actor.Slug != "grok:aa11" || c.Actor.Name != "Grok" {
		t.Fatalf("shorthand stored %+v", c.Actor)
	}
	// Clearing.
	if err := s.Set(c, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if c.Actor != nil {
		t.Fatalf("empty object left %+v", c.Actor)
	}
	// A display name in slug position is refused with the fix in the message.
	if err := s.Set(c, json.RawMessage(`{"slug":"Claude Code"}`)); err == nil || !strings.Contains(err.Error(), "actor.name") {
		t.Fatalf("display-name slug accepted: %v", err)
	}
}
