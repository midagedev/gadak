package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
)

// writeVerbs are the CLI writes whose help used to say "on Jira (needs a
// credential)" — true only for a connected workspace.
var writeVerbs = []string{"create", "comment", "attach", "edit", "transition", "assign"}

func TestWriteVerbHelpNotOnJiraNeedsCredential(t *testing.T) {
	for _, name := range writeVerbs {
		h, ok := helps[name]
		if !ok {
			t.Errorf("helps missing %q", name)
			continue
		}
		if strings.Contains(h.summary, "on Jira (needs a credential") {
			t.Errorf("%s summary still keys writes on Jira+credential: %q", name, h.summary)
		}
		if !strings.Contains(h.summary, "workspace origin") {
			t.Errorf("%s summary missing workspace origin: %q", name, h.summary)
		}
		if !strings.Contains(h.summary, "needs a credential") {
			t.Errorf("%s summary dropped connected credential requirement: %q", name, h.summary)
		}
		if !strings.Contains(h.summary, "standalone") {
			t.Errorf("%s summary missing standalone: %q", name, h.summary)
		}
	}
	if strings.Contains(usage, "Writing through to Jira (needs a credential)") {
		t.Errorf("top-level usage still keys writes on Jira+credential")
	}
}

func TestInitHelpNamesStandalone(t *testing.T) {
	if !strings.Contains(helps["init"].usage, "--standalone") {
		t.Errorf("init usage missing --standalone: %s", helps["init"].usage)
	}
	out := formatHelp("init", nil)
	if !strings.Contains(out, "--standalone") {
		t.Errorf("gadak help init missing --standalone:\n%s", out)
	}
	foundExample := false
	for _, ex := range helps["init"].examples {
		if strings.Contains(ex, "--standalone") {
			foundExample = true
			break
		}
	}
	if !foundExample {
		t.Errorf("init examples missing a --standalone line: %v", helps["init"].examples)
	}
	if !strings.Contains(usage, "--standalone") {
		t.Errorf("top-level usage missing --standalone:\n%s", usage)
	}
	if !strings.Contains(helps["init"].summary, "standalone") {
		t.Errorf("init summary does not cover standalone: %q", helps["init"].summary)
	}
}

func TestServeHelpSyncsStandaloneWithoutCredential(t *testing.T) {
	sum := helps["serve"].summary
	if strings.Contains(sum, "when a credential is configured") && !strings.Contains(sum, "standalone") {
		t.Errorf("serve summary still keys sync on credential only: %q", sum)
	}
	if !strings.Contains(sum, "standalone") {
		t.Errorf("serve summary missing standalone: %q", sum)
	}
	if strings.Contains(usage, "syncs by default when a credential is configured") &&
		!strings.Contains(usage, "standalone") {
		t.Errorf("top-level serve line still keys sync on credential only")
	}
}

func TestFormatTransitionIncludesStatusID(t *testing.T) {
	got := formatTransition(jira.Transition{
		ID:   "2",
		Name: "In Progress",
		To:   jira.Status{ID: "3", Name: "In Progress"},
	})
	want := "In Progress (id 2, → In Progress [status_id 3])"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	empty := formatTransition(jira.Transition{ID: "11", Name: "Triage"})
	if !strings.Contains(empty, "Triage (id 11") {
		t.Fatalf("missing to still formats: %q", empty)
	}
	if strings.Contains(empty, "status_id") {
		t.Fatalf("empty to.id must not invent status_id: %q", empty)
	}
}

func TestStatusJSONIncludesWorkspaceKind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Kind: config.KindStandalone}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var doc struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Kind != config.KindStandalone {
		t.Fatalf("kind = %q, want standalone; body %s", doc.Kind, out)
	}

	cfg.Kind = config.KindConnected
	cfg.Site = "https://example.invalid"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	out, err = capture(t, func() error { return cmdStatus([]string{"--json"}) })
	if err != nil {
		t.Fatalf("status --json connected: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if doc.Kind != config.KindConnected {
		t.Fatalf("kind = %q, want connected; body %s", doc.Kind, out)
	}
}

func TestProfilesJSONIncludesWorkspaceKind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Cleanup(func() { config.SetProfile("") })
	config.SetProfile("")

	seedNamedProfile(t, "", nil, 0, 0, false)
	def, err := config.LoadFor("")
	if err != nil {
		t.Fatal(err)
	}
	def.Kind = config.KindStandalone
	if err := def.Save(); err != nil {
		t.Fatal(err)
	}
	seedNamedProfile(t, "work", &config.Config{
		Site:  "https://work.example.invalid",
		Email: "work-user@example.invalid",
		Token: "test-token-work-never-real",
	}, 1, 0, true)

	out, err := capture(t, func() error { return cmdProfiles([]string{"--json"}) })
	if err != nil {
		t.Fatalf("profiles --json: %v\n%s", err, out)
	}
	var inv struct {
		Profiles []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	byName := map[string]string{}
	for _, p := range inv.Profiles {
		byName[p.Name] = p.Kind
	}
	if byName["default"] != config.KindStandalone {
		t.Errorf("default kind = %q, want standalone", byName["default"])
	}
	if byName["work"] != config.KindConnected {
		t.Errorf("work kind = %q, want connected", byName["work"])
	}
}

func TestInitStandaloneJSONPersistAndFillsMirror(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	out, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	})
	if err != nil {
		t.Fatalf("init --standalone --json: %v\n%s", err, out)
	}
	var doc struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Persist string `json:"persist"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("init json: %v\n%s", err, out)
	}
	if doc.Kind != config.KindStandalone {
		t.Fatalf("kind %q", doc.Kind)
	}
	wantPersist := origin.PersistPath(home)
	if doc.Persist != wantPersist {
		t.Fatalf("persist = %q, want %s", doc.Persist, wantPersist)
	}
	if _, err := os.Stat(doc.Persist); err != nil {
		t.Fatalf("persist file missing at named path: %v", err)
	}

	created, stderr, err := captureBoth(t, func() error {
		return cmdCreate([]string{"after standalone init"})
	})
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	if strings.Contains(stderr, "never finished a sync") {
		t.Fatalf("standalone init must fill the mirror so create is not stale: stderr %q", stderr)
	}
}

func TestInitStandaloneHumanPersistAndNextSteps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	out, err := capture(t, func() error {
		return cmdInit([]string{"--standalone"})
	})
	if err != nil {
		t.Fatalf("init --standalone: %v\n%s", err, out)
	}
	persist := origin.PersistPath(home)
	if !strings.Contains(out, persist) {
		t.Fatalf("human init missing persist path %s:\n%s", persist, out)
	}
	if !strings.Contains(out, "original") {
		t.Fatalf("human init must say the persist file is the original:\n%s", out)
	}
	if strings.Contains(out, "a few minutes") {
		t.Fatalf("standalone next-steps still claims a few minutes:\n%s", out)
	}
}
