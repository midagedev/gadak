package main

// The JQL binding tests the green internal/jql suite could not carry
// (GDK-1564): `gadak search --jql 'reporter = …'` shipped answering 0 rows
// because the CLI adapter jqlIssue filled AssigneeID and dropped ReporterID,
// while the resolver normalizes every person token to the account id — so
// the id field is the only one an id-only row can match on. jql_test.go
// tests jql.Match directly and stayed green; these tests ride the adapter
// the user rides (cmdSearch end to end) and enumerate the field mapping so
// the next dropped column fails by name.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/midagedev/gadak/internal/dashboards"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
)

// seedReporterIssues adds the reporter pair the reporter tests key on:
// NMB-2 carries the full reporter triple, NMB-3 is id-only the way Jira
// cloud hides reporter emails — the row only matchable through ReporterID.
func seedReporterIssues(t *testing.T) {
	t.Helper()
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"3": "inprogress", "10001": "done"},
		Priorities: []string{"Highest", "High", "Medium"},
		Records: []store.IssueRecord{
			{
				Item: store.Item{
					ID: "jira:1002", SourceID: "jira", Kind: "issue", ExternalID: "1002", Key: "NMB-2",
					Title:     "mirror copy drifts from origin",
					CreatedAt: "2026-07-10T00:00:00.000Z", UpdatedAt: "2026-07-20T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "10004",
					Status: "완료", StatusID: "10001", StatusCategory: "done",
					Priority: "Highest",
					Reporter: "Marco Reyes", ReporterID: "acc-rp", ReporterEmail: "rp@example.com",
				},
			},
			{
				Item: store.Item{
					ID: "jira:1003", SourceID: "jira", Kind: "issue", ExternalID: "1003", Key: "NMB-3",
					Title:     "reporter email hidden by origin",
					CreatedAt: "2026-08-02T00:00:00.000Z", UpdatedAt: "2026-08-05T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "NMB", IssueType: "Task", IssueTypeID: "10005",
					Status: "진행 중", StatusID: "3", StatusCategory: "inprogress",
					Priority:   "Medium",
					ReporterID: "acc-rp",
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed reporters: %v", err)
	}
}

// searchJSONKeys runs cmdSearch --jql --json and returns the total plus the
// issue keys in printed order.
func searchJSONKeys(t *testing.T, query string) (int, []string) {
	t.Helper()
	out, err := capture(t, func() error {
		return cmdSearch([]string{"--jql", query, "--json"})
	})
	if err != nil {
		t.Fatalf("search --jql %q: %v\n%s", query, err, out)
	}
	var body struct {
		Total  int `json:"total"`
		Issues []struct {
			IssueKey string `json:"issue_key"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	keys := make([]string, len(body.Issues))
	for i, it := range body.Issues {
		keys[i] = it.IssueKey
	}
	return body.Total, keys
}

// GDK-1564 FAIL-first: a reporter clause must find the id-only row. The
// resolver rewrites "rp@example.com" (and the bare account id) to "acc-rp",
// so both rows match through ReporterID — the exact field the CLI adapter
// dropped. A miss reports an honest-looking empty result, which is why this
// shipped: assert the keys, not just non-zero.
func TestSearchJQLReporterMatchesIDOnlyReporter(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedReporterIssues(t)

	for _, query := range []string{
		`reporter = "rp@example.com"`,
		`reporter = "acc-rp"`,
	} {
		total, keys := searchJSONKeys(t, query)
		if total != 2 || len(keys) != 2 {
			t.Errorf("%s: total %d keys %v, want NMB-2 and NMB-3 (the id-only row must match)", query, total, keys)
			continue
		}
		sort.Strings(keys)
		if keys[0] != "NMB-2" || keys[1] != "NMB-3" {
			t.Errorf("%s: keys %v, want [NMB-2 NMB-3]", query, keys)
		}
	}

	// Not vacuous: a reporter that resolves (NMB-1's assignee is in the
	// roster) but reported nothing still selects nothing. An unresolved
	// value is the wrong control — it degrades to all-issues-plus-warning,
	// the documented partial-apply behavior.
	if total, _ := searchJSONKeys(t, `reporter = "acc-hc"`); total != 0 {
		t.Errorf(`reporter = "acc-hc": total %d, want 0`, total)
	}
}

// TestSearchJQLOrdersLikeDashboardJQL is the GDK-1574 gate: the same query
// must list the same order through `gadak search --jql` and a JQL dashboard
// (dashboards.ExecuteJQL). The comment that promised this parity lived on a
// 30-line copy; the promise lives here now, where a diverging sort actually
// fails.
func TestSearchJQLOrdersLikeDashboardJQL(t *testing.T) {
	mirror(t, "https://unused.example.com")
	seedReporterIssues(t) // NMB-1/2/3 now differ on updated, created, priority

	queries := []string{
		`project = NMB`,                      // default: updated desc
		`project = NMB ORDER BY updated ASC`, // oldest first
		`project = NMB ORDER BY created DESC`,
		`project = NMB ORDER BY priority DESC`,
		`project = NMB ORDER BY created ASC`,
	}
	db, err := store.Open(filepath.Join(os.Getenv("GADAK_HOME"), "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, query := range queries {
		_, cliKeys := searchJSONKeys(t, query)
		res, err := dashboards.ExecuteJQL(context.Background(), db, jql.Identity{Email: "agent@example.com"}, query)
		if err != nil {
			t.Fatalf("ExecuteJQL %q: %v", query, err)
		}
		dashKeys := make([]string, len(res.Rows))
		for i, row := range res.Rows {
			dashKeys[i], _ = row[0].(string)
		}
		if len(cliKeys) != len(dashKeys) {
			t.Fatalf("%q: CLI found %d (%v), dashboard %d (%v)", query, len(cliKeys), cliKeys, len(dashKeys), dashKeys)
		}
		for i := range cliKeys {
			if cliKeys[i] != dashKeys[i] {
				t.Errorf("%q: order diverges at %d: CLI %v, dashboard %v", query, i, cliKeys, dashKeys)
				break
			}
		}
	}
}
