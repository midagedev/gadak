package create

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jira"
)

func priList() []jira.NamedID {
	return []jira.NamedID{
		{ID: "1", Name: "Highest"},
		{ID: "2", Name: "High"},
		{ID: "3", Name: "Medium"},
	}
}

func TestPriorityMatchesNameAndID(t *testing.T) {
	list := priList()
	cases := []struct {
		want, id string
	}{
		{"High", "2"},
		{"high", "2"},
		{"1", "1"},
		{"  Medium ", "3"},
	}
	for _, tc := range cases {
		id, err := Priority(tc.want, list)
		if err != nil || id != tc.id {
			t.Fatalf("Priority(%q) = %q, %v; want %q", tc.want, id, err, tc.id)
		}
	}
}

func TestPriorityMatchesLocalizedName(t *testing.T) {
	list := []jira.NamedID{
		{ID: "1", Name: "가장 높음"},
		{ID: "2", Name: "높음"},
	}
	id, err := Priority("높음", list)
	if err != nil || id != "2" {
		t.Fatalf("got %q %v", id, err)
	}
	if _, err := Priority("High", list); err == nil {
		t.Fatal("English name must not match a Korean catalog")
	}
}

func TestPriorityEmptyListsCatalog(t *testing.T) {
	_, err := Priority("", priList())
	if err == nil || !strings.Contains(err.Error(), "pass --priority") || !strings.Contains(err.Error(), "Highest (id 1)") {
		t.Fatalf("empty: %v", err)
	}
}

func TestPriorityUnmatchedListsCatalog(t *testing.T) {
	_, err := Priority("Urgent", priList())
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{`no priority matching "Urgent"`, "Highest (id 1)", "High (id 2)", "Medium (id 3)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestPriorityFieldIsID(t *testing.T) {
	got := PriorityField("2")
	if got["id"] != "2" || len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if _, ok := got["name"]; ok {
		t.Fatalf("name leaked: %v", got)
	}
}
