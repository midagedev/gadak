package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// Contract ↔ assertion (GDK-161; clause numbers match the task spec):
//
//  1. Allowed tokens are exactly new|inprogress|done (case-insensitive).
//     Aliases (todo, in-progress, in_progress, indeterminate) are not tokens.
//     TestTransitionCategoryTokensAccepted
//     TestTransitionRejectsAliases
//  2. Two or more transitions into the same category → refuse, list them.
//     TestTransitionCategorySingleMatchExecutes
//     TestTransitionCategoryAmbiguousRefuses
//  3. Match order is transition id, then target status id, then name,
//     then target status name, then category.
//     TestTransitionIDBeatsCategory
//     TestTransitionNameBeatsCategory
//     TestTransitionEnglishDoneNameSkipsAmbiguity
//     TestTransitionMatchesByStatusID
//     TestTransitionIDBeatsStatusIDShapedNumber
//     TestTransitionIDAndStatusIDCollisionRefuses
//  4. Paths ①②③ are unchanged (no regression).
//     TestTransitionMatchesByNameAndReportsAlternatives (agent_test.go: "완료")
//     TestTransitionMatchesByID
//     TestTransitionMatchesByTransitionName
//  5. A miss still lists candidates and names reachable category tokens.
//     TestTransitionNoMatchHintsReachableCategories
//     TestTransitionCategoryZeroMatchHintsOthers
//     TestTransitionEmptyListHasNoCategoryHint
//
// Coverage (case × path):
//
//	id            TestTransitionMatchesByID
//	name          TestTransitionMatchesByTransitionName
//	status name   TestTransitionMatchesByNameAndReportsAlternatives, TestTransitionEnglishDoneNameSkipsAmbiguity
//	category      TestTransitionCategoryTokensAccepted
//	ambiguous     TestTransitionCategoryAmbiguousRefuses
//	no match      TestTransitionNoMatchHintsReachableCategories
//	category 0    TestTransitionCategoryZeroMatchHintsOthers
//
// Input defence:
//
//	malicious   TestTransitionRejectsMaliciousTarget   (done; DROP, 10k-byte string)
//	damaged     TestTransitionEmptyOrUnknownCategoryKeyDoesNotMatchNew
//	stale schema TestTransitionMissingToDoesNotMatchCategory
//
// Self-review defect classes:
//
//  1. Feeding the user token through jira.Category would accept "todo"/"undefined"
//     as "new" (Category's default fold). TestTransitionRejectsAliases.
//  2. A status literally named "Done" plus a second done-category transition
//     must take the name path and must not apply the category-ambiguity rule.
//     TestTransitionEnglishDoneNameSkipsAmbiguity.
//  3. A transition *named* "new" that lands in done must fire that transition,
//     not the category-new one. TestTransitionNameBeatsCategory.

func withTransitions(t *testing.T, raw string) *fakeJira {
	t.Helper()
	f := newFakeJira(t)
	f.transitionsJSON = raw
	mirror(t, f.URL)
	return f
}

func postedTransitionID(t *testing.T, f *fakeJira, key string) string {
	t.Helper()
	tag := "POST /issue/" + key + "/transitions"
	body := f.bodies[tag]
	var sent struct {
		Transition struct {
			ID string `json:"id"`
		} `json:"transition"`
	}
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("decode %s %q: %v", tag, body, err)
	}
	if sent.Transition.ID == "" {
		t.Fatalf("empty transition id in %s", body)
	}
	return sent.Transition.ID
}

func mustNotTransition(t *testing.T, f *fakeJira, key string) {
	t.Helper()
	tag := "POST /issue/" + key + "/transitions"
	if f.called(tag) {
		t.Fatalf("must not POST %s; body %s", tag, f.bodies[tag])
	}
}

