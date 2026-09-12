package main

import (
	"strings"
	"testing"
)

// A repeated --field key on transition/close used to overwrite silently
// (GDK-1804): `--field k=a --field k=b` sent only b and said nothing. The
// map shape is right for a transition screen — collecting into a list, the
// way edit/create do (GDK-18), would send the origin a type error — so the
// refusal is the fix, and it has to name the key.
func TestParseTransitionFieldFlagsRefusesRepeatedKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  []string
	}{
		{"different values", []string{"k=a", "k=b"}},
		{"identical values", []string{"k=a", "k=a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTransitionFieldFlags(tc.raw)
			if err == nil {
				t.Fatalf("a repeated --field key must be refused, got %#v", got)
			}
			if !strings.Contains(err.Error(), "k") {
				t.Errorf("the error must name the repeated key, got %q", err)
			}
			if !strings.Contains(err.Error(), "--field") {
				t.Errorf("the error must name the flag, got %q", err)
			}
		})
	}
}

// Distinct keys are the reason the flag stays repeatable — one flag per
// screen field.
func TestParseTransitionFieldFlagsKeepsDistinctKeys(t *testing.T) {
	got, err := parseTransitionFieldFlags([]string{"k=a", "j=2"})
	if err != nil {
		t.Fatalf("two distinct keys must parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 fields, got %#v", got)
	}
	if got["k"] != "a" {
		t.Errorf(`k: want "a", got %#v`, got["k"])
	}
	if got["j"] != float64(2) {
		t.Errorf("j: want JSON 2, got %#v", got["j"])
	}
}

// The rendered help is the only place a user learns the rule, so the flag
// usage carries it rather than the word "(repeatable)", which read as a
// licence to repeat a key.
func TestTransitionHelpSaysAKeyMayNotRepeat(t *testing.T) {
	for _, name := range []string{"transition", "close"} {
		fs, _, _, _, _, _ := newTransitionFlags(name)
		// Scope the assertion to the --field option line: another flag on
		// this command may legitimately say "(repeatable)" one day, and a
		// whole-output match would then blame the wrong one.
		var line string
		for _, l := range strings.Split(formatHelp(name, fs), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "--field") {
				line = l
				break
			}
		}
		if line == "" {
			t.Fatalf("gadak %s --help has no --field option line", name)
		}
		if !strings.Contains(line, "may not repeat") {
			t.Errorf("gadak %s --help --field must say a key may not repeat, got:\n%s", name, line)
		}
		if strings.Contains(line, "(repeatable)") {
			t.Errorf("gadak %s --help --field must not call the key repeatable, got:\n%s", name, line)
		}
	}
}
