package main

// The age_days contract — THEORY.md T4 (age is the risk) and G10 (the agent
// answer carries age) land on the list surface as one derived column.
// Clauses map to the tests that enforce them:
//
//	C1  list TSV header is key priority priority_rank status age_days
//	    updated_at summary; every row has 7 tab-separated fields; the age
//	    cell is empty or a decimal rounded to one fractional digit
//	    → TestListAgeDaysTSVShape
//	C2  --json carries age_days as a number or null; --csv header matches
//	    → TestListAgeDaysJSONAndCSV
//	C3  ready and next (no saved recipe) print the same header as list
//	    → TestReadyAndNextCarryAgeDays
//	C4  nextRecipeSQL and the built-in fallback produce age_days; a saved
//	    recipe is untouched (recipes are user SQL)
//	    → TestNextRecipeSQLCarriesAge, TestSavedRecipeKeepsItsOwnColumns
//	C5  no display-name keying: status_category != done stays, nothing keys
//	    on status or priority names
//	    → TestListSQLKeysOnCategoriesNotDisplayNames
//	C6  SKILL.md carries the When asked what to do next subsection, at most
//	    14 lines, naming age_days / gadak ready / RECIPES.md / THEORY.md and
//	    none of the banned phrases
//	    → TestSkillAnswerShapeGate
//	C7  the doc-check invariants over the surfaces this change edits hold,
//	    and the skill save-command example stays welded to nextRecipeSQL
//	    → TestSkillDocCheckInvariants, TestSkillSaveExampleWeldedToNextRecipeSQL
//
// Rendering note, measured 2026-09-06 through gadak sql: round(x, 1) is a
// REAL and formatSQLRows prints Go float64 with %v, so an integral age
// renders as 12, not 12.0. Always-one-fractional-digit would force
// printf('%.1f'), whose TEXT breaks C2 (age_days must be a JSON number).
// The contract here follows the prescribed round(...,1) expression: a
// decimal with at most one fractional digit, empty when status_changed_at
// is NULL.

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ageDaysCell matches one TSV cell of the age column: empty (NULL
// status_changed_at) or a decimal with at most one fractional digit.
var ageDaysCell = regexp.MustCompile(`^(\d+(\.\d)?)?$`)

const ageColumnIndex = 4

// TestListAgeDaysTSVShape enforces C1 on examples/demo.db — 534 issues, 167
// with NULL status_changed_at and 201 open rows with a timestamp, so the
// default window exercises both the empty and the populated cell.
func TestListAgeDaysTSVShape(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error { return cmdList([]string{"--limit", "10"}) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != listHeader {
		t.Fatalf("header = %q, want %q:\n%s", lines[0], listHeader, out)
	}
	empty, filled := 0, 0
	for _, ln := range lines[1:] {
		f := strings.Split(ln, "\t")
		if len(f) != 7 {
			t.Fatalf("row %q does not have 7 tab-separated fields:\n%s", ln, out)
		}
		if !ageDaysCell.MatchString(f[ageColumnIndex]) {
			t.Fatalf("age_days cell %q is neither empty nor a one-decimal number:\n%s", f[ageColumnIndex], out)
		}
		if f[ageColumnIndex] == "" {
			empty++
		} else {
			filled++
		}
	}
	if filled == 0 {
		t.Fatalf("no row carries a non-empty age_days, but the fixture has 201 open issues with status_changed_at:\n%s", out)
	}
	if empty == 0 {
		t.Fatalf("no row carries an empty age_days, but the fixture has issues with NULL status_changed_at:\n%s", out)
	}
}

// TestListAgeDaysJSONAndCSV enforces C2: the JSON cell is a number or null
// (never a string), and the CSV header names the same seven columns.
func TestListAgeDaysJSONAndCSV(t *testing.T) {
	sqlDemoHome(t)
	jsonOut, err := capture(t, func() error { return cmdList([]string{"--json", "--limit", "10"}) })
	if err != nil {
		t.Fatalf("list --json: %v\n%s", err, jsonOut)
	}
	dec := json.NewDecoder(strings.NewReader(jsonOut))
	numbers, nulls := 0, 0
	for {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("list --json is not a row stream: %v\n%s", err, jsonOut)
		}
		v, ok := row["age_days"]
		if !ok {
			t.Fatalf("age_days missing from the JSON row:\n%v", row)
		}
		switch v.(type) {
		case nil:
			nulls++
		case float64:
			numbers++
		default:
			t.Fatalf("age_days is %T, want number or null:\n%v", v, row)
		}
	}
	if numbers == 0 || nulls == 0 {
		t.Fatalf("want both a numeric and a null age_days across 10 rows (fixture has both kinds), got %d numbers, %d nulls:\n%s", numbers, nulls, jsonOut)
	}

	csvOut, err := capture(t, func() error { return cmdList([]string{"--csv", "--limit", "2"}) })
	if err != nil {
		t.Fatalf("list --csv: %v\n%s", err, csvOut)
	}
	recs, err := csv.NewReader(strings.NewReader(csvOut)).ReadAll()
	if err != nil {
		t.Fatalf("list --csv is not parseable CSV: %v\n%s", err, csvOut)
	}
	wantHeader := []string{"key", "priority", "priority_rank", "status", "age_days", "updated_at", "summary"}
	if strings.Join(recs[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("csv header = %v, want %v:\n%s", recs[0], wantHeader, csvOut)
	}
	for _, rec := range recs[1:] {
		if len(rec) != 7 {
			t.Fatalf("csv row %v does not have 7 fields:\n%s", rec, csvOut)
		}
	}
}

// TestReadyAndNextCarryAgeDays enforces C3. On the demo home ready degrades
// (no origin catalog to resolve the blocking link type) — announced on
// stderr, rows unchanged — which is the documented degradation, so the
// header assertion runs against that exact path.
func TestReadyAndNextCarryAgeDays(t *testing.T) {
	recipesDemoHome(t)
	ready, _, err := captureBoth(t, func() error { return cmdReady([]string{"--limit", "3"}) })
	if err != nil {
		t.Fatalf("ready: %v\n%s", err, ready)
	}
	if firstLine(ready) != listHeader {
		t.Fatalf("ready header = %q, want the list header %q:\n%s", firstLine(ready), listHeader, ready)
	}

	next, stderr, err := captureBoth(t, func() error { return cmdNext(nil) })
	if err != nil {
		t.Fatalf("next with no recipe: %v\n%s", err, next)
	}
	if !strings.Contains(stderr, `no saved recipe "next"`) {
		t.Fatalf("next stderr missing the fallback notice:\n%s", stderr)
	}
	if firstLine(next) != listHeader {
		t.Fatalf("next fallback header = %q, want the list header %q:\n%s", firstLine(next), listHeader, next)
	}
}

// TestNextRecipeSQLCarriesAge enforces the C4 constant half: help and the
// missing-recipe error quote nextRecipeSQL, so the constant itself must
// select the age column and produce it on the demo fixture. The single
// owner of the expression is ageDaysColumn in list.go, which both
// listColumns and nextRecipeSQL are built from.
func TestNextRecipeSQLCarriesAge(t *testing.T) {
	if !strings.Contains(nextRecipeSQL, "as age_days") {
		t.Fatalf("nextRecipeSQL lost the age column:\n%s", nextRecipeSQL)
	}
	recipesDemoHome(t)
	out, err := capture(t, func() error { return cmdSQL([]string{nextRecipeSQL}) })
	if err != nil {
		t.Fatalf("sql nextRecipeSQL: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.Contains(lines[0], "age_days") {
		t.Fatalf("nextRecipeSQL header lost age_days:\n%s", out)
	}
	filled := 0
	for _, ln := range lines[1:] {
		f := strings.Split(ln, "\t")
		if len(f) != 5 {
			t.Fatalf("nextRecipeSQL row %q does not have 5 fields:\n%s", ln, out)
		}
		if f[3] != "" {
			filled++
		}
	}
	if filled == 0 {
		t.Fatalf("nextRecipeSQL produced no populated age_days on the demo fixture:\n%s", out)
	}
}

// TestSavedRecipeKeepsItsOwnColumns enforces the C4 user-SQL half: a saved
// next recipe runs verbatim — the age column is added to the built-in
// default, never injected into what the user saved.
func TestSavedRecipeKeepsItsOwnColumns(t *testing.T) {
	recipesDemoHome(t)
	saved := "select key, summary from issues_full where status_category != 'done' order by key limit 5"
	if _, err := capture(t, func() error { return cmdRecipes([]string{"save", "next", saved}) }); err != nil {
		t.Fatalf("save next: %v", err)
	}
	next, stderr, err := captureBoth(t, func() error { return cmdNext(nil) })
	if err != nil {
		t.Fatalf("next on a saved recipe: %v\n%s", err, next)
	}
	if strings.Contains(stderr, `no saved recipe "next"`) {
		t.Fatalf("next fell back although a recipe is saved:\n%s", stderr)
	}
	if firstLine(next) != "key\tsummary" {
		t.Fatalf("saved recipe output header = %q, want the recipe own columns:\n%s", firstLine(next), next)
	}
	if strings.Contains(next, "age_days") {
		t.Fatalf("age_days leaked into a saved recipe:\n%s", next)
	}
}

// TestListSQLKeysOnCategoriesNotDisplayNames enforces C5: the open filter
// stays on status_category (a Korean account must not silently get 0 rows),
// and no query in the family keys on status or priority display names.
func TestListSQLKeysOnCategoriesNotDisplayNames(t *testing.T) {
	for name, q := range map[string]string{
		"listSQL":        listSQL(5, false, ""),
		"listDefaultSQL": listDefaultSQL(5),
		"nextRecipeSQL":  nextRecipeSQL,
	} {
		if !strings.Contains(q, "status_category != 'done'") {
			t.Errorf("%s lost status_category keying:\n%s", name, q)
		}
	}
	for name, q := range map[string]string{
		"listSQL open":   listSQL(5, false, ""),
		"listSQL all":    listSQL(5, true, ""),
		"listDefaultSQL": listDefaultSQL(5),
		"nextRecipeSQL":  nextRecipeSQL,
	} {
		for _, banned := range []string{"status =", "status=", "priority =", "priority=", `"In Progress"`} {
			if strings.Contains(q, banned) {
				t.Errorf("%s keys on a display name (%q):\n%s", name, banned, q)
			}
		}
	}
}

// skillDoc reads the shipped skill file the way recipes_gate_test.go reads
// docs/RECIPES.md: the skill is a contract surface, not test data.
func skillDoc(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "skills", "gadak", "SKILL.md"))
	if err != nil {
		t.Fatalf("read skills/gadak/SKILL.md: %v", err)
	}
	return string(raw)
}

const skillAnswerHeading = "### When asked what to do next"

// skillAnswerBlock returns the answer-shape subsection: its heading through
// the first blank line.
func skillAnswerBlock(t *testing.T, doc string) []string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	start := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == skillAnswerHeading {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("skills/gadak/SKILL.md has no %q subsection", skillAnswerHeading)
	}
	block := []string{lines[start]}
	i := start + 1
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			break
		}
		block = append(block, lines[i])
	}
	return block
}

