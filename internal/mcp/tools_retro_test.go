package mcp

// Contract tests for gadak_retro (the MCP face of `gadak retro`). Two things
// are pinned: that the tool routes through internal/retro rather than
// recomputing, and that its argument and metric vocabulary is the CLI's —
// internal/mcp/tools.go descriptions have no gate, and the only reader is a
// shell-less agent.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/store"
)

// retroServer builds a server over a real (empty but migrated) mirror: retro
// reads the schema, so a missing file is not enough.
func retroServer(t *testing.T) *Server {
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
		ID: "jira", Kind: "jira", BaseURL: "https://example.invalid",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return &Server{DBPath: dbPath}
}

func callRetro(t *testing.T, s *Server, args map[string]any) (map[string]any, error) {
	t.Helper()
	items, err := s.toolRetro(args)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		t.Fatalf("gadak_retro returned %d content items, want 1", len(items))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(items[0].Text), &out); err != nil {
		t.Fatalf("gadak_retro payload is not JSON: %v\n%s", err, items[0].Text)
	}
	return out, nil
}

// The tool is on the surface at all, with the CLI's own name.
func TestRetroToolIsPublished(t *testing.T) {
	var found *Tool
	for i, tl := range toolDefinitions() {
		if tl.Name == toolRetro {
			found = &toolDefinitions()[i]
		}
	}
	if found == nil {
		t.Fatalf("%s is not in toolDefinitions()", toolRetro)
	}
	props, _ := found.InputSchema["properties"].(map[string]any)
	for _, want := range []string{"since", "session-gap", "by-sprint", "board", "open", "week"} {
		if _, ok := props[want]; !ok {
			t.Errorf("gadak_retro schema has no %q argument", want)
		}
	}
	if extra, ok := found.InputSchema["additionalProperties"].(bool); !ok || extra {
		t.Errorf("gadak_retro must refuse additional properties")
	}
}

// The metric enum is generated from internal/retro, not typed here: a
// description cannot teach a cell the report does not answer.
func TestRetroMetricEnumComesFromRetroPackage(t *testing.T) {
	var found *Tool
	defs := toolDefinitions()
	for i := range defs {
		if defs[i].Name == toolRetro {
			found = &defs[i]
		}
	}
	if found == nil {
		t.Fatalf("%s is not in toolDefinitions()", toolRetro)
	}
	props, _ := found.InputSchema["properties"].(map[string]any)
	open, _ := props["open"].(map[string]any)
	enum, _ := open["enum"].([]any)
	got := make([]string, 0, len(enum))
	for _, v := range enum {
		s, _ := v.(string)
		got = append(got, s)
	}
	if !slices.Equal(got, retro.OpenMetrics) {
		t.Errorf("open enum %v != retro.OpenMetrics %v", got, retro.OpenMetrics)
	}
	// Every enum value must also appear in the description, and the
	// description must name no metric the package does not carry.
	for _, m := range retro.OpenMetrics {
		if !strings.Contains(found.Description, m) {
			t.Errorf("description does not name metric %q", m)
		}
	}
}

// An argument the CLI does not have is refused by name rather than ignored:
// a silently dropped window is a wrong number the reader cannot see.
// GDK-1812 moved the check from this tool's own handler to the one dispatch,
// so the assertion enters through callTool — the same door a host knocks on.
// Same refusal, same two things named; the owner is now the tool's InputSchema
// rather than a list beside the handler. TestEveryToolRefusesAnUnknownArgument
// makes the same assertion for every other tool.
func TestRetroRefusesUnknownArgument(t *testing.T) {
	s := retroServer(t)
	content, isErr := s.callTool(toolRetro, map[string]any{"weeks": 4})
	if !isErr {
		t.Fatal("gadak_retro accepted an argument the CLI does not have")
	}
	var text string
	for _, c := range content {
		text += c.Text
	}
	if !strings.Contains(text, "weeks") || !strings.Contains(text, "since") {
		t.Errorf("error should name the rejected argument and the accepted set, got: %s", text)
	}
}

// --since is the CLI's parser, bounds and sentence — not a second one.
func TestRetroSinceUsesCLIParser(t *testing.T) {
	s := retroServer(t)
	if _, err := callRetro(t, s, map[string]any{"since": "400d"}); err == nil {
		t.Fatal("gadak_retro accepted a window beyond the CLI's cap")
	} else if !strings.Contains(err.Error(), "365") {
		t.Errorf("want the retro.ParseSince sentence, got: %v", err)
	}
	if _, err := callRetro(t, s, map[string]any{"since": "banana"}); err == nil {
		t.Fatal("gadak_retro accepted an unparseable window")
	}
}

