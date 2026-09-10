package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

func TestJiraPriorityIndex(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		rank, srcN, dstN, want int
	}{
		{"unset stays unset", 0, 5, 5, -1},
		{"no target catalog", 3, 5, 0, -1},
		{"same size is position for position", 1, 5, 5, 0},
		{"same size, least urgent", 5, 5, 5, 4},
		{"wider source pins the top", 1, 5, 3, 0},
		{"wider source pins the bottom", 5, 5, 3, 2},
		{"wider source, middle is proportional", 3, 5, 3, 1},
		{"narrower source spreads", 2, 3, 5, 2},
		{"narrower source, bottom", 3, 3, 5, 4},
		{"single-entry source", 1, 1, 4, 0},
		{"rank past a shorter target clamps", 9, 0, 3, 2},
	} {
		if got := jiraPriorityIndex(tc.rank, tc.srcN, tc.dstN); got != tc.want {
			t.Errorf("%s: jiraPriorityIndex(%d,%d,%d) = %d, want %d",
				tc.name, tc.rank, tc.srcN, tc.dstN, got, tc.want)
		}
	}
}

func TestJiraSymmetricLinkFoldsAPairOnce(t *testing.T) {
	issues := []Issue{
		{Key: "A", Links: []Link{{Type: "Blocks", Outward: "B"}, {Type: "Relates", Outward: "C"}}},
		{Key: "B", Links: []Link{{Type: "blocks", Inward: "A"}}},
		{Key: "C", Links: []Link{{Type: "Relates", Inward: "A"}}},
	}
	got := foldRelations(issues, jiraLinkName, jiraSymmetricLink)
	want := map[string]bool{"A blocks B": true, "A relates C": true}
	if len(got) != len(want) {
		t.Fatalf("relations %+v, want %d", got, len(want))
	}
	for _, r := range got {
		if !want[r.from+" "+r.typ+" "+r.to] {
			t.Errorf("unexpected relation %+v", r)
		}
	}
}

// The footer has to survive the round trip the idempotency scan actually
// makes: written into ADF, read back as the plain text Jira's search returns.
func TestJiraDescriptionFooterSurvivesPlainText(t *testing.T) {
	raw := jiraDescription(Issue{Key: "STD-7", Description: "line one\n\nline two"})
	if got := parseMigrateFooter(adf.PlainText(raw)); got != "STD-7" {
		t.Fatalf("footer round trip = %q, want STD-7 (plain text was %q)", got, adf.PlainText(raw))
	}
	if !strings.Contains(adf.PlainText(raw), "line two") {
		t.Fatalf("the body must survive beside the footer: %q", adf.PlainText(raw))
	}
	// An origin ADF body is carried verbatim, not re-wrapped from its text.
	rich := jiraDescription(Issue{Key: "STD-8",
		Description:    "flattened",
		DescriptionADF: `{"type":"doc","version":1,"content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Steps"}]}]}`})
	if !strings.Contains(string(rich), `"heading"`) {
		t.Fatalf("origin ADF must ride as written: %s", rich)
	}
	if strings.Contains(string(rich), "flattened") {
		t.Fatalf("the flattened text must not be added beside the ADF: %s", rich)
	}
	if got := parseMigrateFooter(adf.PlainText(rich)); got != "STD-8" {
		t.Fatalf("footer on a rich body = %q", got)
	}
}

