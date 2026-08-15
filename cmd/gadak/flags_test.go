package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// Contract ↔ tests (GDK-41). ≥2 assertions per clause (happy + violation).
//
//  1. Unknown flag rejected
//     TestParseAroundUnknownFlagBeforePositional
//     TestParseAroundUnknownFlagAfterPositional
//     TestCreateUnknownFlagBeforePositionalWritesNothing
//     TestCreateUnknownFlagAfterPositionalWritesNothing
//  2. Exit status 2
//     TestParseAroundUnknownFlagExitStatus
//     TestCreateUnknownFlagExitStatus
//  3. Nothing written to Jira
//     TestCreateUnknownFlagBeforePositionalWritesNothing
//     TestCreateUnknownFlagAfterPositionalWritesNothing
//     TestEditUnknownFlagWritesNothing
//     TestAttachUnknownFlagWritesNothing
//  4. Bare `-` is still a value
//     TestParseAroundBareDashIsPositional
//     TestCreateReadsDescriptionFromStdin          (create_test.go)
//     TestCreateStdinMinusMDoesNotJoinIntoSummary  (create_test.go)
//     TestViewsOpenKeysStdin                       (views_test.go)
//     TestTeamImportStdinDashIsValue
//  5. Dash inside a value is still fine
//     TestCreateDashInsideSummaryStillWorks
//     TestEditLabelMinusValueIsNotAFlag            (edit_test.go)
//  6. Error lists the accepted flags
//     TestParseAroundUnknownFlagListsAccepted
//     TestCreateUnknownFlagListsAccepted
//  7. `--` end-of-flags (already in parseAround; documented as the dash-leading escape)
//     TestParseAroundDoubleDashStopsFlags
//     TestCreateLeadingDashSummaryAfterDoubleDash
//     TestWantsHelpStopsAtDoubleDash
//  8. Every positional command: unknown token rejected
//     TestParseAroundRejectsUnknownOnFlagSetCommands
//     TestPositionalCommandsRejectUnknownFlag
//
// Reproduction (2026-08-15): `gadak create --project GDK --type 10003
// --summary "…" --priority Low -m -` wrote the flags into the summary.

func createFlagSetForTest() *flag.FlagSet {
	fs := newFlagSet("create")
	fs.String("project", "", "")
	fs.String("type", "", "")
	var labels labelFlags
	fs.Var(&labels, "label", "")
	var attach labelFlags
	fs.Var(&attach, "attach", "")
	fs.String("m", "", "")
	fs.Bool("json", false, "")
	fs.String("batch", "", "")
	return fs
}

func TestParseAroundUnknownFlagBeforePositional(t *testing.T) {
	fs := createFlagSetForTest()
	pos, err := parseAround(fs, []string{"--summary", "ja UI locale", "--project", "GDK", "--type", "10003"})
	if err == nil {
		t.Fatalf("unknown --summary swallowed as positional %q", pos)
	}
	if !strings.Contains(err.Error(), "unknown flag --summary") {
		t.Fatalf("want unknown flag --summary, got %v", err)
	}
}

func TestParseAroundUnknownFlagAfterPositional(t *testing.T) {
	fs := createFlagSetForTest()
	pos, err := parseAround(fs, []string{"fix the gate", "--project", "GDK", "--priority", "Low"})
	if err == nil {
		t.Fatalf("unknown --priority swallowed as positional %q", pos)
	}
	if !strings.Contains(err.Error(), "unknown flag --priority") {
		t.Fatalf("want unknown flag --priority, got %v", err)
	}
}

func TestParseAroundUnknownFlagListsAccepted(t *testing.T) {
	fs := createFlagSetForTest()
	_, err := parseAround(fs, []string{"--summary", "x"})
	if err == nil {
		t.Fatal("expected unknown --summary")
	}
	// VisitAll sorts by name. dashed("m") is -m. File: help.go newUnknownFlag.
	want := "unknown flag --summary — accepted: --attach, --batch, --json, --label, -m, --project, --type\nrun \"gadak create --help\" for examples"
	if err.Error() != want {
		t.Fatalf("exact message:\n got %q\nwant %q", err.Error(), want)
	}
}

func TestParseAroundUnknownFlagExitStatus(t *testing.T) {
	fs := createFlagSetForTest()
	_, err := parseAround(fs, []string{"title", "--pretty"})
	if err == nil {
		t.Fatal("expected unknown --pretty (CLI exit 2)")
	}
	if got := exitStatus(err); got != 2 {
		t.Fatalf("exitStatus=%d want 2; err=%v", got, err)
	}
}

