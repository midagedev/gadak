package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/uifocus"
)

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
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"views": list})
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
	if v.JQL != "" {
		fmt.Printf("jql\t%s\n", v.JQL)
	}
	if v.Hash != "" {
		fmt.Printf("hash\t%s\n", v.Hash)
	}
	if len(v.Applied) > 0 {
		fmt.Printf("applied\t%s\n", strings.Join(v.Applied, ", "))
	}
	if len(v.Unsupported) > 0 {
		fmt.Printf("unsupported\t%s\n", strings.Join(v.Unsupported, "; "))
	}
	return nil
}

func viewsOpen(args []string) error {
	fs := newFlagSet("views")
	jqlFlag := fs.String("jql", "", "open this JQL as a view (instead of a stored name)")
	asJSON := fs.Bool("json", false, "emit the hash and where it was sent")
	noOpenFlag := fs.Bool("no-open", false, "write the hash only; do not open a window")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	var hash, label string
	if strings.TrimSpace(*jqlFlag) != "" {
		h, err := hashFromJQL(*jqlFlag)
		if err != nil {
			return err
		}
		hash, label = h, "jql"
	} else {
		name := strings.TrimSpace(strings.Join(pos, " "))
		if name == "" {
			return usageError("views", `usage: gadak views open <name-or-id> | --jql '…'`)
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
		if v.Hash == "" {
			return fmt.Errorf("view %q has no gadak hash — nothing to focus", v.Name)
		}
		hash, label = v.Hash, v.Name
	}

	if err := uifocus.Write(hash); err != nil {
		return err
	}
	skipOpen := *noOpenFlag || envNoOpen()
	web, desk := "", false
	if !skipOpen {
		web = openServeOnHash(hash)
		desk = focusDesktopApp()
	}
	out := map[string]any{
		"name":    label,
		"hash":    hash,
		"file":    true,
		"web":     web,
		"desktop": desk,
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	fmt.Printf("hash\t%s\n", hash)
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
	email := ""
	if cfg != nil {
		email = cfg.Email
	}
	parsed := jql.Parse(*jqlFlag, jql.Opts{Email: email})
	if parsed.Error != "" {
		return fmt.Errorf("jql: %s", parsed.Message)
	}
	body, err := json.Marshal(struct {
		Filters jql.Filter  `json:"filters"`
		Display jql.Display `json:"display"`
	}{Filters: parsed.Filters, Display: parsed.Display})
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
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
			"id": id, "name": name, "hash": jql.Hash(parsed.Filters, parsed.Display),
			"applied": parsed.Applied, "unsupported": parsed.Unsupported,
		})
	}
	fmt.Printf("saved\t%s\t%s\n", id, name)
	return nil
}

func loadViews(db *store.DB) ([]listedView, error) {
	var out []listedView
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
		out = append(out, listedView{
			Kind: "saved", ID: s.ID, Name: s.Name,
			Hash: hashFromConfig(s.Config), Config: s.Config,
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
	if len(raw) == 0 {
		return ""
	}
	var c struct {
		Filters jql.Filter  `json:"filters"`
		Display jql.Display `json:"display"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return ""
	}
	return jql.Hash(c.Filters, c.Display)
}

func hashFromJQL(text string) (string, error) {
	cfg, err := config.Load()
	email := ""
	if err == nil && cfg != nil {
		email = cfg.Email
	}
	parsed := jql.Parse(text, jql.Opts{Email: email})
	if parsed.Error != "" {
		return "", fmt.Errorf("jql: %s", parsed.Message)
	}
	if email != "" {
		for i, a := range parsed.Filters.AssigneeEmail {
			if strings.EqualFold(a, "currentUser()") {
				parsed.Filters.AssigneeEmail[i] = email
			}
		}
	}
	if len(parsed.Unsupported) > 0 {
		fmt.Fprintf(os.Stderr, "warning: JQL skipped %s\n", strings.Join(parsed.Unsupported, "; "))
	}
	return jql.Hash(parsed.Filters, parsed.Display), nil
}

func compactJQL(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

func openServeOnHash(hash string) string {
	base := findServeBase()
	if base == "" {
		return ""
	}
	frag := "#/"
	if hash != "" {
		frag = "#/?" + hash
	}
	u := strings.TrimRight(base, "/") + "/" + frag
	if err := openBrowser(u); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open %s (%v)\n", u, err)
		return u
	}
	return u
}

func findServeBase() string {
	want := config.Profile()
	ports := make([]string, 0, 24)
	for p := 7777; p <= 7797; p++ {
		ports = append(ports, fmt.Sprintf("%d", p))
	}
	ports = append(ports, "7878")
	for _, port := range ports {
		got := probeGadakOnPort(port, 0)
		if !got.IsGadak || !profileEq(got.Profile, want) {
			continue
		}
		return prettyOpenURL("127.0.0.1", port, nil)
	}
	return ""
}

func profileEq(got, want string) bool {
	norm := func(s string) string {
		if s == "default" {
			return ""
		}
		return s
	}
	return norm(got) == norm(want)
}

func envNoOpen() bool {
	v := strings.TrimSpace(os.Getenv("GADAK_NO_OPEN"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func focusDesktopApp() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if _, err := os.Stat("/Applications/Gadak.app"); err != nil {
		return false
	}
	args := []string{"-a", "Gadak"}
	if p := strings.TrimSpace(config.Profile()); p != "" && p != "default" {
		// Same profile the CLI just wrote ui-focus.json into. Without this,
		// `open -a` focuses whichever Gadak window is already up — often the
		// default profile, which will never see the file.
		args = []string{"--env", "GADAK_PROFILE=" + p, "-a", "Gadak"}
	}
	cmd := exec.Command("open", args...)
	return cmd.Start() == nil
}
