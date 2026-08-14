package main

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
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

func TestViewsOpenNoOpenSkipsLaunch(t *testing.T) {
	mirror(t, "https://unused.example.com")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA AND statusCategory = "In Progress"`, "--no-open", "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	var body struct {
		Hash    string `json:"hash"`
		File    bool   `json:"file"`
		Web     string `json:"web"`
		Desktop bool   `json:"desktop"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	if !body.File || body.Web != "" || body.Desktop || !strings.Contains(body.Hash, "pj=NMA") {
		t.Fatalf("no-open %+v", body)
	}
}

func TestViewsOpenEnvNoOpen(t *testing.T) {
	mirror(t, "https://unused.example.com")
	t.Setenv("GADAK_NO_OPEN", "1")
	out, err := capture(t, func() error {
		return cmdViews([]string{"open", "--jql", `project = NMA`, "--json"})
	})
	if err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"desktop":false`) || !strings.Contains(out, `"web":""`) {
		t.Fatalf("GADAK_NO_OPEN still launched: %s", out)
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
	if got := workspacePrefix("work", "work"); got != "" {
		t.Fatalf("same profile prefix %q", got)
	}
	if got := workspacePrefix("work", ""); got != "/w/work" {
		t.Fatalf("other primary prefix %q", got)
	}
	if got := workspacePrefix("", "work"); got != "/w/default" {
		t.Fatalf("default-on-named prefix %q", got)
	}
	if got := workspacePrefix("default", ""); got != "" {
		t.Fatalf("default==empty prefix %q", got)
	}
	got := composeServeURL("http://127.0.0.1:7777", "/w/work", "ks=NMA-1,NMA-2")
	want := "http://127.0.0.1:7777/w/work/#/?ks=NMA-1,NMA-2"
	if got != want {
		t.Fatalf("url %q want %q", got, want)
	}
	if jql.HashURL(func() jql.Filter {
		f := jql.EmptyFilter()
		f.Keys = []string{"NMA-1", "NMA-2"}
		return f
	}(), jql.Display{}) != "#/?ks=NMA-1,NMA-2" {
		t.Fatal("HashURL should match QueryURL(Hash)")
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
	savedExists, savedRunning, savedList, savedOpen := desktopAppExists, desktopAppRunning, listServes, startOpen
	t.Cleanup(func() {
		config.SetProfile("")
		desktopAppExists, desktopAppRunning, listServes, startOpen = savedExists, savedRunning, savedList, savedOpen
	})
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
	ok, err := focusDesktopApp()
	if runtime.GOOS != "darwin" {
		if ok || err != nil {
			t.Fatalf("non-darwin: focused=%v err=%v", ok, err)
		}
		return
	}
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
	savedExists, savedRunning, savedList, savedOpen := desktopAppExists, desktopAppRunning, listServes, startOpen
	t.Cleanup(func() {
		config.SetProfile("")
		desktopAppExists, desktopAppRunning, listServes, startOpen = savedExists, savedRunning, savedList, savedOpen
	})
	config.SetProfile("work")
	desktopAppExists = func() bool { return true }
	desktopAppRunning = func() bool { return true }
	listServes = func() []gadakProbe { return nil }
	var got []string
	startOpen = func(args ...string) error {
		got = append([]string(nil), args...)
		return nil
	}
	ok, err := focusDesktopApp()
	if runtime.GOOS != "darwin" {
		if ok || err != nil {
			t.Fatalf("non-darwin: focused=%v err=%v", ok, err)
		}
		return
	}
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
	savedExists, savedRunning, savedList, savedOpen := desktopAppExists, desktopAppRunning, listServes, startOpen
	t.Cleanup(func() {
		config.SetProfile("")
		desktopAppExists, desktopAppRunning, listServes, startOpen = savedExists, savedRunning, savedList, savedOpen
	})
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
	ok, err := focusDesktopApp()
	if runtime.GOOS != "darwin" {
		if ok || err != nil {
			t.Fatalf("non-darwin: focused=%v err=%v", ok, err)
		}
		return
	}
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
