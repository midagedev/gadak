package main

// Linear fixture mode (--linear-out <path>): builds examples/demo-linear.db.
//
// The Jira modes of this command push content at a live site. The Linear
// mode has no site to push at — no live Linear workspace is ours to seed —
// and does not need one: the mirror's Linear shape is fully owned by
// sync.RunLinear and internal/linear, so the honest fixture replays the
// trick internal/sync/linear_test.go uses. A deterministic dataset is
// served from an in-process loopback GraphQL stub, and the production sync
// writes the mirror against it. Every mapping decision in the output —
// status categories, priority ranks, link phrases, cycle sprints — is made
// by the same code that maps a real workspace; nothing here re-implements
// a mapping. Writing the mirror by hand would be a second mapping that
// drifts the first time the real one changes.
//
// The dataset is fictional but Linear-shaped where Linear differs from
// Jira (MAPPING.md is the reference): markdown descriptions and comments,
// priority integers with display labels, workflow states typed
// backlog/unstarted/started/completed/canceled/duplicate, relations
// blocks/related/duplicate owned by one side, cycles per team, no issue
// types. Workspace slug "example" matches the scrubbed live captures in
// internal/linear/testdata, and issue URLs carry the identifier-uuid form
// Linear mints.
//
// All stamps hang off the generation moment, so a regenerated fixture is
// always freshly shaped (an active cycle, recently synced) the same way
// `gadak snapshot --spread` re-times the Jira fixture.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/sync"
)

// linearWorkspaceSlug is the workspace slug of the fixture's URLs. The
// scrubbed live captures use "example"; the slug lives in items.url only —
// sources.base_url stays "https://linear.app", the display base sync pins.
const linearWorkspaceSlug = "example"

// linearFixtureAPIKey names the stub credential. It is deliberately not
// lin_api_-shaped: scripts/scan-internal.sh greps the tree for real key
// shapes, same rule as the sync tests.
const linearFixtureAPIKey = "seed-key"

type linearFixtureTeam struct {
	team   linear.Team
	states []linear.WorkflowState
	cycles []linear.Cycle
}

type linearFixture struct {
	teams  []linearFixtureTeam
	issues []linear.Issue
}

// linearUUID mints a deterministic version-4-shaped uuid, the form the live
// captures scrub to (00000000-0000-4000-8000-…).
func linearUUID(n uint64) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", n)
}

var linearFixtureTitles = []string{
	"Uploads fail behind corporate proxies",
	"Add cycle goals to the board header",
	"Search stalls on large description bodies",
	"Keyboard shortcut sheet is out of date",
	"Comment editor loses draft on tab switch",
	"Support arbitrary webhook payloads",
	"Burndown chart double-counts moved issues",
	"Dark mode contrast on stale chips",
	"Offline queue drops the last write",
	"Import CSV with non-UTF-8 encodings",
	"Notification digest bundles too aggressively",
	"Expose cycle velocity via the API",
	"Triaging screen forgets filters",
	"Estimate rollup rounds the wrong way",
	"Slow render on thousand-issue backlogs",
	"Add a 'waiting on customer' state",
	"Two-factor enrolment email goes to spam",
	"Attachment previews flash before load",
	"Sub-issue counts include archived work",
	"Reduce duplicate webhook deliveries",
	"Label picker cannot be keyboard-navigated enough",
	"Per-team default templates",
	"Migrate away from the legacy sync cron",
	"Timezone drift on cycle boundaries",
}

var linearFixtureBodies = []string{
	"## Overview\n\nObserved on the standard plan with a repro of three steps.\n\n- open the upload dialog\n- attach a 2 MB file\n- submit twice quickly\n\n## Impact\n\nAbout a fifth of support tickets this week mention it.",
	"Feature ask from the Q3 planning doc.\n\n- [x] survey existing behaviour\n- [ ] decide on the shape\n- [ ] spec the API\n\nOut of scope for this cycle: bulk operations.",
	"Reported by two customers independently.\n\n```json\n{\"error\": \"rate_limited\", \"retry_after\": 37}\n```\n\nThe client currently treats that as fatal.",
	"Housekeeping item tracked here so it stops living in someone's head.\n\nThe old path is still deployed behind a flag; this removes the flag and the path.",
}

var linearFixtureLabelPool = []string{"bug", "feature", "performance", "ui", "api", "docs", "security", "tech-debt"}

