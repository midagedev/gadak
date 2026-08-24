package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

// Contract ↔ assertion (clause numbers match the task spec):
//
//  1. Summary: positional words joined; empty / whitespace → usage
//     TestCreateJoinsSummaryWords, TestCreateRejectsEmptySummary
//  1b. GDK-594: first positional that is a project key this workspace knows,
//     with more words and no --project, is refused (not joined into the
//     summary). Unquoted multi-word help example ("Fix the flaky gate") and an
//     unknown uppercase acronym ("API timeout on sync") still file.
//     TestCreateRejectsProjectKeyShapedFirstArg
//     TestCreateJoinsSummaryWords
//     TestCreateAcronymSummaryStillFiles
//     TestCreateQuotedSummaryMayStartWithProjectKey
//     TestCreateProjectKeyPrefixAllowedWhenProjectFlagSet
//  2. --project: sole configured default; 2+ lists keys
//     TestCreateUsesSoleConfiguredProject, TestCreateRequiresProjectWhenAmbiguous
//  3. --type: omitted lists name (id N); case-insensitive name; exact id;
//     unmatched lists alternatives; missing project in createmeta
//     TestCreateTypeOmittedListsAvailable, TestCreateTypeMatchesCaseAndID,
//     TestCreateTypeUnmatchedListsAvailable, TestCreateProjectNotInCreateMeta
//  4. -m: plain text → ADF; omitted → no description; `-` reads stdin
//     TestCreateHappyPathSendsFieldsAndPrintsReread,
//     TestCreateOmitsDescriptionWhenMinusMMissing,
//     TestCreateReadsDescriptionFromStdin
//  5. --label repeatable → labels array
//     TestCreateHappyPathSendsFieldsAndPrintsReread (labels present),
//     TestCreateOmitsLabelsWhenNoneGiven
//  6. CreateIssue fields + new key
//     TestCreateHappyPathSendsFieldsAndPrintsReread
//  7. Write-through tail: re-read row / JSON created; refresh fail; not-in-mirror
//     TestCreateHappyPathSendsFieldsAndPrintsReread,
//     TestCreateJSONIncludesIssueAndCreatedKey,
//     TestCreateRefreshFailureExitsZeroMirrorStale,
//     TestCreateOutsideMirrorPrintsKeyAndExitsZero,
//     TestCreateUnmirroredProjectRefusesBeforeOrigin,
//     TestCreateEmptyProjectsDoesNotRefuse
//  8. No credential → mutate's init error
//     TestWritesRefuseToRunWithoutACredential (agent_test.go),
//     TestCreateRefusesWithoutCredential
//  9. Dispatch + help (usage, --type 작업 example, Writing-through line)
//     TestCreateIsRegisteredAndHelpMentionsNonEnglishType
//
// --batch - (GDK-20):
// 10. Happy 3-line batch: input order, 3 POSTs, 3 stdout lines
//     TestCreateBatchHappyPathPreservesOrder
// 11. Per-line override beats flag default
//     TestCreateBatchLineOverridesFlagDefaults
// 12. Line-2 create failure: line-1 printed, exit non-zero, stderr names
//     line 2, no line-3 POST
//     TestCreateBatchStopsOnCreateFailure
// 13. Malformed JSON line: named line, expected shape, no create
//     TestCreateBatchMalformedJSONCreatesNothing
// 14. --batch + positional summary → usage error
//     TestCreateBatchRejectsPositionalSummary
// 15. --json: one object per line, created+issue (single-create shape)
//     TestCreateBatchJSONOneObjectPerLine
// 16. Per-line attach validated before that line's create
//     TestCreateBatchAttachValidatesBeforeCreate
//
// Unknown flags (GDK-41; clause table lives in flags_test.go):
// 17. Dash-leading token is an unknown flag, not a summary
//     TestCreateSummaryStartingWithDashIsUnknownFlag
//     TestCreateLeadingDashSummaryAfterDoubleDash (flags_test.go)
//
// --priority (GDK-42):
// 18. Name and id both land as priority.id in POST /issue
//     TestCreatePriorityByNameAndID
// 19. Korean-locale catalog name resolves to the same id
//     TestCreatePriorityResolvesKoreanName
// 20. Unknown name lists the catalog; no POST /issue
//     TestCreatePriorityUnmatchedListsCatalogWritesNothing
// 21. --batch -: flag default, per-line override, unmatched line writes nothing
//     TestCreateBatchPriorityFromFlagAndLineOverride
//     TestCreateBatchPriorityUnmatchedWritesNothing
// 22. Genuinely unknown flag still rejected (GDK-41)
//     TestCreateUnknownFlagExitStatus (flags_test.go; --pretty)
//     TestCreateHelpListsPriorityFlag
//
// --parent (GDK-19 / GDK-86 parent half):
// 23. --parent KEY lands as fields.parent.key; invalid shape writes nothing
//     TestCreateParentLandsInPayload
//     TestCreateParentInvalidKeyWritesNothing
// 24. --batch -: flag default, per-line override; invalid line writes nothing
//     TestCreateBatchParentFromFlagAndLineOverride
//     TestCreateBatchParentInvalidWritesNothing
// 25. Write-through refresh stores parent_key
//     TestCreateParentMirrorRefreshLandsParentKey
// 26. Jira hierarchy rejection is surfaced verbatim
//     TestCreateParentRejectedByJiraSurfacesMessage
// 27. Help / usage enumerate --parent
//     TestCreateHelpListsParentFlag
//
// defaults (GDK-218):
// 28. Type/project: flag → config default → sole → fail (no types[0])
//     TestCreateOmitsTypeUsesConfigDefault
//     TestCreateResolveTypeTable
//     TestCreateResolveProjectTable
//     TestCreateUsesDefaultProjectWhenAmbiguous
//     TestCreateSoleTypeWhenProjectHasOne
//     TestCreateStaleDefaultTypeErrors
//     TestCreateFlagTypeBeatsConfigDefault
//     TestCreateTypeOmittedListsAvailable (still fails with no config)
// 29. --json resolved.{project,issue_type}.{value,source}
//     TestCreateOmitsTypeUsesConfigDefault
//     TestCreateUsesDefaultProjectWhenAmbiguous
// 30. defaultIssueTypeId is digits, never a display name
//     TestValidateDefaultIssueTypeIDRejectsNames

func TestCreateHappyPathSendsFieldsAndPrintsReread(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdCreate([]string{
			"Fix", "the", "flaky", "gate",
			"--project", "NMB", "--type", "Task",
			"-m", "body",
			"--label", "a", "--label", "b",
		})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !f.called("POST /issue") {
		t.Fatalf("CreateIssue not called; calls %v", f.calls)
	}
	sent := f.bodies["POST /issue"]
	for _, want := range []string{
		`"key":"NMB"`,
		`"id":"10001"`,
		`"summary":"Fix the flaky gate"`,
		`"type":"doc"`,
		`"text":"body"`,
		`"labels":["a","b"]`,
	} {
		if !strings.Contains(sent, want) {
			t.Errorf("POST /issue missing %s: %s", want, sent)
		}
	}
	// Re-read row: new key, status from the fake's search (완료), not a local echo.
	line := strings.TrimSpace(out)
	fields := strings.Split(line, "\t")
	if len(fields) != 4 || fields[0] != "NMB-42" || fields[1] != "완료" || fields[3] != "Fix the flaky gate" {
		t.Fatalf("reread line %q", out)
	}
	if fields[2] != "Dana Whitfield" {
		t.Errorf("assignee %q", fields[2])
	}
}

