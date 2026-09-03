package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
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
//
// --component (GDK-517):
// 16. --component +SDK --component -Docs → update.components add/remove {name}
//     TestEditComponentAddRemoveVerbs
// 17. Bare --component value rejected (add or remove); no PUT
//     TestEditComponentBareValueRejected
// 18. Edit without --component omits the components key
//     TestEditWithoutComponentOmitsComponentsKey
// 19. Origin 400 + editmeta allowedValues → "available components:" names
//     TestEditComponentOrigin400HintsAllowedValues
// 20. --label and --component together each get their own update array
//     TestEditLabelAndComponentBothGoOut
//
// --fix-version (GDK-516 / GDK-123):
// 21. --fix-version +v2.5 --fix-version -10013 → update.fixVersions add/remove {id}
//     TestEditFixVersionAddRemoveVerbs
// 22. Bare --fix-version value rejected (add or remove); no PUT
//     TestEditFixVersionBareValueRejected
// 23. Unknown name lists the project catalog; no PUT
//     TestEditFixVersionUnknownNameListsCatalog
// 24. All-digit values skip the catalog GET
//     TestEditFixVersionAllDigitsSkipCatalog
// 25. Edit without --fix-version omits the fixVersions key
//     TestEditWithoutFixVersionOmitsFixVersionsKey
// 26. Duplicate catalog names refuse as ambiguous (ids listed); no PUT
//     TestEditFixVersionAmbiguousListsIDs
//
// --fix-version mint-by-name (GDK-678):
// 27. Origin CreatesVersionsByName + empty catalog + +v0.18 → PUT {"name":"v0.18"}
//     TestEditFixVersionUnknownNameCreatesWhenOriginAllows
// 28. Connected (capability false) still refuses +unknown (existing 23)
//     TestEditFixVersionUnknownNameListsCatalog
//     TestEditFixVersionUnknownNameRejectedOnEmptyCatalogWhenOriginDoesNotCreate
// 29. -unknown still refuses when the origin would mint on add
//     TestEditFixVersionRemoveUnknownNameRejectedWhenOriginCreates
//
// --type (GDK-962 — a misfiled issue can be corrected):
// 30. --type resolves through create.Type, GDK-741's single owner — id,
//     localized name, untranslatedName, structural alias; unknown and
//     ambiguous refuse
//     TestEditTypeResolvesLikeCreate
// 31. Unavailable type names the project catalog with ids; no PUT
//     TestEditTypeUnmatchedListsAvailableWithIDs
// 32. The project is the mirror's project_key (a prefix that is not the
//     project key cannot misroute); the key prefix is only the fallback
//     TestEditTypeUsesMirrorProjectWhenPrefixDiffers
//     TestEditTypePrefixFallbackWhenMirrorMisses
// 33. The origin's refusal (e.g. Jira's hierarchy 400) passes through
//     verbatim, not flattened
//     TestEditTypeOriginRefusalSurfacesJiraMessage
// 34. Linear refuses loudly before any origin call; never a silent drop
//     TestEditTypeLinearRefusesLoudly
// 35. --batch accepts "type" on the same terms
//     TestEditTypeBatchSendsID
// 36. Empty --type is a usage error — create's empty-want fallbacks must
//     never refile an existing issue
//     TestEditTypeEmptyIsUsageError
// 37. Help / usage enumerate --type
//     TestEditHelpListsTypeFlag
//
// -m vs formatted descriptions (GDK-1001 — a markdown replace must not
// silently destroy a panel/media/mention body; since GDK-1384 headings,
// lists, tables and marks round-trip through markdown and are not loss):
// 38. formatted current description + -m → refused, the node types and marks
//     named, --force-plain offered; no PUT; the guard did read the origin
//     TestEditMRefusesFormattedDescription
// 39. same + --force-plain → PUT carries the new plain body
//     TestEditMForcePlainReplacesFormattedDescription
// 40. plain current description + -m → passes with no flag
//     TestEditMPassesPlainDescriptionWithoutFlag
// 41. -m '' (clear) is the same loss → refused / forced clear lands null
//     TestEditMClearRefusesFormattedDescription
//     TestEditMClearForcePlainClears
// 42. the guard's origin GET failing aborts the edit — "could not verify" is
//     not "nothing to destroy"
//     TestEditMGuardGETFailureAborts
// 43. --batch description lines get the same guard; --force-plain lifts it
//     TestEditBatchDescriptionGuard
// 44. Linear skips the Jira GET (its descriptions are plain end to end)
//     TestEditMGuardSkipsLinear
// 45. Help / usage enumerate --force-plain
//     TestEditHelpListsForcePlainFlag

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
	for _, want := range []string{"--summary", "-m", "--label", "--component", "--fix-version", "--priority", "--parent"} {
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

// GDK-1360: `-m -` on an empty stdin is refused, not written. The failure
// mode was real — a sed that errored left a zero-byte file, the pipe went
// through, the success line printed, and the description was gone. Clearing
// is still one flag away, spelled as itself.
func TestEditEmptyStdinRefusedNotCleared(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	for _, body := range []string{"", "\n  \n"} {
		withStdin(t, body)
		_, err := capture(t, func() error {
			return cmdEdit([]string{"NMB-1", "-m", "-"})
		})
		if err == nil {
			t.Fatalf("stdin %q: empty body accepted", body)
		}
		if !strings.Contains(err.Error(), "empty stdin") || !strings.Contains(err.Error(), "-m ''") {
			t.Fatalf("stdin %q: refusal must name the cause and the deliberate spelling, got %v", body, err)
		}
		if _, wrote := f.bodies["PUT /issue/NMB-1"]; wrote {
			t.Fatalf("stdin %q: a PUT went out despite the refusal", body)
		}
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

func TestEditComponentAddRemoveVerbs(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--component", "+SDK", "--component", "-Docs"})
	})
	if err != nil {
		t.Fatalf("edit --component: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	got := string(putUpdateField(t, body, "components"))
	want := `[{"add":{"name":"SDK"}},{"remove":{"name":"Docs"}}]`
	if got != want {
		t.Fatalf("update.components = %s, want %s (body %s)", got, want, body)
	}
	if _, ok := putPayload(t, body).Fields["summary"]; ok {
		t.Errorf("component-only must omit fields: %s", body)
	}
	if f.called("GET /issue/NMB-1/editmeta") {
		t.Fatalf("success path must not GET editmeta: %v", f.calls)
	}
}

func TestEditComponentBareValueRejected(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--component", "SDK"})
	})
	if err == nil {
		t.Fatal("bare component must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "add or remove") {
		t.Errorf("error must show add-vs-replace wording: %q", msg)
	}
	if !strings.Contains(msg, "--component") {
		t.Errorf("error must name --component: %q", msg)
	}
	if !strings.Contains(msg, "SDK") {
		t.Errorf("error should name the value: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("bare component reached Jira: %v", f.calls)
	}
	if f.called("GET /issue/NMB-1/editmeta") {
		t.Fatalf("local reject must not GET editmeta: %v", f.calls)
	}
}

func TestEditWithoutComponentOmitsComponentsKey(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--label", "+batch"})
	})
	if err != nil {
		t.Fatalf("edit --label: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	payload := putPayload(t, body)
	if _, ok := payload.Update["components"]; ok {
		t.Fatalf("components key present without --component: %s", body)
	}
	if string(payload.Update["labels"]) != `[{"add":"batch"}]` {
		t.Fatalf("labels-only update drifted: %s", body)
	}
}

func TestEditComponentOrigin400HintsAllowedValues(t *testing.T) {
	f := newFakeJira(t)
	f.editMeta = `{"components":{"allowedValues":[{"id":"1","name":"SDK"},{"id":"2","name":"Docs"},{"id":"3","name":"API"}]}}`
	rejectIssuePUT(t, f, `Component name "Nope" is not valid.`)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--component", "+Nope"})
	})
	if err == nil {
		t.Fatal("expected origin 400")
	}
	msg := err.Error()
	if !strings.Contains(msg, `Component name "Nope" is not valid.`) {
		t.Fatalf("must keep origin wording, got %q", msg)
	}
	if !strings.Contains(msg, "available components:") {
		t.Fatalf("missing available-components hint: %q", msg)
	}
	for _, name := range []string{"SDK", "Docs", "API"} {
		if !strings.Contains(msg, name) {
			t.Errorf("hint missing %q: %q", name, msg)
		}
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("valid +/- must reach Jira: %v", f.calls)
	}
	if !f.called("GET /issue/NMB-1/editmeta") {
		t.Fatalf("400 path must GET editmeta once: %v", f.calls)
	}
	n := 0
	for _, c := range f.calls {
		if c == "GET /issue/NMB-1/editmeta" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("editmeta GET count %d, want 1: %v", n, f.calls)
	}
}

func TestEditComponentOrigin400WithoutComponentsFieldKeepsError(t *testing.T) {
	f := newFakeJira(t)
	f.editMeta = `{"summary":{"operations":["set"]}}`
	rejectIssuePUT(t, f, `Component name "Nope" is not valid.`)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--component", "+Nope"})
	})
	if err == nil {
		t.Fatal("expected origin 400")
	}
	msg := err.Error()
	if strings.Contains(msg, "available components:") {
		t.Fatalf("hint attached without components field: %q", msg)
	}
	if !strings.Contains(msg, `Component name "Nope" is not valid.`) {
		t.Fatalf("origin wording lost: %q", msg)
	}
}

