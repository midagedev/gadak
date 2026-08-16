package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
)

func TestIdsToAliasFirstAliasWins(t *testing.T) {
	got := idsToAlias([]config.FieldSpec{
		{Alias: "severity", IDs: []string{"customfield_10", "customfield_20"}},
		{Alias: "other", IDs: []string{"customfield_10"}},
		{Alias: "", IDs: []string{"customfield_30"}},
	})
	want := map[string]string{
		"customfield_10": "other", // id sort then alias sort: other < severity
		"customfield_20": "severity",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSampleIssueKeysDeterministicAndStratified(t *testing.T) {
	// Three projects with uneven sizes; created_at spans each project's history.
	var rows []issueSampleRow
	add := func(proj string, n int) {
		for i := 0; i < n; i++ {
			rows = append(rows, issueSampleRow{
				Key:        proj + "-" + itoa(i+1),
				ProjectKey: proj,
				CreatedAt:  "2020-01-01T00:00:00.000Z", // overwritten below for span
			})
		}
	}
	add("AAA", 10)
	add("BBB", 20)
	add("CCC", 5)
	// Spread created_at within each project so spacing is meaningful.
	for i := range rows {
		rows[i].CreatedAt = sprintfCreated(i)
	}

	// Same input twice → identical sample.
	a := sampleIssueKeys(rows, 12)
	b := sampleIssueKeys(rows, 12)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("not deterministic:\n  %v\n  %v", a, b)
	}
	if len(a) != 12 {
		t.Fatalf("len = %d, want 12", len(a))
	}

	// Every project should appear when n is large enough.
	seen := map[string]int{}
	for _, k := range a {
		proj := k[:3]
		seen[proj]++
	}
	for _, p := range []string{"AAA", "BBB", "CCC"} {
		if seen[p] == 0 {
			t.Errorf("project %s missing from sample %v", p, a)
		}
	}
	// Largest project should get the largest share.
	if seen["BBB"] < seen["AAA"] || seen["BBB"] < seen["CCC"] {
		t.Errorf("expected BBB to get the most slots, got %v", seen)
	}

	// Within one project, samples should span the created order (not only the newest).
	// Rebuild AAA-only pool ordered by created_at.
	var aaa []issueSampleRow
	for _, r := range rows {
		if r.ProjectKey == "AAA" {
			aaa = append(aaa, r)
		}
	}
	// Force chronological order by key number for this assertion.
	for i := range aaa {
		aaa[i].CreatedAt = sprintfCreated(i)
	}
	pick := sampleIssueKeys(aaa, 3)
	if len(pick) != 3 {
		t.Fatalf("aaa sample len = %d", len(pick))
	}
	// evenIndices(10, 3) → 0, 4, 9 → AAA-1, AAA-5, AAA-10
	want := []string{"AAA-1", "AAA-5", "AAA-10"}
	// sampleIssueKeys sorts output keys, so order is lexical not index order.
	gotSet := map[string]bool{}
	for _, k := range pick {
		gotSet[k] = true
	}
	for _, w := range want {
		if !gotSet[w] {
			t.Errorf("expected %s in spaced sample %v", w, pick)
		}
	}

	// Taking all issues returns every key.
	all := sampleIssueKeys(rows, 1000)
	if len(all) != len(rows) {
		t.Errorf("full sample: got %d want %d", len(all), len(rows))
	}
}

func TestEvenIndices(t *testing.T) {
	if got := evenIndices(10, 3); !reflect.DeepEqual(got, []int{0, 4, 9}) {
		t.Errorf("evenIndices(10,3) = %v", got)
	}
	if got := evenIndices(10, 1); !reflect.DeepEqual(got, []int{5}) {
		t.Errorf("evenIndices(10,1) = %v", got)
	}
	if got := evenIndices(3, 5); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Errorf("evenIndices(3,5) = %v", got)
	}
}

func TestIsFilled(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{`null`, false},
		{`""`, false},
		{`[]`, false},
		{`{}`, false},
		{`0`, true},
		{`false`, true},
		{`true`, true},
		{`1`, true},
		{`"hello"`, true},
		{`[1]`, true},
		{`{"a":1}`, true},
		{``, false},
	}
	for _, tc := range cases {
		got := fields.IsFilled(json.RawMessage(tc.raw))
		if got != tc.want {
			t.Errorf("IsFilled(%s) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

func TestSuggestAliasCollisionAppendsTail(t *testing.T) {
	used := map[string]bool{"story_points": true}
	got := fields.SuggestAlias("Story Points", "customfield_10016", used)
	if got != "story_points_10016" {
		t.Errorf("got %q, want story_points_10016", got)
	}
	// No collision → plain snake_case.
	got2 := fields.SuggestAlias("Epic Link", "customfield_10014", used)
	if got2 != "epic_link" {
		t.Errorf("got %q, want epic_link", got2)
	}
	// Second collision on the tailed form.
	used[got] = true
	got3 := fields.SuggestAlias("Story Points", "customfield_10016", used)
	if got3 != "story_points_10016_2" {
		t.Errorf("got %q, want story_points_10016_2", got3)
	}
}

func TestAsciiSlugDropsNonASCII(t *testing.T) {
	if got := fields.ASCIISlug("Story Points"); got != "story_points" {
		t.Errorf("got %q", got)
	}
	if got := fields.ASCIISlug("  CF — Something!! "); got != "cf_something" {
		t.Errorf("got %q", got)
	}
	// Jira localizes field names, so a Korean account sees "순위" where an
	// English one sees "Rank". An alias built from that name would differ per
	// machine, and a fields spec is shared between machines.
	if got := fields.ASCIISlug("순위"); got != "" {
		t.Errorf("ASCIISlug(%q) = %q, want empty so the caller falls back to the id", "순위", got)
	}
	if got := fields.SuggestAlias("순위", "customfield_10019", map[string]bool{}); got != "cf_10019" {
		t.Errorf("SuggestAlias for a non-ASCII name = %q, want cf_10019", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func sprintfCreated(i int) string {
	// Lexicographically ordered timestamps.
	return "2020-01-" + pad2(i%28+1) + "T00:00:00.000Z"
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}
