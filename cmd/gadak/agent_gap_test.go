package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// Read-side write-gap gates (2026-09-10):
//
//  GDK-1734  `gadak issue` link rows print the wire pair "Blocks outward"
//            instead of the sentence the type carries ("blocks")
//            TestIssueLinksPrintTypePhrases
//  GDK-843   the ambiguous-@mention refusal does not say a literal @word can
//            be written in backticks
//            TestAmbiguousMentionTeachesBackticks
//  GDK-959   the mirror cookbook never names the description columns, so
//            agents write `description` and eat a SQL error
//            TestMirrorCookbookNamesDescriptionColumns
//  GDK-257   `gadak issue --help` does not say the output already carries
//            comments and history, so agents guess at a --comments flag
//            TestIssueHelpSaysCommentsAndHistory
//  GDK-595   sqlhint stays silent on json_each's own key column colliding
//            with issues_full.key
//            TestSQLAmbiguousJSONEachGetsHint

func TestIssueLinksPrintTypePhrases(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	// The unit fixture seeds no link-type catalog; sync fills it on real
	// mirrors (schemaV43). One Blocks row is the phrase source.
	home := os.Getenv("GADAK_HOME")
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1002", SourceID: "jira", Kind: "issue", ExternalID: "1002", Key: "NMB-2",
				Title: "linked target", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "NMB", StatusCategory: "new"},
		}},
		LinkTypes: []store.LinkType{{ID: "10000", Name: "Blocks", Inward: "is blocked by", Outward: "blocks"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdIssue([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// The fixture's link on NMB-1 is Blocks outward → the type's outward
	// sentence is "blocks" (GDK-1734). The wire pair must not come back.
	if !strings.Contains(out, "\n  blocks\tNMB-2\t") {
		t.Fatalf("link line does not use the type's sentence:\n%s", out)
	}
	if strings.Contains(out, "Blocks outward") {
		t.Fatalf("link line still prints the wire pair:\n%s", out)
	}
}

func TestAmbiguousMentionTeachesBackticks(t *testing.T) {
	f := newFakeJira(t)
	f.searchUsers = func(string) string {
		return `[{"accountId":"acc-mr","displayName":"Marco Reyes","active":true},
			{"accountId":"acc-mb","displayName":"Marco Bright","active":true}]`
	}
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "ping @Marco"})
	})
	if err == nil || !strings.Contains(err.Error(), "matches 2 users") {
		t.Fatalf("err = %v, want the ambiguous-mention refusal", err)
	}
	// GDK-843: the refusal must teach the escape for a literal @word.
	if !strings.Contains(err.Error(), "backticks") {
		t.Fatalf("err = %v, want the backticks sentence", err)
	}
	noWriteTag(t, f)
}

func TestMirrorCookbookNamesDescriptionColumns(t *testing.T) {
	body, err := os.ReadFile("../../docs/MIRROR.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	// GDK-959: `description` is not a column; the cookbook is the schema
	// surface agents read, so it must name the real pair at least once.
	for _, want := range []string{"description_text", "description_adf"} {
		if !strings.Contains(text, want) {
			t.Errorf("docs/MIRROR.md does not name %s — agents keep writing `description` and eating a SQL error", want)
		}
	}
}

func TestIssueHelpSaysCommentsAndHistory(t *testing.T) {
	out, err := capture(t, func() error { return cmdIssue([]string{"--help"}) })
	if err != nil {
		t.Fatalf("issue --help: %v", err)
	}
	// GDK-257: the output already carries comments and history; the help
	// saying so is what stops an agent from guessing a --comments flag.
	for _, want := range []string{"comments", "history"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue --help does not mention %q:\n%s", want, out)
		}
	}
}

func TestSQLAmbiguousJSONEachGetsHint(t *testing.T) {
	mirror(t, "https://unused.example.com")

	_, err := capture(t, func() error {
		return cmdSQL([]string{"select key from issues_full, json_each(labels) where json_each.value='batch'"})
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous column name: key") {
		t.Fatalf("err = %v, want the ambiguous-column error", err)
	}
	// GDK-595: the hint names json_each's own columns and the fix.
	if !strings.Contains(err.Error(), "json_each exposes") {
		t.Fatalf("err = %v, want the json_each hint", err)
	}

	// The same ambiguity without json_each must stay unhinted — the hint is
	// scoped to the table-valued function's key/value columns.
	_, err = capture(t, func() error {
		return cmdSQL([]string{"select key from issues_full, items"})
	})
	if err == nil || !strings.Contains(err.Error(), "ambiguous column name") {
		t.Fatalf("err = %v, want the ambiguous-column error", err)
	}
	if strings.Contains(err.Error(), "hint:") {
		t.Fatalf("unrelated ambiguity got the json_each hint: %v", err)
	}
}