func TestEditLabelAndComponentBothGoOut(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{
			"NMB-1",
			"--label", "+batch",
			"--label", "-legacy",
			"--component", "+SDK",
			"--component", "-Docs",
		})
	})
	if err != nil {
		t.Fatalf("edit --label --component: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if string(putUpdateField(t, body, "labels")) != `[{"add":"batch"},{"remove":"legacy"}]` {
		t.Fatalf("labels: %s", body)
	}
	if string(putUpdateField(t, body, "components")) != `[{"add":{"name":"SDK"}},{"remove":{"name":"Docs"}}]` {
		t.Fatalf("components: %s", body)
	}
}

func TestEditHelpShowsComponentSyntax(t *testing.T) {
	h, ok := helps["edit"]
	if !ok {
		t.Fatal("edit missing from helps")
	}
	if !strings.Contains(h.usage, "--component +x|-x") {
		t.Errorf("usage missing --component: %s", h.usage)
	}
	joined := strings.Join(h.examples, "\n")
	if !strings.Contains(joined, "--component +") || !strings.Contains(joined, "--component -") {
		t.Errorf("examples missing --component +x --component -y:\n%s", joined)
	}
	if !strings.Contains(editUsage, "--component") {
		t.Errorf("editUsage missing --component: %s", editUsage)
	}
}

