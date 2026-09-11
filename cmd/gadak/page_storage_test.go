package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

/*
 * GDK-1303: the storage round trip. `page get --storage` prints the body's
 * storage document verbatim; `page edit --storage-file F` replaces the whole
 * body with that document. Together they are the lossless edit path for
 * formatting markdown cannot carry — an agent that reads a rich page, edits
 * one section, and writes it back must not lose the rest, which is what the
 * markdown (-m) path refuses rather than risk.
 *
 *   TestPageGetStoragePrintsVerbatimADF
 *        ① exactly the stored document, nothing else — no kv header, no
 *           comments, byte-equal to what the origin accepted
 *   TestPageStorageRoundTripKeepsRichNodes
 *        ② get --storage → append one paragraph → edit --storage-file →
 *           get --storage: the panel (what markdown cannot express) is
 *           still there and the new paragraph rode along
 *   TestPageEditStorageFileRefusesAdfFile
 *        ③ both whole-body replaces at once is a usage error, and the
 *           origin sees no PUT
 *   TestPageGetStorageRefusesJSON
 *        ④ --storage and --json both claim stdout — refused, not silently
 *           merged
 *   TestPageGetStorageEmptyBodySaysSo
 *        ⑤ a page with no storage body errors honestly instead of
 *           printing an empty document that would wipe the page on the
 *           write side
 */

// storageRoundTripPage seeds one page through the real write path — the
// built-in origin, like builtInPages, but with a --doc-file body so the page
// holds formatting markdown cannot carry.
func storageRoundTripPage(t *testing.T, adf string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})
	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(f, []byte(adf), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"create", "--space", origin.DefaultSpaceKey, "--title", "Rich page"}
	if adf != "" {
		args = append(args, "--doc-file", f)
	}
	out, err := capture(t, func() error { return cmdPage(args) })
	if err != nil {
		t.Fatalf("page create: %v\n%s", err, out)
	}
	id := strings.SplitN(strings.TrimSpace(out), "\t", 2)[0]
	if id == "" {
		t.Fatalf("page create printed no id: %q", out)
	}
	return id
}

func TestPageGetStoragePrintsVerbatimADF(t *testing.T) {
	id := storageRoundTripPage(t, cliWikiComplexADF)

	out, err := capture(t, func() error { return cmdPage([]string{"get", id, "--storage"}) })
	if err != nil {
		t.Fatalf("page get --storage: %v\n%s", err, out)
	}
	got := strings.TrimSpace(out)
	if got != cliWikiComplexADF {
		t.Fatalf("--storage must print the stored document verbatim:\ngot:  %s\nwant: %s", got, cliWikiComplexADF)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("--storage printed something that is not JSON:\n%s", got)
	}
	for _, header := range []string{"space", "comments (", "updated"} {
		if strings.Contains(out, header) {
			t.Errorf("--storage leaked the detail header %q:\n%s", header, out)
		}
	}
}

func TestPageStorageRoundTripKeepsRichNodes(t *testing.T) {
	id := storageRoundTripPage(t, cliWikiComplexADF)

	out, err := capture(t, func() error { return cmdPage([]string{"get", id, "--storage"}) })
	if err != nil {
		t.Fatalf("page get --storage: %v\n%s", err, out)
	}
	doc := map[string]any{}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("storage document did not parse: %v", err)
	}
	content, _ := doc["content"].([]any)
	doc["content"] = append(content, map[string]any{
		"type":    "paragraph",
		"content": []map[string]any{{"type": "text", "text": "added by the round trip"}},
	})
	f := filepath.Join(t.TempDir(), "next.json")
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := capture(t, func() error { return cmdPage([]string{"edit", id, "--storage-file", f}) }); err != nil {
		t.Fatalf("page edit --storage-file: %v", err)
	}

	next, err := capture(t, func() error { return cmdPage([]string{"get", id, "--storage"}) })
	if err != nil {
		t.Fatalf("second page get --storage: %v\n%s", err, next)
	}
	for _, want := range []string{`"panel"`, "Steps", "added by the round trip"} {
		if !strings.Contains(next, want) {
			t.Errorf("storage document lost %q in the round trip:\n%s", want, next)
		}
	}
	// The markdown read must show both texts too: the round trip is for
	// editing, and the human-facing form still has to carry the content.
	text, err := capture(t, func() error { return cmdPage([]string{"get", id}) })
	if err != nil {
		t.Fatalf("page get: %v\n%s", err, text)
	}
	for _, want := range []string{"note", "added by the round trip"} {
		if !strings.Contains(text, want) {
			t.Errorf("text form missing %q after the round trip:\n%s", want, text)
		}
	}
}

func TestPageEditStorageFileRefusesAdfFile(t *testing.T) {
	wo := newCLIWikiOrigin(t)
	mirror(t, wo.URL)

	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte(cliWikiSimpleADF), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := capture(t, func() error {
		return cmdPageEdit([]string{"100", "--adf-file", a, "--storage-file", b})
	})
	if err == nil {
		t.Fatal("--adf-file + --storage-file must refuse")
	}
	if !strings.Contains(err.Error(), "pick one") {
		t.Errorf("got %v", err)
	}
	if wo.puts != 0 {
		t.Fatalf("refusal must not PUT, got %d", wo.puts)
	}
}

func TestPageGetStorageRefusesJSON(t *testing.T) {
	id := storageRoundTripPage(t, cliWikiSimpleADF)
	_, err := capture(t, func() error { return cmdPage([]string{"get", id, "--storage", "--json"}) })
	if err == nil {
		t.Fatal("--storage + --json must refuse")
	}
	if !strings.Contains(err.Error(), "pick one") {
		t.Errorf("got %v", err)
	}
}

func TestPageGetStorageNoBodySaysSo(t *testing.T) {
	// A page whose body never reached the mirror (empty body_adf — an
	// interrupted refresh, not an empty document) has nothing to print;
	// an empty success here would become an empty file and, on the write
	// side, a silent wipe. Seeded by SQL on purpose: the write paths all
	// store a real document, so the empty row is a mirror state, not a
	// command outcome.
	id := storageRoundTripPage(t, cliWikiSimpleADF)
	db, err := sql.Open("sqlite", "file:"+filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatalf("open mirror: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE pages SET body_adf = '' WHERE item_id = (SELECT id FROM items WHERE key = ?)`, id); err != nil {
		t.Fatalf("empty body_adf: %v", err)
	}
	_, err = capture(t, func() error { return cmdPage([]string{"get", id, "--storage"}) })
	if err == nil {
		t.Fatal("no storage body must fail, not print an empty document")
	}
	if !strings.Contains(err.Error(), "no storage body") {
		t.Errorf("got %v", err)
	}
}
