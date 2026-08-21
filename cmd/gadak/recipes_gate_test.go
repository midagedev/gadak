package main

// GDK-108 gate. docs/RECIPES.md is one of the three 0.x contracts
// (issues_full + the recipe queries), and until now nothing kept it honest:
// a schema rename would rot every fence silently. This gate reuses the
// package's existing in-process patterns — sqlDemoHome (sql_test.go),
// capture (agent_test.go), stubViewsLaunchSeams (views_test.go) — so no new
// helper machinery and no subprocesses are introduced.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// minSQLFences guards against the extractor quietly going blind: if the doc
// stops yielding fences, the gate must fail instead of vacuously passing.
// A lower bound, not an equality — adding a recipe must not break the gate.
const minSQLFences = 14

func recipesDoc(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "RECIPES.md"))
	if err != nil {
		t.Fatalf("read docs/RECIPES.md: %v", err)
	}
	return string(raw)
}

// extractSQLFences returns the body of every ```sql fence in document order.
func extractSQLFences(t *testing.T, doc string) []string {
	t.Helper()
	var fences []string
	var cur []string
	inFence := false
	for _, ln := range strings.Split(doc, "\n") {
		switch {
		case !inFence && strings.TrimSpace(ln) == "```sql":
			inFence, cur = true, nil
		case inFence && strings.TrimSpace(ln) == "```":
			fences = append(fences, strings.Join(cur, "\n"))
			inFence = false
		case inFence:
			cur = append(cur, ln)
		}
	}
	if inFence {
		t.Fatal("docs/RECIPES.md ends inside an unterminated ```sql fence")
	}
	return fences
}

// TestRecipesSQLFencesRunOnDemoDB executes every recipe exactly the way
// RECIPES.md promises they run ("Every recipe runs as-is with `gadak sql`",
// docs/RECIPES.md:3): through cmdSQL against a throwaway home seeded from
// examples/demo.db, which store.OpenReadOnly opens mode=ro. Execution is the
// contract — zero rows are fine (fences 6, 12, 13 return 0 rows on today's
// demo snapshot); a SQLite error is the rot this gate exists to catch.
func TestRecipesSQLFencesRunOnDemoDB(t *testing.T) {
	fences := extractSQLFences(t, recipesDoc(t))
	if len(fences) < minSQLFences {
		t.Fatalf("docs/RECIPES.md carries %d ```sql fences, want >= %d — the fence extractor or the doc regressed", len(fences), minSQLFences)
	}
	sqlDemoHome(t)
	// Open the throwaway copy so this binary's migrations apply (v30 sprint
	// columns). examples/demo.db itself is not written; cmdSQL is read-only
	// and would otherwise fail on a snapshot whose user_version lags.
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	mig, err := store.Open(path)
	if err != nil {
		t.Fatalf("migrate demo copy: %v", err)
	}
	mig.Close()
	for i, query := range fences {
		t.Run(fmt.Sprintf("fence%02d", i+1), func(t *testing.T) {
			_, err := capture(t, func() error { return cmdSQL([]string{query}) })
			if err != nil {
				t.Fatalf("fence %d of docs/RECIPES.md fails against examples/demo.db: %v\nquery first line: %s", i+1, err, firstLine(query))
			}
		})
	}
}

// pipeQueryFromRecipes anchors the pipe test to a recipe that actually
// exists in the doc, then applies the projection RECIPES.md itself prescribes
// for piping ("Select only `key`: `--keys` splits on commas and whitespace",
// docs/RECIPES.md:170-171). Wrapping is necessary: the doc's only verbatim
// single-key query — the label-'batch' example under "Show on the app" —
// returns 0 rows on examples/demo.db (that label exists only in the unit
// mirror fixture), so it cannot back an order assertion.
func pipeQueryFromRecipes(t *testing.T, fences []string) string {
	t.Helper()
	for _, f := range fences {
		if strings.Contains(f, "select key, summary, created_at from issues_full") {
			return "select key from (" + f + ")"
		}
	}
	t.Fatal("no RECIPES fence containing `select key, summary, created_at from issues_full` — repoint the pipe gate at a key-producing recipe")
	return ""
}

