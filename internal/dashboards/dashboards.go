// Package dashboards owns the interpretation of dashboard rows: the config
// document (HTML + named datasources) and its validation rules, name→row
// resolution, and datasource execution into the {columns, rows, truncated,
// warning?} document the render iframe draws from (GDK-781).
//
// Two surfaces speak dashboards — `gadak dashboards` (cmd/gadak) and the
// /api/v1/dashboards/ handlers (internal/server) — and both import this
// package for the same reason internal/views exists (GDK-612): the rule and
// the row shape must have one owner, not a CLI copy and an API copy that
// drift.
//
// SQL datasources run only on a read-only connection handed in by the caller
// (store.DB.ReadOnly). That is not a convenience choice: arbitrary SQL is
// allowed precisely because it can never take a write path — the mirror is a
// cache of the origin, and this package is where that stays true.
package dashboards

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/sqlhint"
	"github.com/midagedev/gadak/internal/store"
)

// Source is one named datasource: exactly one of SQL or JQL.
type Source struct {
	SQL string `json:"sql,omitempty"`
	JQL string `json:"jql,omitempty"`
}

// Config is the document a dashboard row stores. Datasources may be empty —
// a static HTML dashboard is valid — but never nil after ParseConfig. Libs
// (GDK-808) names cache entries from `gadak dashboards lib add`; existence is
// checked by the save paths (CLI and API), not here — ParseConfig owns the
// shape, callers own the world.
type Config struct {
	HTML        string            `json:"html"`
	Datasources map[string]Source `json:"datasources,omitempty"`
	Libs        []string          `json:"libs,omitempty"`
}

// NamePattern is the datasource-name rule, quoted verbatim in every
// violation message so an agent can fix the name without a second round-trip.
const NamePattern = `[a-z0-9][a-z0-9_-]{0,63}`

var nameRe = regexp.MustCompile(`^` + NamePattern + `$`)

// ValidName is the datasource-name rule: a name is a URL-path-safe token
// (it becomes a path segment on /data/{name}/) starting alphanumeric.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// ParseConfig decodes and validates a stored config document. It is strict
// on purpose: every field it accepts is a field render/data must honor, so
// an unknown key (an older config shape, a typo'd hand edit) is a named
// error, not a silently ignored clause.
func ParseConfig(raw []byte) (Config, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return Config{}, fmt.Errorf("dashboard config is not valid JSON: %w", err)
	}
	var cfg Config
	for key := range top {
		switch key {
		case "html", "datasources", "libs":
		default:
			return Config{}, fmt.Errorf("dashboard config: unknown field %q (expected html, datasources, libs)", key)
		}
	}
	if rawHTML, ok := top["html"]; ok {
		if err := json.Unmarshal(rawHTML, &cfg.HTML); err != nil {
			return Config{}, fmt.Errorf("dashboard config: html must be a string: %w", err)
		}
	}
	if strings.TrimSpace(cfg.HTML) == "" {
		return Config{}, errors.New("dashboard config: html is required and must not be empty")
	}
	cfg.Datasources = map[string]Source{}
	if rawDS, ok := top["datasources"]; ok {
		var items map[string]json.RawMessage
		if err := json.Unmarshal(rawDS, &items); err != nil {
			return Config{}, fmt.Errorf("dashboard config: datasources must be an object: %w", err)
		}
		for name, item := range items {
			if !ValidName(name) {
				return Config{}, fmt.Errorf("datasource name %q must match %s", name, NamePattern)
			}
			src, err := parseSource(item)
			if err != nil {
				return Config{}, fmt.Errorf("datasource %q: %w", name, err)
			}
			cfg.Datasources[name] = src
		}
	}
	if rawLibs, ok := top["libs"]; ok {
		if err := json.Unmarshal(rawLibs, &cfg.Libs); err != nil {
			return Config{}, fmt.Errorf("dashboard config: libs must be an array of lib ids: %w", err)
		}
		if len(cfg.Libs) > MaxLibs {
			return Config{}, fmt.Errorf("dashboard config: libs lists %d entries, at most %d (chart libraries load in order; more than that is a config accident)", len(cfg.Libs), MaxLibs)
		}
		seen := make(map[string]bool, len(cfg.Libs))
		for _, id := range cfg.Libs {
			if !ValidLibID(id) {
				return Config{}, fmt.Errorf("dashboard config: lib id %q must match %s (the id `gadak dashboards lib list` prints)", id, LibIDPattern)
			}
			if seen[id] {
				return Config{}, fmt.Errorf("dashboard config: lib id %q is listed twice", id)
			}
			seen[id] = true
		}
	}
	return cfg, nil
}

