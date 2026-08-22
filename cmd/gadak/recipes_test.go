package main

import (
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func recipesDemoHome(t *testing.T) {
	t.Helper()
	sqlDemoHome(t)
	path, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	mig, err := store.Open(path)
	if err != nil {
		t.Fatalf("migrate demo copy: %v", err)
	}
	if err := mig.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecipesSaveRejectsBrokenSQL(t *testing.T) {
	recipesDemoHome(t)
	out, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "broken", "select nope from not_a_table"})
	})
	if err == nil {
		t.Fatalf("broken SQL must refuse save, stdout %q", out)
	}
	if !strings.Contains(err.Error(), "no such table") && !strings.Contains(err.Error(), "no such column") {
		t.Fatalf("want the sqlite error, got %v", err)
	}
	_, showErr := capture(t, func() error { return cmdRecipes([]string{"show", "broken"}) })
	if showErr == nil {
		t.Fatal("broken recipe was stored")
	}
}

func TestRecipesRunMatchesSQL(t *testing.T) {
	recipesDemoHome(t)
	sqlOut, err := capture(t, func() error { return cmdSQL([]string{nextRecipeSQL}) })
	if err != nil {
		t.Fatalf("sql: %v\n%s", err, sqlOut)
	}
	if _, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "next", nextRecipeSQL})
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	runOut, err := capture(t, func() error { return cmdRecipes([]string{"run", "next"}) })
	if err != nil {
		t.Fatalf("run: %v\n%s", err, runOut)
	}
	if runOut != sqlOut {
		t.Fatalf("recipes run next must match gadak sql\nsql:\n%s\nrun:\n%s", sqlOut, runOut)
	}
	jsonSQL, err := capture(t, func() error { return cmdSQL([]string{"--json", nextRecipeSQL}) })
	if err != nil {
		t.Fatalf("sql --json: %v", err)
	}
	jsonRun, err := capture(t, func() error { return cmdRecipes([]string{"run", "next", "--json"}) })
	if err != nil {
		t.Fatalf("run --json: %v", err)
	}
	if jsonRun != jsonSQL {
		t.Fatalf("--json mismatch\nsql:\n%s\nrun:\n%s", jsonSQL, jsonRun)
	}
}

func TestRecipesShowRoundtrip(t *testing.T) {
	recipesDemoHome(t)
	if _, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "next", nextRecipeSQL})
	}); err != nil {
		t.Fatal(err)
	}
	shown, err := capture(t, func() error { return cmdRecipes([]string{"show", "next"}) })
	if err != nil {
		t.Fatalf("show: %v\n%s", err, shown)
	}
	if strings.TrimSpace(shown) != nextRecipeSQL {
		t.Fatalf("show body = %q, want %q", shown, nextRecipeSQL)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(shown)
		_ = w.Close()
	}()
	savedIn := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = savedIn }()
	if _, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "copy", "-m", "-"})
	}); err != nil {
		t.Fatalf("save from show: %v", err)
	}
	copyShown, err := capture(t, func() error { return cmdRecipes([]string{"show", "copy"}) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(copyShown) != strings.TrimSpace(shown) {
		t.Fatalf("roundtrip lost SQL\nshow:\n%s\ncopy:\n%s", shown, copyShown)
	}
}

func TestNextMissingPrintsSaveCommand(t *testing.T) {
	recipesDemoHome(t)
	_, err := capture(t, func() error { return cmdNext(nil) })
	if err == nil {
		t.Fatal("next with no recipe must error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `gadak recipes save next "`+nextRecipeSQL+`"`) {
		t.Fatalf("missing save command in error:\n%s", msg)
	}
}

func TestNextRunsSavedRecipe(t *testing.T) {
	recipesDemoHome(t)
	if _, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "next", nextRecipeSQL})
	}); err != nil {
		t.Fatal(err)
	}
	sqlOut, err := capture(t, func() error { return cmdSQL([]string{nextRecipeSQL}) })
	if err != nil {
		t.Fatal(err)
	}
	nextOut, err := capture(t, func() error { return cmdNext(nil) })
	if err != nil {
		t.Fatalf("next: %v\n%s", err, nextOut)
	}
	if nextOut != sqlOut {
		t.Fatalf("gadak next must match gadak sql\nsql:\n%s\nnext:\n%s", sqlOut, nextOut)
	}
}

func TestNextRefusesLimitArg(t *testing.T) {
	recipesDemoHome(t)
	_, err := capture(t, func() error { return cmdNext([]string{"5"}) })
	if err == nil {
		t.Fatal("next 5 must be usage, not a LIMIT rewrite")
	}
	if !strings.Contains(err.Error(), nextUsage) {
		t.Fatalf("want %q, got %v", nextUsage, err)
	}
}