// TestWeeklyReopenScriptQuotesFirstRecipe keeps examples/compose/
// weekly-reopen.sh welded to the first RECIPES fence it quotes (GDK-109):
// the script is a copy of a contract surface, and without this check a
// recipe revision would rot it silently.
func TestWeeklyReopenScriptQuotesFirstRecipe(t *testing.T) {
	fences := extractSQLFences(t, recipesDoc(t))
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "compose", "weekly-reopen.sh"))
	if err != nil {
		t.Fatalf("read examples/compose/weekly-reopen.sh: %v", err)
	}
	if !strings.Contains(string(raw), strings.TrimSpace(fences[0])) {
		t.Fatalf("examples/compose/weekly-reopen.sh no longer quotes the first docs/RECIPES.md query verbatim:\n%s", strings.TrimSpace(fences[0]))
	}
}

// TestRecipesSQLToViewsOpenPipePreservesKeyOrder gates the documented pipe
// (docs/RECIPES.md:176-181): `gadak sql --no-header '<key query>' | gadak
// views open --keys -` must present the same keys in the same order —
// "--keys keeps first-seen order, so the ORDER BY is what the list shows"
// (docs/RECIPES.md:169). In-process, via cmdSQL and cmdViews directly.
func TestRecipesSQLToViewsOpenPipePreservesKeyOrder(t *testing.T) {
	query := pipeQueryFromRecipes(t, extractSQLFences(t, recipesDoc(t)))
	sqlDemoHome(t)

	sqlOut, err := capture(t, func() error { return cmdSQL([]string{"--no-header", query}) })
	if err != nil {
		t.Fatalf("gadak sql --no-header: %v\n%s", err, sqlOut)
	}
	var keys []string
	for _, ln := range strings.Split(sqlOut, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			keys = append(keys, ln)
		}
	}
	if len(keys) < 2 {
		t.Fatalf("the piped recipe must yield >= 2 keys for an order assertion to mean anything, got %d: %q", len(keys), sqlOut)
	}

	// Empty discovery so a live gadak serve on this machine cannot touch the
	// run (same reason as TestViewsOpenNoOpenSkipsLaunch); the swapped
	// launchers also fail the test if --no-open ever launches anything.
	stubViewsLaunchSeams(t, nil)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(sqlOut)
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = saved }()
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "--keys", "-"})
	})
	if err != nil {
		t.Fatalf("views open --keys -: %v\n%s", err, out)
	}
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if len(body.Keys) != len(keys) {
		t.Fatalf("pipe lost keys: sql produced %d %v, views open parsed %d %v", len(keys), keys, len(body.Keys), body.Keys)
	}
	for i := range keys {
		if body.Keys[i] != keys[i] {
			t.Fatalf("pipe order broken at %d: sql produced %v, views open parsed %v", i, keys, body.Keys)
		}
	}
}

// GDK-522: demo.db has custom='{}' on every row, so the custom-field recipes
// cannot be proven there. Plant a temporary mirror and run the same shapes.
func TestCustomFieldRecipesOnPlantedMirror(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.invalid"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"3": "inprogress"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "NMB-1", Title: "mapped custom",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "1",
				Status: "Open", StatusID: "3", StatusCategory: "inprogress",
				Custom: map[string]any{
					"story_points": float64(5),
					"labels_axis":  []any{"alpha", "beta"},
				},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	scalar := `select key, json_extract(custom, '$.story_points') as sp
from issues_full
where json_extract(custom, '$.story_points') is not null`
	arrayQ := `select i.key, je.value
from issues_full i, json_each(i.custom, '$.labels_axis') je`

	scalarOut, err := capture(t, func() error { return cmdSQL([]string{"--json", scalar}) })
	if err != nil {
		t.Fatalf("scalar recipe: %v\n%s", err, scalarOut)
	}
	if !strings.Contains(scalarOut, `"NMB-1"`) || !strings.Contains(scalarOut, `"sp":5`) {
		t.Fatalf("scalar recipe missed the planted value:\n%s", scalarOut)
	}

	arrayOut, err := capture(t, func() error { return cmdSQL([]string{"--json", arrayQ}) })
	if err != nil {
		t.Fatalf("array recipe: %v\n%s", err, arrayOut)
	}
	if !strings.Contains(arrayOut, "alpha") || !strings.Contains(arrayOut, "beta") {
		t.Fatalf("array recipe missed json_each values:\n%s", arrayOut)
	}
}
