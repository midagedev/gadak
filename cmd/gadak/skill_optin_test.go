package main

// GDK-2070 — creating the Claude Code skill is consent-gated (GitHub #113).
//
// `gadak init` (every mode), pairing and `gadak install-cli` used to write
// ~/.claude/skills/gadak/SKILL.md whenever ~/.claude existed, without asking.
// Claude Code loads every installed skill's description into every session on
// the machine, so the write changes agent behaviour in unrelated projects, and
// a user who removed the file by hand got it back on the next init. The
// consent point this file pins is creating a copy that does not exist:
// refreshing a copy gadak wrote stays as it was (the user consented when it
// was created), and `gadak skill install` stays the deliberate verb.
//
// The opt-out has three forms — the --no-skill flag, GADAK_SKILL_AUTO=0, and
// the machine-wide skill.autoInstall setting — and all of them must reach
// every automatic path: init (all modes), pairing, install-cli, and the daily
// stale-copy sync.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/pairing"
)

// runBuiltInInitErr runs `gadak init --local` with extra flags on the root
// profile and returns stdout, stderr and the command error — the captureErr
// twin of runBuiltInInitJSON (init_test.go), which has no stderr.
func runBuiltInInitErr(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runBuiltInInitProfileErr(t, "", args...)
}

// runBuiltInInitProfileErr is runBuiltInInitErr for a named workspace, so the
// cross-workspace half of the opt-out can be driven the way a user drives it.
func runBuiltInInitProfileErr(t *testing.T, profile string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	clearCredentialEnv(t)
	config.SetProfile(profile)
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	return captureErr(t, func() error {
		return cmdInit(append([]string{"--local"}, args...))
	})
}

// setSkillNoSkillForTest flips the --no-skill flag var with restore, for the
// tests that drive installCLI directly (its signature has no flag param).
func setSkillNoSkillForTest(t *testing.T, on bool) {
	t.Helper()
	prev := skillNoSkillFlag
	skillNoSkillFlag = on
	t.Cleanup(func() { skillNoSkillFlag = prev })
}

// TestInitNonInteractiveCreatesNoSkill is the headline contract: a
// non-interactive init on a machine with ~/.claude and no skill creates no
// file, says so once on stderr, and reports "skipped" in --json.
//
// FAIL-first (2026-11-03, pre-fix): init succeeded, SKILL.md was created, and
// the JSON carried "skill":"installed" with no stderr line.
func TestInitNonInteractiveCreatesNoSkill(t *testing.T) {
	// Pinned off the GADAK_HOME axis (GDK-1611): this test pins init's
	// consent gate, a real (non-scratch) run. The GADAK_HOME below is the
	// config home's isolation, not a scratch-workspace statement.
	pinSkillHomeNotIsolated(t)
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", home)

	out, stderr, err := runBuiltInInitErr(t, "--json")
	if err != nil {
		t.Fatalf("init --local --json: %v\nstdout=%s\nstderr=%s", err, out, stderr)
	}
	skill, _ := initJSONSkill(t, out)
	if skill != "skipped" {
		t.Fatalf("skill = %q, want skipped (non-interactive init must not install); out=%s", skill, out)
	}
	if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
		t.Fatalf("non-interactive init created the skill: %v", serr)
	}
	if n := strings.Count(stderr, "skill: not installed"); n != 1 {
		t.Fatalf("the skip must be named exactly once on stderr (%d times); got:\n%s", n, stderr)
	}
	if !strings.Contains(stderr, "run gadak skill install") {
		t.Fatalf("the hint must name the deliberate command; got:\n%s", stderr)
	}
}

// TestInitInteractiveYesInstalls — an interactive run that answers y gets the
// skill, exactly as an unasked run did before GDK-2070.
func TestInitInteractiveYesInstalls(t *testing.T) {
	pinSkillHomeNotIsolated(t)
	prompt := skillInteractiveAnswer(t, "y\n")
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", home)

	out, _, err := runBuiltInInitErr(t)
	if err != nil {
		t.Fatalf("init --local: %v\n%s", err, out)
	}
	if !strings.Contains(prompt.String(), "Install the gadak skill for Claude Code into") ||
		!strings.Contains(prompt.String(), "[Y/n]") {
		t.Fatalf("the consent question was not asked; prompt:\n%s", prompt.String())
	}
	got, rerr := os.ReadFile(skillDestUnder(home))
	if rerr != nil {
		t.Fatalf("an answered-yes install did not land: %v", rerr)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatalf("installed bytes differ from embed (%d vs %d)", len(got), len(gadak.SkillMarkdown()))
	}
}

