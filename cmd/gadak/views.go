package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/deeplink"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/uifocus"
	"github.com/midagedev/gadak/internal/workspace"
)

// issueKeyPat is the G3 positional: a Jira key, compared after ToUpper.
var issueKeyPat = regexp.MustCompile(`^[A-Z][A-Z0-9]*-\d+$`)

type listedView struct {
	Kind        string          `json:"kind"` // jira | saved
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	JQL         string          `json:"jql,omitempty"`
	Hash        string          `json:"hash"`
	Favourite   bool            `json:"favourite,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Applied     []string        `json:"applied,omitempty"`
	Unsupported []string        `json:"unsupported,omitempty"`
	Config      json.RawMessage `json:"-"`
}

func cmdViews(args []string) error {
	if wantsHelp(args) {
		printHelp("views")
		return nil
	}
	sub, rest := "list", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list", "show", "open", "save":
			sub, rest = args[0], args[1:]
		default:
			sub, rest = "show", args
		}
	}
	switch sub {
	case "list":
		return viewsList(rest)
	case "show":
		return viewsShow(rest)
	case "open":
		return viewsOpen(rest)
	case "save":
		return viewsSave(rest)
	default:
		return usageError("views", `usage: gadak views [list|show|open|save]`)
	}
}

func viewsList(args []string) error {
	fs := newFlagSet("views")
	asJSON := fs.Bool("json", false, "emit views as JSON")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	list, err := loadViews(db)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"views": jsonList(list)})
	}
	for _, v := range list {
		fav := ""
		if v.Favourite {
			fav = "*"
		}
		fmt.Printf("%s\t%s\t%s%s\t%s\n", v.Kind, v.ID, fav, v.Name, compactJQL(v.JQL))
	}
	return nil
}

func viewsShow(args []string) error {
	fs := newFlagSet("views")
	asJSON := fs.Bool("json", false, "emit one view as JSON")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
	if name == "" {
		return usageError("views", `usage: gadak views show <name-or-id>`)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	v, err := findView(db, name)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(v)
	}
	fmt.Printf("kind\t%s\n", v.Kind)
	fmt.Printf("id\t%s\n", v.ID)
	fmt.Printf("name\t%s\n", v.Name)
	fmt.Printf("jql\t%s\n", v.JQL)
	if v.Hash != "" {
		fmt.Printf("hash\t%s\n", v.Hash)
	}
	fmt.Printf("applied\t%s\n", strings.Join(v.Applied, ", "))
	fmt.Printf("unsupported\t%s\n", strings.Join(v.Unsupported, "; "))
	return nil
}

func viewsOpen(args []string) error {
	fs := newFlagSet("views")
	jqlFlag := fs.String("jql", "", "open this JQL as a view (instead of a stored name)")
	keysFlag := fs.String("keys", "", "issue keys (comma or whitespace); - reads stdin")
	asJSON := fs.Bool("json", false, "emit the hash and where it was sent")
	noOpenFlag := fs.Bool("no-open", false, "write the hash only; do not open a window")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	keysRaw := strings.TrimSpace(*keysFlag)
	jqlRaw := strings.TrimSpace(*jqlFlag)
	name := strings.TrimSpace(strings.Join(pos, " "))
	if keysRaw != "" && (jqlRaw != "" || name != "") {
		return usageError("views", "--keys cannot be combined with --jql or a view name")
	}

	var hash, label string
	var keys []string
	switch {
	case keysRaw != "":
		keys, err = readKeysFlag(keysRaw)
		if err != nil {
			return err
		}
		if err := jql.CheckKeyLimit(len(keys)); err != nil {
			return err
		}
		if len(keys) == 0 {
			return usageError("views", `usage: gadak views open --keys 'KEY,KEY' | --keys -`)
		}
		f := jql.EmptyFilter()
		f.Keys = keys
		hash, label = jql.Hash(f, jql.Display{}), "keys"
	case jqlRaw != "":
		h, err := hashFromJQL(jqlRaw)
		if err != nil {
			return err
		}
		hash, label = h, "jql"
	case name == "":
		return usageError("views", `usage: gadak views open <name-or-id|KEY> | --jql '…' | --keys '…'`)
	case looksLikeIssueKey(name):
		hash, label = issueOrExactView(name)
	default:
		db, err := openStore()
		if err != nil {
			return err
		}
		defer db.Close()
		v, err := findView(db, name)
		if err != nil {
			return err
		}
		if v.Hash == "" {
			return fmt.Errorf("view %q has no gadak hash — nothing to focus", v.Name)
		}
		hash, label = v.Hash, v.Name
	}

	if err := uifocus.Write(hash); err != nil {
		return err
	}
	skipOpen := *noOpenFlag || envNoOpen()
	// G4: always resolve the URL (even under --no-open); only launch when asked.
	web, serveDbg := resolveServeFocus(hash)
	link := deepLinkURL(config.Profile(), hash)
	desk := false
	if !skipOpen {
		if web != "" {
			if err := openFocusURL(web); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not open %s (%v)\n", web, err)
			}
		}
		var ferr error
		desk, ferr = focusDesktopApp(link)
		if ferr != nil {
			// A serve URL already presented the view (including /w/<profile>/
			// on another profile's process). Do not fail the command; the
			// desktop raise would have stolen the wrong window.
			if web != "" {
				fmt.Fprintf(os.Stderr, "warning: %v\n", ferr)
				desk = false
			} else {
				if !*asJSON {
					fmt.Printf("hash\t%s\n", hash)
				}
				return ferr
			}
		}
	}
	out := map[string]any{
		"name":     label,
		"hash":     hash,
		"file":     true,
		"web":      web,
		"desktop":  desk,
		"serve":    serveDbg,
		"deeplink": link,
	}
	if len(keys) > 0 {
		out["keys"] = keys
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	fmt.Printf("hash\t%s\n", hash)
	if link != "" {
		fmt.Printf("deeplink\t%s\n", link)
	}
	if web != "" {
		fmt.Printf("web\t%s\n", web)
	}
	if desk {
		fmt.Println("desktop\tfocused")
	}
	if !skipOpen && web == "" && !desk {
		fmt.Fprintln(os.Stderr, "warning: no running UI found — opened nothing; the hash is waiting if you launch the app or serve within two minutes")
	}
	return nil
}

func readKeysFlag(raw string) ([]string, error) {
	if raw == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading keys from stdin: %w", err)
		}
		raw = string(buf)
	}
	return jql.SplitKeys(raw), nil
}

func looksLikeIssueKey(s string) bool {
	return issueKeyPat.MatchString(strings.ToUpper(strings.TrimSpace(s)))
}

// issueOrExactView focuses detail (issue=KEY) unless a stored view has that
// exact name/id — the view wins. Hash generation does not need a mirror.
func issueOrExactView(name string) (hash, label string) {
	if db, err := openStore(); err == nil {
		v, ferr := findExactView(db, name)
		_ = db.Close()
		if ferr == nil && v.Hash != "" {
			return v.Hash, v.Name
		}
	}
	k := normalizeKey(name)
	return "issue=" + k, k
}

func findExactView(db *store.DB, name string) (listedView, error) {
	list, err := loadViews(db)
	if err != nil {
		return listedView{}, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	var exact []listedView
	for _, v := range list {
		id := strings.ToLower(v.ID)
		nm := strings.ToLower(v.Name)
		ext := ""
		if i := strings.LastIndex(v.ID, ":"); i >= 0 {
			ext = strings.ToLower(v.ID[i+1:])
		}
		if id == want || nm == want || ext == want {
			exact = append(exact, v)
		}
	}
	switch len(exact) {
	case 1:
		return exact[0], nil
	case 0:
		return listedView{}, fmt.Errorf("no view matching %q", name)
	default:
		names := make([]string, len(exact))
		for i, h := range exact {
			names[i] = h.Name
		}
		return listedView{}, fmt.Errorf("%q matches %d views — be more specific: %s", name, len(exact), strings.Join(names, "; "))
	}
}

func viewsSave(args []string) error {
	fs := newFlagSet("views")
	jqlFlag := fs.String("jql", "", "JQL to compile into the view")
	asJSON := fs.Bool("json", false, "emit the saved row as JSON")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
	if name == "" || strings.TrimSpace(*jqlFlag) == "" {
		return usageError("views", `usage: gadak views save <name> --jql '…'`)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	me := identityFromConfig(cfg)
	parsed := jql.Parse(*jqlFlag, jql.Opts{Email: me.Email, AccountID: me.AccountID})
	if parsed.Error != "" {
		return fmt.Errorf("jql: %s", parsed.Message)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	jql.ResolveIdentity(&parsed, peopleFromStore(db), me)
	hash := jql.Hash(parsed.Filters, parsed.Display)
	// Empty hash is what `views open` reports as "nothing to focus". Applied
	// can still list a clause whose values ResolveIdentity then stripped
	// (currentUser() with no identity), so hash — not Applied — is the gate.
	if hash == "" {
		if len(parsed.Unsupported) > 0 {
			return fmt.Errorf("nothing in this JQL can be applied — unsupported: %s", strings.Join(parsed.Unsupported, "; "))
		}
		return fmt.Errorf("nothing in this JQL can be applied")
	}
	body, err := json.Marshal(struct {
		Filters     jql.Filter  `json:"filters"`
		Display     jql.Display `json:"display"`
		JQL         string      `json:"jql,omitempty"`
		Applied     []string    `json:"applied,omitempty"`
		Unsupported []string    `json:"unsupported,omitempty"`
	}{Filters: parsed.Filters, Display: parsed.Display, JQL: strings.TrimSpace(*jqlFlag), Applied: parsed.Applied, Unsupported: parsed.Unsupported})
	if err != nil {
		return err
	}
	id := "cli-" + strings.ReplaceAll(strings.ToLower(name), " ", "-")
	v := store.SavedView{ID: id, Name: name, Config: body}
	if err := db.PutSavedView(context.Background(), v); err != nil {
		return err
	}
	if len(parsed.Unsupported) > 0 {
		fmt.Fprintf(os.Stderr, "warning: JQL skipped %s\n", strings.Join(parsed.Unsupported, "; "))
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"id": id, "name": name, "hash": hash,
			"applied": parsed.Applied, "unsupported": parsed.Unsupported,
		})
	}
	fmt.Printf("saved\t%s\t%s\n", id, name)
	if len(parsed.Unsupported) > 0 {
		fmt.Printf("hash\t%s\n", hash)
		fmt.Printf("applied\t%s\n", strings.Join(parsed.Applied, ", "))
		fmt.Printf("unsupported\t%s\n", strings.Join(parsed.Unsupported, "; "))
	}
	return nil
}

func loadViews(db *store.DB) ([]listedView, error) {
	out := make([]listedView, 0)
	src, err := db.SourceQueries(context.Background(), "jira")
	if err != nil {
		return nil, err
	}
	for _, q := range src {
		out = append(out, listedView{
			Kind: "jira", ID: q.ID, Name: q.Name, JQL: q.QueryText,
			Hash: hashFromConfig(q.Config), Favourite: q.Favourite, Owner: q.Owner,
			Applied: q.Applied, Unsupported: q.Unsupported, Config: q.Config,
		})
	}
	saved, err := db.SavedViews(context.Background())
	if err != nil {
		return nil, err
	}
	for _, s := range saved {
		h, jqlText, applied, unsupported := savedViewFields(s.Config)
		out = append(out, listedView{
			Kind: "saved", ID: s.ID, Name: s.Name,
			JQL: jqlText, Hash: h, Applied: applied, Unsupported: unsupported,
			Config: s.Config,
		})
	}
	return out, nil
}

func findView(db *store.DB, name string) (listedView, error) {
	list, err := loadViews(db)
	if err != nil {
		return listedView{}, err
	}
	want := strings.ToLower(strings.TrimSpace(name))
	var exact, sub []listedView
	for _, v := range list {
		id := strings.ToLower(v.ID)
		nm := strings.ToLower(v.Name)
		ext := ""
		if i := strings.LastIndex(v.ID, ":"); i >= 0 {
			ext = strings.ToLower(v.ID[i+1:])
		}
		if id == want || nm == want || ext == want {
			exact = append(exact, v)
			continue
		}
		if strings.Contains(nm, want) || strings.Contains(id, want) {
			sub = append(sub, v)
		}
	}
	hits := exact
	if len(hits) == 0 {
		hits = sub
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return listedView{}, fmt.Errorf("no view matching %q — run `gadak views` (sync first if you expected Jira filters)", name)
	default:
		names := make([]string, len(hits))
		for i, h := range hits {
			names[i] = h.Name
		}
		return listedView{}, fmt.Errorf("%q matches %d views — be more specific: %s", name, len(hits), strings.Join(names, "; "))
	}
}

func hashFromConfig(raw json.RawMessage) string {
	h, _, _, _ := savedViewFields(raw)
	return h
}

func savedViewFields(raw json.RawMessage) (hash, jqlText string, applied, unsupported []string) {
	if len(raw) == 0 {
		return "", "", nil, nil
	}
	var c struct {
		Filters     jql.Filter  `json:"filters"`
		Display     jql.Display `json:"display"`
		JQL         string      `json:"jql"`
		Applied     []string    `json:"applied"`
		Unsupported []string    `json:"unsupported"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", "", nil, nil
	}
	return jql.Hash(c.Filters, c.Display), c.JQL, c.Applied, c.Unsupported
}

