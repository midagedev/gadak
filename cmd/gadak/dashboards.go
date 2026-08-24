package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/dashboards"
	"github.com/midagedev/gadak/internal/uifocus"
)

// Agent dashboards (GDK-781): a saved HTML wall plus named datasources,
// stored in local.db like a saved view. The verb set follows recipes/views;
// config interpretation (validation, datasource execution) is
// internal/dashboards — this file is the argument surface only.

func cmdDashboards(args []string) error {
	if wantsHelp(args) {
		printHelp("dashboards")
		return nil
	}
	sub, rest := "list", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list", "show", "open", "save", "rm":
			sub, rest = args[0], args[1:]
		default:
			sub, rest = "show", args
		}
	}
	switch sub {
	case "list":
		return dashboardsList(rest)
	case "show":
		return dashboardsShow(rest)
	case "open":
		return dashboardsOpen(rest)
	case "save":
		return dashboardsSave(rest)
	case "rm":
		return dashboardsRm(rest)
	default:
		return usageError("dashboards", `usage: gadak dashboards [list|show|open|save|rm]`)
	}
}

func dashboardsList(args []string) error {
	fs := newFlagSet("dashboards")
	asJSON := fs.Bool("json", false, "emit dashboards as JSON")
	if _, err := parseAround(fs, args); err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	list, err := db.Dashboards(context.Background())
	if err != nil {
		return err
	}
	if *asJSON {
		rows := make([]map[string]string, 0, len(list))
		for _, d := range list {
			rows = append(rows, map[string]string{"id": d.ID, "name": d.Name, "updated_at": d.UpdatedAt})
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"dashboards": rows})
	}
	for _, d := range list {
		fmt.Printf("%s\t%s\t%s\n", d.ID, d.Name, d.UpdatedAt)
	}
	return nil
}

func dashboardsShow(args []string) error {
	fs := newFlagSet("dashboards")
	asJSON := fs.Bool("json", false, "emit one dashboard row as JSON (config included)")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
	if name == "" {
		return usageError("dashboards", `usage: gadak dashboards show <name-or-id>`)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	d, err := dashboards.FindDashboard(db, name)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(d)
	}
	cfg, err := dashboards.ParseConfig(d.Config)
	if err != nil {
		return fmt.Errorf("stored config does not parse: %w", err)
	}
	fmt.Printf("id\t%s\n", d.ID)
	fmt.Printf("name\t%s\n", d.Name)
	fmt.Printf("created_at\t%s\n", d.CreatedAt)
	fmt.Printf("updated_at\t%s\n", d.UpdatedAt)
	fmt.Printf("html_bytes\t%d\n", len(cfg.HTML))
	// Datasource names print sorted so repeated runs diff clean.
	names := make([]string, 0, len(cfg.Datasources))
	for n := range cfg.Datasources {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		src := cfg.Datasources[n]
		kind, query := "sql", src.SQL
		if src.JQL != "" {
			kind, query = "jql", src.JQL
		}
		fmt.Printf("datasource\t%s\t%s\t%s\n", n, kind, compactJQL(query))
	}
	return nil
}

// dashboardsSave composes a config from --html and repeated --datasource
// flags, then hands it to internal/dashboards for validation — the same
// single owner the server API uses. Nothing is written until it validates.
func dashboardsSave(args []string) error {
	fs := newFlagSet("dashboards")
	htmlFlag := fs.String("html", "", "HTML document file; - reads stdin")
	var sources labelFlags
	fs.Var(&sources, "datasource", "named datasource: name=sql:QUERY or name=jql:QUERY (repeatable)")
	asJSON := fs.Bool("json", false, "emit the saved row as JSON")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
	if name == "" || strings.TrimSpace(*htmlFlag) == "" {
		return usageError("dashboards", `usage: gadak dashboards save <name> --html <file|-> [--datasource name=sql:…]…`)
	}
	html, err := readHTMLFlag(*htmlFlag)
	if err != nil {
		return err
	}
	cfg := dashboards.Config{HTML: html, Datasources: map[string]dashboards.Source{}}
	for _, spec := range sources {
		src, err := parseDatasourceFlag(spec)
		if err != nil {
			return err
		}
		cfg.Datasources[src.name] = src.source
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if _, err := dashboards.ParseConfig(body); err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	saved, err := db.SaveDashboard(context.Background(), name, string(body))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(saved)
	}
	fmt.Printf("saved\t%s\t%s\n", saved.ID, saved.Name)
	return nil
}

