package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/clitool"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/serveaddr"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/views"
	"github.com/midagedev/gadak/internal/workspace"
)

func TestViewsListAndShow(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSourceQueries(context.Background(), "jira", []store.SourceQuery{
		{ID: "jira:10008", SourceID: "jira", ExternalID: "10008", Name: "gadak-test: NMA in progress",
			QueryText: `project = NMA AND statusCategory = "In Progress"`,
			Config:    json.RawMessage(`{"filters":{"jira_project":["NMA"],"status_category":["inprogress"]},"display":{"group_by":"status_category"}}`),
			Favourite: true, Applied: []string{"project", "statusCategory"}},
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	out, err := capture(t, func() error { return cmdViews([]string{"--json"}) })
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "gadak-test: NMA in progress") || !strings.Contains(out, "jira:10008") {
		t.Fatalf("list %s", out)
	}

	out, err = capture(t, func() error { return cmdViews([]string{"show", "NMA in progress"}) })
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out, "pj=NMA") || !strings.Contains(out, "sc=inprogress") {
		t.Fatalf("show hash %s", out)
	}
}

// A view whose JQL only partly compiled must say so every time it is listed
// or opened, not once at save (GDK-504): the listing prints the requested JQL,
// so without a marker the view reads as a promise it does not keep.
func TestViewsPartialViewSaysSoOnListAndOpen(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PutSavedView(context.Background(), store.SavedView{
		ID:   "cli-partial",
		Name: "partial queue",
		Config: json.RawMessage(`{"filters":{},"display":{"sort":"priority"},` +
			`"jql":"statusCategory != Done ORDER BY priority",` +
			`"applied":["ORDER BY"],"unsupported":["statusCategory != Done (only = and IN)"]}`),
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	out, err := capture(t, func() error { return cmdViews(nil) })
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[partial]") {
		t.Fatalf("listing must mark a partly-compiled view:\n%s", out)
	}

	stdout, stderr, err := captureBoth(t, func() error {
		return cmdViews([]string{"open", "partial queue", "--no-open"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "statusCategory != Done") {
		t.Fatalf("open must repeat the skipped clause on stderr:\n%s", stderr)
	}

	jout, _, err := captureBoth(t, func() error {
		return cmdViews([]string{"open", "partial queue", "--no-open", "--json"})
	})
	if err != nil {
		t.Fatalf("open --json: %v\n%s", err, jout)
	}
	var body struct {
		Unsupported []string `json:"unsupported"`
	}
	if err := json.Unmarshal([]byte(jout), &body); err != nil {
		t.Fatalf("decode %s: %v", jout, err)
	}
	if len(body.Unsupported) == 0 {
		t.Fatalf("open --json must carry unsupported:\n%s", jout)
	}
}

func TestViewsOpenWritesFocus(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA AND statusCategory = "In Progress"`, "--no-open", "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	var body struct {
		Hash string `json:"hash"`
		File bool   `json:"file"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if !body.File || !strings.Contains(body.Hash, "pj=NMA") || !strings.Contains(body.Hash, "sc=inprogress") {
		t.Fatalf("open %+v", body)
	}
}

// stubViewsLaunchSeams replaces serve discovery + both launch paths.
// hits == nil means "no serve". Callers must not leave the vars swapped.
func stubViewsLaunchSeams(t *testing.T, hits []serveHit) (openedWeb *bool, openedDesk *bool) {
	t.Helper()
	savedDiscover, savedOpen, savedStart := discoverServes, openFocusURL, startOpen
	savedWait := startOpenWait
	web, desk := false, false
	t.Cleanup(func() {
		discoverServes, openFocusURL, startOpen = savedDiscover, savedOpen, savedStart
		startOpenWait = savedWait
	})
	discoverServes = func() []serveHit { return hits }
	openFocusURL = func(u string) error {
		web = true
		t.Errorf("openFocusURL must not run under --no-open (url %s)", u)
		return nil
	}
	startOpen = func(args ...string) error {
		desk = true
		t.Errorf("startOpen must not run under --no-open (args %v)", args)
		return nil
	}
	// The deep-link path is the one that runs `open` with a URL. Stubbed for
	// the same reason as the others, and because the real one would hand a
	// gadak:// URL to LaunchServices from a unit test.
	startOpenWait = func(args ...string) error {
		desk = true
		t.Errorf("startOpenWait must not run under --no-open (args %v)", args)
		return nil
	}
	return &web, &desk
}

// stubFocusSeams sets up a "raise is allowed" world and captures both launch
// paths, so a test can assert which one ran.
func stubFocusSeams(t *testing.T) (deepLinked *[]string, raised *[]string, deepLinkErr *error) {
	t.Helper()
	savedGOOS := desktopFocusGOOS
	savedExists, savedRunning, savedList := desktopAppExists, desktopAppRunning, listServes
	savedOpen, savedWait := startOpen, startOpenWait
	t.Cleanup(func() {
		desktopFocusGOOS = savedGOOS
		desktopAppExists, desktopAppRunning, listServes = savedExists, savedRunning, savedList
		startOpen, startOpenWait = savedOpen, savedWait
	})
	// These tests pin the macOS `open` path; the Windows path has its own cases.
	desktopFocusGOOS = "darwin"
	desktopAppExists = func() bool { return true }
	desktopAppRunning = func() bool { return true }
	listServes = func() []gadakProbe { return nil }
	var viaLink, viaRaise []string
	var linkErr error
	startOpenWait = func(args ...string) error {
		viaLink = append([]string(nil), args...)
		return linkErr
	}
	startOpen = func(args ...string) error {
		viaRaise = append([]string(nil), args...)
		return nil
	}
	return &viaLink, &viaRaise, &linkErr
}

// The deep link is preferred because it does in one call what `open -a` plus
// the uifocus file did in two, and without the file's two-minute window or
// its per-mount consumption.
func TestFocusDesktopAppPrefersTheDeepLink(t *testing.T) {
	viaLink, viaRaise, _ := stubFocusSeams(t)
	const link = "gadak://view/w/work?ks=NMA-1"
	ok, err := focusDesktopApp(link)
	if err != nil || !ok {
		t.Fatalf("focusDesktopApp(%q) = %v, %v; want true, nil", link, ok, err)
	}
	if len(*viaLink) != 1 || (*viaLink)[0] != link {
		t.Fatalf("open args %v, want exactly [%q]", *viaLink, link)
	}
	if len(*viaRaise) != 0 {
		t.Fatalf("open -a also ran (%v); the deep link already raised the app", *viaRaise)
	}
}

// The compatibility case, and the reason the deep-link branch waits on `open`
// at all: a Gadak.app older than the scheme registers no handler, `open`
// answers kLSApplicationNotFoundErr, and without this fallback the user
// watches nothing happen.
func TestFocusDesktopAppFallsBackWhenTheSchemeIsUnregistered(t *testing.T) {
	viaLink, viaRaise, linkErr := stubFocusSeams(t)
	*linkErr = errors.New("No application knows how to open URL")
	ok, err := focusDesktopApp("gadak://view/w/work?ks=NMA-1")
	if err != nil || !ok {
		t.Fatalf("fallback must still focus: ok=%v err=%v", ok, err)
	}
	if len(*viaLink) == 0 {
		t.Fatal("the deep link was never attempted")
	}
	if len(*viaRaise) != 2 || (*viaRaise)[0] != "-a" || (*viaRaise)[1] != "Gadak" {
		t.Fatalf("fallback args %v, want [-a Gadak]", *viaRaise)
	}
}

// No link to offer — `views open` composes "" only when there is no hash —
// so the old path is the whole path.
func TestFocusDesktopAppWithoutALinkRaisesOnly(t *testing.T) {
	viaLink, viaRaise, _ := stubFocusSeams(t)
	if ok, err := focusDesktopApp(""); err != nil || !ok {
		t.Fatalf("focusDesktopApp(\"\") = %v, %v; want true, nil", ok, err)
	}
	if len(*viaLink) != 0 {
		t.Fatalf("open ran with %v for an empty link", *viaLink)
	}
	if len(*viaRaise) != 2 || (*viaRaise)[0] != "-a" {
		t.Fatalf("raise args %v, want [-a Gadak]", *viaRaise)
	}
}

func TestViewsOpenNoOpenSkipsLaunch(t *testing.T) {
	mirror(t, "https://unused.example.com")
	// Inject an empty discovery so a live gadak serve on this machine cannot
	// flip the assertion. Concern is launch, not the URL.
	web, desk := stubViewsLaunchSeams(t, nil)
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA AND statusCategory = "In Progress"`, "--no-open", "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	var body struct {
		Hash     string     `json:"hash"`
		File     bool       `json:"file"`
		Web      string     `json:"web"`
		Desktop  bool       `json:"desktop"`
		DeepLink string     `json:"deeplink"`
		Serve    serveDebug `json:"serve"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if !body.File || body.Desktop || !strings.Contains(body.Hash, "pj=NMA") {
		t.Fatalf("no-open %+v", body)
	}
	if body.Web != "" {
		t.Fatalf("injected empty discovery must yield empty web, got %q", body.Web)
	}
	// The whole point of the deep link: it is there in exactly the case that
	// leaves `web` empty. An agent with no running serve still has something
	// to hand over, and this is the assertion that keeps that true.
	if body.DeepLink != deepLinkURL(config.Profile(), body.Hash) {
		t.Fatalf("deeplink %q does not match the composed link for hash %q",
			body.DeepLink, body.Hash)
	}
	if !strings.HasPrefix(body.DeepLink, "gadak://view") {
		t.Fatalf("deeplink %q is not a gadak:// link", body.DeepLink)
	}
	if *web || *desk {
		t.Fatalf("launch happened: web=%v desk=%v", *web, *desk)
	}
}

func TestViewsOpenEnvNoOpen(t *testing.T) {
	mirror(t, "https://unused.example.com")
	web, desk := stubViewsLaunchSeams(t, nil)
	t.Setenv("GADAK_NO_OPEN", "1")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA`, "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"desktop":false`) {
		t.Fatalf("GADAK_NO_OPEN still launched desktop: %s", out)
	}
	if *web || *desk {
		t.Fatalf("GADAK_NO_OPEN launched: web=%v desk=%v out=%s", *web, *desk, out)
	}
}

// G4: a discovered serve prints the full URL even under --no-open, and still
// does not launch. This is the contract the old web=="" assertion contradicted.
func TestViewsOpenNoOpenPrintsURLWhenServeFound(t *testing.T) {
	mirror(t, "https://unused.example.com")
	web, desk := stubViewsLaunchSeams(t, []serveHit{{
		base: "http://127.0.0.1:7777", profile: "", port: "7777",
	}})
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA`, "--no-open", "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	var body struct {
		Web     string     `json:"web"`
		Desktop bool       `json:"desktop"`
		Serve   serveDebug `json:"serve"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if !strings.Contains(body.Web, "http://127.0.0.1:7777") || !strings.Contains(body.Web, "pj=NMA") {
		t.Fatalf("G4 want serve URL, got %q", body.Web)
	}
	if body.Desktop {
		t.Fatalf("desktop focused under --no-open: %+v", body)
	}
	if body.Serve.Base != "http://127.0.0.1:7777" || body.Serve.Port != "7777" {
		t.Fatalf("serve debug %+v", body.Serve)
	}
	if len(body.Serve.Ports) == 0 {
		t.Fatal("serve.ports must list the probed set")
	}
	if *web || *desk {
		t.Fatalf("G4 launched despite --no-open: web=%v desk=%v", *web, *desk)
	}
}

func TestViewsOpenKeysNoMirror(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "--keys", "NMA-1, NMA-2"})
	})
	if err != nil {
		t.Fatalf("open --keys: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ks=NMA-1,NMA-2") {
		t.Fatalf("hash missing ks=: %s", out)
	}
	var body struct {
		Hash string   `json:"hash"`
		Keys []string `json:"keys"`
		Name string   `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if body.Name != "keys" || len(body.Keys) != 2 || body.Keys[0] != "NMA-1" || body.Keys[1] != "NMA-2" {
		t.Fatalf("keys json %+v", body)
	}
}

func TestViewsOpenKeysStdin(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("NMA-1\nNMA-2\n")
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "--keys", "-"})
	})
	os.Stdin = saved
	if err != nil {
		t.Fatalf("open --keys -: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ks=NMA-1,NMA-2") {
		t.Fatalf("stdin keys %s", out)
	}
}

func TestViewsOpenKeysStdinMixedFirstSeen(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Commas, spaces, tabs, and newlines mixed; NMA-3 repeats. First-seen order.
	go func() {
		_, _ = w.WriteString("NMA-3, nma-1\nNMA-2 NMA-3\tNMA-4\n")
		_ = w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "--keys", "-"})
	})
	os.Stdin = saved
	if err != nil {
		t.Fatalf("open --keys - mixed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ks=NMA-3,NMA-1,NMA-2,NMA-4") {
		t.Fatalf("first-seen order lost: %s", out)
	}
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	want := []string{"NMA-3", "NMA-1", "NMA-2", "NMA-4"}
	if len(body.Keys) != len(want) {
		t.Fatalf("keys %v want %v", body.Keys, want)
	}
	for i := range want {
		if body.Keys[i] != want[i] {
			t.Fatalf("keys %v want %v", body.Keys, want)
		}
	}
}

func TestViewsOpenKeysExclusive(t *testing.T) {
	err := cmdViews([]string{"open", "--keys", "NMA-1", "--jql", "project = NMA"})
	if err == nil || !strings.Contains(err.Error(), "--keys cannot be combined") {
		t.Fatalf("keys+jql: %v", err)
	}
	err = cmdViews([]string{"open", "--keys", "NMA-1", "Night triage"})
	if err == nil || !strings.Contains(err.Error(), "--keys cannot be combined") {
		t.Fatalf("keys+name: %v", err)
	}
}

func TestViewsOpenKeysCap(t *testing.T) {
	vals := make([]string, jql.MaxKeys+1)
	for i := range vals {
		vals[i] = "NMA-" + strconv.Itoa(i+1)
	}
	err := cmdViews([]string{"open", "--no-open", "--keys", strings.Join(vals, ",")})
	if err == nil || !strings.Contains(err.Error(), "501") || !strings.Contains(err.Error(), "500") {
		t.Fatalf("cap: %v", err)
	}
}

func TestViewsOpenPositionalIssueKey(t *testing.T) {
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "nmb-140"})
	})
	if err != nil {
		t.Fatalf("open KEY: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"hash":"issue=NMB-140"`) {
		t.Fatalf("want issue=NMB-140, got %s", out)
	}
}

