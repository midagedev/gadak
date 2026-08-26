// Package apprun is the single owner of the long-lived process boot
// sequence shared by `gadak serve` and gadak-desktop.
//
// This is a move, not a redesign: each step is the same work the two
// mains already did. Caller-specific surfaces stay with the caller
// (serve: addr bind, GuardBrowser, /healthz; desktop: wails window,
// deeplink, Origin-strip mux). Intentional order differences are
// options, not a merged sequence:
//
//   - GDK-658: desktop runs wails SingleInstance (application.New)
//     before AcquireStandalone / StartOriginPassthrough.
//
// Process-start workspace selection (GDK-644) is SelectWorkspace, called
// from each main before flags. Persist flush on the CLI process is still
// cmd/gadak main (every verb, os.Exit(1) on failure); desktop sets
// FlushOnClose so Runtime.Close does that flush.
package apprun

import (
	"log"

	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
	"github.com/midagedev/gadak/internal/workspace"
)

// testStep, if set, is called with a stage name as each boot step
// completes. Production is nil. Tests pin GDK-658 / GDK-468 order here
// instead of at each main.
var testStep func(string)

func note(stage string) {
	if testStep != nil {
		testStep(stage)
	}
}

// Options configures caller-specific boot differences. Zero value is the
// serve-shaped path: persist is taken during Open (after AfterConfig).
type Options struct {
	// Version stamps server.Version. Empty leaves the current value.
	Version string

	// Log receives watch-loop status lines. Default is log.Print. Never
	// pass secrets.
	Log func(string)

	// OpenStore, if set, replaces store.Open(config.DBPath()). serve
	// passes cmd/gadak openStore so rejectUnknownProfile stays there.
	OpenStore func() (*store.DB, error)

	// AfterConfig runs after config.Load and before the store opens (and
	// before persist acquire).
	AfterConfig func(*config.Config) error

	// DeferStandalone skips persist acquire during Open. Desktop sets this
	// so wails SingleInstance (application.New) can os.Exit a second
	// instance before persist is taken (GDK-658).
	DeferStandalone bool

	// FlushOnClose runs origin.Close from Runtime.Close. Desktop sets this
	// (GDK-342/348). serve leaves it false: cmd/gadak main flushes every
	// verb and os.Exit(1)s on failure.
	FlushOnClose bool
}

// Runtime is the opened app surface both mains hold. Callers own when to
// Close (LIFO with their own resources: signal ctx, HTTP listener, wails).
type Runtime struct {
	Cfg *config.Config
	DB  *store.DB
	API *server.Handler
	Reg *workspace.Registry

	log                func(string)
	flushOnClose       bool
	acquiredStandalone bool
	stopOrigin         func()
}

// SelectWorkspace is the process-start workspace pick (GDK-644). Both
// mains call this before flags (CLI) or early-exit probes (desktop).
// Importing internal/config no longer selects a workspace.
func SelectWorkspace() {
	config.ReloadWorkspaceFromEnv()
}

// Open loads config, opens the mirror, builds the API handler (attachment
// cache + server.New), and creates the workspace registry. Version stamp
// is first when Options.Version is set. Persist acquire for standalone
// runs here unless DeferStandalone is set.
func Open(opts Options) (*Runtime, error) {
	rt := &Runtime{
		log:          opts.Log,
		flushOnClose: opts.FlushOnClose,
	}
	if rt.log == nil {
		rt.log = func(s string) { log.Print(s) }
	}
	if err := rt.boot(opts); err != nil {
		_ = rt.Close()
		return nil, err
	}
	return rt, nil
}

func (rt *Runtime) boot(opts Options) error {
	if opts.Version != "" {
		server.Version = opts.Version
		note("version")
		rt.stage("apprun: version stamped")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rt.Cfg = cfg
	note("config")
	rt.stage("apprun: config loaded")

	if opts.AfterConfig != nil {
		if err := opts.AfterConfig(cfg); err != nil {
			return err
		}
		note("after-config")
	}

	if cfg.IsStandalone() && !opts.DeferStandalone {
		if err := rt.acquireStandalone(); err != nil {
			return err
		}
		note("standalone-persist")
		rt.stage("apprun: standalone persist acquired")
	}

	db, err := openDB(opts)
	if err != nil {
		return err
	}
	rt.DB = db
	note("store")
	rt.stage("apprun: store opened")

	rt.API = newHandler(db, cfg)
	note("handler")
	rt.stage("apprun: handler ready")

	rt.Reg = workspace.New()
	note("registry")
	return nil
}

func openDB(opts Options) (*store.DB, error) {
	if opts.OpenStore != nil {
		return opts.OpenStore()
	}
	path, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}

// newHandler is the attach-cache + server.New seam both mains copied.
// A cache failure disables the cache and still returns a handler — same
// warning string as cmd/gadak serve and desktop.
func newHandler(db *store.DB, cfg *config.Config) *server.Handler {
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

func (rt *Runtime) stage(msg string) {
	if rt != nil && rt.log != nil {
		rt.log(msg)
		return
	}
	log.Print(msg)
}

// Close stops any origin cleanup this Runtime started, flushes standalone
// persist when FlushOnClose is set or persist was acquired here, then
// closes the registry, API handler, and store — API before DB (GDK-270).
// Safe on a partial Open.
func (rt *Runtime) Close() error {
	if rt == nil {
		return nil
	}
	if rt.stopOrigin != nil {
		rt.stopOrigin()
		rt.stopOrigin = nil
	}
	var first error
	if rt.acquiredStandalone || rt.flushOnClose {
		if err := origin.Close(); err != nil {
			if rt.flushOnClose {
				log.Printf("warning: standalone persist flush on exit: %v", err)
			}
			if first == nil {
				first = err
			}
		}
		if rt.acquiredStandalone {
			origin.SetInProcess(rt.Cfg, false)
			rt.acquiredStandalone = false
		}
	}
	if rt.Reg != nil {
		rt.Reg.Close()
		rt.Reg = nil
	}
	if rt.API != nil {
		if err := rt.API.Close(); err != nil && first == nil {
			first = err
		}
		rt.API = nil
	}
	if rt.DB != nil {
		if err := rt.DB.Close(); err != nil && first == nil {
			first = err
		}
		rt.DB = nil
	}
	return first
}

func profileDir(cfg *config.Config) string {
	if cfg != nil {
		if d := cfg.Directory(); d != "" {
			return d
		}
	}
	d, err := config.Dir()
	if err != nil || d == "" {
		return ""
	}
	return d
}

func nopStop() {}