func TestCreateJSONIncludesIssueAndCreatedKey(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdCreate([]string{"Fix the flaky gate", "--project", "NMB", "--type", "Task", "--json"})
	})
	if err != nil {
		t.Fatalf("create --json: %v\n%s", err, out)
	}
	var res struct {
		Issue   store.IssueLite `json:"issue"`
		Created struct {
			Key string `json:"key"`
		} `json:"created"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Created.Key != "NMB-42" || res.Issue.IssueKey != "NMB-42" {
		t.Fatalf("json %+v", res)
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &wrap); err != nil {
		t.Fatalf("wrap: %v", err)
	}
	var issueMap map[string]any
	if err := json.Unmarshal(wrap["issue"], &issueMap); err != nil {
		t.Fatalf("issue object: %v", err)
	}
	if issueMap["issue_key"] != "NMB-42" || issueMap["key"] != issueMap["issue_key"] {
		t.Fatalf("create --json issue missing issue_key/key alias pair: %v", issueMap)
	}
	if res.Issue.Status != "완료" {
		t.Errorf("json issue is not the re-read row: %+v", res.Issue)
	}
	if !f.called("POST /issue") {
		t.Fatalf("calls %v", f.calls)
	}
}

func TestCreateJoinsSummaryWords(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"Fix", "the", "flaky", "gate", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"summary":"Fix the flaky gate"`) {
		t.Fatalf("joined summary not sent: %s", sent)
	}
}

func TestCreateRejectsProjectKeyShapedFirstArg(t *testing.T) {
	// GDK-594: `gadak create NMB "이름 없는…"` joined the project key into the
	// summary. Title-case help example "Fix the flaky gate" must keep working
	// (TestCreateJoinsSummaryWords); only a key this workspace knows, with
	// more positionals and no --project, is refused. NMB is in the mirror.
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"NMB", "이름 없는 agent slug에", "--type", "작업"})
	})
	if err == nil {
		t.Fatal("expected project-key-shaped first arg to be refused")
	}
	msg := err.Error()
	for _, want := range []string{"--project", "NMB"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("misparsed create reached Jira: %v", f.calls)
	}
}

func TestCreateAcronymSummaryStillFiles(t *testing.T) {
	// The shape rule alone refused "API timeout on sync" — an uppercase word
	// that is not a project key anywhere in this workspace (lead 2026-08-24).
	// Turning away a legitimate filing is the worse failure, so only known
	// keys are refused.
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"API", "timeout", "on", "sync", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("acronym-led summary: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"summary":"API timeout on sync"`) {
		t.Fatalf("summary: %s", sent)
	}
}

func TestCreateQuotedSummaryMayStartWithProjectKey(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"GDK is missing from the picker", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("quoted summary starting with a key: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"summary":"GDK is missing from the picker"`) {
		t.Fatalf("summary: %s", sent)
	}
}

func TestCreateProjectKeyPrefixAllowedWhenProjectFlagSet(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"GDK", "is a word", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("prefix with --project set: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"summary":"GDK is a word"`) {
		t.Fatalf("summary: %s", sent)
	}
}

func TestCreateRejectsEmptySummary(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "usage: gadak create") {
		t.Fatalf("empty summary: %v", err)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"   ", "--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "usage: gadak create") {
		t.Fatalf("whitespace summary: %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("empty summary reached Jira: %v", f.calls)
	}
}

func TestCreateUsesSoleConfiguredProject(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL) // Projects: [NMB]

	_, err := capture(t, func() error {
		return cmdCreate([]string{"solo project default", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"key":"NMB"`) {
		t.Fatalf("sole project not used: %s", sent)
	}
}

func TestCreateRequiresProjectWhenAmbiguous(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Projects = []string{"NMA", "NMB", "NMS"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := capture(t, func() error {
		return cmdCreate([]string{"needs a project", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("expected --project error")
	}
	for _, want := range []string{"pass --project", "NMA", "NMB", "NMS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("ambiguous project reached Jira: %v", f.calls)
	}

	cfg.Projects = nil
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	_, err = capture(t, func() error {
		return cmdCreate([]string{"needs a project", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "pass --project") {
		t.Fatalf("zero configured: %v", err)
	}
}

func TestCreateTypeOmittedListsAvailable(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"needs a type", "--project", "NMB"})
	})
	if err == nil {
		t.Fatal("expected --type error")
	}
	for _, want := range []string{"pass --type", "Task (id 10001)", "작업 (id 10002)", "Bug (id 10004)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("omitted type reached Jira: %v", f.calls)
	}
}

func TestFormatCreateErrorKeepsFlagAndCatalog(t *testing.T) {
	if got := formatCreateError(&create.NeedProjectError{}).Error(); got != "pass --project" {
		t.Fatalf("empty projects: %q", got)
	}
	if got := formatCreateError(&create.NeedProjectError{Configured: []string{"NMA", "NMB", "NMS"}}).Error(); got != "pass --project, configured: NMA, NMB, NMS" {
		t.Fatalf("configured: %q", got)
	}
	types := []jira.NamedID{{ID: "10001", Name: "Task"}, {ID: "10002", Name: "작업"}, {ID: "10004", Name: "Bug"}}
	if got := formatCreateError(&create.NeedTypeError{Available: types}).Error(); got != "pass --type, available: Task (id 10001); 작업 (id 10002); Bug (id 10004)" {
		t.Fatalf("type: %q", got)
	}
	pris := []jira.NamedID{{ID: "1", Name: "Highest"}, {ID: "2", Name: "High"}, {ID: "3", Name: "Medium"}}
	if got := formatCreateError(&create.NeedPriorityError{Available: pris}).Error(); got != "pass --priority, available: Highest (id 1); High (id 2); Medium (id 3)" {
		t.Fatalf("priority: %q", got)
	}
}

func TestCreateTypeMatchesCaseAndID(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"case fold", "--project", "NMB", "--type", "task"})
	})
	if err != nil {
		t.Fatalf("type task: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"id":"10001"`) {
		t.Fatalf("case-insensitive name: %s", sent)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"by id", "--project", "NMB", "--type", "10002"})
	})
	if err != nil {
		t.Fatalf("type id: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"id":"10002"`) {
		t.Fatalf("exact id: %s", sent)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"korean type", "--project", "NMB", "--type", "작업"})
	})
	if err != nil {
		t.Fatalf("type 작업: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"id":"10002"`) {
		t.Fatalf("korean name: %s", sent)
	}
}

func TestCreateTypeUnmatchedListsAvailable(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"no such type", "--project", "NMB", "--type", "Epic"})
	})
	if err == nil {
		t.Fatal("expected unmatched type error")
	}
	for _, want := range []string{`no issue type matching "Epic"`, "Task (id 10001)", "작업 (id 10002)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("unmatched type reached Jira: %v", f.calls)
	}
}