func TestParseAroundBareDashIsPositional(t *testing.T) {
	fs := createFlagSetForTest()
	pos, err := parseAround(fs, []string{"-", "--project", "NMB"})
	if err != nil {
		t.Fatalf("bare - must be a positional, got %v", err)
	}
	if len(pos) != 1 || pos[0] != "-" {
		t.Fatalf("pos=%q", pos)
	}
}

func TestParseAroundDoubleDashStopsFlags(t *testing.T) {
	fs := createFlagSetForTest()
	pos, err := parseAround(fs, []string{"--project", "NMB", "--type", "Task", "--", "--summary", "is the title"})
	if err != nil {
		t.Fatalf("-- must pass through dash-leading positionals: %v", err)
	}
	if got := strings.Join(pos, " "); got != "--summary is the title" {
		t.Fatalf("pos=%q", got)
	}
	if fs.Lookup("project").Value.String() != "NMB" {
		t.Fatalf("flags before -- were dropped: project=%q", fs.Lookup("project").Value)
	}
}

func TestParseAroundInlineUnknown(t *testing.T) {
	fs := createFlagSetForTest()
	pos, err := parseAround(fs, []string{"title", "--priority=Low"})
	if err == nil {
		t.Fatalf("--priority=Low swallowed as positional %q", pos)
	}
	if !strings.Contains(err.Error(), "unknown flag --priority=Low") && !strings.Contains(err.Error(), "unknown flag --priority") {
		t.Fatalf("want unknown --priority=Low, got %v", err)
	}
}

func TestParseAroundQuotedJQLRelativeDateIsNotAFlag(t *testing.T) {
	// One argv "updated >= -7d" does not start with -; JQL relative dates stay
	// valid when quoted. Unquoted -7d is an unknown flag after this change.
	fs := newFlagSet("search")
	fs.Bool("jql", false, "")
	pos, err := parseAround(fs, []string{"--jql", "updated >= -7d"})
	if err != nil {
		t.Fatalf("quoted-as-one-argv JQL with -7d: %v", err)
	}
	if got := strings.Join(pos, " "); got != "updated >= -7d" {
		t.Fatalf("pos=%q", got)
	}
}

func TestWantsHelpStopsAtDoubleDash(t *testing.T) {
	if !wantsHelp([]string{"--help"}) {
		t.Fatal("--help must still be help")
	}
	if !wantsHelp([]string{"title", "-h"}) {
		t.Fatal("-h after a positional is still help")
	}
	if wantsHelp([]string{"--", "--help"}) {
		t.Fatal("--help after -- is a value, not help")
	}
	if wantsHelp([]string{"--", "-h"}) {
		t.Fatal("-h after -- is a value, not help")
	}
}

func TestParseAroundRejectsUnknownOnFlagSetCommands(t *testing.T) {
	// One FlagSet per parseAround command, flags matching the production
	// registration names so a newly added known flag does not need this table
	// updated — only the command name / extra flags do.
	type spec struct {
		name  string
		flags []string // bool flags as "name"; value flags as "name="
	}
	cmds := []spec{
		{name: "create", flags: []string{"project=", "type=", "label=", "attach=", "m=", "json", "batch="}},
		{name: "edit", flags: []string{"summary=", "m=", "label=", "priority=", "json"}},
		{name: "attach", flags: []string{"json"}},
		{name: "search", flags: []string{"limit=", "json", "jql", "emit"}},
		{name: "api", flags: []string{"query=", "data=", "write", "status"}},
		{name: "views", flags: []string{"jql=", "keys=", "json", "no-open"}},
		{name: "issue", flags: []string{"json"}},
		{name: "comment", flags: []string{"m=", "json"}},
		{name: "transition", flags: []string{"json"}},
		{name: "assign", flags: []string{"json"}},
		{name: "open", flags: nil},
	}
	for _, c := range cmds {
		c := c
		t.Run(c.name, func(t *testing.T) {
			fs := newFlagSet(c.name)
			for _, f := range c.flags {
				name, _, isVal := strings.Cut(f, "=")
				switch {
				case name == "limit":
					fs.Int(name, 0, "")
				case isVal:
					fs.String(name, "", "")
				default:
					fs.Bool(name, false, "")
				}
			}
			pos, err := parseAround(fs, []string{"arg", "--not-a-real-flag"})
			if err == nil {
				t.Fatalf("%s: --not-a-real-flag swallowed as positional %q", c.name, pos)
			}
			if !strings.Contains(err.Error(), "unknown flag --not-a-real-flag") {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := exitStatus(err); got != 2 {
				t.Fatalf("%s: exitStatus=%d", c.name, got)
			}
		})
	}
}

func TestCreateUnknownFlagBeforePositionalWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	// The 2026-08-15 reproduction, against the fake.
	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"--project", "NMB", "--type", "Task",
			"--summary", "일본어(ja) UI 로케일",
			"--priority", "Low",
			"-m", "-",
		})
	})
	if err == nil {
		t.Fatalf("unknown --summary/--priority: expected error, got success; Jira calls %v", f.calls)
	}
	if !strings.Contains(err.Error(), "unknown flag --summary") {
		t.Fatalf("want unknown --summary first, got %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("unknown flag reached Jira: %v", f.calls)
	}
}

func TestCreateUnknownFlagAfterPositionalWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdCreate([]string{
			"Fix the flaky gate",
			"--project", "NMB", "--type", "Task",
			"--priority", "Low",
		})
	})
	if err == nil {
		t.Fatalf("unknown --priority after summary: expected error, got success; Jira calls %v", f.calls)
	}
	if !strings.Contains(err.Error(), "unknown flag --priority") {
		t.Fatalf("want unknown --priority, got %v", err)
	}
	if f.called("POST /issue") {
		t.Fatalf("unknown flag reached Jira: %v", f.calls)
	}
}

func TestCreateUnknownFlagExitStatus(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdCreate([]string{"title", "--project", "NMB", "--type", "Task", "--pretty"})
	})
	if err == nil {
		t.Fatal("expected unknown --pretty (CLI exit 2)")
	}
	if got := exitStatus(err); got != 2 {
		t.Fatalf("exitStatus=%d want 2; err=%v", got, err)
	}
	if f.called("POST /issue") {
		t.Fatalf("reached Jira: %v", f.calls)
	}
}

func TestCreateUnknownFlagListsAccepted(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdCreate([]string{"title", "--summary", "x", "--project", "NMB", "--type", "Task"})
	})
	if err == nil {
		t.Fatal("expected unknown --summary")
	}
	msg := err.Error()
	if !strings.Contains(msg, "accepted:") {
		t.Fatalf("must list accepted flags: %q", msg)
	}
	for _, want := range []string{"--project", "--type", "-m", "--json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("accepted list missing %s: %q", want, msg)
		}
	}
	if f.called("POST /issue") {
		t.Fatalf("reached Jira: %v", f.calls)
	}
}

func TestCreateDashInsideSummaryStillWorks(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdCreate([]string{"fix a-b handling", "--project", "NMB", "--type", "Task"})
	})
	if err != nil {
		t.Fatalf("dash inside summary: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"summary":"fix a-b handling"`) {
		t.Fatalf("summary %s", sent)
	}
}

func TestCreateLeadingDashSummaryAfterDoubleDash(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdCreate([]string{"--project", "NMB", "--type", "Task", "--", "-this", "broke"})
	})
	if err != nil {
		t.Fatalf("create -- -this: %v", err)
	}
	if sent := f.bodies["POST /issue"]; !strings.Contains(sent, `"summary":"-this broke"`) {
		t.Fatalf("leading-dash summary after --: %s", sent)
	}
}

func TestEditUnknownFlagWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdEdit([]string{"NMB-1", "--pretty"})
	})
	if err == nil {
		t.Fatal("expected unknown --pretty")
	}
	if !strings.Contains(err.Error(), "unknown flag --pretty") {
		t.Fatalf("want unknown --pretty, got %v", err)
	}
	if f.called("PUT /issue/NMB-1") {
		t.Fatalf("edit reached Jira: %v", f.calls)
	}
}

