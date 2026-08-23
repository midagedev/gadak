package main

import (
	"context"
	"errors"
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

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/workspace"
)

// serveOpts is the parsed serve CLI surface. cmdServe only parses flags and
// calls stage helpers; nothing else lives on this type.
type serveOpts struct {
	addr              string
	static            string
	allowRemote       bool
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
	// Sync starts by default when HasCredential is true (standalone, or a
	// connected workspace with site+email+token). --no-sync opts out
	// (demo / e2e fixtures).
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
	// Empty host, 0.0.0.0, and :: all mean "every interface" (GDK-542).
	// isLoopback is false for each of those, so dropping the host!=""
	// exception is the whole fix.
	if !allowRemote && !isLoopback(host) {
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
	ui, ok := gadak.WebUI()
	if !ok {
		log.Printf("warning: no web UI embedded in this binary — build with `npm run build` before `go build`, or pass --static")
	}
	return spaHandlerFS(ui)
}

// startServeLoops starts the optional update check and the incremental sync
// loops (primary profile + workspace mounts). Order and log strings match the
// previous inlined cmdServe body.
func startServeLoops(ctx context.Context, api *server.Handler, db *store.DB, cfg *config.Config, reg *workspace.Registry, noSync bool) {
	// Optional once-a-day GitHub release check (opt-out via updateCheck: false).
	// Independent of Jira credentials; silent on failure.
	if dir, err := config.Dir(); err == nil {
		api.StartUpdateCheck(ctx, dir)
	}

	// Default: keep the mirror fresh whenever HasCredential is true
	// (standalone origin, or connected with site+email+token). Empty
	// projects on a connected workspace means "everything this account can
	// see"; standalone copy is serveScopeLog (GDK-464). --no-sync opts out
	// (fixtures with a fake token must pass it). When serve starts without a
	// credential, register the same starter so PUT onboarding/connect/ can
	// kick off Watch once after the first successful save. Mounted workspace
	// profiles with credentials each get their own Watch loop (WatchAll);
	// --no-sync disables those too.
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
			syncer.WatchLoop(ctx, cur, db, syncer.Options{
				Log:      func(s string) { log.Print(s) },
				Reload:   config.Load,
				Phase:    phase,
				Progress: progress,
			})
		}()
	}
	if cfg.HasCredential() {
		startWatch()
	} else {
		api.SetSyncStarter(startWatch)
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
	// Bind before serving so EADDRINUSE can be handled: same-profile gadak →
	// hand off (exit 0); other occupant + default addr → next free port;
	// explicit --addr → hard error (with gadak identity when known).
	ln, bound, existingURL, occupant, err := bindListenDetail(preferred, addrPinned, config.Profile(), nil, nil)
	if err != nil {
		return err
	}
	if existingURL != "" {
		return handoffExistingServe(existingURL, noOpen)
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
	// Advertise the final listen address (after port fallback) so other
	// processes route origin writes here. Connected workspaces skip.
	unpublish, err := publishStandaloneOrigin(cfg, bound)
	if err != nil {
		return err
	}
	defer unpublish()

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
	log.Printf("gadak %s listening on %s", version, openURL)
	for _, line := range serveStartHints(cfg) {
		log.Printf("%s", line)
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

// liveServeURL is the single owner of "this profile already has a live
// serve" (GDK-468). It uses origin.AdvertisedAddr (advertise file + the
// same probe bindListen trusts), so a serve that fell back to another
// port is still found. Empty means no live same-profile serve.
func liveServeURL(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	addr := origin.AdvertisedAddr(cfg)
	if addr == "" {
		return ""
	}
	return browseAddr(addr)
}

// handoffExistingServe prints the existing open-existing line and
// optionally opens a browser, then returns nil (exit 0).
func handoffExistingServe(existingURL string, noOpen bool) error {
	log.Printf("already serving at %s (same profile)", existingURL)
	if !noOpen {
		if openErr := openBrowser(existingURL); openErr != nil {
			log.Printf("could not open a browser: %v — visit %s", openErr, existingURL)
		}
	}
	return nil
}

// serveStartHints is the extra listen-time lines after "listening on".
// Unconfigured workspaces reuse config.ErrNotConfigured (GDK-468); do
// not invent a second init sentence.
func serveStartHints(cfg *config.Config) []string {
	var lines []string
	if p := config.Profile(); p != "" {
		lines = append(lines, "profile: "+p)
	}
	if line := serveScopeLog(cfg); line != "" {
		lines = append(lines, line)
	}
	if cfg == nil || !cfg.HasCredential() {
		lines = append(lines, config.ErrNotConfigured.Error())
	}
	return lines
}

// serveScopeLog is the listen-time project-scope line. Empty means print
// nothing. GDK-464: standalone has no account — name the seeded project
// instead of "this account can see". cfg.IsStandalone() is the only branch.
func serveScopeLog(cfg *config.Config) string {
	if cfg == nil || !cfg.HasCredential() {
		return ""
	}
	if cfg.IsStandalone() {
		if len(cfg.Projects) != 0 {
			return ""
		}
		p := strings.TrimSpace(cfg.DefaultProject)
		if p == "" {
			p = origin.DefaultProjectKey
		}
		return "syncing " + p
	}
	if len(cfg.Projects) == 0 {
		return "no project filter — syncing everything this account can see"
	}
	return ""
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
	// Living-serve detection is the single owner of "this profile is
	// already up" (GDK-468). It must run before the persist lock: a
	// same-profile serve holds that lock, so lock-first turned a
	// re-serve into "persist is locked" instead of open-existing.
	if url := liveServeURL(cfg); url != "" {
		return handoffExistingServe(url, opts.noOpen)
	}
	// Persist owner is this process: origin.Client must embed, never
	// proxy to the advertise file we are about to write (self-loop).
	if cfg.IsStandalone() {
		origin.SetInProcess(true)
		defer origin.SetInProcess(false)
		// Take the persist lock now (GDK-343): a second owner must fail at
		// startup — before its advertise write could steal routing from the
		// live owner — not at the first write.
		if _, err := origin.StandaloneHandler(cfg); err != nil {
			if errors.Is(err, origin.ErrWorkspaceBusy) {
				if url := liveServeURL(cfg); url != "" {
					return handoffExistingServe(url, opts.noOpen)
				}
			}
			return err
		}
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	if opts.importAttachments != "" {
		dbPath, err := config.DBPath()
		if err != nil {
			log.Printf("warning: could not import attachments from %q: %v", opts.importAttachments, err)
		} else if err := importAttachmentDir(opts.importAttachments, cfg.Site, config.Profile(), dbPath); err != nil {
			log.Printf("warning: could not import attachments from %q: %v", opts.importAttachments, err)
		}
	}

	api := newServeAPI(db, cfg)
	// Registered after db.Close and before reg.Close, so the LIFO order is
	// workspaces, then this handler, then the mirror: a background sync must
	// stop before the database it writes to closes (GDK-270).
	defer api.Close()
	spa := serveSPAHandler(opts.static)

	// Workspace mounts share this process's listener; each profile opens lazily.
	// Update checks stay on the primary handler; workspace mirrors get their own
	// sync loops (see WatchAll in startServeLoops) when they have credentials.
	reg := workspace.New()
	defer reg.Close()
	mux := buildServeMux(api, spa, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startServeLoops(ctx, api, db, cfg, reg, opts.noSync)
	return runServeHTTP(ctx, mux, opts.addr, opts.addrPinned, opts.noOpen, cfg)
}

// publishStandaloneOrigin writes serve-origin.json for a standalone
// workspace and returns a cleanup that removes it. Connected workspaces
// and a missing profile dir are no-ops.
//
// A write failure is fatal (GDK-343 / F6): this process holds the persist
// lock, so without the advertise file every concurrent CLI write becomes a
// hard "workspace busy" error instead of routing here. Failing loud at
// startup beats a warning nobody reads.
func publishStandaloneOrigin(cfg *config.Config, bound string) (func(), error) {
	nop := func() {}
	if cfg == nil || !cfg.IsStandalone() || bound == "" {
		return nop, nil
	}
	dir := cfg.Directory()
	if dir == "" {
		var err error
		dir, err = config.Dir()
		if err != nil || dir == "" {
			return nop, nil
		}
	}
	if err := origin.WriteAdvertise(dir, bound); err != nil {
		return nop, fmt.Errorf("could not advertise origin owner: %w", err)
	}
	return func() { _ = origin.RemoveAdvertise(dir) }, nil
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
