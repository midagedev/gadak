package store

// GDK-89 gate. docs/DERIVE.md is the single home of every documented SQL
// query over the derived columns. Until this file existed, the gate ran a
// hand-copied list of the queries (store_test.go's exampleQueries): the spec
// doc claimed "TestDocumentedExampleQueries runs each of them verbatim" while
// nothing ever read the document, so the doc could rot silently. These tests
// parse the document itself and execute what it actually says.
//
// Idiom reused, not reinvented: fence extraction follows
// cmd/gadak/recipes_gate_test.go (extended with min=/ignore info tokens, fence
// line numbers, and titles), and the demo.db copy follows
// internal/mcp/server_test.go. Test helpers do not cross packages, so the
// patterns are local copies; both sources are named here on purpose.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// deriveDoc is docs/DERIVE.md, resolved relative to this package's directory.
const deriveDoc = "../../docs/DERIVE.md"

// docSQL is one ```sql fence from docs/DERIVE.md.
type docSQL struct {
	line   int    // 1-based line of the opening fence — failures name it
	title  string // the fence's `--` heading, quoted in failures
	sql    string
	min    int // rows the query must return against examples/demo.db
	ignore bool // documented but never executed (wrong-shape examples)
}

// parseDocSQLFences reads every ```sql fence in doc. Fence grammar:
//
//	```sql            runnable, must return >= 1 row
//	```sql min=2      runnable, must return >= 2 rows
//	```sql ignore     never executed — documents a query that must not run
//
// Any other token after `sql` is a parse error, so a typo (`min:2`, `ignre`)
// fails the gate instead of quietly dropping the fence. Runnable fences carry
// one statement each: a `;` before the trailing one is rejected (a semicolon
// inside a string literal would trip this; none of the documented queries
// needs one).
func parseDocSQLFences(doc string) ([]docSQL, error) {
	var out []docSQL
	inFence := false
	var cur docSQL
	var body []string
	for i, ln := range strings.Split(doc, "\n") {
		line := i + 1
		trimmed := strings.TrimSpace(ln)
		switch {
		case inFence && trimmed == "```":
			sql := strings.TrimSpace(strings.Join(body, "\n"))
			if sql == "" {
				return nil, fmt.Errorf("docs/DERIVE.md:%d: empty ```sql fence", cur.line)
			}
			if !cur.ignore {
				// Comment lines may contain semicolons in prose; only the
				// statement text carries the one-query contract.
				var stmt []string
				for _, ln := range body {
					if t := strings.TrimSpace(ln); strings.HasPrefix(t, "--") {
						continue
					}
					stmt = append(stmt, ln)
				}
				joined := strings.TrimSpace(strings.Join(stmt, "\n"))
				if strings.Contains(strings.TrimSuffix(joined, ";"), ";") {
					return nil, fmt.Errorf("docs/DERIVE.md:%d: one query per fence — found a `;` that is not the trailing terminator", cur.line)
				}
			}
			cur.title = docTitle(body)
			cur.sql = sql
			out = append(out, cur)
			inFence = false
		case !inFence && strings.HasPrefix(trimmed, "```sql"):
			cur = docSQL{line: line, min: 1}
			for _, tok := range strings.Fields(trimmed)[1:] {
				switch {
				case tok == "sql":
					// the language token itself; already consumed by the prefix
				case tok == "ignore":
					cur.ignore = true
				case strings.HasPrefix(tok, "min="):
					n, err := strconv.Atoi(strings.TrimPrefix(tok, "min="))
					if err != nil || n < 1 {
						return nil, fmt.Errorf("docs/DERIVE.md:%d: bad marker %q — min must be a positive integer", line, tok)
					}
					cur.min = n
				default:
					return nil, fmt.Errorf("docs/DERIVE.md:%d: unknown fence marker %q — expected sql, min=N, or ignore", line, tok)
				}
			}
			inFence, body = true, nil
		case inFence:
			body = append(body, ln)
		}
	}
	if inFence {
		return nil, fmt.Errorf("docs/DERIVE.md ends inside an unterminated ```sql fence")
	}
	return out, nil
}