func TestAttachUnknownFlagWritesNothing(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	p := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(p, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := capture(t, func() error {
		return cmdAttach([]string{"NMB-1", p, "--pretty"})
	})
	if err == nil {
		t.Fatal("expected unknown --pretty")
	}
	if !strings.Contains(err.Error(), "unknown flag --pretty") {
		t.Fatalf("want unknown --pretty, got %v", err)
	}
	if len(f.uploads) != 0 {
		t.Fatalf("uploaded despite unknown flag: %+v", f.uploads)
	}
}

func TestPositionalCommandsRejectUnknownFlag(t *testing.T) {
	// Commands that take positionals: unknown --pretty must not be absorbed.
	// Write commands use the fake so we can assert no Jira call.
	t.Run("search", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		_, err := capture(t, func() error {
			return cmdSearch([]string{"idempotency", "--pretty"})
		})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("search: %v", err)
		}
	})
	t.Run("views-open", func(t *testing.T) {
		t.Setenv("GADAK_HOME", t.TempDir())
		config.SetProfile("")
		err := cmdViews([]string{"open", "--pretty", "Night triage"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("views open: %v", err)
		}
	})
	t.Run("views-save", func(t *testing.T) {
		t.Setenv("GADAK_HOME", t.TempDir())
		config.SetProfile("")
		err := cmdViews([]string{"save", "--pretty", "Night", "--jql", "project = NMB"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("views save: %v", err)
		}
	})
	t.Run("api", func(t *testing.T) {
		_, _, err := captureErr(t, func() error {
			return cmdAPI([]string{"GET", "/rest/api/3/myself", "--pretty"})
		})
		if err == nil || !strings.Contains(err.Error(), "--pretty") {
			t.Fatalf("api: %v", err)
		}
	})
	t.Run("open", func(t *testing.T) {
		t.Setenv("GADAK_HOME", t.TempDir())
		config.SetProfile("")
		err := cmdOpen([]string{"--pretty", "NMB-1"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("open: %v", err)
		}
	})
	t.Run("issue", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		_, err := capture(t, func() error {
			return cmdIssue([]string{"NMB-1", "--pretty"})
		})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("issue: %v", err)
		}
	})
	t.Run("comment", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		_, err := capture(t, func() error {
			return cmdComment([]string{"NMB-1", "--pretty", "-m", "hi"})
		})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("comment: %v", err)
		}
		if f.called("POST /issue/NMB-1/comment") {
			t.Fatalf("comment reached Jira: %v", f.calls)
		}
	})
	t.Run("transition", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		_, err := capture(t, func() error {
			return cmdTransition([]string{"NMB-1", "--pretty", "31"})
		})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("transition: %v", err)
		}
	})
	t.Run("assign", func(t *testing.T) {
		f := newFakeJira(t)
		mirror(t, f.URL)
		_, err := capture(t, func() error {
			return cmdAssign([]string{"NMB-1", "--pretty", "marco@example.com"})
		})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("assign: %v", err)
		}
	})
	t.Run("sql", func(t *testing.T) {
		sqlDemoHome(t)
		_, err := capture(t, func() error {
			return cmdSQL([]string{"--pretty", "select 1"})
		})
		if err == nil || !strings.Contains(err.Error(), "--pretty") {
			t.Fatalf("sql: %v", err)
		}
	})
	t.Run("snapshot", func(t *testing.T) {
		err := cmdSnapshot([]string{"out.db", "--pretty"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("snapshot: %v", err)
		}
	})
	t.Run("import", func(t *testing.T) {
		err := cmdImport([]string{"file.json", "--pretty"})
		if err == nil {
			t.Fatal("import extra/--pretty must not succeed")
		}
	})
	t.Run("export-static", func(t *testing.T) {
		// fs.Parse + ExitOnError would os.Exit; only the leftover-arg path
		// returns. --pretty before the outdir is Go's unknown-flag exit, so
		// we assert the documented usage path (unknown after the positional).
		err := cmdExportStatic([]string{"out", "--pretty"})
		if err == nil {
			t.Fatal("export-static extra arg must not succeed")
		}
	})
	t.Run("team-import", func(t *testing.T) {
		err := cmdTeamImport([]string{"file.json", "--pretty"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("team import: %v", err)
		}
	})
	t.Run("skill-install", func(t *testing.T) {
		err := cmdSkillInstall([]string{"claude", "--pretty"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("skill: %v", err)
		}
	})
	t.Run("mcp-install", func(t *testing.T) {
		err := cmdMCPInstall([]string{"json", "--pretty"})
		if err == nil || !strings.Contains(err.Error(), "unknown flag --pretty") {
			t.Fatalf("mcp install: %v", err)
		}
	})
}

func TestTeamImportStdinDashIsValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	cfg := &config.Config{Site: "https://example.atlassian.net", Email: "a@b.c", Token: "tok"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	body := `{
  "gadak_team_config": 1,
  "exported_at": "2026-08-14T00:00:00Z",
  "settings": {"groupLabels": {"platform": "Platform"}},
  "views": [{"name": "Night triage", "config": {"filters": {}, "display": {}}}]
}`
	withStdin(t, body)
	out, err := capture(t, func() error {
		return cmdTeamImport([]string{"-"})
	})
	if err != nil {
		t.Fatalf("team import -: %v\n%s", err, out)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.GroupLabels["platform"] != "Platform" {
		t.Fatalf("stdin import did not apply: %+v", got.GroupLabels)
	}
}

func TestExitStatusMapper(t *testing.T) {
	if exitStatus(nil) != 0 {
		t.Fatalf("nil: %d", exitStatus(nil))
	}
	if exitStatus(errors.New("no Jira credential")) != 1 {
		t.Fatalf("plain: %d", exitStatus(errors.New("x")))
	}
	fs := createFlagSetForTest()
	err := newUnknownFlag(fs, "--summary")
	if exitStatus(err) != 2 {
		t.Fatalf("unknown: %d / %v", exitStatus(err), err)
	}
}
