package main

// GDK-108 gate. docs/RECIPES.md is one of the three 0.x contracts
// (issues_full + the recipe queries), and until now nothing kept it honest:
// a schema rename would rot every fence silently. This gate reuses the
// package's existing in-process patterns — sqlDemoHome (sql_test.go),
// capture (agent_test.go), stubViewsLaunchSeams (views_test.go) — so no new
// helper machinery and no subprocesses are introduced.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