// TestSkillAnswerShapeGate enforces C6 on the shipped skill file.
func TestSkillAnswerShapeGate(t *testing.T) {
	block := skillAnswerBlock(t, skillDoc(t))
	if len(block) > 14 {
		t.Fatalf("the answer-shape subsection is %d lines, want <= 14:\n%s", len(block), strings.Join(block, "\n"))
	}
	joined := strings.Join(block, "\n")
	for _, want := range []string{"age_days", "gadak ready", "RECIPES.md", "THEORY.md"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the answer-shape subsection does not name %q:\n%s", want, joined)
		}
	}
	for _, banned := range []string{"you should", "you have", "well done"} {
		if strings.Contains(strings.ToLower(joined), banned) {
			t.Errorf("the answer-shape subsection prescribes or praises (%q) — G1/G9:\n%s", banned, joined)
		}
	}
}

// TestSkillDocCheckInvariants mirrors the tools/doc-checks.sh greps over the
// file this change edits, so an edit that would redden doc-checks fails in
// go test too.
func TestSkillDocCheckInvariants(t *testing.T) {
	doc := skillDoc(t)
	if !strings.Contains(doc, "local.db") {
		t.Error("skills/gadak/SKILL.md no longer names local.db (doc-checks GDK-458)")
	}
	for _, ln := range strings.Split(doc, "\n") {
		if strings.Contains(ln, "views save") && strings.Contains(ln, "in the mirror") {
			t.Errorf("skills/gadak/SKILL.md says views save lives in the mirror (doc-checks GDK-458): %s", ln)
		}
	}
}

