package jirafields

import (
	"reflect"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

func catalogField(id, name, typ string) jira.FieldInfo {
	f := jira.FieldInfo{ID: id, Name: name, Custom: true}
	f.Schema.Type = typ
	if typ == "option" {
		f.Schema.Custom = "com.atlassian.jira.plugin.system.customfieldtypes:select"
	}
	if typ == "string" {
		f.Schema.Custom = "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
	}
	return f
}

func TestDiscoverMergesSameNameByFill(t *testing.T) {
	catalog := []jira.FieldInfo{
		catalogField("customfield_10", "Severity Level", "option"),
		catalogField("customfield_20", "Severity Level", "option"),
		catalogField("customfield_30", "Never Filled", "option"),
		// system field ignored
		{ID: "summary", Name: "Summary", Custom: false},
	}
	fill := map[string]int{
		"customfield_10": 5,
		"customfield_20": 12, // more filled → first in IDs
		"customfield_30": 0,
	}
	got := Discover(catalog, fill, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (zero-fill group excluded): %+v", len(got), got)
	}
	s := got[0]
	if s.Label != "Severity Level" || s.Role != "facet" || s.Kind != "option" || !s.Auto {
		t.Errorf("spec = %+v", s)
	}
	if !reflect.DeepEqual(s.IDs, []string{"customfield_20", "customfield_10"}) {
		t.Errorf("IDs = %v, want fill-desc order", s.IDs)
	}
	if s.Alias != "severity_level" {
		t.Errorf("alias = %q", s.Alias)
	}
}

func TestDiscoverPreservesManualAndReusesAutoAlias(t *testing.T) {
	catalog := []jira.FieldInfo{
		catalogField("customfield_10", "Severity Level", "option"),
		catalogField("customfield_99", "Other Note", "string"),
	}
	fill := map[string]int{"customfield_10": 3, "customfield_99": 1}
	prior := []config.FieldSpec{
		{Alias: "sev_fixed", Label: "Severity Level", IDs: []string{"customfield_10"}, Role: "facet", Kind: "option", Auto: false},
		{Alias: "other_note_old", Label: "Other Note", IDs: []string{"customfield_99"}, Role: "plain", Auto: true},
	}
	got := Discover(catalog, fill, prior)
	// Manual kept; auto regenerated with alias reuse; manual id excluded from discovery.
	var manual, auto *config.FieldSpec
	for i := range got {
		if got[i].Alias == "sev_fixed" {
			manual = &got[i]
		}
		if got[i].Alias == "other_note_old" {
			auto = &got[i]
		}
	}
	if manual == nil || manual.Auto {
		t.Fatalf("manual not preserved: %+v", got)
	}
	if auto == nil || !auto.Auto {
		t.Fatalf("auto alias not reused: %+v", got)
	}
	// Severity Level ids were reserved — no second severity auto-spec.
	for _, s := range got {
		if s.Alias != "sev_fixed" && s.Label == "Severity Level" {
			t.Errorf("severity should not reappear as auto: %+v", s)
		}
	}
}

func TestDiscoverNonASCIIFallsBackToCF(t *testing.T) {
	// Neutral non-ASCII name — not a company field label.
	catalog := []jira.FieldInfo{
		catalogField("customfield_10019", "중요도", "option"),
	}
	fill := map[string]int{"customfield_10019": 2}
	got := Discover(catalog, fill, nil)
	if len(got) != 1 {
		t.Fatalf("len = %d: %+v", len(got), got)
	}
	if got[0].Alias != "cf_10019" {
		t.Errorf("alias = %q, want cf_10019", got[0].Alias)
	}
}
