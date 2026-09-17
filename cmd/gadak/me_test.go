package main

// GDK-1973: `gadak me` — a person says who they are on the built-in
// tracker. show prints the resolved identity (name, slug, kind, source,
// workspace); set writes the config actor block as a person through the
// settings registry's actor setter; clear removes the block. On a
// connected Jira/Linear workspace the account is the identity, so set is
// refused with one sentence.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// meHome is the built-in test home: fresh GADAK_HOME, no credentials, and
// — the part clearCredentialEnv does not cover — no actor of any rung, so
// the ladder starts empty however the host shell is set up.
func meHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	for _, k := range []string{"GADAK_ACTOR", "CLAUDECODE", "CLAUDE_CODE_SESSION_ID"} {
		t.Setenv(k, "")
	}
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
}

func TestMeShowNothingResolved(t *testing.T) {
	meHome(t)
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --local: %v", err)
	}
	out, err := capture(t, func() error { return cmdMe(nil) })
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	// The default user is named as the author, and the way to change that
	// is named with it.
	if !strings.Contains(out, "gadak me set") {
		t.Fatalf("me (nothing resolved) = %q; want the how-to-set sentence", out)
	}

	// --json is the same object a resolved rung answers, source "none".
	out, err = capture(t, func() error { return cmdMe([]string{"--json"}) })
	if err != nil {
		t.Fatalf("me --json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("me --json: %v\n%s", err, out)
	}
	if doc["source"] != "none" || doc["slug"] != "" {
		t.Fatalf("me --json (nothing resolved) = %v", doc)
	}
}

func TestMeShowEnvActorIsAnAgent(t *testing.T) {
	meHome(t)
	t.Setenv("GADAK_ACTOR", "claude:354bff2b|Claude (build 1)")
	out, err := capture(t, func() error { return cmdMe([]string{"--json"}) })
	if err != nil {
		t.Fatalf("me --json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["slug"] != "claude:354bff2b" || doc["source"] != "env" || doc["kind"] != "agent" {
		t.Fatalf("me --json (env actor) = %v", doc)
	}
}

func TestMeSetAndClearOnBuiltIn(t *testing.T) {
	meHome(t)
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --local: %v", err)
	}

	if _, err := capture(t, func() error { return cmdMe([]string{"set", "Kim", "Cheolsu"}) }); err != nil {
		t.Fatalf("me set: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Actor == nil || cfg.Actor.Slug != "person:kim-cheolsu" || cfg.Actor.Kind != config.ActorKindPerson || cfg.Actor.Name != "Kim Cheolsu" {
		t.Fatalf("stored actor = %+v", cfg.Actor)
	}

	// Show reads it back, all five fields.
	out, err := capture(t, func() error { return cmdMe(nil) })
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	for _, want := range []string{"person:kim-cheolsu", "Kim Cheolsu", "person", "config"} {
		if !strings.Contains(out, want) {
			t.Fatalf("me after set = %q; want %q in it", out, want)
		}
	}

	// status carries the kind beside the source, human and JSON.
	out, err = capture(t, func() error { return cmdStatus(nil) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "(config · person)") {
		t.Fatalf("status actor line = %q; want the kind beside the source", out)
	}
	out, err = capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var st struct {
		Actor struct {
			Slug string `json:"slug"`
			Kind string `json:"kind"`
		} `json:"actor"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatal(err)
	}
	if st.Actor.Slug != "person:kim-cheolsu" || st.Actor.Kind != "person" {
		t.Fatalf("status --json actor = %+v", st.Actor)
	}

	// Clear removes the block, same as `config set actor ""`.
	if _, err := capture(t, func() error { return cmdMe([]string{"clear"}) }); err != nil {
		t.Fatalf("me clear: %v", err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Actor != nil {
		t.Fatalf("actor block survived clear: %+v", cfg.Actor)
	}
	out, err = capture(t, func() error { return cmdMe(nil) })
	if err != nil {
		t.Fatalf("me after clear: %v", err)
	}
	if !strings.Contains(out, "gadak me set") {
		t.Fatalf("me after clear = %q; want the how-to-set sentence back", out)
	}
}

func TestMeSetRefusedOnJira(t *testing.T) {
	meHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Site = "https://example.atlassian.net"
	cfg.Email = "you@example.com"
	cfg.Token = "t"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, _, err = captureBoth(t, func() error { return cmdMe([]string{"set", "Kim"}) })
	if err == nil {
		t.Fatal("me set on a connected Jira workspace was accepted")
	}
	if !strings.Contains(err.Error(), "the account is the identity") {
		t.Fatalf("me set refusal = %v; want the one-sentence account-is-the-identity reason", err)
	}
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Actor != nil {
		t.Fatalf("refused set still wrote a block: %+v", cfg.Actor)
	}
}

func TestMeSetNeedsAName(t *testing.T) {
	meHome(t)
	if _, err := capture(t, func() error { return cmdMe([]string{"set"}) }); err == nil {
		t.Fatal("me set with no name was accepted")
	}
	if _, err := capture(t, func() error { return cmdMe([]string{"set", "   "}) }); err == nil {
		t.Fatal("me set with a blank name was accepted")
	}
}
