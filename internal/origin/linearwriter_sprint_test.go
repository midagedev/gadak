package origin

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"strings"
	"testing"
	"time"
)

// A linearWriter carries the SprintBoard face (GDK-1667): Linear cycles are
// the workspace's sprints — one board per team, the cycle ids derived into
// the sprint integer space — so `gadak sprint add/remove/create` works on a
// Linear workspace, with start/close refused (a cycle starts and ends by its
// dates, not by a verb).
//
// FAIL-first on the unmodified tree: AsSprintBoard answered ErrNoSprints —
// the adapter declared no sprint surface at all.
func TestLinearWriterIsSprintBoard(t *testing.T) {
	w, _ := testLinearWriter(t)
	if _, err := AsSprintBoard(w); err != nil {
		t.Fatalf("AsSprintBoard(linearWriter) = %v, want the sprint face", err)
	}
}

// fnvSprintID is the test-side oracle of the cycle/board UUID → sprint
// integer derive: FNV-1a 64, top bit cleared, 0 → 1. Spelled out here so
// the tests pin the value the mirror stores, independently of whatever
// helper production shares.
func fnvSprintID(uuid string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(uuid))
	id := int64(h.Sum64() &^ (1 << 63))
	if id == 0 {
		return 1
	}
	return id
}

const (
	linearFixtureTeam  = "00000000-0000-4000-8000-000000000003"
	linearActiveCycle  = "00000000-0000-4000-8000-0000000000c1"
	linearCreatedCycle = "00000000-0000-4000-8000-0000000000c2"
)

// issueUpdateInput is the mutation variables of one issueUpdate:
// {"id": …, "input": {…}}. cycleId is decoded as *string so an explicit
// null reads as non-nil-pointing-at-nil.
type issueUpdateVars struct {
	ID    string         `json:"id"`
	Input map[string]any `json:"input"`
}

func issueUpdateInputOf(t *testing.T, raw json.RawMessage) issueUpdateVars {
	t.Helper()
	var v issueUpdateVars
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("issueUpdate vars: %v", err)
	}
	return v
}

func TestLinearMoveToSprintResolvesCycleAndSendsCycleId(t *testing.T) {
	w, rec := testLinearWriter(t)
	if err := w.MoveToSprint(context.Background(), fnvSprintID(linearActiveCycle), []string{"FIX-1"}); err != nil {
		t.Fatal(err)
	}
	if rec.updates != 1 {
		t.Fatalf("issueUpdate ran %d times, want 1", rec.updates)
	}
	in := issueUpdateInputOf(t, rec.lastVars).Input
	if got, _ := in["cycleId"].(string); got != linearActiveCycle {
		t.Errorf("input.cycleId = %v, want the cycle UUID %s", in["cycleId"], linearActiveCycle)
	}
}

func TestLinearMoveToSprintUnknownSprintIDRefuses(t *testing.T) {
	w, rec := testLinearWriter(t)
	err := w.MoveToSprint(context.Background(), 424242, []string{"FIX-1"})
	if err == nil || !strings.Contains(err.Error(), "no cycle with sprint id 424242") {
		t.Fatalf("err = %v, want it to name the unknown sprint id", err)
	}
	if rec.updates != 0 {
		t.Errorf("issueUpdate ran %d times; the refuse must stay local", rec.updates)
	}
}

func TestLinearMoveToBacklogSendsCycleNull(t *testing.T) {
	w, rec := testLinearWriter(t)
	if err := w.MoveToBacklog(context.Background(), []string{"FIX-1"}); err != nil {
		t.Fatal(err)
	}
	if rec.updates != 1 {
		t.Fatalf("issueUpdate ran %d times, want 1", rec.updates)
	}
	in := issueUpdateInputOf(t, rec.lastVars).Input
	v, ok := in["cycleId"]
	if !ok {
		t.Fatalf("input has no cycleId key — omitted means unchanged, the backlog move must send the explicit null: %v", in)
	}
	if v != nil {
		t.Errorf("input.cycleId = %v, want JSON null (explicit un-membership)", v)
	}
}

func TestLinearMoveToSprintScopedListsOnlyConfiguredTeams(t *testing.T) {
	w, _ := testLinearWriter(t)
	scoped := &linearWriter{c: w.c, teams: []string{"00000000-0000-4000-8000-000000000099"}}
	err := scoped.MoveToSprint(context.Background(), fnvSprintID(linearActiveCycle), []string{"FIX-1"})
	if err == nil || !strings.Contains(err.Error(), "no cycle with sprint id") {
		t.Fatalf("err = %v, want the scoped-out refuse", err)
	}
}

