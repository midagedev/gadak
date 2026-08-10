// Command scry-desktop wraps the scry web UI in a native window. There is no
// TCP listener: the Wails asset server calls straight into the same
// server.Handler that `scry serve` mounts, so ports, addresses, and the
// browser guard's threat model (other pages reaching a loopback port) do not
// apply — the webview is the only client that can reach this handler.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

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

	// window and openURL are filled in below; the single-instance callback and
	// the /desktop/open route read them once the app exists.
	var window *application.WebviewWindow
	var openURL func(string) error

	app := application.New(application.Options{
		// Name labels the macOS app menu; Name + Description are what the
		// About panel shows.
		Name:        "Scry",
		Description: "Jira and Confluence, mirrored to your disk.",
		Mac: application.MacOptions{
			// v2 quit when the only window closed; keep that.
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Assets: application.AssetOptions{
			Handler: assetHandler(ui, fallbackHandler(api, ui, reg, func(u string) error {
				if openURL == nil {
					return fmt.Errorf("browser not ready")
				}
				return openURL(u)
			})),
		},
		SingleInstance: &application.SingleInstanceOptions{
			// Per profile, not global: one window per mirror, and a second
			// profile (SCRY_PROFILE=work open -a Scry) gets its own window.
			UniqueID: "com.midagedev.scry." + profileLockKey(),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if window != nil {
					window.Restore()
					window.Focus()
				}
			},
		},
		OnShutdown: cancel,
	})

	openURL = app.Browser.OpenURL

	// Self-update. nil on dev builds and on any configuration failure — see
	// updater.go; the app runs identically either way, minus the menu item.
	up := initUpdater(app, appVersion)

	// Without an Edit menu, macOS does not wire ⌘C/V/X/A into the webview —
	// paste during onboarding would fail. The app menu supplies About/Quit.
	// Built after application.New: the app menu takes its label from Name.
	appMenu := app.NewMenu()
	appMenu.AddRole(application.AppMenu)
	appMenu.AddRole(application.EditMenu)
	// Tools → Install Command Line Tool… (macOS only; no-op stub elsewhere).
	if runtime.GOOS == "darwin" {
		appendInstallCLIMenu(appMenu)
	}
	// Tools → Check for Updates… goes above the CLI item, so it is added after
	// the submenu exists and prepended into it.
	if up != nil {
		appendCheckForUpdatesMenu(appMenu, up)
	}
	app.Menu.Set(appMenu)

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Scry",
		Width:     1280,
		Height:    820,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
		Mac: application.MacWindow{
			// No native title bar: it spent 28px to repeat a word the sidebar
			// already shows. The window controls stay (they move into the
			// sidebar's first row, which reserves their width and is the drag
			// handle — see .desktop-titlebar-row in web/src/app.css).
			TitleBar: application.MacTitleBarHiddenInset,
		},
	})

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if dir, err := config.Dir(); err == nil {
			api.StartUpdateCheck(ctx, dir)
		}
		// Same opt-out as the sidebar banner: updateCheck: false silences both.
		if up != nil && cfg.UpdateCheckEnabled() {
			go checkForUpdatesQuietly(ctx, up)
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
		}
		// Workspace loops start immediately — those profiles already carry
		// their own credentials (primary may still be waiting on onboarding).
		watched := reg.WatchAll(ctx, config.Profile(), func(s string) { log.Print(s) })
		if len(watched) > 0 {
			log.Printf("syncing %d workspace mirrors: %s", len(watched), strings.Join(watched, ", "))
		}
	})

	return app.Run()
}

// profileLockKey names the single-instance lock for the active profile.
// config.Profile() returns "" for the default profile.
func profileLockKey() string {
	if p := config.Profile(); p != "" {
		return p
	}
	return "default"
}

// assetHandler serves the embedded web UI and hands everything it does not
// hold to next. v2's asset server took the file system and the fallback as two
// separate options and did this split itself; v3 takes one handler, so the
// split lives here — same rule: GET a file that exists, otherwise fall back.
func assetHandler(ui fs.FS, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ui == nil || r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		name := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		file, err := ui.Open(name)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.IsDir() {
			// Directories (including the root) are client-side routes as far
			// as this app is concerned: next serves index.html for them.
			next.ServeHTTP(w, r)
			return
		}
		// ServeContent, not ServeFileFS: the latter redirects /index.html to
		// ./, and the webview should not be sent round a redirect.
		if seeker, canSeek := file.(io.ReadSeeker); canSeek {
			http.ServeContent(w, r, info.Name(), info.ModTime(), seeker)
			return
		}
		if ctype := mime.TypeByExtension(path.Ext(info.Name())); ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		_, _ = io.Copy(w, file)
	})
}

// fallbackHandler serves whatever the Wails asset server does not find as a
// static file: the API, /config.json, workspace mounts, the desktop-only
// open-in-browser route, and the SPA's index.html for client-side routes.
// openURL opens a link in the system browser; nil disables the route (503) —
// it is bound to app.Browser.OpenURL after the application exists.
func fallbackHandler(api http.Handler, ui fs.FS, reg *workspace.Registry, openURL func(string) error) http.Handler {
	mux := http.NewServeMux()
	// The v3 webview has no new-window delegate, so target="_blank" clicks die
	// inside it. The web bundle (in desktop mode only) routes external links
	// here instead; only the webview can reach this — there is no TCP listener.
	mux.HandleFunc("POST /desktop/open", func(w http.ResponseWriter, r *http.Request) {
		if openURL == nil {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
			return
		}
		// http(s) only: the mirror's URLs are web URLs, and nothing else
		// (file:, javascript:, custom schemes) has any business here.
		u, err := url.Parse(body.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			http.Error(w, `{"error":"bad_url"}`, http.StatusBadRequest)
			return
		}
		if err := openURL(u.String()); err != nil {
			log.Printf("open in browser: %v", err)
			http.Error(w, `{"error":"open_failed"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
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
