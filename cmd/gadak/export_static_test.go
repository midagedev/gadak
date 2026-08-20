package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// GDK-389: the public-backlog scrub is whitelist-rebuild — anything the live
// handlers add later must NOT leak into a published snapshot by default.
func TestExportStaticScrubProducesWhitelistOnly(t *testing.T) {
	out := t.TempDir()
	err := cmdExportStatic([]string{
		"--db", "../../examples/demo.db",
		"--attachments", "../../examples/attachments",
		"--scrub",
		"--projects", "NMB,NMA,NMS",
		out,
	})
	if err != nil {
		t.Fatal(err)
	}

	boot, err := os.ReadFile(filepath.Join(out, "bootstrap.json"))
	if err != nil {
		t.Fatal(err)
	}
	var b struct {
		Members []any                        `json:"members"`
		Issues  []map[string]json.RawMessage `json:"issues"`
	}
	if err := json.Unmarshal(boot, &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Members) != 0 {
		t.Fatalf("scrubbed bootstrap carries %d members", len(b.Members))
	}
	if len(b.Issues) == 0 {
		t.Fatal("scrubbed bootstrap has no issues")
	}
	allowed := map[string]bool{}
	for _, k := range issueWhitelist {
		allowed[k] = true
	}
	for _, is := range b.Issues {
		for k := range is {
			if !allowed[k] {
				t.Fatalf("non-whitelisted issue field %q survived the scrub", k)
			}
		}
		if _, ok := is["assignee"]; ok {
			t.Fatal("assignee leaked")
		}
	}

	// Every detail file must be content-empty.
	entries, err := os.ReadDir(filepath.Join(out, "detail"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no detail files")
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(out, "detail", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var d struct {
			DescriptionADF json.RawMessage `json:"description_adf"`
			Attachments    []any           `json:"attachments"`
			Comments       []any           `json:"comments"`
			History        []any           `json:"history"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if string(d.DescriptionADF) != "null" ||
			len(d.Attachments) != 0 || len(d.Comments) != 0 || len(d.History) != 0 {
			t.Fatalf("%s: content survived the scrub", e.Name())
		}
	}

	var cfg map[string]any
	raw, err := os.ReadFile(filepath.Join(out, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["profile"] != "backlog" {
		t.Fatalf("scrubbed config profile = %v, want backlog (cache-scope partition)", cfg["profile"])
	}
}