func TestCreateProjectNotInCreateMeta(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	// Mirrored so the GDK-682 pre-check does not fire; this test is the
	// createmeta miss, not project_not_mirrored.
	cfg.Projects = []string{"NMB", "ZZZ"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := capture(t, func() error {
		return cmdCreate([]string{"cannot file here", "--project", "ZZZ", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "cannot create issues in ZZZ") {
		t.Fatalf("missing createmeta project: %v", err)
	}
	for _, want := range []string{"available:", "NMB", "GDK"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("unmatched project must list catalog %q, got %v", want, err)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("unreachable project reached Jira: %v", f.calls)
	}
}

func TestCreateOmitsDescriptionWhenMinusMMissing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"no body", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if strings.Contains(sent, `"description"`) {
		t.Fatalf("description sent when -m omitted: %s", sent)
	}
}

func TestCreateRejectsSignedLabelSyntax(t *testing.T) {
	// GDK-545: create --label must refuse edit's +x/-x verbs, not send them
	// as literal label names.
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	for _, label := range []string{"+regression", "-needs-triage"} {
		_, err := capture(t, func() error {
			return cmdCreate([]string{"a summary", "--label", label, "--project", "NMB", "--type", "Task"})
		})
		if err == nil {
			t.Fatalf("--label %q was accepted", label)
		}
		msg := err.Error()
		if !strings.Contains(msg, "plain label names") {
			t.Errorf("--label %q error missing 'plain label names': %q", label, msg)
		}
		if !strings.Contains(msg, "gadak edit") {
			t.Errorf("--label %q error must point at gadak edit: %q", label, msg)
		}
	}
}

func TestCreateOmitsLabelsWhenNoneGiven(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"no labels", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if strings.Contains(sent, `"labels"`) {
		t.Fatalf("labels sent when none given: %s", sent)
	}
}

func TestCreateReadsDescriptionFromStdin(t *testing.T) {
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
		return cmdCreate([]string{"stdin body", "-m", "-", "--project", "NMB", "--type", "Task"})
	})
	os.Stdin = saved
	if err != nil {
		t.Fatalf("create -m -: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if !strings.Contains(sent, `"type":"doc"`) || !strings.Contains(sent, "line two") {
		t.Fatalf("stdin body not sent: %s", sent)
	}
}

func TestCreateRefreshFailureExitsZeroMirrorStale(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		f.rereadStatus = 422

		stdout, stderr, err := captureBoth(t, func() error {
			return cmdCreate([]string{"refresh will fail", "--project", "NMB", "--type", "Task"})
		})
		if err != nil {
			t.Fatalf("create must exit 0 after a landed write: %v", err)
		}
		if !strings.Contains(stderr, "write applied to NMB-42, but the mirror did not refresh") {
			t.Fatalf("stderr missing write-applied warning: %q", stderr)
		}
		if strings.TrimSpace(stdout) != "NMB-42" {
			t.Fatalf("stdout %q, want the new key (no mirror row)", stdout)
		}
		if !f.called("POST /issue") {
			t.Fatal("write must have reached Jira before the refresh warning")
		}
	})

	t.Run("json", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		f.rereadStatus = 422

		stdout, stderr, err := captureBoth(t, func() error {
			return cmdCreate([]string{"refresh json", "--project", "NMB", "--type", "Task", "--json"})
		})
		if err != nil {
			t.Fatalf("create --json must exit 0 after a landed write: %v", err)
		}
		if !strings.Contains(stderr, "write applied to NMB-42, but the mirror did not refresh") {
			t.Fatalf("stderr missing write-applied warning: %q", stderr)
		}
		var body struct {
			Created struct {
				Key string `json:"key"`
			} `json:"created"`
			Issue       *store.IssueLite `json:"issue"`
			MirrorStale bool             `json:"mirror_stale"`
		}
		if err := json.Unmarshal([]byte(stdout), &body); err != nil {
			t.Fatalf("decode %q: %v", stdout, err)
		}
		if body.Created.Key != "NMB-42" || body.Issue != nil || !body.MirrorStale {
			t.Fatalf("json %+v (raw %q)", body, stdout)
		}
		if !f.called("POST /issue") {
			t.Fatal("write must have reached Jira before the refresh warning")
		}
	})
}