func hashFromJQL(text string) (string, error) {
	cfg, err := config.Load()
	me := jql.Identity{}
	if err == nil {
		me = identityFromConfig(cfg)
	}
	parsed := jql.Parse(text, jql.Opts{Email: me.Email, AccountID: me.AccountID})
	if parsed.Error != "" {
		return "", fmt.Errorf("jql: %s", parsed.Message)
	}
	jql.ResolveIdentity(&parsed, nil, me)
	if len(parsed.Unsupported) > 0 {
		fmt.Fprintf(os.Stderr, "warning: JQL skipped %s\n", strings.Join(parsed.Unsupported, "; "))
	}
	return jql.Hash(parsed.Filters, parsed.Display), nil
}

func identityFromConfig(cfg *config.Config) jql.Identity {
	if cfg == nil {
		return jql.Identity{}
	}
	return jql.Identity{Email: cfg.Email, AccountID: cfg.AccountID}
}

func peopleFromStore(db *store.DB) []jql.Person {
	if db == nil {
		return nil
	}
	lites, err := db.IssueLites(context.Background())
	if err != nil {
		return nil
	}
	issues := make([]jql.Issue, len(lites))
	for i, l := range lites {
		issues[i] = jql.Issue{
			Assignee:      deref(l.Assignee, ""),
			AssigneeEmail: deref(l.AssigneeEmail, ""),
			AssigneeID:    deref(l.AssigneeID, ""),
			Reporter:      deref(l.Reporter, ""),
			ReporterEmail: deref(l.ReporterEmail, ""),
			ReporterID:    deref(l.ReporterID, ""),
		}
	}
	return jql.PeopleFromIssues(issues)
}