func TestEditFixVersionAddRemoveVerbs(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+v2.5", "--fix-version", "-10013"})
	})
	if err != nil {
		t.Fatalf("edit --fix-version: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	got := string(putUpdateField(t, body, "fixVersions"))
	want := `[{"add":{"id":"10012"}},{"remove":{"id":"10013"}}]`
	if got != want {
		t.Fatalf("update.fixVersions = %s, want %s (body %s)", got, want, body)
	}
	if _, ok := putPayload(t, body).Fields["summary"]; ok {
		t.Errorf("fix-version-only must omit fields: %s", body)
	}
	n := 0
	for _, c := range f.calls {
		if c == "GET /project/NMB/versions" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("catalog GET count %d, want 1: %v", n, f.calls)
	}
}

func TestEditFixVersionBareValueRejected(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "v2.5"})
	})
	if err == nil {
		t.Fatal("bare fix-version must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "add or remove") {
		t.Errorf("error must show add-vs-replace wording: %q", msg)
	}
	if !strings.Contains(msg, "--fix-version") {
		t.Errorf("error must name --fix-version: %q", msg)
	}
	if !strings.Contains(msg, "v2.5") {
		t.Errorf("error should name the value: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("bare fix-version reached Jira: %v", f.calls)
	}
	if f.called("GET /project/NMB/versions") {
		t.Fatalf("local reject must not GET versions: %v", f.calls)
	}
}

func TestEditFixVersionUnknownNameListsCatalog(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+nope"})
	})
	if err == nil {
		t.Fatal("expected unmatched fix-version error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `no fix version matching "nope"`) {
		t.Errorf("wording: %q", msg)
	}
	if !strings.Contains(msg, "v2.5") {
		t.Errorf("error must list catalog names: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("unmatched name reached Jira: %v", f.calls)
	}
}

func TestEditFixVersionAllDigitsSkipCatalog(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+10012", "--fix-version", "-10013"})
	})
	if err != nil {
		t.Fatalf("edit --fix-version ids: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	got := string(putUpdateField(t, body, "fixVersions"))
	want := `[{"add":{"id":"10012"}},{"remove":{"id":"10013"}}]`
	if got != want {
		t.Fatalf("update.fixVersions = %s, want %s (body %s)", got, want, body)
	}
	if f.called("GET /project/NMB/versions") {
		t.Fatalf("all-digit values must not GET versions: %v", f.calls)
	}
}

func TestEditWithoutFixVersionOmitsFixVersionsKey(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--label", "+batch"})
	})
	if err != nil {
		t.Fatalf("edit --label: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	payload := putPayload(t, body)
	if _, ok := payload.Update["fixVersions"]; ok {
		t.Fatalf("fixVersions key present without --fix-version: %s", body)
	}
	if string(payload.Update["labels"]) != `[{"add":"batch"}]` {
		t.Fatalf("labels-only update drifted: %s", body)
	}
	if f.called("GET /project/NMB/versions") {
		t.Fatalf("label-only must not GET versions: %v", f.calls)
	}
}

func TestEditBatchAcceptsTheFlagsSingularSpelling(t *testing.T) {
	// GDK-1259: the flag is --label, the batch field was labels; a line in
	// the flag's vocabulary is the same axis, and both at once is refused.
	f := newFakeJira(t)
	mirror(t, f.URL)

	withStdin(t, `{"key":"NMB-1","label":["+batch"]}`+"\n")
	out, err := capture(t, func() error { return cmdEdit([]string{"--batch", "-"}) })
	if err != nil {
		t.Fatalf("edit --batch with label: %v\n%s", err, out)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if string(putUpdateField(t, body, "labels")) != `[{"add":"batch"}]` {
		t.Fatalf("singular label did not reach update.labels: %s", body)
	}

	obj := map[string]json.RawMessage{"label": json.RawMessage(`["+a"]`), "labels": json.RawMessage(`["+b"]`)}
	if err := foldBatchSingulars(obj); err == nil || !strings.Contains(err.Error(), "same axis") {
		t.Fatalf("label and labels on one line must be refused, got %v", err)
	}
}

func TestEditFixVersionAmbiguousListsIDs(t *testing.T) {
	f := newFakeJira(t)
	f.versionsJSON = `[{"id":"10012","name":"v2.5"},{"id":"10099","name":"v2.5"}]`
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+v2.5"})
	})
	if err == nil {
		t.Fatal("expected ambiguous fix-version error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ambiguous") {
		t.Errorf("error must say ambiguous: %q", msg)
	}
	if !strings.Contains(msg, "10012") || !strings.Contains(msg, "10099") {
		t.Errorf("error must list matching ids: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("ambiguous name reached Jira: %v", f.calls)
	}
}

func TestEditFixVersionNameIsCaseInsensitive(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+V2.5"})
	})
	if err != nil {
		t.Fatalf("edit --fix-version +V2.5: %v", err)
	}
	got := string(putUpdateField(t, f.bodies["PUT /issue/NMB-1"], "fixVersions"))
	if got != `[{"add":{"id":"10012"}}]` {
		t.Fatalf("case-insensitive name: %s", got)
	}
}

func enableNameCreatedVersions(t *testing.T) {
	t.Helper()
	prev := jira.DefaultCreatesVersionsByName
	jira.DefaultCreatesVersionsByName = true
	t.Cleanup(func() { jira.DefaultCreatesVersionsByName = prev })
}

func TestEditFixVersionCatalogHitStillSendsIDWhenOriginCreates(t *testing.T) {
	enableNameCreatedVersions(t)
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+v2.5"})
	})
	if err != nil {
		t.Fatalf("edit --fix-version +v2.5 with CreatesVersionsByName: %v", err)
	}
	got := string(putUpdateField(t, f.bodies["PUT /issue/NMB-1"], "fixVersions"))
	if got != `[{"add":{"id":"10012"}}]` {
		t.Fatalf("catalog hit must still send id, got %s", got)
	}
}

func TestEditFixVersionUnknownNameCreatesWhenOriginAllows(t *testing.T) {
	enableNameCreatedVersions(t)
	f := newFakeJira(t)
	f.versionsJSON = `[]`
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+v0.18"})
	})
	if err != nil {
		t.Fatalf("edit --fix-version +v0.18 with CreatesVersionsByName: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	got := string(putUpdateField(t, body, "fixVersions"))
	want := `[{"add":{"name":"v0.18"}}]`
	if got != want {
		t.Fatalf("update.fixVersions = %s, want %s (body %s)", got, want, body)
	}
}

func TestEditFixVersionUnknownNameRejectedOnEmptyCatalogWhenOriginDoesNotCreate(t *testing.T) {
	f := newFakeJira(t)
	f.versionsJSON = `[]`
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "+v0.18"})
	})
	if err == nil {
		t.Fatal("expected unmatched fix-version error on connected empty catalog")
	}
	msg := err.Error()
	if !strings.Contains(msg, `no fix version matching "v0.18"`) {
		t.Errorf("wording: %q", msg)
	}
	if !strings.Contains(msg, "available: (none)") {
		t.Errorf("error must keep available: (none): %q", msg)
	}
	if !strings.Contains(msg, "creates versions by name: false") {
		t.Errorf("error must show capability false: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("unmatched name reached Jira: %v", f.calls)
	}
}

func TestEditFixVersionRemoveUnknownNameRejectedWhenOriginCreates(t *testing.T) {
	enableNameCreatedVersions(t)
	f := newFakeJira(t)
	f.versionsJSON = `[]`
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--fix-version", "-v0.18"})
	})
	if err == nil {
		t.Fatal("expected unmatched fix-version error on remove")
	}
	msg := err.Error()
	if !strings.Contains(msg, `no fix version matching "v0.18"`) {
		t.Errorf("wording: %q", msg)
	}
	if !strings.Contains(msg, "available: (none)") {
		t.Errorf("error must keep available: (none): %q", msg)
	}
	if !strings.Contains(msg, "creates versions by name: true") {
		t.Errorf("error must show capability true: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("remove of unknown name reached Jira: %v", f.calls)
	}
}

