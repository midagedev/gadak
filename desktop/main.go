// Command gadak-desktop wraps the gadak web UI in a native window. There is no
// TCP listener: the Wails asset server calls straight into the same
// server.Handler that `gadak serve` mounts, so ports, addresses, and the
// browser guard's threat model (other pages reaching a loopback port) do not
// apply — the webview is the only client that can reach this handler.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/attachcache"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/workspace"
)

// appVersion is stamped by desktop/build-app.sh (-X main.appVersion). The
// default ("dev") keeps local builds identifiable. StartUpdateCheck compares
// server.Version, which cmd/gadak assigns from its own ldflags; without the
// same assignment here the sidebar banner stays silent (Version is 0.0.0-dev).
var appVersion = "dev"

func main() {
	// Same assignment cmd/gadak/main.go makes. "dev" is this file's default;
	// selfupdate.Check only skips the CLI default ("0.0.0-dev").
	if appVersion == "" || appVersion == "dev" {
		server.Version = "0.0.0-dev"
	} else {
		server.Version = strings.TrimPrefix(appVersion, "v")
	}
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

	ui, ok := gadak.WebUI()
	if !ok {
		log.Printf("warning: no web UI embedded — run `npm run build` at the repo root before building")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := workspace.New()
	defer reg.Close()

	// window and openURL are filled in below; the single-instance callback and
	// the /desktop/open route read them once the app exists. browse drives real
	// webviews only after bind, for the same reason.
	var window *application.WebviewWindow
	var openURL func(string) error
	browse := newBrowseTabs()

	// Both gadak:// delivery routes go through one function; see deeplink.go
	// for why they are two. Assigned after the window exists, for the same
	// reason openURL is — the single-instance callback can fire before then.
	var applyDeepLink func(string)

	app := application.New(application.Options{
		// Name labels the macOS app menu; Name + Description are what the
		// About panel shows.
		Name:        "Gadak",
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
			}, browse, func(text string) bool {
				return application.Get().Clipboard.SetText(text)
			})),
		},
		SingleInstance: &application.SingleInstanceOptions{
			// Per profile, not global: one window per mirror, and a second
			// profile (GADAK_PROFILE=work open -a Gadak) gets its own window.
			UniqueID: "com.midagedev.gadak." + profileLockKey(),
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
				if window == nil {
					return
				}
				raiseWindow(window)
				// A gadak:// launch arrives as a second instance: macOS starts
				// a process, wails captures the pending Apple Event there and
				// forwards the URL in argv. Raise first, then navigate — the
				// user asked for a view, and the window they get should be the
				// one showing it.
				if raw := firstDeepLinkArg(d.Args); raw != "" && applyDeepLink != nil {
					applyDeepLink(raw)
				}
			},
		},
		OnShutdown: cancel,
	})

	openURL = app.Browser.OpenURL
	browse.bind(newPlatformEmbedder(func() unsafe.Pointer {
		if window == nil {
			return nil
		}
		return window.NativeWindow()
	}))
	// With focus inside an embedded Jira page the SPA never sees keyDowns —
	// its Escape → hide-browse dies there (GDK-78). The native monitor hands
	// that one key back; ⌘W never needed it only because menus see keys first.
	installEscapeRelay()

	// Without an Edit menu, macOS does not wire ⌘C/V/X/A into the webview —
	// paste during onboarding would fail. The app menu supplies About/Quit.
	// Built after application.New: the app menu takes its label from Name.
	appMenu := app.NewMenu()
	appMenu.AddRole(application.AppMenu)
	appMenu.AddRole(application.EditMenu)
	appMenu.AddRole(application.WindowMenu)
	// ⌘W closes the visible in-app browser tab and nothing else. It has to be
	// a menu accelerator: with focus inside the embedded page the SPA never
	// sees the keystroke. Deliberately not the stock CloseWindow role — that
	// closes the focused window, and here that is the app.
	if item := appMenu.FindByLabel("Window"); item != nil && item.IsSubmenu() {
		win := item.GetSubmenu()
		win.AddSeparator()
		win.Add("Close Tab").
			SetAccelerator("CmdOrCtrl+w").
			OnClick(func(*application.Context) { browse.CloseActive() })
	}
	// Tools → Install Command Line Tool… (macOS only; no-op stub elsewhere).
	if runtime.GOOS == "darwin" {
		appendInstallCLIMenu(appMenu)
	}
	app.Menu.Set(appMenu)

	window = app.Window.NewWithOptions(mainWindowOptions())
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		log.Print("wails runtime ready — --wails-draggable listeners are attached")
	})

	applyDeepLink = deepLinkNavigator(config.Profile(), func(path string) {
		window.SetURL(path)
		raiseWindow(window)
	})
	// The other delivery route: the app was already running, so the Apple
	// Event reaches this process and wails emits it as an application event.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl,
		func(e *application.ApplicationEvent) {
			applyDeepLink(e.Context().URL())
		})
	// Dock click (minimised or no visible window). wails' own handler only
	// Show()s when HasVisibleWindows is false; a miniaturised window is still
	// "visible", so Restore+Focus is the same raise the second-instance path
	// already uses. The event is defined on every GOOS and never fires off
	// darwin — same as ApplicationLaunchedWithUrl.
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen,
		func(*application.ApplicationEvent) {
			raiseWindow(window)
		})

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		// Sidebar banner only (internal/selfupdate). updateCheck: false
		// silences it; StartUpdateCheck also no-ops internally when disabled.
		// Installing an update is brew cask / a new dmg — no in-app swap.
		if cfg.UpdateCheckEnabled() {
			if dir, err := config.Dir(); err == nil {
				api.StartUpdateCheck(ctx, dir)
			}
		}
		// Same delayed-start seam as cmd/gadak serve: with a credential the
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