func TestViewsOpenStoredViewWinsOverKeyShape(t *testing.T) {
	mirror(t, "https://unused.example.com")
	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	cfg := json.RawMessage(`{"filters":{"jira_project":["NMA"]},"display":{}}`)
	if err := db.PutSavedView(context.Background(), store.SavedView{ID: "cli-nmb-140", Name: "NMB-140", Config: cfg}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "NMB-140"})
	})
	if err != nil {
		t.Fatalf("open named view: %v\n%s", err, out)
	}
	if strings.Contains(out, "issue=NMB-140") {
		t.Fatalf("stored view should win: %s", out)
	}
	if !strings.Contains(out, "pj=NMA") {
		t.Fatalf("want view hash, got %s", out)
	}
}

func TestComposeServeURLAndPrefix(t *testing.T) {
	// The prefix rule's own cases live with the rule, in
	// internal/workspace (TestPrefix) — the desktop app's deep-link handler
	// needs the same answer, and a copy here would be the second definition.
	// What this test owns is the seam: the URL `views open` hands the user is
	// the shared prefix joined to the hash, not a locally reinvented path.
	if got := composeServeURL("http://127.0.0.1:7777",
		workspace.Prefix("work", ""), "ks=NMA-1,NMA-2"); got != "http://127.0.0.1:7777/w/work/#/?ks=NMA-1,NMA-2" {
		t.Fatalf("views open URL through the shared prefix: %q", got)
	}
	got := composeServeURL("http://127.0.0.1:7777", "/w/work", "ks=NMA-1,NMA-2")
	want := "http://127.0.0.1:7777/w/work/#/?ks=NMA-1,NMA-2"
	if got != want {
		t.Fatalf("url %q want %q", got, want)
	}
	if jql.QueryURL(jql.Hash(func() jql.Filter {
		f := jql.EmptyFilter()
		f.Keys = []string{"NMA-1", "NMA-2"}
		return f
	}(), jql.Display{})) != "#/?ks=NMA-1,NMA-2" {
		t.Fatal("QueryURL(Hash) should match the fragment views open hands over")
	}
}