// parseSource decodes one datasource item strictly: the keys sql and jql are
// the whole vocabulary, and exactly one must carry a non-empty query.
func parseSource(item json.RawMessage) (Source, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(item, &fields); err != nil {
		return Source{}, fmt.Errorf("must be an object with sql or jql: %w", err)
	}
	var src Source
	for key := range fields {
		switch key {
		case "sql":
			if err := json.Unmarshal(fields[key], &src.SQL); err != nil {
				return Source{}, fmt.Errorf("sql must be a string: %w", err)
			}
		case "jql":
			if err := json.Unmarshal(fields[key], &src.JQL); err != nil {
				return Source{}, fmt.Errorf("jql must be a string: %w", err)
			}
		default:
			return Source{}, fmt.Errorf("unknown field %q (each datasource is {\"sql\": …} or {\"jql\": …})", key)
		}
	}
	switch {
	case src.SQL != "" && src.JQL != "":
		return Source{}, errors.New("exactly one of sql or jql is required (got both)")
	case src.SQL == "" && src.JQL == "":
		return Source{}, errors.New("exactly one of sql or jql is required (got neither)")
	}
	return src, nil
}

// FindDashboard resolves one name to one row: exact id or exact name
// (case-insensitive) first, then a single substring hit. Same resolution
// policy as views.FindView so `gadak dashboards show triage` and a future
// MCP tool behave alike; only the miss policy is dashboard-specific (the
// message names the command that lists what exists).
func FindDashboard(db *store.DB, name string) (store.Dashboard, error) {
	list, err := db.Dashboards(context.Background())
	if err != nil {
		return store.Dashboard{}, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	var exact, sub []store.Dashboard
	for _, d := range list {
		id := strings.ToLower(d.ID)
		nm := strings.ToLower(d.Name)
		if id == want || nm == want {
			exact = append(exact, d)
			continue
		}
		if strings.Contains(nm, want) || strings.Contains(id, want) {
			sub = append(sub, d)
		}
	}
	hits := exact
	if len(hits) == 0 {
		hits = sub
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		if len(list) == 0 {
			return store.Dashboard{}, fmt.Errorf("no dashboard matching %q — none saved; `gadak dashboards save NAME --html FILE` creates one", name)
		}
		names := make([]string, 0, len(list))
		for _, d := range list {
			names = append(names, d.Name)
		}
		return store.Dashboard{}, fmt.Errorf("no dashboard matching %q — available: %s", name, strings.Join(names, "; "))
	default:
		names := make([]string, len(hits))
		for i, h := range hits {
			names[i] = h.Name
		}
		return store.Dashboard{}, fmt.Errorf("%q matches %d dashboards — be more specific: %s", name, len(hits), strings.Join(names, "; "))
	}
}

// Result is the datasource execution document. It is also the postMessage
// payload contract for the web round: {type:'data', name, columns, rows,
// truncated, warning?} carries these fields verbatim.
type Result struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
	Warning   string   `json:"warning,omitempty"`
}

// Row ceilings for datasource execution (server halves of the same
// contract). Generous on purpose — this is a performance guard for a local
// single-user tool, not a quota: a triage wall wants thousands of rows long
// before anyone writes a query that returns them.
const (
	MaxRows     = 10000
	MaxRowBytes = 2 << 20 // 2 MiB of marshaled row payload
)

