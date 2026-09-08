package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// Linear cycles land in the sprint columns and the sprints/boards tables
// (GDK-1667). The integer the mirror stores is derived from the cycle's
// UUID — FNV-1a 64 of the string, top bit cleared, 0 → 1 — and this file
// re-derives it independently so the tests pin the value, not whatever
// helper the production path ends up sharing.
func fnvSprintID(uuid string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(uuid))
	id := int64(h.Sum64() &^ (1 << 63))
	if id == 0 {
		return 1
	}
	return id
}

// linearStamp formats a test instant the way Linear serializes a DateTime
// (ISO-8601 UTC with milliseconds) — the format the mirror stores verbatim.
func linearStamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

const cycleTeamUUID = "00000000-0000-4000-8000-000000000003"

func cycleNode(id string, number int, name, description, startsAt, endsAt, completedAt string) map[string]any {
	return map[string]any{
		"id": id, "number": number, "name": name, "description": description,
		"startsAt": startsAt, "endsAt": endsAt, "completedAt": completedAt,
	}
}

func cycleRef(id string, number int, name, startsAt, endsAt, completedAt string) map[string]any {
	return map[string]any{
		"id": id, "number": number, "name": name,
		"startsAt": startsAt, "endsAt": endsAt, "completedAt": completedAt,
	}
}

// cyclesStub answers the three documents a Linear pass sends, dispatching on
// the query text the way linearGQL dispatches on the cursor: the issues page,
// the teams listing, and one team's cycles. Mutations are refused — the
// mirror is read-only. bodies is consulted on every request so a test can
// flip the origin between two runs.
func cyclesStub(t *testing.T, bodies map[string]*string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad request body: %v", err)
			http.Error(w, "bad body", 400)
			return
		}
		pick := func(kind string) string {
			p := bodies[kind]
			if p == nil {
				t.Errorf("stub has no %s body for query: %s", kind, req.Query)
				return `{"errors":[{"message":"no body"}]}`
			}
			return *p
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(req.Query, "mutation"):
			t.Errorf("mirror sent a mutation: %s", req.Query)
			http.Error(w, "read-only", 400)
		case strings.Contains(req.Query, "query Issues("):
			_, _ = w.Write([]byte(pick("issues")))
		case strings.Contains(req.Query, "query Teams"):
			_, _ = w.Write([]byte(pick("teams")))
		case strings.Contains(req.Query, "query TeamCycles"):
			_, _ = w.Write([]byte(pick("cycles")))
		default:
			t.Errorf("unexpected graphql document: %s", req.Query)
			http.Error(w, "no", 400)
		}
	}))
}