func TestDesktopAppRunningPgrepName(t *testing.T) {
	var got string
	saved := lookDesktopProcess
	t.Cleanup(func() { lookDesktopProcess = saved })
	lookDesktopProcess = func(name string) bool {
		got = name
		return true
	}
	if !desktopAppRunning() {
		t.Fatal("desktopAppRunning should report the injected match")
	}
	// Gadak.app/Contents/MacOS/gadak-desktop — pgrep -x matches this, not "Gadak".
	if desktopProcessName != "gadak-desktop" {
		t.Fatalf("desktopProcessName = %q, want gadak-desktop", desktopProcessName)
	}
	if got != desktopProcessName {
		t.Fatalf("pgrep name = %q, want %q", got, desktopProcessName)
	}
}

func TestDecideDesktopFocusWrongProfileDoesNotRaise(t *testing.T) {
	// App not running; only another profile's serve. open -a would launch
	// default — D5 forbids that. A serve exists, so this is not an error
	// (the web path already opened /w/work/); just do not raise.
	running := []gadakProbe{{IsGadak: true, Profile: "default"}}
	act, msg := decideDesktopFocus(true, false, "work", running)
	if act != desktopFocusNone {
		t.Fatalf("must not open -a for another profile's serve, got %d (%s)", act, msg)
	}
}

