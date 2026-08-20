package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// Contract ↔ assertion:
//
//  1. Each flag alone → one PUT with only that change
//     TestEditSummaryAlone, TestEditDescriptionAlone, TestEditLabelAlone, TestEditPriorityAlone
//  2. Combined flags in one PUT
//     TestEditCombinedFlagsOnePUT
//  3. Bare --label value rejected (shows +/- syntax); no PUT
//     TestEditLabelBareValueRejected
//  4. --label +a --label -b → update verbs exactly
//     TestEditLabelAddRemoveVerbs
//  5. Priority resolves name case-insensitively and by id; unmatched lists catalog
//     TestEditPriorityResolvesNameAndID, TestEditPriorityUnmatchedListsCatalog
//  6. -m "" clears (description:null)
//     TestEditClearsDescription
//  7. No edit flags → usage error listing them
//     TestEditNoFlagsIsUsageError
//  8. No credential → shared error
//     TestWritesRefuseToRunWithoutACredential (agent_test.go),
//     TestEditRefusesWithoutCredential
//  9. Re-read row printed (mirror refreshed)
//     TestEditSummaryAlone (status 완료)
// 10. Dispatch + help (`--label +x --label -y` example)
//     TestEditIsRegisteredAndHelpShowsLabelSyntax
// 11. Self-review: label names may contain + / - after the verb prefix
//     TestEditLabelInternalPlusMinus
//
// --parent (GDK-19 / GDK-86 parent half):
// 12. --parent KEY re-parents; --parent none clears; invalid shape writes nothing
//     TestEditParentReparents
//     TestEditParentNoneClears
//     TestEditParentInvalidKeyWritesNothing
// 13. --parent alone is an edit (not a usage error)
//     TestEditParentAlone
// 14. Write-through refresh stores / clears parent_key
//     TestEditParentMirrorRefreshLandsParentKey
//     TestEditParentNoneClearsMirrorParentKey
// 15. Jira hierarchy rejection is surfaced verbatim
//     TestEditParentRejectedByJiraSurfacesMessage

func TestEditSummaryAlone(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--summary", "Renamed without Jira"})
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"summary":"Renamed without Jira"`) {
		t.Fatalf("PUT body %s", body)
	}
	if strings.Contains(body, `"update"`) {
		t.Errorf("summary-only must not send update: %s", body)
	}
	if strings.Contains(out, "NMB-1\t완료\t") == false {
		t.Fatalf("stale line %q", out)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("calls %v", f.calls)
	}
	// SyncIssue's re-read also hits GET /priority (site catalog). The
	// contract is that the PUT itself has no priority field.
	if strings.Contains(body, `"priority"`) {
		t.Errorf("summary-only PUT must not set priority: %s", body)
	}
}

func TestEditDescriptionAlone(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", "new body"})
	})
	if err != nil {
		t.Fatalf("edit -m: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"type":"doc"`) || !strings.Contains(body, `"text":"new body"`) {
		t.Fatalf("description not ADF: %s", body)
	}
	if strings.Contains(body, `"update"`) {
		t.Errorf("-m only must not send update: %s", body)
	}
}

func TestEditLabelAlone(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--label", "+batch"})
	})
	if err != nil {
		t.Fatalf("edit --label: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"update"`) || !strings.Contains(body, `"add":"batch"`) {
		t.Fatalf("label add missing: %s", body)
	}
	if strings.Contains(body, `"fields"`) {
		t.Errorf("label-only must omit fields: %s", body)
	}
}

func TestEditPriorityAlone(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--priority", "High"})
	})
	if err != nil {
		t.Fatalf("edit --priority: %v", err)
	}
	if !f.called("GET /priority") {
		t.Fatalf("PriorityCatalog not called: %v", f.calls)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"priority":{"id":"2"}`) {
		t.Fatalf("priority id: %s", body)
	}
}

func TestEditCombinedFlagsOnePUT(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{
			"NMB-1",
			"--summary", "both",
			"-m", "desc",
			"--label", "+a",
			"--label", "-b",
			"--priority", "medium",
		})
	})
	if err != nil {
		t.Fatalf("edit combined: %v", err)
	}
	n := 0
	for _, c := range f.calls {
		if c == "PUT /issue/NMB-1" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want 1 PUT, got %d: %v", n, f.calls)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	for _, want := range []string{
		`"summary":"both"`,
		`"text":"desc"`,
		`"id":"3"`,
		`"add":"a"`,
		`"remove":"b"`,
		`"fields"`,
		`"update"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("combined PUT missing %s: %s", want, body)
		}
	}
}

