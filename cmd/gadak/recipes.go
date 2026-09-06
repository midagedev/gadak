package main

// gadak recipes — a name for a read-only mirror SQL query, stored in
// local.recipes. gadak next is the thin alias that runs the recipe named
// "next"; when none is saved it falls back to the built-in list default
// (listDefaultSQL in list.go) with a stderr line saying so. Rank comes
// from the mirror (priority_rank, status_category); there is no private
// ranking engine.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/store"
)

// nextRecipeSQL is the help-example priority pick. Quoted by `gadak next`
// when that recipe is missing, and by recipes/next help examples. Keep this
// the single copy. The age column is ageDaysColumn from list.go — the same
// expression the built-in list carries, one owner.
const nextRecipeSQL = `select key, priority_rank, status, ` + ageDaysColumn + `, summary from issues_full where status_category != 'done' order by priority_rank, updated_at desc limit 10`

const (
	recipesUsage     = `usage: gadak recipes [list|save|run|show|rm]`
	recipesSaveUsage = `usage: gadak recipes save NAME ["sql"|-m <text|->]`
	recipesRunUsage  = `usage: gadak recipes run NAME [--json|--csv|--no-header]`
	recipesShowUsage = `usage: gadak recipes show NAME`
	recipesRmUsage   = `usage: gadak recipes rm NAME`
	nextUsage        = `usage: gadak next [--json|--csv|--no-header]`
)

func cmdRecipes(args []string) error {
	if wantsHelp(args) {
		printHelp("recipes")
		return nil
	}
	sub, rest := "list", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list", "save", "run", "show", "rm":
			sub, rest = args[0], args[1:]
		default:
			return usageError("recipes", recipesUsage)
		}
	}
	switch sub {
	case "list":
		return recipesList(rest)
	case "save":
		return recipesSave(rest)
	case "run":
		return recipesRun(rest)
	case "show":
		return recipesShow(rest)
	case "rm":
		return recipesRm(rest)
	default:
		return usageError("recipes", recipesUsage)
	}
}

func cmdNext(args []string) error {
	if wantsHelp(args) {
		printHelp("next")
		return nil
	}
	fs := newFlagSet("next")
	asJSON := fs.Bool("json", false, "emit one JSON object per row")
	asCSV := fs.Bool("csv", false, "emit CSV with a header row")
	noHeader := fs.Bool("no-header", false, "omit the TSV/CSV header row (no-op with --json)")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageError("next", nextUsage)
	}
	out := sqlOutput{JSON: *asJSON, CSV: *asCSV, NoHeader: *noHeader}
	if !hasSavedRecipe("next") {
		return runNextDefault(out)
	}
	return runNamedRecipe("next", out)
}

// hasSavedRecipe reports whether local.recipes holds name. An unreadable
// mirror answers false here so runNextDefault (or runNamedRecipe, for
// `recipes run`) can re-open and surface the real error on its own path.
func hasSavedRecipe(name string) bool {
	db, err := openReadOnly()
	if err != nil {
		return false
	}
	defer db.Close()
	var query string
	return db.QueryRow(`SELECT sql FROM local.recipes WHERE name = ?`, name).Scan(&query) == nil
}

// runNextDefault is the no-recipe path for next/pick: the built-in list
// default plus one stderr line naming the save command. An agent guessing
// verbs used to end here with an error and give up; the question "what is
// next" is answerable from the mirror, so it gets answered (exit 0), and
// the notice keeps the recipe escape hatch visible.
func runNextDefault(out sqlOutput) error {
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	fmt.Fprintf(os.Stderr, "no saved recipe %q — built-in default shown; customize: gadak recipes save next %q\n",
		"next", listDefaultSQL(defaultListLimit))
	warnIfStale(db)
	return writeSQLQuery(db, listDefaultSQL(defaultListLimit), out)
}

func recipesList(args []string) error {
	fs := newFlagSet("recipes")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageError("recipes", recipesUsage)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	list, err := db.Recipes(context.Background())
	if err != nil {
		return err
	}
	fmt.Println("name\tupdated_at\tsql")
	for _, r := range list {
		fmt.Printf("%s\t%s\t%s\n", r.Name, r.UpdatedAt, foldRecipeSQL(r.SQL, 60))
	}
	return nil
}