func TestDecideDesktopFocusDefaultAppRunningDespiteOtherServe(t *testing.T) {
	// Side regression: default desktop up + unrelated `gadak serve --profile work`
	// must still raise the default app. Serve occupancy cannot name the desktop window.
	running := []gadakProbe{{IsGadak: true, Profile: "work"}}
	act, msg := decideDesktopFocus(true, true, "", running)
	if act != desktopFocusRaise {
		t.Fatalf("default app running must raise even with another serve, got %d (%s)", act, msg)
	}
}

func TestDecideDesktopFocusNamedNoAppNoServeErrors(t *testing.T) {
	act, msg := decideDesktopFocus(true, false, "work", nil)
	if act != desktopFocusError {
		t.Fatalf("named profile with no app and no serve must error, got %d", act)
	}
	if !strings.Contains(msg, `profile "work"`) {
		t.Fatalf("msg should name the wanted profile: %q", msg)
	}
	if !strings.Contains(msg, "gadak serve --profile work") {
		t.Fatalf("msg should tell the user how to start it: %q", msg)
	}
}

func TestDecideDesktopFocusSameProfileRaises(t *testing.T) {
	running := []gadakProbe{{IsGadak: true, Profile: "work"}}
	act, msg := decideDesktopFocus(true, true, "work", running)
	if act != desktopFocusRaise {
		t.Fatalf("same-profile window must raise, got %d (%s)", act, msg)
	}
	act, msg = decideDesktopFocus(true, true, "", []gadakProbe{{IsGadak: true, Profile: ""}})
	if act != desktopFocusRaise {
		t.Fatalf("default-on-default must raise, got %d (%s)", act, msg)
	}
	act, msg = decideDesktopFocus(true, false, "work", running)
	if act != desktopFocusNone {
		t.Fatalf("matching CLI serve must not launch Gadak.app, got %d (%s)", act, msg)
	}
}