func TestCreateOutsideMirrorPrintsKeyAndExitsZero(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.skipCreateReread = true

	stdout, stderr, err := captureBoth(t, func() error {
		return cmdCreate([]string{"outside the mirror", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create must exit 0 after a successful write: %v", err)
	}
	if strings.TrimSpace(stdout) != "NMB-42" {
		t.Fatalf("stdout %q, want the new key", stdout)
	}
	if !strings.Contains(stderr, "not in the mirror — is it outside the configured projects?") {
		t.Fatalf("stderr missing outside-projects wording: %q", stderr)
	}

	stdout, stderr, err = captureBoth(t, func() error {
		return cmdCreate([]string{"outside json", "--project", "NMB", "--type", "Task", "--json"})
	})
	if err != nil {
		t.Fatalf("create --json must exit 0 after a successful write: %v", err)
	}
	var body struct {
		Created struct {
			Key string `json:"key"`
		} `json:"created"`
		Issue *store.IssueLite `json:"issue"`
	}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if body.Created.Key != "NMB-43" || body.Issue != nil {
		t.Fatalf("json miss %+v (raw %q)", body, stdout)
	}
	if !strings.Contains(stderr, "not in the mirror") {
		t.Fatalf("json miss stderr %q", stderr)
	}
}

func TestCreateIntoUnconfiguredProjectStillPrintsRow(t *testing.T) {
	// GDK-682 inverted this: a create in GDK when cfg.Projects is [NMB] is
	// refused before origin (REST already did). Refresh-by-key for a
	// successful write is TestCreateHappyPath / TestCreateOutsideMirrorPrintsKeyAndExitsZero.
	TestCreateUnmirroredProjectRefusesBeforeOrigin(t)
}

// TestCreateUnmirroredProjectRefusesBeforeOrigin is FAIL-first for GDK-682:
// REST refuses project_not_mirrored before CreateIssue; CLI currently writes
// then warns. The pre-check must not call origin at all.
func TestCreateUnmirroredProjectRefusesBeforeOrigin(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL) // Projects: [NMB]

	_, err := capture(t, func() error {
		return cmdCreate([]string{"filed in GDK", "--project", "GDK", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("create into a project outside cfg.Projects must refuse")
	}
	msg := err.Error()
	if !strings.Contains(msg, "GDK") {
		t.Errorf("must name the refused project: %v", err)
	}
	if !strings.Contains(msg, "NMB") {
		t.Errorf("must list mirrored projects: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("origin called before refuse: %v", f.calls)
	}
	t.Log(msg)
}

// TestCreateEmptyProjectsDoesNotRefuse pins GDK-467: paired workspaces have
// no local Projects copy. An empty list is not "allow 0 projects".
func TestCreateEmptyProjectsDoesNotRefuse(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Projects = nil
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := capture(t, func() error {
		return cmdCreate([]string{"paired workspace create", "--project", "GDK", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("empty cfg.Projects must not refuse create: %v", err)
	}
	if !f.called("POST /issue") {
		t.Fatalf("CreateIssue not called: %v", f.calls)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"key":"GDK"`) {
		t.Fatalf("GDK not sent: %s", sent)
	}
}

func TestCreateRefusesWithoutCredential(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Token = ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdCreate([]string{"no cred", "--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "gadak init") {
		t.Fatalf("no credential: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("called Jira anyway: %v", f.calls)
	}
}

func TestCreateIsRegisteredAndHelpMentionsNonEnglishType(t *testing.T) {
	run, ok := commands["create"]
	if !ok || run == nil {
		t.Fatal("create missing from dispatch map")
	}
	h, ok := helps["create"]
	if !ok {
		t.Fatal("create missing from helps")
	}
	if !strings.Contains(h.usage, "gadak [--workspace <name>] create") {
		t.Errorf("usage: %s", h.usage)
	}
	joined := strings.Join(h.examples, "\n")
	if !strings.Contains(joined, "--type 작업") {
		t.Errorf("examples missing --type 작업:\n%s", joined)
	}
	if !strings.Contains(usage, "create     create an issue") {
		t.Errorf("top-level Writing-through block missing create:\n%s", usage)
	}
}

// Self-review defect classes.

func TestCreateSummaryStartingWithDashIsUnknownFlag(t *testing.T) {
	// Was: TestCreateSummaryStartingWithDashIsPositional — that assertion
	// protected the swallow that GDK-41 closes. A summary that starts with
	// `-` now goes after `--` (TestCreateLeadingDashSummaryAfterDoubleDash).
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"-this", "broke", "--project", "NMB", "--type", "Task"})
	})
	if err == nil {
		t.Fatalf("-this treated as summary; Jira calls %v", f.calls)
	}
	if !strings.Contains(err.Error(), "unknown flag -this") {
		t.Fatalf("want unknown flag -this, got %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("leading-dash token reached Jira: %v", f.calls)
	}
}

func TestCreateAttachValidatesBeforeCreate(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	missing := filepath.Join(t.TempDir(), "typo.png")

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"with attach typo", "--project", "NMB", "--type", "Task",
			"--attach", missing,
		})
	})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("typo: %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("create reached Jira: %v", f.calls)
	}
	if len(f.uploads) != 0 {
		t.Fatalf("uploaded despite typo: %+v", f.uploads)
	}
}

func TestCreateAttachUploadFailureReportsKeyAndDoesNotRetry(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.failNthAttach = 1
	p := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(p, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"created then attach fails", "--project", "NMB", "--type", "Task",
			"--attach", p,
		})
	})
	if err == nil {
		t.Fatal("expected attach failure after create")
	}
	msg := err.Error()
	if !strings.Contains(msg, "NMB-42") {
		t.Errorf("error must name the new key: %q", msg)
	}
	if !strings.Contains(msg, "shot.png") && !strings.Contains(msg, p) {
		t.Errorf("error must name the file: %q", msg)
	}
	nCreate := 0
	for _, c := range f.calls {
		if c == "POST /issue" {
			nCreate++
		}
	}
	if nCreate != 1 {
		t.Fatalf("create retried or skipped: %d POSTs in %v", nCreate, f.calls)
	}
	if f.nextCreateN != 43 {
		t.Fatalf("nextCreateN=%d, want 43 (no second create)", f.nextCreateN)
	}
}

func TestCreateAttachJSONIncludesAttached(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.log")
	if err := os.WriteFile(a, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdCreate([]string{
			"with files", "--project", "NMB", "--type", "Task",
			"--attach", a, "--attach", b, "--json",
		})
	})
	if err != nil {
		t.Fatalf("create --attach --json: %v\n%s", err, out)
	}
	if !f.called("POST /issue") {
		t.Fatalf("create not called: %v", f.calls)
	}
	if len(f.uploads) != 2 || f.uploads[0].Filename != "a.png" || f.uploads[1].Filename != "b.log" {
		t.Fatalf("uploads %+v", f.uploads)
	}
	if f.uploads[0].Key != "NMB-42" {
		t.Fatalf("uploaded to %q, want NMB-42", f.uploads[0].Key)
	}
	var res struct {
		Issue   store.IssueLite `json:"issue"`
		Created struct {
			Key string `json:"key"`
		} `json:"created"`
		Attached []struct {
			ID       string `json:"id"`
			Filename string `json:"filename"`
		} `json:"attached"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Created.Key != "NMB-42" || res.Issue.IssueKey != "NMB-42" {
		t.Fatalf("json %+v", res)
	}
	if len(res.Attached) != 2 || res.Attached[0].ID != "20001" || res.Attached[1].Filename != "b.log" {
		t.Fatalf("attached %+v", res.Attached)
	}
}

func TestCreateStdinMinusMDoesNotJoinIntoSummary(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("from stdin")
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	_, err = capture(t, func() error {
		return cmdCreate([]string{"Title", "here", "-m", "-", "--project", "NMB", "--type", "Task"})
	})
	os.Stdin = saved
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if !strings.Contains(sent, `"summary":"Title here"`) {
		t.Errorf("summary %s", sent)
	}
	if !strings.Contains(sent, "from stdin") {
		t.Errorf("description %s", sent)
	}
}

func TestCreateBatchHappyPathPreservesOrder(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		"{\"summary\":\"first\"}\n"+
		"\n"+
		"{\"summary\":\"second\"}\n"+
		"{\"summary\":\"third\"}\n")

	out, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("batch: %v\n%s", err, out)
	}
	if n := countCalls(f, "POST /issue"); n != 3 {
		t.Fatalf("POST /issue count %d in %v", n, f.calls)
	}
	lines := nonEmptyLines(out)
	if len(lines) != 3 {
		t.Fatalf("stdout lines %q", out)
	}
	want := []struct{ key, summary string }{
		{"NMB-42", "first"},
		{"NMB-43", "second"},
		{"NMB-44", "third"},
	}
	for i, w := range want {
		fields := strings.Split(lines[i], "\t")
		if len(fields) != 2 {
			t.Errorf("line %d: want KEY\\tsummary, got %d fields in %q", i+1, len(fields), lines[i])
		}
		key, summary := tsvKeySummary(lines[i])
		if key != w.key || summary != w.summary {
			t.Errorf("line %d: %q want %s / %s", i+1, lines[i], w.key, w.summary)
		}
		if !strings.Contains(f.createBodies[i], `"summary":"`+w.summary+`"`) {
			t.Errorf("POST %d missing summary %q: %s", i+1, w.summary, f.createBodies[i])
		}
	}
}

func TestCreateBatchLineOverridesFlagDefaults(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Projects = []string{"NMB", "GDK"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	withStdin(t, ""+
		"{\"summary\":\"from flags\"}\n"+
		"{\"summary\":\"from line\",\"project\":\"GDK\",\"type\":\"Task\",\"labels\":[\"fromline\"],\"description\":\"line body\"}\n")

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"--batch", "-",
			"--project", "NMB", "--type", "Task",
			"--label", "fromflag",
			"-m", "flag body",
		})
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(f.createBodies) != 2 {
		t.Fatalf("bodies %d: %v", len(f.createBodies), f.createBodies)
	}
	first, second := f.createBodies[0], f.createBodies[1]
	for _, want := range []string{`"key":"NMB"`, `"id":"10001"`, `"summary":"from flags"`, `"labels":["fromflag"]`, `"text":"flag body"`} {
		if !strings.Contains(first, want) {
			t.Errorf("line 1 POST missing %s: %s", want, first)
		}
	}
	for _, want := range []string{`"key":"GDK"`, `"id":"10001"`, `"summary":"from line"`, `"labels":["fromline"]`, `"text":"line body"`} {
		if !strings.Contains(second, want) {
			t.Errorf("line 2 POST missing %s: %s", want, second)
		}
	}
	if strings.Contains(second, "fromflag") || strings.Contains(second, "flag body") {
		t.Errorf("line 2 still carries flag defaults: %s", second)
	}
}

func TestCreateBatchStopsOnCreateFailure(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.failNthCreate = 2
	withStdin(t, ""+
		"{\"summary\":\"kept\"}\n"+
		"{\"summary\":\"fails\"}\n"+
		"{\"summary\":\"never\"}\n")

	stdout, stderr, err := captureBoth(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("expected line-2 create failure")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error must name line 2: %v", err)
	}
	if !strings.Contains(err.Error(), "forced create failure") {
		t.Errorf("error must carry the Jira message: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("stdout must be the first success only, got %q", stdout)
	}
	key, summary := tsvKeySummary(lines[0])
	if key != "NMB-42" || summary != "kept" {
		t.Errorf("printed %q", lines[0])
	}
	if n := countCalls(f, "POST /issue"); n != 2 {
		t.Fatalf("want 2 POSTs (line 3 must not create), got %d in %v", n, f.calls)
	}
	if f.nextCreateN != 43 {
		t.Fatalf("nextCreateN=%d, want 43 (failed create must not consume a key)", f.nextCreateN)
	}
	_ = stderr
}

func TestCreateBatchMalformedJSONCreatesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, "{\"summary\":\"ok\"\n") // missing closing brace

	_, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "line 1") {
		t.Errorf("must name line 1: %v", err)
	}
	if !strings.Contains(msg, `"summary"`) || !strings.Contains(msg, "attach") || !strings.Contains(msg, "parent") {
		t.Errorf("must remind of the expected shape: %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("malformed JSON reached Jira: %v", f.calls)
	}
}

func TestCreateBatchMalformedJSONAfterSuccessStops(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		"{\"summary\":\"kept\"}\n"+
		"\n"+
		"{not-json}\n"+
		"{\"summary\":\"never\"}\n")

	stdout, _, err := captureBoth(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("expected malformed JSON on line 3")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("physical line number, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"summary"`) {
		t.Errorf("shape reminder: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("stdout %q", stdout)
	}
	if n := countCalls(f, "POST /issue"); n != 1 {
		t.Fatalf("POSTs %d in %v", n, f.calls)
	}
}

func TestCreateBatchRejectsNonDashValue(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "issues.jsonl", "--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "--batch only accepts -") {
		t.Fatalf("--batch path: %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("reached Jira: %v", f.calls)
	}
}

func TestCreateBatchRejectsPositionalSummary(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, "{\"summary\":\"unused\"}\n")

	_, err := capture(t, func() error {
		return cmdCreate([]string{"a summary", "--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "--batch") && !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("usage error should name the conflict: %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("positional+batch reached Jira: %v", f.calls)
	}
}

func TestCreateBatchJSONOneObjectPerLine(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		"{\"summary\":\"alpha\"}\n"+
		"{\"summary\":\"beta\"}\n")

	out, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task", "--json"})
	})
	if err != nil {
		t.Fatalf("batch --json: %v\n%s", err, out)
	}
	dec := json.NewDecoder(strings.NewReader(out))
	var got []struct {
		Issue   store.IssueLite `json:"issue"`
		Created struct {
			Key string `json:"key"`
		} `json:"created"`
	}
	for dec.More() {
		var row struct {
			Issue   store.IssueLite `json:"issue"`
			Created struct {
				Key string `json:"key"`
			} `json:"created"`
		}
		if err := dec.Decode(&row); err != nil {
			t.Fatalf("decode %q: %v", out, err)
		}
		got = append(got, row)
	}
	if len(got) != 2 {
		t.Fatalf("json objects %d in %q", len(got), out)
	}
	if got[0].Created.Key != "NMB-42" || got[0].Issue.IssueKey != "NMB-42" || got[0].Issue.Summary != "alpha" {
		t.Errorf("row 0 %+v", got[0])
	}
	if got[1].Created.Key != "NMB-43" || got[1].Issue.IssueKey != "NMB-43" || got[1].Issue.Summary != "beta" {
		t.Errorf("row 1 %+v", got[1])
	}
	if got[0].Issue.Status != "완료" || got[1].Issue.Status != "완료" {
		t.Errorf("json issue is not the re-read row: %+v", got)
	}
}

func TestCreateBatchAttachValidatesBeforeCreate(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	missing := filepath.Join(t.TempDir(), "typo.png")
	withStdin(t, fmt.Sprintf("{\"summary\":\"with typo\",\"attach\":[%q]}\n{\"summary\":\"never\"}\n", missing))

	_, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("typo: %v", err)
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("must name line 1: %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("create reached Jira: %v", f.calls)
	}
	if len(f.uploads) != 0 {
		t.Fatalf("uploaded despite typo: %+v", f.uploads)
	}
}

func TestCreateBatchAttachValidatesThatLineOnly(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	missing := filepath.Join(t.TempDir(), "typo.png")
	withStdin(t, fmt.Sprintf("{\"summary\":\"ok\"}\n{\"summary\":\"bad\",\"attach\":[%q]}\n{\"summary\":\"never\"}\n", missing))

	stdout, _, err := captureBoth(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("typo: %v", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("must name line 2: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("stdout %q", stdout)
	}
	if n := countCalls(f, "POST /issue"); n != 1 {
		t.Fatalf("POSTs %d in %v", n, f.calls)
	}
}

func TestCreatePriorityByNameAndID(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"by name", "--project", "NMB", "--type", "Task", "--priority", "High"})
	})
	if err != nil {
		t.Fatalf("priority High: %v", err)
	}
	if !f.called("GET /priority") {
		t.Fatalf("PriorityCatalog not called: %v", f.calls)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"priority":{"id":"2"}`) {
		t.Fatalf("name High should send id 2: %s", sent)
	}
	pri, post := -1, -1
	for i, c := range f.calls {
		if c == "GET /priority" && pri < 0 {
			pri = i
		}
		if c == "POST /issue" && post < 0 {
			post = i
		}
	}
	if pri < 0 || post < 0 || pri > post {
		t.Fatalf("catalog must resolve before create: %v", f.calls)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"by id", "--project", "NMB", "--type", "Task", "--priority", "1"})
	})
	if err != nil {
		t.Fatalf("priority id 1: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"priority":{"id":"1"}`) {
		t.Fatalf("id 1 should send id 1: %s", sent)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"case fold", "--project", "NMB", "--type", "Task", "--priority", "medium"})
	})
	if err != nil {
		t.Fatalf("priority medium: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"priority":{"id":"3"}`) {
		t.Fatalf("case-insensitive Medium: %s", sent)
	}
}

func TestCreatePriorityResolvesKoreanName(t *testing.T) {
	f := newFakeJira(t)
	f.lang = "ko"
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"korean priority", "--project", "NMB", "--type", "Task", "--priority", "높음"})
	})
	if err != nil {
		t.Fatalf("priority 높음: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"priority":{"id":"2"}`) {
		t.Fatalf("높음 should send id 2: %s", sent)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"korean highest", "--project", "NMB", "--type", "Task", "--priority", "가장 높음"})
	})
	if err != nil {
		t.Fatalf("priority 가장 높음: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"priority":{"id":"1"}`) {
		t.Fatalf("가장 높음 should send id 1: %s", sent)
	}

	// English display names are not a second key. The catalog is already in
	// the account language (PriorityCatalog); matching "High" here would
	// reintroduce the localized-name trap.
	_, err = capture(t, func() error {
		return cmdCreate([]string{"english on ko", "--project", "NMB", "--type", "Task", "--priority", "High"})
	})
	if err == nil {
		t.Fatal("English High must not match a Korean catalog")
	}
	if !strings.Contains(err.Error(), `no priority matching "High"`) || !strings.Contains(err.Error(), "높음 (id 2)") {
		t.Fatalf("want Korean catalog listed, got %v", err)
	}
	// Two successful creates already POSTed; this miss must not add a third.
	if n := countCalls(f, "POST /issue"); n != 2 {
		t.Fatalf("English miss reached Jira: %d POSTs in %v", n, f.calls)
	}
}

