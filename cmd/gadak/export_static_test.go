package main

import (
	"database/sql"
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

// GDK-389 review: the label filter is a whitelist, so the interesting cases
// are the ones a denylist gets wrong — an issue with no labels at all, and an
// issue whose label merely contains the required one as a substring.
func TestKeepLabelledPublishesOnlyTheMarked(t *testing.T) {
	boot := []byte(`{"members":[],"issues":[
		{"issue_key":"GDK-1","labels":["public","bug"]},
		{"issue_key":"GDK-2","labels":["launch"]},
		{"issue_key":"GDK-3"},
		{"issue_key":"GDK-4","labels":[]},
		{"issue_key":"GDK-5","labels":["publication"]},
		{"issue_key":"GDK-6","labels":["docs","public"]}
	]}`)
	out, err := keepLabelled(boot, "public")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Members []any `json:"members"`
		Issues  []struct {
			Key string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, i := range got.Issues {
		keys = append(keys, i.Key)
	}
	want := []string{"GDK-1", "GDK-6"}
	if len(keys) != len(want) {
		t.Fatalf("kept %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("kept %v, want %v", keys, want)
		}
	}
	if got.Members == nil {
		t.Fatal("keepLabelled dropped a sibling bootstrap field; it must only filter issues")
	}
}

// An empty result is a filter that did not match anything — publishing it
// would replace the backlog with nothing, silently.
func TestKeepLabelledRefusesAnEmptyResult(t *testing.T) {
	boot := []byte(`{"issues":[{"issue_key":"GDK-1","labels":["launch"]}]}`)
	if _, err := keepLabelled(boot, "public"); err == nil {
		t.Fatal("expected a refusal when no issue carries the label")
	}
}

// GDK-429: the mirror runs in WAL mode, so a byte copy of the main file alone
// silently omits committed writes. FAIL-first: with the old
// os.ReadFile/os.WriteFile pair this test sees 1 row instead of 2, and the
// export reports success either way — which on the publishing path means a
// `public` label removed minutes ago stays published.
func TestCopyMirrorSeesUncheckpointedWAL(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "mirror.db")

	db, err := sql.Open("sqlite", "file:"+src+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`create table t (k text primary key)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`insert into t values ('early')`); err != nil {
		t.Fatal(err)
	}
	// Force the early row down into the main file, then write one that stays
	// in the WAL — the two halves the old copy could not tell apart.
	if _, err := db.Exec(`pragma wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`insert into t values ('in-wal')`); err != nil {
		t.Fatal(err)
	}
	var mode string
	if err := db.QueryRow(`pragma journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Skipf("journal_mode = %q, not wal — nothing to prove here", mode)
	}

	dst := filepath.Join(dir, "copy.db")
	if err := copyMirror(src, dst); err != nil {
		t.Fatalf("copyMirror: %v", err)
	}
	_ = db.Close()

	got, err := sql.Open("sqlite", "file:"+dst+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	var n int
	if err := got.QueryRow(`select count(*) from t`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("copy holds %d rows, want 2 — the WAL was dropped", n)
	}
}