func TestEditHelpShowsFixVersionSyntax(t *testing.T) {
	h, ok := helps["edit"]
	if !ok {
		t.Fatal("edit missing from helps")
	}
	if !strings.Contains(h.usage, "--fix-version +id-or-name|-id-or-name") {
		t.Errorf("usage missing --fix-version: %s", h.usage)
	}
	if strings.Contains(h.usage, "--fixversion") {
		t.Errorf("usage must not alias --fixversion: %s", h.usage)
	}
	joined := strings.Join(h.examples, "\n")
	if !strings.Contains(joined, "--fix-version +") || !strings.Contains(joined, "--fix-version -") {
		t.Errorf("examples missing --fix-version +x --fix-version -y:\n%s", joined)
	}
	if !strings.Contains(editUsage, "--fix-version") {
		t.Errorf("editUsage missing --fix-version: %s", editUsage)
	}
}

func putPayload(t *testing.T, body string) struct {
	Fields map[string]json.RawMessage `json:"fields"`
	Update map[string]json.RawMessage `json:"update"`
} {
	t.Helper()
	var payload struct {
		Fields map[string]json.RawMessage `json:"fields"`
		Update map[string]json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("PUT JSON: %v (%s)", err, body)
	}
	return payload
}

func putUpdateField(t *testing.T, body, field string) json.RawMessage {
	t.Helper()
	payload := putPayload(t, body)
	raw, ok := payload.Update[field]
	if !ok {
		t.Fatalf("update.%s missing: %s", field, body)
	}
	return raw
}

// rejectIssuePUT answers 400 with jiraMsg on PUT /issue/{key} (not editmeta).
func rejectIssuePUT(t *testing.T, f *fakeJira, jiraMsg string) {
	t.Helper()
	inner := f.Config.Handler
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
		isIssuePUT := r.Method == http.MethodPut && strings.HasPrefix(path, "/issue/") && strings.Count(path, "/") == 2
		rec := httptest.NewRecorder()
		inner.ServeHTTP(rec, r)
		if isIssuePUT {
			msg, _ := json.Marshal(jiraMsg)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errorMessages":[` + string(msg) + `]}`))
			return
		}
		for k, vs := range rec.Header() {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		status := rec.Code
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(rec.Body.Bytes())
	})
}

// --field alias=value (GDK-513):
// 27. option name resolves against editmeta allowedValues, then FieldValue wraps
//     TestEditFieldOptionWrapsViaFieldValue
// 28. unknown alias lists configured aliases; no PUT
//     TestEditUnknownFieldAliasListsConfiguredRefusesPUT
// 29. --field absent → no customfield_* key (byte-identical to pre-flag body)
//     TestEditWithoutFieldOmitsCustomfieldKey
// 30. configured alias missing from this issue's editmeta → refuse; no PUT
//     TestEditFieldNotOnIssueRefusesPUT
// 31. Help / usage enumerate --field
//     TestEditHelpListsFieldFlag

func seedSeverityAlias(t *testing.T, f *fakeJira) {
	t.Helper()
	f.editMeta = `{
		"customfield_10001": {"required":true,"schema":{"type":"option"},"operations":["set"],
			"allowedValues":[{"id":"1","value":"High"}]}
	}`
	cfg := mirror(t, f.URL)
	cfg.Fields = []config.FieldSpec{
		{Alias: "severity", Label: "Severity", IDs: []string{"customfield_10001"}, Role: "facet", Kind: "option"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestEditFieldOptionWrapsViaFieldValue(t *testing.T) {
	f := newFakeJira(t)
	seedSeverityAlias(t, f)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--field", "severity=High"})
	})
	if err != nil {
		t.Fatalf("edit --field severity=High: %v", err)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("PUT missing: %v", f.calls)
	}
	if !f.called("GET /issue/NMB-1/editmeta") {
		t.Fatalf("editmeta missing: %v", f.calls)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	got := putPayload(t, body).Fields["customfield_10001"]
	wantVal, err := fields.FieldValue("option", json.RawMessage(`"1"`))
	if err != nil {
		t.Fatalf("FieldValue: %v", err)
	}
	want, err := json.Marshal(wantVal)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("customfield_10001 = %s, want FieldValue(option, \"1\") = %s (body %s)", got, want, body)
	}
}

func TestEditUnknownFieldAliasListsConfiguredRefusesPUT(t *testing.T) {
	f := newFakeJira(t)
	seedSeverityAlias(t, f)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--field", "nosuchalias=x"})
	})
	if err == nil {
		t.Fatal("unknown alias must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nosuchalias") {
		t.Errorf("error must name the alias: %q", msg)
	}
	if !strings.Contains(msg, "severity") {
		t.Errorf("error must list configured aliases: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("unknown alias reached PUT: %v", f.calls)
	}
}

func TestEditWithoutFieldOmitsCustomfieldKey(t *testing.T) {
	f := newFakeJira(t)
	seedSeverityAlias(t, f)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--summary", "no custom"})
	})
	if err != nil {
		t.Fatalf("edit --summary: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if strings.Contains(body, "customfield_") {
		t.Fatalf("--field absent must omit customfield keys: %s", body)
	}
	if f.called("GET /issue/NMB-1/editmeta") {
		t.Fatalf("summary-only must not GET editmeta: %v", f.calls)
	}
}