// TestSkillSaveExampleWeldedToNextRecipeSQL is the divergence detector for
// the one verbatim copy of nextRecipeSQL that lives outside Go: the skill
// save-command example must move with the constant (the pattern of
// TestWeeklyReopenScriptQuotesFirstRecipe, which welds a script to its
// RECIPES fence).
func TestSkillSaveExampleWeldedToNextRecipeSQL(t *testing.T) {
	doc := skillDoc(t)
	want := "gadak recipes save next \"" + nextRecipeSQL + "\""
	if !strings.Contains(doc, want) {
		t.Fatalf("skills/gadak/SKILL.md no longer quotes nextRecipeSQL in the save example, want:\n%s", want)
	}
}

// TestAgeDaysConsistentWithStatusChangedAt recomputes listed rows through
// gadak sql. The listed and recomputed executions read julianday(now) at
// different instants and round to one decimal, so one half-step (0.1) of
// slack covers a rounding-boundary flip, not a defect.
func TestAgeDaysConsistentWithStatusChangedAt(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error { return cmdList([]string{"--limit", "10"}) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	var filledKey, filledAge, nullKey string
	for _, ln := range strings.Split(strings.TrimRight(out, "\n"), "\n")[1:] {
		f := strings.Split(ln, "\t")
		if filledKey == "" && f[ageColumnIndex] != "" {
			filledKey, filledAge = f[0], f[ageColumnIndex]
		}
		if nullKey == "" && f[ageColumnIndex] == "" {
			nullKey = f[0]
		}
	}
	if filledKey == "" || nullKey == "" {
		t.Fatalf("want one populated and one empty age_days row for the recompute, got %q / %q:\n%s", filledKey, nullKey, out)
	}

	listed, err := strconv.ParseFloat(filledAge, 64)
	if err != nil {
		t.Fatalf("listed age_days %q is not a number:\n%s", filledAge, out)
	}
	row := ageRow(t, filledKey)
	if row.statusChangedAt == "" {
		t.Fatalf("%s listed a populated age_days but status_changed_at is empty:\n%s", filledKey, out)
	}
	if math.Abs(row.age-listed) > 0.1000001 {
		t.Fatalf("%s listed age_days %v but recomputed %v from status_changed_at %s", filledKey, listed, row.age, row.statusChangedAt)
	}

	// The empty cell must mean NULL status_changed_at, not a lost value.
	if nullRow := ageRow(t, nullKey); nullRow.statusChangedAt != "" {
		t.Fatalf("%s listed an empty age_days but status_changed_at is %s — the cell lost a value", nullKey, nullRow.statusChangedAt)
	}
}

// ageRow reads one issue status_changed_at and recomputed age through
// gadak sql, the same read path an agent verifying a list row would take.
func ageRow(t *testing.T, key string) (row struct {
	statusChangedAt string
	age             float64
}) {
	t.Helper()
	out, err := capture(t, func() error {
		return cmdSQL([]string{
			"select status_changed_at, round(julianday('now') - julianday(status_changed_at), 1) as age from issues_full where key = " + sqlLiteral(key),
		})
	})
	if err != nil {
		t.Fatalf("sql recompute for %s: %v\n%s", key, err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("recompute for %s: want header + 1 row:\n%s", key, out)
	}
	f := strings.Split(lines[1], "\t")
	row.statusChangedAt = f[0]
	if f[1] != "" {
		age, err := strconv.ParseFloat(f[1], 64)
		if err != nil {
			t.Fatalf("recomputed age %q is not a number:\n%s", f[1], out)
		}
		row.age = age
	}
	return row
}
