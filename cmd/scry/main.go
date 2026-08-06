// Command scry serves a local mirror of your issue tracker.
//
// Implemented: init, sync (--full/--watch), serve (syncs by default), tui,
// issue, search, comment, transition, assign, sql, status, mcp, demo,
// export-static, install-service, profiles, version, snapshot, team.
// See specs/000-product/tasks.md for the current state of each.
//
// The agent-facing commands live in agent.go; AGENTS.md is their reference.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	scry "github.com/midagedev/scry"
	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/jira"
	"github.com/midagedev/scry/internal/selfupdate"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/snapshot"
	"github.com/midagedev/scry/internal/store"
	syncer "github.com/midagedev/scry/internal/sync"
)

var version = "0.0.0-dev"

// spaHandler serves the built UI, falling back to index.html so client-side
// routes survive a reload.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

// spaHandlerFS serves the embedded web UI with the same SPA fallback:
// unknown paths get index.html so hash-routed deep links work.
func spaHandlerFS(fsys fs.FS) http.Handler {
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(fsys, "index.html")
		if err != nil {
			http.Error(w, "web UI not embedded — build with `npm run build` before `go build`, or pass --static", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

func openStore() (*store.DB, error) {
	path, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}

func cmdServe(args []string) error {
	fs := newFlagSet("serve")
	addr := fs.String("addr", "127.0.0.1:7777", "listen address")
	static := fs.String("static", "", "serve the web UI from this directory instead of the embedded copy")
	allowRemote := fs.Bool("allow-remote", false,
		"permit binding a non-loopback address (the mirror has no auth; do not expose it)")
	// Sync starts by default when a credential is configured. --sync is kept as
	// a deprecated no-op alias; --no-sync opts out (demo / e2e fixtures).
	withSync := fs.Bool("sync", false, "deprecated alias (sync already starts when a credential is configured)")
	noSync := fs.Bool("no-sync", false, "do not run the incremental sync loop")
	importAttachments := fs.String("import-attachments", "",
		"seed the attachment cache from a directory holding manifest.json (see examples/attachments)")
	noOpen := fs.Bool("no-open", false, "do not open the browser after the server starts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// --addr pin: user forced a port → no fallback on conflict (rule 3).
	addrPinned := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrPinned = true
		}
	})

	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("bad --addr %q: %w", *addr, err)
	}
	// The server has no authentication by design, so a non-loopback bind would
	// publish the whole mirror to the network. Require an explicit opt-in.
	if !*allowRemote && host != "" && !isLoopback(host) {
		return fmt.Errorf("refusing to bind non-loopback address %q without --allow-remote", host)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	if *importAttachments != "" {
		if err := importAttachmentDir(*importAttachments); err != nil {
			log.Printf("warning: could not import attachments from %q: %v", *importAttachments, err)
		}
	}

	// Attachment bytes live on disk next to the mirror: the first view fetches
	// them from Jira, every later one is local, and a cached image still renders
	// with no credential at all.
	var api *server.Handler
	if dir, err := config.AttachmentDir(); err != nil {
		log.Printf("warning: attachment cache disabled: %v", err)
		api = server.New(db, cfg)
	} else if cache, err := attachcache.New(dir, int64(cfg.AttachmentCacheMB)<<20); err != nil {
		log.Printf("warning: attachment cache disabled: %v", err)
		api = server.New(db, cfg)
	} else {
		api = server.NewWithCache(db, cfg, cache)
	}

	// The embedded UI is the default; --static overrides it for development
	// (`npm run build` output) without rebuilding the binary.
	var spa http.Handler
	if *static != "" {
		if _, err := os.Stat(*static); err != nil {
			log.Printf("warning: static dir %q not found — run `npm run build` first", *static)
		}
		spa = spaHandler(*static)
	} else {
		ui, ok := scry.WebUI()
		if !ok {
			log.Printf("warning: no web UI embedded in this binary — build with `npm run build` before `go build`, or pass --static")
		}
		spa = spaHandlerFS(ui)
	}

	// Workspace mounts share this process's listener; each profile opens lazily.
	// Background sync and update checks stay on the primary handler only.
	reg := newWorkspaceRegistry()
	defer reg.Close()
	mux := buildServeMux(api, spa, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Optional once-a-day GitHub release check (opt-out via updateCheck: false).
	// Independent of Jira credentials; silent on failure.
	if dir, err := config.Dir(); err == nil {
		api.StartUpdateCheck(ctx, dir)
	}

	// Default: keep the mirror fresh whenever a credential exists. Empty
	// projects means "everything this account can see". --no-sync opts out
	// (fixtures with a fake token must pass it). --sync remains a silent alias
	// when the loop would start anyway; with no credential it still prints the
	// old guidance line. When serve starts without a credential, register the
	// same starter so PUT onboarding/connect/ can kick off Watch once after
	// the first successful save. Workspace handlers have no background loop.
	if !*noSync {
		startWatch := func() {
			go func() {
				// Reload so a late setup does not capture a stale empty config.
				cur, err := config.Load()
				if err != nil {
					log.Printf("sync loop: load config: %v", err)
					return
				}
				if err := syncer.Watch(ctx, cur, db, syncer.Options{Log: func(s string) { log.Print(s) }}); err != nil && ctx.Err() == nil {
					log.Printf("sync loop stopped: %v", err)
				}
			}()
		}
		if cfg.HasCredential() {
			startWatch()
		} else {
			api.SetSyncStarter(startWatch)
			if *withSync {
				log.Printf("--sync ignored: run `scry init` first")
			}
		}
	}

	// Bind before serving so EADDRINUSE can be handled: same-profile scry →
	// hand off (exit 0); other occupant + default addr → next free port;
	// explicit --addr → hard error (with scry identity when known).
	preferred := *addr
	ln, bound, existingURL, occupant, err := bindListenDetail(preferred, addrPinned, config.Profile(), nil, nil)
	if err != nil {
		return err
	}
	if existingURL != "" {
		log.Printf("already serving at %s (same profile)", existingURL)
		if !*noOpen {
			if openErr := openBrowser(existingURL); openErr != nil {
				log.Printf("could not open a browser: %v — visit %s", openErr, existingURL)
			}
		}
		return nil
	}
	if bound != preferred {
		_, prefPort, _ := net.SplitHostPort(preferred)
		_, boundPort, _ := net.SplitHostPort(bound)
		if occupant == "" {
			occupant = "another process"
		}
		log.Printf("port %s busy (%s) — serving on %s", prefPort, occupant, boundPort)
	}
	defer ln.Close()

	srv := &http.Server{
		Addr:              bound,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shctx)
	}()
	openURL := browseAddr(bound)
	log.Printf("scry %s listening on %s", version, openURL)
	if p := config.Profile(); p != "" {
		log.Printf("profile: %s", p)
	}
	if len(cfg.Projects) == 0 && cfg.HasCredential() {
		log.Printf("no project filter — syncing everything this account can see")
	}
	if !*noOpen {
		go openOnceUp(openURL)
	}
	err = srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// stdinIsTerminal reports whether stdin is a character device. Used so init can
// refuse to block on a prompt when an agent or pipe is driving the CLI.
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Injection points so tests can exercise the interactive branch, which cannot
// be reached through a pipe.
var initStdin io.Reader = os.Stdin
var initIsTerminal = stdinIsTerminal

// parseProjectKeys splits a comma-separated project list the same way the
// interactive init path always has: trim, upper-case, drop empties.
func parseProjectKeys(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(strings.ToUpper(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// initMissingError lists every value still empty and how to supply it without a
// prompt. reason is why prompting was skipped (not a TTY, --json, --token-stdin).
// projects is optional (empty = every project the account can see).
func initMissingError(missing []string, reason string) error {
	return fmt.Errorf("missing: %s (%s)\nsupply them with flags (--site, --email) or environment\n(SCRY_SITE, SCRY_EMAIL, SCRY_TOKEN); for the token also\n--token-file <path> or --token-stdin; optional --projects / SCRY_PROJECTS",
		strings.Join(missing, ", "), reason)
}

// cmdInit writes site/email/token/projects to config after verifying against
// /myself. Classic interactive mode (TTY, no supply flags/env, no --json)
// re-prompts credentials (and optional projects) so a human can replace an
// expired token. Any non-interactive supply turns prompting off entirely.
// Projects are optional: blank means sync every project the account can see.
func cmdInit(args []string) error {
	fs := newFlagSet("init")
	siteFlag := fs.String("site", "", "Jira site URL (https://your-site.atlassian.net)")
	emailFlag := fs.String("email", "", "account email")
	projectsFlag := fs.String("projects", "", "project keys, comma-separated (optional — blank syncs every project you can see)")
	tokenFile := fs.String("token-file", "", "read API token from this file")
	tokenStdin := fs.Bool("token-stdin", false, "read API token from stdin")
	// Defined only so a mistaken `--token secret` gets a clear error instead of
	// "flag provided but not defined"; the value must never be accepted (ps/history).
	tokenFlag := fs.String("token", "", "not accepted; use SCRY_TOKEN, --token-file, or --token-stdin")
	jsonOut := fs.Bool("json", false, "emit one JSON object on success")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tokenFlag != "" {
		return fmt.Errorf("--token is not accepted: it would be visible in `ps` and shell history.\nuse SCRY_TOKEN=..., --token-file <path>, or --token-stdin")
	}
	if *tokenStdin && *tokenFile != "" {
		return fmt.Errorf("use only one of --token-file or --token-stdin")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	envSite := os.Getenv("SCRY_SITE")
	envEmail := os.Getenv("SCRY_EMAIL")
	envToken := os.Getenv("SCRY_TOKEN")
	envProjects := os.Getenv("SCRY_PROJECTS")

	// Any supply flag or env forces non-interactive; half-prompted states are unpredictable for agents.
	suppliedFlag := *siteFlag != "" || *emailFlag != "" || *projectsFlag != "" || *tokenFile != "" || *tokenStdin
	suppliedEnv := envSite != "" || envEmail != "" || envToken != "" || envProjects != ""
	classic := initIsTerminal() && !*jsonOut && !suppliedFlag && !suppliedEnv

	var site, email, token string
	var projects []string

	if classic {
		// Start from saved values; empty answers keep them (token never echoed).
		site = strings.TrimRight(cfg.Site, "/")
		email = cfg.Email
		token = cfg.Token
		projects = append([]string(nil), cfg.Projects...)

		in := bufio.NewReader(initStdin)
		prompt := func(label, current string) string {
			if current != "" {
				fmt.Printf("%s [%s]: ", label, current)
			} else {
				fmt.Printf("%s: ", label)
			}
			line, _ := in.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				return current
			}
			return line
		}
		site = strings.TrimRight(prompt("Jira site URL (https://your-site.atlassian.net)", site), "/")
		email = prompt("Account email", email)
		// Token: keep-hint in the label only — never print the secret as [current].
		tokenLabel := "API token (id.atlassian.com/manage-profile/security/api-tokens)"
		if token != "" {
			tokenLabel += " [configured; enter to keep]"
		}
		if v := prompt(tokenLabel, ""); v != "" {
			token = v
		}
		projects = parseProjectKeys(prompt("Project keys, comma-separated (optional — blank syncs every project you can see)", strings.Join(projects, ",")))
	} else {
		// flag > env > saved; never prompt.
		site = strings.TrimRight(cfg.Site, "/")
		if envSite != "" {
			site = strings.TrimRight(envSite, "/")
		}
		if *siteFlag != "" {
			site = strings.TrimRight(*siteFlag, "/")
		}

		email = cfg.Email
		if envEmail != "" {
			email = envEmail
		}
		if *emailFlag != "" {
			email = *emailFlag
		}

		token = cfg.Token
		if envToken != "" {
			token = envToken
		}
		switch {
		case *tokenStdin:
			b, err := io.ReadAll(initStdin)
			if err != nil {
				return fmt.Errorf("reading token from stdin: %w", err)
			}
			token = strings.TrimSpace(string(b))
		case *tokenFile != "":
			b, err := os.ReadFile(*tokenFile)
			if err != nil {
				return fmt.Errorf("reading --token-file: %w", err)
			}
			token = strings.TrimSpace(string(b))
		}

		projects = append([]string(nil), cfg.Projects...)
		if envProjects != "" {
			projects = parseProjectKeys(envProjects)
		}
		if *projectsFlag != "" {
			projects = parseProjectKeys(*projectsFlag)
		}

		var missing []string
		if site == "" {
			missing = append(missing, "site")
		}
		if email == "" {
			missing = append(missing, "email")
		}
		if token == "" {
			missing = append(missing, "token")
		}
		// projects is optional: empty means every project the account can see.
		if len(missing) > 0 {
			reason := "stdin is not a terminal, so init cannot prompt"
			switch {
			case *jsonOut:
				reason = "--json forbids interactive prompts"
			case *tokenStdin:
				reason = "--token-stdin consumes stdin, so init cannot prompt"
			case suppliedFlag || suppliedEnv:
				// TTY but non-classic: flags/env opted into non-interactive fill.
				if initIsTerminal() {
					reason = "non-interactive supply was used, so init cannot prompt"
				}
			}
			return initMissingError(missing, reason)
		}
	}

	cfg.Site = site
	cfg.Email = email
	cfg.Token = token
	cfg.Projects = projects

	if !cfg.HasCredential() {
		return fmt.Errorf("site, email, and token are all required")
	}
	// Same verification as the server credential / onboarding endpoints (jira /myself).
	me, err := jira.New(cfg.Site, cfg.Email, cfg.Token).Myself(context.Background())
	if err != nil {
		if errors.Is(err, jira.ErrAuth) {
			// Restore the pre-jira.Myself hint: org API keys are a common mistake.
			return fmt.Errorf("credential check failed: %w (org API keys do not work; use a user token)", err)
		}
		return fmt.Errorf("credential check failed: %w", err)
	}
	name := me.DisplayName
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		// One line, no HTML escaping — machine consumers parse this.
		enc.SetEscapeHTML(false)
		return enc.Encode(struct {
			Profile  string   `json:"profile"`
			Account  string   `json:"account"`
			Site     string   `json:"site"`
			Projects []string `json:"projects"`
			Path     string   `json:"path"`
		}{
			Profile:  config.Profile(),
			Account:  name,
			Site:     cfg.Site,
			Projects: cfg.Projects,
			Path:     p,
		})
	}
	fmt.Printf("verified as %s — saved %s\n", name, p)
	if len(cfg.Projects) == 0 {
		fmt.Println("no project filter — syncing everything this account can see; narrow it later in Settings → Sync")
	}
	fmt.Println("next: scry sync")
	return nil
}

func cmdSync(args []string) error {
	fs := newFlagSet("sync")
	full := fs.Bool("full", false, "force a full sync")
	watch := fs.Bool("watch", false, "keep syncing on an interval")
	source := fs.String("source", "all", "which source to sync: jira, confluence, or all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch *source {
	case "jira", "confluence", "all":
	default:
		return fmt.Errorf("unknown --source %q (want jira, confluence, or all)", *source)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return fmt.Errorf("not configured — run `scry init` first")
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	opts := syncer.Options{Full: *full, Log: func(s string) { log.Print(s) }}
	if *watch {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return syncer.Watch(ctx, cfg, db, opts)
	}
	runJira := *source == "all" || *source == "jira"
	runConf := (*source == "all" || *source == "confluence") && cfg.Confluence != nil
	if *source == "confluence" && cfg.Confluence == nil {
		return fmt.Errorf("confluence is not configured — add a confluence section to config.json")
	}
	if runJira {
		res, err := syncer.Run(context.Background(), cfg, db, opts)
		if err != nil {
			return err
		}
		kind := "incremental"
		if res.Full {
			kind = "full"
		}
		fmt.Printf("%s sync: fetched %d, changed %d, deleted %d, watermark %s\n",
			kind, res.Fetched, res.Changed, res.Deleted, res.Watermark)
	}
	if runConf {
		cres, err := syncer.RunConfluence(context.Background(), cfg, db, opts)
		if err != nil {
			if runJira {
				// Jira already succeeded; log confluence failure but still exit non-zero.
				log.Printf("confluence sync failed: %v", err)
			}
			return err
		}
		kind := "incremental"
		if cres.Full {
			kind = "full"
		}
		fmt.Printf("confluence %s sync: fetched %d pages, changed %d, watermark %s\n",
			kind, cres.Fetched, cres.Changed, cres.Watermark)
	}
	printUpdateNotice(cfg, false)
	return nil
}

// printUpdateNotice prints a one-line brew upgrade hint when a newer release
// is known. withURL adds the release page on a second line (status). Failures
// and opt-out are silent — this is courtesy, not a feature path.
func printUpdateNotice(cfg *config.Config, withURL bool) {
	if cfg == nil || !cfg.UpdateCheckEnabled() {
		return
	}
	dir, err := config.Dir()
	if err != nil {
		return
	}
	info, ok := selfupdate.Check(context.Background(), dir, version, true)
	if !ok || !selfupdate.Newer(version, info.Latest) {
		return
	}
	fmt.Printf("update: v%s available (running v%s) — brew upgrade midagedev/tap/scry\n",
		info.Latest, version)
	if withURL && info.URL != "" {
		fmt.Println(info.URL)
	}
}

// openReadOnly gives sql/status a connection that cannot write, so a typo'd
// UPDATE cannot corrupt the mirror while the server holds the single writer.
func openReadOnly() (*sql.DB, error) {
	path, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no mirror at %s — run `scry sync` first", path)
	}
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

func cmdSQL(args []string) error {
	// The two flags are matched by name wherever they appear instead of with
	// flag.Parse, because a query legitimately starts with `--` — a SQL comment,
	// which flag.Parse reads as an undefined flag and refuses. That is exactly what
	// happens when an agent pastes a commented query out of AGENTS.md.
	if wantsHelp(args) {
		printHelp("sql")
		return nil
	}
	var asJSON, asCSV bool
	var words []string
	for _, a := range args {
		switch a {
		case "--json", "-json":
			asJSON = true
		case "--csv", "-csv":
			asCSV = true
		default:
			words = append(words, a)
		}
	}
	query := strings.TrimSpace(strings.Join(words, " "))
	if query == "" {
		return usageError("sql", `usage: scry sql [--json|--csv] "select ..."`)
	}
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	var csvOut *csv.Writer
	switch {
	case asCSV:
		csvOut = csv.NewWriter(os.Stdout)
		if err := csvOut.Write(cols); err != nil {
			return err
		}
	case !asJSON:
		fmt.Println(strings.Join(cols, "\t"))
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	enc := json.NewEncoder(os.Stdout)
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		if asJSON {
			obj := make(map[string]any, len(cols))
			for i, c := range cols {
				obj[c] = cell(vals[i])
			}
			_ = enc.Encode(obj)
			continue
		}
		out := make([]string, len(cols))
		for i := range vals {
			out[i] = text(vals[i])
		}
		if csvOut != nil {
			if err := csvOut.Write(out); err != nil {
				return err
			}
			continue
		}
		fmt.Println(strings.Join(out, "\t"))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if csvOut != nil {
		csvOut.Flush()
		return csvOut.Error()
	}
	return nil
}

func cell(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// text renders a cell for the row-oriented outputs. NULL prints as empty rather
// than as Go's "<nil>", which no consumer of a tab or CSV row wants to parse.
func text(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(cell(v))
}

// cmdSnapshot writes a shareable mirror copy (no personal tables, no credentials).
// See specs/000-product/contracts/sync.md "Snapshot generation".
//
// Flags may appear before or after <out.db> (same ergonomics as `scry sql`).
func cmdSnapshot(args []string) error {
	const snapshotUsage = "usage: scry snapshot <out.db> [--from db] [--spread 90d] [--scale N] [--seed N] [--now RFC3339] [--force]"
	var from, spread, nowArg string
	scale := 0
	seed := int64(1)
	force := false
	var positionals []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--from" || a == "-from":
			if i+1 >= len(args) {
				return fmt.Errorf("--from needs a path")
			}
			i++
			from = args[i]
		case strings.HasPrefix(a, "--from="):
			from = strings.TrimPrefix(a, "--from=")
		case a == "--spread" || a == "-spread":
			if i+1 >= len(args) {
				return fmt.Errorf("--spread needs a duration")
			}
			i++
			spread = args[i]
		case strings.HasPrefix(a, "--spread="):
			spread = strings.TrimPrefix(a, "--spread=")
		case a == "--scale" || a == "-scale":
			if i+1 >= len(args) {
				return fmt.Errorf("--scale needs an integer")
			}
			i++
			n, err := atoiArg(args[i], "--scale")
			if err != nil {
				return err
			}
			scale = n
		case strings.HasPrefix(a, "--scale="):
			n, err := atoiArg(strings.TrimPrefix(a, "--scale="), "--scale")
			if err != nil {
				return err
			}
			scale = n
		case a == "--seed" || a == "-seed":
			if i+1 >= len(args) {
				return fmt.Errorf("--seed needs an integer")
			}
			i++
			n, err := atoi64Arg(args[i], "--seed")
			if err != nil {
				return err
			}
			seed = n
		case strings.HasPrefix(a, "--seed="):
			n, err := atoi64Arg(strings.TrimPrefix(a, "--seed="), "--seed")
			if err != nil {
				return err
			}
			seed = n
		case a == "--now" || a == "-now":
			if i+1 >= len(args) {
				return fmt.Errorf("--now needs an RFC3339 timestamp")
			}
			i++
			nowArg = args[i]
		case strings.HasPrefix(a, "--now="):
			nowArg = strings.TrimPrefix(a, "--now=")
		case a == "--force" || a == "-force":
			force = true
		case a == "-h" || a == "--help":
			printHelp("snapshot")
			return nil
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %s", a)
		default:
			positionals = append(positionals, a)
		}
	}
	if len(positionals) != 1 {
		return usageError("snapshot", snapshotUsage)
	}
	// --now pins the clock the spread window ends at, so a snapshot can be
	// rebuilt byte-for-byte. Left unset, a snapshot is dated now, which is what
	// keeps a regenerated demo from looking abandoned.
	var now time.Time
	if nowArg != "" {
		t, err := time.Parse(time.RFC3339, nowArg)
		if err != nil {
			return fmt.Errorf("--now %q: want RFC3339 (e.g. 2026-08-05T00:00:00Z)", nowArg)
		}
		now = t
	}
	out := positionals[0]
	src := from
	if src == "" {
		path, err := config.DBPath()
		if err != nil {
			return err
		}
		src = path
	}
	var window time.Duration
	if spread != "" {
		d, err := snapshot.ParseWindow(spread)
		if err != nil {
			return err
		}
		window = d
	}
	res, err := snapshot.Build(snapshot.Options{
		From:   src,
		Out:    out,
		Spread: window,
		Scale:  scale,
		Seed:   seed,
		Force:  force,
		Now:    now,
	})
	if err != nil {
		return err
	}
	extra := ""
	if res.Spread > 0 {
		extra = fmt.Sprintf(", spread %s", res.Spread)
	}
	if scale > 0 {
		extra += fmt.Sprintf(", scale %d", scale)
	}
	fmt.Printf("snapshot %s: %d issues, %d comments, %d changelog%s (%s)\n",
		res.Path, res.Issues, res.Comments, res.Changelog, extra, formatBytes(res.Bytes))
	return nil
}

func atoiArg(s, name string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", name, s)
	}
	return n, nil
}

func atoi64Arg(s, name string) (int64, error) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", name, s)
	}
	return n, nil
}

func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func cmdStatus(args []string) error {
	fs := newFlagSet("status")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// store.Open migrates the mirror (schema v7 field_usage). status only reads
	// issue rows; it may create an empty mirror when none exists yet.
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	st := map[string]any{"profile": config.Profile()}
	if ss, err := db.SyncState("jira"); err == nil {
		st["watermark"] = ss.Watermark
		st["version"] = ss.Version
		st["schema_version"] = ss.SchemaVersion
		st["sync_count"] = ss.SyncCount
		if ss.LastFullSyncAt != nil && *ss.LastFullSyncAt != "" {
			st["last_full_sync_at"] = *ss.LastFullSyncAt
		}
		if ss.LastError != nil && *ss.LastError != "" {
			st["last_error"] = *ss.LastError
		}
		if ss.FirstSyncAt != nil && *ss.FirstSyncAt != "" {
			st["first_sync_at"] = *ss.FirstSyncAt
		}
	}
	if n, err := db.TableCount("issues"); err == nil {
		st["issues"] = n
	}
	if n, err := db.TableCount("comments"); err == nil {
		st["comments"] = n
	}
	usage, err := db.APIUsageSummary()
	if err != nil {
		usage = store.APIUsageSummary{Today: store.APIUsageDay{Day: time.Now().UTC().Format("2006-01-02")}}
	}
	st["api_usage"] = usage

	cfg, _ := config.Load()
	var updateInfo selfupdate.Info
	var updateOK bool
	if cfg != nil && cfg.UpdateCheckEnabled() {
		if dir, err := config.Dir(); err == nil {
			updateInfo, updateOK = selfupdate.Check(context.Background(), dir, version, true)
			if updateOK && selfupdate.Newer(version, updateInfo.Latest) {
				st["update"] = map[string]string{
					"latest": updateInfo.Latest,
					"url":    updateInfo.URL,
				}
			}
		}
	}

	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(st)
	}
	for _, k := range []string{"profile", "issues", "comments", "watermark", "version", "last_full_sync_at", "last_error"} {
		if v, ok := st[k]; ok && v != "" {
			fmt.Printf("%-18s %v\n", k, v)
		}
	}
	if line := formatAPIUsageLine(usage); line != "" {
		fmt.Printf("%-18s %s\n", "api (today)", line)
	}
	if updateOK && selfupdate.Newer(version, updateInfo.Latest) {
		fmt.Printf("update: v%s available (running v%s) — brew upgrade midagedev/tap/scry\n",
			updateInfo.Latest, version)
		if updateInfo.URL != "" {
			fmt.Println(updateInfo.URL)
		}
	}
	return nil
}

// formatAPIUsageLine returns "" when nothing has been counted in the last week,
// so a fresh or credential-less profile does not carry a line that only ever
// says zero. The value is aligned with the other status rows by the caller.
func formatAPIUsageLine(u store.APIUsageSummary) string {
	if u.Today.Requests == 0 && u.Last7Days.Requests == 0 {
		return ""
	}
	line := fmt.Sprintf("%d requests", u.Today.Requests)
	if u.Last7Days.Requests != u.Today.Requests {
		line += fmt.Sprintf(" (%d in 7 days)", u.Last7Days.Requests)
	}
	if u.Today.Throttled > 0 {
		line += fmt.Sprintf(", %d throttled", u.Today.Throttled)
		at := u.Today.LastThrottledAt
		if at == nil {
			at = u.Last7Days.LastThrottledAt
		}
		if at != nil && *at != "" {
			if t, err := time.Parse("2006-01-02T15:04:05.000Z", *at); err == nil {
				line += fmt.Sprintf(" (last %s)", t.UTC().Format("15:04Z"))
			} else if t, err := time.Parse(time.RFC3339, *at); err == nil {
				line += fmt.Sprintf(" (last %s)", t.UTC().Format("15:04Z"))
			}
		}
	}
	return line
}

// cmdDemo serves the bundled snapshot from a throwaway home, so evaluating the
// UI needs no Jira account and cannot touch a real profile.
// importDemoAttachments loads <snapshotDir>/attachments/ into the demo cache.
func importDemoAttachments(snapshotDir, home string) error {
	return importAttachmentsInto(filepath.Join(snapshotDir, "attachments"),
		filepath.Join(home, "attachments"))
}

// importAttachmentDir seeds this profile's cache from a manifest directory. It is
// how a snapshot ships renderable images: bytes cannot be proxied without a
// credential, so a fixture (the demo, the test server, a shared snapshot) hands
// them over instead.
func importAttachmentDir(dir string) error {
	cacheDir, err := config.AttachmentDir()
	if err != nil {
		return err
	}
	return importAttachmentsInto(dir, cacheDir)
}

// importAttachmentsInto is the shared body. A missing directory is not an error:
// the snapshot simply has no images.
func importAttachmentsInto(dir, cacheDir string) error {
	manifestPath := filepath.Join(dir, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var manifest struct {
		Attachments []struct {
			ID          string `json:"id"`
			File        string `json:"file"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
		} `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return err
	}
	cache, err := attachcache.New(cacheDir, 0)
	if err != nil {
		return err
	}
	for _, a := range manifest.Attachments {
		src := filepath.Join(dir, a.File)
		if err := cache.ImportFile(a.ID, src, a.ContentType, a.Filename); err != nil {
			return fmt.Errorf("import %s: %w", a.File, err)
		}
	}
	return nil
}

// freshenDemoClock stamps the throwaway demo copy as just-synced.
func freshenDemoClock(dbPath string) error {
	db, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.FreshenSyncClock()
}

func cmdDemo(args []string) error {
	fs := newFlagSet("demo")
	addr := fs.String("addr", "127.0.0.1:7878", "listen address")
	static := fs.String("static", "dist/app", "directory holding the built web UI")
	dbPath := fs.String("db", "examples/demo.db", "snapshot to serve")
	noOpen := fs.Bool("no-open", false, "do not open the browser after the server starts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	src, err := os.ReadFile(*dbPath)
	if err != nil {
		return fmt.Errorf("demo snapshot %q not found — run from the repo root or pass --db: %w", *dbPath, err)
	}
	home, err := os.MkdirTemp("", "scry-demo-")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(home, "scry.db"), src, 0o600); err != nil {
		return err
	}
	demoCfg := []byte(`{"projects":["NMB","NMA","NMS"]}`)
	if err := os.WriteFile(filepath.Join(home, "config.json"), demoCfg, 0o600); err != nil {
		return err
	}
	os.Setenv("SCRY_HOME", home)
	config.SetProfile("")
	// Attachment bytes cannot be proxied without a credential, so the snapshot
	// ships them and they are imported into the cache: the demo shows real
	// screenshots and inline comment images with no Jira account at all.
	if err := importDemoAttachments(filepath.Dir(*dbPath), home); err != nil {
		log.Printf("demo: attachment bytes unavailable (%v) — images will not render", err)
	}
	// The snapshot ages on the shelf, and a demo that opens with "Sync delayed"
	// reads as a defect rather than as the freshness guard it is. This is a
	// throwaway copy, so stamp its clock as current.
	if err := freshenDemoClock(filepath.Join(home, "scry.db")); err != nil {
		log.Printf("demo: could not freshen the sync clock: %v", err)
	}
	log.Printf("demo mirror in %s (deleted on exit)", home)
	defer os.RemoveAll(home)
	serveArgs := []string{"--addr", *addr, "--static", *static}
	if *noOpen {
		serveArgs = append(serveArgs, "--no-open")
	}
	return cmdServe(serveArgs)
}

// browseAddr turns a listen address into the URL a person should visit:
// a blank or wildcard host becomes localhost; loopback binds prefer
// scry.localhost when the system resolver maps it only to loopback IPs.
func browseAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	return prettyOpenURL(host, port, nil)
}

// hostLookup is the injectable DNS path for prettyOpenURL (tests pass a stub).
type hostLookup func(ctx context.Context, host string) ([]string, error)

// prettyOpenURL chooses the host shown in the terminal and opened in a browser.
// Bind address is unchanged — only the display/open URL may become scry.localhost.
//
// Rules:
//   - non-loopback bind (LAN / --allow-remote) → keep that host
//   - loopback bind (127.0.0.1, ::1, localhost, empty) and lookup("scry.localhost")
//     returns only loopback IPs → http://scry.localhost:<port>
//   - lookup timeout, failure, empty, or any non-loopback result → fallback host
//
// lookup nil uses the default resolver with a 500ms budget so a slow DNS path
// never stalls serve startup.
func prettyOpenURL(bindHost, port string, lookup hostLookup) string {
	fallback := bindHost
	if fallback == "" || fallback == "0.0.0.0" || fallback == "::" {
		fallback = "localhost"
	}
	fallbackURL := "http://" + net.JoinHostPort(fallback, port)

	// Only rewrite when the process is listening on loopback (or empty host,
	// which net treats as all-interfaces but is still the local machine).
	// 0.0.0.0 / :: require --allow-remote and stay on the fallback form.
	if bindHost != "" && !isLoopback(bindHost) {
		return fallbackURL
	}

	if lookup == nil {
		lookup = lookupHostDefault
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	addrs, err := lookup(ctx, "scry.localhost")
	if err != nil || len(addrs) == 0 {
		return fallbackURL
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil || !ip.IsLoopback() {
			return fallbackURL
		}
	}
	return "http://" + net.JoinHostPort("scry.localhost", port)
}

func lookupHostDefault(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

// openOnceUp opens the browser as soon as the server answers /healthz, so the
// tab never lands on a connection error. Gives up quietly after ~5s — a browser
// failing to open must never take the server down with it.
func openOnceUp(u string) {
	for i := 0; i < 50; i++ {
		res, err := http.Get(u + "/healthz")
		if err == nil {
			res.Body.Close()
			if err := openBrowser(u); err != nil {
				log.Printf("could not open a browser: %v — visit %s", err, u)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func cmdProfiles(args []string) error {
	if wantsHelp(args) {
		printHelp("profiles")
		return nil
	}
	names, err := config.Profiles()
	if err != nil {
		return err
	}
	fmt.Println("default")
	for _, n := range names {
		fmt.Println(n)
	}
	return nil
}

func cmdVersion(args []string) error {
	if wantsHelp(args) {
		printHelp("version")
		return nil
	}
	fmt.Println(version)
	return nil
}

const usage = `scry — a local-first mirror of your issue tracker

Usage:
  scry [--profile <name>] <command>

Commands:
  init             configure site, credentials, and projects
                   [--site] [--email] [--projects] [--token-file|--token-stdin] [--json]
  sync             mirror Jira into SQLite   [--full] [--watch]
  serve            web UI + API on loopback  [--addr] [--static] [--no-sync] [--no-open] [--allow-remote]
                   (syncs by default when a credential is configured; --no-sync opts out)
  install-service  keep serve running across reboots (launchd / systemd user)
                   [--uninstall]
  status           sync state and row counts [--json]
  demo             serve the bundled snapshot, no Jira account needed
  export-static    freeze demo.db into static JSON for hosted demo  <outdir>
  tui              terminal issue navigator (local mirror)
  profiles         list configured profiles
  version          print version

Reading the mirror (no network; see AGENTS.md):
  issue      full detail for one issue    <KEY> [--json]
  open       open the issue on your Jira site in the browser  <KEY>
  search     full-text search            [--limit N] [--json] "text"
  sql        read-only SQL               [--json|--csv] "select ..."
  snapshot   shareable copy of the mirror <out.db> [--from db] [--spread 90d] [--scale N]
  mcp        MCP server on stdio; mcp install <client> pins profile (docs/MCP.md)

Writing through to Jira (needs a credential):
  comment    add a comment    <KEY> -m <text|-> [--json]
  transition change status    <KEY> <status-or-id> [--json]
  assign     set assignee     <KEY> <email|-> [--json]
  fields     custom-field usage report  [--sample N] [--json] [--all] [--project KEY]
  team       share team settings/views  export [--out] [--with-members]
                                        import <FILE|-> [--dry-run] [--overwrite]

Profiles keep separate credentials and mirrors (e.g. work and demo):
  scry --profile demo init && scry --profile demo serve --addr 127.0.0.1:7778
`

func main() {
	log.SetFlags(0)
	server.Version = version
	args := os.Args[1:]
	// The global --profile is only accepted before the subcommand.
	if len(args) >= 2 && (args[0] == "--profile" || args[0] == "-p") {
		config.SetProfile(args[1])
		args = args[2:]
	}
	if len(args) < 1 {
		fmt.Print(usage)
		os.Exit(2)
	}
	// scry help [<cmd>] — bare help is top-level usage; with a name, same as
	// `<cmd> --help`. Aliases --help/-h alone also print top-level usage.
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if len(args) == 1 {
			fmt.Print(usage)
			return
		}
		args = []string{args[1], "--help"}
	}
	if args[0] == "--version" || args[0] == "-v" {
		args = []string{"version"}
	}
	var err error
	switch args[0] {
	case "serve":
		err = cmdServe(args[1:])
	case "init":
		err = cmdInit(args[1:])
	case "sync":
		err = cmdSync(args[1:])
	case "sql":
		err = cmdSQL(args[1:])
	case "issue":
		err = cmdIssue(args[1:])
	case "open":
		err = cmdOpen(args[1:])
	case "search":
		err = cmdSearch(args[1:])
	case "comment":
		err = cmdComment(args[1:])
	case "transition":
		err = cmdTransition(args[1:])
	case "assign":
		err = cmdAssign(args[1:])
	case "fields":
		err = cmdFields(args[1:])
	case "status":
		err = cmdStatus(args[1:])
	case "mcp":
		err = cmdMCP(args[1:])
	case "demo":
		err = cmdDemo(args[1:])
	case "export-static":
		err = cmdExportStatic(args[1:])
	case "tui":
		err = cmdTUI(args[1:])
	case "profiles":
		err = cmdProfiles(args[1:])
	case "install-service":
		err = cmdInstallService(args[1:])
	case "version":
		err = cmdVersion(args[1:])
	case "snapshot":
		err = cmdSnapshot(args[1:])
	case "team":
		err = cmdTeam(args[1:])
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("scry: %v", err)
	}
}
