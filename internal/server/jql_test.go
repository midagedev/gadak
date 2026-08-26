package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/uifocus"
)

func TestJqlParseAndEmit(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"jql/",
		`{"input":"project = NMB AND statusCategory = \"In Progress\"","email":"hc@example.com"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[jql.Result](t, rec)
	if got.Error != "" {
		t.Fatalf("error %s: %s", got.Error, got.Message)
	}
	if len(got.Filters.JiraProject) != 1 || got.Filters.JiraProject[0] != "NMB" {
		t.Fatalf("project %+v", got.Filters.JiraProject)
	}
	if len(got.Filters.StatusCategory) != 1 || got.Filters.StatusCategory[0] != "inprogress" {
		t.Fatalf("category %+v", got.Filters.StatusCategory)
	}
	if !strings.Contains(got.JQL, "project = NMB") {
		t.Fatalf("canonical %q", got.JQL)
	}

	rec = get(t, h, apiBase+"jql/?q=project%20%3D%20NMA", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %d: %s", rec.Code, rec.Body.String())
	}
	got = decode[jql.Result](t, rec)
	if len(got.Filters.JiraProject) != 1 || got.Filters.JiraProject[0] != "NMA" {
		t.Fatalf("GET project %+v", got.Filters.JiraProject)
	}

	rec = send(t, h, http.MethodPost, apiBase+"jql/emit/",
		`{"filters":{"jira_project":["NMB"],"status_category":["inprogress"],"status":[],"assignee_email":[],"reporter_email":[],"team_group":[],"labels":[],"priority":[],"severity":[],"issue_type":[],"components":[],"fix_versions":[],"qa_run":[],"qa_suite":[],"qa_impact":[],"deploy_state":[],"source_project":[],"fields":{},"reopened":true},"display":{"sort":"updated","dir":"desc"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("emit %d: %s", rec.Code, rec.Body.String())
	}
	var emitted struct {
		JQL     string   `json:"jql"`
		Omitted []string `json:"omitted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &emitted); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(emitted.JQL, "project = NMB") ||
		!strings.Contains(emitted.JQL, `statusCategory = "In Progress"`) {
		t.Fatalf("emit jql %q", emitted.JQL)
	}
	if len(emitted.Omitted) != 1 || emitted.Omitted[0] != "reopened" {
		t.Fatalf("omitted %v", emitted.Omitted)
	}
}

func TestJqlReporterNameResolvesToAccountID(t *testing.T) {
	db, cfg := fixture(t)
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"jql/",
		`{"input":"reporter = \"박보고\""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[jql.Result](t, rec)
	if len(got.Filters.ReporterEmail) != 1 || got.Filters.ReporterEmail[0] != "acc-rp" {
		t.Fatalf("reporter_email %+v (want acc-rp)", got.Filters.ReporterEmail)
	}
}

func TestJqlCurrentUserResolvesToAccountID(t *testing.T) {
	db, cfg := fixture(t)
	cfg.AccountID = "acc-hc"
	h := New(db, cfg)

	rec := send(t, h, http.MethodPost, apiBase+"jql/",
		`{"input":"assignee = currentUser()"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[jql.Result](t, rec)
	if got.Error != "" {
		t.Fatalf("error %s: %s", got.Error, got.Message)
	}
	if len(got.Filters.AssigneeEmail) != 1 || got.Filters.AssigneeEmail[0] != "acc-hc" {
		t.Fatalf("assignee_email %+v (want acc-hc)", got.Filters.AssigneeEmail)
	}
}

func TestJqlDoesNotFullScanIssueLites(t *testing.T) {
	// GDK-756: /jql/ used to load every IssueLite to resolve assignee/reporter
	// names. QueryActorPeople is the six-column projection CLI adopted in GDK-748.
	src, err := os.ReadFile("jql.go")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(src, []byte(".IssueLites(")) {
		t.Fatal("jql.go still calls IssueLites (full mirror scan); use QueryActorPeople")
	}
	if !bytes.Contains(src, []byte(".QueryActorPeople(")) {
		t.Fatal("jql.go never calls QueryActorPeople")
	}
}

func TestUIFocusTakeIsOneShot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	if err := uifocus.Write("pj=NMA&sc=inprogress"); err != nil {
		t.Fatal(err)
	}
	h := New(nil, nil)
	got := decode[struct {
		Hash          string `json:"hash"`
		At            string `json:"at"`
		ConfigVersion string `json:"configVersion"`
	}](t, get(t, h, apiBase+"ui-focus/", nil))
	if got.Hash != "pj=NMA&sc=inprogress" {
		t.Fatalf("hash %q", got.Hash)
	}
	if got.At == "" {
		t.Fatal("first poll must carry at")
	}
	rec := get(t, h, apiBase+"ui-focus/", nil)
	// GDK-791 (2026-08-24): the idle poll is 200 + configVersion, not 204 —
	// the body carries the settings-change signal.
	// GDK-960: the file is no longer consumed on read. Both polls return
	// the same hash+at; one-shot apply is the client's job.
	if rec.Code != http.StatusOK {
		t.Fatalf("second poll %d", rec.Code)
	}
	idle := decode[struct {
		Hash          string `json:"hash"`
		At            string `json:"at"`
		ConfigVersion string `json:"configVersion"`
	}](t, rec)
	if idle.Hash != got.Hash {
		t.Fatalf("second poll hash %q, want %q", idle.Hash, got.Hash)
	}
	if idle.At != got.At {
		t.Fatalf("second poll at %q, want %q", idle.At, got.At)
	}
	if idle.ConfigVersion == "" {
		t.Fatal("poll must carry configVersion")
	}
}

func TestViewsIncludeSourceQueries(t *testing.T) {
	db, cfg := fixture(t)
	if err := db.ReplaceSourceQueries(context.Background(), "jira", []store.SourceQuery{
		{ID: "jira:1", SourceID: "jira", ExternalID: "1", Name: "Open bugs",
			QueryText: `type = Bug`, Config: json.RawMessage(`{"filters":{}}`), Favourite: true},
	}); err != nil {
		t.Fatal(err)
	}
	h := New(db, cfg)
	got := decode[struct {
		Views  []savedView  `json:"views"`
		Source []sourceView `json:"source"`
	}](t, get(t, h, apiBase+"views/", nil))
	if len(got.Source) != 1 || got.Source[0].Name != "Open bugs" || got.Source[0].JQL != "type = Bug" ||
		got.Source[0].ExternalID != "1" {
		t.Fatalf("source %+v", got.Source)
	}
}