func TestCreatePriorityUnmatchedListsCatalogWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"no such priority", "--project", "NMB", "--type", "Task", "--priority", "Urgent"})
	})
	if err == nil {
		t.Fatal("expected unmatched priority error")
	}
	for _, want := range []string{`no priority matching "Urgent"`, "Highest (id 1)", "High (id 2)", "Medium (id 3)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("unmatched priority reached Jira: %v", f.calls)
	}
}

func TestCreateOmitsPriorityWhenNoneGiven(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"no priority", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if strings.Contains(sent, `"priority"`) {
		t.Fatalf("priority sent when flag omitted: %s", sent)
	}
	// SyncIssue's re-read also hits GET /priority (site catalog). The
	// contract is that the create POST itself has no priority field.
}

func TestCreateBatchPriorityFromFlagAndLineOverride(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		"{\"summary\":\"from flag\"}\n"+
		"{\"summary\":\"from line\",\"priority\":\"Medium\"}\n"+
		"{\"summary\":\"id on line\",\"priority\":\"1\"}\n")

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"--batch", "-",
			"--project", "NMB", "--type", "Task",
			"--priority", "High",
		})
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(f.createBodies) != 3 {
		t.Fatalf("bodies %d: %v", len(f.createBodies), f.createBodies)
	}
	if !strings.Contains(f.createBodies[0], `"priority":{"id":"2"}`) {
		t.Errorf("line 1 should use flag High → id 2: %s", f.createBodies[0])
	}
	if !strings.Contains(f.createBodies[1], `"priority":{"id":"3"}`) {
		t.Errorf("line 2 should override to Medium → id 3: %s", f.createBodies[1])
	}
	if !strings.Contains(f.createBodies[2], `"priority":{"id":"1"}`) {
		t.Errorf("line 3 should override to id 1: %s", f.createBodies[2])
	}
}

func TestCreateBatchPriorityUnmatchedWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		"{\"summary\":\"kept\"}\n"+
		"{\"summary\":\"bad\",\"priority\":\"Urgent\"}\n"+
		"{\"summary\":\"never\"}\n")

	stdout, _, err := captureBoth(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task", "--priority", "High"})
	})
	if err == nil {
		t.Fatal("expected unmatched priority on line 2")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("must name line 2: %v", err)
	}
	if !strings.Contains(err.Error(), `no priority matching "Urgent"`) {
		t.Errorf("must list the miss: %v", err)
	}
	for _, want := range []string{"Highest (id 1)", "High (id 2)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("stdout must be the first success only, got %q", stdout)
	}
	if n := countCalls(f, "POST /issue"); n != 1 {
		t.Fatalf("want 1 POST (line 2 must not create), got %d in %v", n, f.calls)
	}
	if !strings.Contains(f.createBodies[0], `"priority":{"id":"2"}`) {
		t.Errorf("line 1 POST missing flag priority: %s", f.createBodies[0])
	}
}

