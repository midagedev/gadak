package mcp

// agent_surface_gate_test.go — the standing assertions for the one surface in
// this repo that no other gate reads.
//
// CLAUDE.md names it: "internal/mcp/tools.go 의 서술은 게이트가 없는 표면이다"
// — a description that teaches a value the server never emits passes
// `go build`, `go vet`, `sourcelint`, `doc-checks` and every e2e spec, and the
// only reader is an agent with no shell to check it with. tools/mcp-tool-enums.sh
// exists and prints THIS SCRIPT DOES NOT ENFORCE ANYTHING (GDK-1820).
//
// These three tests close what is closeable from inside the package:
//
//   - TestEveryToolRefusesAnUnknownArgument — the tool's own InputSchema is the
//     allowlist for its arguments, and the dispatch enforces it for every tool,
//     not just the two newest (GDK-1812).
//   - TestNoHandlerReadsAnUndeclaredArgument — no handler reads an argument name
//     the schema does not declare, which after the above would be unreachable
//     code pretending to be a feature.
//   - TestQuerySchemaBlockNamesOnlyRealColumns — every snake_case name in the
//     gadak_query schema block is a real table, view or column of the shipped
//     fixture (GDK-1813's rider: the block gained six columns; this is what
//     stops the seventh from being a typo nobody can see).
//
// What they do NOT close: the omission direction — a new column or a new verb
// that no description mentions. tools/audit/surface-coverage.sh detects that
// and enforces nothing; GDK-1820 owns turning it into a gate.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// schemaArgNames is the argument allowlist a tool publishes: the property names
// of its own InputSchema. One owner — the same map tools/list hands the client.
func schemaArgNames(t Tool) []string {
	props, _ := t.InputSchema["properties"].(map[string]any)
	out := make([]string, 0, len(props))
	for k := range props {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Every tool refuses an argument it does not declare, and names it. Before
// GDK-1812 only gadak_retro and gadak_recents did; the other seven read the
// keys they knew and ignored the rest, so a host that sent `weeks` instead of
// `since` got a confident answer for a window it did not ask for.
func TestEveryToolRefusesAnUnknownArgument(t *testing.T) {
	dbPath := demoDB(t)
	t.Setenv("GADAK_HOME", t.TempDir())
	for _, def := range toolDefinitions() {
		t.Run(def.Name, func(t *testing.T) {
			srv := &Server{DBPath: dbPath, Version: "test"}
			content, isErr := srv.callTool(def.Name, map[string]any{"__nope__": 1})
			if !isErr {
				t.Fatalf("%s accepted an argument it does not declare", def.Name)
			}
			var text string
			for _, c := range content {
				text += c.Text
			}
			if !strings.Contains(text, "__nope__") {
				t.Fatalf("%s refused but never named the argument it refused: %s", def.Name, text)
			}
		})
	}
}

// No handler reads an argument name that no tool declares. After the dispatch
// validator such a read is unreachable — the call is refused before the handler
// runs — so it is a description-shaped lie in code form: the surface looks like
// it takes the argument and never can.
//
// The scan is narrow on purpose: an argument name reaches a handler either as
// args["name"] or as a string literal passed beside the args map itself
// (stringArg(args, "sql"), intArg(args, "limit", 0), boolArg(args, "by-sprint")).
// Literals that travel through a variable are out of reach here and out of
// reach of a reader too.
func TestNoHandlerReadsAnUndeclaredArgument(t *testing.T) {
	declared := map[string]bool{}
	for _, def := range toolDefinitions() {
		for _, n := range schemaArgNames(def) {
			declared[n] = true
		}
	}
	// Same walk as TestMirrorReadsGoThroughTheVisitRecorder next door:
	// parser.ParseFile over the package's own non-test files (ParseDir is
	// deprecated and ignores build tags).
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = f
	}
	if len(files) == 0 {
		t.Fatal("parsed no package files — the assertion would pass vacuously")
	}
	lit := func(e ast.Expr) (string, bool) {
		b, ok := e.(*ast.BasicLit)
		if !ok || b.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(b.Value)
		return s, err == nil
	}
	for path, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.IndexExpr:
				id, ok := e.X.(*ast.Ident)
				if !ok || id.Name != "args" {
					return true
				}
				if name, ok := lit(e.Index); ok && !declared[name] {
					t.Errorf("%s: reads args[%q], which no tool's InputSchema declares — the dispatch refuses that argument before the handler runs",
						path, name)
				}
			case *ast.CallExpr:
				if len(e.Args) == 0 {
					return true
				}
				if id, ok := e.Args[0].(*ast.Ident); !ok || id.Name != "args" {
					return true
				}
				for _, a := range e.Args[1:] {
					if name, ok := lit(a); ok && !declared[name] {
						t.Errorf("%s: reads the argument %q, which no tool's InputSchema declares — the dispatch refuses it before the handler runs",
							path, name)
					}
				}
			}
			return true
		})
	}
}

