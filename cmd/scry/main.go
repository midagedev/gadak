// Command scry serves a local mirror of your issue tracker.
//
// Implemented: init, sync (--full/--watch), serve (--sync), sql, status,
// profiles, version. Specified but not implemented: demo, snapshot, mcp.
// See specs/000-product/tasks.md for the current state of each.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
	static := fs.String("static", "dist/app", "directory holding the built web UI")
	allowRemote := fs.Bool("allow-remote", false,
		"permit binding a non-loopback address (the mirror has no auth; do not expose it)")
	withSync := fs.Bool("sync", false, "run the incremental sync loop inside the server")
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
	mux.Handle("/api/", server.New(db, cfg))

	if _, err := os.Stat(*static); err != nil {
		log.Printf("warning: static dir %q not found — run `npm run build` first", *static)
	}
	mux.Handle("/", spaHandler(*static))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *withSync {
		if !cfg.HasCredential() || len(cfg.Projects) == 0 {
			log.Printf("--sync ignored: run `scry init` first")
		} else {
			go func() {
				if err := syncer.Watch(ctx, cfg, db, syncer.Options{Log: func(s string) { log.Print(s) }}); err != nil && ctx.Err() == nil {
					log.Printf("sync loop stopped: %v", err)
				}
			}()
		}
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
	fs := flag.NewFlagSet("sql", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emit one JSON object per row")
	if err := fs.Parse(args); err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		return fmt.Errorf("usage: scry sql [--json] \"select ...\"")
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
	if !*asJSON {
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
		if *asJSON {
			obj := make(map[string]any, len(cols))
			for i, c := range cols {
				obj[c] = cell(vals[i])
			}
			_ = enc.Encode(obj)
			continue
		}
		out := make([]string, len(cols))
		for i := range vals {
			out[i] = fmt.Sprint(cell(vals[i]))
		}
		fmt.Println(strings.Join(out, "\t"))
	}
	return rows.Err()
}

func cell(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
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
		COALESCE(last_error,''), schema_version FROM sync_state WHERE source_id = 'jira'`)
	var watermark, lastFull, lastErr string
	var ver, schema int
	if err := row.Scan(&watermark, &ver, &lastFull, &lastErr, &schema); err == nil {
		st["watermark"], st["version"], st["last_full_sync_at"], st["schema_version"] = watermark, ver, lastFull, schema
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
  init       configure site, credentials, and projects (interactive)
  sync       mirror Jira into SQLite   [--full] [--watch]
  serve      web UI + API on loopback  [--addr] [--static] [--sync] [--allow-remote]
  sql        read-only SQL against the mirror   [--json] "select ..."
  status     sync state and row counts [--json]
  profiles   list configured profiles
  version    print version

Profiles keep separate credentials and mirrors (e.g. work and demo):
  scry --profile demo init && scry --profile demo serve --sync --addr 127.0.0.1:7778

Not implemented yet (specified in specs/000-product/):
  scry demo | snapshot | mcp
`

func main() {
	log.SetFlags(0)
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
	case "status":
		err = cmdStatus(args[1:])
	case "profiles":
		err = cmdProfiles()
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	case "demo", "snapshot", "mcp":
		log.Fatalf("scry %s: not implemented yet — see specs/000-product/tasks.md", args[0])
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("scry: %v", err)
	}
}
