package main

// GDK-1030: the agent-memory verbs. `memory add` leaves a note where the
// next session can find it (a page in the memory space, written through
// the same origin path as page create); `memory search` scopes the mirror's
// full-text search to that space. These tests pin the roundtrip, the
// connected-unset refusal, title derivation, stdin, and the 0-hit notice.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

// localOriginMemory is the memory-verb fixture: a local-origin workspace (the
// in-process origin, no network) and nothing else — memory add creates the
// first page itself.
func localOriginMemory(t *testing.T) {
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
	if out, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --standalone: %v\n%s", err, out)
	}
}

// TestMemoryAddSearchRoundtrip is the GDK-1030 contract: a note added now
// is findable now, through the same origin write path and mirror refresh
// page create uses. Text form on both ends.
func TestMemoryAddSearchRoundtrip(t *testing.T) {
	localOriginMemory(t)

	out, err := capture(t, func() error {
		return cmdMemory([]string{"add", "deploy lock on nmb: always gadak claim, never the raw API"})
	})
	if err != nil {
		t.Fatalf("memory add: %v\n%s", err, out)
	}
	fields := strings.Split(strings.TrimSpace(out), "\t")
	if len(fields) != 3 {
		t.Fatalf("memory add must print id\\ttitle\\tspace, got %q", out)
	}
	id, title, space := fields[0], fields[1], fields[2]
	if id == "" {
		t.Fatalf("memory add printed no id: %q", out)
	}
	if !strings.Contains(title, "deploy lock") {
		t.Errorf("derived title must carry the first line, got %q", title)
	}
	if space != origin.DefaultSpaceKey {
		t.Errorf("local-origin default space = %q, want the seeded %q", space, origin.DefaultSpaceKey)
	}

	sout, serr := capture(t, func() error {
		return cmdMemory([]string{"search", "deploy lock"})
	})
	if serr != nil {
		t.Fatalf("memory search: %v\n%s", serr, sout)
	}
	if !strings.Contains(sout, "id\ttitle\tsnippet\tupdated_at") {
		t.Errorf("memory search must print the TSV header, got:\n%s", sout)
	}
	var row string
	for _, line := range strings.Split(sout, "\n") {
		if strings.HasPrefix(line, id+"\t") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("memory search did not return the just-added page %s:\n%s", id, sout)
	}
	if !strings.Contains(row, "deploy lock") {
		t.Errorf("row missing the note's words: %q", row)
	}
	if strings.Count(row, "\t") != 3 {
		t.Errorf("row must be id\\ttitle\\tsnippet\\tupdated_at: %q", row)
	}
}

