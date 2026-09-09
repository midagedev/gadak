package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

// The write-gap round (2026-09-10): each test pins one refusal or output
// shape a dogfooding agent hit on a paired workspace, so the friction cannot
// return once closed.
//
//  GDK-1733  create with no configured projects and one createable project
//            refuses instead of using it
//            TestCreateSoleCatalogProjectDefault
//  GDK-1620  --parent names the project; create still demands --project
//            TestCreateParentNamesTheProject
//  GDK-1593  (1) the ambiguous-project refusal names the keys (already true —
//            pinned); (2) the --type refusal names the config escape;
//            (3) an empty key sends PUT /issue/ instead of a usage error
//            TestCreateAmbiguousCatalogListsEveryKey
//            TestCreateNeedsTypeNamesConfigDefault
//            TestWriteWithEmptyKeyIsUsageErrorBeforeOrigin
//  GDK-1716  create --json has no top-level key; scripts' ["key"] misses
//            TestCreateJSONCarriesTopLevelKey
//  GDK-836   the echo said the project default type, not the one requested
//            (verify-and-close 2026-09-10: the text echo no longer prints a
//            type; the JSON row is the refreshed mirror row)
//            TestCreateJSONEchoCarriesRequestedType
//  GDK-1487  create --batch prints no header row
//            TestCreateBatchPrintsHeader
//  GDK-1458  a type name matching two catalog ids refuses even when the
//            mirror is unambiguous about which id it uses
//            TestCreateAmbiguousTypeSettlesByMirrorUse

// soleCatalogMeta is a createmeta payload with exactly one createable
// project — the paired-workspace shape behind GDK-1733.
const soleCatalogMeta = `{"projects":[
	{"key":"STD","name":"Standard","issuetypes":[
		{"id":"10001","name":"Task"},
		{"id":"10004","name":"Bug"}]}
]}`