// --dry-run must not open a single connection: the client points at a
// server that fails the test on any request (the Linear sibling's gate).
func TestToJiraDryRunIsOffline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("dry-run made a request: %s", r.URL.Path)
		w.WriteHeader(500)
	}))
	defer srv.Close()
	c := jira.New(srv.URL, "a@example.com", "not-a-token")

	sqlDB, err := store.OpenReadOnly(seedMirror(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	doc, st, err := Build(context.Background(), sqlDB, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ToJira(context.Background(), c, doc, st, JiraOptions{ProjectKey: "ENG", DryRun: true, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun || rep.Project != "ENG" {
		t.Fatalf("report %+v", rep)
	}
	byMetric := map[string]VerifyRow{}
	for _, r := range rep.Counts {
		byMetric[r.Metric] = r
	}
	if byMetric["issues"].Source != 2 || byMetric["comments"].Source != 1 || byMetric["links"].Source != 1 {
		t.Fatalf("counts %+v", rep.Counts)
	}
	if !strings.Contains(strings.Join(rep.NotMigrated, "\n"), "history") {
		t.Fatalf("history must be reported as not migrated: %q", rep.NotMigrated)
	}
}

func TestToJiraRefusesWithoutProject(t *testing.T) {
	_, err := ToJira(context.Background(), nil, &Doc{}, nil, JiraOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "--project") {
		t.Fatalf("err = %v, want a refusal naming --project", err)
	}
}

// fakeJira is the httptest stand-in for a Jira site: canned catalogs, a
// recorded POST log, and a mutable set of issues the idempotency scan reads
// back. Statuses are deliberately named in a language the mapping must not
// key on.
type fakeJira struct {
	mu sync.Mutex
	// created maps target key → the fields body the run POSTed.
	created map[string]map[string]any
	order   []string
	// comments, links, transitions and uploads are the recorded side calls.
	comments    map[string][]json.RawMessage
	links       []map[string]any
	transitions map[string]string
	uploads     map[string][]string
	// preexisting seeds the scan: target key → description ADF.
	preexisting map[string]string
	// linkTypes is the site catalog; a test can shorten it.
	linkTypes string
	// priorities is the site catalog, most urgent first.
	priorities string
	next       int
}

func newFakeJira() *fakeJira {
	return &fakeJira{
		created: map[string]map[string]any{}, comments: map[string][]json.RawMessage{},
		transitions: map[string]string{}, uploads: map[string][]string{},
		preexisting: map[string]string{},
		linkTypes:   `{"id":"lt-1","name":"Blocks"},{"id":"lt-2","name":"Relates"}`,
		priorities:  `{"id":"p1","name":"Highest"},{"id":"p2","name":"High"},{"id":"p3","name":"Medium"},{"id":"p4","name":"Low"},{"id":"p5","name":"Lowest"}`,
	}
}

func (f *fakeJira) start(t *testing.T) *jira.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.serve(t)))
	t.Cleanup(srv.Close)
	c := jira.New(srv.URL, "a@example.com", "not-a-token")
	c.Retries, c.Backoff = 1, 0
	return c
}

func (f *fakeJira) serve(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && strings.HasSuffix(p, "/issue/createmeta"):
			// Sub-task first on purpose: the default type must skip it.
			fmt.Fprint(w, `{"projects":[{"key":"ENG","name":"Eng","issuetypes":[`+
				`{"id":"t-sub","name":"Sub-task","subtask":true},`+
				`{"id":"t-task","name":"Task"},{"id":"t-bug","name":"Bug"}]}]}`)
		case r.Method == "GET" && strings.HasSuffix(p, "/priority"):
			fmt.Fprint(w, "["+f.priorities+"]")
		case r.Method == "GET" && strings.HasSuffix(p, "/issueLinkType"):
			fmt.Fprint(w, `{"issueLinkTypes":[`+f.linkTypes+`]}`)
		case r.Method == "POST" && strings.HasSuffix(p, "/search/jql"):
			var q struct {
				JQL string `json:"jql"`
			}
			_ = json.Unmarshal(body, &q)
			if !strings.Contains(q.JQL, migrateLabel) {
				t.Errorf("the idempotency scan must ask for the footer marker: %q", q.JQL)
			}
			rows := []string{}
			for key, desc := range f.preexisting {
				rows = append(rows, fmt.Sprintf(`{"key":%q,"fields":{"description":%s}}`, key, desc))
			}
			fmt.Fprintf(w, `{"issues":[%s],"isLast":true}`, strings.Join(rows, ","))
		case r.Method == "POST" && strings.HasSuffix(p, "/issue"):
			var in struct {
				Fields map[string]any `json:"fields"`
			}
			if err := json.Unmarshal(body, &in); err != nil {
				t.Errorf("create body: %v", err)
			}
			f.next++
			key := fmt.Sprintf("ENG-%d", f.next)
			f.created[key] = in.Fields
			f.order = append(f.order, key)
			fmt.Fprintf(w, `{"id":"%d","key":%q}`, f.next, key)
		case r.Method == "GET" && strings.Contains(p, "/transitions"):
			// Names are Korean and match nothing; only the category can
			// carry the mapping.
			fmt.Fprint(w, `{"transitions":[`+
				`{"id":"21","name":"진행 중으로","to":{"id":"3","name":"진행 중","statusCategory":{"key":"indeterminate"}}},`+
				`{"id":"31","name":"완료로","to":{"id":"10001","name":"완료","statusCategory":{"key":"done"}}}]}`)
		case r.Method == "POST" && strings.Contains(p, "/transitions"):
			var in struct {
				Transition struct {
					ID string `json:"id"`
				} `json:"transition"`
			}
			_ = json.Unmarshal(body, &in)
			f.transitions[issueKeyOf(p, "/transitions")] = in.Transition.ID
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "POST" && strings.HasSuffix(p, "/comment"):
			var in struct {
				Body json.RawMessage `json:"body"`
			}
			_ = json.Unmarshal(body, &in)
			key := issueKeyOf(p, "/comment")
			f.comments[key] = append(f.comments[key], in.Body)
			fmt.Fprint(w, `{"id":"c1"}`)
		case r.Method == "POST" && strings.HasSuffix(p, "/issueLink"):
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			f.links = append(f.links, in)
			w.WriteHeader(http.StatusCreated)
		case r.Method == "POST" && strings.HasSuffix(p, "/attachments"):
			key := issueKeyOf(p, "/attachments")
			f.uploads[key] = append(f.uploads[key], "1")
			fmt.Fprint(w, `[{"id":"att-1","filename":"notes.txt"}]`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, p)
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func issueKeyOf(path, suffix string) string {
	p := strings.TrimSuffix(path, suffix)
	i := strings.LastIndex(p, "/")
	return p[i+1:]
}

// fixtureDoc is the migration input the write-path tests share: two issues
// (one a child of the other), one comment, one link, one label.
func fixtureDoc() *Doc {
	return &Doc{
		Users:      []User{{AccountID: "acc-a", DisplayName: "Ada"}},
		Priorities: []Priority{{ID: "1", Name: "Highest"}, {ID: "2", Name: "High"}, {ID: "3", Name: "Medium"}, {ID: "4", Name: "Low"}, {ID: "5", Name: "Lowest"}},
		IssueTypes: []IssueType{{ID: "10001", Name: "Task"}, {ID: "10002", Name: "Epic"}},
		Issues: []Issue{
			{Key: "STD-1", Summary: "the parent", Description: "parent body",
				Type: "10002", StatusCategory: "done", PriorityRank: 2, Labels: []string{"public"}},
			{Key: "STD-2", Summary: "the child", Description: "child body",
				Type: "10001", StatusCategory: "inprogress", PriorityRank: 1, Parent: "STD-1",
				Comments: []Comment{{Author: "acc-a", Body: "first", Created: "2026-01-01T01:00:00.000Z"}},
				Links:    []Link{{Type: "Blocks", Outward: "STD-1"}}},
		},
	}
}

// The write path's shape gate: two issues with the right fields, one
// comment carrying its author, one link, one parent — and the type/priority
// mapping resolved through the target's own catalogs.
func TestToJiraCreatesIssuesCommentsLinksAndParent(t *testing.T) {
	f := newFakeJira()
	c := f.start(t)
	rep, err := ToJira(context.Background(), c, fixtureDoc(), nil, JiraOptions{ProjectKey: "ENG"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 2 {
		t.Fatalf("created %d issues, want 2: %+v", len(f.created), f.created)
	}
	// Parent-first: STD-1 has no parent, so it is created first and its key
	// is what STD-2's parent field carries.
	parentKey, childKey := f.order[0], f.order[1]
	parent, child := f.created[parentKey], f.created[childKey]
	if parent["summary"] != "the parent" || child["summary"] != "the child" {
		t.Fatalf("order wrong: %v / %v", parent["summary"], child["summary"])
	}
	if got := child["parent"].(map[string]any)["key"]; got != parentKey {
		t.Errorf("child parent = %v, want %s", got, parentKey)
	}
	// The source type name has no counterpart ("Epic" is absent from the
	// project) so the parent falls back to the first non-subtask type; the
	// child's "Task" matches by name.
	if got := parent["issuetype"].(map[string]any)["id"]; got != "t-task" {
		t.Errorf("parent issuetype = %v, want the default t-task (never t-sub)", got)
	}
	if got := child["issuetype"].(map[string]any)["id"]; got != "t-task" {
		t.Errorf("child issuetype = %v, want t-task by name", got)
	}
	// priority_rank 1 → the target catalog's first entry; 2 → its second.
	if got := child["priority"].(map[string]any)["id"]; got != "p1" {
		t.Errorf("child priority = %v, want p1 (rank 1)", got)
	}
	if got := parent["priority"].(map[string]any)["id"]; got != "p2" {
		t.Errorf("parent priority = %v, want p2 (rank 2)", got)
	}
	if labels, _ := parent["labels"].([]any); len(labels) != 1 || labels[0] != "public" {
		t.Errorf("parent labels = %v, want [public]", parent["labels"])
	}
	// One comment, with the source author and time as its first line.
	if len(f.comments[childKey]) != 1 {
		t.Fatalf("comments on %s = %d, want 1", childKey, len(f.comments[childKey]))
	}
	text := adf.PlainText(f.comments[childKey][0])
	if !strings.HasPrefix(text, "Ada · 2026-01-01T01:00:00.000Z") || !strings.Contains(text, "first") {
		t.Errorf("comment body = %q, want the author·time header then the body", text)
	}
	// One link, of the site's Blocks type, in the source's direction.
	if len(f.links) != 1 {
		t.Fatalf("links = %+v, want 1", f.links)
	}
	l := f.links[0]
	if l["type"].(map[string]any)["id"] != "lt-1" ||
		l["outwardIssue"].(map[string]any)["key"] != childKey ||
		l["inwardIssue"].(map[string]any)["key"] != parentKey {
		t.Errorf("link = %+v, want lt-1 %s → %s", l, childKey, parentKey)
	}
	byMetric := map[string]VerifyRow{}
	for _, r := range rep.Counts {
		byMetric[r.Metric] = r
	}
	for _, m := range []string{"issues", "comments", "parents", "links"} {
		if r := byMetric[m]; r.Source != r.Migrated || r.Skipped != 0 {
			t.Errorf("%s row %+v, want source == migrated and nothing skipped", m, r)
		}
	}
}

// The status mapping must key on status_category alone. The fake's
// transition names are Korean and its status names are too, so a name-based
// implementation has nothing to match — and the categories are the only
// thing that can pick 21 for inprogress and 31 for done.
func TestToJiraTransitionsByCategoryNotName(t *testing.T) {
	f := newFakeJira()
	c := f.start(t)
	if _, err := ToJira(context.Background(), c, fixtureDoc(), nil, JiraOptions{ProjectKey: "ENG"}); err != nil {
		t.Fatal(err)
	}
	parentKey, childKey := f.order[0], f.order[1]
	if got := f.transitions[parentKey]; got != "31" {
		t.Errorf("done issue took transition %q, want 31 (the done-category one)", got)
	}
	if got := f.transitions[childKey]; got != "21" {
		t.Errorf("inprogress issue took transition %q, want 21 (the indeterminate-category one)", got)
	}
	if len(f.transitions) != 2 {
		t.Errorf("transitions %+v — a `new` issue must not be transitioned at all", f.transitions)
	}
}

// Re-running over a project that already holds the run's output writes
// nothing: the footer scan matches every issue and the report counts them
// as already there.
func TestToJiraReRunSkipsEverything(t *testing.T) {
	f := newFakeJira()
	c := f.start(t)
	if _, err := ToJira(context.Background(), c, fixtureDoc(), nil, JiraOptions{ProjectKey: "ENG"}); err != nil {
		t.Fatal(err)
	}
	// Feed the first run's descriptions back as the site's existing rows.
	for key, fields := range f.created {
		desc, err := json.Marshal(fields["description"])
		if err != nil {
			t.Fatal(err)
		}
		f.preexisting[key] = string(desc)
	}
	before := len(f.created)
	f.comments = map[string][]json.RawMessage{}
	f.links = nil
	f.transitions = map[string]string{}

	rep, err := ToJira(context.Background(), c, fixtureDoc(), nil, JiraOptions{ProjectKey: "ENG"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.created) != before {
		t.Fatalf("the re-run created %d more issues", len(f.created)-before)
	}
	if len(f.comments) != 0 || len(f.links) != 0 || len(f.transitions) != 0 {
		t.Fatalf("the re-run wrote children: comments %v links %v transitions %v", f.comments, f.links, f.transitions)
	}
	byMetric := map[string]VerifyRow{}
	for _, r := range rep.Counts {
		byMetric[r.Metric] = r
	}
	for _, m := range []string{"issues", "comments", "parents", "links"} {
		r := byMetric[m]
		if r.Skipped != r.Source || r.Migrated != r.Source {
			t.Errorf("%s row %+v, want everything counted as already there", m, r)
		}
	}
}

// A link type the target site does not have falls back to Relates, once,
// with a warning that names the type.
func TestToJiraUnknownLinkTypeFallsBackToRelates(t *testing.T) {
	f := newFakeJira()
	f.linkTypes = `{"id":"lt-2","name":"Relates"}`
	c := f.start(t)
	rep, err := ToJira(context.Background(), c, fixtureDoc(), nil, JiraOptions{ProjectKey: "ENG"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.links) != 1 || f.links[0]["type"].(map[string]any)["id"] != "lt-2" {
		t.Fatalf("links = %+v, want one Relates", f.links)
	}
	if !strings.Contains(strings.Join(rep.Warnings, "\n"), `"blocks"`) {
		t.Fatalf("the fallback must name the type: %q", rep.Warnings)
	}
}

// Attachment bytes ride the export (InlineAttachments fills DataBase64) and
// are uploaded per issue; --skip-attachments uploads none and says so.
func TestToJiraUploadsAttachmentBytesUnlessSkipped(t *testing.T) {
	doc := func() *Doc {
		d := fixtureDoc()
		d.Issues[1].Attachments = []Attachment{{Filename: "notes.txt",
			MimeType: "text/plain", DataBase64: "aGVsbG8="}}
		return d
	}
	f := newFakeJira()
	c := f.start(t)
	if _, err := ToJira(context.Background(), c, doc(), nil, JiraOptions{ProjectKey: "ENG"}); err != nil {
		t.Fatal(err)
	}
	if n := len(f.uploads[f.order[1]]); n != 1 {
		t.Fatalf("uploads on the child = %d, want 1 (%+v)", n, f.uploads)
	}

	f2 := newFakeJira()
	c2 := f2.start(t)
	rep, err := ToJira(context.Background(), c2, doc(), nil, JiraOptions{ProjectKey: "ENG", SkipAttachments: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(f2.uploads) != 0 {
		t.Fatalf("--skip-attachments uploaded %+v", f2.uploads)
	}
	if !strings.Contains(strings.Join(rep.NotMigrated, "\n"), "--skip-attachments") {
		t.Fatalf("the skip must be reported: %q", rep.NotMigrated)
	}
}
