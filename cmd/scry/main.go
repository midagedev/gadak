// Command scry serves a local mirror of your issue tracker.
//
// Implemented: init, sync (--full/--watch), serve (syncs by default), tui,
// issue, search, comment, transition, assign, sql, status, mcp, demo,
// install-service, profiles, version.
// Specified but not implemented: snapshot.
// See specs/000-product/tasks.md for the current state of each.
//
// The agent-facing commands live in agent.go; AGENTS.md is their reference.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
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
	"github.com/midagedev/scry/internal/server"
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
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
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

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
	})
	// PUT settings/ 가 디스크의 config 를 갱신하므로 요청마다 다시 읽는다.
	// 파일 하나 읽기라 비용은 없고, 재시작 없이 설정 변경이 반영된다.
	mux.HandleFunc("/config.json", func(w http.ResponseWriter, r *http.Request) {
		cur, err := config.Load()
		if err != nil {
			http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
			return
		}
		doc, err := server.WebConfig(cur)
		if err != nil {
			http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(doc)
	})
	if *importAttachments != "" {
		if err := importAttachmentDir(*importAttachments); err != nil {
			log.Printf("warning: could not import attachments from %q: %v", *importAttachments, err)
		}
	}

	// Attachment bytes live on disk next to the mirror: the first view fetches
	// them from Jira, every later one is local, and a cached image still renders
	// with no credential at all.
	var apiHandler http.Handler
	if dir, err := config.AttachmentDir(); err != nil {
		log.Printf("warning: attachment cache disabled: %v", err)
		apiHandler = server.New(db, cfg)
	} else if cache, err := attachcache.New(dir, int64(cfg.AttachmentCacheMB)<<20); err != nil {
		log.Printf("warning: attachment cache disabled: %v", err)
		apiHandler = server.New(db, cfg)
	} else {
		apiHandler = server.NewWithCache(db, cfg, cache)
	}
	mux.Handle("/api/", apiHandler)

	// The embedded UI is the default; --static overrides it for development
	// (`npm run build` output) without rebuilding the binary.
	if *static != "" {
		if _, err := os.Stat(*static); err != nil {
			log.Printf("warning: static dir %q not found — run `npm run build` first", *static)
		}
		mux.Handle("/", spaHandler(*static))
	} else {
		ui, ok := scry.WebUI()
		if !ok {
			log.Printf("warning: no web UI embedded in this binary — build with `npm run build` before `go build`, or pass --static")
		}
		mux.Handle("/", spaHandlerFS(ui))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Default: keep the mirror fresh whenever a credential exists. --no-sync
	// opts out (fixtures with a fake token must pass it). --sync remains a
	// silent alias when the loop would start anyway; with no credential it
	// still prints the old guidance line.
	startSync := !*noSync && cfg.HasCredential() && len(cfg.Projects) > 0
	if *withSync && !startSync && !*noSync {
		log.Printf("--sync ignored: run `scry init` first")
	}
	if startSync {
		go func() {
			if err := syncer.Watch(ctx, cfg, db, syncer.Options{Log: func(s string) { log.Print(s) }}); err != nil && ctx.Err() == nil {
				log.Printf("sync loop stopped: %v", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shctx)
	}()
	log.Printf("scry %s listening on http://%s", version, *addr)
	if p := config.Profile(); p != "" {
		log.Printf("profile: %s", p)
	}
	if len(cfg.Projects) == 0 {
		log.Printf("no projects configured — run `scry init`")
	}
	if !*noOpen {
		go openOnceUp(browseAddr(*addr))
	}
	err = srv.ListenAndServe()
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

// cmdInit prompts for the connection settings, verifies them against Jira, and
// writes ~/.scry/config.json (0600). Existing values are kept on empty input.
func cmdInit(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	in := bufio.NewReader(os.Stdin)
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
	cfg.Site = strings.TrimRight(prompt("Jira site URL (https://your-site.atlassian.net)", cfg.Site), "/")
	cfg.Email = prompt("Account email", cfg.Email)
	hint := ""
	if cfg.Token != "" {
		hint = "configured; enter to keep"
	}
	cfg.Token = prompt("API token (id.atlassian.com/manage-profile/security/api-tokens)"+
		func() string {
			if hint != "" {
				return " [" + hint + "]"
			}
			return ""
		}(), cfg.Token)
	projects := prompt("Project keys, comma-separated", strings.Join(cfg.Projects, ","))
	cfg.Projects = nil
	for _, p := range strings.Split(projects, ",") {
		if p = strings.TrimSpace(strings.ToUpper(p)); p != "" {
			cfg.Projects = append(cfg.Projects, p)
		}
	}

	if !cfg.HasCredential() {
		return fmt.Errorf("site, email, and token are all required")
	}
	name, err := verifyCredential(cfg)
	if err != nil {
		return fmt.Errorf("credential check failed: %w", err)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	p, _ := config.Path()
	fmt.Printf("verified as %s — saved %s\n", name, p)
	fmt.Println("next: scry sync")
	return nil
}

// verifyCredential calls /rest/api/3/myself directly; the sync client is not
// needed for a single authenticated GET.
func verifyCredential(cfg *config.Config) (string, error) {
	req, err := http.NewRequest("GET", cfg.Site+"/rest/api/3/myself", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic "+
		base64.StdEncoding.EncodeToString([]byte(cfg.Email+":"+cfg.Token)))
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from /myself (org API keys do not work; use a user token)", resp.StatusCode)
	}
	var me struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return "", err
	}
	return me.DisplayName, nil
}

func cmdSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	full := fs.Bool("full", false, "force a full sync")
	watch := fs.Bool("watch", false, "keep syncing on an interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() || len(cfg.Projects) == 0 {
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
	return nil
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
		return fmt.Errorf("usage: scry sql [--json|--csv] \"select ...\"")
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

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()

	st := map[string]any{"profile": config.Profile()}
	row := db.QueryRow(`SELECT watermark, version, COALESCE(last_full_sync_at,''),
		COALESCE(last_error,''), schema_version,
		COALESCE(first_sync_at,''), COALESCE(sync_count, 0)
		FROM sync_state WHERE source_id = 'jira'`)
	var watermark, lastFull, lastErr, firstSync string
	var ver, schema int
	var syncCount int64
	if err := row.Scan(&watermark, &ver, &lastFull, &lastErr, &schema, &firstSync, &syncCount); err == nil {
		st["watermark"], st["version"], st["last_full_sync_at"], st["schema_version"] = watermark, ver, lastFull, schema
		st["sync_count"] = syncCount
		if firstSync != "" {
			st["first_sync_at"] = firstSync
		}
		if lastErr != "" {
			st["last_error"] = lastErr
		}
	}
	for name, q := range map[string]string{
		"issues":   "SELECT COUNT(*) FROM issues",
		"comments": "SELECT COUNT(*) FROM comments",
	} {
		var n int
		if err := db.QueryRow(q).Scan(&n); err == nil {
			st[name] = n
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
	return nil
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
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
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
// a blank or wildcard host becomes localhost.
func browseAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
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

func cmdProfiles() error {
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

const usage = `scry — a local-first mirror of your issue tracker

Usage:
  scry [--profile <name>] <command>

Commands:
  init             configure site, credentials, and projects (interactive)
  sync             mirror Jira into SQLite   [--full] [--watch]
  serve            web UI + API on loopback  [--addr] [--static] [--no-sync] [--no-open] [--allow-remote]
                   (syncs by default when a credential is configured; --no-sync opts out)
  install-service  keep serve running across reboots (launchd / systemd user)
                   [--uninstall]
  status           sync state and row counts [--json]
  demo             serve the bundled snapshot, no Jira account needed
  tui              terminal issue navigator (local mirror)
  profiles         list configured profiles
  version          print version

Reading the mirror (no network; see AGENTS.md):
  issue      full detail for one issue    <KEY> [--json]
  open       open the issue on your Jira site in the browser  <KEY>
  search     full-text search            [--limit N] [--json] "text"
  sql        read-only SQL               [--json|--csv] "select ..."
  mcp        MCP server on stdio (for clients without a shell; see docs/MCP.md)

Writing through to Jira (needs a credential):
  comment    add a comment    <KEY> -m <text|-> [--json]
  transition change status    <KEY> <status-or-id> [--json]
  assign     set assignee     <KEY> <email|-> [--json]

Profiles keep separate credentials and mirrors (e.g. work and demo):
  scry --profile demo init && scry --profile demo serve --addr 127.0.0.1:7778

Not implemented yet (specified in specs/000-product/):
  scry snapshot
`

func main() {
	log.SetFlags(0)
	server.Version = version
	args := os.Args[1:]
	// 글로벌 --profile 은 서브커맨드 앞에서만 받는다.
	if len(args) >= 2 && (args[0] == "--profile" || args[0] == "-p") {
		config.SetProfile(args[1])
		args = args[2:]
	}
	if len(args) < 1 {
		fmt.Print(usage)
		os.Exit(2)
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
	case "status":
		err = cmdStatus(args[1:])
	case "mcp":
		err = cmdMCP(args[1:])
	case "demo":
		err = cmdDemo(args[1:])
	case "tui":
		err = cmdTUI(args[1:])
	case "profiles":
		err = cmdProfiles()
	case "install-service":
		err = cmdInstallService(args[1:])
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	case "snapshot":
		log.Fatalf("scry %s: not implemented yet — see specs/000-product/tasks.md", args[0])
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("scry: %v", err)
	}
}