func compactJQL(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

type serveTarget struct {
	base    string
	profile string
}

// serveHit is one discovered listener. Production fills this from loopback
// probes; tests inject discoverServes so a live gadak serve cannot flip gates.
type serveHit struct {
	base    string
	profile string
	port    string
}

// serveDebug is the --json explanation of how a serve base was chosen.
// Added as a new object (existing keys stay stable).
type serveDebug struct {
	Ports   []string `json:"ports"`
	Base    string   `json:"base,omitempty"`
	Profile string   `json:"profile,omitempty"`
	Port    string   `json:"port,omitempty"`
}

// openFocusURL is the browser-launch seam for views open. Production calls
// openBrowser; tests replace it so --no-open assertions do not depend on
// whether a listener exists.
var openFocusURL = openBrowser

// discoverServes is the single serve-discovery seam. serveFocusURL and
// (production) listServes both go through it. Tests replace it.
var discoverServes = func() []serveHit {
	var out []serveHit
	for _, port := range serveProbePorts() {
		got := probeGadakOnPort(port, 0)
		if !got.IsGadak {
			continue
		}
		out = append(out, serveHit{
			base:    prettyOpenURL("127.0.0.1", port, nil),
			profile: got.Profile,
			port:    port,
		})
	}
	return out
}

// serveFocusURL is the URL a tab should open, including /w/<profile>/ when
// this CLI profile is not the serve process's primary. Empty when no serve
// is listening. Does not launch a browser.
func serveFocusURL(hash string) string {
	u, _ := resolveServeFocus(hash)
	return u
}

func resolveServeFocus(hash string) (string, serveDebug) {
	t, dbg := findServeTarget()
	if t.base == "" {
		return "", dbg
	}
	return composeServeURL(t.base, workspace.Prefix(config.Profile(), t.profile), hash), dbg
}

func composeServeURL(base, prefix, hash string) string {
	return strings.TrimRight(base, "/") + prefix + "/" + jql.QueryURL(hash)
}

// deepLinkURL is the gadak:// link for a view — the one form of handoff that
// needs neither a shell nor a running serve, which is what makes it the one
// an agent can put in a chat message.
//
// It is always computable, unlike the "web" field beside it in `views open
// --json`: that one is empty unless a serve is already listening, because it
// has to be told which port to name. This is a pure function of the profile
// and the hash, so an agent that built a view always has something to hand
// over. What it does not promise is that anything will answer — that needs
// the desktop app installed. The platforms that ship, and which of them
// receive the deep-link event, live in the table in desktop/README.md;
// focusDesktopApp raises on darwin and windows.
//
// Built from the same prefix the serve URL uses, so the two links describe
// the same view; desktop/deeplink.go reverses exactly this.
func deepLinkURL(profile, hash string) string {
	return deeplink.Compose(deeplink.ActionView, workspace.Prefix(profile, ""), hash)
}

func findServeTarget() (serveTarget, serveDebug) {
	want := config.Profile()
	dbg := serveDebug{Ports: serveProbePorts()}
	var any serveTarget
	var anyPort string
	for _, hit := range discoverServes() {
		t := serveTarget{base: hit.base, profile: hit.profile}
		if workspace.ProfileEq(hit.profile, want) {
			dbg.Base = hit.base
			dbg.Profile = hit.profile
			dbg.Port = hit.port
			return t, dbg
		}
		if any.base == "" {
			any = t
			anyPort = hit.port
		}
	}
	if any.base != "" {
		dbg.Base = any.base
		dbg.Profile = any.profile
		dbg.Port = anyPort
	}
	return any, dbg
}

func serveProbePorts() []string {
	ports := make([]string, 0, 24)
	for p := 7777; p <= 7797; p++ {
		ports = append(ports, fmt.Sprintf("%d", p))
	}
	ports = append(ports, "7878")
	return ports
}

// openServeOnHash resolves the URL and opens a browser. Kept for callers that
// still want the combined action; views open uses serveFocusURL + openBrowser
// so --no-open can still print the link.
func openServeOnHash(hash string) string {
	u := serveFocusURL(hash)
	if u == "" {
		return ""
	}
	if err := openBrowser(u); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open %s (%v)\n", u, err)
		return u
	}
	return u
}

