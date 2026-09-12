package mcp

// GDK-1826: the agent's sprint answer is the CLI's answer.
//
// The point of the tool is that it is a third CALLER of store.SprintBurnup,
// not a third reconstruction. What makes that checkable rather than a claim
// in a comment is the shape: `gadak sprint show --json` prints
// {"burnup": <BurnupDoc>, "has_history": bool} and so does this, with the
// same no-history rule (retro.OriginSuppliesChangelog owns it, and a sprint
// it says no to comes back with no days rather than a row of zeros —
// GDK-1679).
//
// If someone later computes the series here instead, or softens the
// no-history case into zeros, these go red.

import (
	"context"
	gosql "database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"

	_ "modernc.org/sqlite" // same pure-Go driver the store opens with
)

// sprintServer is retroServer's sibling with one sprint row seeded: the
// handler must reach a real store, because what is under test is that it
// CALLS store.SprintBurnup rather than building a series of its own.
func sprintServer(t *testing.T, sourceKind string) *Server {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
	dbPath := filepath.Join(home, "gadak.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{
		ID: sourceKind, Kind: sourceKind, BaseURL: "https://example.invalid",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// The board and sprint rows go in through the file: store keeps its
	// writer unexported, and seeding a fixture is not a reason to widen it.
	raw, err := gosql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO boards (source_id, id, name, type, project_key)
		VALUES (?, 1, 'Team board', 'scrum', '')`, sourceKind); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sprints
		(source_id, id, board_id, name, goal, state, start_at, end_at, external_id)
		VALUES (?, 7, 1, 'Sprint 7', '', 'active', '2026-09-01T00:00:00Z', '2026-09-15T00:00:00Z', '7')`,
		sourceKind); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	return &Server{DBPath: dbPath}
}

func callSprint(t *testing.T, s *Server, args map[string]any) (map[string]any, error) {
	t.Helper()
	if err := s.ensureDB(); err != nil {
		t.Fatal(err)
	}
	items, err := s.toolSprint(args)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		t.Fatalf("%s returned %d content items, want 1", toolSprint, len(items))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(items[0].Text), &out); err != nil {
		t.Fatalf("%s payload is not JSON: %v\n%s", toolSprint, err, items[0].Text)
	}
	return out, nil
}

func TestSprintToolAnswersTheCLIDocument(t *testing.T) {
	s := sprintServer(t, "jira")
	out, err := callSprint(t, s, map[string]any{"id": 7})
	if err != nil {
		t.Fatal(err)
	}
	// The two keys `gadak sprint show --json` prints, and only those.
	for _, k := range []string{"burnup", "has_history"} {
		if _, ok := out[k]; !ok {
			t.Errorf("result has no %q — the CLI's --json prints it, so an agent reading both surfaces sees two answers", k)
		}
	}
	if len(out) != 2 {
		t.Errorf("result has %d keys, the CLI prints 2", len(out))
	}
	if out["has_history"] != true {
		t.Errorf("a jira sprint keeps a changelog; has_history = %v", out["has_history"])
	}
	doc, _ := out["burnup"].(map[string]any)
	if doc["name"] != "Sprint 7" || doc["state"] != "active" {
		t.Errorf("burnup is not the seeded sprint: %v", doc)
	}
}

// GDK-1679's rule, on this surface: an origin with no change history gets the
// sentence and no days, never a row of zeros that reads as a quiet sprint.
func TestSprintToolRefusesZerosWhereThereIsNoHistory(t *testing.T) {
	s := sprintServer(t, "linear")
	out, err := callSprint(t, s, map[string]any{"id": 7})
	if err != nil {
		t.Fatal(err)
	}
	if out["has_history"] != false {
		t.Fatalf("linear supplies no changelog; has_history = %v", out["has_history"])
	}
	doc, _ := out["burnup"].(map[string]any)
	if days, ok := doc["days"]; ok && days != nil {
		t.Errorf("no-history sprint carries days %v — a flat line reads as a sprint where nothing happened", days)
	}
}

// The id is required and the refusal names a door an agent actually has.
func TestSprintToolRefusesAMissingID(t *testing.T) {
	s := sprintServer(t, "jira")
	if _, err := callSprint(t, s, map[string]any{}); err == nil {
		t.Fatal("a call with no id was accepted")
	} else if !strings.Contains(err.Error(), "gadak_query") {
		t.Errorf("refusal does not name the surface that has the ids: %v", err)
	}
}

func TestSprintToolIsAdvertisedAndCarriesFreshness(t *testing.T) {
	def, ok := lookupTool(toolSprint)
	if !ok {
		t.Fatalf("%s is not in tools/list — a tool the dispatch knows and the list does not is unreachable", toolSprint)
	}
	// The notice's two sides read one owner (GDK-1813); this tool reads the
	// mirror, so both sides must say so.
	if !carriesFreshnessNotice(toolSprint) {
		t.Errorf("%s reads the mirror but carriesFreshnessNotice says it emits no staleness notice", toolSprint)
	}
	if !strings.Contains(def.Description, "Mirror freshness:") {
		t.Errorf("%s emits the freshness notice but its description does not announce it", toolSprint)
	}
	// The recourse an agent can actually take: it has no shell, so the
	// description must not send it to a CLI verb for the ids.
	if !strings.Contains(def.Description, "gadak_query") {
		t.Errorf("%s must name the surface an agent can use to find sprint ids, not a CLI verb", toolSprint)
	}
}
