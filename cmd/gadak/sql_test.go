package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// sqlDemoHome copies examples/demo.db into a throwaway GADAK_HOME, the same
// fixture pattern as TestDoctorDemoDBCounts.
func sqlDemoHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	src := filepath.Join("..", "..", "examples", "demo.db")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db"), raw, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}
}

func TestSQLNoHeaderOmitsTSVHeader(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--no-header", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --no-header: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("empty stdout: %q", out)
	}
	if lines[0] == "key" {
		t.Fatalf("first row is the header, want an issue key: %q", out)
	}
	if !looksLikeIssueKey(lines[0]) {
		t.Fatalf("first row %q is not an issue key", lines[0])
	}
	if len(lines) != 3 {
		t.Fatalf("want 3 data rows, got %d: %q", len(lines), out)
	}
}

func TestSQLDefaultKeepsTSVHeader(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql default: %v\n%s", err, out)
	}
	first, _, _ := strings.Cut(out, "\n")
	if first != "key" {
		t.Fatalf("default first row must stay the header, got %q", first)
	}
}

func TestSQLJSONNoHeaderIsNoop(t *testing.T) {
	sqlDemoHome(t)
	plain, err := capture(t, func() error {
		return cmdSQL([]string{"--json", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --json: %v\n%s", err, plain)
	}
	with, err := capture(t, func() error {
		return cmdSQL([]string{"--json", "--no-header", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --json --no-header: %v\n%s", err, with)
	}
	if with != plain {
		t.Fatalf("--json --no-header must match --json\n--json:\n%s\n--json --no-header:\n%s", plain, with)
	}
}

func TestSQLNoHeaderOmitsCSVHeader(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--csv", "--no-header", "select key from issues limit 3"})
	})
	if err != nil {
		t.Fatalf("sql --csv --no-header: %v\n%s", err, out)
	}
	first, _, _ := strings.Cut(out, "\n")
	if first == "key" {
		t.Fatalf("csv first row is the header, want an issue key: %q", out)
	}
	if !looksLikeIssueKey(first) {
		t.Fatalf("csv first row %q is not an issue key", first)
	}
}

func TestSQLUnknownFlagIsUsageError(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"--pretty", "select key from issues limit 1"})
	})
	if err == nil {
		t.Fatalf("unknown flag --pretty must be a usage error, got nil err and stdout %q", out)
	}
	if !strings.Contains(err.Error(), "--pretty") {
		t.Fatalf("usage error must echo --pretty, got %v", err)
	}
	if !strings.Contains(err.Error(), `run "gadak sql --help"`) {
		t.Fatalf("want usageError help pointer, got %v", err)
	}
}

func TestSQLQuotedCommentQueryStillRuns(t *testing.T) {
	sqlDemoHome(t)
	out, err := capture(t, func() error {
		return cmdSQL([]string{"-- comment\nselect key from issues limit 1"})
	})
	if err != nil {
		t.Fatalf("quoted SQL starting with -- comment: %v\n%s", err, out)
	}
	first, _, _ := strings.Cut(out, "\n")
	if first != "key" {
		t.Fatalf("want header, got %q\n%s", first, out)
	}
}
