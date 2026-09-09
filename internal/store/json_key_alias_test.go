package store

import (
	"encoding/json"
	"testing"
)

// The struct-tag walk that used to live here is a source lint and now lives
// behind the sourcelint tag in issue_key_tag_walk_test.go — run it with
// `bash tools/sourcelint.sh` (GDK-1144).

// TestIssueKeyAliasOnWire is the GDK-255 seal for store JSON types: issue_key
// and key must both be present and equal. Derived at marshal time so a
// constructor cannot emit one without the other.
func TestIssueKeyAliasOnWire(t *testing.T) {
	const k = "NMB-1"
	// Detail is not in this list: it is embedded anonymously in cmd/gadak's
	// issueDoc, and an embedded Marshaler replaces the outer object. CLI
	// issue --json uses issueDoc.MarshalJSON; HTTP detail uses detailResponse.
	cases := []any{
		IssueLite{IssueKey: k},
		FeedItem{IssueKey: k},
	}
	for _, v := range cases {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%T: %v", v, err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%T unmarshal: %v\n%s", v, err, raw)
		}
		gotIssueKey, _ := m["issue_key"].(string)
		gotKey, _ := m["key"].(string)
		if gotIssueKey != k {
			t.Errorf("%T issue_key=%q, want %q in %s", v, gotIssueKey, k, raw)
		}
		if gotKey != k {
			t.Errorf("%T key=%q, want %q (alias of issue_key) in %s", v, gotKey, k, raw)
		}
	}
}

func TestAliasIssueKeyMap(t *testing.T) {
	m := map[string]any{"issue_key": "GDK-255", "summary": "x"}
	AliasIssueKey(m)
	if m["key"] != "GDK-255" || m["issue_key"] != "GDK-255" {
		t.Fatalf("AliasIssueKey = %#v", m)
	}
	AliasIssueKey(nil) // must not panic
}
