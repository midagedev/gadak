package main

// GDK-1281: `--standalone` was the flag until the vocabulary split, and a
// flag someone already put in a script is a contract. It keeps working
// forever; it is simply not taught any more, so the help names only
// --local. The same holds for --replace-standalone → --replace-local.

import (
	"strings"
	"testing"
)

func TestInitLocalOriginFlagStillWorksAsAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)

	out, err := capture(t, func() error { return cmdInit([]string{"--standalone", "--json"}) })
	if err != nil {
		t.Fatalf("--standalone must still seed a workspace: %v\n%s", err, out)
	}
	if !strings.Contains(out, "\"kind\"") {
		t.Errorf("--standalone produced no init document: %s", out)
	}
}

// The help teaches one name. If --standalone ever appears there again,
// two spellings are being taught for one thing, which is what the rename
// was for.
func TestInitHelpTeachesOnlyTheNewFlag(t *testing.T) {
	h := helps["init"]
	surfaces := []string{h.usage, h.summary, strings.Join(h.examples, "\n")}
	for _, f := range h.options {
		surfaces = append(surfaces, f.name, f.desc)
	}
	for _, s := range surfaces {
		if strings.Contains(s, "--standalone") || s == "standalone" {
			t.Errorf("init help still teaches the old flag name: %q", s)
		}
	}
	if !strings.Contains(h.usage, "--local") {
		t.Errorf("init usage does not name --local: %s", h.usage)
	}
}

// GDK-1312: the alias table mapped "replace-local" to itself and never knew
// the old spelling, so `--replace-standalone` was "flag provided but not
// defined" — the exact contract the function's comment promises to keep.
// Parsing is enough here: the flag must be recognised, whatever the
// workspace then says about replacing.
func TestInitReplaceStandaloneFlagStillParses(t *testing.T) {
	got := renameLegacyInitFlags([]string{"--replace-standalone", "--standalone", "-replace-standalone=true", "--json"})
	want := []string{"--replace-local", "--local", "-replace-local=true", "--json"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("renameLegacyInitFlags = %q, want %q", got, want)
	}
}
