package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	scry "github.com/midagedev/scry"
	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/store"
	syncer "github.com/midagedev/scry/internal/sync"
	"github.com/midagedev/scry/internal/workspace"
)

// serveOpts is the parsed serve CLI surface. cmdServe only parses flags and
// calls stage helpers; nothing else lives on this type.
type serveOpts struct {
	addr              string
	static            string
	allowRemote       bool
	withSync          bool
	noSync            bool
	importAttachments string
	noOpen            bool
	// addrPinned is true when the user passed --addr (no port fallback on conflict).
	addrPinned bool
}

func parseServeOpts(args []string) (serveOpts, error) {
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
		return serveOpts{}, err
	}
	// --addr pin: user forced a port → no fallback on conflict (rule 3).
	addrPinned := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrPinned = true
		}
	})
	return serveOpts{
		addr:              *addr,
		static:            *static,
		allowRemote:       *allowRemote,
		withSync:          *withSync,
		noSync:            *noSync,
		importAttachments: *importAttachments,
		noOpen:            *noOpen,
		addrPinned:        addrPinned,
	}, nil
}

// checkServeAddr rejects a non-loopback bind unless --allow-remote is set.
// The server has no authentication by design.
func checkServeAddr(addr string, allowRemote bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("bad --addr %q: %w", addr, err)
	}
	if !allowRemote && host != "" && !isLoopback(host) {
		return fmt.Errorf("refusing to bind non-loopback address %q without --allow-remote", host)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newServeAPI builds the primary API handler. Attachment bytes live on disk
// next to the mirror: the first view fetches them from Jira, every later one
// is local, and a cached image still renders with no credential at all.
func newServeAPI(db *store.DB, cfg *config.Config) *server.Handler {
	if dir, err := config.AttachmentDir(); err != nil {
		log.Printf("warning: attachment cache disabled: %v", err)
		return server.New(db, cfg)
	} else if cache, err := attachcache.New(dir, int64(cfg.AttachmentCacheMB)<<20); err != nil {
		log.Printf("warning: attachment cache disabled: %v", err)
		return server.New(db, cfg)
	} else {
		return server.NewWithCache(db, cfg, cache)
	}
}

// serveSPAHandler picks the embedded UI or a --static directory for development
// (`npm run build` output) without rebuilding the binary.
func serveSPAHandler(static string) http.Handler {
	if static != "" {
		if _, err := os.Stat(static); err != nil {
			log.Printf("warning: static dir %q not found — run `npm run build` first", static)
		}
		return spaHandler(static)
	}
	ui, ok := scry.WebUI()
	if !ok {
		log.Printf("warning: no web UI embedded in this binary — build with `npm run build` before `go build`, or pass --static")
	}
	return spaHandlerFS(ui)
}

// startServeLoops starts the optional update check and the incremental sync
// loops (primary profile + workspace mounts). Order and log strings match the
// previous inlined cmdServe body.
func startServeLoops(ctx context.Context, api *server.Handler, db *store.DB, cfg *config.Config, reg *workspace.Registry, noSync, withSync bool) {
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
	// the first successful save. Mounted workspace profiles with credentials
	// each get their own Watch loop (WatchAll); --no-sync disables those too.
	if noSync {
		return
	}
	startWatch := func() {
		go func() {
			// Reload so a late setup does not capture a stale empty config.
			cur, err := config.Load()
			if err != nil {
				log.Printf("sync loop: load config: %v", err)
				return
			}
			phase, progress := api.SyncActivityHooks()
			if err := syncer.Watch(ctx, cur, db, syncer.Options{
				Log:      func(s string) { log.Print(s) },
				Reload:   config.Load,
				Phase:    phase,
				Progress: progress,
			}); err != nil && ctx.Err() == nil {
				log.Printf("sync loop stopped: %v", err)
			}
		}()
	}
	if cfg.HasCredential() {
		startWatch()
	} else {
		api.SetSyncStarter(startWatch)
		if withSync {
			log.Printf("--sync ignored: run `scry init` first")
		}
	}
	watched := reg.WatchAll(ctx, config.Profile(), func(s string) { log.Print(s) })
	if len(watched) > 0 {
		log.Printf("syncing %d workspace mirrors: %s", len(watched), strings.Join(watched, ", "))
	}
}

// runServeHTTP binds the listener (with port-busy rules), serves mux until
// ctx is cancelled, and optionally opens a browser. Same-profile handoff
// returns nil without starting a server.
func runServeHTTP(ctx context.Context, mux http.Handler, preferred string, addrPinned, noOpen bool, cfg *config.Config) error {
	// Bind before serving so EADDRINUSE can be handled: same-profile scry →
	// hand off (exit 0); other occupant + default addr → next free port;
	// explicit --addr → hard error (with scry identity when known).
	ln, bound, existingURL, occupant, err := bindListenDetail(preferred, addrPinned, config.Profile(), nil, nil)
	if err != nil {
		return err
	}
	if existingURL != "" {
		log.Printf("already serving at %s (same profile)", existingURL)
		if !noOpen {
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
	if !noOpen {
		go openOnceUp(openURL)
	}
	err = srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func cmdServe(args []string) error {
	opts, err := parseServeOpts(args)
	if err != nil {
		return err
	}
	if err := checkServeAddr(opts.addr, opts.allowRemote); err != nil {
		return err
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

	if opts.importAttachments != "" {
		if err := importAttachmentDir(opts.importAttachments); err != nil {
			log.Printf("warning: could not import attachments from %q: %v", opts.importAttachments, err)
		}
	}

	api := newServeAPI(db, cfg)
	spa := serveSPAHandler(opts.static)

	// Workspace mounts share this process's listener; each profile opens lazily.
	// Update checks stay on the primary handler; workspace mirrors get their own
	// sync loops (see WatchAll in startServeLoops) when they have credentials.
	reg := workspace.New()
	defer reg.Close()
	mux := buildServeMux(api, spa, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startServeLoops(ctx, api, db, cfg, reg, opts.noSync, opts.withSync)
	return runServeHTTP(ctx, mux, opts.addr, opts.addrPinned, opts.noOpen, cfg)
}