// rowSink accumulates rows under the two ceilings. A row is always added
// before the size check so a single giant row still surfaces (with
// truncated set) instead of yielding an empty, silently-cut result.
type rowSink struct {
	res       Result
	size      int
	truncated bool
}

func (s *rowSink) columns(cols []string) {
	s.res.Columns = cols
	s.res.Rows = [][]any{}
}

func (s *rowSink) add(row []any) bool {
	// true when the caller must stop reading — a ceiling was hit.
	if len(s.res.Rows) >= MaxRows {
		s.truncated = true
		return true
	}
	s.res.Rows = append(s.res.Rows, row)
	b, err := json.Marshal(row)
	if err == nil {
		s.size += len(b)
	}
	if s.size >= MaxRowBytes {
		s.truncated = true
		return true
	}
	return false
}

func (s *rowSink) result(warning string) Result {
	s.res.Truncated = s.truncated
	s.res.Warning = warning
	return s.res
}

// ExecuteSQL runs one datasource SQL statement on ro, which callers must
// open through store.DB.ReadOnly — the read-only connection is what makes
// arbitrary SQL safe to allow. Zero rows plus a display-name comparison
// yields the sqlhint warning verbatim (the locale trap, surfaced where the
// agent will see it instead of as a mystery empty card).
func ExecuteSQL(ro *sql.DB, query string) (Result, error) {
	rows, err := ro.Query(query)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}
	sink := &rowSink{}
	sink.columns(cols)
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return Result{}, err
		}
		row := make([]any, len(cols))
		for i := range vals {
			row[i] = cell(vals[i])
		}
		if sink.add(row) {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	return sink.result(sqlhint.ZeroRowDisplayNameWarning(query, len(sink.res.Rows))), nil
}

// jqlColumns is the fixed projection of a JQL datasource: stable keys first
// (status_category), the display status for humans, and the sort-relevant
// fields. A dashboard needing more columns writes a SQL datasource.
var jqlColumns = []string{"issue_key", "summary", "status_category", "status", "priority_rank", "updated_at"}

// ExecuteJQL runs one datasource JQL query against the mirror: parse,
// resolve currentUser() against the configured identity and the actor
// directory, match, apply the ORDER BY, and project to jqlColumns. It is the
// server jql execution path (internal/server jql.go's parse+identity) with
// the CLI's match step (cmd/gadak searchJQL) — the two halves this feature
// finally needs in one place.
func ExecuteJQL(ctx context.Context, db *store.DB, me jql.Identity, query string) (Result, error) {
	parsed := jql.Parse(query, jql.Opts{Now: time.Now(), Email: me.Email, AccountID: me.AccountID})
	if parsed.Error != "" {
		return Result{}, fmt.Errorf("jql: %s", parsed.Message)
	}
	people, err := db.QueryActorPeople(ctx)
	if err != nil {
		return Result{}, err
	}
	jql.ResolveIdentity(&parsed, actorsToPeople(people), me)
	if len(parsed.Applied) == 0 && len(parsed.Unsupported) > 0 {
		return Result{}, fmt.Errorf("cannot apply JQL — %s", strings.Join(parsed.Unsupported, "; "))
	}
	lites, err := db.IssueLites(ctx)
	if err != nil {
		return Result{}, err
	}
	matched := make([]store.IssueLite, 0, len(lites))
	for _, l := range lites {
		if jql.Match(liteToIssue(l), parsed.Filters) {
			matched = append(matched, l)
		}
	}
	sortDisplay(matched, parsed.Display)
	sink := &rowSink{}
	sink.columns(jqlColumns)
	for _, l := range matched {
		row := []any{l.IssueKey, l.Summary, l.StatusCategory, l.Status, l.PriorityRank, deref(l.UpdatedAt, "")}
		if sink.add(row) {
			break
		}
	}
	// A query that applies less than it wrote must say so: measured live on a
	// demo home, `assignee = currentUser() AND resolution is EMPTY` with no
	// configured identity silently becomes "all open issues" — the applicable
	// half still runs, and the card reads true. The resolver's own strings
	// (why each clause was skipped) ride along under the CLI's wording.
	warning := ""
	if len(parsed.Unsupported) > 0 {
		warning = "jql skipped: " + strings.Join(parsed.Unsupported, "; ")
	}
	return sink.result(warning), nil
}

