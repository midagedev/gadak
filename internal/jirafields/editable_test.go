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

func TestResolveEditableID(t *testing.T) {
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
	id, kind, ok := ResolveEditableID([]string{"customfield_missing", "customfield_10"}, meta)
	if !ok || id != "customfield_10" || kind != "option" {
		t.Fatalf("got %q %q %v", id, kind, ok)
	}
	// Present but no editor kind → skip.
	if _, _, ok := ResolveEditableID([]string{"customfield_skip"}, meta); ok {
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
	if _, _, ok := ResolveEditableID([]string{"customfield_x"}, meta); ok {
		t.Fatal("missing id should not resolve")
	}
}
