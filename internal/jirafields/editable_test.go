package jirafields

import (
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

func TestEditKind(t *testing.T) {
	cases := []struct {
		typ, items, want string
	}{
		{"option", "", "option"},
		{"user", "", "user"},
		{"array", "version", "version_array"},
		{"array", "option", "multi_option"},
		{"string", "", "text"},
		{"number", "", "number"},
		{"date", "", "date"},
		{"array", "component", ""},
		{"datetime", "", ""},
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

func TestResolveEditablePicksEditableCandidate(t *testing.T) {
	meta := map[string]jira.FieldMeta{
		"customfield_10": func() jira.FieldMeta {
			var m jira.FieldMeta
			m.Schema.Type = "option"
			return m
		}(),
		"customfield_skip": func() jira.FieldMeta {
			var m jira.FieldMeta
			m.Schema.Type = "datetime" // no editor in this slice
			return m
		}(),
	}
	// First candidate missing, second present.
	id, kind, ok := ResolveEditable([]string{"customfield_missing", "customfield_10"}, meta, "")
	if !ok || id != "customfield_10" || kind != "option" {
		t.Fatalf("got %q %q %v", id, kind, ok)
	}
	// Present but no editor kind → skip.
	if _, _, ok := ResolveEditable([]string{"customfield_skip"}, meta, ""); ok {
		t.Fatal("unsupported schema should skip")
	}

	// textarea (ADF) is string but has no editor in this slice.
	var textarea jira.FieldMeta
	textarea.Schema.Type = "string"
	textarea.Schema.Custom = "com.atlassian.jira.plugin.system.customfieldtypes:textarea"
	if got := EditKind(textarea); got != "" {
		t.Fatalf("textarea EditKind=%q, want empty (excluded this slice)", got)
	}
	// None present.
	if _, _, ok := ResolveEditable([]string{"customfield_x"}, meta, ""); ok {
		t.Fatal("missing id should not resolve")
	}
}

func TestResolveEditableFallsBackToConfiguredKind(t *testing.T) {
	meta := map[string]jira.FieldMeta{
		"customfield_dt": func() jira.FieldMeta {
			var m jira.FieldMeta
			m.Schema.Type = "datetime"
			return m
		}(),
	}
	// FAIL-first (history): the ID-only call shape (fallbackKind "") skipped
	// unrecognized schemas, so CLI edit's kind fallback never ran while
	// create's own loop accepted the same id via the configured kind.
	if _, _, ok := ResolveEditable([]string{"customfield_dt"}, meta, ""); ok {
		t.Fatal("datetime has no editor; empty fallbackKind should skip")
	}
	id, kind, ok := ResolveEditable([]string{"customfield_dt"}, meta, "date")
	if !ok || id != "customfield_dt" || kind != "date" {
		t.Fatalf("got %q %q %v, want customfield_dt date true", id, kind, ok)
	}
}

func TestFieldMetaFromCreatePreservesSchemaAndAllowed(t *testing.T) {
	var f jira.CreateFieldMeta
	f.FieldID = "customfield_1"
	f.Required = true
	f.Schema.Type = "option"
	f.AllowedValues = []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
		Name  string `json:"name"`
	}{{ID: "10", Value: "High"}}
	m := FieldMetaFromCreate(f)
	if !m.Required || m.Schema.Type != "option" {
		t.Fatalf("schema %+v required %v", m.Schema, m.Required)
	}
	if len(m.AllowedValues) != 1 || m.AllowedValues[0].ID != "10" || m.AllowedValues[0].Value != "High" {
		t.Fatalf("allowed %+v", m.AllowedValues)
	}
	id, kind, ok := ResolveEditable([]string{"customfield_1"}, map[string]jira.FieldMeta{"customfield_1": m}, "")
	if !ok || id != "customfield_1" || kind != "option" {
		t.Fatalf("create-shaped meta did not resolve: %q %q %v", id, kind, ok)
	}
}