func TestEditLabelBareValueRejected(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--label", "batch"})
	})
	if err == nil {
		t.Fatal("bare label must be a usage error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "+") || !strings.Contains(msg, "-") {
		t.Errorf("error must show +/- syntax: %q", msg)
	}
	if strings.Contains(msg, "batch") == false {
		t.Errorf("error should name the value: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("bare label reached Jira: %v", f.calls)
	}
}

func TestEditLabelAddRemoveVerbs(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--label", "+a", "--label", "-b"})
	})
	if err != nil {
		t.Fatalf("edit labels: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"labels":[{"add":"a"},{"remove":"b"}]`) {
		t.Fatalf("update verbs: %s", body)
	}
	if strings.Contains(body, `"fields"`) {
		t.Errorf("labels-only must omit fields: %s", body)
	}
}

func TestEditPriorityResolvesNameAndID(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--priority", "highest"})
	})
	if err != nil {
		t.Fatalf("priority highest: %v", err)
	}
	if body := f.bodies["PUT /issue/NMB-1"]; !strings.Contains(body, `"id":"1"`) {
		t.Fatalf("case-insensitive name: %s", body)
	}

	_, err = capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--priority", "3"})
	})
	if err != nil {
		t.Fatalf("priority id: %v", err)
	}
	if body := f.bodies["PUT /issue/NMB-1"]; !strings.Contains(body, `"id":"3"`) {
		t.Fatalf("exact id: %s", body)
	}
}

func TestEditPriorityUnmatchedListsCatalog(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--priority", "Urgent"})
	})
	if err == nil {
		t.Fatal("expected unmatched priority error")
	}
	for _, want := range []string{`no priority matching "Urgent"`, "Highest (id 1)", "High (id 2)", "Medium (id 3)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("unmatched priority reached Jira: %v", f.calls)
	}
}

func TestEditClearsDescription(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", ""})
	})
	if err != nil {
		t.Fatalf("edit -m \"\": %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"description":null`) {
		t.Fatalf("clear description: %s", body)
	}
}

func TestEditNoFlagsIsUsageError(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1"})
	})
	if err == nil || !strings.Contains(err.Error(), "usage: gadak edit") {
		t.Fatalf("no flags: %v", err)
	}
	for _, want := range []string{"--summary", "-m", "--label", "--priority", "--parent"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("usage %q missing %q", err, want)
		}
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("no-flag edit reached Jira: %v", f.calls)
	}

	_, err = capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--json"})
	})
	if err == nil || !strings.Contains(err.Error(), "usage: gadak edit") {
		t.Fatalf("--json alone: %v", err)
	}
}

func TestEditEmptySummaryIsUsageError(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--summary", "   "})
	})
	if err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("empty summary: %v", err)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("empty summary reached Jira: %v", f.calls)
	}
}

func TestEditRefusesWithoutCredential(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Token = ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--summary", "x"})
	})
	if err == nil || !strings.Contains(err.Error(), "gadak init") {
		t.Fatalf("no credential: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("called Jira anyway: %v", f.calls)
	}
}

func TestEditLabelInternalPlusMinus(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--label", "+hotfix-2", "--label", "-qa+"})
	})
	if err != nil {
		t.Fatalf("internal +/-: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"add":"hotfix-2"`) || !strings.Contains(body, `"remove":"qa+"`) {
		t.Fatalf("internal +/- verbs: %s", body)
	}
}

func TestEditReadsDescriptionFromStdin(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("line one\nline two\n")
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	_, err = capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", "-"})
	})
	os.Stdin = saved
	if err != nil {
		t.Fatalf("edit -m -: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"type":"doc"`) || !strings.Contains(body, "line two") {
		t.Fatalf("stdin body not sent: %s", body)
	}
}

func TestEditJSONPrintsReread(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--summary", "json edit", "--json"})
	})
	if err != nil {
		t.Fatalf("edit --json: %v\n%s", err, out)
	}
	var res struct {
		Issue struct {
			IssueKey string `json:"issue_key"`
			Status   string `json:"status"`
		} `json:"issue"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		// IssueLite may use `key` — accept either by raw contains.
		if !strings.Contains(out, `"status":"완료"`) {
			t.Fatalf("decode %q: %v", out, err)
		}
		return
	}
	if res.Issue.Status != "완료" && !strings.Contains(out, `"status":"완료"`) {
		t.Fatalf("json not re-read: %s", out)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("calls %v", f.calls)
	}
}

func TestEditIsRegisteredAndHelpShowsLabelSyntax(t *testing.T) {
	run, ok := commands["edit"]
	if !ok || run == nil {
		t.Fatal("edit missing from dispatch map")
	}
	h, ok := helps["edit"]
	if !ok {
		t.Fatal("edit missing from helps")
	}
	if !strings.Contains(h.usage, "gadak [--workspace <name>] edit") {
		t.Errorf("usage: %s", h.usage)
	}
	joined := strings.Join(h.examples, "\n")
	if !strings.Contains(joined, "--label +") || !strings.Contains(joined, "--label -") {
		t.Errorf("examples missing --label +x --label -y:\n%s", joined)
	}
	if !strings.Contains(usage, "edit       edit an issue") {
		t.Errorf("top-level Writing-through block missing edit:\n%s", usage)
	}
}

func TestEditLabelMinusValueIsNotAFlag(t *testing.T) {
	// parseAround must take `-legacy` as --label's value, not as an unknown flag.
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--label", "-legacy"})
	})
	if err != nil {
		t.Fatalf("label -legacy: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"remove":"legacy"`) {
		t.Fatalf("remove verb: %s", body)
	}
}