// TestInitInteractiveNoRecordsOptOutAcrossWorkspaces — answering n installs
// nothing, records the machine-wide opt-out, says the exact undo command, and
// a later interactive init of a DIFFERENT workspace neither prompts nor
// installs. That last half is the reason the setting is machine-wide rather
// than per-workspace config.json.
func TestInitInteractiveNoRecordsOptOutAcrossWorkspaces(t *testing.T) {
	pinSkillHomeNotIsolated(t)
	skillInteractiveAnswer(t, "n\n")
	home := isolateHomeWithClaude(t)
	gadakHome := t.TempDir()
	t.Setenv("GADAK_HOME", gadakHome)

	_, stderr, err := runBuiltInInitErr(t)
	if err != nil {
		t.Fatalf("init --local: %v\nstderr=%s", err, stderr)
	}
	if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
		t.Fatalf("a declined install still created the skill: %v", serr)
	}
	if !strings.Contains(stderr, "gadak config set skill.autoInstall true") {
		t.Fatalf("the decline must print the exact undo command; got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "gadak skill install") {
		t.Fatalf("the decline must say the deliberate verb still works; got:\n%s", stderr)
	}
	if config.SkillAutoInstallEnabled() {
		t.Fatal("answering n did not record the machine-wide opt-out")
	}
	// The recorded opt-out lives at the gadak home root, beside the daily
	// sync's rate-limit stamp — not inside any workspace's config.json.
	raw, rerr := os.ReadFile(gadakHome + string(os.PathSeparator) + "machine-settings.json")
	if rerr != nil {
		t.Fatalf("no machine-settings.json at the gadak home root: %v", rerr)
	}
	if !strings.Contains(string(raw), `"skill.autoInstall": false`) {
		t.Fatalf("machine-settings.json does not carry the opt-out:\n%s", raw)
	}

	// A different workspace, interactive again: no prompt, no install.
	next := skillInteractiveAnswer(t, "y\n")
	out, stderr2, err := runBuiltInInitProfileErr(t, "fresh-ws")
	if err != nil {
		t.Fatalf("init --workspace fresh-ws: %v\nstdout=%s\nstderr=%s", err, out, stderr2)
	}
	if next.Len() != 0 {
		t.Fatalf("an opted-out machine still prompted:\n%s", next.String())
	}
	if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
		t.Fatalf("an opted-out machine still installed the skill: %v", serr)
	}
}

// TestSkillAutoOptOutFlagEnvAndSetting — each of the three opt-out forms,
// tested separately, stops the automatic install silently: no write, no
// prompt, no line; --json still reports "skipped" (no new enum value).
func TestSkillAutoOptOutFlagEnvAndSetting(t *testing.T) {
	pinSkillHomeNotIsolated(t)

	t.Run("flag", func(t *testing.T) {
		home := isolateHomeWithClaude(t)
		t.Setenv("GADAK_HOME", home)
		out, stderr, err := runBuiltInInitErr(t, "--no-skill", "--json")
		if err != nil {
			t.Fatalf("init --no-skill: %v\nstdout=%s\nstderr=%s", err, out, stderr)
		}
		if skill, _ := initJSONSkill(t, out); skill != "skipped" {
			t.Fatalf("skill = %q, want skipped; out=%s", skill, out)
		}
		if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
			t.Fatalf("--no-skill wrote the skill: %v", serr)
		}
		if strings.Contains(stderr, "skill") || strings.Contains(out, "skill: ") {
			t.Fatalf("--no-skill must say nothing about the skill; stderr:\n%s\nstdout:\n%s", stderr, out)
		}
	})

	t.Run("env", func(t *testing.T) {
		home := isolateHomeWithClaude(t)
		t.Setenv("GADAK_HOME", home)
		t.Setenv("GADAK_SKILL_AUTO", "0")
		prompt := skillInteractiveAnswer(t, "y\n") // would install if it asked
		out, stderr, err := runBuiltInInitErr(t, "--json")
		if err != nil {
			t.Fatalf("init under GADAK_SKILL_AUTO=0: %v\nstdout=%s\nstderr=%s", err, out, stderr)
		}
		if skill, _ := initJSONSkill(t, out); skill != "skipped" {
			t.Fatalf("skill = %q, want skipped; out=%s", skill, out)
		}
		if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
			t.Fatalf("GADAK_SKILL_AUTO=0 wrote the skill: %v", serr)
		}
		if prompt.Len() != 0 {
			t.Fatalf("GADAK_SKILL_AUTO=0 still prompted:\n%s", prompt.String())
		}
	})

	t.Run("setting", func(t *testing.T) {
		home := isolateHomeWithClaude(t)
		gadakHome := t.TempDir()
		t.Setenv("GADAK_HOME", gadakHome)
		if err := config.SetSkillAutoInstall(false); err != nil {
			t.Fatal(err)
		}
		prompt := skillInteractiveAnswer(t, "y\n")
		out, _, err := runBuiltInInitProfileErr(t, "brand-new-ws", "--json")
		if err != nil {
			t.Fatalf("init for a new workspace under the opt-out: %v\n%s", err, out)
		}
		if skill, _ := initJSONSkill(t, out); skill != "skipped" {
			t.Fatalf("skill = %q, want skipped; out=%s", skill, out)
		}
		if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
			t.Fatalf("the machine-wide opt-out wrote the skill for a new workspace: %v", serr)
		}
		if prompt.Len() != 0 {
			t.Fatalf("the machine-wide opt-out still prompted:\n%s", prompt.String())
		}
	})
}