func recipesSave(args []string) error {
	if wantsHelp(args) {
		printHelp("recipes")
		return nil
	}
	name, query, err := parseRecipeSave(args)
	if err != nil {
		return err
	}
	if err := store.ValidateRecipeName(name); err != nil {
		return err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return errors.New("recipe sql is empty")
	}
	if err := validateRecipeSQL(query); err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	saved, err := db.PutRecipe(context.Background(), name, query)
	if err != nil {
		return err
	}
	fmt.Printf("saved\t%s\n", saved.Name)
	return nil
}

func parseRecipeSave(args []string) (name, query string, err error) {
	var sawM bool
	var words []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-m", "--m":
			if i+1 >= len(args) {
				return "", "", usageError("recipes", recipesSaveUsage)
			}
			i++
			query = args[i]
			sawM = true
		default:
			words = append(words, a)
		}
	}
	if len(words) == 0 {
		return "", "", usageError("recipes", recipesSaveUsage)
	}
	name = words[0]
	if !sawM {
		query = strings.Join(words[1:], " ")
	} else if len(words) > 1 {
		return "", "", usageError("recipes", "recipe sql given twice — positional text and -m; pick one")
	}
	if query == "-" {
		buf, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return "", "", readErr
		}
		query = string(buf)
	}
	if strings.TrimSpace(name) == "" {
		return "", "", usageError("recipes", recipesSaveUsage)
	}
	return name, query, nil
}

func validateRecipeSQL(query string) error {
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	return drainSQLQuery(db, query)
}

func recipesRun(args []string) error {
	if wantsHelp(args) {
		printHelp("recipes")
		return nil
	}
	fs := newFlagSet("recipes")
	asJSON := fs.Bool("json", false, "emit one JSON object per row")
	asCSV := fs.Bool("csv", false, "emit CSV with a header row")
	noHeader := fs.Bool("no-header", false, "omit the TSV/CSV header row (no-op with --json)")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("recipes", recipesRunUsage)
	}
	return runNamedRecipe(pos[0], sqlOutput{JSON: *asJSON, CSV: *asCSV, NoHeader: *noHeader})
}

func runNamedRecipe(name string, out sqlOutput) error {
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	var query string
	err = db.QueryRow(`SELECT sql FROM local.recipes WHERE name = ?`, name).Scan(&query)
	if errors.Is(err, sql.ErrNoRows) {
		return missingRecipe(name, recipeNamesOn(db))
	}
	if err != nil {
		return err
	}
	warnIfStale(db)
	return writeSQLQuery(db, query, out)
}

func recipesShow(args []string) error {
	if wantsHelp(args) {
		printHelp("recipes")
		return nil
	}
	fs := newFlagSet("recipes")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("recipes", recipesShowUsage)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	r, err := db.Recipe(context.Background(), pos[0])
	if errors.Is(err, store.ErrNotFound) {
		names, _ := db.RecipeNames(context.Background())
		return missingRecipe(pos[0], names)
	}
	if err != nil {
		return err
	}
	fmt.Println(r.SQL)
	return nil
}

func recipesRm(args []string) error {
	if wantsHelp(args) {
		printHelp("recipes")
		return nil
	}
	fs := newFlagSet("recipes")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("recipes", recipesRmUsage)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.DeleteRecipe(context.Background(), pos[0]); errors.Is(err, store.ErrNotFound) {
		names, _ := db.RecipeNames(context.Background())
		return missingRecipe(pos[0], names)
	} else if err != nil {
		return err
	}
	return nil
}

func recipeNamesOn(db *sql.DB) []string {
	rows, err := db.Query(`SELECT name FROM local.recipes ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return out
		}
		out = append(out, n)
	}
	return out
}

func missingRecipe(name string, saved []string) error {
	if name == "next" {
		return fmt.Errorf("no recipe named %q — save one:\ngadak recipes save next %q", name, nextRecipeSQL)
	}
	if len(saved) == 0 {
		return fmt.Errorf("no recipe named %q — none saved", name)
	}
	return fmt.Errorf("no recipe named %q — saved: %s", name, strings.Join(saved, ", "))
}

// foldRecipeSQL is the list column: one line, first n characters.
func foldRecipeSQL(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if n > 0 && len(r) > n {
		return string(r[:n])
	}
	return s
}