func TestTransitionFieldResolvesAlias(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Fields = []config.FieldSpec{
		{Alias: "severity", Label: "Severity", IDs: []string{"customfield_10001"}, Role: "facet", Kind: "option"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "31", "--field", "severity=High"})
	}); err != nil {
		t.Fatalf("transition --field severity=High: %v", err)
	}
	body := f.bodies["POST /issue/NMB-1/transitions"]
	if !strings.Contains(body, `"customfield_10001"`) {
		t.Fatalf("alias was not resolved to customfield id: %s", body)
	}
	if strings.Contains(body, `"severity"`) {
		t.Fatalf("alias name leaked into transition fields: %s", body)
	}
}

func TestTransitionCategoryTokensAccepted(t *testing.T) {
	// Default fake: Start work → indeterminate ("진행 중"), Close → done ("완료").
	// "done" / "inprogress" are category fallbacks; Korean names never equal-fold them.
	cases := []struct {
		arg, wantID string
	}{
		{"done", "31"},
		{"DONE", "31"},
		{"Done", "31"},
		{"inprogress", "21"},
		{"INPROGRESS", "21"},
	}
	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			f := newFakeJira(t)
			// GDK-632: a category target checks the current status before
			// firing, and the fake's default is 진행 중 — which would make the
			// inprogress cases legitimate no-ops. This test is about token
			// acceptance, so pin the current status to new and let every
			// case really write.
			f.issueStatusJSON = `{"fields":{"status":{"id":"1","name":"할 일","statusCategory":{"key":"new"}},"assignee":null}}`
			mirror(t, f.URL)
			if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", tc.arg}) }); err != nil {
				t.Fatalf("transition %q: %v", tc.arg, err)
			}
			if got := postedTransitionID(t, f, "NMB-1"); got != tc.wantID {
				t.Fatalf("posted id %q, want %q", got, tc.wantID)
			}
		})
	}

	// "new" needs a transition whose Jira key is actually "new".
	f := withTransitions(t, `{"transitions":[
		{"id":"11","name":"Backlog","to":{"id":"1","name":"해야 할 일","statusCategory":{"key":"new"}}},
		{"id":"31","name":"Close","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}}]}`)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "new"}) }); err != nil {
		t.Fatalf("transition new: %v", err)
	}
	if got := postedTransitionID(t, f, "NMB-1"); got != "11" {
		t.Fatalf("posted id %q, want 11", got)
	}
}

func TestTransitionRejectsAliases(t *testing.T) {
	// A real "new" and "inprogress" landing exist, so a fold through
	// jira.Category (or jql.mapStatusCategory) would silently execute.
	f := withTransitions(t, `{"transitions":[
		{"id":"11","name":"Backlog","to":{"id":"1","name":"해야 할 일","statusCategory":{"key":"new"}}},
		{"id":"21","name":"Start work","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}},
		{"id":"31","name":"Close","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}}]}`)
	for _, arg := range []string{"todo", "in-progress", "in_progress", "indeterminate"} {
		t.Run(arg, func(t *testing.T) {
			f.calls = nil
			f.bodies = map[string]string{}
			_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", arg}) })
			if err == nil {
				t.Fatalf("%q must not match a category token", arg)
			}
			mustNotTransition(t, f, "NMB-1")
			if !strings.Contains(err.Error(), "no transition matching") {
				t.Fatalf("error %q", err)
			}
		})
	}
}

func TestTransitionCategorySingleMatchExecutes(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done"}) }); err != nil {
		t.Fatalf("done: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "31" {
		t.Fatal("exactly one done-category transition must run")
	}
}

func TestTransitionCategoryAmbiguousRefuses(t *testing.T) {
	f := withTransitions(t, `{"transitions":[
		{"id":"31","name":"Close","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}},
		{"id":"41","name":"폐기","to":{"id":"10002","name":"폐기됨","statusCategory":{"key":"done"}}}]}`)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done"}) })
	if err == nil {
		t.Fatal("two done-category landings must refuse")
	}
	mustNotTransition(t, f, "NMB-1")
	msg := err.Error()
	for _, want := range []string{
		`transition "done" is ambiguous on NMB-1`,
		"2 transitions land there",
		"Close (id 31, → 완료 [status_id 10001])",
		"폐기 (id 41, → 폐기됨 [status_id 10002])",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestTransitionMatchesByID(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "21"}) }); err != nil {
		t.Fatalf("id 21: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "21" {
		t.Fatal("id match must keep posting that id")
	}
}