// TestSkillAutoOptOutReachesInstallCLIAndSync — the same opt-out must reach
// the two paths that are not init: install-cli's automatic install and the
// daily stale-copy sync.
func TestSkillAutoOptOutReachesInstallCLIAndSync(t *testing.T) {
	pinSkillHomeNotIsolated(t)
	home := isolateHomeWithClaude(t)
	gadakHome := t.TempDir()
	t.Setenv("GADAK_HOME", gadakHome)
	if err := config.SetSkillAutoInstall(false); err != nil {
		t.Fatal(err)
	}

	// install-cli's automatic install (driven at installCLI level with the
	// flag var, the seam its pinned signature leaves).
	setSkillNoSkillForTest(t, true)
	source, dir := seedInstallCLI(t)
	var out bytes.Buffer
	_, stderr, err := captureErr(t, func() error {
		return installCLI(&out, source, dir, false, false, dir, "zsh", "linux")
	})
	setSkillNoSkillForTest(t, false)
	if err != nil {
		t.Fatalf("installCLI: %v", err)
	}
	if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
		t.Fatalf("an opted-out install-cli wrote the skill: %v", serr)
	}
	if strings.Contains(stderr, "skill") || strings.Contains(out.String(), "skill: ") {
		t.Fatalf("--no-skill on install-cli must say nothing about the skill; stderr:\n%s\nstdout:\n%s", stderr, out.String())
	}

	// The daily sync: a stale gadak-written copy that would refresh, doesn't.
	releaseVersionForTest(t)
	dest := skillDestUnder(home)
	autoSyncSeedStaleCopy(t, dest)
	var syncBuf bytes.Buffer
	maybeAutoSyncSkill(&syncBuf, "version")
	got, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatal("the daily sync refreshed a stale copy on an opted-out machine")
	}
	if strings.Contains(syncBuf.String(), "skill:") {
		t.Fatalf("the opted-out sync must be silent; got:\n%s", syncBuf.String())
	}
}

// TestSkillStaleCopyStillRefreshesWithoutAsking — the consent point is
// creating a copy that does not exist. A copy gadak wrote and that has fallen
// behind refreshes on a non-interactive run exactly as before GDK-2070: the
// user consented when it was created. The second half arms a "n" answer and
// proves an interactive run is not consulted either — a stale copy is not a
// question.
func TestSkillStaleCopyStillRefreshesWithoutAsking(t *testing.T) {
	releaseVersionForTest(t)
	pinSkillHomeNotIsolated(t)
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", home)
	dest := skillDestUnder(home)
	autoSyncSeedStaleCopy(t, dest)

	out, _, err := runBuiltInInitErr(t, "--json")
	if err != nil {
		t.Fatalf("init --local --json: %v\n%s", err, out)
	}
	if skill, _ := initJSONSkill(t, out); skill != "installed" {
		t.Fatalf("skill = %q, want installed (a stale gadak copy refreshes); out=%s", skill, out)
	}
	got, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if !bytes.Equal(got, gadak.SkillMarkdown()) {
		t.Fatal("the stale copy was not brought current")
	}

	// Interactive run, armed with a decline: still no question, still current.
	armed := skillInteractiveAnswer(t, "n\n")
	autoSyncSeedStaleCopy(t, dest)
	var buf bytes.Buffer
	if got := autoInstallSkill(&buf, skillAutoOptions{}); got != "installed" {
		t.Fatalf("status = %q, want installed; buf=%q", got, buf.String())
	}
	if armed.Len() != 0 {
		t.Fatalf("refreshing a stale copy must not ask;\n%s", armed.String())
	}
}