func TestCreateHelpListsPriorityFlag(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdCreate([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("create --help: %v", err)
	}
	if !strings.Contains(out, "--priority") {
		t.Fatalf("help Options missing --priority:\n%s", out)
	}
	if !strings.Contains(out, "--priority") || !strings.Contains(helps["create"].usage, "--priority") {
		t.Errorf("usage line missing --priority: %s", helps["create"].usage)
	}
	joined := strings.Join(helps["create"].examples, "\n")
	if !strings.Contains(joined, "--priority") {
		t.Errorf("examples missing --priority:\n%s", joined)
	}
}

func withStdin(t *testing.T, s string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(s)
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = saved })
}

func countCalls(f *fakeJira, tag string) int {
	n := 0
	for _, c := range f.calls {
		if c == tag {
			n++
		}
	}
	return n
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func tsvKeySummary(line string) (key, summary string) {
	fields := strings.Split(strings.TrimRight(line, "\n"), "\t")
	if len(fields) == 0 {
		return "", ""
	}
	key = fields[0]
	if len(fields) > 0 {
		summary = fields[len(fields)-1]
	}
	return key, summary
}

func captureBoth(t *testing.T, fn func() error) (stdout, stderr string, err error) {
	t.Helper()
	or, ow, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	er, ew, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	saveOut, saveErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = ow, ew
	err = fn()
	os.Stdout, os.Stderr = saveOut, saveErr
	_ = ow.Close()
	_ = ew.Close()
	outB, _ := io.ReadAll(or)
	errB, _ := io.ReadAll(er)
	return string(outB), string(errB), err
}

func TestCreateParentLandsInPayload(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"child of the audit", "--project", "NMB", "--type", "Task",
			"--parent", "nmb-1",
		})
	})
	if err != nil {
		t.Fatalf("create --parent: %v", err)
	}
	if !f.called("POST /issue") {
		t.Fatalf("CreateIssue not called; calls %v", f.calls)
	}
	sent := f.bodies["POST /issue"]
	if !strings.Contains(sent, `"parent":{"key":"NMB-1"}`) {
		t.Fatalf("parent key not sent (or not normalized): %s", sent)
	}
	for _, want := range []string{`"key":"NMB"`, `"id":"10001"`, `"summary":"child of the audit"`} {
		if !strings.Contains(sent, want) {
			t.Errorf("POST /issue missing %s: %s", want, sent)
		}
	}
}

func TestCreateOmitsParentWhenNoneGiven(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"no parent", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if strings.Contains(sent, `"parent"`) {
		t.Fatalf("parent sent when flag omitted: %s", sent)
	}
}

func TestCreateParentInvalidKeyWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"bad parent", "--project", "NMB", "--type", "Task", "--parent", "not-a-key"})
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
	if len(f.calls) != 0 {
		t.Fatalf("invalid parent reached Jira: %v", f.calls)
	}

	_, err = capture(t, func() error {
		return cmdCreate([]string{"none is not a create parent", "--project", "NMB", "--type", "Task", "--parent", "none"})
	})
	if err == nil || !strings.Contains(err.Error(), `not a Jira key (want ABC-123)`) {
		t.Fatalf("create --parent none must be a key error, got %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("--parent none reached Jira: %v", f.calls)
	}
}

func TestCreateBatchParentFromFlagAndLineOverride(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		"{\"summary\":\"from flag\"}\n"+
		"{\"summary\":\"from line\",\"parent\":\"GDK-1\"}\n")

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"--batch", "-",
			"--project", "NMB", "--type", "Task",
			"--parent", "NMB-1",
		})
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(f.createBodies) != 2 {
		t.Fatalf("bodies %d: %v", len(f.createBodies), f.createBodies)
	}
	if !strings.Contains(f.createBodies[0], `"parent":{"key":"NMB-1"}`) {
		t.Errorf("line 1 should use flag parent NMB-1: %s", f.createBodies[0])
	}
	if !strings.Contains(f.createBodies[1], `"parent":{"key":"GDK-1"}`) {
		t.Errorf("line 2 should override to GDK-1: %s", f.createBodies[1])
	}
	if strings.Contains(f.createBodies[1], `"parent":{"key":"NMB-1"}`) {
		t.Errorf("line 2 still carries flag parent: %s", f.createBodies[1])
	}
}

func TestCreateBatchParentInvalidWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		"{\"summary\":\"kept\"}\n"+
		"{\"summary\":\"bad\",\"parent\":\"not-a-key\"}\n"+
		"{\"summary\":\"never\"}\n")

	stdout, _, err := captureBoth(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task", "--parent", "NMB-1"})
	})
	if err == nil {
		t.Fatal("expected invalid parent on line 2")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("must name line 2: %v", err)
	}
	if !strings.Contains(err.Error(), `not a Jira key (want ABC-123)`) {
		t.Errorf("must use the key wording: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("stdout must be the first success only, got %q", stdout)
	}
	if n := countCalls(f, "POST /issue"); n != 1 {
		t.Fatalf("want 1 POST (line 2 must not create), got %d in %v", n, f.calls)
	}
	if !strings.Contains(f.createBodies[0], `"parent":{"key":"NMB-1"}`) {
		t.Errorf("line 1 POST missing flag parent: %s", f.createBodies[0])
	}
}