func TestEditFieldNotOnIssueRefusesPUT(t *testing.T) {
	f := newFakeJira(t)
	seedSeverityAlias(t, f)
	f.editMeta = `{}`

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--field", "severity=High"})
	})
	if err == nil {
		t.Fatal("alias missing from editmeta must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "severity") {
		t.Errorf("error must name the alias: %q", msg)
	}
	if !strings.Contains(msg, "not editable") {
		t.Errorf("error must say not editable: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("uneditable field reached PUT: %v", f.calls)
	}
}

func TestEditUserFieldResolvesDisplayName(t *testing.T) {
	f := newFakeJira(t)
	seedUserAlias(t, f)
	f.searchUsers = func(query string) string {
		if query == "Dana Whitfield" {
			return `[{"accountId":"acc-hc","displayName":"Dana Whitfield","emailAddress":"dana@example.com","active":true}]`
		}
		return "[]"
	}

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--field", "reviewer=Dana Whitfield"})
	})
	if err != nil {
		t.Fatalf("edit --field reviewer=Dana Whitfield: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	got := putPayload(t, body).Fields["customfield_10020"]
	if !strings.Contains(string(got), `"accountId":"acc-hc"`) {
		t.Fatalf("user field PUT %s, want accountId acc-hc (body %s)", got, body)
	}
	if strings.Contains(string(got), "Dana Whitfield") {
		t.Fatalf("display name leaked into PUT body: %s", body)
	}
}

func TestEditUserFieldUnknownRefuses(t *testing.T) {
	f := newFakeJira(t)
	seedUserAlias(t, f)
	f.searchUsers = func(string) string { return "[]" }

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--field", "reviewer=Nobody Known"})
	})
	if err == nil {
		t.Fatal("unknown user token must be refused")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Nobody Known") {
		t.Errorf("error must name the token: %q", msg)
	}
	if !strings.Contains(msg, "issues.assignee_id") && !strings.Contains(msg, "editmeta") {
		t.Errorf("error must name issues.assignee_id or editmeta as sources: %q", msg)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("unknown user reached PUT: %v", f.calls)
	}
}

func seedUserAlias(t *testing.T, f *fakeJira) {
	t.Helper()
	f.editMeta = `{
		"customfield_10020": {"required":false,"schema":{"type":"user","system":"","custom":"com.atlassian.jira.plugin.system.customfieldtypes:userpicker"},"operations":["set"]}
	}`
	cfg := mirror(t, f.URL)
	cfg.Fields = []config.FieldSpec{
		{Alias: "reviewer", Label: "Reviewer", IDs: []string{"customfield_10020"}, Role: "facet", Kind: "user"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestEditHelpListsFieldFlag(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdEdit([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("edit --help: %v", err)
	}
	if !strings.Contains(out, "--field") {
		t.Fatalf("help Options missing --field:\n%s", out)
	}
	if !strings.Contains(helps["edit"].usage, "--field") {
		t.Errorf("usage line missing --field: %s", helps["edit"].usage)
	}
	if !strings.Contains(editUsage, "--field") {
		t.Errorf("editUsage missing --field: %s", editUsage)
	}
	joined := strings.Join(helps["edit"].examples, "\n")
	if !strings.Contains(joined, "--field") {
		t.Errorf("examples missing --field:\n%s", joined)
	}
}

// overrideCreateMeta answers GET /issue/createmeta with raw (ignoring the
// projectKeys filter the way the fake does); every other request falls
// through, the same wrapping shape rejectIssuePUT uses.
func overrideCreateMeta(t *testing.T, f *fakeJira, raw string) {
	t.Helper()
	inner := f.Config.Handler
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
		if r.Method == http.MethodGet && path == "/issue/createmeta" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(raw))
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// nmbCreateMeta wraps issue types as the NMB project's createmeta payload.
func nmbCreateMeta(types string) string {
	return `{"projects":[{"key":"NMB","name":"Numbers","issuetypes":[` + types + `]}]}`
}

func TestEditTypeResolvesLikeCreate(t *testing.T) {
	cases := []struct {
		name, want, types, id, err string
	}{
		{"id", "10001",
			`{"id":"10001","name":"Task"}`, "10001", ""},
		{"name case-insensitive", "task",
			`{"id":"10001","name":"Task"}`, "10001", ""},
		{"localized name", "작업",
			`{"id":"10002","name":"작업"}`, "10002", ""},
		{"untranslatedName alias", "Defect",
			`{"id":"10004","name":"버그","untranslatedName":"Defect"}`, "10004", ""},
		{"structural epic", "epic",
			`{"id":"10005","name":"에픽","hierarchyLevel":1}`, "10005", ""},
		{"structural subtask spelling", "sub-task",
			`{"id":"10006","name":"하위 작업","subtask":true}`, "10006", ""},
		{"unknown names the catalog", "Story",
			`{"id":"10001","name":"Task"},{"id":"10004","name":"Bug"}`, "",
			`no issue type matching "Story"`},
		{"ambiguous refuses", "subtask",
			`{"id":"10006","name":"하위 작업","subtask":true},{"id":"10007","name":"부작업","subtask":true}`, "",
			`issue type "subtask" matches more than one`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeJira(t)
			mirror(t, f.URL)
			overrideCreateMeta(t, f, nmbCreateMeta(tc.types))
			_, err := capture(t, func() error {
				return cmdEdit([]string{"NMB-1", "--type", tc.want})
			})
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err %v, want %q", err, tc.err)
				}
				if f.called("PUT /issue/NMB-1") {
					t.Fatalf("refused --type reached Jira: %v", f.calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("edit --type %s: %v", tc.want, err)
			}
			body := f.bodies["PUT /issue/NMB-1"]
			if !strings.Contains(body, `"issuetype":{"id":"`+tc.id+`"}`) {
				t.Fatalf("PUT body %s, want issuetype id %s", body, tc.id)
			}
		})
	}
}

func TestEditTypeAloneIsOnePUTWithOnlyIssuetype(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--type", "Bug"})
	})
	if err != nil {
		t.Fatalf("edit --type Bug: %v", err)
	}
	if !f.called("GET /issue/createmeta") {
		t.Fatalf("type resolution must read createmeta: %v", f.calls)
	}
	if n := countCalls(f, "PUT /issue/NMB-1"); n != 1 {
		t.Fatalf("want 1 PUT, got %d: %v", n, f.calls)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	payload := putPayload(t, body)
	if got := string(payload.Fields["issuetype"]); got != `{"id":"10004"}` {
		t.Fatalf("fields.issuetype = %s (body %s)", got, body)
	}
	if len(payload.Fields) != 1 {
		t.Fatalf("type-only PUT must carry only issuetype: %s", body)
	}
	if payload.Update != nil {
		t.Fatalf("type-only PUT must not send update: %s", body)
	}
}

func TestEditTypeUnmatchedListsAvailableWithIDs(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--type", "Story"})
	})
	if err == nil {
		t.Fatal("unmatched type must be refused")
	}
	msg := err.Error()
	for _, want := range []string{
		`no issue type matching "Story"`,
		"Task (id 10001)", "작업 (id 10002)", "Bug (id 10004)",
		"an id always works",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("unmatched type reached Jira: %v", f.calls)
	}
}