func TestTransitionMatchesByStatusID(t *testing.T) {
	// GDK-313: sql hands out status_id; that id is to.id, not the transition id.
	f := withTransitions(t, `{"transitions":[
		{"id":"21","name":"Start work","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}},
		{"id":"41","name":"Close","to":{"id":"10003","name":"완료","statusCategory":{"key":"done"}}}]}`)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "10003"}) }); err != nil {
		t.Fatalf("status_id 10003: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "41" {
		t.Fatal("target status id must post that landing's transition id")
	}
}

func TestTransitionIDBeatsStatusIDShapedNumber(t *testing.T) {
	// 10003 is a transition id here, not any other transition's to.id.
	f := withTransitions(t, `{"transitions":[
		{"id":"10003","name":"Start work","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}},
		{"id":"41","name":"Close","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}}]}`)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "10003"}) }); err != nil {
		t.Fatalf("transition id 10003: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "10003" {
		t.Fatal("exact transition id must still win")
	}
}

func TestTransitionSameIDAndStatusIDIsNotCollision(t *testing.T) {
	f := withTransitions(t, `{"transitions":[
		{"id":"10003","name":"Close","to":{"id":"10003","name":"완료","statusCategory":{"key":"done"}}}]}`)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "10003"}) }); err != nil {
		t.Fatalf("same-space 10003: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "10003" {
		t.Fatal("id equal to own to.id is not a collision")
	}
}

func TestTransitionIDAndStatusIDCollisionRefuses(t *testing.T) {
	f := withTransitions(t, `{"transitions":[
		{"id":"10003","name":"Start work","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}},
		{"id":"41","name":"Close","to":{"id":"10003","name":"완료","statusCategory":{"key":"done"}}}]}`)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "10003"}) })
	if err == nil {
		t.Fatal("id/status_id overlap must refuse")
	}
	mustNotTransition(t, f, "NMB-1")
	msg := err.Error()
	for _, want := range []string{
		`"10003" matches a transition id and a different target status id on NMB-1`,
		"transition id: Start work (id 10003, → 진행 중 [status_id 3])",
		"target status id: Close (id 41, → 완료 [status_id 10003])",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestTransitionMatchesByTransitionName(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "Close"}) }); err != nil {
		t.Fatalf("name Close: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "31" {
		t.Fatal("transition name must still match")
	}
}

func TestTransitionIDBeatsCategory(t *testing.T) {
	// Literal id "done" plus a second done-category landing: ① fires, ④ does not
	// get to apply the ambiguity rule.
	f := withTransitions(t, `{"transitions":[
		{"id":"done","name":"Close","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}},
		{"id":"41","name":"폐기","to":{"id":"10002","name":"폐기됨","statusCategory":{"key":"done"}}}]}`)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done"}) }); err != nil {
		t.Fatalf("id done: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "done" {
		t.Fatal("exact id must beat category")
	}
}

func TestTransitionNameBeatsCategory(t *testing.T) {
	// Transition named "new" that lands in done, plus a real new-category landing.
	f := withTransitions(t, `{"transitions":[
		{"id":"31","name":"new","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}},
		{"id":"11","name":"Backlog","to":{"id":"1","name":"해야 할 일","statusCategory":{"key":"new"}}}]}`)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "new"}) }); err != nil {
		t.Fatalf("name new: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "31" {
		t.Fatal("transition name must beat the new-category fallback")
	}
}

