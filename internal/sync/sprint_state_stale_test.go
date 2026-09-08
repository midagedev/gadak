package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// GDK-1661, measured on Jira Software 11.3.11 DC: `gadak sprint close 1`
// left the three issues of sprint 1 reading sprint_state = 'active' next to
// a sprints row that said closed. Closing a sprint moves only the incomplete
// issues out (their `updated` moves); the done ones stay put, so the issue
// watermark never sees the change — and every sprint_state = 'active' query,
// the board's active-sprint scope included, keeps counting last sprint's
// done issues as active.
//
// FAIL-first on the unmodified tree, both assertions fired:
//   - `sprints.state` stayed "active" — a tick where only a sprint's state
//     changed is by definition a quiet tick, and importAgile was gated on
//     Full || Reconcile || Changed > 0, so even the sprints row was not
//     refreshed;
//   - `issues.sprint_state` stayed "active" — nothing derived it from the
//     sprints row even when that row was fresh.
//
// The fix pair this pins: importAgile runs on every tick (the agile listing
// is the only observation path a sprint state change has), and
// ReplaceAgile derives each issue row's sprint_state from the sprints row of
// the same (source, sprint id) inside the same transaction.
func TestSprintStateDoesNotGoStaleOnQuietTick(t *testing.T) {
	// The gh-sprint custom field as Server projects it: the GreenHopper
	// toString array, state ACTIVE on the wire (lowercase 'active' is what
	// the mirror stores). The issue keeps saying ACTIVE in both phases —
	// the incremental tick re-reads nothing, so the toString cannot heal
	// the row; only the derivation can.
	const sprintToString = `["com.atlassian.greenhopper.service.sprint.Sprint@4ffcc813[autoStartStop=false,completeDate=<null>,endDate=2026-09-15T10:00:00.000Z,goal=<null>,id=1,incompleteIssuesDestinationId=<null>,name=Sprint 1,rapidViewId=1,sequence=1,startDate=2026-09-01T10:00:00.000Z,state=ACTIVE,synced=false]"]`
	issue := map[string]any{
		"id": "9001", "key": "NMB-1",
		"fields": map[string]any{
			"summary":   "done issue that stays in the sprint being closed",
			"project":   map[string]any{"key": "NMB"},
			"issuetype": map[string]any{"id": "10004", "name": "Bug"},
			"status": map[string]any{
				"id": "5", "name": "Done",
				"statusCategory": map[string]any{"key": "done"},
			},
			"created": "2026-09-01T10:00:00.000+0000",
			"updated": "2026-09-02T10:00:00.000+0000",
			// Sprint 1, ACTIVE — this is the projection that goes stale.
			"customfield_10001": json.RawMessage(sprintToString),
		},
	}
	issueJSON, err := json.Marshal(issue)
	if err != nil {
		t.Fatal(err)
	}
	// sprintState is what the Agile API answers for board 1's sprint 1;
	// the test flips it to closed between the two runs. quiet makes the
	// page walk answer nothing — the incremental tick sees no changed
	// issues, exactly like the DC where the closing touched no `updated`.
	sprintState, quiet := "active", false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/search":
			// One route serves the count probe (maxResults 0 — the
			// divergence probe's question) and every page walk (sync,
			// reconcile key scan) alike. The issue stays in scope in both
			// phases, so the count must keep answering 1.
			var body struct {
				MaxResults int `json:"maxResults"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("bad search body: %v", err)
				return
			}
			if body.MaxResults == 0 {
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":0,"total":1,"issues":[]}`))
				return
			}
			if quiet {
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":100,"total":0,"issues":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":100,"total":1,"issues":[` + string(issueJSON) + `]}`))
		case "/rest/api/2/status":
			_, _ = w.Write([]byte(`[{"id":"5","name":"Done","statusCategory":{"key":"done"}}]`))
		case "/rest/api/2/priority":
			_, _ = w.Write([]byte(`[{"id":"2","name":"High"}]`))
		case "/rest/api/2/issueLinkType":
			_, _ = w.Write([]byte(`{"issueLinkTypes":[]}`))
		case "/rest/api/2/filter/favourite": // Server has no /filter/my (GDK-1652)
			_, _ = w.Write([]byte(`[]`))
		case "/rest/api/2/field":
			// The sprint-field discovery reads the catalog; the gh-sprint
			// custom suffix is what makes customfield_10001 the sprint.
			_, _ = w.Write([]byte(`[{"id":"customfield_10001","name":"Sprint","custom":true,"schema":{"type":"array","custom":"com.pyxis.greenhopper.jira:gh-sprint"}}]`))
		case "/rest/api/2/project/NMB/versions":
			_, _ = w.Write([]byte(`[]`))
		case "/rest/agile/1.0/board":
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"total":1,"values":[{"id":1,"name":"NMB board","type":"scrum","location":{"projectKey":"NMB"}}]}`))
		case "/rest/agile/1.0/board/1/sprint":
			_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"id":1,"name":"Sprint 1","state":"` + sprintState + `","startDate":"2026-09-01T10:00:00.000Z","endDate":"2026-09-15T10:00:00.000Z","originBoardId":1}]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "no", http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client := jira.NewServer(srv.URL, "pat-token")
	client.Retries, client.Backoff = 1, 0 // no sleeping in tests

	cfg := &config.Config{
		Kind: config.OriginJiraServer, Site: srv.URL, Token: "pat-token",
		Projects: []string{"NMB"}, FieldMap: map[string]string{"severity": "customfield_10050"},
	}
	db := newMirror(t)
	if _, err := Run(context.Background(), cfg, db.DB, Options{Full: true, Client: client}); err != nil {
		t.Fatal(err)
	}
	// The full sync projected the sprint from the toString: both rows agree
	// on 'active' before anything goes stale.
	if got := db.column(t, "issues", "sprint_state", "NMB-1"); got != "active" {
		t.Fatalf("after the full sync issues.sprint_state = %q, want active", got)
	}
	var state string
	if err := db.DB.QueryRow(`SELECT state FROM sprints WHERE source_id = ? AND id = 1`, SourceID).Scan(&state); err != nil {
		t.Fatalf("sprints row: %v", err)
	}
	if state != "active" {
		t.Fatalf("after the full sync sprints.state = %q, want active", state)
	}

	// The origin closes sprint 1. No issue's `updated` moves — the done
	// issue stays exactly where it was — so the next tick is quiet.
	sprintState, quiet = "closed", true
	res, err := Run(context.Background(), cfg, db.DB, Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full || res.Fetched != 0 || res.Changed != 0 {
		t.Fatalf("the tick was not quiet: %+v", res)
	}
	if err := db.DB.QueryRow(`SELECT state FROM sprints WHERE source_id = ? AND id = 1`, SourceID).Scan(&state); err != nil {
		t.Fatalf("sprints row: %v", err)
	}
	if state != "closed" {
		t.Errorf("sprints.state = %q, want closed — a sprint state change is invisible to the issue watermark, so the tick must still run the agile listing", state)
	}
	if got := db.column(t, "issues", "sprint_state", "NMB-1"); got != "closed" {
		t.Errorf("issues.sprint_state = %q, want closed — the denormalized column must be derived from the sprints row, not left at what the issue sync projected", got)
	}
}