// seedForeignPrefixIssue plants STD-7 whose mirror project_key is NMB — the
// shape that proves --type reads the project from the mirror, not the key
// prefix (the prefix is not the project key).
func seedForeignPrefixIssue(t *testing.T) {
	t.Helper()
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1007", SourceID: "jira", Kind: "issue", ExternalID: "1007", Key: "STD-7",
				Title: "key prefix is not the project key", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestEditTypeUsesMirrorProjectWhenPrefixDiffers(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedForeignPrefixIssue(t)
	// No STD project in createmeta: prefix routing would fail here, the
	// mirror's NMB row is what resolves.
	overrideCreateMeta(t, f, nmbCreateMeta(`{"id":"10004","name":"Bug"}`))

	_, err := capture(t, func() error {
		return cmdEdit([]string{"STD-7", "--type", "Bug"})
	})
	if err != nil {
		t.Fatalf("edit STD-7 --type Bug: %v", err)
	}
	body := f.bodies["PUT /issue/STD-7"]
	if !strings.Contains(body, `"issuetype":{"id":"10004"}`) {
		t.Fatalf("PUT body %s", body)
	}
}

func TestEditTypePrefixFallbackWhenMirrorMisses(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	// GDK-1 has no mirror row; the default createmeta offers GDK/Task.

	_, err := capture(t, func() error {
		return cmdEdit([]string{"GDK-1", "--type", "Task"})
	})
	// The write lands; the re-read cannot mirror GDK-1 back (Projects is
	// NMB-only in this fixture), which is the stale-mirror warning, not a
	// failed write. The contract under test is the routed PUT itself.
	if err != nil && !strings.Contains(err.Error(), "write applied to GDK-1") {
		t.Fatalf("edit GDK-1 --type Task: %v", err)
	}
	body := f.bodies["PUT /issue/GDK-1"]
	if !strings.Contains(body, `"issuetype":{"id":"10001"}`) {
		t.Fatalf("PUT body %s", body)
	}
}

func TestEditTypeOriginRefusalSurfacesJiraMessage(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	const jiraSays = "Issues with sub-tasks cannot be converted to sub-tasks."
	rejectIssuePUT(t, f, jiraSays)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--type", "Bug"})
	})
	if err == nil {
		t.Fatal("origin 400 must surface as an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, jiraSays) {
		t.Fatalf("origin's own message was flattened: %q", msg)
	}
	if !strings.Contains(msg, "PUT") || !strings.Contains(msg, "issue/NMB-1") {
		t.Fatalf("error must name the refused call: %q", msg)
	}
}

func TestEditTypeLinearRefusesLoudly(t *testing.T) {
	cfg := mirror(t, "")
	cfg.Linear = &config.LinearConfig{APIKey: linearTestKey}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "linear", Kind: "linear", BaseURL: "https://linear.app"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "linear:fix-1", SourceID: "linear", Kind: "issue", ExternalID: "fix-1", Key: "FIX-1",
				Title: "misfiled on linear", CreatedAt: "2026-08-01T00:00:00.000Z", UpdatedAt: "2026-08-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: "FIX", StatusCategory: "new", Status: "Todo"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Any GraphQL leaving this test is itself a failure: the refusal must
	// precede every origin call, including createmeta (a Linear type
	// catalog cannot exist — fetching one would advertise its synthetic
	// "Issue" entry as retryable).
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	origin.LinearEndpoint = srv.URL
	t.Cleanup(func() { origin.LinearEndpoint = "" })

	_, err = capture(t, func() error {
		return cmdEdit([]string{"FIX-1", "--type", "Bug"})
	})
	if err == nil {
		t.Fatal("--type against Linear must refuse, not silently drop")
	}
	if !strings.Contains(err.Error(), origin.ErrNoIssueTypes.Error()) {
		t.Fatalf("error must say Linear has no issue types: %q", err)
	}
	if hits != 0 {
		t.Fatalf("refusal must precede every Linear call; %d GraphQL documents left the process", hits)
	}
}