// notAColumn are the snake_case words the gadak_query description spells that
// are not names in the mirror. Sibling tool names are not listed by hand — they
// come from the roster, so a new tool the description points at needs no edit
// here. Anything else has to be a real table, view or column of the fixture.
var notAColumn = map[string]bool{
	"json_each":    true, // SQLite function
	"json_extract": true, // SQLite function
	"story_points": true, // the custom-field alias the example demonstrates
	"jira_server":  true, // an origin_type value, not a column
}

func notAColumnName(word string) bool {
	if notAColumn[word] {
		return true
	}
	for _, def := range toolDefinitions() {
		if def.Name == word {
			return true
		}
	}
	return false
}

// Every snake_case name in the gadak_query schema block is a real table, view
// or column of examples/demo.db. This is the direction CLAUDE.md warns about:
// a description that teaches a name the server cannot answer costs a shell-less
// agent a whole turn, and nothing else in the tree reads this text.
//
// The fixture is the mirror at the schema version the release ships
// (user_version 51 at the time of writing), so a column added by a migration
// that has not reached the fixture fails here — which is the same rule
// `make demo-fixture` already carries for e2e.
func TestQuerySchemaBlockNamesOnlyRealColumns(t *testing.T) {
	dbPath := demoDB(t)
	handle, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	known := map[string]bool{}
	rows, err := handle.Query(`SELECT name FROM sqlite_master WHERE type IN ('table','view')`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		known[n] = true
		tables = append(tables, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, tbl := range tables {
		cols, err := handle.Query(fmt.Sprintf(`SELECT name FROM pragma_table_info(%s)`, quoteLiteral(tbl)))
		if err != nil {
			t.Fatal(err)
		}
		for cols.Next() {
			var n string
			if err := cols.Scan(&n); err != nil {
				t.Fatal(err)
			}
			known[n] = true
		}
		cols.Close()
	}

	for _, name := range snakeCaseWords(toolQueryDescription) {
		if notAColumnName(name) || known[name] {
			continue
		}
		t.Errorf("the gadak_query schema block names %q; examples/demo.db has no table, view or column by that name — "+
			"an agent with no shell cannot find that out. Fix the description, or add the word to notAColumn if it is not a mirror name.", name)
	}
}

func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// gadak_status answers which mirror replied and what chose it — the fact
// SKILL.md's "say which mirror you read" rule needs and `profile` alone cannot
// carry: it is the empty string on the root workspace either way (GDK-1813).
// Values are the CLI's, so a transcript quoting one surface reads the same as
// a transcript quoting the other.
func TestStatusNamesTheMirrorAndWhatChoseIt(t *testing.T) {
	// Registered before the Setenv calls on purpose: cleanups run LIFO, so
	// each t.Setenv's own restore runs first and this re-reads the workspace
	// from the environment as it really is. Registered after them it would run
	// while GADAK_WORKSPACE=third was still set and leave the whole package on
	// a workspace named "third" (config.Profile is process-global).
	t.Cleanup(config.ReloadWorkspaceFromEnv)
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("GADAK_WORKSPACE", "")
	t.Setenv("GADAK_PROFILE", "")
	t.Setenv("SCRY_PROFILE", "")
	config.ReloadWorkspaceFromEnv()

	dbPath := demoDB(t)
	read := func(t *testing.T, profile string) map[string]any {
		t.Helper()
		srv := &Server{DBPath: dbPath, Profile: profile, Version: "test"}
		items, isErr := srv.callTool(toolStatus, map[string]any{})
		if isErr {
			t.Fatalf("gadak_status: %s", items[0].Text)
		}
		var st map[string]any
		if err := json.Unmarshal([]byte(items[0].Text), &st); err != nil {
			t.Fatalf("gadak_status payload is not JSON: %v", err)
		}
		return st
	}

	st := read(t, "")
	if st["workspace"] != "default" {
		t.Errorf(`root workspace reported %v, want "default" (config.NormalizeProfile owns this name)`, st["workspace"])
	}
	if st["workspace_source"] != config.SourceDefault {
		t.Errorf("workspace_source %v, want %q", st["workspace_source"], config.SourceDefault)
	}

	// A stored default is the case `profile` could never express: nobody in the
	// conversation named this workspace, and it is not the root either.
	dir, err := config.DirFor("second")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := config.SetStoredWorkspace("second"); err != nil {
		t.Fatal(err)
	}
	st = read(t, "second")
	if st["workspace"] != "second" {
		t.Errorf("workspace %v, want \"second\"", st["workspace"])
	}
	if st["workspace_source"] != config.SourceStored {
		t.Errorf("workspace_source %v, want %q", st["workspace_source"], config.SourceStored)
	}

	// The environment case names the variable, not the word "env" — the same
	// value `gadak status --json` prints, so the agent can quote the variable
	// a human would have to go and look at.
	t.Setenv("GADAK_WORKSPACE", "third")
	config.ReloadWorkspaceFromEnv()
	if st := read(t, "third"); st["workspace_source"] != "GADAK_WORKSPACE" {
		t.Errorf("workspace_source %v, want the env var name GADAK_WORKSPACE", st["workspace_source"])
	}
}

// The description teaches the three keys, or a shell-less agent never looks for
// them. This is the gate CLAUDE.md says this surface does not have — narrow,
// but it is the direction that costs a turn.
func TestStatusDescriptionTeachesWhichMirrorAnswered(t *testing.T) {
	for _, want := range []string{"workspace", "workspace_source", "actor", "stored"} {
		if !strings.Contains(toolStatusDescription, want) {
			t.Errorf("gadak_status description never names %q", want)
		}
	}
}

// The GDK-599 staleness notice is announced in exactly the descriptions of the
// tools that emit it. carriesFreshnessNotice is the single owner of that set
// (freshness.go); this asserts the descriptions are generated from it and not
// hand-copied, in both directions.
func TestFreshnessNoticeIsDescribedByExactlyTheToolsThatEmitIt(t *testing.T) {
	for _, def := range toolDefinitions() {
		described := strings.Contains(def.Description, freshnessNoticeSentence)
		switch {
		case carriesFreshnessNotice(def.Name) && !described:
			t.Errorf("%s appends the Mirror freshness notice and its description never says so", def.Name)
		case !carriesFreshnessNotice(def.Name) && described:
			t.Errorf("%s cannot emit the Mirror freshness notice and its description promises one", def.Name)
		}
	}
}

// snakeCaseWords pulls every lower_snake_case identifier out of prose: a word
// of [a-z0-9_] holding at least one underscore, with no letter, digit or dot
// touching either end (so issues.item_id yields item_id, and a hyphenated
// placeholder like account-id-from-the-mirror yields nothing).
func snakeCaseWords(text string) []string {
	isWord := func(b byte) bool {
		return b == '_' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
	}
	isBoundaryBlocker := func(b byte) bool {
		return b == '.' || b == '-' || (b >= 'A' && b <= 'Z')
	}
	seen := map[string]bool{}
	var out []string
	for i := 0; i < len(text); {
		if !isWord(text[i]) {
			i++
			continue
		}
		j := i
		for j < len(text) && isWord(text[j]) {
			j++
		}
		word := text[i:j]
		leftOK := i == 0 || !isBoundaryBlocker(text[i-1])
		rightOK := j == len(text) || !isBoundaryBlocker(text[j])
		if leftOK && rightOK && strings.Contains(word, "_") &&
			word[0] != '_' && word[len(word)-1] != '_' && !seen[word] {
			seen[word] = true
			out = append(out, word)
		}
		i = j
	}
	sort.Strings(out)
	return out
}