func TestLinearCreateSprintCreatesCycle(t *testing.T) {
	w, rec := testLinearWriter(t)
	before := time.Now().UTC()
	s, err := w.CreateSprint(context.Background(), fnvSprintID(linearFixtureTeam), "Cycle 13", "ship it")
	if err != nil {
		t.Fatal(err)
	}
	if rec.cycleCreates != 1 {
		t.Fatalf("cycleCreate ran %d times, want 1", rec.cycleCreates)
	}
	var vars struct {
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(rec.lastCycleVars, &vars); err != nil {
		t.Fatal(err)
	}
	in := vars.Input
	if got, _ := in["teamId"].(string); got != linearFixtureTeam {
		t.Errorf("input.teamId = %v, want the fixture team", in["teamId"])
	}
	if got, _ := in["name"].(string); got != "Cycle 13" {
		t.Errorf("input.name = %v", in["name"])
	}
	if got, _ := in["description"].(string); got != "ship it" {
		t.Errorf("input.description = %v, want the goal", in["description"])
	}
	startsAt, err := time.Parse(time.RFC3339, stringOrEmpty(in["startsAt"]))
	if err != nil {
		t.Fatalf("input.startsAt = %v, not a timestamp", in["startsAt"])
	}
	if !startsAt.After(before) || startsAt.Sub(before) > 25*time.Hour {
		t.Errorf("input.startsAt = %v, want the next UTC midnight after %v", startsAt, before)
	}
	if h, m, s := startsAt.Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("input.startsAt = %v, want a UTC midnight", startsAt)
	}
	endsAt, _ := in["endsAt"].(string)
	wantEnd := startsAt.Add(14 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	if endsAt != wantEnd {
		t.Errorf("input.endsAt = %v, want startsAt+14d %s", in["endsAt"], wantEnd)
	}
	// The echo derives the id and the state by the shared rule.
	if s.ID != fnvSprintID(linearCreatedCycle) {
		t.Errorf("echo ID = %d, want the derive of the created cycle", s.ID)
	}
	if s.State != "future" {
		t.Errorf("echo State = %q, want future (dates in the canned answer)", s.State)
	}
}

func TestLinearCreateSprintUnknownBoardRefuses(t *testing.T) {
	w, rec := testLinearWriter(t)
	_, err := w.CreateSprint(context.Background(), 987654, "Cycle 13", "")
	if err == nil || !strings.Contains(err.Error(), "no team with board id 987654") {
		t.Fatalf("err = %v, want it to name the unknown board id", err)
	}
	if rec.cycleCreates != 0 {
		t.Errorf("cycleCreate ran %d times; the refuse must stay local", rec.cycleCreates)
	}
}

func TestLinearUpdateSprintRefusesStateChanges(t *testing.T) {
	w, rec := testLinearWriter(t)
	ctx := context.Background()
	id := fnvSprintID(linearActiveCycle)
	for name, fields := range map[string]map[string]any{
		"state alone": {"state": "active"},
		"state mixed": {"state": "closed", "name": "renamed too"},
	} {
		if _, err := w.UpdateSprint(ctx, id, fields); !errors.Is(err, ErrLinearCycleByDates) {
			t.Errorf("%s: err = %v, want ErrLinearCycleByDates", name, err)
		}
	}
	if rec.cycleUpdates != 0 || rec.updates != 0 {
		t.Errorf("cycleUpdates=%d issueUpdates=%d — the refuse must happen before any mutation leaves", rec.cycleUpdates, rec.updates)
	}
}

func TestLinearUpdateSprintRenamesAndReschedules(t *testing.T) {
	w, rec := testLinearWriter(t)
	_, err := w.UpdateSprint(context.Background(), fnvSprintID(linearActiveCycle), map[string]any{
		"name":      "Sprint X",
		"goal":      "g2",
		"startDate": "2026-08-01T00:00:00.000+09:00",
		"endDate":   "2026-08-15T00:00:00.000+09:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.cycleUpdates != 1 {
		t.Fatalf("cycleUpdate ran %d times, want 1", rec.cycleUpdates)
	}
	var vars struct {
		ID    string         `json:"id"`
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(rec.lastCycleVars, &vars); err != nil {
		t.Fatal(err)
	}
	if vars.ID != linearActiveCycle {
		t.Errorf("cycleUpdate id = %q, want the cycle UUID", vars.ID)
	}
	in := vars.Input
	if got, _ := in["name"].(string); got != "Sprint X" {
		t.Errorf("input.name = %v", in["name"])
	}
	if got, _ := in["description"].(string); got != "g2" {
		t.Errorf("input.description = %v, want the goal", in["description"])
	}
	// The CLI's Jira-format stamps re-express as the same instants in UTC.
	if got, _ := in["startsAt"].(string); got != "2026-07-31T15:00:00.000Z" {
		t.Errorf("input.startsAt = %v, want 2026-07-31T15:00:00.000Z", in["startsAt"])
	}
	if got, _ := in["endsAt"].(string); got != "2026-08-14T15:00:00.000Z" {
		t.Errorf("input.endsAt = %v, want 2026-08-14T15:00:00.000Z", in["endsAt"])
	}
}

func TestLinearUpdateSprintUnknownIDRefuses(t *testing.T) {
	w, rec := testLinearWriter(t)
	_, err := w.UpdateSprint(context.Background(), 424242, map[string]any{"name": "x"})
	if err == nil || !strings.Contains(err.Error(), "no cycle with sprint id 424242") {
		t.Fatalf("err = %v, want it to name the unknown sprint id", err)
	}
	if rec.cycleUpdates != 0 {
		t.Errorf("cycleUpdate ran %d times; the refuse must stay local", rec.cycleUpdates)
	}
}

func stringOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}