func TestEditTypeBatchSendsID(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1","type":"작업"}`+"\n")

	out, err := capture(t, func() error {
		return cmdEdit([]string{"--batch", "-"})
	})
	if err != nil {
		t.Fatalf("edit --batch: %v\n%s", err, out)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"issuetype":{"id":"10002"}`) {
		t.Fatalf("batch type not resolved to id: %s", body)
	}
	if !strings.Contains(out, "NMB-1") {
		t.Fatalf("envelope row missing: %s", out)
	}
}

func TestEditTypeEmptyIsUsageError(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--type", ""})
	})
	if err == nil || !strings.Contains(err.Error(), "usage: gadak edit") {
		t.Fatalf("empty --type: %v", err)
	}
	if f.called("GET /issue/createmeta") || f.called("PUT /issue/NMB-1") {
		t.Fatalf("empty --type reached the origin: %v", f.calls)
	}
}

func TestEditHelpListsTypeFlag(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdEdit([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("edit --help: %v", err)
	}
	if !strings.Contains(out, "--type") {
		t.Fatalf("help Options missing --type:\n%s", out)
	}
	if !strings.Contains(helps["edit"].usage, "--type") {
		t.Errorf("usage line missing --type: %s", helps["edit"].usage)
	}
	if !strings.Contains(editUsage, "--type") {
		t.Errorf("editUsage missing --type: %s", editUsage)
	}
	joined := strings.Join(helps["edit"].examples, "\n")
	if !strings.Contains(joined, "--type") {
		t.Errorf("examples missing --type:\n%s", joined)
	}
}

// formattedDescADF carries one of each loss class markdown cannot hold
// (GDK-1384 — strong, table and heading round-trip through markdown now and
// are no longer loss): a mark (textColor), a block (panel), and a mention.
const formattedDescADF = `{"type":"doc","version":1,"content":[` +
	`{"type":"paragraph","content":[{"type":"text","text":"plain lead"},` +
	`{"type":"text","text":"red","marks":[{"type":"textColor","attrs":{"color":"#ff0000"}}]}]},` +
	`{"type":"panel","attrs":{"panelType":"info"},"content":[` +
	`{"type":"paragraph","content":[{"type":"text","text":"note"}]}]},` +
	`{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"acc","text":"@Dana"}}]}]}`

// plainDescADF is exactly what jira.Doc emits for a two-line plain body.
const plainDescADF = `{"type":"doc","version":1,"content":[` +
	`{"type":"paragraph","content":[{"type":"text","text":"line one"}]},` +
	`{"type":"paragraph","content":[{"type":"text","text":"line two"}]}]}`

// seedDescriptionADF answers POST /search/jql with the single issue whose
// description is the given ADF, so the guard's pre-PUT read (and the
// write-through re-read) both see it. Same wrapping shape as
// overrideCreateMeta; the call/body recording matches f.route so f.called
// keeps working.
func seedDescriptionADF(t *testing.T, f *fakeJira, key, desc string) {
	t.Helper()
	inner := f.Config.Handler
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
		if r.Method == http.MethodPost && path == "/search/jql" {
			tag := r.Method + " " + path
			f.calls = append(f.calls, tag)
			body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if len(body) > 0 {
				f.bodies[tag] = string(body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issues":[{"id":"1001","key":"` + key + `","fields":{
				"summary":"batch worker drops the last page",
				"description":` + desc + `,
				"status":{"id":"10001","name":"완료","statusCategory":{"key":"done"}},
				"project":{"key":"NMB"},"issuetype":{"id":"10004","name":"Bug"},
				"assignee":{"accountId":"acc-hc","displayName":"Dana Whitfield"},
				"created":"2026-07-01T00:00:00.000+0900","updated":"2026-08-04T12:00:00.000+0900"
			}}],"isLast":true}`))
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func TestEditMRefusesFormattedDescription(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", formattedDescADF)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", "rewritten"})
	})
	if err == nil {
		t.Fatal("formatted description must refuse a plain-text replace")
	}
	msg := err.Error()
	for _, want := range []string{"NMB-1", "panel", "textColor", "mention", "--force-plain"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal %q missing %q", msg, want)
		}
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("refused edit still reached Jira: %v", f.calls)
	}
	if !f.called("POST /search/jql") {
		t.Fatalf("guard must read the current description from the origin: %v", f.calls)
	}
}

func TestEditMForcePlainReplacesFormattedDescription(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", formattedDescADF)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", "rewritten", "--force-plain"})
	})
	if err != nil {
		t.Fatalf("edit -m --force-plain: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"text":"rewritten"`) {
		t.Fatalf("forced replace body: %s", body)
	}
}

func TestEditMPassesPlainDescriptionWithoutFlag(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", plainDescADF)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", "rewritten"})
	})
	if err != nil {
		t.Fatalf("plain description needs no flag: %v", err)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("PUT missing: %v", f.calls)
	}
}

func TestEditMClearRefusesFormattedDescription(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", formattedDescADF)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", ""})
	})
	if err == nil {
		t.Fatal("clearing a formatted description must refuse without --force-plain")
	}
	if !strings.Contains(err.Error(), "--force-plain") {
		t.Fatalf("refusal must name the escape flag: %q", err)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("refused clear reached Jira: %v", f.calls)
	}
}

func TestEditMClearForcePlainClears(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", formattedDescADF)

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", "", "--force-plain"})
	})
	if err != nil {
		t.Fatalf("edit -m '' --force-plain: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"description":null`) {
		t.Fatalf("forced clear: %s", body)
	}
}

func TestEditMGuardGETFailureAborts(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	// 404, not 500: reads retry 429/500/502/503/504 (httppolicy.IsRetryable),
	// so a 5xx burns the retry budget for nothing — 404 fails the same GET
	// without the wait. Either way the guard must not wave it through.
	f.rereadStatus = 404

	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "-m", "rewritten"})
	})
	if err == nil {
		t.Fatal("an unreadable current description must abort the replace")
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("unverified replace reached Jira: %v", f.calls)
	}
}

func TestEditBatchDescriptionGuard(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", formattedDescADF)

	withStdin(t, `{"key":"NMB-1","description":"batch body"}`+"\n")
	out, err := capture(t, func() error {
		return cmdEdit([]string{"--batch", "-"})
	})
	if err == nil {
		t.Fatalf("batch guard must fail the line (out %s)", out)
	}
	if !strings.Contains(out, "--force-plain") {
		t.Fatalf("envelope row must carry the refusal: %s", out)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("guarded batch line reached Jira: %v", f.calls)
	}

	withStdin(t, `{"key":"NMB-1","description":"batch body"}`+"\n")
	_, err = capture(t, func() error {
		return cmdEdit([]string{"--batch", "-", "--force-plain"})
	})
	if err != nil {
		t.Fatalf("batch --force-plain: %v", err)
	}
	if !f.called("PUT /issue/NMB-1") {
		t.Fatalf("forced batch PUT missing: %v", f.calls)
	}
}

func TestEditMGuardSkipsLinear(t *testing.T) {
	cfg := mirror(t, "")
	cfg.Linear = &config.LinearConfig{APIKey: linearTestKey}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// A Jira description GET needs an Atlassian credential this workspace does
	// not have — the guard must skip Linear (descriptions are plain text end
	// to end), not fail the edit.
	if err := guardDescriptionReplace(context.Background(), cfg, "linear", "FIX-1", false); err != nil {
		t.Fatalf("linear descriptions are plain end to end; guard must skip: %v", err)
	}
}