// TestInitPairedOptOutWritesNothing — the pairing path is the fifth caller
// of the policy; under the machine-wide opt-out it writes nothing and does
// not ask, and --json still carries "skipped".
func TestInitPairedOptOutWritesNothing(t *testing.T) {
	pinSkillHomeNotIsolated(t)
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	t.Setenv("GADAK_SKILL_AUTO", "0")
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	srv, _ := pairingOriginServe(t, http.StatusOK)
	offer := mustOffer(t, pairing.Offer{
		V: pairing.OfferV1, Endpoint: srv.URL, Token: "pair-token-optout", Label: "laptop",
	})
	prompt := skillInteractiveAnswer(t, "y\n") // would install if it asked
	out, stderr, err := captureErr(t, func() error {
		return cmdInit([]string{"--pairing-code", offer, "--json"})
	})
	if err != nil {
		t.Fatalf("init --pairing-code: %v\nstdout=%s\nstderr=%s", err, out, stderr)
	}
	if skill, _ := initJSONSkill(t, out); skill != "skipped" {
		t.Fatalf("skill = %q, want skipped; out=%s", skill, out)
	}
	if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
		t.Fatalf("an opted-out paired init created the skill: %v", serr)
	}
	if prompt.Len() != 0 {
		t.Fatalf("an opted-out paired init still prompted:\n%s", prompt.String())
	}
}

// TestSkillExplicitInstallIgnoresOptOut — `gadak skill install` is the
// deliberate verb; the opt-out never takes it away (it goes through
// installSkill, never the consent gate).
func TestSkillExplicitInstallIgnoresOptOut(t *testing.T) {
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	if err := config.SetSkillAutoInstall(false); err != nil {
		t.Fatal(err)
	}
	dest := skillDestUnder(home)
	if err := installSkill(io.Discard, gadak.SkillMarkdown(), dest, false, false); err != nil {
		t.Fatalf("explicit install under the opt-out: %v", err)
	}
	if _, serr := os.Stat(dest); serr != nil {
		t.Fatalf("the deliberate install must land: %v", serr)
	}
}

// TestSkillConsentPromptEdges — EOF declines without recording (the next
// interactive run asks again), and an answer that is neither yes nor no is
// asked exactly once more and then declines without recording either.
func TestSkillConsentPromptEdges(t *testing.T) {
	pinSkillHomeNotIsolated(t)

	t.Run("eof", func(t *testing.T) {
		home := isolateHomeWithClaude(t)
		t.Setenv("GADAK_HOME", t.TempDir())
		skillInteractiveAnswer(t, "") // reader with nothing in it
		var buf bytes.Buffer
		if got := autoInstallSkill(&buf, skillAutoOptions{}); got != "skipped" {
			t.Fatalf("status = %q, want skipped", got)
		}
		if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
			t.Fatalf("EOF at the prompt installed the skill: %v", serr)
		}
		if config.SkillAutoInstallEnabled() != true {
			t.Fatal("EOF recorded an opt-out the user never gave")
		}
		// And the next interactive run asks again.
		next := skillInteractiveAnswer(t, "y\n")
		var buf2 bytes.Buffer
		if got := autoInstallSkill(&buf2, skillAutoOptions{}); got != "installed" {
			t.Fatalf("status = %q, want installed (EOF must not opt out); buf=%q", got, buf2.String())
		}
		if !strings.Contains(next.String(), "[Y/n]") {
			t.Fatalf("the second run did not ask again;\n%s", next.String())
		}
	})

	t.Run("garbage-then-anything", func(t *testing.T) {
		home := isolateHomeWithClaude(t)
		t.Setenv("GADAK_HOME", t.TempDir())
		prompt := skillInteractiveAnswer(t, "perhaps\ny\n")
		var buf bytes.Buffer
		if got := autoInstallSkill(&buf, skillAutoOptions{}); got != "skipped" {
			t.Fatalf("status = %q, want skipped (an unreadable answer never installs)", got)
		}
		if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
			t.Fatalf("an unreadable answer installed the skill: %v", serr)
		}
		if n := strings.Count(prompt.String(), "[Y/n]"); n != 2 {
			t.Fatalf("the gate must ask exactly twice on garbage (%d times);\n%s", n, prompt.String())
		}
		if config.SkillAutoInstallEnabled() != true {
			t.Fatal("a garbage answer recorded an opt-out the user never gave")
		}
	})
}

