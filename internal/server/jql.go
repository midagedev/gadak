package server

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
)

// POST/GET /api/v1/issues/jql/ — parse a JQL string or Jira URL into the
// ViewFilters shape the client already applies. GET takes ?q= and optional
// ?email=. Never 4xx for "cannot express this clause": those land in
// unsupported. 400 only for an empty body.
func (s *server) handleJql(w http.ResponseWriter, r *http.Request) {
	var input, email string
	switch r.Method {
	case http.MethodGet:
		input = r.URL.Query().Get("q")
		email = r.URL.Query().Get("email")
	default:
		var req struct {
			Input string `json:"input"`
			Email string `json:"email"`
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			fail(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				fail(w, http.StatusBadRequest, "invalid_json")
				return
			}
		}
		input, email = req.Input, req.Email
	}
	me := configuredIdentity(s, email)
	if input == "" {
		fail(w, http.StatusBadRequest, "jql_required")
		return
	}

	res := jql.Parse(input, jql.Opts{Now: time.Now(), Email: me.Email, AccountID: me.AccountID})
	if res.Error == "" {
		if lites, err := s.db.IssueLites(r.Context()); err == nil {
			jql.ResolveIdentity(&res, peopleFromLites(lites), me)
			res.JQL, res.Omitted = jql.Emit(res.Filters, res.Display, jql.EmitOpts{Email: me.Email, AccountID: me.AccountID})
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) handleJqlEmit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filters jql.Filter  `json:"filters"`
		Display jql.Display `json:"display"`
		Email   string      `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "invalid_json")
		return
	}
	me := configuredIdentity(s, req.Email)
	canonical, omitted := jql.Emit(req.Filters, req.Display, jql.EmitOpts{Email: me.Email, AccountID: me.AccountID})
	writeJSON(w, http.StatusOK, map[string]any{
		"jql":     canonical,
		"omitted": omitted,
	})
}

func configuredIdentity(s *server, email string) jql.Identity {
	me := jql.Identity{Email: email}
	if cfg := s.config(); cfg != nil {
		if me.Email == "" {
			me.Email = cfg.Email
		}
		me.AccountID = cfg.AccountID
	}
	return me
}

func peopleFromLites(lites []store.IssueLite) []jql.Person {
	issues := make([]jql.Issue, len(lites))
	for i, l := range lites {
		issues[i] = jql.Issue{
			Assignee:      deref(l.Assignee),
			AssigneeEmail: deref(l.AssigneeEmail),
			AssigneeID:    deref(l.AssigneeID),
			Reporter:      deref(l.Reporter),
			ReporterEmail: deref(l.ReporterEmail),
			ReporterID:    deref(l.ReporterID),
		}
	}
	return jql.PeopleFromIssues(issues)
}