func TestDecideDesktopFocusNamedWithoutWindowErrors(t *testing.T) {
	// Desktop app serves in-process and does not open the CLI probe ports.
	// App running + no serve + named profile must raise, not error.
	act, msg := decideDesktopFocus(true, true, "work", nil)
	if act != desktopFocusRaise {
		t.Fatalf("app running, no serve, named profile must raise, got %d (%s)", act, msg)
	}
}

func TestDecideDesktopFocusDefaultLaunchesWhenIdle(t *testing.T) {
	act, msg := decideDesktopFocus(true, false, "", nil)
	if act != desktopFocusRaise {
		t.Fatalf("default with no occupant should launch, got %d (%s)", act, msg)
	}
	act, _ = decideDesktopFocus(false, false, "work", nil)
	if act != desktopFocusNone {
		t.Fatalf("no app bundle: action = %d", act)
	}
}

func TestFocusDesktopAppWrongProfileDoesNotOpen(t *testing.T) {
	savedGOOS := desktopFocusGOOS
	savedExists, savedRunning, savedList, savedOpen := desktopAppExists, desktopAppRunning, listServes, startOpen
	savedWin := startWindowsDesktop
	t.Cleanup(func() {
		config.SetProfile("")
		desktopFocusGOOS = savedGOOS
		desktopAppExists, desktopAppRunning, listServes, startOpen = savedExists, savedRunning, savedList, savedOpen
		startWindowsDesktop = savedWin
	})
	desktopFocusGOOS = "darwin"
	config.SetProfile("work")
	desktopAppExists = func() bool { return true }
	desktopAppRunning = func() bool { return false }
	listServes = func() []gadakProbe {
		return []gadakProbe{{IsGadak: true, Profile: "default"}}
	}
	opened := false
	startOpen = func(args ...string) error {
		opened = true
		t.Errorf("must not call open: %v", args)
		return nil
	}
	startWindowsDesktop = func(link string) error {
		t.Errorf("must not launch windows desktop: %s", link)
		return nil
	}
	ok, err := focusDesktopApp("")
	if err != nil {
		t.Fatalf("other serve is not a hard error (web path handles it): %v", err)
	}
	if opened {
		t.Fatal("open -a must not run for the wrong profile")
	}
	if ok {
		t.Fatal("desktop focused flag must be false")
	}
}

func TestFocusDesktopAppNamedNoServeRaises(t *testing.T) {
	savedGOOS := desktopFocusGOOS
	savedExists, savedRunning, savedList, savedOpen := desktopAppExists, desktopAppRunning, listServes, startOpen
	t.Cleanup(func() {
		config.SetProfile("")
		desktopFocusGOOS = savedGOOS
		desktopAppExists, desktopAppRunning, listServes, startOpen = savedExists, savedRunning, savedList, savedOpen
	})
	desktopFocusGOOS = "darwin"
	config.SetProfile("work")
	desktopAppExists = func() bool { return true }
	desktopAppRunning = func() bool { return true }
	listServes = func() []gadakProbe { return nil }
	var got []string
	startOpen = func(args ...string) error {
		got = append([]string(nil), args...)
		return nil
	}
	ok, err := focusDesktopApp("")
	if err != nil {
		t.Fatalf("desktop-only named profile must raise, not error: %v", err)
	}
	if !ok {
		t.Fatal("desktop-only named profile should report focused")
	}
	if len(got) < 2 || got[0] != "-a" || got[1] != "Gadak" {
		t.Fatalf("open args = %v, want -a Gadak", got)
	}
}

func TestFocusDesktopAppSameProfileOpensWithoutEnv(t *testing.T) {
	savedGOOS := desktopFocusGOOS
	savedExists, savedRunning, savedList, savedOpen := desktopAppExists, desktopAppRunning, listServes, startOpen
	t.Cleanup(func() {
		config.SetProfile("")
		desktopFocusGOOS = savedGOOS
		desktopAppExists, desktopAppRunning, listServes, startOpen = savedExists, savedRunning, savedList, savedOpen
	})
	desktopFocusGOOS = "darwin"
	config.SetProfile("work")
	desktopAppExists = func() bool { return true }
	desktopAppRunning = func() bool { return true }
	listServes = func() []gadakProbe {
		return []gadakProbe{{IsGadak: true, Profile: "work"}}
	}
	var got []string
	startOpen = func(args ...string) error {
		got = append([]string(nil), args...)
		return nil
	}
	ok, err := focusDesktopApp("")
	if err != nil {
		t.Fatalf("same-profile raise: %v", err)
	}
	if !ok {
		t.Fatal("same-profile raise should report focused")
	}
	for _, a := range got {
		if a == "-n" || strings.HasPrefix(a, "--env") {
			t.Fatalf("must not pass -n or --env: %v", got)
		}
	}
	if len(got) < 2 || got[0] != "-a" || got[1] != "Gadak" {
		t.Fatalf("open args = %v, want -a Gadak", got)
	}
}