var linearFixtureCommentBodies = []string{
	"Reproduced on staging — attaching the log excerpt to the internal doc.",
	"Noticed the same shape on the mobile client, probably one root cause.",
	"Spec drafted in the cycle doc; linking once it is reviewed.",
	"Workaround for now: retry once after a cold start.",
	"Moved this out of the current cycle, it needs the migration first.",
	"Regression suite covers the happy path; the retry branch is next.",
}

// linearFixtureUsers mirrors the Jira fixture's fictional cast so the two
// fixtures read as one product's world. Index 0 (Dana) is the account the
// e2e config identifies as.
var linearFixtureUsers = []linear.User{
	{ID: linearUUID(0xb1), Name: "Dana Whitfield", DisplayName: "Dana Whitfield", Email: "dana@example.com"},
	{ID: linearUUID(0xb2), Name: "Marco Silva", DisplayName: "Marco Silva", Email: "marco@example.com"},
	{ID: linearUUID(0xb3), Name: "Priya Nair", DisplayName: "Priya Nair", Email: "priya@example.com"},
	{ID: linearUUID(0xb4), Name: "Alex Kim", DisplayName: "Alex Kim", Email: "alex@example.com"},
}

// linearFixtureStateSpecs is one team's workflow. Names include a custom
// state (In Review) on purpose: names are display text, the type is the
// stable axis, and the fixture should exercise a custom name mapping
// through a standard type.
var linearFixtureStateSpecs = []linear.WorkflowState{
	{Name: "Backlog", Type: "backlog"},
	{Name: "Todo", Type: "unstarted"},
	{Name: "In Progress", Type: "started"},
	{Name: "In Review", Type: "started"},
	{Name: "Done", Type: "completed"},
	{Name: "Canceled", Type: "canceled"},
	{Name: "Duplicate", Type: "duplicate"},
}

// linearFixtureRelations is the fixed relation set. It is hand-shaped, not
// generated, because the e2e asserts exact labels against these keys: an
// owned blocks relation gives the owner the outward "blocks" row and the
// target the inward "is blocked by" row; related/duplicate have no catalog
// entry and render as their type name.
var linearFixtureRelations = []struct {
	from, to, typ string
}{
	{"LNX-1", "LNX-2", "blocks"},
	{"LNX-1", "LNX-3", "related"},
	{"LNX-2", "LNX-4", "duplicate"},
	{"LNX-6", "LNX-7", "blocks"},
	{"LNX-7", "LNX-8", "related"},
	{"LNM-1", "LNM-2", "blocks"},
	{"LNM-3", "LNM-4", "related"},
}

// linearIssueCountFor bounds --linear-issues: the fixed relations and the
// sub-issue tree reference LNX-1..8 and LNM-1..4, so a fixture smaller than
// this would name keys that do not exist.
const linearIssueCountFor = 16

