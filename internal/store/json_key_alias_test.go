package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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

// TestIssueKeyStructTagsUseHelper fails closed: a new production
// `json:"issue_key"` tag that does not go through MarshalWithIssueKeyAlias
// (or the export_static whitelist copy) is a drift hole.
func TestIssueKeyStructTagsUseHelper(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	allowed := map[string]string{
		"internal/store/read.go":     "MarshalWithIssueKeyAlias",
		"internal/store/feed.go":     "MarshalWithIssueKeyAlias",
		"internal/server/read.go":    "MarshalWithIssueKeyAlias",
		"cmd/gadak/export_static.go": `"key"`, // whitelist + scrubDetail copy
		// The terminal session's issue binding (GDK-1158) is not a mirror
		// row: it is runtime state on a PTY session, emitted by the binding
		// route. The GDK-255 alias would be a lie here — a session Info has
		// no issues_full.key to alias — so the tags are allowed without
		// MarshalWithIssueKeyAlias, pinned by the handler that owns them.
		"internal/server/terminal.go": "handleTerminalIssue",
	}
	var unexpected []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "vendor" || base == "node_modules" || base == ".git" || base == "scratch" || base == "dist" || base == "examples" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), `json:"issue_key"`) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		need, ok := allowed[rel]
		if !ok {
			unexpected = append(unexpected, rel)
			return nil
		}
		if !strings.Contains(string(body), need) {
			t.Errorf("%s has json:\"issue_key\" but does not contain %s", rel, need)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unexpected) > 0 {
		t.Errorf("new json:\"issue_key\" emit sites must use MarshalWithIssueKeyAlias (or be added to the allowed map with a reason): %s", strings.Join(unexpected, ", "))
	}
}
