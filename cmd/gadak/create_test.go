package main

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// Contract ↔ assertion (clause numbers match the task spec):
//
//  1. Summary: positional words joined; empty / whitespace → usage
//     TestCreateJoinsSummaryWords, TestCreateRejectsEmptySummary
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
//     TestCreateRefreshFailureKeepsWriteAppliedWording,
//     TestCreateOutsideMirrorPrintsKeyAndExitsZero,
//     TestCreateIntoUnconfiguredProjectStillPrintsRow
//  8. No credential → mutate's init error
//     TestWritesRefuseToRunWithoutACredential (agent_test.go),
//     TestCreateRefusesWithoutCredential
//  9. Dispatch + help (usage, --type 작업 example, Writing-through line)
//     TestCreateIsRegisteredAndHelpMentionsNonEnglishType

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
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"cannot file here", "--project", "ZZZ", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "cannot create issues in ZZZ") {
		t.Fatalf("missing createmeta project: %v", err)
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

func TestCreateRefreshFailureKeepsWriteAppliedWording(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	f.rereadStatus = 422

	_, err := capture(t, func() error {
		return cmdCreate([]string{"refresh will fail", "--project", "NMB", "--type", "Task"})
	})
	if err == nil || !strings.Contains(err.Error(), "write applied to NMB-42, but the mirror did not refresh") {
		t.Fatalf("refresh fail: %v", err)
	}
	if !f.called("POST /issue") {
		t.Fatal("write must have reached Jira before the refresh error")
	}
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
	// Spec correction: SyncIssue fetches by exact key, so a create in GDK
	// (not in cfg.Projects) still lands in the mirror after refresh.
	f := newFakeJira(t)
	mirror(t, f.URL) // Projects: [NMB]

	out, err := capture(t, func() error {
		return cmdCreate([]string{"filed in GDK", "--project", "GDK", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create into GDK: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "GDK-42\t") {
		t.Fatalf("expected re-read row for GDK-42, got %q", out)
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
	if !strings.Contains(h.usage, "gadak [--profile <name>] create") {
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

func TestCreateSummaryStartingWithDashIsPositional(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"-this", "broke", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("leading-dash summary: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"summary":"-this broke"`) {
		t.Fatalf("leading dash eaten: %s", sent)
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