// buildLinearFixture assembles the deterministic dataset. Everything hangs
// off `now` (cycle windows, spread stamps) and the seed; the same
// (now-truncated-to-the-day, count, seed) triple reproduces the same data.
func buildLinearFixture(now time.Time, count int, seed int64) (*linearFixture, error) {
	if count < linearIssueCountFor {
		return nil, fmt.Errorf("--linear-issues must be at least %d (fixed relations and sub-issues reference early keys)", linearIssueCountFor)
	}
	rng := rand.New(rand.NewSource(seed))

	// Two teams, six-in-ten issues to Core — a workspace with one large and
	// one small team, the shape the demo screenshots want.
	teams := []linearFixtureTeam{
		{team: linear.Team{ID: linearUUID(0xa1), Key: "LNX", Name: "Nimbus Core"}},
		{team: linear.Team{ID: linearUUID(0xa2), Key: "LNM", Name: "Nimbus Mobile"}},
	}
	for ti := range teams {
		base := uint64(0xc0 + ti*0x20)
		for si, spec := range linearFixtureStateSpecs {
			teams[ti].states = append(teams[ti].states, linear.WorkflowState{
				ID:   linearUUID(base + uint64(si)),
				Name: spec.Name, Type: spec.Type, Position: si + 1,
			})
		}
		// Three cycles around now: closed, active (unnamed, so the "Cycle
		// <n>" fallback name is exercised), future.
		day := 24 * time.Hour
		teams[ti].cycles = []linear.Cycle{
			{ID: linearUUID(base + 0x10), Number: 12, StartsAt: linearStamp(now.Add(-17 * day)), EndsAt: linearStamp(now.Add(-3 * day)), CompletedAt: linearStamp(now.Add(-3 * day).Add(2 * time.Hour)), Description: "Close out the import backlog."},
			{ID: linearUUID(base + 0x11), Number: 13, StartsAt: linearStamp(now.Add(-4 * day)), EndsAt: linearStamp(now.Add(10 * day))},
			{ID: linearUUID(base + 0x12), Number: 14, StartsAt: linearStamp(now.Add(11 * day)), EndsAt: linearStamp(now.Add(25 * day)), Description: "API hardening."},
		}
	}

	fx := &linearFixture{teams: teams}
	num := map[string]int{}
	byKey := map[string]*linear.Issue{}
	for i := 0; i < count; i++ {
		ti := 0
		if i%5 >= 3 {
			ti = 1
		}
		t := &teams[ti]
		num[t.team.Key]++
		key := fmt.Sprintf("%s-%d", t.team.Key, num[t.team.Key])
		created := now.Add(-time.Duration(180-i) * 24 * time.Hour).Add(time.Duration(i) * 40 * time.Minute)
		updated := created.Add(time.Duration(rng.Intn(72)) * time.Hour)
		if updated.After(now) {
			updated = now
		}
		state := t.states[i%len(t.states)]
		priority := i % 5 // 0 No priority … 4 Low: every rank appears

		iss := linear.Issue{
			ID:         linearUUID(0x1000 + uint64(i)),
			Identifier: key,
			Number:     num[t.team.Key],
			Title:      linearFixtureTitles[i%len(linearFixtureTitles)],
			URL:        fmt.Sprintf("https://linear.app/%s/issue/%s-%s", linearWorkspaceSlug, key, linearUUID(0x1000+uint64(i))),
			CreatedAt:  linearStamp(created),
			UpdatedAt:  linearStamp(updated),
			Priority:   priority,
			State:      state,
		}
		iss.Team.ID, iss.Team.Key, iss.Team.Name = t.team.ID, t.team.Key, t.team.Name
		iss.PriorityLabel = []string{"No priority", "Urgent", "High", "Medium", "Low"}[priority]
		iss.Description = linearFixtureBodies[i%len(linearFixtureBodies)]

		// People: round-robin so Dana (index 0) always has open assigned
		// work — the fixture's "mine" surfaces depend on her.
		iss.Creator = userRef(linearFixtureUsers[(i+1)%len(linearFixtureUsers)])
		if i%7 != 6 {
			iss.Assignee = userRef(linearFixtureUsers[i%len(linearFixtureUsers)])
		}
		// Labels: none / one / two by residue.
		for n := 0; n < i%3; n++ {
			iss.Labels.Nodes = append(iss.Labels.Nodes, linear.Label{
				ID:   linearUUID(0x2000 + uint64(i*8+n)),
				Name: linearFixtureLabelPool[(i+n)%len(linearFixtureLabelPool)],
			})
		}
		// Flow stamps from the state's own semantics, the only fields
		// NoHistory derive can consult.
		switch state.Type {
		case "started":
			iss.StartedAt = linearStamp(created.Add(time.Duration(1+rng.Intn(5)) * 24 * time.Hour))
			iss.Cycle = cycleRef(t.cycles[1]) // the active cycle
		case "completed":
			iss.StartedAt = linearStamp(created.Add(24 * time.Hour))
			iss.CompletedAt = linearStamp(created.Add(time.Duration(3+rng.Intn(9)) * 24 * time.Hour))
			iss.Cycle = cycleRef(t.cycles[0]) // the closed cycle
		case "canceled", "duplicate":
			iss.CanceledAt = linearStamp(created.Add(time.Duration(2+rng.Intn(6)) * 24 * time.Hour))
		}
		if i%3 == 0 {
			iss.DueDate = created.Add(45 * 24 * time.Hour).UTC().Format("2006-01-02")
		}
		// Comments inside created..updated, newest last.
		for n := 0; n < (i*7)%4; n++ {
			at := created.Add(time.Duration(n+1) * time.Hour)
			if at.After(updated) {
				at = updated
			}
			iss.Comments.Nodes = append(iss.Comments.Nodes, linear.Comment{
				ID:        linearUUID(0x3000 + uint64(i*8+n)),
				Body:      linearFixtureCommentBodies[(i+n)%len(linearFixtureCommentBodies)],
				CreatedAt: linearStamp(at),
				UpdatedAt: linearStamp(at),
				User:      userRef(linearFixtureUsers[(i+n+2)%len(linearFixtureUsers)]),
			})
		}
		// Sub-issues: one parent under LNX-5.
		if ti == 0 && num["LNX"] > 5 && num["LNX"] <= 9 {
			iss.Parent = &linear.ParentRef{ID: linearUUID(0x1000 + 4), Identifier: "LNX-5"}
		}
		fx.issues = append(fx.issues, iss)
	}
	// Pointers only after the last append: a pointer taken mid-loop dies
	// with the backing array the next append reallocates, and the relation
	// pass below would decorate stale copies — measured: 0 links landed in
	// the mirror until this moved below the loop.
	for i := range fx.issues {
		byKey[fx.issues[i].Identifier] = &fx.issues[i]
	}

	// Relations: the owner carries the relation on `relations`, the target
	// sees it back on `inverseRelations` — the split buildLinearRecord maps
	// to outward/inward links.
	for idx, r := range linearFixtureRelations {
		from, to := byKey[r.from], byKey[r.to]
		node := linear.IssueRelation{
			ID:           linearUUID(0x4000 + uint64(idx)),
			Type:         r.typ,
			Issue:        linear.ParentRef{ID: from.ID, Identifier: from.Identifier},
			RelatedIssue: linear.ParentRef{ID: to.ID, Identifier: to.Identifier},
		}
		from.Relations.Nodes = append(from.Relations.Nodes, node)
		to.InverseRelations.Nodes = append(to.InverseRelations.Nodes, node)
	}
	return fx, nil
}