func TestTransitionEnglishDoneNameSkipsAmbiguity(t *testing.T) {
	f := withTransitions(t, `{"transitions":[
		{"id":"31","name":"Close","to":{"id":"10001","name":"Done","statusCategory":{"key":"done"}}},
		{"id":"41","name":"Decline","to":{"id":"10002","name":"Won't Do","statusCategory":{"key":"done"}}}]}`)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done"}) }); err != nil {
		t.Fatalf("status name Done: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "31" {
		t.Fatal("target status name Done must fire, not the category-ambiguity error")
	}
}

func TestTransitionNoMatchHintsReachableCategories(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "Ship it"}) })
	if err == nil {
		t.Fatal("unmatched must fail")
	}
	mustNotTransition(t, f, "NMB-1")
	msg := err.Error()
	for _, want := range []string{
		`no transition matching "Ship it" on NMB-1`,
		"Start work (id 21, → 진행 중 [status_id 3])",
		"Close (id 31, → 완료 [status_id 10001])",
		"also accepts a status category: inprogress, done",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "new") {
		t.Errorf("hint listed unreachable new: %q", msg)
	}
}

func TestTransitionCategoryZeroMatchHintsOthers(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "new"}) })
	if err == nil {
		t.Fatal("new must miss when no new-category landing exists")
	}
	mustNotTransition(t, f, "NMB-1")
	msg := err.Error()
	if !strings.Contains(msg, `no transition matching "new" on NMB-1`) {
		t.Errorf("error %q", msg)
	}
	if !strings.Contains(msg, "also accepts a status category: inprogress, done") {
		t.Errorf("missing reachable-category hint: %q", msg)
	}
}

func TestTransitionEmptyListHasNoCategoryHint(t *testing.T) {
	f := withTransitions(t, `{"transitions":[]}`)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done"}) })
	if err == nil {
		t.Fatal("empty list must fail")
	}
	mustNotTransition(t, f, "NMB-1")
	msg := err.Error()
	if !strings.Contains(msg, "NMB-1 has no available transitions") {
		t.Errorf("error %q", msg)
	}
	if strings.Contains(msg, "also accepts") {
		t.Errorf("empty list must not advertise categories: %q", msg)
	}
}

func TestTransitionRejectsMaliciousTarget(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done; DROP"}) })
	if err == nil {
		t.Fatal("SQL-shaped token must not match")
	}
	mustNotTransition(t, f, "NMB-1")
	if !strings.Contains(err.Error(), "no transition matching") {
		t.Errorf("error %q", err)
	}

	long := strings.Repeat("a", 10000)
	f.calls = nil
	f.bodies = map[string]string{}
	_, err = capture(t, func() error { return cmdTransition([]string{"NMB-1", long}) })
	if err == nil {
		t.Fatal("long string must not match")
	}
	mustNotTransition(t, f, "NMB-1")
}

func TestTransitionEmptyOrUnknownCategoryKeyDoesNotMatchNew(t *testing.T) {
	// statuscat.Category("") and Category("undefined") both fold to "new". Matching
	// that fold would move the issue on damaged payloads.
	f := withTransitions(t, `{"transitions":[
		{"id":"11","name":"Triage","to":{"id":"1","name":"해야 할 일","statusCategory":{"key":""}}},
		{"id":"12","name":"Mystery","to":{"id":"9","name":"알 수 없음","statusCategory":{"key":"undefined"}}}]}`)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "new"}) })
	if err == nil {
		t.Fatal("empty/unknown StatusCategory.Key must not match new")
	}
	mustNotTransition(t, f, "NMB-1")
	if !strings.Contains(err.Error(), `no transition matching "new"`) {
		t.Errorf("error %q", err)
	}
	// Folding would also invent a reachable "new" in the hint.
	if strings.Contains(err.Error(), "also accepts a status category: new") {
		t.Errorf("hint must not advertise folded new: %q", err)
	}
}

