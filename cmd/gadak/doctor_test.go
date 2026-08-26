package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/applog"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

func TestParseLsofHoldersDropsSelfAndDedups(t *testing.T) {
	const self = 99
	out := []byte("p99\ncgadak\np42\ncGadak\np42\ncGadak\np7\ncgadak\n")
	got := parseLsofHolders(out, self)
	if len(got) != 2 {
		t.Fatalf("got %d holders, want 2 (self excluded, pid 42 deduped): %+v", len(got), got)
	}
	if got[0].PID != 7 || got[0].Command != "gadak" {
		t.Errorf("first = %+v, want pid 7 gadak", got[0])
	}
	if got[1].PID != 42 || got[1].Command != "Gadak" {
		t.Errorf("second = %+v, want pid 42 Gadak", got[1])
	}
	if n := len(parseLsofHolders(nil, self)); n != 0 {
		t.Fatalf("empty lsof output: %d holders, want 0", n)
	}
}

func TestDoctorMirrorHoldersSurface(t *testing.T) {
	held := formatDoctorText(doctorReport{
		MirrorHolders: &doctorMirrorHolders{
			Count: 2,
			Processes: []doctorMirrorProcess{
				{PID: 11, Command: "gadak"},
				{PID: 22, Command: "Gadak"},
			},
		},
	})
	if !strings.Contains(held, "mirror_holders:") {
		t.Fatalf("populated holders missing section:\n%s", held)
	}
	empty := formatDoctorText(doctorReport{
		MirrorHolders: &doctorMirrorHolders{Count: 0},
	})
	if !strings.Contains(empty, "mirror_holders:") {
		t.Fatalf("count-0 holders missing section:\n%s", empty)
	}
	omitted := formatDoctorText(doctorReport{})
	if strings.Contains(omitted, "mirror_holders:") {
		t.Fatalf("nil holders (scan skipped) must omit the section:\n%s", omitted)
	}
}

func TestDoctorNoMirrorSucceeds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	// doctor now reports on ~/.claude too (GDK-92); keep the suite off the
	// developer's real agent config.
	t.Setenv("HOME", home)
	config.SetProfile("")

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor with no mirror: %v\n%s", err, out)
	}
	if !strings.Contains(out, "not found") {
		t.Fatalf("expected not-found mirror line, got:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "# gadak doctor") {
		t.Fatalf("missing safety banner:\n%s", out)
	}
	// Must not create a database just by diagnosing.
	if _, err := os.Stat(filepath.Join(home, "gadak.db")); !os.IsNotExist(err) {
		t.Fatalf("doctor must not create gadak.db when missing; stat err=%v", err)
	}
}

func TestDoctorDemoDBCounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	// doctor now reports on ~/.claude too (GDK-92); keep the suite off the
	// developer's real agent config.
	t.Setenv("HOME", home)
	config.SetProfile("")

	src := filepath.Join("..", "..", "examples", "demo.db")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read demo.db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db"), raw, 0o600); err != nil {
		t.Fatalf("copy demo.db: %v", err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "mirror:") || !strings.Contains(out, "present") {
		t.Fatalf("expected present mirror:\n%s", out)
	}
	// Sanity: open the same copy and compare schema + row counts.
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open copy: %v", err)
	}
	defer db.Close()
	wantSchema := db.SchemaVersion()
	if !strings.Contains(out, "schema_version:") || !strings.Contains(out, strconv.Itoa(wantSchema)) {
		t.Fatalf("expected schema_version %d in:\n%s", wantSchema, out)
	}
	if strings.Contains(out, "issues:                n/a") {
		t.Fatalf("issues should be counted:\n%s", out)
	}
	n, err := db.TableCount(context.Background(), "issues")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 1 {
		t.Fatalf("demo.db has no issues?")
	}
	if !strings.Contains(out, strconv.Itoa(n)) {
		t.Fatalf("output missing issues count %d:\n%s", n, out)
	}
	if got := doctorValue(t, out, "schema_audit"); got != "ok" {
		t.Fatalf("demo.db schema_audit = %q, want ok", got)
	}
}