func TestCreateParentMirrorRefreshLandsParentKey(t *testing.T) {
	f := newFakeJira(t)
	echoParentOnReread(t, f)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdCreate([]string{
			"child of the audit", "--project", "NMB", "--type", "Task",
			"--parent", "NMB-1", "--json",
		})
	})
	if err != nil {
		t.Fatalf("create --parent --json: %v\n%s", err, out)
	}
	var res struct {
		Issue store.IssueLite `json:"issue"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Issue.ParentKey == nil || *res.Issue.ParentKey != "NMB-1" {
		t.Fatalf("mirror parent_key = %v, want NMB-1", res.Issue.ParentKey)
	}
	if res.Issue.IssueKey != "NMB-42" {
		t.Fatalf("json issue %+v", res.Issue)
	}
}

func TestCreateParentRejectedByJiraSurfacesMessage(t *testing.T) {
	f := newFakeJira(t)
	rejectParentWrite(t, f, "NMB-999", "Issue type cannot be a child of the selected parent.")
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"hierarchy miss", "--project", "NMB", "--type", "Task",
			"--parent", "NMB-999",
		})
	})
	if err == nil {
		t.Fatal("expected Jira hierarchy rejection")
	}
	if !strings.Contains(err.Error(), "Issue type cannot be a child of the selected parent.") {
		t.Fatalf("must carry Jira's message, got %v", err)
	}
	if !f.called("POST /issue") {
		t.Fatalf("valid key must reach Jira: %v", f.calls)
	}
}

func TestCreateHelpListsParentFlag(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdCreate([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("create --help: %v", err)
	}
	if !strings.Contains(out, "--parent") {
		t.Fatalf("help Options missing --parent:\n%s", out)
	}
	if !strings.Contains(helps["create"].usage, "--parent") {
		t.Errorf("usage line missing --parent: %s", helps["create"].usage)
	}
	if !strings.Contains(createUsage, "--parent") {
		t.Errorf("createUsage missing --parent: %s", createUsage)
	}
}

func TestCreateDueLandsInPayload(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"due next week", "--project", "NMB", "--type", "Task",
			"--due", "2026-09-01",
		})
	})
	if err != nil {
		t.Fatalf("create --due: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if !strings.Contains(sent, `"duedate":"2026-09-01"`) {
		t.Fatalf("duedate not sent: %s", sent)
	}
}

func TestCreateOmitsDueWhenNoneGiven(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"no due", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if strings.Contains(sent, `"duedate"`) {
		t.Fatalf("duedate sent when flag omitted: %s", sent)
	}
}

func TestCreateDueInvalidWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"bad due", "--project", "NMB", "--type", "Task", "--due", "01/09/2026"})
	})
	if err == nil {
		t.Fatal("expected invalid --due error")
	}
	if !strings.Contains(err.Error(), "want YYYY-MM-DD") {
		t.Fatalf("wording: %v", err)
	}
	if !strings.Contains(err.Error(), "01/09/2026") {
		t.Fatalf("error should name the value: %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("invalid --due reached Jira: %v", f.calls)
	}
}

func TestCreateDueNoneIsNotADate(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"none is not create due", "--project", "NMB", "--type", "Task", "--due", "none"})
	})
	if err == nil || !strings.Contains(err.Error(), "want YYYY-MM-DD") {
		t.Fatalf("create --due none must be a format error, got %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("--due none reached Jira: %v", f.calls)
	}
}

func TestCreateHelpListsDueFlag(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdCreate([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("create --help: %v", err)
	}
	if !strings.Contains(out, "--due") {
		t.Fatalf("help Options missing --due:\n%s", out)
	}
	if !strings.Contains(helps["create"].usage, "--due") {
		t.Errorf("usage line missing --due: %s", helps["create"].usage)
	}
	if !strings.Contains(createUsage, "--due") {
		t.Errorf("createUsage missing --due: %s", createUsage)
	}
}

// echoParentOnReread rewrites the fake's /search/jql payload so a created or
// edited issue's fields.parent matches the last write. The harness itself
// lives in agent_test.go (outside this round's file whitelist).
func echoParentOnReread(t *testing.T, f *fakeJira) {
	t.Helper()
	parents := map[string]string{}
	inner := f.Config.Handler
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		inner.ServeHTTP(rec, r)

		path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
		switch {
		case r.Method == http.MethodPost && path == "/issue":
			if k := f.lastCreatedKey; k != "" {
				if p, set, clear := parentFieldFromWriteBody(f.bodies["POST /issue"]); set {
					if clear {
						delete(parents, k)
					} else {
						parents[k] = p
					}
				}
			}
		case r.Method == http.MethodPut && strings.HasPrefix(path, "/issue/") && strings.Count(path, "/") == 2:
			key := strings.TrimPrefix(path, "/issue/")
			if p, set, clear := parentFieldFromWriteBody(f.bodies[r.Method+" "+path]); set {
				if clear {
					delete(parents, key)
				} else {
					parents[key] = p
				}
			}
		}

		body := rec.Body.Bytes()
		if path == "/search/jql" {
			body = injectParentIntoSearch(body, parents)
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
		_, _ = w.Write(body)
	})
}

// rejectParentWrite answers 400 with jiraMsg when a create/edit payload
// names that parent key, so tests can assert Jira's wording is not rewritten.
func rejectParentWrite(t *testing.T, f *fakeJira, parentKey, jiraMsg string) {
	t.Helper()
	inner := f.Config.Handler
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		inner.ServeHTTP(rec, r)
		path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
		isWrite := (r.Method == http.MethodPost && path == "/issue") ||
			(r.Method == http.MethodPut && strings.HasPrefix(path, "/issue/") && strings.Count(path, "/") == 2)
		if isWrite {
			tag := r.Method + " " + path
			if p, set, _ := parentFieldFromWriteBody(f.bodies[tag]); set && p == parentKey {
				msg, _ := json.Marshal(jiraMsg)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"errorMessages":[` + string(msg) + `]}`))
				return
			}
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

func parentFieldFromWriteBody(body string) (key string, set, clear bool) {
	var req struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if json.Unmarshal([]byte(body), &req) != nil || req.Fields == nil {
		return "", false, false
	}
	raw, ok := req.Fields["parent"]
	if !ok {
		return "", false, false
	}
	if string(raw) == "null" {
		return "", true, true
	}
	var p struct {
		Key string `json:"key"`
	}
	if json.Unmarshal(raw, &p) != nil || p.Key == "" {
		return "", false, false
	}
	return p.Key, true, false
}

func injectParentIntoSearch(body []byte, parents map[string]string) []byte {
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return body
	}
	issues, _ := doc["issues"].([]any)
	if len(issues) == 0 {
		return body
	}
	issue, _ := issues[0].(map[string]any)
	if issue == nil {
		return body
	}
	key, _ := issue["key"].(string)
	parent, ok := parents[key]
	if !ok {
		return body
	}
	fields, _ := issue["fields"].(map[string]any)
	if fields == nil {
		return body
	}
	fields["parent"] = map[string]string{"key": parent}
	out, err := json.Marshal(doc)
	if err != nil {
		return body
	}
	return out
}

// GDK-218: a configured default type id must let create omit --type.
// FAIL-first: on the pre-change resolveCreateType this errors "pass --type".
func TestCreateOmitsTypeUsesConfigDefault(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.DefaultIssueTypeID = "10001"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdCreate([]string{"from default type", "--project", "NMB", "--json"})
	})
	if err != nil {
		t.Fatalf("create with defaultIssueTypeId and no --type: %v\n%s", err, out)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"id":"10001"`) {
		t.Fatalf("default type id not sent: %s", sent)
	}
	var res struct {
		Created struct {
			Key string `json:"key"`
		} `json:"created"`
		Resolved struct {
			IssueType struct {
				Value  string `json:"value"`
				Source string `json:"source"`
			} `json:"issue_type"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if res.Created.Key != "NMB-42" {
		t.Fatalf("created %+v", res.Created)
	}
	if res.Resolved.IssueType.Value != "10001" || res.Resolved.IssueType.Source != "config" {
		t.Fatalf("resolved issue_type %+v", res.Resolved.IssueType)
	}
}

func TestCreateResolveTypeTable(t *testing.T) {
	many := []jira.NamedID{
		{ID: "10001", Name: "Task"},
		{ID: "10002", Name: "작업"},
		{ID: "10004", Name: "Bug"},
	}
	one := []jira.NamedID{{ID: "10001", Name: "Task"}}
	cfg := func(id string) *config.Config {
		return &config.Config{DefaultIssueTypeID: id}
	}
	cases := []struct {
		name, want, project string
		types               []jira.NamedID
		cfg                 *config.Config
		value, source, err  string
	}{
		{"flag name", "Task", "NMB", many, nil, "10001", create.SourceFlag, ""},
		{"flag id", "10004", "NMB", many, nil, "10004", create.SourceFlag, ""},
		{"flag korean", "작업", "NMB", many, nil, "10002", create.SourceFlag, ""},
		{"config", "", "NMB", many, cfg("10001"), "10001", create.SourceConfig, ""},
		{"sole", "", "GDK", one, nil, "10001", create.SourceSole, ""},
		{"omitted no evidence", "", "NMB", many, nil, "", "", "issue type required"},
		{"stale default", "", "NMB", many, cfg("99999"), "", "", "99999"},
		{"stale does not fall to sole", "", "GDK", one, cfg("99999"), "", "", "99999"},
		{"flag beats config", "Bug", "NMB", many, cfg("10001"), "10004", create.SourceFlag, ""},
		{"config name is not an id", "", "NMB", many, cfg("Task"), "", "", "Task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := create.Type(tc.want, tc.types, tc.cfg, tc.project)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Value != tc.value || got.Source != tc.source {
				t.Fatalf("got %+v, want value=%s source=%s", got, tc.value, tc.source)
			}
		})
	}
}

func TestCreateResolveProjectTable(t *testing.T) {
	cases := []struct {
		name, want    string
		cfg           *config.Config
		value, source string
		err           string
	}{
		{"flag", "NMB", &config.Config{Projects: []string{"NMA", "NMB"}, DefaultProject: "NMA"}, "NMB", create.SourceFlag, ""},
		{"config", "", &config.Config{Projects: []string{"NMA", "NMB"}, DefaultProject: "NMB"}, "NMB", create.SourceConfig, ""},
		{"sole", "", &config.Config{Projects: []string{"NMB"}}, "NMB", create.SourceSole, ""},
		{"ambiguous", "", &config.Config{Projects: []string{"NMA", "NMB"}}, "", "", "project required"},
		{"none", "", &config.Config{}, "", "", "project required"},
		{"flag beats config", "GDK", &config.Config{DefaultProject: "NMB", Projects: []string{"NMB", "GDK"}}, "GDK", create.SourceFlag, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := create.Project(tc.want, tc.cfg)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Value != tc.value || got.Source != tc.source {
				t.Fatalf("got %+v, want value=%s source=%s", got, tc.value, tc.source)
			}
		})
	}
}