func userRef(u linear.User) *linear.User {
	id, name, disp, email := u.ID, u.Name, u.DisplayName, u.Email
	return &linear.User{ID: id, Name: name, DisplayName: disp, Email: email}
}

func cycleRef(c linear.Cycle) *linear.CycleRef {
	return &linear.CycleRef{ID: c.ID, Number: c.Number, Name: c.Name, StartsAt: c.StartsAt, EndsAt: c.EndsAt, CompletedAt: c.CompletedAt}
}

// linearStamp is Linear's ISO-8601 UTC-with-milliseconds form, the format
// the mirror stores unmodified (types.go).
func linearStamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// serveLinearFixture answers the GraphQL surface the sync walks: paged
// issues by cursor, the team list, one team's cycles, and the per-issue
// completion queries for pages beyond the inline page size (the fixture
// never truncates, so those last two are belt-and-braces). Mutations are
// rejected — the connector is read-only by constitution and a fixture that
// somehow sent one should fail loudly, not quietly mutate.
func serveLinearFixture(fx *linearFixture) *httptest.Server {
	pages := map[string][]linear.Issue{"": fx.issues}
	for start := 0; start < len(fx.issues); start += 50 {
		end := start + 50
		if end > len(fx.issues) {
			end = len(fx.issues)
		}
		if start > 0 {
			pages[fmt.Sprintf("seed-page-%d", start/50-1)] = fx.issues[start:end]
		}
	}
	teams := make([]linear.Team, len(fx.teams))
	cycles := map[string][]linear.Cycle{}
	for i, t := range fx.teams {
		teams[i] = t.team
		cycles[t.team.ID] = t.cycles
	}

	write := func(w http.ResponseWriter, payload any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if strings.Contains(req.Query, "mutation") {
			http.Error(w, "read-only", http.StatusBadRequest)
			return
		}
		switch {
		case strings.Contains(req.Query, "query Issues"):
			after, _ := req.Variables["after"].(string)
			page, ok := pages[after]
			if !ok {
				http.Error(w, fmt.Sprintf("no page for cursor %q", after), http.StatusBadRequest)
				return
			}
			hasNext := len(page) == 50
			cursor := ""
			if hasNext {
				cursor = fmt.Sprintf("seed-page-%d", indexOfPage(fx.issues, page))
			}
			write(w, map[string]any{"data": map[string]any{"issues": linear.IssueConnection{
				PageInfo: linear.PageInfo{HasNextPage: hasNext, EndCursor: cursor},
				Nodes:    page,
			}}})
		case strings.Contains(req.Query, "query Teams"):
			write(w, map[string]any{"data": map[string]any{"teams": linear.TeamConnection{
				PageInfo: linear.PageInfo{},
				Nodes:    teams,
			}}})
		case strings.Contains(req.Query, "query TeamCycles"):
			id, _ := req.Variables["team"].(string)
			write(w, map[string]any{"data": map[string]any{"team": map[string]any{"cycles": linear.CycleConn{
				PageInfo: linear.PageInfo{},
				Nodes:    cycles[id],
			}}}})
		case strings.Contains(req.Query, "query Issue("):
			// Single-issue reads (SyncLinearIssue) never run in a pass, but
			// the stub answers honestly if one ever does.
			id, _ := req.Variables["id"].(string)
			for _, iss := range fx.issues {
				if iss.ID == id || iss.Identifier == id {
					write(w, map[string]any{"data": map[string]any{"issue": iss}})
					return
				}
			}
			write(w, map[string]any{"data": map[string]any{"issue": nil}})
		default:
			// Completion queries (comments/labels/attachments beyond the
			// inline page): the fixture never truncates, so an empty page
			// is the truthful answer.
			write(w, map[string]any{"data": map[string]any{"issue": map[string]any{
				"comments":    linear.CommentConn{PageInfo: linear.PageInfo{}},
				"labels":      linear.LabelConn{PageInfo: linear.PageInfo{}},
				"attachments": linear.AttachmentConn{PageInfo: linear.PageInfo{}},
			}}})
		}
	}))
}

