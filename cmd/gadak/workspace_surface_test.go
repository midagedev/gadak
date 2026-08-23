package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
)

// writeVerbs are the CLI writes whose --help first line used to repeat the
// origin-policy paragraph (GDK-469).
var writeVerbs = []string{"create", "comment", "attach", "edit", "transition", "assign"}

// TestWriteVerbHelpFirstLinesAreTheVerb is GDK-469: each write verb's first
// help line is what that verb does. The origin-policy sentence lives once in
// top-level usage, not cloned across six summaries.
func TestWriteVerbHelpFirstLinesAreTheVerb(t *testing.T) {
	seen := map[string]string{}
	for _, name := range writeVerbs {
		h, ok := helps[name]
		if !ok {
			t.Errorf("helps missing %q", name)
			continue
		}
		if strings.Contains(h.summary, writeThroughOriginPhrase) {
			t.Errorf("%s summary still clones the origin-policy paragraph: %q", name, h.summary)
		}
		if strings.Contains(h.summary, "workspace origin") {
			t.Errorf("%s summary still carries origin policy: %q", name, h.summary)
		}
		first := firstLine(formatHelp(name, nil))
		if prev, ok := seen[first]; ok {
			t.Errorf("%s and %s share a --help first line: %q", prev, name, first)
		}
		seen[first] = name
	}
	if !strings.Contains(usage, writeThroughOriginPhrase) {
		t.Errorf("top-level usage dropped the origin-policy sentence")
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
	got := jira.FormatTransition(jira.Transition{
		ID:   "2",
		Name: "In Progress",
		To:   jira.Status{ID: "3", Name: "In Progress"},
	})
	want := "In Progress (id 2, → In Progress [status_id 3])"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	empty := jira.FormatTransition(jira.Transition{ID: "11", Name: "Triage"})
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
	if _, err := os.Stat(origin.LegacyYAMLPath(home)); !os.IsNotExist(err) {
		t.Fatalf("init --standalone must not write %s: %v", origin.LegacyYAMLPath(home), err)
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
	if strings.Contains(out, "this account") {
		t.Fatalf("standalone init still uses a Jira-account sentence:\n%s", out)
	}
	// GDK-465: skill-first next; sync already filled the mirror; MCP is secondary.
	next := out
	if i := strings.Index(out, "next:"); i >= 0 {
		next = out[i:]
	} else {
		t.Fatalf("standalone init missing next:\n%s", out)
	}
	if !strings.Contains(next, "gadak create") {
		t.Fatalf("standalone next missing create:\n%s", out)
	}
	if !strings.Contains(next, "gadak serve") {
		t.Fatalf("standalone next missing serve:\n%s", out)
	}
	if !strings.Contains(next, "gadak skill install") {
		t.Fatalf("standalone next missing skill install:\n%s", out)
	}
	if strings.Contains(next, "gadak sync") {
		t.Fatalf("standalone next still tells you to sync a filled mirror:\n%s", out)
	}
	// GDK-482: name the default author. No CLI verb changes it (measured:
	// config list has no display-name path; issuetap seeds the fixture user).
	if !strings.Contains(out, "authored as") {
		t.Fatalf("standalone init must name the default author:\n%s", out)
	}
	reporter, err := capture(t, func() error {
		return cmdSQL([]string{"--no-header", "select reporter from issues_full where reporter is not null and reporter != '' limit 1"})
	})
	if err != nil {
		t.Fatalf("sql reporter: %v\n%s", err, reporter)
	}
	if name := strings.TrimSpace(reporter); name != "" && !strings.Contains(out, name) {
		t.Fatalf("standalone init must name default author %q:\n%s", name, out)
	}

	// GDK-465: re-init of an already-standalone home is one line.
	again, err := capture(t, func() error {
		return cmdInit([]string{"--standalone"})
	})
	if err != nil {
		t.Fatalf("reinit --standalone: %v\n%s", err, again)
	}
	if !strings.Contains(again, "already standalone at") {
		t.Fatalf("reinit missing already-standalone line:\n%s", again)
	}
	if !strings.Contains(again, persist) {
		t.Fatalf("reinit must name persist %s:\n%s", persist, again)
	}
	if strings.Contains(again, "next:") {
		t.Fatalf("reinit still prints first-run next:\n%s", again)
	}
}

// TestStandaloneSyncCopyOmitsAccount is GDK-464: a workspace with no Jira
// account must not talk about "this account", Jira filters, or custom-field
// discovery tips.
func TestStandaloneSyncCopyOmitsAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	if _, err := capture(t, func() error {
		return cmdInit([]string{"--standalone", "--json"})
	}); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	stdout, err := capture(t, func() error { return cmdSync(nil) })
	if err != nil {
		t.Fatalf("sync: %v\n%s\n%s", err, stdout, logs.String())
	}
	combined := logs.String() + stdout
	for _, needle := range []string{"this account", "from Jira", "auto-configure custom fields"} {
		if strings.Contains(combined, needle) {
			t.Errorf("standalone sync still has %q:\n%s", needle, combined)
		}
	}
}
