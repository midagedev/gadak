package main

// Contract ↔ assertion map for `gadak dashboards` (GDK-781):
//
//	contract clause                    | assertions
//	-----------------------------------+-----------------------------------------------
//	save→list→show→rm round trip       | TestDashboardsRoundTrip (id stable, TSV shape, json shape)
//	save works serverless (local.db)   | TestDashboardsRoundTrip (no serve in the demo home)
//	--datasource grammar is enforced   | TestDashboardsSaveFlagErrors (no prefix, no =, bad name)
//	config validation before any write | TestDashboardsSaveFlagErrors (empty html saves nothing)
//	open writes hash dash=<id>         | TestDashboardsOpenWritesFocusHash (--no-open, one-shot file)
//	help registered, dispatches        | TestDashboardsHelp (parity with commands map is a separate standing test)
//	lib add prints the evidence        | TestDashboardsLibCLI (url/sha384/size/path, sha384 verified)
//	lib add loopback http only         | TestDashboardsLibCLI (external http refused, nothing cached)
//	save --lib: unknown id refused     | TestDashboardsLibCLI (recipe in the error, row not written)
//	lib list/rm round trip             | TestDashboardsLibCLI (manifest row in, rm drops it)

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/dashboards"
	"github.com/midagedev/gadak/internal/uifocus"
)

func writeHTML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "wall.html")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("html fixture: %v", err)
	}
	return p
}

func TestDashboardsRoundTrip(t *testing.T) {
	sqlDemoHome(t)
	html := writeHTML(t, "<!doctype html><html><body><h1>triage</h1></body></html>")

	out, err := capture(t, func() error {
		return cmdDashboards([]string{
			"save", "Triage Wall",
			"--html", html,
			"--datasource", "open_by_status=sql:select status_category, count(*) as n from issues_full where status_category != 'done' group by 1",
			"--datasource", "mine=jql:assignee = currentUser() AND resolution is EMPTY",
		})
	})
	if err != nil {
		t.Fatalf("save: %v\n%s", err, out)
	}
	id := field(t, out, "saved")

	// list: one TSV row id/name/updated_at.
	out, err = capture(t, func() error { return cmdDashboards([]string{"list"}) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	cols := strings.Fields(out) // id, name, updated_at — name has a space
	if len(cols) < 3 || !strings.HasPrefix(out, id+"\t") || !strings.Contains(out, "Triage Wall") {
		t.Fatalf("list row = %q, want id %s + name", out, id)
	}

	// show: identity fields plus both datasources with their kinds.
	out, err = capture(t, func() error { return cmdDashboards([]string{"show", "triage"}) })
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	for _, want := range []string{
		"id\t" + id, "name\tTriage Wall", "html_bytes\t56",
		"datasource\tmine\tjql\tassignee = currentUser() AND resolution is EMPTY",
		// The sql line is compactJQL-truncated at 80 columns, so assert a
		// leading slice, not the whole statement.
		"datasource\topen_by_status\tsql\tselect status_category, count(*) as n from issues_full",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q:\n%s", want, out)
		}
	}

	// show --json round-trips the stored config verbatim.
	out, err = capture(t, func() error { return cmdDashboards([]string{"show", "triage", "--json"}) })
	if err != nil {
		t.Fatalf("show --json: %v\n%s", err, out)
	}
	var row struct {
		ID     string          `json:"id"`
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if row.ID != id || !strings.Contains(string(row.Config), "open_by_status") {
		t.Fatalf("json row = %s", out)
	}

	// Same-name save updates (stable id) — the saved-views convention.
	out, err = capture(t, func() error {
		return cmdDashboards([]string{"save", "Triage Wall", "--html", html})
	})
	if err != nil {
		t.Fatalf("re-save: %v\n%s", err, out)
	}
	if field(t, out, "saved") != id {
		t.Fatalf("re-save minted a new id: %s", out)
	}

	// rm, then the list is empty and show names what exists.
	out, err = capture(t, func() error { return cmdDashboards([]string{"rm", "triage"}) })
	if err != nil {
		t.Fatalf("rm: %v\n%s", err, out)
	}
	if !strings.HasPrefix(out, "deleted\t"+id) {
		t.Fatalf("rm output = %q", out)
	}
	_, _, err = captureErr(t, func() error { return cmdDashboards([]string{"show", "triage"}) })
	if err == nil || !strings.Contains(err.Error(), "none saved") {
		t.Fatalf("show after rm = %v, want none-saved diagnosis", err)
	}
}

func field(t *testing.T, out, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if rest, ok := strings.CutPrefix(line, prefix+"\t"); ok {
			id, _, _ := strings.Cut(rest, "\t")
			return id
		}
	}
	t.Fatalf("no %s line in %q", prefix, out)
	return ""
}

func TestDashboardsSaveFlagErrors(t *testing.T) {
	sqlDemoHome(t)
	html := writeHTML(t, "<p>x</p>")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no kind prefix", []string{"save", "x", "--html", html, "--datasource", "a=select 1"}, "name=sql:QUERY or name=jql:QUERY"},
		{"no equals", []string{"save", "x", "--html", html, "--datasource", "select 1"}, "name=sql:QUERY or name=jql:QUERY"},
		{"empty query", []string{"save", "x", "--html", html, "--datasource", "a=sql: "}, "got neither"},
		{"invalid datasource name", []string{"save", "x", "--html", html, "--datasource", "Bad Name=sql:select 1"}, "must match"},
		{"no html flag", []string{"save", "x"}, "usage: gadak dashboards save"},
		{"missing html file", []string{"save", "x", "--html", "/nonexistent/x.html"}, "no such file"},
		{"empty html document", []string{"save", "x", "--html", writeHTML(t, "   ")}, "html is required"},
	}
	for _, tc := range cases {
		_, stderr, err := captureErr(t, func() error { return cmdDashboards(tc.args) })
		if err == nil {
			t.Errorf("%s: accepted %v", tc.name, tc.args)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) && !strings.Contains(stderr, tc.want) {
			t.Errorf("%s: error %q / stderr %q missing %q", tc.name, err.Error(), stderr, tc.want)
		}
	}

	// None of the rejected saves wrote a row.
	out, err := capture(t, func() error { return cmdDashboards([]string{"list"}) })
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("rejected saves left rows: %q", out)
	}
}

