package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// localOriginPages is the page-verb fixture: a local-origin workspace (the
// in-process origin, no network) with one page created through the real
// write path. It returns the created page id.
func localOriginPages(t *testing.T) string {
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
	out, err := capture(t, func() error {
		return cmdPage([]string{"create", "--space", origin.DefaultSpaceKey, "--title", "Retention notes", "-m", "first draft"})
	})
	if err != nil {
		t.Fatalf("page create: %v\n%s", err, out)
	}
	id := strings.SplitN(strings.TrimSpace(out), "\t", 2)[0]
	if id == "" {
		t.Fatalf("page create printed no id: %q", out)
	}
	return id
}

func TestPageGetPrintsTitleAndBody(t *testing.T) {
	id := localOriginPages(t)
	out, err := capture(t, func() error { return cmdPage([]string{"get", id}) })
	if err != nil {
		t.Fatalf("page get: %v\n%s", err, out)
	}
	// printIssue's rhythm: headline id\ttitle, kv header, indented body
	// section, comments count.
	for _, want := range []string{
		id + "\tRetention notes",
		"space         LOC",
		"body\n    first draft",
		"\ncomments (0)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "description") {
		t.Errorf("page output borrowed the issue's description header:\n%s", out)
	}
}

func TestPageGetJSONIsTheDetailDocument(t *testing.T) {
	id := localOriginPages(t)
	out, err := capture(t, func() error { return cmdPage([]string{"get", id, "--json"}) })
	if err != nil {
		t.Fatalf("page get --json: %v\n%s", err, out)
	}
	var doc struct {
		Key      string `json:"key"`
		SpaceKey string `json:"space_key"`
		Title    string `json:"title"`
		Version  int    `json:"version"`
		BodyText string `json:"body_text"`
		Comments []any  `json:"comments"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if doc.Key != id || doc.Title != "Retention notes" || doc.SpaceKey != origin.DefaultSpaceKey {
		t.Errorf("doc = %+v", doc)
	}
	if doc.BodyText != "first draft" {
		t.Errorf("body_text = %q", doc.BodyText)
	}
	if doc.Comments == nil || len(doc.Comments) != 0 {
		t.Errorf("comments = %#v, want []", doc.Comments)
	}
}

func TestPageGetUnknownIDListsNextStep(t *testing.T) {
	localOriginPages(t)
	_, err := capture(t, func() error { return cmdPage([]string{"get", "99999"}) })
	if err == nil {
		t.Fatal("unknown page id must fail")
	}
	if !strings.Contains(err.Error(), "no page 99999 in the mirror") ||
		!strings.Contains(err.Error(), "gadak page list") {
		t.Fatalf("err = %v, want the page list next step", err)
	}
}

func TestPageGetEmptyBodySaysSo(t *testing.T) {
	// GDK-1020 contract: absence is an answer — the body section keeps its
	// line with the (none) marker, not silence.
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
	out, err := capture(t, func() error {
		return cmdPage([]string{"create", "--space", origin.DefaultSpaceKey, "--title", "Empty"})
	})
	if err != nil {
		t.Fatalf("page create: %v\n%s", err, out)
	}
	id := strings.SplitN(strings.TrimSpace(out), "\t", 2)[0]
	out, err = capture(t, func() error { return cmdPage([]string{"get", id}) })
	if err != nil {
		t.Fatalf("page get: %v\n%s", err, out)
	}
	if !strings.Contains(out, "\nbody (none)\n") {
		t.Fatalf("empty body must carry its marker:\n%s", out)
	}
}

func TestPageGetShowsComments(t *testing.T) {
	id := localOriginPages(t)
	if _, err := capture(t, func() error {
		return cmdPage([]string{"comment", id, "-m", "a question"})
	}); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error { return cmdPage([]string{"get", id}) })
	if err != nil {
		t.Fatalf("page get: %v\n%s", err, out)
	}
	if !strings.Contains(out, "\ncomments (1)\n") || !strings.Contains(out, "a question") {
		t.Fatalf("comment missing from:\n%s", out)
	}
}

func TestPageListRowsAndSpaceFilter(t *testing.T) {
	id := localOriginPages(t)
	out, err := capture(t, func() error { return cmdPage([]string{"list"}) })
	if err != nil {
		t.Fatalf("page list: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "id\tspace\ttitle\tupdated_at" {
		t.Fatalf("header = %q", lines[0])
	}
	if len(lines) != 2 || !strings.HasPrefix(lines[1], id+"\t"+origin.DefaultSpaceKey+"\tRetention notes\t") {
		t.Fatalf("rows = %#v", lines)
	}

	// Case-insensitive on purpose (formatPageSpaceError matches space keys
	// with EqualFold); a wrong case returning zero rows silently is the
	// display-name trap in miniature.
	out, err = capture(t, func() error {
		return cmdPage([]string{"list", "--space", strings.ToLower(origin.DefaultSpaceKey)})
	})
	if err != nil {
		t.Fatalf("page list --space: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Retention notes") {
		t.Fatalf("lowercase space key lost the row:\n%s", out)
	}

	out, err = capture(t, func() error { return cmdPage([]string{"list", "--space", "NOPE"}) })
	if err != nil {
		t.Fatalf("page list --space NOPE: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "(no pages in the mirror — sync first, or create one: gadak page create --space K --title T -m TEXT)" {
		t.Fatalf("no-hit space must print the empty marker, got %q", out)
	}
}

func TestPageListEmptyWorkspaceSaysSo(t *testing.T) {
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
	// Fresh local-origin seeds no pages (verified: select count(*) from pages
	// = 0). The text form says so — views-list stance, one tab-free line.
	out, err := capture(t, func() error { return cmdPage([]string{"list"}) })
	if err != nil {
		t.Fatalf("page list: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "(no pages in the mirror — sync first, or create one: gadak page create --space K --title T -m TEXT)" {
		t.Fatalf("empty mirror must print the marker, got %q", out)
	}
	// The parsed formats keep their zero-row stream contracts.
	out, err = capture(t, func() error { return cmdPage([]string{"list", "--json"}) })
	if err != nil {
		t.Fatalf("page list --json: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("--json on zero rows must print nothing, got %q", out)
	}
}

func TestPageUnknownSubcommandNamesReadsToo(t *testing.T) {
	_, err := capture(t, func() error { return cmdPage([]string{"print", "1"}) })
	if err == nil {
		t.Fatal("unknown subcommand must fail")
	}
	if !strings.Contains(err.Error(), "gadak page get|list|create|edit|comment") {
		t.Fatalf("err = %v, want the read verbs in the hint", err)
	}
}
