// Command scry serves a local mirror of your issue tracker.
//
// Implemented: serve (static UI + runtime config + health), version.
// Specified but not implemented: init, sync, demo, snapshot, sql, mcp.
// See specs/000-product/tasks.md for the current state of each.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "0.0.0-dev"

// Config is the on-disk configuration, also the source of the document the web
// UI fetches at boot. Credentials live here and are never echoed to the client.
type Config struct {
	Site     string          `json:"site"`             // https://your-site.atlassian.net
	Email    string          `json:"email"`            // Jira account email
	Token    string          `json:"token"`            // API token, never served
	Projects []string        `json:"projects"`         // project keys to mirror
	QADash   string          `json:"qa_dashboard_url"` // optional external link base
	Features map[string]bool `json:"features"`         // optional surface toggles
}

// clientConfig is the subset served to the browser as config.json.
type clientConfig struct {
	APIBase        string            `json:"apiBase"`
	AuthBase       string            `json:"authBase"`
	JiraBaseURL    string            `json:"jiraBaseUrl"`
	QADashboardURL string            `json:"qaDashboardUrl"`
	Projects       []string          `json:"projects"`
	GroupLabels    map[string]string `json:"groupLabels"`
	GroupColors    map[string]string `json:"groupColors"`
	ProductByGroup map[string]any    `json:"productByGroup"`
	Features       map[string]bool   `json:"features"`
}

func configDir() string {
	if dir := os.Getenv("SCRY_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".scry"
	}
	return filepath.Join(home, ".scry")
}

// loadConfig reads the config file. A missing file is not an error: serve still
// works and the UI reports that nothing is configured yet.
func loadConfig() (*Config, error) {
	path := filepath.Join(configDir(), "config.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// toClient strips secrets and fills the defaults the UI expects. Every optional
// feature stays off unless the config turns it on.
func (c *Config) toClient() clientConfig {
	features := map[string]bool{
		"presence": false, "feed": false, "push": false,
		"deploy": false, "qa": false, "teamGroups": false,
	}
	for k, v := range c.Features {
		if _, known := features[k]; known {
			features[k] = v
		}
	}
	projects := c.Projects
	if projects == nil {
		projects = []string{}
	}
	return clientConfig{
		APIBase:        "/api/v1/issues/",
		AuthBase:       "/api/v1/auth/",
		JiraBaseURL:    strings.TrimRight(c.Site, "/"),
		QADashboardURL: c.QADash,
		Projects:       projects,
		GroupLabels:    map[string]string{},
		GroupColors:    map[string]string{},
		ProductByGroup: map[string]any{},
		Features:       features,
	}
}

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

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7777", "listen address")
	static := fs.String("static", "dist/app", "directory holding the built web UI")
	allowRemote := fs.Bool("allow-remote", false,
		"permit binding a non-loopback address (the mirror has no auth; do not expose it)")
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

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": version})
	})
	mux.HandleFunc("/config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(cfg.toClient())
	})
	// The read and write API lands here; until then the UI's optional surfaces
	// stay off and its data calls fail cleanly rather than hanging.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not_implemented"}`, http.StatusNotFound)
	})

	if _, err := os.Stat(*static); err != nil {
		log.Printf("warning: static dir %q not found — run `npm run build` first", *static)
	}
	mux.Handle("/", spaHandler(*static))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("scry %s listening on http://%s", version, *addr)
	if len(cfg.Projects) == 0 {
		log.Printf("no projects configured — run `scry init` (see specs/000-product/tasks.md)")
	}
	return srv.ListenAndServe()
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

const usage = `scry — a local-first mirror of your issue tracker

Usage:
  scry serve [--addr 127.0.0.1:7777] [--static dist/app]
  scry version

Not implemented yet (specified in specs/000-product/):
  scry init | sync | demo | snapshot | sql | mcp
`

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		if err := serve(os.Args[2:]); err != nil {
			log.Fatalf("scry: %v", err)
		}
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	case "init", "sync", "demo", "snapshot", "sql", "mcp":
		log.Fatalf("scry %s: not implemented yet — see specs/000-product/tasks.md", os.Args[1])
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
}