func TestDashboardsOpenWritesFocusHash(t *testing.T) {
	sqlDemoHome(t)
	html := writeHTML(t, "<p>ok</p>")
	out, err := capture(t, func() error { return cmdDashboards([]string{"save", "wall", "--html", html}) })
	if err != nil {
		t.Fatalf("save: %v\n%s", err, out)
	}
	id := field(t, out, "saved")

	// --no-open + no serve + no desktop: only the one-shot file is written.
	prevServes := discoverServes
	discoverServes = func() []serveHit { return nil }
	t.Cleanup(func() { discoverServes = prevServes })
	out, _, err = captureBoth(t, func() error { return cmdDashboards([]string{"open", "wall", "--no-open"}) })
	if err != nil {
		t.Fatalf("open --no-open: %v\n%s", err, out)
	}
	if !strings.Contains(out, "hash\tdash="+id) {
		t.Fatalf("open output = %q, want hash dash=%s", out, id)
	}
	// The one-shot file carries the same hash and was written to this
	// profile's config dir (the temp GADAK_HOME), not the real one.
	hash, ok, err := uifocus.TakeFor(config.Profile())
	if err != nil || !ok {
		t.Fatalf("focus file: hash=%q ok=%v err=%v", hash, ok, err)
	}
	if hash != "dash="+id {
		t.Fatalf("focus hash = %q, want dash=%s", hash, id)
	}
}

func TestDashboardsHelp(t *testing.T) {
	out, err := capture(t, func() error { return cmdDashboards([]string{"--help"}) })
	if err != nil {
		t.Fatalf("help: %v\n%s", err, out)
	}
	if !strings.Contains(out, "dashboards save <name>") {
		t.Fatalf("help output missing usage: %q", out)
	}
}