func TestTransitionMissingToDoesNotMatchCategory(t *testing.T) {
	f := withTransitions(t, `{"transitions":[{"id":"11","name":"Triage"}]}`)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "new"}) })
	if err == nil {
		t.Fatal("a transition with no to must not match a category token")
	}
	mustNotTransition(t, f, "NMB-1")

	// Name matching on the same payload still works — the old path is intact.
	f.calls = nil
	f.bodies = map[string]string{}
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "Triage"}) }); err != nil {
		t.Fatalf("name match on missing to: %v", err)
	}
	if postedTransitionID(t, f, "NMB-1") != "11" {
		t.Fatal("name match must survive a missing to")
	}
}

func TestTransitionHelpNamesCategory(t *testing.T) {
	out, err := capture(t, func() error { return cmdTransition([]string{"--help"}) })
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{
		"status category new|inprogress|done",
		"target status id",
		"gadak transition NMB-140 done",
		"<transition-id|status-id|name|new|inprogress|done>",
		"--resolution",
		"--field",
		"-m",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q\n%s", want, out)
		}
	}
}

func TestTransitionKeyOnlyListsAvailable(t *testing.T) {
	withTransitions(t, "")
	out, err := capture(t, func() error { return cmdTransition([]string{"NMB-1"}) })
	if err != nil {
		t.Fatalf("key-only transition must list available, not usage: %v\n%s", err, out)
	}
	for _, want := range []string{"available:", "Start work", "Close", "id 21", "id 31", "inprogress", "done"} {
		if !strings.Contains(out, want) {
			t.Errorf("list missing %q\n%s", want, out)
		}
	}
}

func TestTransitionNoKeyIsUsage(t *testing.T) {
	_, err := capture(t, func() error { return cmdTransition(nil) })
	if err == nil {
		t.Fatal("missing key must be a usage error")
	}
	if !strings.Contains(err.Error(), "usage: gadak transition <KEY>") {
		t.Errorf("usage %q", err)
	}
}

func alreadyDoneOrigin(t *testing.T) *fakeJira {
	t.Helper()
	f := withTransitions(t, `{"transitions":[]}`)
	f.issueStatusJSON = `{"fields":{"status":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},"assignee":null}}`
	return f
}

func TestTransitionAlreadyDoneIsNoop(t *testing.T) {
	f := alreadyDoneOrigin(t)
	out, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "-m", "retry"})
	})
	if err != nil {
		t.Fatalf("already done: %v\n%s", err, out)
	}
	mustNotTransition(t, f, "NMB-1")
	if !strings.Contains(out, "already done — nothing to do") {
		t.Fatalf("human no-op missing: %q", out)
	}
	if !strings.Contains(out, "comment not posted") {
		t.Fatalf("must say the comment was not posted: %q", out)
	}
}

func TestTransitionAlreadyDoneJSON(t *testing.T) {
	f := alreadyDoneOrigin(t)
	out, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "-m", "retry", "--json"})
	})
	if err != nil {
		t.Fatalf("already done --json: %v\n%s", err, out)
	}
	mustNotTransition(t, f, "NMB-1")
	var wrap struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal([]byte(out), &wrap); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if wrap.Changed {
		t.Fatalf("want changed=false: %s", out)
	}
}

