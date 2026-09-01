package sync

import (
	"context"
	"encoding/json"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
)

// importFilters pulls the account's owned and starred Jira filters and
// compiles each JQL into a ViewConfig. Failure is logged and the previous
// rows stay — a filter-list 500 must not undo a finished issue pass.
func importFilters(ctx context.Context, c *jira.Client, cfg *config.Config, db *store.DB, opts Options) {
	filters, err := c.MyFilters(ctx)
	if err != nil {
		opts.logf("filters: skipped (%v)", err)
		return
	}
	people := peopleFromDB(ctx, db)
	out := make([]store.SourceQuery, 0, len(filters))
	for _, f := range filters {
		q := store.SourceQuery{
			ID:         SourceID + ":" + f.ID,
			SourceID:   SourceID,
			ExternalID: f.ID,
			Name:       f.Name,
			QueryText:  f.JQL,
			Favourite:  f.Favourite,
			Owner:      f.Owner,
		}
		q.Config, q.Applied, q.Unsupported = compileFilter(f.JQL, people, IdentityFromConfig(cfg))
		out = append(out, q)
	}
	if err := db.ReplaceSourceQueries(ctx, SourceID, out); err != nil {
		opts.logf("filters: store failed (%v)", err)
		return
	}
	if cfg == nil || !cfg.HasLocalOrigin() {
		opts.logf("filters: %d from Jira", len(out))
	}
}

// IdentityFromConfig is the JQL identity (email / account id) cfg carries —
// the "me" of currentUser() when a JQL filter is compiled. Single owner
// (GDK-619): filter import and `gadak views save` both read it.
func IdentityFromConfig(cfg *config.Config) jql.Identity {
	if cfg == nil {
		return jql.Identity{}
	}
	return jql.Identity{Email: cfg.Email, AccountID: cfg.AccountID}
}

func compileFilter(jqlText string, people []jql.Person, me jql.Identity) (json.RawMessage, []string, []string) {
	parsed := jql.Parse(jqlText, jql.Opts{Email: me.Email, AccountID: me.AccountID})
	if parsed.Error != "" {
		return emptyViewConfig(), nil, []string{parsed.Message}
	}
	jql.ResolveIdentity(&parsed, people, me)
	type display struct {
		GroupBy string `json:"group_by"`
		Sort    string `json:"sort,omitempty"`
		Dir     string `json:"dir,omitempty"`
	}
	d := display{GroupBy: "status_category"}
	if parsed.Display.Sort != "" {
		d.Sort = parsed.Display.Sort
		d.Dir = parsed.Display.Dir
	}
	body, err := json.Marshal(struct {
		Filters jql.Filter `json:"filters"`
		Display display    `json:"display"`
	}{Filters: parsed.Filters, Display: d})
	if err != nil {
		return emptyViewConfig(), parsed.Applied, parsed.Unsupported
	}
	return body, parsed.Applied, parsed.Unsupported
}

func emptyViewConfig() json.RawMessage {
	return json.RawMessage(`{"filters":{},"display":{"group_by":"status_category"}}`)
}

func peopleFromDB(ctx context.Context, db *store.DB) []jql.Person {
	lites, err := db.IssueLites(ctx)
	if err != nil {
		return nil
	}
	issues := make([]jql.Issue, len(lites))
	for i, l := range lites {
		issues[i] = jql.Issue{
			ParentKey:     derefPtr(l.ParentKey),
			Assignee:      derefPtr(l.Assignee),
			AssigneeEmail: derefPtr(l.AssigneeEmail),
			AssigneeID:    derefPtr(l.AssigneeID),
			Reporter:      derefPtr(l.Reporter),
			ReporterEmail: derefPtr(l.ReporterEmail),
			ReporterID:    derefPtr(l.ReporterID),
		}
	}
	return jql.PeopleFromIssues(issues)
}

func derefPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