type datasourceFlag struct {
	name   string
	source dashboards.Source
}

// parseDatasourceFlag splits name=sql:QUERY / name=jql:QUERY. The prefix is
// required — a bare query has no kind, and guessing (sql, because most are)
// would save a dashboard whose card silently never runs.
func parseDatasourceFlag(spec string) (datasourceFlag, error) {
	name, value, ok := strings.Cut(spec, "=")
	if !ok {
		return datasourceFlag{}, usageError("dashboards",
			`--datasource must be name=sql:QUERY or name=jql:QUERY, got `+spec)
	}
	var out datasourceFlag
	switch {
	case strings.HasPrefix(value, "sql:"):
		out = datasourceFlag{name: name, source: dashboards.Source{SQL: strings.TrimSpace(value[len("sql:"):])}}
	case strings.HasPrefix(value, "jql:"):
		out = datasourceFlag{name: name, source: dashboards.Source{JQL: strings.TrimSpace(value[len("jql:"):])}}
	default:
		return datasourceFlag{}, usageError("dashboards",
			`--datasource must be name=sql:QUERY or name=jql:QUERY, got `+spec)
	}
	return out, nil
}

func readHTMLFlag(v string) (string, error) {
	if v == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading html from stdin: %w", err)
		}
		return string(buf), nil
	}
	buf, err := os.ReadFile(v)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func dashboardsRm(args []string) error {
	fs := newFlagSet("dashboards")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
	if name == "" {
		return usageError("dashboards", `usage: gadak dashboards rm <name-or-id>`)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	d, err := dashboards.FindDashboard(db, name)
	if err != nil {
		return err
	}
	if err := db.DeleteDashboard(context.Background(), d.ID); err != nil {
		return err
	}
	fmt.Printf("deleted\t%s\t%s\n", d.ID, d.Name)
	return nil
}

// dashboardsOpen focuses a dashboard in the running UI. The focus hash is
// `dash=<id>` — a different namespace from view hashes, read by the same
// one-shot file. Views' focus matrix (serve URL, deep link, desktop raise)
// is hash-agnostic, so it is reused as-is.
func dashboardsOpen(args []string) error {
	fs := newFlagSet("dashboards")
	noOpenFlag := fs.Bool("no-open", false, "write the focus hash only; do not open a window")
	asJSON := fs.Bool("json", false, "emit the hash and where it was sent")
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
	if name == "" {
		return usageError("dashboards", `usage: gadak dashboards open <name-or-id>`)
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	d, err := dashboards.FindDashboard(db, name)
	if err != nil {
		return err
	}
	hash := "dash=" + d.ID
	if err := uifocus.Write(hash); err != nil {
		return err
	}
	skipOpen := *noOpenFlag || envNoOpen()
	web, serveDbg := resolveServeFocus(hash)
	link := deepLinkURL(config.Profile(), hash)
	desk := false
	if !skipOpen {
		if web != "" {
			if err := openFocusURL(web); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not open %s (%v)\n", web, err)
			}
		}
		desk, err = focusDesktopApp(link)
		if err != nil {
			if web != "" {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				desk = false
			} else {
				if !*asJSON {
					fmt.Printf("hash\t%s\n", hash)
				}
				return err
			}
		}
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"name": d.Name, "hash": hash, "file": true,
			"web": web, "desktop": desk, "serve": serveDbg, "deeplink": link,
		})
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