func TestTransitionRefreshFailureExitsZeroMirrorStale(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		f.rereadStatus = 422

		stdout, stderr, err := captureBoth(t, func() error {
			return cmdTransition([]string{"NMB-1", "done"})
		})
		if err != nil {
			t.Fatalf("transition must exit 0 after a landed write: %v", err)
		}
		if postedTransitionID(t, f, "NMB-1") != "31" {
			t.Fatal("write must have reached Jira before the refresh warning")
		}
		if !strings.Contains(stderr, "write applied to NMB-1, but the mirror did not refresh") {
			t.Fatalf("stderr missing write-applied warning: %q", stderr)
		}
		want := "NMB-1\t진행 중\tDana Whitfield\tbatch worker drops the last page"
		if strings.TrimSpace(stdout) != want {
			t.Fatalf("stdout %q, want the pre-write mirror row %q", stdout, want)
		}
	})

	t.Run("json", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		f.rereadStatus = 422

		stdout, stderr, err := captureBoth(t, func() error {
			return cmdTransition([]string{"NMB-1", "done", "--json"})
		})
		if err != nil {
			t.Fatalf("transition --json must exit 0 after a landed write: %v", err)
		}
		if postedTransitionID(t, f, "NMB-1") != "31" {
			t.Fatal("write must have reached Jira before the refresh warning")
		}
		if !strings.Contains(stderr, "write applied to NMB-1, but the mirror did not refresh") {
			t.Fatalf("stderr missing write-applied warning: %q", stderr)
		}
		var body struct {
			Issue       store.IssueLite `json:"issue"`
			Changed     bool            `json:"changed"`
			MirrorStale bool            `json:"mirror_stale"`
		}
		if err := json.Unmarshal([]byte(stdout), &body); err != nil {
			t.Fatalf("decode %q: %v", stdout, err)
		}
		if !body.Changed || !body.MirrorStale {
			t.Fatalf("json %+v (raw %q)", body, stdout)
		}
		if body.Issue.IssueKey != "NMB-1" || body.Issue.Status != "진행 중" {
			t.Fatalf("json must carry the pre-write row, got %+v", body.Issue)
		}
	})
}

func TestTransitionWriteJSONReportsChanged(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	out, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "--json"})
	})
	if err != nil {
		t.Fatalf("done --json: %v\n%s", err, out)
	}
	if postedTransitionID(t, f, "NMB-1") != "31" {
		t.Fatal("write path must still POST")
	}
	var wrap struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal([]byte(out), &wrap); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if !wrap.Changed {
		t.Fatalf("write path must report changed=true: %s", out)
	}
}

// requiredResolutionJSON is one done-category transition whose screen requires
// resolution. allowedValues ids are deliberately different from the default
// site catalog so a catalog lookup cannot accidentally satisfy the name test.
const requiredResolutionJSON = `{"transitions":[
	{"id":"41","name":"Resolve","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},
	 "fields":{"resolution":{"required":true,"name":"Resolution","schema":{"type":"resolution"},
	   "allowedValues":[{"id":"10099","name":"Won't Do"},{"id":"10000","name":"Done"}]}}}]}`

func postedTransitionRaw(t *testing.T, f *fakeJira, key string) string {
	t.Helper()
	tag := "POST /issue/" + key + "/transitions"
	body := f.bodies[tag]
	if body == "" {
		t.Fatalf("no POST body for %s; calls %v", tag, f.calls)
	}
	return body
}

func postedTransitionMap(t *testing.T, f *fakeJira, key string) map[string]json.RawMessage {
	t.Helper()
	raw := postedTransitionRaw(t, f, key)
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return sent
}

