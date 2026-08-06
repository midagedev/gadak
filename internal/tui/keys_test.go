package tui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

// TestHelpLinesCoverAllBindings ensures every keyMap binding's help key appears
// in helpLines(). New bindings must be registered in helpLines or this fails.
// Matching is by Help().Key so aliases that share a help key (e.g. ClearFilter
// and Back both use "esc") count as covered when either is listed.
func TestHelpLinesCoverAllBindings(t *testing.T) {
	km := defaultKeys()
	lines := km.helpLines()
	helpKeys := make(map[string]bool, len(lines))
	for _, pair := range lines {
		helpKeys[pair[0]] = true
	}

	v := reflect.ValueOf(km)
	typ := v.Type()
	var missing []string
	for i := 0; i < v.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		b, ok := v.Field(i).Interface().(key.Binding)
		if !ok {
			continue
		}
		h := b.Help()
		if h.Key == "" {
			continue
		}
		if !helpKeys[h.Key] {
			missing = append(missing, fmt.Sprintf("%s (%q)", field.Name, h.Key))
		}
	}
	if len(missing) > 0 {
		t.Fatalf("keyMap bindings missing from helpLines(): %v", missing)
	}
}

func TestPageScrollBindings(t *testing.T) {
	km := defaultKeys()
	for _, s := range []string{"pgdown", "ctrl+d"} {
		if !keyMatches(km.PageDown, s) {
			t.Errorf("PageDown should match %q", s)
		}
	}
	for _, s := range []string{"pgup", "ctrl+u"} {
		if !keyMatches(km.PageUp, s) {
			t.Errorf("PageUp should match %q", s)
		}
	}
}
