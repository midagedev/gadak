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
	"time"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/applog"
	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/retro"
	"github.com/midagedev/gadak/internal/skillinstall"
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

// TestDoctorProjectsMismatch is FAIL-first for GDK-973: the standing report
// of the config↔mirror rename signature. Counts only — the keys themselves
// never enter a document the banner promises is safe to paste (the same rule
// TestDoctorRedaction enforces); `gadak status` and the sync warning name them.
func TestDoctorProjectsMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

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
				Key: "NMB-1", Title: "renamed upstream",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "NMB", IssueType: "Bug", IssueTypeID: "1",
				Status: "Open", StatusID: "3", StatusCategory: "inprogress",
			},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Site: "https://example.atlassian.net", Email: "someone@example.com", Token: "token",
		Projects: []string{"DI"}, // the stale side of the rename
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "projects_mismatch:") {
		t.Fatalf("human output missing projects_mismatch line:\n%s", out)
	}
	if !strings.Contains(out, "configured_not_mirrored=1") || !strings.Contains(out, "mirrored_not_configured=1") {
		t.Fatalf("projects_mismatch line missing counts:\n%s", out)
	}
	if strings.Contains(out, "NMB") {
		t.Errorf("project key leaked into the paste-safe document:\n%s", out)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	if strings.Contains(raw, "NMB") {
		t.Errorf("project key leaked into the JSON document: %s", raw)
	}
	var rep struct {
		ProjectsMismatch *struct {
			ConfiguredNotMirrored      int `json:"configured_not_mirrored"`
			MirroredNotConfigured      int `json:"mirrored_not_configured"`
			MirroredNotConfiguredIssue int `json:"mirrored_not_configured_issues"`
		} `json:"projects_mismatch"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.ProjectsMismatch == nil {
		t.Fatal("json missing projects_mismatch on the rename signature")
	}
	if got := *rep.ProjectsMismatch; got.ConfiguredNotMirrored != 1 || got.MirroredNotConfigured != 1 || got.MirroredNotConfiguredIssue != 1 {
		t.Fatalf("projects_mismatch = %+v, want 1/1/1", got)
	}

	// Control: config matching the mirror is not a mismatch — field omitted.
	cfg.Projects = []string{"NMB"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err = capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json (control): %v\n%s", err, raw)
	}
	rep.ProjectsMismatch = nil // Unmarshal keeps absent fields; reset before re-decoding
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.ProjectsMismatch != nil {
		t.Fatalf("projects_mismatch present with config matching the mirror: %+v", *rep.ProjectsMismatch)
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

// GDK-1170 layer ③: "why is my open board not showing the write I just made?"
// is one command. doctor reports the identity the ui-focus poll compares and
// when the mirror bytes last moved, so the answer is read rather than guessed.
//
// FAIL-first: before doctorMirror carried the pair, the JSON had no
// mirror.version and this stopped on "doctor must report mirror.version".
func TestDoctorReportsMirrorVersionAndAge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	db.Close()

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.Mirror.Version == "" {
		t.Fatalf("doctor must report mirror.version:\n%s", raw)
	}
	if rep.Mirror.ChangedAt == "" {
		t.Fatalf("doctor must report mirror.changed_at:\n%s", raw)
	}
	if _, err := time.Parse(time.RFC3339, rep.Mirror.ChangedAt); err != nil {
		t.Fatalf("mirror.changed_at %q is not RFC3339: %v", rep.Mirror.ChangedAt, err)
	}
	// The value doctor prints is the one the poll would have seen — doctor
	// opening the mirror must not be what moved it.
	if want := store.MirrorVersion(filepath.Join(home, "gadak.db")); rep.Mirror.Version != want {
		t.Fatalf("doctor moved the mirror by diagnosing it: reported %q, now %q", rep.Mirror.Version, want)
	}

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("human: %v", err)
	}
	if !strings.Contains(human, "mirror_version:") {
		t.Fatalf("human form missing mirror_version:\n%s", human)
	}
	if !strings.Contains(human, "moved ") {
		t.Fatalf("human mirror_version must say how long ago it moved:\n%s", human)
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
		// fakeSite is deliberately NOT a sample host: the unmasked site line
		// must show it (GDK-1631), while every other secret stays out. The
		// sample-host classification has its own test (TestDoctorFlagsSampleConfig).
		fakeSite  = "https://x.atlassian.net"
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

	// GDK-1631 (2026-09-10): the site host no longer leaks by definition — it
	// is the one value the document must be able to name when it is wrong, so
	// fakeSite moved from the forbidden list to the positive markers below.
	// The host inside the planted sync ERROR still must not appear: the error
	// body stays classified to "http NNN (kind)" even now that the site line
	// shows a hostname. Derivation: the issue's measured misdiagnosis (a
	// placeholder site read as an Atlassian outage for hours because doctor
	// masked the only surface that named it). FAIL-first for this rewrite is
	// in scratch/store/gate-1631-failfirst.txt.
	forbidden := []string{
		"example.atlassian.net", // only the seeded error body + source row carry it
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
		"site:                  x.atlassian.net",
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

// TestDoctorFlagsSampleConfig is FAIL-first for GDK-1631: the placeholder
// config.json the incident found (site example.atlassian.net, token
// "secret-token", email "someone@example.com") passed doctor as a fully
// configured workspace while every request died looking like an Atlassian
// outage. All three are sample literals; doctor now reports them as config
// errors instead of "present"/"configured", and the site line names the host
// so a wrong site is visible at all.
func TestDoctorFlagsSampleConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	cfg := &config.Config{
		Site:  "https://example.atlassian.net",
		Email: "someone@example.com",
		Token: "secret-token",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if got := doctorValue(t, out, "site"); got != "example.atlassian.net (sample placeholder — run gadak init)" {
		t.Fatalf("site = %q, want the sample-placeholder verdict", got)
	}
	if got := doctorValue(t, out, "credential"); got != "sample placeholder" {
		t.Fatalf("credential = %q, want sample placeholder", got)
	}
	if got := doctorValue(t, out, "email"); got != "sample placeholder" {
		t.Fatalf("email = %q, want sample placeholder", got)
	}
	// The sample token literal itself never appears — the classification is
	// the report, not the value.
	if strings.Contains(out, "secret-token") {
		t.Errorf("the sample token literal leaked:\n%s", out)
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

// TestDoctorReportsSkillLastAutoCheck — GDK-996. "Why did/didn't my skill
// update?" is one doctor line: the once-a-day stamp under the gadak home.
func TestDoctorReportsSkillLastAutoCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GADAK_HOME", home)
	config.SetProfile("")
	t.Chdir(home)

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	if got := doctorValue(t, human, "skill.last_auto_check"); got != "never" {
		t.Errorf("no stamp yet: last_auto_check = %q, want never", got)
	}

	p, err := skillAutoSyncStampPath()
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC().Truncate(time.Second)
	writeSkillAutoSyncStamp(p, when)
	human, err = capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor after a check: %v\n%s", err, human)
	}
	if got := doctorValue(t, human, "skill.last_auto_check"); got != when.Format(time.RFC3339) {
		t.Errorf("last_auto_check = %q, want the stamp %q", got, when.Format(time.RFC3339))
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var rep struct {
		Skill struct {
			LastAutoCheck string `json:"last_auto_check"`
		} `json:"skill"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.Skill.LastAutoCheck != when.Format(time.RFC3339) {
		t.Errorf("json last_auto_check = %q, want %q", rep.Skill.LastAutoCheck, when.Format(time.RFC3339))
	}
}

func TestDoctorReportsBuiltInWithCredential(t *testing.T) {
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
	// kind still reports the stored value (GDK-1280 added origin_type and
	// transport beside it rather than replacing it).
	if !strings.Contains(got, "kind=standalone") {
		t.Errorf("workspace line missing kind=standalone: %q", got)
	}
	if !strings.Contains(got, "site_token=yes") {
		t.Errorf("workspace line missing site_token=yes: %q", got)
	}
	if !strings.Contains(got, "inconsistent") {
		t.Errorf("built-in-with-token must say inconsistent: %q", got)
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

// TestSiteReport — GDK-1631 (2026-09-10 rewrite of TestRedactSite). The
// masking hid the one value whose wrongness explained every symptom: a
// placeholder site answered every request with 404 "Site temporarily
// unavailable" for hours while doctor kept saying "<redacted>.atlassian.net".
// A hostname names the server, not the user, so the site line shows it — and
// the reserved sample hosts say so as a config error, not a site. Gate
// modification triple: attribution here, derivation in the issue
// (misdiagnosis measured 2026-09-08), FAIL-first in scratch/store/
// gate-1631-failfirst.txt (this table ran red against the masking code).
func TestSiteReport(t *testing.T) {
	cases := map[string]string{
		"":                                "none",
		"https://x.atlassian.net":         "x.atlassian.net",
		"https://x.atlassian.net/":        "x.atlassian.net",
		"https://jira.acme.com:8443/x":    "jira.acme.com",
		"https://user:secretpw@acme.net":  "acme.net",        // userinfo never shown
		"x.atlassian.net":                 "x.atlassian.net", // bare host, no scheme
		"https://example.atlassian.net":   "example.atlassian.net (sample placeholder — run gadak init)",
		"your-site.atlassian.net":         "your-site.atlassian.net (sample placeholder — run gadak init)",
		"https://Your-Site.Atlassian.Net": "your-site.atlassian.net (sample placeholder — run gadak init)",
	}
	for in, want := range cases {
		if got := siteReport(in); got != want {
			t.Errorf("siteReport(%q) = %q, want %q", in, got, want)
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

// TestDoctorConfluenceSpacesNotOnOrigin is FAIL-first for GDK-1484: a
// configured wiki space the origin never resolved leaves no row in the
// spaces catalog, so the mirror itself can answer "this key mirrors
// nothing" without a network call. Counts only — the key stays out of the
// paste-safe document (TestDoctorRedaction's rule); `gadak status` names it.
func TestDoctorConfluenceSpacesNotOnOrigin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")

	ctx := context.Background()
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(ctx, store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://example.atlassian.net/wiki"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	// The catalog the pass wrote: only the space the origin actually has.
	if err := db.UpsertSpaces(ctx, "confluence", []store.SpaceRow{{Key: "GDK", Name: "Gadak", Kind: "global"}}); err != nil {
		t.Fatalf("spaces: %v", err)
	}
	// A pass has run — without that the mirror cannot answer at all.
	if err := db.RecordSync(ctx, "confluence", store.SyncResult{}); err != nil {
		t.Fatalf("record sync: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Site: "https://example.atlassian.net", Email: "someone@example.com", Token: "token",
		// The stale built-in default that survived a change of origin.
		Confluence: &config.ConfluenceConfig{Spaces: []string{"LOCSTALE"}},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "confluence_spaces:") {
		t.Fatalf("human output missing confluence_spaces line:\n%s", out)
	}
	if !strings.Contains(out, "configured=1") || !strings.Contains(out, "not_on_origin=1") {
		t.Fatalf("confluence_spaces line missing counts:\n%s", out)
	}
	if strings.Contains(out, "LOCSTALE") {
		t.Errorf("space key leaked into the paste-safe document:\n%s", out)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	if strings.Contains(raw, "LOCSTALE") {
		t.Errorf("space key leaked into the JSON document: %s", raw)
	}
	var rep struct {
		ConfluenceSpaces *struct {
			Configured  int `json:"configured"`
			NotOnOrigin int `json:"not_on_origin"`
		} `json:"confluence_spaces"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.ConfluenceSpaces == nil {
		t.Fatal("json missing confluence_spaces on a scope the origin does not have")
	}
	if got := *rep.ConfluenceSpaces; got.Configured != 1 || got.NotOnOrigin != 1 {
		t.Fatalf("confluence_spaces = %+v, want 1/1", got)
	}

	// Control: a scope the catalog knows is not a mismatch — field omitted.
	cfg.Confluence = &config.ConfluenceConfig{Spaces: []string{"GDK"}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw2, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw2)
	}
	if strings.Contains(raw2, "confluence_spaces") {
		t.Errorf("confluence_spaces present when every configured key is in the catalog: %s", raw2)
	}
}

// TestDoctorFindsACodexOnlyInstall — GDK-1508. doctor used to look at
// ~/.claude and nowhere else, so a user who had correctly installed the skill
// for Codex was told "missing" and, following that advice, installed it a
// second time for an agent they do not run.
//
// CODEX_HOME is cleared rather than pointed at a scratch directory: the home
// here is what contains the test, and orca's tripwire records the Codex binary
// ignoring the equivalent sandbox on Windows.
func TestDoctorFindsACodexOnlyInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GADAK_HOME", home)
	t.Setenv("CODEX_HOME", "")
	config.SetProfile("")
	t.Chdir(home)

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	if got := doctorValue(t, human, "skill"); !strings.HasPrefix(got, "missing") {
		t.Fatalf("empty home: skill = %q, want missing", got)
	}

	dest := filepath.Join(home, ".codex", "skills", "gadak", "SKILL.md")
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
		t.Errorf("codex-only install: skill = %q, want current", got)
	}
	if !strings.Contains(human, "codex") {
		t.Errorf("the skill line should name the host it found:\n%s", human)
	}
}

// TestDoctorReportsEverySkillHost — the per-host array, and the rule that keeps
// the summary line one line: a host with a copy or a configuration directory
// gets a row, a host that has neither is left out rather than listed as
// missing.
func TestDoctorReportsEverySkillHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GADAK_HOME", home)
	t.Setenv("CODEX_HOME", "")
	config.SetProfile("")
	t.Chdir(home)

	// claude: current. codex: one byte behind, and no receipt, so the
	// installer calls it a conflict — the file is the user's.
	// agents: present on the machine, nothing installed.
	// grok, cursor, gemini, opencode: not on this machine at all.
	writeAt := func(rel string, body []byte) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeAt(filepath.Join(".claude", "skills", "gadak", "SKILL.md"), gadak.SkillMarkdown())
	writeAt(filepath.Join(".codex", "skills", "gadak", "SKILL.md"), append(append([]byte{}, gadak.SkillMarkdown()...), '\n'))
	writeAt(filepath.Join(".agents", "marker"), []byte("x"))

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	got := map[string]doctorSkillHost{}
	for _, h := range rep.Skill.Hosts {
		got[h.Client] = h
	}
	if len(got) != 3 {
		t.Fatalf("hosts = %+v, want claude, codex and agents only", rep.Skill.Hosts)
	}
	if h := got["claude"]; h.Status != "current" || h.Scope != "user" || h.Path != "~/.claude/skills/gadak/SKILL.md" {
		t.Errorf("claude host = %+v", h)
	}
	// conflict, not stale: nothing gadak wrote is at that path, so gadak will
	// not replace it without --force. The summary line folds it into stale.
	if h := got["codex"]; h.Status != "conflict" || h.Path != "~/.codex/skills/gadak/SKILL.md" {
		t.Errorf("codex host = %+v", h)
	}
	if h := got["agents"]; h.Status != "missing" || h.Path != "~/.agents/skills/gadak/SKILL.md" {
		t.Errorf("agents host = %+v", h)
	}
	if !strings.Contains(raw, `"hosts"`) || !strings.Contains(raw, `"client"`) {
		t.Errorf("JSON report missing the hosts array:\n%s", raw)
	}

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatal(err)
	}
	line := doctorValue(t, human, "skill")
	if !strings.Contains(line, "current (claude)") || !strings.Contains(line, "stale (codex)") || !strings.Contains(line, "missing (agents)") {
		t.Errorf("summary line = %q, want the hosts grouped by verdict", line)
	}
	if strings.Contains(line, "\n") {
		t.Errorf("the summary must stay one line: %q", line)
	}
}

// TestDoctorFindsAProjectScopeCodexInstall — .agents/skills is what Codex walks
// up to find, so a repo that carries the skill counts as installed even with an
// empty home.
func TestDoctorFindsAProjectScopeCodexInstall(t *testing.T) {
	home := t.TempDir()
	proj := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GADAK_HOME", home)
	t.Setenv("CODEX_HOME", "")
	config.SetProfile("")
	t.Chdir(proj)

	dest := filepath.Join(proj, ".agents", "skills", "gadak", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, gadak.SkillMarkdown(), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatal(err)
	}
	var codex *doctorSkillHost
	for i := range rep.Skill.Hosts {
		if rep.Skill.Hosts[i].Client == "codex" {
			codex = &rep.Skill.Hosts[i]
		}
	}
	if codex == nil {
		t.Fatalf("no codex row: %+v", rep.Skill.Hosts)
	}
	if codex.Status != "current" || codex.Scope != "project" {
		t.Errorf("codex host = %+v, want a current project install", codex)
	}
	// The path is the literal relative one: the banner promises this report is
	// safe to paste, and an absolute path would carry the project's name.
	if codex.Path != filepath.Join(".agents", "skills", "gadak", "SKILL.md") {
		t.Errorf("project path = %q, want the literal relative path", codex.Path)
	}
	if strings.Contains(raw, proj) {
		t.Errorf("the report leaked the working directory:\n%s", raw)
	}
}

// TestDoctorNamesADevTreeSkillCopy — GDK-1531. `current` alone was what let an
// uncommitted working-tree SKILL.md sit in the developer's agent home looking
// like a shipped release: doctor compared bytes against the running binary and
// both came from the same checkout, so of course they matched. The receipt
// beside the file is the only record of which kind of binary wrote it, and
// doctor now reads it — in the JSON per host, and in the summary line.
func TestDoctorNamesADevTreeSkillCopy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GADAK_HOME", home)
	t.Setenv("CODEX_HOME", "")
	config.SetProfile("")
	t.Chdir(home)

	dir := filepath.Join(home, ".claude", "skills", "gadak")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(dest, gadak.SkillMarkdown(), 0o644); err != nil {
		t.Fatal(err)
	}
	// The receipt a checkout build leaves: the digest of the bytes on disk,
	// the dev version, and the tree it came from.
	receipt := skillinstall.Receipt{
		SHA256:       skillinstall.Digest(gadak.SkillMarkdown()),
		GadakVersion: skillinstall.DevVersion,
		InstalledAt:  "2026-09-07T09:12:34Z",
		Source:       skillinstall.SourceDevTree,
		Revision:     "ac8e154+",
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillinstall.ReceiptName), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	var claude *doctorSkillHost
	for i := range rep.Skill.Hosts {
		if rep.Skill.Hosts[i].Client == "claude" {
			claude = &rep.Skill.Hosts[i]
		}
	}
	if claude == nil {
		t.Fatalf("no claude row: %+v", rep.Skill.Hosts)
	}
	if claude.Status != "current" {
		t.Errorf("status = %q, want current — the bytes really do match", claude.Status)
	}
	if claude.Source != skillinstall.SourceDevTree {
		t.Errorf("source = %q, want %q", claude.Source, skillinstall.SourceDevTree)
	}
	if claude.InstalledByVersion != skillinstall.DevVersion {
		t.Errorf("installed_by_version = %q, want %q", claude.InstalledByVersion, skillinstall.DevVersion)
	}
	if claude.Revision != "ac8e154+" {
		t.Errorf("revision = %q, want ac8e154+", claude.Revision)
	}
	if line := formatDoctorSkill(rep.Skill); !strings.Contains(line, "dev-tree ac8e154+") {
		t.Errorf("summary line hides the dev-tree copy: %q", line)
	}
}

// TestDoctorSkillReceiptMustDescribeTheFileOnDisk — a receipt belongs to the
// bytes it names. After a hand edit the receipt beside the file is the
// *previous* copy's, and reporting its provenance as this file's would be the
// same lie in the other direction (GDK-1531).
func TestDoctorSkillReceiptMustDescribeTheFileOnDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GADAK_HOME", home)
	t.Setenv("CODEX_HOME", "")
	config.SetProfile("")
	t.Chdir(home)

	dir := filepath.Join(home, ".claude", "skills", "gadak")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := []byte("---\nname: gadak\ndescription: my own house rules\n---\n\nask me first\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), mine, 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(skillinstall.Receipt{
		SHA256:       skillinstall.Digest(gadak.SkillMarkdown()), // the copy this replaced
		GadakVersion: skillinstall.DevVersion,
		Source:       skillinstall.SourceDevTree,
		Revision:     "ac8e154+",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skillinstall.ReceiptName), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v", err)
	}
	var rep doctorReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	for _, h := range rep.Skill.Hosts {
		if h.Client != "claude" {
			continue
		}
		if h.Status != "conflict" {
			t.Errorf("status = %q, want conflict", h.Status)
		}
		if h.Source != "" || h.InstalledByVersion != "" || h.Revision != "" {
			t.Errorf("a stale receipt was reported as this file's provenance: %+v", h)
		}
	}
	if line := formatDoctorSkill(rep.Skill); strings.Contains(line, "dev-tree") {
		t.Errorf("summary line claims provenance it does not have: %q", line)
	}
}

// A registration that lives only in Claude Desktop's own config is still a
// registration. doctor read Claude Code's two files and nothing else, so it
// answered "absent" to exactly the host `gadak mcp install claude-desktop`
// exists for (GDK-1643).
func TestCollectMCPStatusClaudeDesktopOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", "")
	// Somewhere with no .mcp.json, so the project branch cannot answer.
	t.Chdir(t.TempDir())

	if got := collectMCPStatus(); got.Status != "absent" {
		t.Fatalf("nothing registered anywhere: %+v", got)
	}

	// Ask the path's owner rather than spelling it out: the file sits in a
	// different place per OS and this test runs on more than one. A literal
	// darwin path here would be green on a Mac and red on CI — the shape of
	// run 34233540027.
	path, err := clitool.ClaudeDesktopConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const reg = `{"mcpServers":{"gadak":{"command":"gadak","args":["mcp"]}}}`
	if err := os.WriteFile(path, []byte(reg), 0o600); err != nil {
		t.Fatal(err)
	}
	got := collectMCPStatus()
	if got.Status != "registered" || got.Scope != "claude-desktop" {
		t.Errorf("claude-desktop only = %+v, want registered/claude-desktop", got)
	}
	if got.Path != tildeHome(path) {
		t.Errorf("path = %q, want %q", got.Path, tildeHome(path))
	}

	// Claude Code's own registration still answers first when both exist:
	// the new branch is a fallback, not a reordering.
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(reg), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := collectMCPStatus(); got.Scope != "user" {
		t.Errorf("both registered = %+v, want scope user", got)
	}
}

// TestDoctorCountsSplitCommentsByMeaning is GDK-1113 (same owners as
// `gadak status --json`, GDK-628): the shared comments table mixes issue
// and wiki comments, so doctor labels each share by meaning instead of one
// "comments" row that disagrees with the settings runtime on every mirror
// with wiki comments. Seeds 2 issue comments and 1 page comment — a mixed
// figure of 3 would be wrong for both meanings.
func TestDoctorCountsSplitCommentsByMeaning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()
	if err := db.UpsertSource(ctx, store.Source{ID: "jira", Kind: "jira", BaseURL: "https://192.0.2.10"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(ctx, store.Source{ID: "confluence", Kind: "confluence", BaseURL: "https://192.0.2.10/wiki"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertIssues(ctx, store.Batch{
		Categories: map[string]string{"1": "new"},
		Records: []store.IssueRecord{{
			Item: store.Item{
				ID: "jira:1", SourceID: "jira", Kind: "issue", ExternalID: "1",
				Key: "STD-1", Title: "seeding doctor counts",
				CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z",
			},
			Issue: store.Issue{
				ProjectKey: "STD", IssueType: "Bug", IssueTypeID: "10004",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
			Comments: []store.Comment{
				{ID: "jira:c-1", Author: "Dana", BodyText: "repro", CreatedAt: "2026-01-02T00:00:00.000Z"},
				{ID: "jira:c-2", Author: "Lee", BodyText: "confirmed", CreatedAt: "2026-01-03T00:00:00.000Z"},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertPages(ctx, []store.PageRecord{{
		Item: store.Item{
			ID: "confluence:100", SourceID: "confluence", Kind: "page", ExternalID: "100",
			Key: "100", Title: "회의록", BodyText: "논의 사항",
			CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-02T00:00:00.000Z",
		},
		Page: store.Page{SpaceKey: "ENG", Version: 1, Status: "current"},
		Comments: []store.Comment{{
			ID: "confluence:c-1", Author: "Kim", BodyText: "ok",
			CreatedAt: "2026-01-02T00:00:00.000Z",
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if got := doctorValue(t, out, "issue_comments"); got != "2" {
		t.Errorf("issue_comments = %q, want 2 (the issue share of the shared table):\n%s", got, out)
	}
	if got := doctorValue(t, out, "page_comments"); got != "1" {
		t.Errorf("page_comments = %q, want 1 (the wiki share):\n%s", got, out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "comments:") {
			t.Errorf("a mixed comments row is back — it disagrees with the settings runtime on every mirror with wiki comments:\n%s", line)
		}
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var rep struct {
		Counts *struct {
			IssueComments int `json:"issue_comments"`
			PageComments  int `json:"page_comments"`
		} `json:"counts"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.Counts == nil {
		t.Fatalf("no counts in:\n%s", raw)
	}
	if rep.Counts.IssueComments != 2 || rep.Counts.PageComments != 1 {
		t.Errorf("counts = issue_comments:%d page_comments:%d, want 2/1:\n%s",
			rep.Counts.IssueComments, rep.Counts.PageComments, raw)
	}
	var mixed map[string]any
	if err := json.Unmarshal([]byte(raw), &mixed); err != nil {
		t.Fatal(err)
	}
	if counts, ok := mixed["counts"].(map[string]any); ok {
		if _, still := counts["comments"]; still {
			t.Errorf("counts still carries the mixed \"comments\" key:\n%s", raw)
		}
	}
}

/* ── GDK-307 / GDK-1413 / GDK-1549: doctor as the consumer of the store
   diagnostics the w11-server round landed (MirrorSidecarBytes,
   OpenPriorityDistribution, LastSessionEnd). The tests decode into
   anonymous structs so they compile against the unwired source and fail at
   runtime — that output is the FAIL-first evidence for the wiring. ── */

// seedDoctorMirror opens a throwaway home's mirror, registers the source a
// sync would have left, and closes it — leaving gadak.db on disk with no
// sidecars (a clean close checkpoints -wal away, which is the state a user
// who just quit gadak is in).
func seedDoctorMirror(t *testing.T, home string) {
	t.Helper()
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestDoctorReportsWALSidecars — GDK-307: the -wal/-shm sizes beside the
// mirror are a doctor line, not a shell incantation. store.Open mints both
// sidecars and nothing caps the -wal, so a serve that never closes can
// starve checkpointing; the visible symptom is a sidecar growing beside a
// mirror whose bytes barely moved. doctor stats them before it opens the
// mirror (the same ordering note collectDoctor carries for Version), so the
// number is what an open board's filesystem showed a moment ago.
//
// FAIL-first: against the unwired source the JSON carries no wal_bytes and
// the human form has no mirror_wal line.
func TestDoctorReportsWALSidecars(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	seedDoctorMirror(t, home)

	// Sidecars with known lengths: this test pins the reporting, not
	// SQLite's checkpoint policy.
	const walLen, shmLen = 4096, 512
	wal := make([]byte, walLen)
	for i := range wal {
		wal[i] = 'w'
	}
	if err := os.WriteFile(filepath.Join(home, "gadak.db-wal"), wal, 0o600); err != nil {
		t.Fatal(err)
	}
	shm := make([]byte, shmLen)
	if err := os.WriteFile(filepath.Join(home, "gadak.db-shm"), shm, 0o600); err != nil {
		t.Fatal(err)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var rep struct {
		Mirror struct {
			Wal *int64 `json:"wal_bytes"`
			Shm *int64 `json:"shm_bytes"`
		} `json:"mirror"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.Mirror.Wal == nil || *rep.Mirror.Wal != walLen {
		t.Fatalf("wal_bytes = %v, want %d:\n%s", rep.Mirror.Wal, walLen, raw)
	}
	if rep.Mirror.Shm == nil || *rep.Mirror.Shm != shmLen {
		t.Fatalf("shm_bytes = %v, want %d:\n%s", rep.Mirror.Shm, shmLen, raw)
	}

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	if got := doctorValue(t, human, "mirror_wal"); got == "" {
		t.Fatalf("human form missing mirror_wal:\n%s", human)
	} else if !strings.Contains(got, "shm") {
		t.Fatalf("mirror_wal %q does not name the shm side too:\n%s", got, human)
	}

	// Control: sidecars gone (the clean-close state) reads as zero — "no
	// -wal right now", which is itself the answer to "is checkpointing
	// starved?".
	for _, side := range []string{"gadak.db-wal", "gadak.db-shm"} {
		// doctor's own opens above may already have checkpointed the fake
		// sidecar away — absence is the state this control asserts.
		if err := os.Remove(filepath.Join(home, side)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	raw, err = capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json (control): %v\n%s", err, raw)
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.Mirror.Wal == nil || *rep.Mirror.Wal != 0 {
		t.Fatalf("wal_bytes = %v, want an explicit 0 when no sidecar exists:\n%s", rep.Mirror.Wal, raw)
	}
}

// seedDoctorPriorities writes n open issues per priority display name; the
// site list maps High→rank 2, Low→rank 4 (derive.go priorityRank, index+1).
func seedDoctorPriorities(t *testing.T, home string, high, low int) {
	t.Helper()
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.UpsertSource(context.Background(), store.Source{ID: "jira", Kind: "jira", BaseURL: "https://example.atlassian.net"}); err != nil {
		t.Fatalf("source: %v", err)
	}
	var recs []store.IssueRecord
	n := 0
	add := func(prio string, count int) {
		for i := 0; i < count; i++ {
			n++
			recs = append(recs, store.IssueRecord{
				Item: store.Item{
					ID: "jira:" + strconv.Itoa(n), SourceID: "jira", Kind: "issue",
					ExternalID: strconv.Itoa(n), Key: "STD-" + strconv.Itoa(n),
					Title: "seed", CreatedAt: "2026-01-01T00:00:00.000Z", UpdatedAt: "2026-01-01T00:00:00.000Z",
				},
				Issue: store.Issue{
					ProjectKey: "STD", IssueType: "Bug", IssueTypeID: "1",
					Status: "Open", StatusID: "3", StatusCategory: "inprogress",
					Priority: prio,
				},
			})
		}
	}
	add("High", high)
	add("Low", low)
	if _, err := db.UpsertIssues(context.Background(), store.Batch{
		Categories: map[string]string{"3": "inprogress"},
		Priorities: []string{"Highest", "High", "Medium", "Low"},
		Records:    recs,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestDoctorPriorityConcentrationLine — GDK-1413: the census sentence ("80%
// of 10 open issues are priority rank 2") is a doctor line. The store owns
// the query and the 70% threshold (diagnostics.go DominantSharePct,
// attribution there); doctor states the fact Concentrated returns — counts
// and a share, no verdict, no keys, the paste-safe rule every section
// follows. Nil when the values are spread or nothing is open, the healthy
// shapes both.
//
// FAIL-first: the unwired JSON carries no priority_entropy and the human
// form has no priority line.
func TestDoctorPriorityConcentrationLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	seedDoctorPriorities(t, home, 8, 2) // 8 High (rank 2) of 10 open = 80%

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	if strings.Contains(raw, "STD-") {
		t.Fatalf("issue key leaked into the paste-safe document:\n%s", raw)
	}
	var rep struct {
		PriorityEntropy *struct {
			Rank  int `json:"rank"`
			Count int `json:"count"`
			Pct   int `json:"pct"`
			Open  int `json:"open"`
		} `json:"priority_entropy"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.PriorityEntropy == nil {
		t.Fatalf("concentrated priorities (80%% of 10) missing priority_entropy:\n%s", raw)
	}
	if got := *rep.PriorityEntropy; got.Rank != 2 || got.Count != 8 || got.Pct != 80 || got.Open != 10 {
		t.Fatalf("priority_entropy = %+v, want rank 2 count 8 pct 80 open 10", got)
	}

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	if !strings.Contains(human, "80% of 10 open issues") || !strings.Contains(human, "rank 2") {
		t.Fatalf("human form missing the census sentence:\n%s", human)
	}

	// Control: values spread 5/5 — no line, the healthy shape.
	spread := t.TempDir()
	t.Setenv("GADAK_HOME", spread)
	t.Setenv("HOME", spread)
	seedDoctorPriorities(t, spread, 5, 5)
	raw, err = capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json (control): %v\n%s", err, raw)
	}
	rep.PriorityEntropy = nil // Unmarshal keeps absent fields; reset before re-decoding
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.PriorityEntropy != nil {
		t.Fatalf("spread priorities still carry priority_entropy: %+v", *rep.PriorityEntropy)
	}
}

// TestDoctorLocalSchemaSkew — GDK-596: the standing detail behind the short
// local.db notice. A newer gadak migrated personal history further than this
// build reads; every command logs one short pointer line, and this is the
// line the pointer names — the versions, the file, and the remediation the
// notice dropped. Version 99 is a planted future stamp: the cmd package
// cannot see store's localMigrations length, and the skew verdict is
// "version ahead of supported", which holds for any supported value.
//
// FAIL-first: the unwired JSON carries no local_schema_skew.
func TestDoctorLocalSchemaSkew(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	seedDoctorMirror(t, home) // leaves local.db at this build's version

	local, err := sql.Open("sqlite", "file:"+filepath.Join(home, "local.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Exec("PRAGMA user_version = 99"); err != nil {
		local.Close()
		t.Fatal(err)
	}
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var rep struct {
		LocalSchemaSkew *struct {
			Version   int    `json:"version"`
			Supported int    `json:"supported"`
			Path      string `json:"path"`
		} `json:"local_schema_skew"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.LocalSchemaSkew == nil {
		t.Fatalf("a skewed local.db but no local_schema_skew:\n%s", raw)
	}
	if got := rep.LocalSchemaSkew; got.Version != 99 || got.Supported <= 0 || got.Version <= got.Supported {
		t.Fatalf("local_schema_skew = %+v, want version 99 ahead of supported", got)
	}
	if !strings.Contains(rep.LocalSchemaSkew.Path, "local.db") {
		t.Fatalf("local_schema_skew.path = %q, want the local.db path", rep.LocalSchemaSkew.Path)
	}

	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	got := doctorValue(t, human, "local_schema_skew")
	for _, want := range []string{"99", "upgrade gadak", "--workspace", "GADAK_HOME"} {
		if !strings.Contains(got, want) {
			t.Errorf("local_schema_skew %q missing %q — the remediation the short notice dropped lives here", got, want)
		}
	}

	// Control: this build's own local.db (seedDoctorMirror just left one at
	// the current version) — no section, the healthy shape.
	fresh := t.TempDir()
	t.Setenv("GADAK_HOME", fresh)
	t.Setenv("HOME", fresh)
	seedDoctorMirror(t, fresh)
	raw, err = capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json (control): %v\n%s", err, raw)
	}
	rep.LocalSchemaSkew = nil // Unmarshal keeps absent fields; reset before re-decoding
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.LocalSchemaSkew != nil {
		t.Fatalf("a current local.db still carries local_schema_skew: %+v", *rep.LocalSchemaSkew)
	}
}

// seedDoctorVisit rows one person read with an explicit stamp into home's
// local.db — a third connection, no ATTACH, the road session_strip_test.go
// takes (RecordVisit stamps Now() and cannot express "two hours ago").
func seedDoctorVisit(t *testing.T, home string, at time.Time, source string) {
	t.Helper()
	local, err := sql.Open("sqlite", "file:"+filepath.Join(home, "local.db"))
	if err != nil {
		t.Fatalf("open local.db: %v", err)
	}
	defer local.Close()
	if _, err := local.Exec(
		`INSERT INTO visits (kind, key, viewed_at, source) VALUES (?,?,?,?)`,
		store.VisitKindIssue, "NMB-1", at.UTC().Format(config.ISOMilli), source); err != nil {
		t.Fatalf("seed visit at %s: %v", at, err)
	}
}

// TestDoctorSessionBoundary — GDK-1549: the boundary the bootstrap probe
// header serves (X-Gadak-Session-Boundary) is also a doctor line, so "when
// did my last session end?" is answerable with the serve down. The owner is
// db.LastSessionEnd with retro.SessionGap; doctor formats the same
// ISOMilli-UTC string read.go serves, and this test cross-checks the two
// readers against each other so they cannot drift.
//
// FAIL-first: the unwired JSON carries no session_boundary.
func TestDoctorSessionBoundary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	seedDoctorMirror(t, home)
	now := time.Now().UTC()
	// Session 1 (2h back), a read in the current session (1m back): the
	// boundary is session 1's read; the 1m read must not move it.
	prev := now.Add(-2 * time.Hour)
	seedDoctorVisit(t, home, prev, store.VisitSourceUI)
	seedDoctorVisit(t, home, now.Add(-time.Minute), store.VisitSourceUI)

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, raw)
	}
	var rep struct {
		SessionBoundary string `json:"session_boundary"`
	}
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.SessionBoundary == "" {
		t.Fatalf("a previous session on record but no session_boundary:\n%s", raw)
	}
	if got, err := time.Parse(config.ISOMilli, rep.SessionBoundary); err != nil {
		t.Fatalf("session_boundary %q is not ISOMilli: %v", rep.SessionBoundary, err)
	} else if d := got.Sub(prev); d < -time.Second || d > time.Second {
		t.Fatalf("session_boundary = %s, want the previous session's end %s", rep.SessionBoundary, prev.Format(config.ISOMilli))
	}

	// Cross-check: the same reader the probe header uses, on the same home.
	db, err := store.Open(filepath.Join(home, "gadak.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	end, err := db.LastSessionEnd(context.Background(), time.Now(), retro.SessionGap)
	if err != nil || end == nil {
		t.Fatalf("LastSessionEnd: %v %v", end, err)
	}
	if want := end.UTC().Format(config.ISOMilli); rep.SessionBoundary != want {
		t.Fatalf("doctor session_boundary %q != the header's own value %q", rep.SessionBoundary, want)
	}

	// Control: no visits at all — the key is absent, not empty-quoted.
	quiet := t.TempDir()
	t.Setenv("GADAK_HOME", quiet)
	t.Setenv("HOME", quiet)
	seedDoctorMirror(t, quiet)
	raw, err = capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json (control): %v\n%s", err, raw)
	}
	rep.SessionBoundary = ""
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if rep.SessionBoundary != "" {
		t.Fatalf("session_boundary = %q with no visits on record", rep.SessionBoundary)
	}

	// The human form carries the line too.
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	human, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, human)
	}
	if !strings.Contains(human, "session_boundary:") {
		t.Fatalf("human form missing session_boundary:\n%s", human)
	}
}

// TestDoctorReportsPairingVersionSkew is FAIL-first for GDK-1273's doctor
// half: the paste-into-a-bug-report document names both gadak versions of a
// paired workspace, so a skew report is a copy of one line instead of an
// archaeology dig. The credential is planted raw — the on-disk contract an
// older pair already wrote.
func TestDoctorReportsPairingVersionSkew(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })

	cred := map[string]any{
		"endpoint":      "http://192.0.2.10:7877",
		"token":         "pair-token",
		"label":         "laptop",
		"serverVersion": "0.19.1",
	}
	b, err := json.Marshal(cred)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "remote-origin.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}

	raw, err := capture(t, func() error { return cmdDoctor([]string{"--json"}) })
	if err != nil {
		t.Fatalf("doctor --json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	skew, _ := doc["pairing_skew"].(string)
	if skew == "" {
		t.Fatalf("doctor must report pairing_skew for a paired workspace, got %s", raw)
	}
	if !strings.Contains(skew, "0.19.1") {
		t.Fatalf("pairing_skew must name the home serve's version, got %q", skew)
	}

	text, err := capture(t, func() error { return cmdDoctor(nil) })
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(text, "pairing skew") || !strings.Contains(text, "0.19.1") {
		t.Fatalf("doctor text must carry a pairing skew line naming the version, got:\n%s", text)
	}
}