// paired drops the local project scope so create.Project has nothing — the
// GDK-467/GDK-1733 gap is exactly "empty cfg.Projects + a CreateMeta catalog".
func paired(t *testing.T, f *fakeJira) {
	t.Helper()
	cfg := mirror(t, f.URL)
	cfg.Projects = nil
	cfg.DefaultProject = ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSoleCatalogProjectDefault(t *testing.T) {
	f := newFakeJira(t)
	f.createMetaJSON = soleCatalogMeta
	paired(t, f)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"paired workspace create", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"key":"STD"`) {
		t.Fatalf("sole catalog project not used as the default: %s", sent)
	}
}

func TestCreateParentNamesTheProject(t *testing.T) {
	f := newFakeJira(t)
	paired(t, f) // default catalog: NMB + GDK, so the sole path must not fire

	_, err := capture(t, func() error {
		return cmdCreate([]string{"child of NMB-1", "--type", "Task", "--parent", "NMB-1"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sent := f.bodies["POST /issue"]
	if !strings.Contains(sent, `"project":{"key":"NMB"}`) {
		t.Fatalf("parent key did not name the project: %s", sent)
	}
	if !strings.Contains(sent, `"parent":{"key":"NMB-1"}`) {
		t.Fatalf("parent not sent: %s", sent)
	}
}

func TestCreateAmbiguousCatalogListsEveryKey(t *testing.T) {
	f := newFakeJira(t)
	paired(t, f)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"needs a project", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("expected --project error")
	}
	// The refusal must name the projects that exist — both of them, not the
	// one it already knows (GDK-1593 pt 1). This half was already true via
	// FillNeedProject; the test pins it beside the sole-catalog default so
	// the two cannot drift apart.
	for _, want := range []string{"pass --project", "NMB", "GDK"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("ambiguous project reached Jira: %v", f.calls)
	}
}

func TestCreateNeedsTypeNamesConfigDefault(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error {
		return cmdCreate([]string{"needs a type", "--project", "NMB"})
	})
	if err == nil {
		t.Fatal("expected --type error")
	}
	// GDK-1593 pt 2: a paired workspace has no recorded type default, so the
	// refusal must name the way out of paying it on every create.
	for _, want := range []string{"pass --type", "gadak config set defaultIssueTypeId"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("omitted type reached Jira: %v", f.calls)
	}
}

// noWriteTag asserts the fake saw no write method at all — the empty-key
// check must fire before any request leaves the process (GDK-1593 pt 3).
func noWriteTag(t *testing.T, f *fakeJira) {
	t.Helper()
	for _, c := range f.calls {
		for _, m := range []string{"POST ", "PUT ", "DELETE ", "PATCH "} {
			if strings.HasPrefix(c, m) {
				t.Fatalf("write reached the origin: %s", c)
			}
		}
	}
}

func TestWriteWithEmptyKeyIsUsageErrorBeforeOrigin(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	_, err := capture(t, func() error { return cmdEdit([]string{"", "--label", "+gap"}) })
	if err == nil || !strings.Contains(err.Error(), "empty issue key") {
		t.Fatalf("edit: %v", err)
	}
	noWriteTag(t, f)

	_, err = capture(t, func() error { return cmdComment([]string{"", "-m", "hi"}) })
	if err == nil || !strings.Contains(err.Error(), "empty issue key") {
		t.Fatalf("comment: %v", err)
	}
	noWriteTag(t, f)

	_, err = capture(t, func() error { return cmdTransition([]string{"", "done"}) })
	if err == nil || !strings.Contains(err.Error(), "empty issue key") {
		t.Fatalf("transition: %v", err)
	}
	noWriteTag(t, f)
}

func TestCreateJSONCarriesTopLevelKey(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdCreate([]string{"json shape probe", "--project", "NMB", "--type", "Task", "--json"})
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("create --json: %v\n%s", err, out)
	}
	// GDK-1716: the first extractor a script writes (["key"]) must find the
	// new key; created.key stays for the readers that already use it.
	if key, _ := body["key"].(string); key != "NMB-42" {
		t.Fatalf("top-level key = %q, want NMB-42", key)
	}
	created, _ := body["created"].(map[string]any)
	if created["key"] != "NMB-42" {
		t.Fatalf("created.key missing: %v", body)
	}
}

// GDK-836: a create with an explicit non-default type echoed the project
// default (Task) instead of the stored one — an agent re-checks the write
// when the confirmation disagrees with the request. The text echo no longer
// prints a type at all (summaryLine: key/status/assignee/summary); the JSON
// row is the refreshed mirror row, and this pins that it names the type the
// request carried.
func TestCreateJSONEchoCarriesRequestedType(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)

	out, err := capture(t, func() error {
		return cmdCreate([]string{"bug echo probe", "--project", "NMB", "--type", "Bug", "--json"})
	})
	if err != nil {
		t.Fatalf("create: %v\n%s", err, out)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("create --json: %v\n%s", err, out)
	}
	issue, _ := body["issue"].(map[string]any)
	if issue["issue_type"] != "Bug" || issue["issue_type_id"] != "10004" {
		t.Fatalf("echo type = %v/%v, want Bug/10004", issue["issue_type"], issue["issue_type_id"])
	}
	resolved, _ := body["resolved"].(map[string]any)
	typ, _ := resolved["issue_type"].(map[string]any)
	if typ["value"] != "10004" || typ["source"] != "flag" {
		t.Fatalf("resolved.issue_type = %v, want 10004 from flag", typ)
	}
}

func TestCreateBatchPrintsHeader(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t,
		`{"summary":"first"}`+"\n"+
			`{"summary":"second"}`+"\n")

	out, err := capture(t, func() error {
		return cmdCreate([]string{"--batch", "-", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("create --batch: %v\n%s", err, out)
	}
	// GDK-1487: comment/transition/edit batch lines all open with a header;
	// create's column set differs (it is the created row, not an envelope)
	// but the header's presence must not.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("want header + 2 rows, got %d lines:\n%s", len(lines), out)
	}
	if lines[0] != "key\tsummary" {
		t.Fatalf("header = %q, want %q", lines[0], "key\tsummary")
	}
}

// seedMirrorType adds one issue row so a type name's mirror usage is
// unambiguous (GDK-1458's settle input).
func seedMirrorType(t *testing.T, key, project, name, id string) {
	t.Helper()
	home := os.Getenv("GADAK_HOME")
	if home == "" {
		t.Fatal("GADAK_HOME not set — call mirror() first")
	}
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.UpsertIssues(context.Background(), store.Batch{
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:" + id, SourceID: "jira", Kind: "issue", ExternalID: id, Key: key,
				Title: name + " row", CreatedAt: "2026-07-01T00:00:00.000Z", UpdatedAt: "2026-07-01T00:00:00.000Z",
			},
			Issue: store.Issue{ProjectKey: project, IssueType: name, IssueTypeID: id, StatusCategory: "new"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// duplicateTypeMeta carries two catalog ids under one display name — the
// GDK-1458 shape (an Epic the site admin duplicated).
const duplicateTypeMeta = `{"projects":[
	{"key":"NMB","name":"Numbers","issuetypes":[
		{"id":"10000","name":"Epic"},
		{"id":"10005","name":"Epic"},
		{"id":"10001","name":"Task"}]}
]}`

func TestCreateAmbiguousTypeSettlesByMirrorUse(t *testing.T) {
	f := newFakeJira(t)
	f.createMetaJSON = duplicateTypeMeta
	mirror(t, f.URL)
	seedMirrorType(t, "NMB-9", "NMB", "Epic", "10005")

	out, err := capture(t, func() error {
		return cmdCreate([]string{"ambiguous epic", "--project", "NMB", "--type", "Epic", "--json"})
	})
	if err != nil {
		t.Fatalf("create: %v\n%s", err, out)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"issuetype":{"id":"10005"}`) {
		t.Fatalf("mirror-settled id not sent: %s", sent)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("create --json: %v\n%s", err, out)
	}
	resolved, _ := body["resolved"].(map[string]any)
	typ, _ := resolved["issue_type"].(map[string]any)
	if typ["source"] != "mirror" {
		t.Fatalf("resolved.issue_type.source = %v, want mirror", typ["source"])
	}

	// When the mirror itself uses both ids the name stays ambiguous — the
	// settle must not invent a winner.
	seedMirrorType(t, "NMB-8", "NMB", "Epic", "10000")
	_, err = capture(t, func() error {
		return cmdCreate([]string{"still ambiguous", "--project", "NMB", "--type", "Epic"})
	})
	if err == nil || !strings.Contains(err.Error(), "more than one catalog type") {
		t.Fatalf("both ids in use must stay refused: %v", err)
	}
}
