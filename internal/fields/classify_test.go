package fields

import (
	"testing"

	"github.com/midagedev/scry/internal/jira"
)

func field(name, typ, items, custom string) jira.FieldInfo {
	f := jira.FieldInfo{ID: "customfield_1", Name: name, Custom: true}
	f.Schema.Type = typ
	f.Schema.Items = items
	f.Schema.Custom = custom
	return f
}

func TestClassifyTable(t *testing.T) {
	cases := []struct {
		name     string
		f        jira.FieldInfo
		wantRole string
		wantKind string
		wantOK   bool
	}{
		{"type any", field("X", "any", "", ""), "", "", false},
		{"type team", field("Team", "team", "", ""), "", "", false},
		{"type atlas-project", field("Proj", "atlas-project", "", ""), "", "", false},
		{"type sd-service", field("SD", "sd-customerrequesttype", "", ""), "", "", false},
		{"array json", field("Plugin", "array", "json", ""), "", "", false},
		{"array any", field("Plugin2", "array", "any", ""), "", "", false},
		{"chart prefix", field("[CHART] Time in Status", "number", "", ""), "", "", false},
		{"textarea", field("Repro Steps", "string", "", "com.atlassian.jira.plugin.system.customfieldtypes:textarea"), "body", "", true},
		{"option", field("Severity Level", "option", "", "com.atlassian…:select"), "facet", "option", true},
		{"option-with-child", field("Casc", "option-with-child", "", ""), "facet", "", true},
		{"multi option", field("Tags", "array", "option", ""), "facet", "multi_option", true},
		{"version array", field("Target", "array", "version", ""), "facet", "version_array", true},
		{"component array", field("Extra comps", "array", "component", ""), "facet", "", true},
		{"string array labels", field("Extra labels", "array", "string", ""), "facet", "", true},
		{"user", field("Owner", "user", "", ""), "user", "user", true},
		{"user array", field("Reviewers", "array", "user", ""), "user", "", true},
		{"string plain", field("URL note", "string", "", ""), "plain", "", true},
		{"number plain", field("Score", "number", "", ""), "plain", "", true},
		{"date plain", field("Due custom", "date", "", ""), "plain", "", true},
		{"datetime plain", field("When", "datetime", "", ""), "plain", "", true},
		{"url plain", field("Link", "url", "", ""), "plain", "", true},
		{"unknown type plain", field("Mystery", "unknown-type", "", ""), "plain", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, kind, ok := Classify(tc.f)
			if ok != tc.wantOK || role != tc.wantRole || kind != tc.wantKind {
				t.Fatalf("Classify(%s) = (%q,%q,%v), want (%q,%q,%v)",
					tc.name, role, kind, ok, tc.wantRole, tc.wantKind, tc.wantOK)
			}
		})
	}
}