func TestDoctorRedaction(t *testing.T) {
	// Capture the real home before isolating: the leak check at the bottom is
	// about the *account* username, so it must keep asking about the real one.
	realHome, realHomeErr := os.UserHomeDir()

	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	// doctor now reports on ~/.claude too (GDK-92); keep the suite off the
	// developer's real agent config.
	t.Setenv("HOME", home)
	config.SetProfile("")

	// Seed a tiny mirror with project keys that must never appear in output.
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"3": "inprogress"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "LEAKY-42", Title: "do not leak this summary",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "SECRET", IssueType: "Bug", IssueTypeID: "1",
				Status: "Open", StatusID: "3", StatusCategory: "inprogress",
			},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Plant a last_error that embeds a host, key, and URL — classifier only.
	if err := db.RecordSync(context.Background(), "jira", store.SyncResult{
		Err: errString("GET /rest/api/3/search: jira: 403: You do not have access to LEAKY-42 on https://example.atlassian.net"),
	}); err != nil {
		t.Fatalf("record sync: %v", err)
	}
	_ = db.Close()

	const (
		fakeSite  = "https://example.atlassian.net"
		fakeEmail = "alice.secret@example.invalid"
		fakeToken = "NOT-A-REAL-TOKEN-fixture-value-0123456789"
	)
	cfg := &config.Config{
		Site:     fakeSite,
		Email:    fakeEmail,
		Token:    fakeToken,
		Projects: []string{"SECRET", "LEAKME"},
		Fields: []config.FieldSpec{{
			Alias: "internal_risk",
			Label: "Internal Risk Score",
			IDs:   []string{"customfield_99100"},
			Role:  "facet",
		}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	jsonOut, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, jsonOut)
	}

	forbidden := []string{
		fakeSite,
		"example.atlassian.net",
		"example",
		fakeEmail,
		"alice.secret",
		fakeToken,
		"NOT-A-REAL-TOKEN",
		"LEAKY-42",
		"SECRET",
		"LEAKME",
		"do not leak this summary",
		"Internal Risk Score",
		"internal_risk",
		"customfield_99100",
		"You do not have access",
	}
	for _, bad := range forbidden {
		if strings.Contains(out, bad) {
			t.Errorf("human output leaked %q", bad)
		}
		if strings.Contains(jsonOut, bad) {
			t.Errorf("json output leaked %q", bad)
		}
	}
	// Positive redaction markers that should appear.
	for _, want := range []string{
		"credential:            present",
		"email:                 configured",
		"<redacted>.atlassian.net",
		"http 403 (auth)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}
	if got := doctorValue(t, out, "custom_fields"); !strings.Contains(got, "1 alias mapped") {
		t.Errorf("custom_fields = %q, want a mapped-alias summary", got)
	}
	// Username from the temp path must not appear if home-relative.
	if realHomeErr == nil {
		base := filepath.Base(realHome)
		// Only flag when the home base is a plausible username length and
		// appears as a path segment (not as part of unrelated words).
		if base != "" && base != "tmp" && strings.Contains(out, "/"+base+"/") {
			t.Errorf("username path segment %q appeared in output:\n%s", base, out)
		}
	}
}

func TestDoctorJSONMatchesHumanFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	// doctor now reports on ~/.claude too (GDK-92); keep the suite off the
	// developer's real agent config.
	t.Setenv("HOME", home)
	config.SetProfile("")

	// Empty home is enough — doctor succeeds without a mirror.
	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("human: %v", err)
	}
	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.GadakVersion == "" || rep.GoVersion == "" || rep.OS == "" || rep.Arch == "" {
		t.Fatalf("missing runtime fields: %+v", rep)
	}
	if rep.Profile != "default" {
		t.Fatalf("profile = %q, want default", rep.Profile)
	}
	if rep.Mirror.Status != "not_found" {
		t.Fatalf("mirror.status = %q, want not_found", rep.Mirror.Status)
	}
	// Human form carries the same facts (as text).
	for _, want := range []string{
		"gadak_version:",
		"go_version:",
		"os:",
		"profile:",
		"mirror_path:",
		"mirror:",
		"schema_version:",
		"migrations:",
		"credential:",
		"workspace:",
		"site:",
		"email:",
		"api_usage.day:",
		"api_usage.requests:",
		"api_usage.throttled:",
		"api_usage.retries:",
	} {
		if !strings.Contains(human, want) {
			t.Errorf("human missing %q", want)
		}
	}
	if rep.APIUsage.Day == "" {
		t.Fatal("json api_usage.day empty")
	}
	if rep.Credential != "absent" || rep.Site != "none" || rep.Email != "none" {
		t.Fatalf("empty config: credential=%q site=%q email=%q", rep.Credential, rep.Site, rep.Email)
	}
}

func TestDoctorLogsSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	logsDir := filepath.Join(home, "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("hello log\n")
	if err := os.WriteFile(filepath.Join(logsDir, "gadak.log"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "gadak.log.1"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "logs.path:") {
		t.Fatalf("human missing logs section:\n%s", out)
	}
	if !strings.Contains(out, "gadak.log") {
		t.Fatalf("human missing log path:\n%s", out)
	}
	if !strings.Contains(out, "rotated") {
		t.Fatalf("human missing rotation marker:\n%s", out)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("json: %v\n%s", err, raw)
	}
	var rep struct {
		Logs struct {
			Path    string   `json:"path"`
			Size    *int64   `json:"size"`
			Rotated bool     `json:"rotated"`
			Recent  []string `json:"recent"`
		} `json:"logs"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if !strings.Contains(rep.Logs.Path, "gadak.log") {
		t.Fatalf("json logs.path = %q", rep.Logs.Path)
	}
	if rep.Logs.Size == nil || *rep.Logs.Size != int64(len(payload)) {
		t.Fatalf("json logs.size = %v, want %d", rep.Logs.Size, len(payload))
	}
	if !rep.Logs.Rotated {
		t.Fatal("json logs.rotated = false, want true")
	}
}

func TestDoctorLogsRecentErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	oldErr := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devnull
	t.Cleanup(func() {
		os.Stderr = oldErr
		_ = devnull.Close()
	})

	prevOut, prevFlags := log.Writer(), log.Flags()
	closer, err := applog.Install(home)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	t.Cleanup(func() {
		closer()
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	log.SetFlags(0)
	log.Println("ordinary diagnostic")
	log.Println("sync failed: something")
	log.Println("permission denied for attach")
	log.Println("still fine")

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "logs.recent:") {
		t.Fatalf("human missing logs.recent:\n%s", out)
	}
	if !strings.Contains(out, "sync failed") || !strings.Contains(out, "denied") {
		t.Fatalf("human recent errors missing expected lines:\n%s", out)
	}
	if strings.Contains(out, "ordinary diagnostic") || strings.Contains(out, "still fine") {
		t.Fatalf("non-error lines leaked into logs.recent:\n%s", out)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("json: %v\n%s", err, raw)
	}
	var rep struct {
		Logs struct {
			Recent []string `json:"recent"`
		} `json:"logs"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if len(rep.Logs.Recent) != 2 {
		t.Fatalf("json recent = %#v, want 2 error-ish lines", rep.Logs.Recent)
	}
}

// TestDoctorReportsSkillAndMCP — GDK-92. A user whose skill is a release behind
// had no way to find out; doctor now answers it. Identity is the content hash,
// never mtime, so the whole test drives the file bytes.
func TestDoctorReportsSkillAndMCP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	// --project would resolve against the package directory; keep it inside the
	// scratch home so the real checkout is never consulted.
	t.Chdir(home)

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	if !strings.Contains(human, "skill:") {
		t.Fatalf("doctor grew no skill line:\n%s", human)
	}
	if !strings.Contains(human, "mcp:") {
		t.Fatalf("doctor grew no mcp line:\n%s", human)
	}
	if got := doctorValue(t, human, "skill"); !strings.HasPrefix(got, "missing") {
		t.Errorf("empty home: skill = %q, want missing", got)
	}
	if got := doctorValue(t, human, "mcp"); !strings.HasPrefix(got, "absent") {
		t.Errorf("no claude config: mcp = %q, want absent", got)
	}

	dest := filepath.Join(home, ".claude", "skills", "gadak", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, gadak.SkillMarkdown(), 0o644); err != nil {
		t.Fatal(err)
	}
	human, err = capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if got := doctorValue(t, human, "skill"); !strings.HasPrefix(got, "current") {
		t.Errorf("byte-equal skill = %q, want current", got)
	}
	if !strings.Contains(human, "~/.claude/skills/gadak/SKILL.md") {
		t.Errorf("skill line should carry the tilde-abbreviated path, got:\n%s", human)
	}

	// One byte behind is stale — same file name, same mtime granularity.
	if err := os.WriteFile(dest, append(append([]byte{}, gadak.SkillMarkdown()...), '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	human, err = capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if got := doctorValue(t, human, "skill"); !strings.HasPrefix(got, "stale") {
		t.Errorf("one-byte-behind skill = %q, want stale", got)
	}

	// Claude Code's user-scope MCP registration.
	claudeCfg := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeCfg, []byte(`{"mcpServers":{"gadak":{"command":"gadak","args":["mcp"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	human, err = capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if got := doctorValue(t, human, "mcp"); !strings.HasPrefix(got, "registered") {
		t.Errorf("registered server: mcp = %q, want registered", got)
	}
	if !strings.Contains(human, "~/.claude.json") {
		t.Errorf("mcp line should carry the tilde-abbreviated config path, got:\n%s", human)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.Skill.Status != "stale" || rep.Skill.Scope != "user" || rep.Skill.Path != "~/.claude/skills/gadak/SKILL.md" {
		t.Errorf("json skill = %+v", rep.Skill)
	}
	if rep.MCP.Status != "registered" || rep.MCP.Scope != "user" || rep.MCP.Path != "~/.claude.json" {
		t.Errorf("json mcp = %+v", rep.MCP)
	}
	// The JSON tags are the contract: name them literally, so a rename of a
	// struct field cannot pass by silently.
	for _, want := range []string{`"skill"`, `"mcp"`, `"status"`, `"scope"`, `"path"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("JSON report missing %s:\n%s", want, raw)
		}
	}
}

// doctorValue pulls one "key: value" line out of the human report so the tests
// assert on the value, not on the column padding.
func doctorValue(t *testing.T, out, key string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(line, key+":")
		if !ok {
			continue
		}
		return strings.TrimSpace(rest)
	}
	t.Fatalf("no %q line in doctor output:\n%s", key, out)
	return ""
}

func TestDoctorReportsStandaloneWithCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	const planted = "NOT-A-REAL-TOKEN-doctor-inconsistent"
	cfg := &config.Config{Kind: config.KindStandalone, Token: planted}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, planted) {
		t.Fatalf("doctor leaked the token:\n%s", out)
	}
	got := doctorValue(t, out, "workspace")
	if !strings.Contains(got, "kind=standalone") {
		t.Errorf("workspace line missing kind=standalone: %q", got)
	}
	if !strings.Contains(got, "site_token=yes") {
		t.Errorf("workspace line missing site_token=yes: %q", got)
	}
	if !strings.Contains(got, "inconsistent") {
		t.Errorf("standalone-with-token must say inconsistent: %q", got)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	if strings.Contains(raw, planted) {
		t.Fatalf("json leaked the token:\n%s", raw)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if !rep.Workspace.Inconsistent || !rep.Workspace.HasSiteToken || rep.Workspace.Kind != config.KindStandalone {
		t.Fatalf("json workspace = %+v", rep.Workspace)
	}
}

func TestDoctorOriginOwnerEmbedded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Kind: config.KindStandalone}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	got := doctorValue(t, out, "origin owner")
	if got != "embedded (no live serve)" {
		t.Fatalf("origin owner = %q", got)
	}
}