// Windows is no longer a silent false (GDK-244): decideDesktopFocus still
// owns the table, and a raise launches the portable exe with the gadak://
// link so the running app navigates. The GOOS seam lets this run on Linux CI.
func TestFocusDesktopAppWindowsLaunchesExeWithLink(t *testing.T) {
	savedGOOS := desktopFocusGOOS
	savedExists, savedRunning, savedList := desktopAppExists, desktopAppRunning, listServes
	savedStart := startWindowsDesktop
	savedRaise := raiseWindowsWindow
	t.Cleanup(func() {
		desktopFocusGOOS = savedGOOS
		desktopAppExists, desktopAppRunning, listServes = savedExists, savedRunning, savedList
		startWindowsDesktop, raiseWindowsWindow = savedStart, savedRaise
	})
	desktopFocusGOOS = "windows"
	desktopAppExists = func() bool { return true }
	desktopAppRunning = func() bool { return true }
	listServes = func() []gadakProbe { return nil }
	var started string
	startWindowsDesktop = func(link string) error {
		started = link
		return nil
	}
	raiseWindowsWindow = func() bool { return true }

	const link = "gadak://view?ks=NMA-1"
	ok, err := focusDesktopApp(link)
	if err != nil || !ok {
		t.Fatalf("windows focus = %v, %v; want true, nil", ok, err)
	}
	if started != link {
		t.Fatalf("started %q, want the gadak:// link", started)
	}
}

func TestFocusDesktopAppWindowsRespectsDecideNone(t *testing.T) {
	savedGOOS := desktopFocusGOOS
	savedExists, savedRunning, savedList := desktopAppExists, desktopAppRunning, listServes
	savedStart := startWindowsDesktop
	t.Cleanup(func() {
		config.SetProfile("")
		desktopFocusGOOS = savedGOOS
		desktopAppExists, desktopAppRunning, listServes = savedExists, savedRunning, savedList
		startWindowsDesktop = savedStart
	})
	desktopFocusGOOS = "windows"
	config.SetProfile("work")
	desktopAppExists = func() bool { return true }
	desktopAppRunning = func() bool { return false }
	listServes = func() []gadakProbe {
		return []gadakProbe{{IsGadak: true, Profile: "default"}}
	}
	startWindowsDesktop = func(link string) error {
		t.Errorf("must not launch when decide says none (link %s)", link)
		return nil
	}
	ok, err := focusDesktopApp("gadak://view/w/work?ks=NMA-1")
	if ok || err != nil {
		t.Fatalf("wrong-profile windows focus = %v, %v; want false, nil", ok, err)
	}
}

func TestFindWindowsDesktopExeRecordedFallback(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "gadak-desktop.exe")
	if err := os.WriteFile(exe, []byte("desk"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(root, "desktop-exe-path")
	if err := os.WriteFile(rec, []byte(exe+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := clitool.DesktopExePathFile
	clitool.DesktopExePathFile = rec
	t.Cleanup(func() { clitool.DesktopExePathFile = prev })
	t.Setenv("PATH", filepath.Join(root, "empty-path"))

	got := findWindowsDesktopExe()
	if got != exe {
		t.Fatalf("recorded fallback = %q, want %q", got, exe)
	}

	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	got = findWindowsDesktopExe()
	if got != "" {
		t.Fatalf("missing target must be empty, got %q", got)
	}
}

func TestFocusDesktopAppLinuxStillNoop(t *testing.T) {
	savedGOOS := desktopFocusGOOS
	t.Cleanup(func() { desktopFocusGOOS = savedGOOS })
	desktopFocusGOOS = "linux"
	ok, err := focusDesktopApp("gadak://view?ks=NMA-1")
	if ok || err != nil {
		t.Fatalf("linux focus = %v, %v; want false, nil", ok, err)
	}
}

func TestViewsSaveResolvesCurrentUserToAccountID(t *testing.T) {
	cfg := mirror(t, "https://unused.example.com")
	cfg.AccountID = "acc-me"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdViews([]string{"save", "Mine", "--jql", `assignee = currentUser()`, "--json"})
	})
	if err != nil {
		t.Fatalf("save: %v\n%s", err, out)
	}

	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved views %d", len(saved))
	}
	var body struct {
		Filters jql.Filter `json:"filters"`
	}
	if err := json.Unmarshal(saved[0].Config, &body); err != nil {
		t.Fatalf("config %s: %v", saved[0].Config, err)
	}
	if len(body.Filters.AssigneeEmail) != 1 || body.Filters.AssigneeEmail[0] != "acc-me" {
		t.Fatalf("saved assignee %+v (want acc-me); stdout %s", body.Filters.AssigneeEmail, out)
	}
}

func TestViewsOpenCurrentUserHashUsesAccountID(t *testing.T) {
	cfg := mirror(t, "https://unused.example.com")
	cfg.AccountID = "acc-me"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "--jql", `assignee = currentUser()`})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	if strings.Contains(out, "currentUser") {
		t.Fatalf("hash still has currentUser(): %s", out)
	}
	if !strings.Contains(out, "as=acc-me") {
		t.Fatalf("hash missing as=acc-me: %s", out)
	}
}