func TestEditHelpListsForcePlainFlag(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdEdit([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("edit --help: %v", err)
	}
	if !strings.Contains(out, "--force-plain") {
		t.Fatalf("help Options missing --force-plain:\n%s", out)
	}
	if !strings.Contains(helps["edit"].usage, "--force-plain") {
		t.Errorf("usage line missing --force-plain: %s", helps["edit"].usage)
	}
	if !strings.Contains(editUsage, "--force-plain") {
		t.Errorf("editUsage missing --force-plain: %s", editUsage)
	}
}

// GDK-1395: --adf-file is the raw round trip. The file replaces the
// description as it is, the format-loss guard does not run (no markdown is
// involved, so nothing markdown could drop), and -m beside it is refused
// rather than one of them silently winning.
func TestEditADFFileSendsTheDocumentAsItIs(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", formattedDescADF)
	path := filepath.Join(t.TempDir(), "body.json")
	doc := `{"type":"doc","version":1,"content":[{"type":"panel","attrs":{"panelType":"note"},"content":[{"type":"paragraph","content":[{"type":"text","text":"kept"}]}]}]}`
	if err := os.WriteFile(path, []byte(doc+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := capture(t, func() error { return cmdEdit([]string{"NMB-1", "--adf-file", path}) }); err != nil {
		t.Fatalf("edit --adf-file: %v", err)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"panelType":"note"`) || !strings.Contains(body, `"text":"kept"`) {
		t.Fatalf("PUT must carry the file's document: %s", body)
	}
	// The guard reads the origin before the PUT; the search after the PUT is
	// the mirror refresh every write does. Order, not presence, is the claim.
	for _, call := range f.calls {
		if call == "PUT /issue/NMB-1" {
			break
		}
		if call == "POST /search/jql" {
			t.Fatalf("--adf-file must not run the markdown loss guard before the PUT: %v", f.calls)
		}
	}

	_, err := capture(t, func() error { return cmdEdit([]string{"NMB-1", "--adf-file", path, "-m", "x"}) })
	if err == nil || !strings.Contains(err.Error(), "exclusive") {
		t.Fatalf("-m beside --adf-file must be refused, got %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"type":"paragraph"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = capture(t, func() error { return cmdEdit([]string{"NMB-1", "--adf-file", path}) })
	if err == nil || !strings.Contains(err.Error(), "not an ADF document") {
		t.Fatalf("a non-doc root must be refused, got %v", err)
	}
}

func TestCommentADFFilePostsTheDocumentAsItIs(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	path := filepath.Join(t.TempDir(), "comment.json")
	doc := `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"acc-1","text":"@Dana"}},{"type":"text","text":" look"}]}]}`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := capture(t, func() error { return cmdComment([]string{"NMB-1", "--adf-file", path}) }); err != nil {
		t.Fatalf("comment --adf-file: %v", err)
	}
	body := f.bodies["POST /issue/NMB-1/comment"]
	if !strings.Contains(body, `"type":"mention"`) || !strings.Contains(body, `"id":"acc-1"`) {
		t.Fatalf("POST must carry the file's document: %s", body)
	}
	_, err := capture(t, func() error { return cmdComment([]string{"NMB-1", "--adf-file", path, "-m", "x"}) })
	if err == nil || !strings.Contains(err.Error(), "exclusive") {
		t.Fatalf("-m beside --adf-file must be refused, got %v", err)
	}
}

// GDK-1396: `gadak issue` prints a formatted description with placeholders;
// `edit -m -` with that text puts the preserved nodes back where the markers
// stand, so the round trip keeps the panel, the coloured run and the
// mention while the plain text between them changes. A draft with none of
// the markers is still the plain replace GDK-1001 refuses; a deleted marker
// deletes its node and stderr says which.
func TestEditMWithPlaceholdersKeepsThePreservedNodes(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	seedDescriptionADF(t, f, "NMB-1", formattedDescADF)
	src := adf.Source(json.RawMessage(formattedDescADF))
	kept := adf.Preserved(json.RawMessage(formattedDescADF))
	if len(kept) != 3 || kept[0].Type != "textColor" || kept[1].Type != "panel" || kept[2].Type != "mention" {
		t.Fatalf("fixture kept %+v", kept)
	}

	edited := strings.Replace(src, "plain lead", "edited lead", 1)
	edited = strings.Replace(edited, "\n\nnote\n\n", "\n\nnote, **now**\n\n", 1)
	if _, err := capture(t, func() error { return cmdEdit([]string{"NMB-1", "-m", edited}) }); err != nil {
		t.Fatalf("edit with placeholders: %v\n%s", err, edited)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	for _, want := range []string{`"text":"edited lead"`, `"color":"#ff0000"`, `"panelType":"info"`, `"text":"now"`, `"id":"acc"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("PUT lacks %s: %s", want, body)
		}
	}

	// Refusal message now points at the placeholders.
	_, err := capture(t, func() error { return cmdEdit([]string{"NMB-1", "-m", "rewritten"}) })
	if err == nil || !strings.Contains(err.Error(), "placeholders") || !strings.Contains(err.Error(), "--force-plain") {
		t.Fatalf("plain replace must name the placeholder path, got %v", err)
	}

	// A stale marker is refused, not written over.
	seedDescriptionADF(t, f, "NMB-1", strings.Replace(formattedDescADF, `"panelType":"info"`, `"panelType":"note"`, 1))
	_, err = capture(t, func() error { return cmdEdit([]string{"NMB-1", "-m", src}) })
	if err == nil || !strings.Contains(err.Error(), "changed since the body was read") {
		t.Fatalf("stale placeholder must be refused, got %v", err)
	}

	// --force-plain with markers in the text: refused — nothing stands behind them.
	_, err = capture(t, func() error { return cmdEdit([]string{"NMB-1", "-m", src, "--force-plain"}) })
	if err == nil || !strings.Contains(err.Error(), "--force-plain replaces the body without reading it") {
		t.Fatalf("force-plain with markers must be refused, got %v", err)
	}
}

// GDK-1396: a placeholder has meaning only against the body it was read
// from. create -m and comment -m have no such body, so a marker pasted into
// them is refused instead of being stored as visible text.
func TestCreateAndCommentRefuseStrayPlaceholders(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	marker := "<!-- adf:1:0badf00d panel info -->\n\ncloned\n\n<!-- /adf:1 -->"
	_, err := capture(t, func() error {
		return cmdCreate([]string{"cloned issue", "--project", "NMB", "--type", "Task", "-m", marker})
	})
	if err == nil || !strings.Contains(err.Error(), "placeholders") {
		t.Fatalf("create -m with a marker must be refused, got %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("refused create still reached Jira: %v", f.calls)
	}
	_, err = capture(t, func() error { return cmdComment([]string{"NMB-1", "-m", marker}) })
	if err == nil || !strings.Contains(err.Error(), "placeholders") {
		t.Fatalf("comment -m with a marker must be refused, got %v", err)
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("refused comment still reached Jira: %v", f.calls)
	}
}