func TestDoctorOriginOwnerIgnoresStaleAdvertise(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cfg := &config.Config{Kind: config.KindStandalone}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Directory(), "serve-origin.json"),
		[]byte(`{"addr":"127.0.0.1:1","pid":1,"startedAt":"2020-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	got := doctorValue(t, out, "origin owner")
	if got != "embedded (no live serve)" {
		t.Fatalf("origin owner = %q, want leftover serve-origin.json ignored", got)
	}
}

func TestClassifyLastError(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "none"},
		{"GET /rest/api/3/search: jira: 403: denied", "http 403 (auth)"},
		{"jira: 401: unauthorized", "http 401 (auth)"},
		{"jira: 429: Rate limited", "http 429 (throttled)"},
		{"upstream status 503", "http 503 (server)"},
		{"HTTP 404 not found", "http 404 (not_found)"},
		{"context deadline exceeded", "timeout"},
		{"dial tcp: connection refused", "network"},
		{"jira: credential rejected", "auth"},
		{"something opaque went wrong", "error"},
	}
	for _, tc := range cases {
		if got := classifyLastError(tc.in); got != tc.want {
			t.Errorf("classifyLastError(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRedactSite(t *testing.T) {
	cases := map[string]string{
		"":                               "none",
		"https://example.atlassian.net":  "<redacted>.atlassian.net",
		"https://example.atlassian.net/": "<redacted>.atlassian.net",
		"https://jira.example.com":       "configured (cloud)",
		"http://localhost:8080":          "configured (cloud)",
	}
	for in, want := range cases {
		if got := redactSite(in); got != want {
			t.Errorf("redactSite(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTildeHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	got := tildeHome(filepath.Join(home, ".gadak", "gadak.db"))
	if got != "~/.gadak/gadak.db" && got != `~\.gadak\gadak.db` {
		// On Unix we expect forward slashes from filepath under home.
		if !strings.HasPrefix(got, "~") || strings.Contains(got, home) {
			t.Fatalf("tildeHome = %q, still contains home or missing ~", got)
		}
	}
	if strings.Contains(got, home) {
		t.Fatalf("tildeHome leaked home dir: %q", got)
	}
}

// errString plants a last_error without pulling in fmt at every call site.
type errString string

func (e errString) Error() string { return string(e) }

// doctor is what someone runs when the mirror stopped opening, so the one
// cause it can name from the file itself must not read as "open failed"
// (GDK-498).
func TestDoctorNamesNewerMirrorInsteadOfOpenFailed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	path := filepath.Join(home, "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("seed mirror: %v", err)
	}
	applied := db.SchemaVersion()
	db.Close()
	future := applied + 2
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version = " + strconv.Itoa(future)); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor must still report on a mirror it cannot open: %v\n%s", err, out)
	}
	if !strings.Contains(out, "schema_too_new") {
		t.Fatalf("doctor must name the cause:\n%s", out)
	}
	if strings.Contains(out, "open failed") {
		t.Fatalf("doctor must not report a nameable cause as a generic open failure:\n%s", out)
	}
	// The version pair is the diagnosis: what the file has, what this build reads.
	if !strings.Contains(out, strconv.Itoa(future)) || !strings.Contains(out, strconv.Itoa(applied)) {
		t.Fatalf("expected both schema versions (%d found, %d supported):\n%s", future, applied, out)
	}
}

// GDK-522: unmapped config + raw still carrying customfield_ keys must name
// `gadak fields --apply`. The human line used to be just the mapped count.
func TestDoctorCustomFieldsUnmappedRawHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
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
				Key: "NMB-1", Title: "has an unmapped custom field",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "1",
				Status: "Open", StatusID: "3", StatusCategory: "inprogress",
				Raw: []byte(`{"fields":{"customfield_10016":8}}`),
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Site:  "https://example.invalid",
		Email: "a@example.invalid",
		Token: "token",
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	got := doctorValue(t, human, "custom_fields")
	if !strings.Contains(got, "run gadak fields --apply") {
		t.Fatalf("unmapped+raw doctor line must name fields --apply, got %q\n%s", got, human)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	cf, ok := doc["custom_fields"].(map[string]any)
	if !ok {
		t.Fatalf("custom_fields want object, got %T %v", doc["custom_fields"], doc["custom_fields"])
	}
	if mapped, _ := cf["mapped"].(float64); int(mapped) != 0 {
		t.Errorf("mapped = %v, want 0", cf["mapped"])
	}
	if has, _ := cf["raw_has_custom"].(bool); !has {
		t.Errorf("raw_has_custom = %v, want true", cf["raw_has_custom"])
	}
}

func TestDoctorCustomFieldsMappedSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceFieldUsage(context.Background(), []store.FieldUsageRow{
		{ProjectKey: "NMB", Alias: "story_points", Filled: 1, Total: 1},
		{ProjectKey: "NMA", Alias: "story_points", Filled: 0, Total: 2},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	const applied = "2026-08-21T12:00:00.000Z"
	cfg := &config.Config{
		Site:  "https://example.invalid",
		Email: "a@example.invalid",
		Token: "token",
		Fields: []config.FieldSpec{
			{Alias: "story_points", Label: "Story Points", IDs: []string{"customfield_1"}, Role: "plain"},
			{Alias: "severity", Label: "Severity", IDs: []string{"customfield_2"}, Role: "facet"},
		},
		FieldsAppliedAt: applied,
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	got := doctorValue(t, human, "custom_fields")
	for _, want := range []string{"2 aliases mapped", "applied " + applied, "usage rows 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("custom_fields %q missing %q", got, want)
		}
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	cf, ok := doc["custom_fields"].(map[string]any)
	if !ok {
		t.Fatalf("custom_fields want object, got %T %v", doc["custom_fields"], doc["custom_fields"])
	}
	if mapped, _ := cf["mapped"].(float64); int(mapped) != 2 {
		t.Errorf("mapped = %v, want 2", cf["mapped"])
	}
	if at, _ := cf["applied_at"].(string); at != applied {
		t.Errorf("applied_at = %q, want %q", at, applied)
	}
	if n, _ := cf["usage_rows"].(float64); int(n) != 2 {
		t.Errorf("usage_rows = %v, want 2", cf["usage_rows"])
	}
}

func TestDoctorSchemaAuditOKOnCleanMirror(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor must succeed on a clean mirror: %v\n%s", err, out)
	}
	if got := doctorValue(t, out, "schema_audit"); got != "ok" {
		t.Fatalf("schema_audit = %q, want ok", got)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.SchemaAudit == nil || rep.SchemaAudit.Status != "ok" {
		t.Fatalf("json schema_audit = %+v", rep.SchemaAudit)
	}
	if rep.SchemaAudit.Missing != 0 {
		t.Fatalf("json missing = %d, want 0", rep.SchemaAudit.Missing)
	}
}

// GDK-180: stamp matches this build so Open's migrate no-ops, but a table is
// gone. doctor names the damage and still exits 0 — same policy as
// not_found / schema_too_new / open_error (diagnosis, never a failed run).
func TestDoctorSchemaAuditDetectsFrankenstein(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	path := filepath.Join(home, "gadak.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stamp := db.SchemaVersion()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	rawSQL, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawSQL.Exec(`DROP TABLE versions`); err != nil {
		rawSQL.Close()
		t.Fatal(err)
	}
	if _, err := rawSQL.Exec("PRAGMA user_version = " + strconv.Itoa(stamp)); err != nil {
		rawSQL.Close()
		t.Fatal(err)
	}
	if err := rawSQL.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor must still exit 0 on a damaged mirror: %v\n%s", err, out)
	}
	got := doctorValue(t, out, "schema_audit")
	for _, want := range []string{
		"mismatch",
		"versions",
		"stamp=" + strconv.Itoa(stamp),
		"this_build=" + strconv.Itoa(stamp),
		"mirror is damaged",
		"delete the mirror file",
		"gadak sync",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("schema_audit missing %q: %q", want, got)
		}
	}

	js, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, js)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(js), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, js)
	}
	if rep.SchemaAudit == nil || rep.SchemaAudit.Status != "mismatch" {
		t.Fatalf("json schema_audit = %+v", rep.SchemaAudit)
	}
	if rep.SchemaAudit.Missing < 1 {
		t.Fatalf("json missing count = %d", rep.SchemaAudit.Missing)
	}
	found := false
	for _, s := range rep.SchemaAudit.Sample {
		if s == "versions" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("json sample = %v, want versions", rep.SchemaAudit.Sample)
	}
	for _, want := range []string{`"schema_audit"`, `"stamp"`, `"supported"`, `"missing"`, `"extra"`} {
		if !strings.Contains(js, want) {
			t.Errorf("JSON report missing %s:\n%s", want, js)
		}
	}
}