func TestSearchJQLKeysReturnsThoseIssues(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error {
		return cmdSearch([]string{"--jql", `key in (NMB-1)`})
	})
	if err != nil {
		t.Fatalf("search --jql key in: %v\n%s", err, out)
	}
	if !strings.Contains(out, "NMB-1") {
		t.Fatalf("expected NMB-1, got %s", out)
	}
	out, err = capture(t, func() error {
		return cmdSearch([]string{"--jql", `key in (NMA-999)`})
	})
	if err != nil {
		t.Fatalf("search miss: %v\n%s", err, out)
	}
	if strings.Contains(out, "NMB-1") {
		t.Fatalf("NMA-999 should not return NMB-1: %s", out)
	}
}

// Fixture rewrite (GDK-771, 2026-08-24): `statusCategory != Done` was the
// canonical unsupported clause here — negation now compiles on every axis,
// so the all-unsupported fixture moved to sprint equality, which stays
// outside the subset.
func TestViewsSaveAllUnsupportedFails(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error {
		return cmdViews([]string{"save", "Night triage", "--jql", `sprint = "Sprint 41"`})
	})
	if err == nil {
		t.Fatalf("all-unsupported JQL saved as success; stdout %q", out)
	}
	msg := err.Error()
	if !strings.Contains(msg, "nothing in this JQL can be applied") || !strings.Contains(msg, "unsupported:") {
		t.Fatalf("want applied-nothing error naming unsupported clauses, got %q", msg)
	}

	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 0 {
		t.Fatalf("saved %d views; all-unsupported must write nothing", len(saved))
	}
}

func TestViewsSavePartialPrintsAppliedAndFillsHash(t *testing.T) {
	mirror(t, "https://unused.example.com")
	// GDK-771: the unsupported half is sprint equality now — the old
	// `statusCategory != Done` compiles (scn=done) and no longer keeps this
	// fixture partial.
	out, err := capture(t, func() error {
		return cmdViews([]string{"save", "Not STD", "--jql", `project NOT IN (STD) AND sprint = "Sprint 41"`})
	})
	if err != nil {
		t.Fatalf("partial save: %v\n%s", err, out)
	}
	if !strings.Contains(out, "pjn=STD") {
		t.Fatalf("stdout missing filled hash, got %q", out)
	}
	if !strings.Contains(out, "applied\t") || !strings.Contains(out, "unsupported\t") {
		t.Fatalf("stdout missing applied/unsupported, got %q", out)
	}

	db, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved, err := db.SavedViews(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved views %d", len(saved))
	}
	if h := views.HashFromConfig(saved[0].Config); !strings.Contains(h, "pjn=STD") {
		t.Fatalf("stored hash %q, want pjn=STD", h)
	}
}

