package skillinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testEnv is the whole point of Env being a parameter: a destination table
// exercised against a literal can never resolve into the developer's home, and
// no test has to set CODEX_HOME to stay contained. orca's tripwire records that
// the Codex binary ignores the USERPROFILE sandbox on Windows, so CODEX_HOME is
// a preference to honour, never a fence to hide behind.
func testEnv(home, cwd string, env map[string]string) Env {
	return Env{
		Home:   home,
		Cwd:    cwd,
		Getenv: func(k string) string { return env[k] },
	}
}

func TestHomeDestForEveryClient(t *testing.T) {
	home := filepath.Join("/h", "user")
	env := testEnv(home, "/w/proj", nil)

	want := map[string]string{
		"claude":   filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md"),
		"codex":    filepath.Join(home, ".codex", "skills", "gadak", "SKILL.md"),
		"agents":   filepath.Join(home, ".agents", "skills", "gadak", "SKILL.md"),
		"cursor":   filepath.Join(home, ".cursor", "skills", "gadak", "SKILL.md"),
		"gemini":   filepath.Join(home, ".gemini", "skills", "gadak", "SKILL.md"),
		"opencode": filepath.Join(home, ".config", "opencode", "skills", "gadak", "SKILL.md"),
		"grok":     filepath.Join(home, ".grok", "skills", "gadak", "SKILL.md"),
	}
	if len(want) != len(clients) {
		t.Fatalf("table has %d clients, test covers %d — a new host must land here too", len(clients), len(want))
	}
	for _, c := range Clients() {
		got, err := c.HomeDest(env)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if got != want[c.Name] {
			t.Errorf("%s home dest = %q, want %q", c.Name, got, want[c.Name])
		}
	}
}

// TestCodexHomeEnvOverride — Codex's own installer writes under $CODEX_HOME and
// the binary reads it, so the override is the destination when it is set. The
// lookup is injected: nothing here touches the process environment.
func TestCodexHomeEnvOverride(t *testing.T) {
	codex, ok := Lookup("codex")
	if !ok {
		t.Fatal("no codex client")
	}

	custom := t.TempDir()
	got, err := codex.HomeDest(testEnv("/h/user", "/w", map[string]string{"CODEX_HOME": custom}))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(custom, "skills", "gadak", "SKILL.md"); got != want {
		t.Errorf("with CODEX_HOME: %q, want %q", got, want)
	}

	// An empty or whitespace value is not an override — it is an unset
	// variable that a shell exported anyway.
	for _, v := range []string{"", "   "} {
		got, err = codex.HomeDest(testEnv("/h/user", "/w", map[string]string{"CODEX_HOME": v}))
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join("/h/user", ".codex", "skills", "gadak", "SKILL.md"); got != want {
			t.Errorf("CODEX_HOME=%q: %q, want the default %q", v, got, want)
		}
	}
}

func TestProjectScope(t *testing.T) {
	cwd := filepath.Join("/w", "proj")
	env := testEnv("/h/user", cwd, nil)

	want := map[string]string{
		"claude": filepath.Join(cwd, ".claude", "skills", "gadak", "SKILL.md"),
		"codex":  filepath.Join(cwd, ".agents", "skills", "gadak", "SKILL.md"),
		"agents": filepath.Join(cwd, ".agents", "skills", "gadak", "SKILL.md"),
	}
	for _, c := range Clients() {
		got, err := c.ProjectDest(env)
		if w, ok := want[c.Name]; ok {
			if err != nil {
				t.Errorf("%s --project: %v", c.Name, err)
				continue
			}
			if got != w {
				t.Errorf("%s project dest = %q, want %q", c.Name, got, w)
			}
			if !c.HasProjectScope() {
				t.Errorf("%s resolved a project dest but reports no project scope", c.Name)
			}
			continue
		}
		// Everyone else refuses, and the refusal names the format that is
		// actually there — a rules file, a manifest, a plugin — so the user is
		// not told "no" without being told what to look for instead.
		if err == nil {
			t.Errorf("%s --project resolved %q, want a refusal", c.Name, got)
			continue
		}
		if !strings.Contains(err.Error(), c.Name) {
			t.Errorf("%s refusal does not name the client: %v", c.Name, err)
		}
		if !strings.Contains(err.Error(), "not a skill directory") && !strings.Contains(err.Error(), "no measured project") {
			t.Errorf("%s refusal does not say why: %v", c.Name, err)
		}
	}
}

