// Command scry-desktop wraps the scry web UI in a native window. There is no
// TCP listener: the Wails asset server calls straight into the same
// server.Handler that `scry serve` mounts, so ports, addresses, and the
// browser guard's threat model (other pages reaching a loopback port) do not
// apply — the webview is the only client that can reach this handler.
package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	scry "github.com/midagedev/scry"
	"github.com/midagedev/scry/internal/attachcache"
	"github.com/midagedev/scry/internal/config"
	"github.com/midagedev/scry/internal/server"
	"github.com/midagedev/scry/internal/store"
	syncer "github.com/midagedev/scry/internal/sync"
	"github.com/midagedev/scry/internal/workspace"
)

// appVersion is the desktop binary version string (overridable at link time by
// a future build script; default keeps local builds identifiable).
var appVersion = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dbPath, err := config.DBPath()
	if err != nil {
		return err
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

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

	ui, ok := scry.WebUI()
	if !ok {
		log.Printf("warning: no web UI embedded — run `npm run build` at the repo root before building")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := workspace.New()
	defer reg.Close()

	// Without an Edit menu, macOS does not wire ⌘C/V/X/A into the webview —
	// paste during onboarding would fail. AppMenu supplies About/Quit.
	// wailsCtx is set in OnStartup so menu handlers can open native dialogs.
	var wailsCtx context.Context
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	appMenu.Append(menu.EditMenu())
	// Tools → Install Command Line Tool… (macOS only; no-op stub elsewhere).
	if runtime.GOOS == "darwin" {
		appendInstallCLIMenu(appMenu, &wailsCtx)
	}

	app := &options.App{
		Title:     "Scry",
		Width:     1280,
		Height:    820,
		MinWidth:  720,
		MinHeight: 480,
		Menu:      appMenu,
		AssetServer: &assetserver.Options{
			Assets:  ui,
			Handler: fallbackHandler(api, ui, reg),
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			// Per profile, not global: one window per mirror, and a second
			// profile (SCRY_PROFILE=work open -a Scry) gets its own window.
			UniqueId: "com.midagedev.scry." + profileLockKey(),
		},
		OnStartup: func(startupCtx context.Context) {
			wailsCtx = startupCtx
			if dir, err := config.Dir(); err == nil {
				api.StartUpdateCheck(ctx, dir)
			}
			// Same delayed-start seam as cmd/scry serve: with a credential the
			// watch loop starts now; without one, in-app onboarding fires it.
			startWatch := func() {
				go func() {
					// Reload so onboarding's save is what the loop runs with.
					cur, err := config.Load()
					if err != nil {
						log.Printf("sync loop: load config: %v", err)
						return
					}
					if err := syncer.Watch(ctx, cur, db, syncer.Options{
						Log:    func(s string) { log.Print(s) },
						Reload: config.Load,
					}); err != nil && ctx.Err() == nil {
						log.Printf("sync loop stopped: %v", err)
					}
				}()
			}
			if cfg.HasCredential() {
				startWatch()
			} else {
				api.SetSyncStarter(startWatch)
			}
			// Workspace loops start immediately — those profiles already carry
			// their own credentials (primary may still be waiting on onboarding).
			watched := reg.WatchAll(ctx, config.Profile(), func(s string) { log.Print(s) })
			if len(watched) > 0 {
				log.Printf("syncing %d workspace mirrors: %s", len(watched), strings.Join(watched, ", "))
			}
		},
		OnShutdown: func(context.Context) { cancel() },
		Mac: &mac.Options{
			// No native title bar: it spent 28px to repeat a word the sidebar
			// already shows. The window controls stay (they move into the
			// sidebar's first row, which reserves their width and is the drag
			// handle — see .desktop-titlebar-row in web/src/app.css).
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "Scry",
				Message: "Jira and Confluence, mirrored to your disk.",
			},
		},
	}
	return wails.Run(app)
}

// profileLockKey names the single-instance lock for the active profile.
// config.Profile() returns "" for the default profile.
func profileLockKey() string {
	if p := config.Profile(); p != "" {
		return p
	}
	return "default"
}

// fallbackHandler serves whatever the Wails asset server does not find as a
// static file: the API, /config.json, workspace mounts, and the SPA's
// index.html for client-side routes.
func fallbackHandler(api http.Handler, ui fs.FS, reg *workspace.Registry) http.Handler {
	mux := http.NewServeMux()
	// PUT settings/ rewrites the config on disk, so re-read it per request
	// (mirrors cmd/scry buildServeMux).
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
		doc, err = withDesktopFlag(doc)
		if err != nil {
			http.Error(w, `{"error":"config_unreadable"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(doc)
	})
	mux.HandleFunc("GET /api/v1/workspaces", workspace.ListHandler())
	mux.HandleFunc("GET /api/v1/workspaces/{$}", workspace.ListHandler())
	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The webview sends wails://-scheme Origin values the browser guard
		// would reject. These requests never crossed a network boundary —
		// present them as the guard's happy path instead of widening the
		// guard itself.
		r.Header.Del("Origin")
		r.Host = "127.0.0.1"
		api.ServeHTTP(w, r)
	}))
	spa := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, ui, "index.html")
	})
	if reg != nil {
		ws := reg.Handler(spa, appVersion)
		mux.HandleFunc("/w/", func(w http.ResponseWriter, r *http.Request) {
			// Same webview identity normalization as /api/ so workspace API
			// routes pass the browser guard.
			r.Header.Del("Origin")
			r.Host = "127.0.0.1"
			ws(w, r)
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: unknown paths are client-side routes.
		http.ServeFileFS(w, r, ui, "index.html")
	})
	return mux
}

// withDesktopFlag marks the config document the app serves. One web bundle is
// shared with `scry serve`, and only here is the native title bar hidden — the
// UI has to reserve the window controls' corner, and a browser tab must not.
func withDesktopFlag(doc []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		return nil, err
	}
	m["desktop"] = true
	return json.Marshal(m)
}