// TestDashboardsLibCLI (GDK-808): the lib cache's front door. `lib add`
// prints the evidence (url, sha384, size, path) so the run that fetched the
// bytes and the config that pins them can be compared by eye; a save naming
// an uncached id is refused with the recipe; list/rm manage the manifest.
// The "CDN" is a loopback httptest server — http to 127.0.0.1 is exactly the
// self-hosting case the url rule allows, and no test leaves the machine.
func TestDashboardsLibCLI(t *testing.T) {
	sqlDemoHome(t)
	body := "window.demoLib = {version: 1};\n"
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(cdn.Close)

	out, err := capture(t, func() error {
		return cmdDashboards([]string{"lib", "add", cdn.URL + "/demo-lib.iife.js"})
	})
	if err != nil {
		t.Fatalf("lib add: %v\n%s", err, out)
	}
	id := field(t, out, "added")
	sum := sha512.Sum384([]byte(body))
	if got := field(t, out, "sha384"); got != hex.EncodeToString(sum[:]) {
		t.Errorf("sha384 = %s, want the hash of the fetched bytes", got)
	}
	if got := field(t, out, "url"); got != cdn.URL+"/demo-lib.iife.js" {
		t.Errorf("url = %s", got)
	}
	if got := field(t, out, "size"); got != fmt.Sprint(len(body)) {
		t.Errorf("size = %s", got)
	}
	// The printed path is a real file inside this profile's cache dir.
	p := field(t, out, "path")
	if filepath.Base(p) != id || !strings.Contains(p, "dashboards"+string(filepath.Separator)+"libs"+string(filepath.Separator)) {
		t.Errorf("path %q must be <profile>/dashboards/libs/%s", p, id)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("printed path does not exist: %v", err)
	}

	// list carries the row; save --lib pins the id into the config.
	if out, err = capture(t, func() error { return cmdDashboards([]string{"lib", "list"}) }); err != nil {
		t.Fatalf("lib list: %v\n%s", err, out)
	}
	if !strings.HasPrefix(out, id+"\t") {
		t.Fatalf("lib list row = %q", out)
	}
	html := writeHTML(t, "<p>wall</p>")
	if out, err = capture(t, func() error {
		return cmdDashboards([]string{"save", "WithLib", "--html", html, "--lib", id})
	}); err != nil {
		t.Fatalf("save --lib: %v\n%s", err, out)
	}
	if out, err = capture(t, func() error { return cmdDashboards([]string{"show", "withlib"}) }); err != nil || !strings.Contains(out, "lib\t"+id) {
		t.Fatalf("show after save --lib = %q (%v)", out, err)
	}

	// rm: the manifest row and the bytes go; a save naming the removed id is
	// refused with the recipe, and nothing is written.
	if out, err = capture(t, func() error { return cmdDashboards([]string{"lib", "rm", id}) }); err != nil || !strings.HasPrefix(out, "deleted\t"+id) {
		t.Fatalf("lib rm = %q (%v)", out, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("cache file survived lib rm")
	}
	_, _, err = captureErr(t, func() error {
		return cmdDashboards([]string{"save", "AfterRm", "--html", html, "--lib", id})
	})
	if err == nil || !strings.Contains(err.Error(), "gadak dashboards lib add") {
		t.Fatalf("save with removed lib = %v, want the lib add recipe", err)
	}
	if out, err := capture(t, func() error { return cmdDashboards([]string{"show", "afterrm"}) }); err == nil {
		t.Fatalf("refused save still wrote a row: %q", out)
	}

	// The url rule, at the front door: an external http url is refused
	// before any dial, and the cache is unchanged by the refusal.
	_, stderr, err := captureErr(t, func() error {
		return cmdDashboards([]string{"lib", "add", "http://cdn.example.com/evil.js"})
	})
	if err == nil || !strings.Contains(err.Error(), "https") && !strings.Contains(stderr, "https") {
		t.Fatalf("external http accepted: %v / %s", err, stderr)
	}
	if out, err = capture(t, func() error { return cmdDashboards([]string{"lib", "list", "--json"}) }); err != nil {
		t.Fatalf("lib list --json: %v\n%s", err, out)
	}
	var m struct {
		Libs []dashboards.Lib `json:"libs"`
	}
	if err := json.Unmarshal([]byte(out), &m); err != nil || len(m.Libs) != 0 {
		t.Fatalf("cache after refused add = %s (%v)", out, err)
	}
}
