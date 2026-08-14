package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/store"
)

func TestViewsListAndShow(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSourceQueries(context.Background(), "jira", []store.SourceQuery{
		{ID: "jira:10008", SourceID: "jira", ExternalID: "10008", Name: "gadak-test: NMA in progress",
			QueryText: `project = NMA AND statusCategory = "In Progress"`,
			Config:    json.RawMessage(`{"filters":{"jira_project":["NMA"],"status_category":["inprogress"]},"display":{"group_by":"status_category"}}`),
			Favourite: true, Applied: []string{"project", "statusCategory"}},
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	out, err := capture(t, func() error { return cmdViews([]string{"--json"}) })
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "gadak-test: NMA in progress") || !strings.Contains(out, "jira:10008") {
		t.Fatalf("list %s", out)
	}

	out, err = capture(t, func() error { return cmdViews([]string{"show", "NMA in progress"}) })
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "pj=NMA") || !strings.Contains(out, "sc=inprogress") {
		t.Fatalf("show hash %s", out)
	}
}

func TestViewsOpenWritesFocus(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA AND statusCategory = "In Progress"`, "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	var body struct {
		Hash string `json:"hash"`
		File bool   `json:"file"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if !body.File || !strings.Contains(body.Hash, "pj=NMA") || !strings.Contains(body.Hash, "sc=inprogress") {
		t.Fatalf("open %+v", body)
	}
}

func TestViewsOpenNoOpenSkipsLaunch(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA AND statusCategory = "In Progress"`, "--no-open", "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	var body struct {
		Hash    string `json:"hash"`
		File    bool   `json:"file"`
		Web     string `json:"web"`
		Desktop bool   `json:"desktop"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if !body.File || body.Web != "" || body.Desktop || !strings.Contains(body.Hash, "pj=NMA") {
		t.Fatalf("no-open %+v", body)
	}
}

func TestViewsOpenEnvNoOpen(t *testing.T) {
	mirror(t, "https://unused.example.com")
	t.Setenv("GADAK_NO_OPEN", "1")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA`, "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"desktop":false`) || !strings.Contains(out, `"web":""`) {
		t.Fatalf("GADAK_NO_OPEN still launched: %s", out)
	}
}