// The three helpers below are the package-boundary copies this feature
// could not import: their originals live in package main (cmd/gadak) or in
// the server package, and internal/jql deliberately does not import store
// (its own doc comment says so). Each copy cites its sibling; if a third
// surface needs one, promote that helper to a shared home first.

// liteToIssue maps a mirror row onto jql's neutral shape. Sibling:
// jqlIssue in cmd/gadak/agent.go (searchJQL's match step).
func liteToIssue(l store.IssueLite) jql.Issue {
	return jql.Issue{
		Key:            l.IssueKey,
		ParentKey:      deref(l.ParentKey, ""),
		Project:        l.ProjectKey,
		Status:         l.Status,
		StatusCategory: l.StatusCategory,
		Type:           l.IssueType,
		Priority:       deref(l.Priority, ""),
		Assignee:       deref(l.Assignee, ""),
		AssigneeEmail:  deref(l.AssigneeEmail, ""),
		AssigneeID:     deref(l.AssigneeID, ""),
		Reporter:       deref(l.Reporter, ""),
		ReporterEmail:  deref(l.ReporterEmail, ""),
		ReporterID:     deref(l.ReporterID, ""),
		Labels:         l.Labels,
		Components:     l.Components,
		FixVersions:    l.FixVersions,
		CreatedAt:      deref(l.CreatedAt, ""),
		UpdatedAt:      deref(l.UpdatedAt, ""),
		Duedate:        deref(l.Duedate, ""),
		ResolvedAt:     deref(l.ResolvedAt, ""),
		SprintID:       sprintIDString(l.SprintID),
		SprintState:    deref(l.SprintState, ""),
	}
}

// actorsToPeople turns the narrow actor projection into ResolveIdentity's
// input. Sibling: peopleFromActors in internal/server/jql.go.
func actorsToPeople(people []store.ActorPerson) []jql.Person {
	issues := make([]jql.Issue, len(people))
	for i, p := range people {
		issues[i] = jql.Issue{
			Assignee:      p.AssigneeName,
			AssigneeEmail: p.AssigneeEmail,
			AssigneeID:    p.AssigneeID,
			Reporter:      p.ReporterName,
			ReporterEmail: p.ReporterEmail,
			ReporterID:    p.ReporterID,
		}
	}
	return jql.PeopleFromIssues(issues)
}

// sortDisplay applies the parsed ORDER BY. Sibling: sortJQL in
// cmd/gadak/agent.go — same ordering semantics (dir defaults to desc,
// priority tiebreaks on updated_at) so a JQL dashboard and `gadak search
// --jql` list the same order.
func sortDisplay(list []store.IssueLite, d jql.Display) {
	dir := 1
	if d.Dir != "asc" {
		dir = -1
	}
	lessTime := func(a, b *string) bool {
		av, bv := deref(a, ""), deref(b, "")
		if dir < 0 {
			return av > bv
		}
		return av < bv
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch d.Sort {
		case "created":
			return lessTime(a.CreatedAt, b.CreatedAt)
		case "priority":
			if a.PriorityRank != b.PriorityRank {
				if dir < 0 {
					return a.PriorityRank < b.PriorityRank
				}
				return a.PriorityRank > b.PriorityRank
			}
			return deref(a.UpdatedAt, "") > deref(b.UpdatedAt, "")
		default:
			return lessTime(a.UpdatedAt, b.UpdatedAt)
		}
	})
}

func sprintIDString(id *int64) string {
	if id == nil {
		return ""
	}
	return fmt.Sprintf("%d", *id)
}

func deref(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// cell normalizes one SQL value for JSON: bytes become strings (SQLite TEXT
// arrives as []byte), NULL stays null, everything else marshals as itself.
// Sibling: cell in cmd/gadak/sql.go.
func cell(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