func teamsBody(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"teams": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
				"nodes": []map[string]any{{
					"id": cycleTeamUUID, "key": "FIX", "name": "Fixture Team", "private": false,
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func cyclesBody(t *testing.T, cycles ...map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"team": map[string]any{
				"id": cycleTeamUUID, "key": "FIX", "name": "Fixture Team",
				"cycles": map[string]any{
					"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					"nodes":    cycles,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sprintColumns reads one issue's sprint projection as stored.
func sprintColumns(t *testing.T, db *mirror, key string) (id sql.NullInt64, name, state sql.NullString) {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.QueryRow(`SELECT sprint_id, sprint_name, sprint_state FROM issues WHERE key = ?`, key).
		Scan(&id, &name, &state); err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	return
}

type sprintRow struct {
	id, boardID                     int64
	name, goal, state               string
	startAt, endAt, completeAt, ext string
	extValid, startValid, endValid  bool
	completeValid                   bool
}

func sprintByExternalID(t *testing.T, db *mirror, ext string) sprintRow {
	t.Helper()
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var r sprintRow
	var start, end, complete sql.NullString
	err = conn.QueryRow(`
		SELECT id, board_id, name, COALESCE(goal, ''), state, start_at, end_at, complete_at, external_id
		FROM sprints WHERE source_id = ? AND external_id = ?`, LinearSourceID, ext).
		Scan(&r.id, &r.boardID, &r.name, &r.goal, &r.state, &start, &end, &complete, &r.ext)
	if err != nil {
		t.Fatalf("sprints row external_id=%s: %v", ext, err)
	}
	r.startValid, r.startAt = start.Valid, start.String
	r.endValid, r.endAt = end.Valid, end.String
	r.completeValid, r.completeAt = complete.Valid, complete.String
	return r
}

// TestRunLinearMirrorsCyclesIntoSprints: one full Linear pass reads the
// issue's cycle into sprint_id/sprint_name/sprint_state and the cycles
// listing into the sprints and boards tables — the derived integer for both,
// the UUID verbatim in sprints.external_id, and the state trio derived from
// the cycle's dates at sync time. A cycle the listing does not carry (another
// team's cycle on an in-scope issue) keeps its issue-side projection — honest
// absence, the same rule as a Jira board the credential cannot read.
//
// FAIL-first on the unmodified tree: sprint_id was NULL for every Linear
// issue (MAPPING.md called Cycle unmapped), and the sprints/boards tables
// held no 'linear' rows at all.
func TestRunLinearMirrorsCyclesIntoSprints(t *testing.T) {
	now := time.Now().UTC()
	const (
		activeUUID = "00000000-0000-4000-8000-0000000000c1"
		futureUUID = "00000000-0000-4000-8000-0000000000c2"
		closedUUID = "00000000-0000-4000-8000-0000000000c0"
		orphanUUID = "00000000-0000-4000-8000-0000000000c9"
	)
	activeStart, activeEnd := linearStamp(now.Add(-72*time.Hour)), linearStamp(now.Add(7*24*time.Hour))
	futureStart, futureEnd := linearStamp(now.Add(14*24*time.Hour)), linearStamp(now.Add(28*24*time.Hour))
	closedStart, closedEnd := linearStamp(now.Add(-40*24*time.Hour)), linearStamp(now.Add(-20*24*time.Hour))
	closedComplete := linearStamp(now.Add(-19 * 24 * time.Hour))
	orphanStart := linearStamp(now.Add(30 * 24 * time.Hour))

	issues := linearIssuesResponse(t, []map[string]any{
		linearNode("FIX-101", "1", "started", "In Progress", 1, "Urgent", map[string]any{
			"cycle": cycleRef(activeUUID, 12, "Cycle 12", activeStart, activeEnd, ""),
		}),
		linearNode("FIX-102", "2", "started", "In Progress", 1, "Urgent", map[string]any{
			"cycle": nil,
		}),
		linearNode("FIX-103", "3", "started", "In Progress", 1, "Urgent", map[string]any{
			"cycle": cycleRef(futureUUID, 13, "Cycle 13", futureStart, futureEnd, ""),
		}),
		linearNode("FIX-104", "4", "started", "In Progress", 1, "Urgent", map[string]any{
			// A cycle the workspace's team listing does not carry: the
			// issue-side projection is all the mirror gets.
			"cycle": cycleRef(orphanUUID, 99, "Orphan 99", orphanStart, linearStamp(now.Add(44*24*time.Hour)), ""),
		}),
	})
	cycles := cyclesBody(t,
		cycleNode(activeUUID, 12, "Cycle 12", "ship the cycles feature", activeStart, activeEnd, ""),
		cycleNode(futureUUID, 13, "Cycle 13", "", futureStart, futureEnd, ""),
		cycleNode(closedUUID, 11, "Cycle 11", "done last month", closedStart, closedEnd, closedComplete),
	)
	teams := teamsBody(t)
	srv := cyclesStub(t, map[string]*string{
		"issues": &issues, "teams": &teams, "cycles": &cycles,
	})
	t.Cleanup(srv.Close)

	db := newMirror(t)
	if _, err := RunLinear(context.Background(), linearTestConfig(), db.DB, Options{
		Full: true, LinearClient: testLinearClient(t, srv),
	}); err != nil {
		t.Fatal(err)
	}

	assertIssue := func(key, uuid, name, state string) {
		t.Helper()
		id, gotName, gotState := sprintColumns(t, db, key)
		want := fnvSprintID(uuid)
		if !id.Valid || id.Int64 != want {
			t.Errorf("%s sprint_id = %v, want the derived id %d of cycle %s", key, id, want, uuid)
		}
		if !gotName.Valid || gotName.String != name {
			t.Errorf("%s sprint_name = %v, want %q", key, gotName, name)
		}
		if !gotState.Valid || gotState.String != state {
			t.Errorf("%s sprint_state = %v, want %q — the state comes from the cycle's dates at sync time", key, gotState, state)
		}
	}
	assertIssue("FIX-101", activeUUID, "Cycle 12", "active")
	assertIssue("FIX-103", futureUUID, "Cycle 13", "future")
	assertIssue("FIX-104", orphanUUID, "Orphan 99", "future")
	id, _, _ := sprintColumns(t, db, "FIX-102")
	if id.Valid {
		t.Errorf("FIX-102 sprint_id = %d, want NULL — an issue with no cycle has no sprint", id.Int64)
	}

	// The listing: three cycles, the UUID verbatim, the same date-derived
	// state, and the goal from the cycle's description.
	active := sprintByExternalID(t, db, activeUUID)
	if active.id != fnvSprintID(activeUUID) {
		t.Errorf("sprints.id = %d, want the derived id %d", active.id, fnvSprintID(activeUUID))
	}
	if active.boardID != fnvSprintID(cycleTeamUUID) {
		t.Errorf("sprints.board_id = %d, want the team's derived id %d", active.boardID, fnvSprintID(cycleTeamUUID))
	}
	if active.state != "active" {
		t.Errorf("active cycle state = %q, want active", active.state)
	}
	if !active.startValid || active.startAt != activeStart {
		t.Errorf("active cycle start_at = %q (valid=%v), want %q", active.startAt, active.startValid, activeStart)
	}
	if !active.endValid || active.endAt != activeEnd {
		t.Errorf("active cycle end_at = %q (valid=%v), want %q", active.endAt, active.endValid, activeEnd)
	}
	if active.completeValid {
		t.Errorf("active cycle complete_at = %q, want NULL", active.completeAt)
	}
	if active.goal != "ship the cycles feature" {
		t.Errorf("active cycle goal = %q, want the cycle description", active.goal)
	}
	if got := sprintByExternalID(t, db, futureUUID); got.state != "future" {
		t.Errorf("future cycle state = %q, want future", got.state)
	}
	if got := sprintByExternalID(t, db, closedUUID); got.state != "closed" || !got.completeValid || got.completeAt != closedComplete {
		t.Errorf("closed cycle = state %q complete_at %q (valid=%v), want closed / %q", got.state, got.completeAt, got.completeValid, closedComplete)
	}

	// One board per team, typed "cycles", keyed by the same derive.
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var boardID int64
	var boardName, boardType, boardKey string
	if err := conn.QueryRow(`SELECT id, name, type, COALESCE(project_key, '') FROM boards WHERE source_id = ?`, LinearSourceID).
		Scan(&boardID, &boardName, &boardType, &boardKey); err != nil {
		t.Fatalf("boards row: %v", err)
	}
	if boardID != fnvSprintID(cycleTeamUUID) || boardName != "Fixture Team" || boardType != "cycles" || boardKey != "FIX" {
		t.Errorf("board = (%d, %q, %q, %q), want (%d, %q, %q, %q)",
			boardID, boardName, boardType, boardKey,
			fnvSprintID(cycleTeamUUID), "Fixture Team", "cycles", "FIX")
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM sprints WHERE source_id = ?`, LinearSourceID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("sprints rows for linear = %d, want 3 (the orphan cycle is issue-side only)", n)
	}
}

// TestRunLinearCycleStateHealsOnQuietTick: completing a cycle moves no issue
// updatedAt — Linear only stamps the cycle's completedAt — so the next
// incremental tick is quiet. The cycles listing is the sole observation path
// the state change has; it runs on every tick and ReplaceAgile re-derives
// sprint_state from the sprints row inside the same transaction (GDK-1661's
// rule, now on the Linear side too).
//
// FAIL-first on the unmodified tree: the pre-change mirror held no sprints
// rows at all, so the phase-one assertions fired first.
func TestRunLinearCycleStateHealsOnQuietTick(t *testing.T) {
	now := time.Now().UTC()
	const cycleUUID = "00000000-0000-4000-8000-0000000000d1"
	startsAt, endsAt := linearStamp(now.Add(-72*time.Hour)), linearStamp(now.Add(7*24*time.Hour))
	completedAt := linearStamp(now.Add(-time.Hour))

	openIssues := linearIssuesResponse(t, []map[string]any{
		linearNode("FIX-201", "1", "started", "In Progress", 1, "Urgent", map[string]any{
			"cycle": cycleRef(cycleUUID, 21, "Cycle 21", startsAt, endsAt, ""),
		}),
	})
	quietIssues := linearIssuesResponse(t, []map[string]any{})
	openCycles := cyclesBody(t, cycleNode(cycleUUID, 21, "Cycle 21", "", startsAt, endsAt, ""))
	doneCycles := cyclesBody(t, cycleNode(cycleUUID, 21, "Cycle 21", "", startsAt, endsAt, completedAt))
	teams := teamsBody(t)

	bodies := map[string]*string{"issues": &openIssues, "teams": &teams, "cycles": &openCycles}
	srv := cyclesStub(t, bodies)
	t.Cleanup(srv.Close)

	db := newMirror(t)
	cfg := linearTestConfig()
	client := testLinearClient(t, srv)
	if _, err := RunLinear(context.Background(), cfg, db.DB, Options{Full: true, LinearClient: client}); err != nil {
		t.Fatal(err)
	}
	if _, name, state := sprintColumns(t, db, "FIX-201"); !state.Valid || state.String != "active" || !name.Valid || name.String != "Cycle 21" {
		t.Fatalf("after the full pass sprint = (name %v, state %v), want (\"Cycle 21\", \"active\")", name, state)
	}

	// The origin completes the cycle; no issue's updatedAt moves.
	bodies["issues"], bodies["cycles"] = &quietIssues, &doneCycles
	res, err := RunLinear(context.Background(), cfg, db.DB, Options{LinearClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Full || res.Fetched != 0 || res.Changed != 0 {
		t.Fatalf("the tick was not quiet: %+v", res)
	}
	if got := sprintByExternalID(t, db, cycleUUID); got.state != "closed" {
		t.Errorf("sprints.state = %q, want closed — a cycle completing is invisible to the issue watermark, so every tick must still list cycles", got.state)
	}
	if _, _, state := sprintColumns(t, db, "FIX-201"); !state.Valid || state.String != "closed" {
		t.Errorf("issues.sprint_state = %v, want closed — the column must be derived from the sprints row the cycles listing just wrote", state)
	}
}

// TestRefreshAgileLinearDispatchesToCycles: after a sprint write on a Linear
// workspace, the re-read must be a Linear one — cycles, not the Jira Agile
// API (the refresh dispatches on src the way refreshIssue does). The config
// is Linear-only: a wrong dispatch builds a Jira client with no Atlassian
// credential and errors instead of reaching the stub, so the sprints row at
// the end proves the right origin was asked.
func TestRefreshAgileLinearDispatchesToCycles(t *testing.T) {
	now := time.Now().UTC()
	const cycleUUID = "00000000-0000-4000-8000-0000000000e1"
	cycles := cyclesBody(t, cycleNode(cycleUUID, 31, "Cycle 31", "", linearStamp(now.Add(-72*time.Hour)), linearStamp(now.Add(7*24*time.Hour)), ""))
	teams := teamsBody(t)
	// No issues body on purpose: the refresh must not walk the issue listing,
	// and the stub reports any document it was not told to expect.
	srv := cyclesStub(t, map[string]*string{"teams": &teams, "cycles": &cycles})
	t.Cleanup(srv.Close)

	prev := origin.LinearEndpoint
	origin.LinearEndpoint = srv.URL
	t.Cleanup(func() { origin.LinearEndpoint = prev })

	cfg := &config.Config{Linear: &config.LinearConfig{APIKey: "test-key"}}
	db := newMirror(t)
	// The precondition every real refresh carries: the mirror has synced
	// once (the sprint verb's id came from rows only a sync writes), and the
	// pass's UpsertSource registered the source. boards.source_id references
	// sources, so seeding it here models the mirror the verb actually runs
	// against — a never-synced mirror has no sprint rows to refresh.
	conn, err := sql.Open("sqlite", "file:"+db.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO sources (id, kind) VALUES (?, ?)`, LinearSourceID, KindLinear); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	conn.Close()
	if err := RefreshAgile(context.Background(), cfg, db.DB, LinearSourceID); err != nil {
		t.Fatal(err)
	}
	if got := sprintByExternalID(t, db, cycleUUID); got.state != "active" || got.name != "Cycle 31" {
		t.Errorf("after the refresh sprints row = (%q, %q), want (\"Cycle 31\", \"active\")", got.name, got.state)
	}
}