// indexOfPage names the cursor of the page that FOLLOWS page p: the stub's
// cursor for page k is "seed-page-<k-1>", minted when the page was cut.
func indexOfPage(all, page []linear.Issue) int {
	if len(page) == 0 {
		return 0
	}
	first := page[0].ID
	for i, iss := range all {
		if iss.ID == first {
			return i / 50
		}
	}
	return 0
}

// writeLinearFixture runs the production sync against the stub onto a fresh
// mirror at path, and prints what landed. The mirror is written by
// sync.RunLinear only — this function writes no rows itself.
func writeLinearFixture(path string, count int, seed int64) int {
	fx, res, err := seedLinearMirror(path, count, seed, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for _, t := range fx.teams {
		fmt.Printf("linear team %s (%s): %d states, %d cycles\n", t.team.Key, t.team.Name, len(t.states), len(t.cycles))
	}
	fmt.Printf("linear sync: %d fetched, %d changed, full=%v\n", res.Fetched, res.Changed, res.Full)
	printLinearFixtureCounts(path)
	fmt.Printf("mirror written to %s\n", path)
	return 0
}

// seedLinearMirror is writeLinearFixture's body without the printing: build
// the dataset, serve it, run the real sync onto a fresh mirror. The test
// drives this so what it asserts is what the Makefile target produces.
func seedLinearMirror(path string, count int, seed int64, now time.Time) (*linearFixture, sync.Result, error) {
	fx, err := buildLinearFixture(now, count, seed)
	if err != nil {
		return nil, sync.Result{}, err
	}
	srv := serveLinearFixture(fx)
	defer srv.Close()

	c := linear.New(linearFixtureAPIKey)
	c.Endpoint = srv.URL
	c.HTTP = srv.Client()
	c.Retries = 1

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return nil, sync.Result{}, fmt.Errorf("remove %s: %w", path+suffix, err)
		}
	}
	db, err := store.Open(path)
	if err != nil {
		return nil, sync.Result{}, fmt.Errorf("open mirror: %w", err)
	}
	cfg := &config.Config{Linear: &config.LinearConfig{APIKey: linearFixtureAPIKey}}
	res, err := sync.RunLinear(context.Background(), cfg, db, sync.Options{LinearClient: c})
	closeErr := db.Close()
	if err != nil {
		return nil, sync.Result{}, fmt.Errorf("linear sync failed: %w", err)
	}
	if closeErr != nil {
		return nil, sync.Result{}, fmt.Errorf("close mirror: %w", closeErr)
	}
	return fx, res, nil
}

// printLinearFixtureCounts reads the finished mirror back so the fixture's
// own summary is measured, not assumed — the same stance demo-schema.sh
// takes when the Makefile target runs it next.
func printLinearFixtureCounts(path string) {
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return
	}
	defer raw.Close()
	for _, q := range []struct {
		label string
		sql   string
	}{
		{"issues", `SELECT count(*) FROM issues`},
		{"comments", `SELECT count(*) FROM comments`},
		{"links", `SELECT count(*) FROM links`},
		{"boards", `SELECT count(*) FROM boards WHERE type = 'cycles'`},
		{"sprints", `SELECT count(*) FROM sprints`},
	} {
		var n int
		if err := raw.QueryRow(q.sql).Scan(&n); err == nil {
			fmt.Printf("linear mirror %s: %d\n", q.label, n)
		}
	}
	var base string
	if err := raw.QueryRow(`SELECT id || ' kind=' || kind || ' base=' || base_url FROM sources`).Scan(&base); err == nil {
		fmt.Printf("linear source: %s\n", base)
	}
}
