package main

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-816 FAIL-first: the TTY search page row used to be prose
// ("page  SPACE/Title  URL") with no identifier, so the value the default
// read surface printed could not be pasted into `gadak page edit <ID>` —
// the write wants the origin page id, which is PageLite.Key (items.key).
// The row must be TSV with the key in field 1, sharing the issue rows' key
// column (docs/MIRROR.md: cut -f1 gives keys).
func TestSearchTTYPageRowLeadsWithKey(t *testing.T) {
	saved := searchIsTTY
	searchIsTTY = func() bool { return true }
	t.Cleanup(func() { searchIsTTY = saved })

	pages := []store.PageLite{{
		Key: "1731040459", SpaceKey: "LOC", Title: "Retention notes",
		URL: "https://nimbus.example.com/wiki/spaces/LOC/pages/1731040459",
	}}
	out, err := capture(t, func() error {
		printSearchText(nil, pages, nil, nil, false, 0)
		return nil
	})
	if err != nil {
		t.Fatalf("printSearchText: %v", err)
	}
	line := strings.SplitN(strings.TrimRight(out, "\n"), "\n", 2)[0]
	fields := strings.Split(line, "\t")
	if len(fields) < 2 || fields[0] != "1731040459" {
		t.Fatalf("page row = %q, want field 1 = the page key (origin page id) so cut -f1 works", line)
	}
	if fields[1] != "page" {
		t.Fatalf("page row field 2 = %q, want the literal kind marker \"page\"", fields[1])
	}
	// The origin URL the old prose line carried must survive the TSV switch.
	if !strings.Contains(line, pages[0].URL) {
		t.Fatalf("page row lost the origin URL: %q", line)
	}
}

// GDK-816: issue rows and page rows must share the key column — one
// `cut -f1` over a mixed result answers keys for both kinds.
func TestSearchTTYIssueAndPageRowsShareKeyColumn(t *testing.T) {
	saved := searchIsTTY
	searchIsTTY = func() bool { return true }
	t.Cleanup(func() { searchIsTTY = saved })

	who := "Dana Whitfield"
	lites := []store.IssueLite{{
		IssueKey: "NMB-1", Status: "진행 중", Assignee: &who,
		Summary: "batch worker drops the last page",
	}}
	pages := []store.PageLite{{
		Key: "1731040459", SpaceKey: "LOC", Title: "Retention notes", URL: "https://nimbus.example.com/x",
	}}
	out, err := capture(t, func() error {
		printSearchText(lites, pages, nil, nil, false, 0)
		return nil
	})
	if err != nil {
		t.Fatalf("printSearchText: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header + 2 rows, got %d: %q", len(lines), out)
	}
	if lines[0] != searchTSVHeader {
		t.Fatalf("header = %q, want %q (issue column contract unchanged)", lines[0], searchTSVHeader)
	}
	for i, want := range map[int]string{1: "NMB-1", 2: "1731040459"} {
		got := strings.SplitN(lines[i], "\t", 2)[0]
		if got != want {
			t.Errorf("row %d field 1 = %q, want %q", i, got, want)
		}
	}
}