func TestViewsShowAlwaysPrintsJQLAppliedUnsupported(t *testing.T) {
	mirror(t, "https://unused.example.com")
	if _, err := capture(t, func() error {
		return cmdViews([]string{"save", "Open work", "--jql", `resolution is EMPTY`})
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := capture(t, func() error { return cmdViews([]string{"show", "Open work"}) })
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	for _, col := range []string{"jql\t", "applied\t", "unsupported\t"} {
		if !strings.Contains(out, col) {
			t.Fatalf("show missing %q:\n%s", col, out)
		}
	}
	if !strings.Contains(out, "resolution is EMPTY") {
		t.Fatalf("show missing saved JQL:\n%s", out)
	}
}

func TestViewsSaveCurrentUserWithoutIdentityFails(t *testing.T) {
	cfg := mirror(t, "https://unused.example.com")
	cfg.Email = ""
	cfg.AccountID = ""
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	out, err := capture(t, func() error {
		return cmdViews([]string{"save", "Mine", "--jql", `assignee = currentUser()`})
	})
	if err == nil {
		t.Fatalf("currentUser() with no identity saved; stdout %q", out)
	}
	if !strings.Contains(err.Error(), "nothing in this JQL can be applied") {
		t.Fatalf("want applied-nothing error, got %v", err)
	}
}

// startGadakProbe is a loopback listener that answers origin.ProbePath the
// way a live gadak serve does (X-Gadak + X-Gadak-Profile). The port is
// outside 7777–7797 / 7878 so a sweep-only discoverServes cannot find it.
func startGadakProbe(t *testing.T, profile string) (addr, port string) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != origin.ProbePath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Gadak", "1")
		w.Header().Set("X-Gadak-Profile", profile)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	addr = ts.Listener.Addr().String()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Skipf("httptest host %q not 127.0.0.1", host)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	if (n >= 7777 && n <= 7797) || port == "7878" {
		t.Skipf("httptest port %s landed inside the sweep range", port)
	}
	return addr, port
}

func isolateGadakHome(t *testing.T) {
	t.Helper()
	t.Setenv("GADAK_HOME", t.TempDir())
	config.SetProfile("")
	t.Cleanup(func() { config.SetProfile("") })
}

// GDK-859: views/dashboards open must find a serve advertised in the run
// directory even when its port is outside 7777–7797. Does not inject
// discoverServes — that would not exercise the production path.
func TestFindServeTargetUsesRunDirOutsideSweep(t *testing.T) {
	isolateGadakHome(t)
	addr, port := startGadakProbe(t, "")
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(dir, addr, "from-file"); err != nil {
		t.Fatal(err)
	}

	target, dbg := findServeTarget()
	if target.base == "" {
		t.Fatalf("no serve found; run-dir file advertised %s (port %s)", addr, port)
	}
	if dbg.Port != port {
		t.Fatalf("serve.port = %q, want %q (base=%q)", dbg.Port, port, dbg.Base)
	}
	if !strings.Contains(dbg.Base, port) {
		t.Fatalf("serve.base %q does not name port %s", dbg.Base, port)
	}
	if dbg.Source != serveSourceRun {
		t.Fatalf("serve.source = %q, want %q", dbg.Source, serveSourceRun)
	}
	if got, want := strings.Join(dbg.Ports, ","), strings.Join(serveProbePorts(), ","); got != want {
		t.Fatalf("serve.ports changed meaning: %q vs sweep %q", got, want)
	}
}

func TestViewsOpenJSONFindsServeOutsideSweep(t *testing.T) {
	isolateGadakHome(t)
	addr, port := startGadakProbe(t, "")
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(dir, addr, ""); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--no-open", "--json", "--keys", "NMA-1"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	var body struct {
		Web   string `json:"web"`
		Serve struct {
			Port   string   `json:"port"`
			Base   string   `json:"base"`
			Ports  []string `json:"ports"`
			Source string   `json:"source"`
		} `json:"serve"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if body.Web == "" {
		t.Fatalf("web empty; serve on port %s was advertised in run dir\n%s", port, out)
	}
	if body.Serve.Port != port {
		t.Fatalf("serve.port = %q, want %q\n%s", body.Serve.Port, port, out)
	}
	if !strings.Contains(body.Web, port) {
		t.Fatalf("web %q does not name port %s", body.Web, port)
	}
	if body.Serve.Source != serveSourceRun {
		t.Fatalf("serve.source = %q, want %q\n%s", body.Serve.Source, serveSourceRun, out)
	}
}

func TestDiscoverServesRemovesStaleRunFile(t *testing.T) {
	isolateGadakHome(t)
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(dir, "127.0.0.1:1", ""); err != nil {
		t.Fatal(err)
	}
	for _, h := range discoverServes() {
		if h.port == "1" {
			t.Fatalf("stale port 1 in hits: %+v", h)
		}
	}
	if _, err := os.Stat(serveaddr.Path(dir, "1")); !os.IsNotExist(err) {
		t.Fatalf("stale file not removed: %v", err)
	}
}

func TestDiscoverServesUsesProbeProfileNotFile(t *testing.T) {
	isolateGadakHome(t)
	addr, port := startGadakProbe(t, "from-probe")
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(dir, addr, "from-file"); err != nil {
		t.Fatal(err)
	}
	var found serveHit
	for _, h := range discoverServes() {
		if h.port == port {
			found = h
		}
	}
	if found.port == "" {
		t.Fatal("missing run-dir hit")
	}
	if found.profile != "from-probe" {
		t.Fatalf("profile = %q, want probe value from-probe (not file)", found.profile)
	}
	if found.source != serveSourceRun {
		t.Fatalf("source = %q, want run", found.source)
	}
}

func listenGadakOnSweepPort(t *testing.T, port, profile string) {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Skipf("port %s busy: %v", port, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(origin.ProbePath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Gadak", "1")
		w.Header().Set("X-Gadak-Profile", profile)
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

func TestDiscoverServesSweepFindsUnadvertised(t *testing.T) {
	isolateGadakHome(t)
	listenGadakOnSweepPort(t, "7878", "")
	var found serveHit
	for _, h := range discoverServes() {
		if h.port == "7878" {
			found = h
		}
	}
	if found.port == "" {
		t.Fatal("sweep missed unadvertised serve on 7878")
	}
	if found.source != serveSourceSweep {
		t.Fatalf("source = %q, want sweep", found.source)
	}
}

func TestDiscoverServesDedupsRunDirAndSweep(t *testing.T) {
	isolateGadakHome(t)
	listenGadakOnSweepPort(t, "7878", "")
	dir, err := serveaddr.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := serveaddr.Write(dir, "127.0.0.1:7878", ""); err != nil {
		t.Fatal(err)
	}
	n := 0
	var src string
	for _, h := range discoverServes() {
		if h.port == "7878" {
			n++
			src = h.source
		}
	}
	if n != 1 {
		t.Fatalf("port 7878 appeared %d times, want 1", n)
	}
	if src != serveSourceRun {
		t.Fatalf("deduped hit source = %q, want run (run dir is first)", src)
	}
}
