package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// GDK-1973: a person can say who they are on the built-in tracker. The slug
// is derived from the name they typed, by one owner — PersonSlug — and
// ValidateActor applies the person rule: a person block with no slug gets
// the derived one; a person with no name is refused (nothing to derive
// from, nothing to display). Nothing else derives slugs.

func TestPersonSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Kim", "person:kim"},
		{"  Kim  ", "person:kim"},
		{"Dana Whitfield", "person:dana-whitfield"},
		{"Dana \t\n Whitfield", "person:dana-whitfield"}, // one dash per whitespace run
		{"김현철", "person:김현철"},                            // non-ASCII passes through unchanged
		{"YAMADA Taro", "person:yamada-taro"},            // ASCII upper folds; nothing else does
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := PersonSlug(c.in); got != c.want {
			t.Errorf("PersonSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// The cap is the actor slug cap and it cuts on a rune boundary, so the
	// result is valid UTF-8 under the byte cap ValidateActor enforces.
	long := PersonSlug(strings.Repeat("a", 300))
	if want := "person:" + strings.Repeat("a", maxActorSlugLen-len("person:")); long != want {
		t.Fatalf("PersonSlug(300 a's) = %q, want %q", long, want)
	}
	cjk := PersonSlug(strings.Repeat("철", 100))
	if len(cjk) > maxActorSlugLen || !strings.HasPrefix(cjk, "person:") {
		t.Fatalf("PersonSlug(100 Hangul) = %q (len %d), want at most %d bytes", cjk, len(cjk), maxActorSlugLen)
	}
}

func TestValidateActorPersonRule(t *testing.T) {
	// A person with only a name: the slug is derived here, once.
	a, err := ValidateActor("", "Kim Cheolsu", ActorKindPerson)
	if err != nil || a == nil {
		t.Fatalf("person with name only: %v", err)
	}
	if a.Slug != "person:kim-cheolsu" || a.Name != "Kim Cheolsu" || a.Kind != ActorKindPerson {
		t.Fatalf("derived block = %+v", a)
	}

	// A person with no name is refused — the slug would be a bare prefix
	// and the origin would show an empty display name.
	if _, err := ValidateActor("", "  ", ActorKindPerson); err == nil || !strings.Contains(err.Error(), "a person needs a display name") {
		t.Fatalf("person without a name: err = %v", err)
	}

	// An explicit slug is kept: derivation fills in only what was absent.
	if a, err = ValidateActor("person:kim", "Kim", ActorKindPerson); err != nil || a.Slug != "person:kim" {
		t.Fatalf("explicit person slug: %+v %v", a, err)
	}

	// Agents keep the old rule: a slug is an identity you already have.
	if _, err := ValidateActor("", "Kim", ActorKindAgent); err == nil {
		t.Fatal("agent with name only was accepted; the slug must stay explicit")
	}

	// Garbage kind is refused at the same door the value entered.
	if _, err := ValidateActor("kim", "Kim", "human"); err == nil || !strings.Contains(err.Error(), `actor.kind must be "agent" or "person"`) {
		t.Fatalf("garbage kind: err = %v", err)
	}

	// "" is the old block, byte for byte.
	if a, err = ValidateActor("claude:354bff2b", "Claude Code", ""); err != nil || a.Slug != "claude:354bff2b" || a.Kind != "" {
		t.Fatalf("agent default: %+v %v", a, err)
	}
}

// The actor setting round-trips a person: `gadak me set` writes {name,
// kind: person} through this same setter, and ResolveActor reports the kind.
func TestActorSettingPersonRoundTrip(t *testing.T) {
	s, ok := SettingByPath("actor")
	if !ok {
		t.Fatal("no actor setting")
	}
	cfg := &Config{}
	if err := s.Set(cfg, json.RawMessage(`{"name":"Kim Cheolsu","kind":"person"}`)); err != nil {
		t.Fatalf("set person: %v", err)
	}
	if cfg.Actor == nil || cfg.Actor.Slug != "person:kim-cheolsu" || cfg.Actor.Kind != ActorKindPerson {
		t.Fatalf("stored block = %+v", cfg.Actor)
	}
	a, ok := ResolveActor(cfg)
	if !ok || a.Kind != ActorKindPerson || a.Slug != "person:kim-cheolsu" {
		t.Fatalf("resolved = %+v ok=%v", a, ok)
	}
}

func TestResolveActorKindDefaults(t *testing.T) {
	// A config block without a kind is the agent block it always was.
	cfg := &Config{Actor: &ActorConfig{Slug: "claude:1", Name: "Claude"}}
	if a, _ := ResolveActor(cfg); a.Kind != ActorKindAgent {
		t.Fatalf("kindless block resolved kind %q, want agent", a.Kind)
	}
	// Garbage normalizes to agent rather than forking a third kind.
	cfg = &Config{Actor: &ActorConfig{Slug: "x", Kind: "human"}}
	if a, _ := ResolveActor(cfg); a.Kind != ActorKindAgent {
		t.Fatalf("garbage kind resolved %q, want agent", a.Kind)
	}
	// The env rung is an agent by construction.
	t.Setenv("GADAK_ACTOR", "grok:aa11")
	a, ok := ResolveActor(nil)
	if !ok || a.Kind != ActorKindAgent {
		t.Fatalf("env rung kind %q ok=%v, want agent", a.Kind, ok)
	}
}
