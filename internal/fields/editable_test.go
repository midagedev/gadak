package fields

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
)

func TestEditKind(t *testing.T) {
	cases := []struct {
		typ, items, want string
	}{
		{"option", "", "option"},
		{"user", "", "user"},
		{"array", "version", "version_array"},
		{"array", "option", "multi_option"},
		{"string", "", ""},
		{"array", "component", ""},
	}
	for _, tc := range cases {
		var m jira.FieldMeta
		m.Schema.Type = tc.typ
		m.Schema.Items = tc.items
		if got := EditKind(m); got != tc.want {
			t.Errorf("EditKind(%s/%s)=%q want %q", tc.typ, tc.items, got, tc.want)
		}
	}
}

func TestValueFromIDs(t *testing.T) {
	cases := []struct {
		name string
		kind string
		ids  []string
		want any
	}{
		{"option set", "option", []string{"10001"}, map[string]string{"id": "10001"}},
		{"option clear empty", "option", nil, nil},
		{"option clear blank", "option", []string{""}, nil},
		{"user set", "user", []string{"acc-1"}, map[string]string{"accountId": "acc-1"}},
		{"user clear", "user", nil, nil},
		{"multi set", "multi_option", []string{"1", "2"}, []any{map[string]string{"id": "1"}, map[string]string{"id": "2"}}},
		{"multi clear", "multi_option", nil, []any{}},
		{"multi clear empty slice", "multi_option", []string{}, []any{}},
		{"version_array set", "version_array", []string{"v1"}, []any{map[string]string{"id": "v1"}}},
		{"version_array clear", "version_array", nil, []any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValueFromIDs(tc.kind, tc.ids)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestFieldValueMatchesValueFromIDs(t *testing.T) {
	// Clear / null / empty raw must stay byte-identical to the pre-extract path.
	cases := []struct {
		name string
		kind string
		raw  string
		want any
	}{
		{"null option", "option", "null", nil},
		{"empty raw option", "option", "", nil},
		{"empty string option", "option", `""`, nil},
		{"option id", "option", `"10001"`, map[string]string{"id": "10001"}},
		{"user id", "user", `"acc-1"`, map[string]string{"accountId": "acc-1"}},
		{"null multi", "multi_option", "null", []any{}},
		{"empty raw multi", "multi_option", "", []any{}},
		{"multi ids", "multi_option", `["a","b"]`, []any{map[string]string{"id": "a"}, map[string]string{"id": "b"}}},
		{"multi empty arr", "multi_option", `[]`, []any{}},
		{"null version_array", "version_array", "null", []any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FieldValue(tc.kind, json.RawMessage(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestFieldValueBadJSON(t *testing.T) {
	if _, err := FieldValue("option", json.RawMessage(`{"not":"string"}`)); err == nil {
		t.Fatal("expected error for non-string option value")
	}
	if _, err := FieldValue("multi_option", json.RawMessage(`"not-array"`)); err == nil {
		t.Fatal("expected error for non-array multi value")
	}
}

func TestEditableAliasesLegacyWins(t *testing.T) {
	cfg := &config.Config{
		Fields: []config.FieldSpec{
			{Alias: "severity", Label: "Severity Level", IDs: []string{"customfield_10"}, Kind: "option"},
			{Alias: "fix_versions", Label: "Fix Versions", IDs: []string{"customfield_20"}, Kind: "version_array"},
			{Alias: "display_only", Label: "Display Only", IDs: []string{"customfield_30"}}, // no Kind
		},
		EditableFields: map[string]string{
			"severity": "customfield_legacy",
		},
	}
	got := EditableAliases(cfg)
	if len(got) != 2 {
		t.Fatalf("aliases=%v", got)
	}
	// Legacy replaces the FieldSpecs entry for the same alias.
	if !reflect.DeepEqual(got["severity"].IDs, []string{"customfield_legacy"}) {
		t.Fatalf("severity legacy IDs: %+v", got["severity"])
	}
	if got["severity"].Kind != "" {
		t.Fatalf("legacy Kind should be empty, got %q", got["severity"].Kind)
	}
	if !reflect.DeepEqual(got["fix_versions"].IDs, []string{"customfield_20"}) || got["fix_versions"].Kind != "version_array" {
		t.Fatalf("fix_versions: %+v", got["fix_versions"])
	}
	if _, ok := got["display_only"]; ok {
		t.Fatal("display-only (empty Kind) must not enter write allowlist")
	}
}

func TestEditableAliasesNilConfig(t *testing.T) {
	if got := EditableAliases(nil); len(got) != 0 {
		t.Fatalf("nil cfg → %v", got)
	}
}

func TestResolveEditableID(t *testing.T) {
	meta := map[string]jira.FieldMeta{
		"customfield_10": func() jira.FieldMeta {
			var m jira.FieldMeta
			m.Schema.Type = "option"
			return m
		}(),
		"customfield_skip": func() jira.FieldMeta {
			var m jira.FieldMeta
			m.Schema.Type = "string" // no editor
			return m
		}(),
	}
	// First candidate missing, second present.
	id, kind, ok := ResolveEditableID([]string{"customfield_missing", "customfield_10"}, meta)
	if !ok || id != "customfield_10" || kind != "option" {
		t.Fatalf("got %q %q %v", id, kind, ok)
	}
	// Present but no editor kind → skip.
	if _, _, ok := ResolveEditableID([]string{"customfield_skip"}, meta); ok {
		t.Fatal("unsupported schema should skip")
	}
	// None present.
	if _, _, ok := ResolveEditableID([]string{"customfield_x"}, meta); ok {
		t.Fatal("missing id should not resolve")
	}
}