func TestEditParentReparents(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "gdk-1"})
	})
	if err != nil {
		t.Fatalf("edit --parent: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"parent":{"key":"GDK-1"}`) {
		t.Fatalf("reparent PUT: %s", body)
	}
	if strings.Contains(body, `"update"`) {
		t.Errorf("parent-only must not send update: %s", body)
	}
}

func TestEditParentNoneClears(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "none"})
	})
	if err != nil {
		t.Fatalf("edit --parent none: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"parent":null`) {
		t.Fatalf("clear parent: %s", body)
	}
}

func TestEditParentInvalidKeyWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "not-a-key"})
	})
	if err == nil {
		t.Fatal("expected invalid parent key error")
	}
	if !strings.Contains(err.Error(), `not a Jira key (want ABC-123)`) {
		t.Fatalf("wording: %v", err)
	}
	if !strings.Contains(err.Error(), "not-a-key") {
		t.Fatalf("error should name the value: %v", err)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("invalid parent reached Jira: %v", f.calls)
	}

	_, err = capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "NONE"})
	})
	if err == nil || !strings.Contains(err.Error(), `not a Jira key (want ABC-123)`) {
		t.Fatalf("NONE is not the literal none: %v", err)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("NONE reached Jira: %v", f.calls)
	}
}

func TestEditParentAlone(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "NMB-2"})
	})
	if err != nil {
		t.Fatalf("--parent alone must be an edit: %v", err)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("calls %v", f.calls)
	}
}

func TestEditParentMirrorRefreshLandsParentKey(t *testing.T) {
	f := newFakeJira(t)
	echoParentOnReread(t, f)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "GDK-1", "--json"})
	})
	if err != nil {
		t.Fatalf("edit --parent --json: %v\n%s", err, out)
	}
	var res struct {
		Issue store.IssueLite `json:"issue"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Issue.ParentKey == nil || *res.Issue.ParentKey != "GDK-1" {
		t.Fatalf("mirror parent_key = %v, want GDK-1", res.Issue.ParentKey)
	}
}

func TestEditParentNoneClearsMirrorParentKey(t *testing.T) {
	f := newFakeJira(t)
	echoParentOnReread(t, f)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "GDK-1"})
	})
	if err != nil {
		t.Fatalf("reparent: %v", err)
	}

	out, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "none", "--json"})
	})
	if err != nil {
		t.Fatalf("clear: %v\n%s", err, out)
	}
	var res struct {
		Issue store.IssueLite `json:"issue"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Issue.ParentKey != nil {
		t.Fatalf("cleared parent_key = %v, want nil", *res.Issue.ParentKey)
	}
}

func TestEditParentRejectedByJiraSurfacesMessage(t *testing.T) {
	f := newFakeJira(t)
	rejectParentWrite(t, f, "NMB-999", "Issue type cannot be a child of the selected parent.")
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--parent", "NMB-999"})
	})
	if err == nil {
		t.Fatal("expected Jira hierarchy rejection")
	}
	if !strings.Contains(err.Error(), "Issue type cannot be a child of the selected parent.") {
		t.Fatalf("must carry Jira's message, got %v", err)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("valid key must reach Jira: %v", f.calls)
	}
}

func TestEditDueSetsDate(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--due", "2026-09-01"})
	})
	if err != nil {
		t.Fatalf("edit --due: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"duedate":"2026-09-01"`) {
		t.Fatalf("set due: %s", body)
	}
}

func TestEditDueNoneClears(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--due", "none"})
	})
	if err != nil {
		t.Fatalf("edit --due none: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"duedate":null`) {
		t.Fatalf("clear due: %s", body)
	}
}

func TestEditDueInvalidWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--due", "01/09/2026"})
	})
	if err == nil {
		t.Fatal("expected invalid --due error")
	}
	if !strings.Contains(err.Error(), "want YYYY-MM-DD") {
		t.Fatalf("wording: %v", err)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("invalid --due reached Jira: %v", f.calls)
	}
}

func TestEditDueNoneProcessedBeforeFormat(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--due", "none"})
	})
	if err != nil {
		t.Fatalf("--due none must clear, not fail format: %v", err)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("calls %v", f.calls)
	}

	_, err = capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--due", "NONE"})
	})
	if err == nil || !strings.Contains(err.Error(), "want YYYY-MM-DD") {
		t.Fatalf("NONE is not the literal none: %v", err)
	}
}