func TestRecipesRmAndMissingListsNames(t *testing.T) {
	recipesDemoHome(t)
	if _, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "night-triage", "select 1 as n"})
	}); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error { return cmdRecipes([]string{"run", "nope"}) })
	if err == nil {
		t.Fatal("run missing name must error")
	}
	if !strings.Contains(err.Error(), "night-triage") {
		t.Fatalf("missing-name error must list saved recipes, got %v", err)
	}
	if _, err := capture(t, func() error { return cmdRecipes([]string{"rm", "night-triage"}) }); err != nil {
		t.Fatal(err)
	}
	list, err := capture(t, func() error { return cmdRecipes(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(list, "night-triage") {
		t.Fatalf("rm left the row in the list:\n%s", list)
	}
	first, _, _ := strings.Cut(list, "\n")
	if first != "name\tupdated_at\tsql" {
		t.Fatalf("list header = %q", first)
	}
}

func TestRecipesListFoldsSQL(t *testing.T) {
	recipesDemoHome(t)
	long := "select\n1 as n, 2 as m, 3 as o, 4 as p, 5 as q, 6 as r, 7 as s, 8 as t, 9 as u, 10 as v"
	if _, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "fold-me", long})
	}); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error { return cmdRecipes([]string{"list"}) })
	if err != nil {
		t.Fatal(err)
	}
	var data string
	for _, ln := range strings.Split(out, "\n") {
		if strings.HasPrefix(ln, "fold-me\t") {
			data = ln
			break
		}
	}
	if data == "" {
		t.Fatalf("list missing fold-me:\n%s", out)
	}
	cols := strings.Split(data, "\t")
	if len(cols) != 3 {
		t.Fatalf("list columns = %d (%q), want 3", len(cols), data)
	}
	if strings.ContainsAny(cols[2], "\n\r") {
		t.Fatalf("sql preview is not one line: %q", cols[2])
	}
	if len([]rune(cols[2])) > 60 {
		t.Fatalf("sql preview is %d runes, want <= 60: %q", len([]rune(cols[2])), cols[2])
	}
}

func TestRecipesNoSeed(t *testing.T) {
	recipesDemoHome(t)
	out, err := capture(t, func() error { return cmdRecipes(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "next") {
		t.Fatalf("must not seed a next recipe:\n%s", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 1 || lines[0] != "name\tupdated_at\tsql" {
		t.Fatalf("empty list must be the header alone, got %q", out)
	}
}

func TestHelpNamesDisplayTrapAndPickRecipe(t *testing.T) {
	for _, cmd := range []string{"sql", "search", "recipes"} {
		h := formatHelp(cmd, nil)
		if !strings.Contains(h, "status_category") || !strings.Contains(h, "priority_rank") || !strings.Contains(h, "issue_type_id") {
			t.Errorf("%s --help missing the display-name trap:\n%s", cmd, h)
		}
		if !strings.Contains(h, "In Progress") {
			t.Errorf("%s --help missing the In Progress 0-row example:\n%s", cmd, h)
		}
	}
	recipes := formatHelp("recipes", nil)
	if !strings.Contains(recipes, nextRecipeSQL) {
		t.Fatalf("recipes --help must quote nextRecipeSQL, got:\n%s", recipes)
	}
	next := formatHelp("next", nil)
	if !strings.Contains(next, "origin write") && !strings.Contains(next, "occupancy") {
		t.Fatalf("next --help must say this is not occupancy:\n%s", next)
	}
	if !strings.Contains(next, nextRecipeSQL) {
		t.Fatalf("next --help must quote the save command:\n%s", next)
	}
}

func TestRecipesSaveZeroRowsIsSuccess(t *testing.T) {
	recipesDemoHome(t)
	empty := "select key from issues_full where 1=0"
	out, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "empty-ok", empty})
	})
	if err != nil {
		t.Fatalf("0-row SELECT must save, got %v\n%s", err, out)
	}
	shown, err := capture(t, func() error { return cmdRecipes([]string{"show", "empty-ok"}) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(shown) != empty {
		t.Fatalf("stored %q, want %q", shown, empty)
	}
}

func TestRecipesSaveWriteIsRefused(t *testing.T) {
	recipesDemoHome(t)
	_, err := capture(t, func() error {
		return cmdRecipes([]string{"save", "mutate", "update issues set key = key"})
	})
	if err == nil {
		t.Fatal("UPDATE must not save")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "readonly") &&
		!strings.Contains(strings.ToLower(err.Error()), "read-only") &&
		!strings.Contains(err.Error(), "attempt to write") {
		t.Fatalf("want readonly write error, got %v", err)
	}
}