// TestHomeDocMatchesResolvedPath keeps the help/doc caption honest: it is a
// caption for configDir, not a second source of truth.
func TestHomeDocMatchesResolvedPath(t *testing.T) {
	home := filepath.Join("/h", "user")
	env := testEnv(home, "/w", nil)
	for _, c := range Clients() {
		dest, err := c.HomeDest(env)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		doc := c.HomeDoc()
		if doc == "" {
			t.Errorf("%s has no documented home path", c.Name)
			continue
		}
		// The caption may carry a parenthetical about an override; the default
		// it documents is the part before it.
		def := doc
		if i := strings.Index(def, "(default "); i >= 0 {
			def = strings.TrimSuffix(strings.TrimSpace(def[i+len("(default "):]), ")")
		}
		want := strings.TrimSuffix(filepath.ToSlash(filepath.Dir(dest)), "/") + "/"
		want = strings.Replace(want, filepath.ToSlash(home), "~", 1)
		if def != want {
			t.Errorf("%s documents %q but resolves to %q", c.Name, def, want)
		}
	}
}

func TestLookupNamesAndDefault(t *testing.T) {
	if _, ok := Lookup(DefaultClient); !ok {
		t.Fatalf("default client %q is not in the table", DefaultClient)
	}
	if got := Names()[0]; got != DefaultClient {
		t.Errorf("first listed client = %q, want the default %q", got, DefaultClient)
	}
	if c, ok := Lookup("  CoDeX "); !ok || c.Name != "codex" {
		t.Errorf("Lookup should be case- and space-insensitive, got %+v ok=%v", c, ok)
	}
	if _, ok := Lookup("windsurf"); ok {
		t.Error("windsurf reads an always-on rules file — it must not be in the skill table")
	}
	seen := map[string]bool{}
	for _, n := range Names() {
		if seen[n] {
			t.Errorf("duplicate client %q", n)
		}
		seen[n] = true
	}
}

func TestUnknownClientErrorListsSupported(t *testing.T) {
	err := UnknownClientError("windsurf")
	if err == nil {
		t.Fatal("want an error")
	}
	s := err.Error()
	if !strings.Contains(s, "windsurf") {
		t.Errorf("error should name what was asked for: %v", err)
	}
	for _, n := range Names() {
		if !strings.Contains(s, n) {
			t.Errorf("error should list %q: %v", n, err)
		}
	}
}

// TestIsOSMetadata — orca's incident, ported. One Finder visit writes
// .DS_Store, and that alone was enough to make an untouched copy read as
// "unrecognized" there.
func TestIsOSMetadata(t *testing.T) {
	for _, name := range []string{".DS_Store", ".ds_store", "Thumbs.db", "thumbs.db", "desktop.ini", "._SKILL.md", "._"} {
		if !IsOSMetadata(name) {
			t.Errorf("%q should be tolerated as OS metadata", name)
		}
	}
	for _, name := range []string{"SKILL.md", ".gadak-skill.json", "reference.md", ".hidden", "store.db"} {
		if IsOSMetadata(name) {
			t.Errorf("%q is not OS metadata", name)
		}
	}
}

// TestPresentIgnoresOSMetadata — a config directory holding nothing but a
// Finder droplet is not evidence a host is installed, so doctor must not grow a
// row the user cannot act on.
func TestPresentIgnoresOSMetadata(t *testing.T) {
	home := t.TempDir()
	env := testEnv(home, home, nil)
	claude, _ := Lookup("claude")

	if claude.Present(env) {
		t.Error("absent ~/.claude reported present")
	}

	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if claude.Present(env) {
		t.Error("empty ~/.claude reported present")
	}
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("finder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if claude.Present(env) {
		t.Error("~/.claude holding only .DS_Store reported present")
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !claude.Present(env) {
		t.Error("~/.claude with real content reported absent")
	}
}

func TestDirDestOverridesEveryHost(t *testing.T) {
	root := t.TempDir()
	got, err := DirDest(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "gadak", "SKILL.md"); got != want {
		t.Errorf("DirDest = %q, want %q", got, want)
	}
	// Relative paths become absolute, so the plan a user reads names a real
	// place rather than something relative to a directory they have forgotten.
	rel, err := DirDest("skills-preview")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(rel) {
		t.Errorf("DirDest(%q) = %q, want an absolute path", "skills-preview", rel)
	}
}

func TestMissingHomeIsReportedNotGuessed(t *testing.T) {
	env := Env{Cwd: "/w/proj"}
	claude, _ := Lookup("claude")
	if _, err := claude.HomeDest(env); err == nil {
		t.Error("a home-less environment must fail, not resolve to a relative path")
	}
	// ...but the project scope does not need a home directory.
	if _, err := claude.ProjectDest(env); err != nil {
		t.Errorf("--project should not need a home directory: %v", err)
	}
}