// mainWindowOptions is the single construction site for the window so a test
// can pin that /wails/runtime.js is not also loaded via the JS option.
// serveDesktopIndex injects the <script> tag into every index.html response.
func mainWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:     "Gadak",
		Width:     1280,
		Height:    820,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
		// No JS: option. /wails/runtime.js is injected once by
		// serveDesktopIndex so a navigation does not load the module twice
		// (the option ran on every navigation, on top of the <script> tag).
		Mac: application.MacWindow{
			// No native title bar: it spent 28px to repeat a word the sidebar
			// already shows. The window controls stay (they move into the
			// sidebar's first row, which reserves their width and is a drag
			// handle — see .desktop-titlebar-row in web/src/app.css). The
			// list toolbar is the other handle (.desktop-drag-region).
			TitleBar: application.MacTitleBarHiddenInset,
		},
	}
}

// raiseWindow is the one raise path: second-instance, deeplink, and dock
// reopen. Restore un-minimises; Focus brings the window forward.
func raiseWindow(w *application.WebviewWindow) {
	if w == nil {
		return
	}
	w.Restore()
	w.Focus()
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
		if name == "index.html" {
			serveDesktopIndex(w, ui)
			return
		}
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
// open-in-browser and browse-tab routes, and the SPA's index.html for
// client-side routes. openURL opens a link in the system browser; nil disables
// the route (503) — it is bound to app.Browser.OpenURL after the application
// exists. browse is the in-app browser pane registry; before bind() its routes
// answer 503 the same way.
func fallbackHandler(api http.Handler, ui fs.FS, reg *workspace.Registry, openURL func(string) error, browse *browseTabs, setClipboard func(string) bool) http.Handler {
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
	// navigator.clipboard is dead inside the wails webview — measured on the
	// installed 0.15 build: writeText rejected while the UI's old
	// catch-and-confirm toast still said "copied" (GDK-178). The web bundle
	// (desktop mode only, lib/copy-text.ts) posts the text here and the app
	// writes the system pasteboard, the same call install_cli.go uses. Only
	// the webview can reach this — there is no TCP listener.
	mux.HandleFunc("POST /desktop/clipboard", func(w http.ResponseWriter, r *http.Request) {
		if setClipboard == nil {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
			return
		}
		if !setClipboard(body.Text) {
			log.Printf("clipboard: SetText refused %d bytes", len(body.Text))
			http.Error(w, `{"error":"clipboard_failed"}`, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// The in-app browser pane. The SPA sends Atlassian-origin links here
	// instead of /desktop/open (web/src/lib/desktop-links.ts decides which);
	// each becomes an embedded webview tab layered over the pane rect the SPA
	// reports. Same reachability story as /desktop/open: only the webview can
	// call these, so the URL filter mirrors that route rather than re-deriving
	// the configured site per workspace.
	browseErr := func(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, errBrowseUnavailable):
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
		default:
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
		}
	}
	decodeInto := func(w http.ResponseWriter, r *http.Request, v any) bool {
		if err := json.NewDecoder(r.Body).Decode(v); err != nil {
			http.Error(w, `{"error":"bad_request"}`, http.StatusBadRequest)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /desktop/browse", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if !decodeInto(w, r, &body) {
			return
		}
		u, err := url.Parse(body.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			http.Error(w, `{"error":"bad_url"}`, http.StatusBadRequest)
			return
		}
		id, err := browse.Open(u.String())
		if err != nil {
			log.Printf("open browse tab: %v", err)
			browseErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	})
	// The SPA polls this while it has tabs open: strip order, live titles for
	// the tab labels, and the current URL so the pane can show where the user
	// actually is. A known id missing from open means the tab closed (⌘W) —
	// resync that item.
	mux.HandleFunc("GET /desktop/browse/state", func(w http.ResponseWriter, r *http.Request) {
		open, active := browse.State()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"open": open, "active": active})
	})
	// Which tab is visible; "" means none (the SPA is showing its own UI, or
	// has an overlay up that must not be painted over by a native view).
	mux.HandleFunc("POST /desktop/browse/activate", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if !decodeInto(w, r, &body) {
			return
		}
		if err := browse.Activate(body.ID); err != nil {
			browseErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /desktop/browse/close", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if !decodeInto(w, r, &body) {
			return
		}
		// "" closes everything: a freshly mounted SPA document asks this on
		// boot so a predecessor's webviews cannot outlive it (GDK-80).
		if body.ID == "" {
			browse.CloseAll()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := browse.CloseTab(body.ID); err != nil {
			browseErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// The pane rect in the SPA's own coordinates (CSS px, y from top). Sent on
	// mount and on layout changes; window resizes track natively in between.
	mux.HandleFunc("POST /desktop/browse/frame", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			X, Y, W, H float64
		}
		if !decodeInto(w, r, &body) {
			return
		}
		if body.W < 0 || body.H < 0 {
			http.Error(w, `{"error":"bad_frame"}`, http.StatusBadRequest)
			return
		}
		browse.SetFrame(frameRect{X: body.X, Y: body.Y, W: body.W, H: body.H})
		w.WriteHeader(http.StatusNoContent)
	})
	// PUT settings/ rewrites the config on disk, so re-read it per request
	// (mirrors cmd/gadak buildServeMux).
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
		serveDesktopIndex(w, ui)
	})
	if reg != nil {
		ws := reg.Handler(spa, appVersion)
		mux.HandleFunc("/w/", func(w http.ResponseWriter, r *http.Request) {
			// Same webview identity normalization as /api/ so workspace API
			// routes pass the browser guard.
			r.Header.Del("Origin")
			r.Host = "127.0.0.1"
			// A workspace page in this webview is still the desktop app. The
			// registry serves the base web config (it also serves plain
			// `gadak serve`), so the desktop flag must be stamped here — the
			// same one the root /config.json gets — or the SPA on /w/<name>/
			// thinks it is in a browser and uses transports that are dead in
			// the webview (GDK-178: navigator.clipboard rejected while the
			// copy toast said copied; /desktop/open interception, same class).
			if strings.HasSuffix(r.URL.Path, "/config.json") {
				rec := &bufferedResponse{header: http.Header{}, code: http.StatusOK}
				ws(rec, r)
				body := rec.buf.Bytes()
				if rec.code == http.StatusOK {
					if doc, err := withDesktopFlag(body); err == nil {
						body = doc
					}
				}
				for k, vs := range rec.header {
					if k == "Content-Length" {
						continue
					}
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(rec.code)
				_, _ = w.Write(body)
				return
			}
			ws(w, r)
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: unknown paths are client-side routes.
		serveDesktopIndex(w, ui)
	})
	return mux
}

// wailsRuntimeScript is how every v3 example loads drag. The same index.html
// is embedded for `gadak serve`, so the tag cannot live in the file; only
// this desktop handler injects it.
const wailsRuntimeScript = `<script type="module" src="/wails/runtime.js"></script>`

func injectWailsRuntime(html []byte) []byte {
	if bytes.Contains(html, []byte("/wails/runtime.js")) {
		return html
	}
	if i := bytes.LastIndex(html, []byte("</head>")); i >= 0 {
		out := make([]byte, 0, len(html)+len(wailsRuntimeScript))
		out = append(out, html[:i]...)
		out = append(out, wailsRuntimeScript...)
		out = append(out, html[i:]...)
		return out
	}
	return append(append([]byte(nil), html...), wailsRuntimeScript...)
}

func serveDesktopIndex(w http.ResponseWriter, ui fs.FS) {
	if ui == nil {
		http.Error(w, "no ui", http.StatusNotFound)
		return
	}
	raw, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(injectWailsRuntime(raw))
}

// withDesktopFlag marks the config document the app serves. One web bundle is
// shared with `gadak serve`, and only here is the native title bar hidden — the
// UI has to reserve the window controls' corner, and a browser tab must not.
// bufferedResponse captures a handler's response so the /w/ config.json can
// be stamped with the desktop flag before it leaves (see fallbackHandler).
type bufferedResponse struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }
func (b *bufferedResponse) WriteHeader(code int) { b.code = code }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.buf.Write(p) }

func withDesktopFlag(doc []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		return nil, err
	}
	m["desktop"] = true
	return json.Marshal(m)
}