func TestCreateUsesDefaultProjectWhenAmbiguous(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Projects = []string{"NMA", "NMB", "NMS"}
	cfg.DefaultProject = "NMB"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdCreate([]string{"from default project", "--type", "Task", "--json"})
	})
	if err != nil {
		t.Fatalf("create: %v\n%s", err, out)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"key":"NMB"`) {
		t.Fatalf("default project not sent: %s", sent)
	}
	if !strings.Contains(out, `"source":"config"`) || !strings.Contains(out, `"value":"NMB"`) {
		t.Fatalf("resolved project missing from json: %s", out)
	}
}

func TestCreateSoleTypeWhenProjectHasOne(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.Projects = []string{"NMB", "GDK"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdCreate([]string{"sole type", "--project", "GDK", "--json"})
	})
	if err != nil {
		t.Fatalf("create: %v\n%s", err, out)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"id":"10001"`) {
		t.Fatalf("sole type not sent: %s", sent)
	}
	if !strings.Contains(out, `"source":"sole"`) {
		t.Fatalf("resolved type source not sole: %s", out)
	}
}

func TestCreateStaleDefaultTypeErrors(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.DefaultIssueTypeID = "99999"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := capture(t, func() error {
		return cmdCreate([]string{"stale default", "--project", "NMB"})
	})
	if err == nil {
		t.Fatal("expected stale default error")
	}
	msg := err.Error()
	for _, want := range []string{"99999", "not available", "NMB", "Task (id 10001)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("stale default reached Jira: %v", f.calls)
	}
}

func TestCreateFlagTypeBeatsConfigDefault(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	cfg.DefaultIssueTypeID = "10001"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := capture(t, func() error {
		return cmdCreate([]string{"flag wins", "--project", "NMB", "--type", "Bug"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"id":"10004"`) {
		t.Fatalf("flag type not sent: %s", sent)
	}
}

func TestCreateHelpMentionsConfiguredDefaults(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdCreate([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("create --help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "configured default") {
		t.Fatalf("--help missing configured default wording:\n%s", out)
	}
}

func TestValidateDefaultIssueTypeIDRejectsNames(t *testing.T) {
	if _, err := config.ValidateDefaultIssueTypeID("Task"); err == nil {
		t.Fatal("display name accepted")
	}
	if _, err := config.ValidateDefaultIssueTypeID("작업"); err == nil {
		t.Fatal("localized name accepted")
	}
	got, err := config.ValidateDefaultIssueTypeID("10001")
	if err != nil || got != "10001" {
		t.Fatalf("id: %q %v", got, err)
	}
	got, err = config.ValidateDefaultIssueTypeID("  ")
	if err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
	if _, err := config.ValidateDefaultProject("NMB extra"); err == nil {
		t.Fatal("whitespace project accepted")
	}
}

// --field alias=value / batch "fields" (GDK-513):
// 31. required custom field from createmeta lands in POST /issue
//     TestCreateFieldLandsInPOST
// 32. --field absent → no customfield_* key
//     TestCreateWithoutFieldOmitsCustomfieldKey
// 33. batch line "fields" produces the same POST shape
//     TestCreateBatchFieldsKeySamePOSTShape
// 34. Help / usage enumerate --field
//     TestCreateHelpListsFieldFlag

const severityCreateFieldsJSON = `{"maxResults":50,"startAt":0,"total":1,"fields":[` +
	`{"fieldId":"customfield_10001","name":"Severity","required":true,"hasDefaultValue":false,` +
	`"schema":{"type":"option"},` +
	`"allowedValues":[{"id":"1","value":"High"},{"id":"2","value":"Low"}]}]}`

func serveCreateFields(t *testing.T, f *fakeJira, fieldsJSON string) {
	t.Helper()
	inner := f.Config.Handler
	f.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/api/3")
		if r.Method == http.MethodGet && strings.Contains(path, "/issue/createmeta/") && strings.Contains(path, "/issuetypes/") {
			tag := r.Method + " " + path
			f.calls = append(f.calls, tag)
			w.Header().Set("Content-Type", "application/json")
			body := fieldsJSON
			if body == "" {
				body = `{"maxResults":50,"startAt":0,"total":0,"fields":[]}`
			}
			_, _ = w.Write([]byte(body))
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func seedCreateSeverityAlias(t *testing.T, f *fakeJira) {
	t.Helper()
	serveCreateFields(t, f, severityCreateFieldsJSON)
	cfg := mirror(t, f.URL)
	cfg.Fields = []config.FieldSpec{
		{Alias: "severity", Label: "Severity", IDs: []string{"customfield_10001"}, Role: "facet", Kind: "option"},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func postIssueCustom(t *testing.T, f *fakeJira, id string) json.RawMessage {
	t.Helper()
	sent := f.bodies["POST /issue"]
	var req struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal([]byte(sent), &req); err != nil {
		t.Fatalf("POST JSON: %v (%s)", err, sent)
	}
	got, ok := req.Fields[id]
	if !ok {
		t.Fatalf("POST fields missing %s: %s", id, sent)
	}
	return got
}

func TestCreateFieldLandsInPOST(t *testing.T) {
	f := newFakeJira(t)
	seedCreateSeverityAlias(t, f)

	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"Needs severity", "--project", "NMB", "--type", "Task",
			"--field", "severity=High",
		})
	})
	if err != nil {
		t.Fatalf("create --field: %v", err)
	}
	if !f.called("POST /issue") {
		t.Fatalf("CreateIssue not called: %v", f.calls)
	}
	got := postIssueCustom(t, f, "customfield_10001")
	// createmeta carries allowedValues, so the name "High" maps to its id
	// before FieldValue wraps it — the same name→id rule the edit path uses.
	if string(got) != `{"id":"1"}` {
		t.Fatalf("customfield_10001 = %s, want {\"id\":\"1\"} (body %s)", got, f.bodies["POST /issue"])
	}
}

func TestCreateWithoutFieldOmitsCustomfieldKey(t *testing.T) {
	f := newFakeJira(t)
	seedCreateSeverityAlias(t, f)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"plain create", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if strings.Contains(sent, "customfield_") {
		t.Fatalf("--field absent must omit customfield keys: %s", sent)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "/createmeta/") && strings.Contains(c, "/issuetypes/") {
			t.Fatalf("no --field must not GET CreateFields: %v", f.calls)
		}
	}
}

func TestCreateBatchFieldsKeySamePOSTShape(t *testing.T) {
	f := newFakeJira(t)
	seedCreateSeverityAlias(t, f)
	withStdin(t, `{"summary":"batch severity","fields":{"severity":"High"}}`+"\n")

	_, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("batch fields: %v", err)
	}
	if !f.called("POST /issue") {
		t.Fatalf("CreateIssue not called: %v", f.calls)
	}
	got := postIssueCustom(t, f, "customfield_10001")
	if string(got) != `{"id":"1"}` {
		t.Fatalf("batch customfield_10001 = %s, want {\"id\":\"1\"} (body %s)", got, f.bodies["POST /issue"])
	}
}

func TestCreateHelpListsFieldFlag(t *testing.T) {
	out, err := capture(t, func() error {
		return cmdCreate([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("create --help: %v", err)
	}
	if !strings.Contains(out, "--field") {
		t.Fatalf("help Options missing --field:\n%s", out)
	}
	if !strings.Contains(helps["create"].usage, "--field") {
		t.Errorf("usage line missing --field: %s", helps["create"].usage)
	}
	if !strings.Contains(createUsage, "--field") {
		t.Errorf("createUsage missing --field: %s", createUsage)
	}
	joined := strings.Join(helps["create"].examples, "\n")
	if !strings.Contains(joined, "--field") {
		t.Errorf("examples missing --field:\n%s", joined)
	}
}