// docTitle is the fence's first `--` comment line, which DERIVE.md uses as the
// query's name; falls back to the first non-empty line.
func docTitle(body []string) string {
	for _, ln := range body {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "--") {
			return strings.TrimSpace(strings.TrimPrefix(t, "--"))
		}
	}
	for _, ln := range body {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return "(untitled)"
}

// openDemoCopy opens a throwaway copy of examples/demo.db (534 issues — the
// seed fixture is too small for the aggregate queries). The repo fixture is
// immutable and Open migrates (and writes WAL sidecars), so tests never open
// it in place. A missing or unreadable fixture is Fatal, never Skip: a gate
// that quietly skips is not a gate.
func openDemoCopy(t *testing.T) *DB {
	t.Helper()
	in, err := os.ReadFile(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Fatalf("read examples/demo.db: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(path, in, 0o600); err != nil {
		t.Fatalf("copy examples/demo.db into %s: %v", path, err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open demo copy: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestDocumentedExampleQueries executes every runnable ```sql fence in
// docs/DERIVE.md against a copy of examples/demo.db. The document is the
// single source: editing it is editing the test input. Zero runnable fences is
// a failure, so a markdown restructure that blinds the parser cannot pass
// vacuously.
func TestDocumentedExampleQueries(t *testing.T) {
	raw, err := os.ReadFile(deriveDoc)
	if err != nil {
		t.Fatalf("read docs/DERIVE.md: %v", err)
	}
	fences, err := parseDocSQLFences(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	runnable := 0
	for _, f := range fences {
		if !f.ignore {
			runnable++
		}
	}
	if runnable == 0 {
		t.Fatal("docs/DERIVE.md yielded 0 runnable ```sql fences — the extractor went blind or the queries moved away")
	}
	t.Logf("docs/DERIVE.md: %d sql fences parsed (%d runnable, %d ignored)", len(fences), runnable, len(fences)-runnable)

	db := openDemoCopy(t)
	for _, f := range fences {
		if f.ignore {
			continue
		}
		t.Run(fmt.Sprintf("line%d", f.line), func(t *testing.T) {
			rows, err := db.sql.QueryContext(context.Background(), f.sql)
			if err != nil {
				t.Fatalf("docs/DERIVE.md:%d %q: %v", f.line, f.title, err)
			}
			n := 0
			for rows.Next() {
				n++
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				t.Fatalf("docs/DERIVE.md:%d %q: %v", f.line, f.title, err)
			}
			rows.Close()
			if n < f.min {
				t.Errorf("docs/DERIVE.md:%d %q: returned %d rows, want at least %d", f.line, f.title, n, f.min)
			}
		})
	}
}

// TestDerivedColumnsDocumented keeps docs/DERIVE.md covering every derived
// column: it walks the Derived struct — the one type the write path fills —
// and requires each field's column name to appear in the document. A derived
// field added without documentation fails here with every missing name listed.
func TestDerivedColumnsDocumented(t *testing.T) {
	raw, err := os.ReadFile(deriveDoc)
	if err != nil {
		t.Fatalf("read docs/DERIVE.md: %v", err)
	}
	doc := string(raw)

	var missing []string
	tt := reflect.TypeOf(Derived{})
	for i := 0; i < tt.NumField(); i++ {
		col := camelToSnake(tt.Field(i).Name)
		if !strings.Contains(doc, col) {
			missing = append(missing, col)
		}
	}
	// Two lists on purpose: epic_key is recomputed by a table-wide UPDATE, not
	// carried by Derived, and item_refs is a table rather than a column — the
	// struct walk cannot see either, so they are asserted literally.
	for _, col := range []string{"epic_key", "item_refs"} {
		if !strings.Contains(doc, col) {
			missing = append(missing, col)
		}
	}
	if len(missing) > 0 {
		t.Errorf("docs/DERIVE.md does not document derived column(s): %s — the doc and the derive rules have drifted", strings.Join(missing, ", "))
	}
}

// camelToSnake maps a Go field name to its column name (ReopenCount →
// reopen_count). Derived has no consecutive capitals, so the simple split is
// exact for every field it will see; one that breaks the assumption fails the
// coverage test loudly instead of passing silently. (No equivalent helper
// exists in package store — internal/fields/slug.go serves a different shape.)
func camelToSnake(name string) string {
	var b strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