func envNoOpen() bool {
	v := strings.TrimSpace(os.Getenv("GADAK_NO_OPEN"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

// desktopFocusAction is the D5 decision: raise the existing same-profile
// app, refuse to steal another profile's window, or do nothing.
type desktopFocusAction int

const (
	desktopFocusNone desktopFocusAction = iota
	desktopFocusRaise
	desktopFocusError
)

// desktopFocusGOOS is the platform seam for focusDesktopApp. Production is
// runtime.GOOS; tests inject "windows" on a Linux/macOS host.
var desktopFocusGOOS = runtime.GOOS

// desktopAppExists / startOpen / listServes are the D5 seams. Production
// talks to the real app bundle and loopback probes; tests replace them.
var desktopAppExists = func() bool {
	if desktopFocusGOOS == "windows" {
		return findWindowsDesktopExe() != ""
	}
	_, err := os.Stat("/Applications/Gadak.app")
	return err == nil
}

var startOpen = func(args ...string) error {
	return exec.Command("open", args...).Start()
}

// startOpenWait is startOpen for a launch whose failure has to be seen.
//
// The deep-link path needs it: an installed Gadak.app older than the
// gadak:// scheme does not register a handler, and `open` reports that only
// through its exit status — measured 2026-08-16, rc=1 with
// kLSApplicationNotFoundErr in ~9ms. startOpen's Start() would discard it and
// the user would watch nothing happen. Waiting costs nothing at that speed,
// and on the success path `open` returns as soon as LaunchServices accepts
// the request rather than when the app finishes launching.
var startOpenWait = func(args ...string) error {
	return exec.Command("open", args...).Run()
}

// desktopProcessName is the executable inside Gadak.app
// (Contents/MacOS/gadak-desktop). pgrep -x matches this, not the bundle
// display name "Gadak". Single owner — every running-app check goes through
// lookDesktopProcess(desktopProcessName).
const desktopProcessName = "gadak-desktop"

// lookDesktopProcess is the process-census seam. Production uses pgrep on
// Unix and tasklist on Windows (inbox, not PowerShell); tests replace it
// so they can assert the name without touching a live process.
var lookDesktopProcess = func(name string) bool {
	if desktopFocusGOOS == "windows" {
		return lookWindowsProcess(name)
	}
	return exec.Command("pgrep", "-xq", name).Run() == nil
}

// desktopAppRunning is true when a Gadak.app process is already up.
// open -a launches the default profile when nothing is running; we only
// raise when the process exists so a CLI `gadak serve --profile X` does
// not spawn a default window.
var desktopAppRunning = func() bool {
	return lookDesktopProcess(desktopProcessName)
}

var listServes = func() []gadakProbe {
	var out []gadakProbe
	for _, hit := range discoverServes() {
		out = append(out, gadakProbe{IsGadak: true, Profile: hit.profile})
	}
	return out
}

// decideDesktopFocus is the single owner of "may we run open -a?".
// The desktop app serves in-process and does not bind the CLI probe ports,
// so a matching serve is sufficient but not necessary. If the app process
// is up, raise it. Error only when there is no app process and no serve
// at all — that is the case where open -a would launch the default profile
// under a named request (D5: do not silently focus/launch another profile).
func decideDesktopFocus(appInstalled, appRunning bool, want string, running []gadakProbe) (desktopFocusAction, string) {
	if !appInstalled {
		return desktopFocusNone, ""
	}
	wantN := want
	if wantN == "default" {
		wantN = ""
	}
	var seen []string
	match := false
	for _, p := range running {
		if !p.IsGadak {
			continue
		}
		got := p.Profile
		if got == "default" {
			got = ""
		}
		label := got
		if label == "" {
			label = "default"
		}
		seen = append(seen, label)
		if got == wantN {
			match = true
		}
	}
	if appRunning {
		return desktopFocusRaise, ""
	}
	if match {
		// CLI serve is the UI; do not launch Gadak.app (that would be default).
		return desktopFocusNone, ""
	}
	name := wantN
	if name == "" {
		name = "default"
	}
	if len(seen) == 0 && wantN != "" {
		return desktopFocusError, noProfileWindowMsg(name, seen)
	}
	if len(seen) == 0 {
		// Default profile, Gadak.app present, nothing listening: launch it.
		return desktopFocusRaise, ""
	}
	// Some other profile's serve is up. The web path already opened /w/<want>/.
	// Do not open -a (would launch or raise the wrong profile).
	return desktopFocusNone, ""
}

func noProfileWindowMsg(name string, running []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "no window for profile %q", name)
	if len(running) > 0 {
		fmt.Fprintf(&b, " (running: %s)", strings.Join(running, ", "))
	}
	if name == "default" {
		b.WriteString(" — start it with `gadak serve` or switch workspace in the running app")
	} else {
		fmt.Fprintf(&b, " — start it with `gadak serve --profile %s` or switch workspace in the running app", name)
	}
	return b.String()
}

// focusDesktopApp raises Gadak.app on the view named by link, when the D5
// rules allow raising at all.
//
// link is the gadak:// URL for what we are focusing, and it is the preferred
// way in: one `open` both launches-or-raises the app and tells it which view
// to show, where `open -a Gadak` only raises and leaves the uifocus file to
// carry the view separately. That file is written either way — a serve tab
// reads it too — but as the desktop app's channel it had two failure modes
// the link does not: the two-minute freshness window, and a hash consumed
// per workspace mount, so one written for a profile the window is not
// currently showing is never applied.
//
// An empty link, or an installed app too old to register the scheme, falls
// back to the raise-only path with the file. That is why this branch waits
// on `open`: an unregistered scheme is reported through the exit status and
// nowhere else.
func focusDesktopApp(link string) (bool, error) {
	if desktopFocusGOOS != "darwin" && desktopFocusGOOS != "windows" {
		return false, nil
	}
	act, msg := decideDesktopFocus(desktopAppExists(), desktopAppRunning(), config.Profile(), listServes())
	switch act {
	case desktopFocusRaise:
		if desktopFocusGOOS == "windows" {
			return raiseDesktopWindows(link)
		}
		if link != "" && startOpenWait(link) == nil {
			return true, nil
		}
		if err := startOpen("-a", "Gadak"); err != nil {
			return false, nil
		}
		return true, nil
	case desktopFocusError:
		return false, fmt.Errorf("%s", msg)
	default:
		return false, nil
	}
}

// startWindowsDesktop launches the portable gadak-desktop.exe. A second
// instance is forwarded by wails SingleInstance (raise + applyDeepLink);
// a first instance reads the gadak:// arg after the window is ready.
var startWindowsDesktop = startWindowsDesktopImpl

// raiseWindowsWindow brings the existing Gadak window forward by title.
var raiseWindowsWindow = raiseWindowsWindowImpl

func raiseDesktopWindows(link string) (bool, error) {
	err := startWindowsDesktop(link)
	raised := raiseWindowsWindow()
	if err != nil {
		if raised && link == "" {
			return true, nil
		}
		if raised && link != "" {
			return false, fmt.Errorf("raised Gadak but could not deliver the link: %w", err)
		}
		return false, nil
	}
	return true, nil
}

func startWindowsDesktopImpl(link string) error {
	exe := findWindowsDesktopExe()
	if exe == "" {
		return fmt.Errorf("gadak-desktop.exe not found next to this CLI or on PATH")
	}
	var args []string
	if link != "" {
		args = []string{link}
	}
	return exec.Command(exe, args...).Start()
}

// findWindowsDesktopExe is the portable-bundle layout: gadak.exe and
// gadak-desktop.exe sit in the same directory (desktop/build-windows.ps1).
func findWindowsDesktopExe() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		for _, name := range []string{"gadak-desktop.exe", "gadak-desktop"} {
			p := filepath.Join(dir, name)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	if p, err := exec.LookPath("gadak-desktop.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("gadak-desktop"); err == nil {
		return p
	}
	return ""
}

func lookWindowsProcess(name string) bool {
	image := name
	if !strings.HasSuffix(strings.ToLower(image), ".exe") {
		image += ".exe"
	}
	// tasklist is the inbox census. golang.org/x/sys/windows is
	// GOOS-constrained and this file is compiled on every platform; we do
	// not shell out to PowerShell.
	if runtime.GOOS != "windows" {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+image, "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(image))
}

func raiseWindowsWindowImpl() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return raiseGadakWindowByTitle("Gadak")
}