func TestTransitionRequiredResolutionRefusesWithoutFlag(t *testing.T) {
	f := withTransitions(t, requiredResolutionJSON)
	_, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done"}) })
	if err == nil {
		t.Fatal("required resolution must refuse before POST")
	}
	mustNotTransition(t, f, "NMB-1")
	msg := err.Error()
	for _, want := range []string{"resolution", "Won't Do", "Done"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestTransitionResolutionNameUsesAllowedValues(t *testing.T) {
	f := withTransitions(t, requiredResolutionJSON)
	if _, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "--resolution", "Won't Do"})
	}); err != nil {
		t.Fatalf("resolution name: %v", err)
	}
	sent := postedTransitionMap(t, f, "NMB-1")
	var fields struct {
		Resolution struct {
			ID string `json:"id"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(sent["fields"], &fields); err != nil {
		t.Fatalf("fields %s: %v", sent["fields"], err)
	}
	if fields.Resolution.ID != "10099" {
		t.Fatalf("resolution id %q, want 10099 from allowedValues (not the catalog)", fields.Resolution.ID)
	}
}

func TestTransitionResolutionDigitsAreID(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "--resolution", "10002"})
	}); err != nil {
		t.Fatalf("resolution id: %v", err)
	}
	sent := postedTransitionMap(t, f, "NMB-1")
	var fields struct {
		Resolution struct {
			ID string `json:"id"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(sent["fields"], &fields); err != nil {
		t.Fatalf("fields %s: %v", sent["fields"], err)
	}
	if fields.Resolution.ID != "10002" {
		t.Fatalf("resolution id %q, want 10002 as typed", fields.Resolution.ID)
	}
}

func TestTransitionResolutionNameUsesCatalogWhenNotOnScreen(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "--resolution", "Won't Do"})
	}); err != nil {
		t.Fatalf("catalog name: %v", err)
	}
	sent := postedTransitionMap(t, f, "NMB-1")
	var fields struct {
		Resolution struct {
			ID string `json:"id"`
		} `json:"resolution"`
	}
	if err := json.Unmarshal(sent["fields"], &fields); err != nil {
		t.Fatalf("fields %s: %v", sent["fields"], err)
	}
	if fields.Resolution.ID != "10002" {
		t.Fatalf("resolution id %q, want 10002 from GET /resolution", fields.Resolution.ID)
	}
}

func TestTransitionResolutionUnknownNameListsCatalog(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "--resolution", "Mystery"})
	})
	if err == nil {
		t.Fatal("unknown resolution name must refuse")
	}
	mustNotTransition(t, f, "NMB-1")
	msg := err.Error()
	for _, want := range []string{"Mystery", "Done", "Won't Do"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestTransitionCommentSendsADFUpdate(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "-m", "closing out"})
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	sent := postedTransitionMap(t, f, "NMB-1")
	var update struct {
		Comment []struct {
			Add struct {
				Body json.RawMessage `json:"body"`
			} `json:"add"`
		} `json:"comment"`
	}
	if err := json.Unmarshal(sent["update"], &update); err != nil {
		t.Fatalf("update %s: %v", sent["update"], err)
	}
	if len(update.Comment) != 1 {
		t.Fatalf("comment ops %d, want 1; %s", len(update.Comment), sent["update"])
	}
	want := string(jira.Doc("closing out", nil))
	if string(update.Comment[0].Add.Body) != want {
		t.Fatalf("comment ADF %s, want %s", update.Comment[0].Add.Body, want)
	}
}

func TestTransitionWithoutFlagsOmitsFieldsAndUpdate(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error { return cmdTransition([]string{"NMB-1", "done"}) }); err != nil {
		t.Fatalf("done: %v", err)
	}
	raw := postedTransitionRaw(t, f, "NMB-1")
	if raw != `{"transition":{"id":"31"}}` {
		t.Fatalf("POST body %q, want exactly {\"transition\":{\"id\":\"31\"}}", raw)
	}
}

func TestTransitionFieldJSONAndString(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	if _, err := capture(t, func() error {
		return cmdTransition([]string{
			"NMB-1", "done",
			"--field", `customfield_10010={"id":"3"}`,
			"--field", "environment=staging",
		})
	}); err != nil {
		t.Fatalf("--field: %v", err)
	}
	sent := postedTransitionMap(t, f, "NMB-1")
	var fields map[string]any
	if err := json.Unmarshal(sent["fields"], &fields); err != nil {
		t.Fatalf("fields %s: %v", sent["fields"], err)
	}
	obj, _ := fields["customfield_10010"].(map[string]any)
	if obj["id"] != "3" {
		t.Fatalf("customfield_10010 %v, want {\"id\":\"3\"}", fields["customfield_10010"])
	}
	if fields["environment"] != "staging" {
		t.Fatalf("environment %v, want string staging", fields["environment"])
	}
}