// The default answer is the same document retro.Report.JSON() emits — the
// owner the CLI's --json and GET /api/v1/issues/retro/ already share.
func TestRetroAnswersTheReportDocument(t *testing.T) {
	s := retroServer(t)
	out, err := callRetro(t, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"buckets", "definitions", "bucket_noun", "session_gap", "aging"} {
		if _, ok := out[key]; !ok {
			t.Errorf("retro document has no %q", key)
		}
	}
	buckets, _ := out["buckets"].([]any)
	if len(buckets) == 0 {
		t.Error("retro document has no buckets")
	}
}

// --open names one cell and answers its keys; there is no window here, so the
// keys ARE the answer.
func TestRetroOpenAnswersKeys(t *testing.T) {
	s := retroServer(t)
	out, err := callRetro(t, s, map[string]any{"open": "closed"})
	if err != nil {
		t.Fatal(err)
	}
	if out["metric"] != "closed" {
		t.Errorf("metric = %v, want closed", out["metric"])
	}
	if _, ok := out["keys"]; !ok {
		t.Error("--open answer has no keys")
	}
	if _, err := callRetro(t, s, map[string]any{"open": "nonsense"}); err == nil {
		t.Error("gadak_retro accepted a metric the report does not answer")
	}
}

// aging is measured at now, so a week beside it is a request the report
// cannot honour — said, not dropped. Same rule as the CLI.
func TestRetroAgingRefusesWeek(t *testing.T) {
	s := retroServer(t)
	_, err := callRetro(t, s, map[string]any{"open": "aging", "week": 1})
	if err == nil {
		t.Fatal("gadak_retro accepted --week beside an at-now metric")
	}
	if !strings.Contains(err.Error(), "week") {
		t.Errorf("error should name week, got: %v", err)
	}
}

// --by-sprint and --since choose the columns two different ways; the CLI
// refuses both at once and so must this.
func TestRetroRefusesSinceWithBySprint(t *testing.T) {
	s := retroServer(t)
	_, err := callRetro(t, s, map[string]any{"since": "30d", "by-sprint": true})
	if err == nil {
		t.Fatal("gadak_retro accepted --since beside --by-sprint")
	}
}

// The byte cap is the one every other tool honours; a 365-day report is the
// largest window the parser admits and must still return a usable message
// rather than a truncated document.
func TestRetroRespectsByteCap(t *testing.T) {
	s := retroServer(t)
	s.resultByteCap = 64
	if _, err := s.toolRetro(map[string]any{}); err == nil {
		t.Fatal("gadak_retro ignored the result byte cap")
	} else if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("want the marshalResult cap sentence, got: %v", err)
	}
}

// The plainest call must not be the most expensive one. The full document is
// 238 KB over four weeks and 447 KB over a year on examples/demo.db — the
// second does not fit the result cap at all — so the table answer leaves out
// the per-bucket event log and key lists that `open` returns on demand, and
// says it did.
func TestRetroTableAnswerOmitsPerBucketKeyLists(t *testing.T) {
	s := retroServer(t)
	out, err := callRetro(t, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	buckets, _ := out["buckets"].([]any)
	if len(buckets) == 0 {
		t.Fatal("no buckets")
	}
	for _, entry := range buckets {
		b, _ := entry.(map[string]any)
		for _, banned := range []string{"events", "keys"} {
			if _, present := b[banned]; present {
				t.Errorf("table answer still carries bucket.%s", banned)
			}
		}
		for _, name := range []string{"unplanned", "seen_not_moved", "moved_not_seen"} {
			sub, _ := b[name].(map[string]any)
			if _, present := sub["keys"]; present {
				t.Errorf("table answer still carries bucket.%s.keys", name)
			}
		}
		// The counts stay: leaving them out would be a different report.
		if _, present := b["closed"]; !present {
			t.Error("table answer dropped the closed count")
		}
	}
	omitted, _ := out["omitted"].(map[string]any)
	if omitted == nil {
		t.Fatal("table answer does not say what it omitted")
	}
	why, _ := omitted["why"].(string)
	if !strings.Contains(why, "open") {
		t.Errorf("omitted.why should point at open, got %q", why)
	}
}
