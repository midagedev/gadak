package fields

import (
	"encoding/json"
	"testing"

	"github.com/midagedev/scry/internal/config"
)

func TestCoalesceFirstFilled(t *testing.T) {
	specs := []config.FieldSpec{
		{Alias: "severity", IDs: []string{"customfield_10", "customfield_20"}},
		{Alias: "empty_all", IDs: []string{"customfield_30"}},
	}
	extra := map[string]json.RawMessage{
		"customfield_10": json.RawMessage(`null`),
		"customfield_20": json.RawMessage(`{"value":"Sev1"}`),
		"customfield_30": json.RawMessage(`[]`),
	}
	got := CoalesceSpecs(specs, extra)
	if got == nil || got["severity"] == nil {
		t.Fatalf("got %+v", got)
	}
	m, ok := got["severity"].(map[string]any)
	if !ok || m["value"] != "Sev1" {
		t.Errorf("severity = %#v", got["severity"])
	}
	if _, ok := got["empty_all"]; ok {
		t.Error("empty_all should be omitted")
	}
}

func TestIsFilled(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{`null`, false},
		{`""`, false},
		{`[]`, false},
		{`{}`, false},
		{`0`, true},
		{`false`, true},
		{`"hello"`, true},
		{`[1]`, true},
		{`{"a":1}`, true},
		{``, false},
	}
	for _, tc := range cases {
		if got := IsFilled(json.RawMessage(tc.raw)); got != tc.want {
			t.Errorf("IsFilled(%s) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestSuggestAlias(t *testing.T) {
	used := map[string]bool{"story_points": true}
	if got := SuggestAlias("Story Points", "customfield_10016", used); got != "story_points_10016" {
		t.Errorf("got %q", got)
	}
	if got := ASCIISlug("순위"); got != "" {
		t.Errorf("ASCIISlug non-ascii = %q", got)
	}
	if got := SuggestAlias("순위", "customfield_10019", map[string]bool{}); got != "cf_10019" {
		t.Errorf("got %q", got)
	}
}
