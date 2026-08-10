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
	// Non-body values flatten to display text: {"value":"Sev1"} → "Sev1".
	if s, ok := got["severity"].(string); !ok || s != "Sev1" {
		t.Errorf("severity = %#v, want flattened string", got["severity"])
	}
	if _, ok := got["empty_all"]; ok {
		t.Error("empty_all should be omitted")
	}
}

func TestDisplayValueShapes(t *testing.T) {
	cases := []struct {
		in   any
		want any
	}{
		{map[string]any{"value": "High"}, "High"},
		{map[string]any{"name": "1.2.0"}, "1.2.0"},
		{map[string]any{"displayName": "Kim"}, "Kim"},
		{map[string]any{"id": "only-id"}, nil},
		{"plain", "plain"},
		{float64(7), float64(7)},
	}
	for _, c := range cases {
		if got := DisplayValue(c.in); got != c.want {
			t.Errorf("DisplayValue(%#v) = %#v, want %#v", c.in, got, c.want)
		}
	}
	arr := DisplayValue([]any{map[string]any{"value": "Chrome"}, map[string]any{"value": "Safari"}})
	list, ok := arr.([]string)
	if !ok || len(list) != 2 || list[0] != "Chrome" || list[1] != "Safari" {
		t.Errorf("array = %#v", arr)
	}
}

func TestCoalesceUserAccountIDs(t *testing.T) {
	specs := []config.FieldSpec{
		{Alias: "owner", IDs: []string{"customfield_10"}, Role: "user"},
		{Alias: "reviewers", IDs: []string{"customfield_20"}, Role: "user"},
	}
	extra := map[string]json.RawMessage{
		"customfield_10": json.RawMessage(`{"accountId":"acc-1","displayName":"Ada"}`),
		"customfield_20": json.RawMessage(`[
			{"accountId":"acc-2","displayName":"Grace"},
			{"accountId":"acc-3","displayName":"Linus"}
		]`),
	}
	got := CoalesceSpecs(specs, extra)
	if got["owner"] != "Ada" {
		t.Fatalf("owner display = %#v", got["owner"])
	}
	ownerIDs, ok := got["owner"+UserAccountIDsSuffix].([]string)
	if !ok || len(ownerIDs) != 1 || ownerIDs[0] != "acc-1" {
		t.Fatalf("owner ids = %#v", got["owner"+UserAccountIDsSuffix])
	}
	reviewerIDs, ok := got["reviewers"+UserAccountIDsSuffix].([]string)
	if !ok || len(reviewerIDs) != 2 || reviewerIDs[0] != "acc-2" || reviewerIDs[1] != "acc-3" {
		t.Fatalf("reviewer ids = %#v", got["reviewers"+UserAccountIDsSuffix])
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