// TestMemoryAddSearchJSON: --json on both verbs. Add names what it wrote
// (id, title, space); search streams one object per row.
func TestMemoryAddSearchJSON(t *testing.T) {
	localOriginMemory(t)

	out, err := capture(t, func() error {
		return cmdMemory([]string{"add", "release audit rhythm", "--title", "Release audit notes", "--json"})
	})
	if err != nil {
		t.Fatalf("memory add --json: %v\n%s", err, out)
	}
	var added struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Space string `json:"space"`
	}
	if err := json.Unmarshal([]byte(out), &added); err != nil {
		t.Fatalf("add --json not an object: %v\n%s", err, out)
	}
	if added.Title != "Release audit notes" {
		t.Errorf("--title must win over derivation, got %q", added.Title)
	}
	if added.Space != origin.DefaultSpaceKey {
		t.Errorf("space = %q, want %q", added.Space, origin.DefaultSpaceKey)
	}

	sout, serr := capture(t, func() error {
		return cmdMemory([]string{"search", "rhythm", "--json"})
	})
	if serr != nil {
		t.Fatalf("memory search --json: %v\n%s", serr, sout)
	}
	var row struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Space     string `json:"space"`
		Snippet   string `json:"snippet"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(sout)), &row); err != nil {
		t.Fatalf("search --json not one object per row: %v\n%s", err, sout)
	}
	if row.ID != added.ID || row.Title != added.Title {
		t.Errorf("search --json row %+v does not match added %+v", row, added)
	}
}

// TestMemorySearchNoMatchSaysSo: 0 hits is an answer, not silence — one
// line that says so and names the verb that would fill the space.
func TestMemorySearchNoMatchSaysSo(t *testing.T) {
	localOriginMemory(t)

	out, err := capture(t, func() error {
		return cmdMemory([]string{"search", "quantum-flensing-ritual"})
	})
	if err != nil {
		t.Fatalf("memory search 0-hit: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("0 hits must print exactly one line, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "no memory") || !strings.Contains(lines[0], "memory add") {
		t.Errorf("the notice must say no memory and point at memory add: %q", lines[0])
	}
}

// TestMemoryAddRefusesConnectedWithoutSpace: a connected workspace with no
// memory.space gets a refusal that says why and how to fix it — never a
// quiet write to whatever team space happens to be first.
func TestMemoryAddRefusesConnectedWithoutSpace(t *testing.T) {
	mirror(t, "https://team.example.com")

	_, err := capture(t, func() error {
		return cmdMemory([]string{"add", "note the team should not see"})
	})
	if err == nil {
		t.Fatal("connected + unset memory.space must refuse, got success")
	}
	msg := err.Error()
	for _, want := range []string{"memory.space is not set", "gadak config set memory.space", "team"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q: %v", want, msg)
		}
	}
	t.Log(msg)
}

// TestMemorySearchRefusesConnectedWithoutSpace: search shares the space
// resolution, so the same refusal guards the read — an unscoped "search
// everything" is not what the verb promises.
func TestMemorySearchRefusesConnectedWithoutSpace(t *testing.T) {
	mirror(t, "https://team.example.com")

	_, err := capture(t, func() error {
		return cmdMemory([]string{"search", "anything"})
	})
	if err == nil {
		t.Fatal("connected + unset memory.space must refuse on search too")
	}
	if !strings.Contains(err.Error(), "memory.space is not set") {
		t.Errorf("got %v", err)
	}
}

// TestMemoryAddTitleDerivedFromFirstLine: no --title means the first line
// is the title, whitespace-collapsed and clipped when overly long.
func TestMemoryAddTitleDerivedFromFirstLine(t *testing.T) {
	localOriginMemory(t)

	out, err := capture(t, func() error {
		return cmdMemory([]string{"add", "short title line\nthe body that actually carries the detail"})
	})
	if err != nil {
		t.Fatalf("memory add: %v\n%s", err, out)
	}
	if got := strings.Split(strings.TrimSpace(out), "\t")[1]; got != "short title line" {
		t.Errorf("title = %q, want the first line only", got)
	}

	long := "this first line just keeps going and going past every reasonable page title width so it must be clipped " + strings.Repeat("x", 60)
	out, err = capture(t, func() error {
		return cmdMemory([]string{"add", long})
	})
	if err != nil {
		t.Fatalf("memory add long: %v\n%s", err, out)
	}
	got := strings.Split(strings.TrimSpace(out), "\t")[1]
	if len([]rune(got)) > 80 {
		t.Errorf("derived title must be clipped to 80 runes, got %d: %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a clipped title must end in an ellipsis, got %q", got)
	}
}

// TestMemoryAddStdinDash: `-m -` reads the note from stdin, the same idiom
// every write verb shares.
func TestMemoryAddStdinDash(t *testing.T) {
	localOriginMemory(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("stdin note body\n")
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	out, cerr := capture(t, func() error { return cmdMemory([]string{"add", "-m", "-"}) })
	os.Stdin = saved
	if cerr != nil {
		t.Fatalf("memory add -m -: %v\n%s", cerr, out)
	}
	if !strings.Contains(out, "stdin note body") {
		t.Errorf("stdin note not reflected in the title: %q", out)
	}
}

// TestMemoryAddGivenTwiceRefused: the note as a positional and as -m at the
// same time is the comment verb's "pick one" refusal, not a concatenation.
func TestMemoryAddGivenTwiceRefused(t *testing.T) {
	localOriginMemory(t)

	_, err := capture(t, func() error {
		return cmdMemory([]string{"add", "positional note", "-m", "flag note"})
	})
	if err == nil {
		t.Fatal("positional + -m must refuse")
	}
	if !strings.Contains(err.Error(), "pick one") {
		t.Errorf("got %v", err)
	}
}

// TestMemorySearchScopedToConfiguredSpace: the scope is the space filter —
// pages in other spaces answer `search`, not `memory search`, even when
// their bodies match better.
func TestMemorySearchScopedToConfiguredSpace(t *testing.T) {
	localOriginMemory(t)

	home := os.Getenv("GADAK_HOME")
	db, err := store.Open(home + "/gadak.db")
	if err != nil {
		t.Fatal(err)
	}
	seed := func(ext, spaceKey, title, body string) store.PageRecord {
		return store.PageRecord{
			Item: store.Item{
				ID: "confluence:" + ext, SourceID: "confluence", Kind: "page", ExternalID: ext, Key: ext,
				Title: title, BodyText: body,
				CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Page: store.Page{SpaceKey: spaceKey, Version: 1, Status: "current"},
		}
	}
	if _, err := db.UpsertPages(context.Background(), []store.PageRecord{
		seed("101", "MEM", "memory hit", "the quokka census notes live here"),
		seed("102", "ENG", "team hit", "the quokka census runbook lives in the team space"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := capture(t, func() error {
		return cmdConfig([]string{"set", "memory.space", "MEM"})
	}); err != nil {
		t.Fatalf("config set memory.space: %v\n%s", err, out)
	}

	out, err := capture(t, func() error {
		return cmdMemory([]string{"search", "quokka"})
	})
	if err != nil {
		t.Fatalf("memory search: %v\n%s", err, out)
	}
	if !strings.Contains(out, "memory hit") {
		t.Errorf("the memory-space page must be found:\n%s", out)
	}
	if strings.Contains(out, "team hit") {
		t.Errorf("a page outside memory.space must not answer memory search:\n%s", out)
	}
}

// TestMemoryAddEmptyNoteRefused: whitespace is not a note.
func TestMemoryAddEmptyNoteRefused(t *testing.T) {
	localOriginMemory(t)

	_, err := capture(t, func() error { return cmdMemory([]string{"add", "   "}) })
	if err == nil {
		t.Fatal("an empty note must refuse")
	}
}