// TestSkillInteractiveDefinition — the prompt runs only when both stdio are
// terminals, --json is off, and this run has not already consumed stdin.
// --json and a consumed stdin each veto it even on a full TTY.
func TestSkillInteractiveDefinition(t *testing.T) {
	skillInteractiveAnswer(t, "y\n") // seams on for every case below
	home := isolateHomeWithClaude(t)
	t.Setenv("GADAK_HOME", t.TempDir())

	cases := []struct {
		name string
		opts skillAutoOptions
		want bool
	}{
		{"plain", skillAutoOptions{}, true},
		{"json", skillAutoOptions{jsonOut: true}, false},
		{"stdin used", skillAutoOptions{stdinUsed: true}, false},
		{"both", skillAutoOptions{jsonOut: true, stdinUsed: true}, false},
	}
	for _, c := range cases {
		if got := c.opts.interactive(); got != c.want {
			t.Errorf("%s: interactive = %v, want %v", c.name, got, c.want)
		}
	}
	// --json vetoes the prompt through cmdInit too, not just the struct.
	prompt := skillInteractiveAnswer(t, "y\n")
	out, _, err := runBuiltInInitErr(t, "--json")
	if err != nil {
		t.Fatalf("init --local --json: %v\n%s", err, out)
	}
	if prompt.Len() != 0 {
		t.Fatalf("--json must not prompt;\n%s", prompt.String())
	}
	if _, serr := os.Stat(skillDestUnder(home)); !os.IsNotExist(serr) {
		t.Fatalf("--json with a TTY still installed the skill: %v", serr)
	}
}

// TestSkillAutoInstallSettingRoundTrip — the machine-wide switch through the
// settings catalog (`gadak config set/get/list`): default true, false
// persists at the gadak home root, true returns to the default by removing
// the key, and the catalog lists the path.
func TestSkillAutoInstallSettingRoundTrip(t *testing.T) {
	gadakHome := t.TempDir()
	t.Setenv("GADAK_HOME", gadakHome)

	s, ok := config.SettingByPath(config.SkillAutoInstallPath)
	if !ok {
		t.Fatalf("%s is not in the settings catalog", config.SkillAutoInstallPath)
	}
	if got := s.Get(nil); got != true {
		t.Fatalf("default get = %v, want true", got)
	}
	if err := s.Set(nil, json.RawMessage("false")); err != nil {
		t.Fatalf("set false: %v", err)
	}
	if got := s.Get(nil); got != false {
		t.Fatalf("get after set false = %v, want false", got)
	}
	raw, rerr := os.ReadFile(gadakHome + string(os.PathSeparator) + "machine-settings.json")
	if rerr != nil {
		t.Fatalf("the opt-out must persist at the gadak home root: %v", rerr)
	}
	if !strings.Contains(string(raw), `"skill.autoInstall": false`) {
		t.Fatalf("machine-settings.json content:\n%s", raw)
	}
	// A non-bool refuses through the shared decoder.
	if err := s.Set(nil, json.RawMessage(`"no"`)); err == nil {
		t.Fatal("a non-bool value must refuse")
	}
	// true is the default: the key leaves the file, not an explicit on.
	if err := s.Set(nil, json.RawMessage("true")); err != nil {
		t.Fatalf("set true: %v", err)
	}
	if got := s.Get(nil); got != true {
		t.Fatalf("get after set true = %v, want true", got)
	}
	if _, serr := os.Stat(gadakHome + string(os.PathSeparator) + "machine-settings.json"); !os.IsNotExist(serr) {
		raw, _ := os.ReadFile(gadakHome + string(os.PathSeparator) + "machine-settings.json")
		t.Fatalf("returning to the default must leave nothing stored:\n%s", raw)
	}
	// `gadak config list` shows one row per fact — the path must be on it.
	listed := false
	for _, p := range config.SettingPaths() {
		if p == config.SkillAutoInstallPath {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("config list does not carry %s", config.SkillAutoInstallPath)
	}
}
