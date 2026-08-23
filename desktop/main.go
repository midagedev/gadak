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
	"os"
	"path"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	gadak "github.com/midagedev/gadak"
	"github.com/midagedev/gadak/internal/apprun"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/integrations"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/workspace"
)

// appVersion is stamped by desktop/build-app.sh (-X main.appVersion). The
// default ("dev") keeps local builds identifiable. StartUpdateCheck compares
// server.Version, which cmd/gadak assigns from its own ldflags; without the
// same assignment here the sidebar banner stays silent (Version is 0.0.0-dev).
var appVersion = "dev"

func main() {
	// Importing internal/config no longer selects a workspace (GDK-644);
	// each main reads GADAK_WORKSPACE/GADAK_PROFILE/SCRY_PROFILE itself —
	// this is what makes `GADAK_PROFILE=work open -a Gadak` work.
	apprun.SelectWorkspace()
	if unregisterGadakProtocolIfRequested(os.Args[1:]) {
		return
	}
	if printWindowChromeIfRequested(os.Args[1:]) {
		return
	}
	if printIntegrationsIfRequested(os.Args[1:]) {
		return
	}
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

// coldStartDecision is the single owner of how a first-launch gadak://
// reaches this process: from argv, or from ApplicationLaunchedWithUrl.
type coldStartDecision struct {
	ApplyArgv    bool
	DeferToEvent bool
}

// coldStartDecisionFor reports the cold-start URL source for this GOOS and
// the process argv (full os.Args, including argv[0]).
//
//   - darwin: event only. LaunchServices delivers the URL as an Apple Event;
//     applying argv as well would navigate twice.
//   - windows: event when wails will emit ApplicationLaunchedWithUrl — that
//     is len(args)==2 and args[1] contains "://" (wails v3.0.0-beta.12
//     pkg/application/application_windows.go:159-162). Every other argv
//     shape is ignored by wails, so argv is the fallback.
//   - linux: same as windows. GTK4 run() in this pin
//     (application_linux.go:91-99) emits the event for that argv shape
//     (wailsapp/wails#6000, landed in beta.10). Every other argv shape is
//     the argv fallback. Unknown GOOS stays argv.
func coldStartDecisionFor(goos string, args []string) coldStartDecision {
	switch goos {
	case "darwin":
		return coldStartDecision{ApplyArgv: false, DeferToEvent: true}
	case "windows":
		if wailsEmitsLaunchURL(args) {
			return coldStartDecision{ApplyArgv: false, DeferToEvent: true}
		}
		return coldStartDecision{ApplyArgv: true, DeferToEvent: false}
	case "linux":
		if wailsEmitsLaunchURL(args) {
			return coldStartDecision{ApplyArgv: false, DeferToEvent: true}
		}
		return coldStartDecision{ApplyArgv: true, DeferToEvent: false}
	default:
		return coldStartDecision{ApplyArgv: true, DeferToEvent: false}
	}
}

// wailsEmitsLaunchURL is the argv shape wails v3.0.0-beta.12 special-cases
// on Windows (application_windows.go:159-162) and on GTK4 Linux
// (application_linux.go:91-99; wailsapp/wails#6000 landed in beta.10).
// GTK3 has the same check; this pin compiles GTK4.
func wailsEmitsLaunchURL(args []string) bool {
	return len(args) == 2 && strings.Contains(args[1], "://")
}

// wailsModuleVersion is the version of github.com/wailsapp/wails/v3 linked
// into this binary (debug.ReadBuildInfo). "unknown" if build info is missing
// or the module is not a dependency — neither happens for a normal build.
func wailsModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, m := range info.Deps {
		if m.Path != "github.com/wailsapp/wails/v3" {
			continue
		}
		if m.Replace != nil && m.Replace.Version != "" {
			return m.Replace.Version
		}
		if m.Version != "" {
			return m.Version
		}
	}
	return "unknown"
}

// coldStartGate applies a cold-start URL only after WindowRuntimeReady.
// Bound: one slot. An offer that arrives before ready is queued; a second
// offer before ready is dropped (first wins). After ready, offer applies
// immediately. A URL that is still queued when markReady runs is flushed
// then. There is no drop-on-timeout — the window either becomes ready or
// the process exits.
type coldStartGate struct {
	mu         sync.Mutex
	ready      bool
	pendingRaw string
	pendingSrc string
	apply      func(raw, source string)
}

func (g *coldStartGate) offer(raw, source string) {
	if raw == "" {
		return
	}
	g.mu.Lock()
	if !g.ready {
		if g.pendingRaw == "" {
			g.pendingRaw = raw
			g.pendingSrc = source
		}
		g.mu.Unlock()
		return
	}
	apply := g.apply
	g.mu.Unlock()
	if apply != nil {
		apply(raw, source)
	}
}

func (g *coldStartGate) markReady() {
	g.mu.Lock()
	g.ready = true
	raw, src := g.pendingRaw, g.pendingSrc
	g.pendingRaw, g.pendingSrc = "", ""
	apply := g.apply
	g.mu.Unlock()
	if raw != "" && apply != nil {
		apply(raw, src)
	}
}

func run() error {
	rt, err := apprun.Open(apprun.Options{
		Version:         server.Version,
		DeferStandalone: true, // GDK-658: persist after application.New
		FlushOnClose:    true, // GDK-342/348: desktop has no cmd/gadak main flush
	})
	if err != nil {
		return err
	}
	// LIFO with cancel below: cancel Watch, then Close (origin listener
	// stop before persist flush, API before DB).
	defer rt.Close()
	cfg, api := rt.Cfg, rt.API

	ui, ok := gadak.WebUI()
	if !ok {
		log.Printf("warning: no web UI embedded — run `npm run build` at the repo root before building")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := rt.Reg

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

	if cfg.IsStandalone() {
		// Second instance prints this then os.Exits inside New(); persist
		// and advertise are not held yet (GDK-658).
		log.Printf("gadak-desktop: single-instance check (standalone persist not held)")
	}
	app := application.New(application.Options{
		// Name labels the macOS app menu; Name + Description are what the
		// About panel shows.
		Name:        "Gadak",
		Description: "Jira and Confluence, mirrored to your disk.",
		// wails still os.Exit(1) after this returns. We show a dialog and
		// write stderr first so a missing WebView2 is not a silent death.
		ErrorHandler: handleDesktopFatal,
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
	// paste during onboarding would fail. The app menu supplies About/Quit
	// and Settings… ⌘, (CLI install lives on Settings → Integrations).
	// Built after application.New: the app menu takes its label from Name.
	appMenu := app.NewMenu()
	if runtime.GOOS == "darwin" {
		gadakMenu := appMenu.AddSubmenu("Gadak")
		gadakMenu.AddRole(application.About)
		gadakMenu.AddSeparator()
		gadakMenu.Add("Settings…").
			SetAccelerator("CmdOrCtrl+,").
			OnClick(func(*application.Context) {
				if window == nil || applyDeepLink == nil {
					return
				}
				applyDeepLink("gadak://view?settings=sync")
			})
		gadakMenu.Add("Check for Updates…").
			OnClick(func(*application.Context) {
				// Network I/O off the click path. Settings opens after the
				// check so GET update/ already has the result.
				go func() {
					if dir, err := config.Dir(); err == nil {
						_ = api.CheckNow(context.Background(), dir)
					}
					if window == nil || applyDeepLink == nil {
						return
					}
					applyDeepLink("gadak://view?settings=sync")
				}()
			})
		gadakMenu.AddSeparator()
		gadakMenu.AddRole(application.ServicesMenu)
		gadakMenu.AddSeparator()
		gadakMenu.AddRole(application.Hide)
		gadakMenu.AddRole(application.HideOthers)
		gadakMenu.AddRole(application.UnHide)
		gadakMenu.AddSeparator()
		gadakMenu.AddRole(application.Quit)
	} else {
		appMenu.AddRole(application.AppMenu)
	}
	appMenu.AddRole(application.EditMenu)
	appMenu.AddRole(application.WindowMenu)
	// ⌘W closes the visible in-app browser tab and nothing else. It has to be
	// a menu accelerator: with focus inside the embedded page the SPA never
	// sees the keystroke. Deliberately not the stock CloseWindow role — that
	// closes the focused window, and here that is the app. On a build without
	// the pane the item is not created at all: browse.CloseActive() would be
	// a no-op there, and a Ctrl+W that visibly does nothing was os-audit F-2
	// (GDK-351). Falling back to closing the window was considered and
	// rejected — it would discard an open comment draft on a reflex keystroke.
	if item := appMenu.FindByRole(application.WindowMenu); paneSupported && item != nil && item.IsSubmenu() {
		win := item.GetSubmenu()
		win.AddSeparator()
		win.Add("Close Tab").
			SetAccelerator("CmdOrCtrl+w").
			OnClick(func(*application.Context) { browse.CloseActive() })
	}
	// Help is darwin-only this round: Windows/Linux still use the stock
	// AppMenu/Edit/Window roles. openURL is app.Browser.OpenURL, which on
	// darwin is `open <url>` with no scheme filter, so mailto uses the same
	// path as https.
	if runtime.GOOS == "darwin" {
		openHelp := func(u string) func(*application.Context) {
			return func(*application.Context) {
				if openURL == nil {
					return
				}
				if err := openURL(u); err != nil {
					log.Printf("help menu: open %s: %v", u, err)
				}
			}
		}
		helpMenu := appMenu.AddSubmenu("Help")
		helpMenu.Add("GitHub Repository").OnClick(openHelp("https://github.com/midagedev/gadak"))
		helpMenu.Add("Report an Issue").OnClick(openHelp("https://github.com/midagedev/gadak/issues"))
		helpMenu.Add("Contact by Email").OnClick(openHelp("mailto:midagedev@gmail.com"))
		helpMenu.Add("@midagedev on X").OnClick(openHelp("https://x.com/midagedev"))
	}
	app.Menu.Set(appMenu)

	decision := coldStartDecisionFor(runtime.GOOS, os.Args)
	// One line answers "which wails, does this process defer to the
	// launch event, and is the window opted into the application menu"
	// without opening go.mod or upstream source.
	coldStart := "argv"
	if decision.DeferToEvent {
		coldStart = "event"
	}
	windowOpts := mainWindowOptions()
	log.Printf("gadak-desktop version=%s wails=%s cold_start=%s use_application_menu=%t", appVersion, wailsModuleVersion(), coldStart, windowOpts.UseApplicationMenu)
	if protocolRegistersFor(runtime.GOOS) {
		exe, err := os.Executable()
		if err != nil {
			log.Printf("gadak:// registration: %v", err)
		} else if status, err := registerGadakProtocol(exe); err != nil {
			log.Printf("gadak:// registration: %v", err)
		} else {
			log.Printf("gadak:// registration: %s", status)
		}
	}
	var gate coldStartGate
	gate.apply = func(raw, source string) {
		if applyDeepLink == nil {
			return
		}
		log.Printf("deep link source=%s", source)
		applyDeepLink(raw)
	}

	window = app.Window.NewWithOptions(windowOpts)
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		log.Print("wails runtime ready — --wails-draggable listeners are attached")
		// Cold-start URL source is coldStartDecisionFor, not "!= darwin".
		// Linux and Windows read argv only when wails will not emit the event
		// (len(os.Args) != 2 or the single arg has no "://"; Linux GTK4 emits
		// since wailsapp/wails#6000, landed in beta.10). macOS never reads
		// argv. Nothing is handed to the webview until this event: an
		// ApplicationLaunchedWithUrl that arrived earlier sits in gate
		// (one slot, first offer wins) and is flushed here. Argv, when this
		// process owns it, is offered after the flush so it cannot race a
		// pending event.
		gate.markReady()
		if decision.ApplyArgv {
			if raw := firstDeepLinkArg(os.Args[1:]); raw != "" {
				gate.offer(raw, "argv")
			}
		}
	})

	applyDeepLink = deepLinkNavigator(config.Profile(), func(path string) {
		window.SetURL(path)
		raiseWindow(window)
	})
	// ApplicationLaunchedWithUrl: macOS Apple Event (first launch and
	// same-process reopen) and Windows/Linux when wails sees a single
	// argument containing "://" (GTK4 Linux: wailsapp/wails#6000, landed
	// in beta.10). Offers go through the ready gate so a URL that arrives
	// before the webview exists is queued, not applied.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl,
		func(e *application.ApplicationEvent) {
			if !decision.DeferToEvent {
				return
			}
			gate.offer(e.Context().URL(), "event")
		})
	// Dock click (minimised or no visible window). wails' own handler only
	// Show()s when HasVisibleWindows is false; a miniaturised window is still
	// "visible", so Restore+Focus is the same raise the second-instance path
	// already uses. ApplicationShouldHandleReopen is macOS-only; do not
	// analogise it to ApplicationLaunchedWithUrl, which Windows does emit.
	app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen,
		func(*application.ApplicationEvent) {
			raiseWindow(window)
		})

	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		// Sidebar banner only (internal/selfupdate). updateCheck: false
		// silences it; StartUpdateCheck also no-ops internally when disabled.
		// Installing an update is brew cask / a new dmg — no in-app swap.
		// WatchLoop re-entry and the onboarding starter live in apprun
		// (GDK-663); this event is when desktop starts them — serve starts
		// immediately after mux construction.
		rt.StartWatch(ctx, false)
	})

	if cfg.IsStandalone() {
		// After application.New so wails SingleInstance can os.Exit the
		// second process before persist is taken (GDK-658).
		if _, err := rt.StartOriginPassthrough(); err != nil {
			return err
		}
	}

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

// Window chrome is one fact: do the window controls sit in the content
// (traffic-light inset) or in a native title bar? Window options and the
// config.json document both consume this; they must not decide independently.
// Values name the chrome, not the GOOS — a later platform that uses the same
// inset layout should return the inset token, not a new "is Mac" flag.
const (
	windowChromeNative             = "native"
	windowChromeTrafficLightsInset = "traffic-lights-inset"
)

func windowChrome() string {
	return windowChromeFor(runtime.GOOS)
}

func windowChromeFor(goos string) string {
	if goos == "darwin" {
		return windowChromeTrafficLightsInset
	}
	return windowChromeNative
}

// printWindowChromeIfRequested is the one-command probe for "why is there a
// gap / a native title bar". doctor.go would be the natural home but cannot
// import this module (and must not copy the GOOS rule). Matches the exact
// flag only — argv also carries gadak:// deeplinks.
func printWindowChromeIfRequested(args []string) bool {
	for _, a := range args {
		if a == "--print-window-chrome" {
			fmt.Printf("window_chrome=%s\n", windowChrome())
			return true
		}
	}
	return false
}

// printIntegrationsIfRequested is the one-command probe for "what would
// Settings → Integrations show on this host". Same shape as
// --print-window-chrome: match the exact flag, print, exit, no window.
func printIntegrationsIfRequested(args []string) bool {
	for _, a := range args {
		if a == "--print-integrations" {
			enc := json.NewEncoder(os.Stdout)
			_ = enc.Encode(map[string]any{"items": integrations.List()})
			return true
		}
	}
	return false
}

// webview2EvergreenURL is Microsoft's Evergreen Runtime installer page.
// Named in the missing-runtime dialog and on stderr.
const webview2EvergreenURL = "https://developer.microsoft.com/en-us/microsoft-edge/webview2/"

func isMissingWebView2(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "no webview2 found") {
		return true
	}
	if !strings.Contains(s, "webview2") {
		return false
	}
	return strings.Contains(s, "not found") ||
		strings.Contains(s, "does not exist") ||
		strings.Contains(s, "runtime version")
}

func webview2UserMessage(err error) string {
	var b strings.Builder
	if isMissingWebView2(err) {
		b.WriteString("Gadak needs the Microsoft Edge WebView2 Runtime, which was not found on this PC.")
	} else if err != nil {
		b.WriteString("Gadak could not start its window.\n\n")
		b.WriteString(err.Error())
	} else {
		b.WriteString("Gadak could not start its window.")
	}
	b.WriteString("\n\nInstall the Evergreen Runtime from:\n")
	b.WriteString(webview2EvergreenURL)
	return b.String()
}

func handleDesktopFatal(err error) {
	// wails os.Exit(1)s after this returns, skipping every defer — including
	// the standalone persist flush in run(). Flush here or a fatal inside
	// the debounce window silently drops the last write (GDK-348). Close is
	// idempotent and a no-op when no embedded origin is live.
	if cerr := origin.Close(); cerr != nil {
		fmt.Fprintf(os.Stderr, "warning: standalone persist flush on fatal: %v\n", cerr)
	}
	msg := webview2UserMessage(err)
	fmt.Fprintln(os.Stderr, msg)
	showNativeError("Gadak", msg)
}

func showNativeError(title, text string) {
	if runtime.GOOS != "windows" {
		return
	}
	windowsMessageBox(title, text)
}

func applyWindowChrome(opts *application.WebviewWindowOptions, chrome string) {
	if chrome != windowChromeTrafficLightsInset {
		return
	}
	// No native title bar: it spent 28px to repeat a word the sidebar
	// already shows. The window controls stay (they move into the
	// sidebar's first row, which reserves their width and is a drag
	// handle — see .desktop-titlebar-row in web/src/app.css). The
	// list toolbar is the other handle (.desktop-drag-region).
	opts.Mac.TitleBar = application.MacTitleBarHiddenInset
}

// mainWindowOptions is the single construction site for the window so a test
// can pin that /wails/runtime.js is not also loaded via the JS option.
// serveDesktopIndex injects the <script> tag into every index.html response.
func mainWindowOptions() application.WebviewWindowOptions {
	opts := application.WebviewWindowOptions{
		Title:     "Gadak",
		Width:     1280,
		Height:    820,
		MinWidth:  720,
		MinHeight: 480,
		URL:       "/",
		// No JS: option. /wails/runtime.js is injected once by
		// serveDesktopIndex so a navigation does not load the module twice
		// (the option ran on every navigation, on top of the <script> tag).

		// Windows creates the HWND with an empty HMENU unless this is set
		// or a per-window Windows.Menu is supplied
		// (wails v3.0.0-beta.12, webview_window_windows.go:447-464), so
		// without it the app menu we build above never reaches the window.
		// Linux is documented the same way and already falls back on its
		// own; darwin ignores the flag, since its app menu is always
		// global (application.go applies it at Run(), by GOOS).
		UseApplicationMenu: true,
	}
	applyWindowChrome(&opts, windowChrome())
	return opts
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
		// http(s) plus mailto (GDK-339, the About tab's contact link) only:
		// nothing else (file:, javascript:, custom schemes) has any business
		// here. openURL is `open <url>` on darwin, which handles both.
		u, err := url.Parse(body.URL)
		webURL := err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
		mailURL := err == nil && u.Scheme == "mailto" && u.Opaque != ""
		if !webURL && !mailURL {
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
	// Settings → Integrations. Desktop-only: gadak serve does not mount these.
	// List is local-file / local-process detection; install streams the
	// bundled CLI (gadak raycast|skill|mcp install) line by line.
	mux.HandleFunc("GET /desktop/integrations", handleIntegrationsGET)
	mux.HandleFunc("POST /desktop/integrations/{id}/install", handleIntegrationsInstall)
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

// bufferedResponse captures a handler's response so the /w/ config.json can
// be stamped with the desktop flag before it leaves (see fallbackHandler).
type bufferedResponse struct {
	header http.Header
	code   int
	buf    bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) WriteHeader(code int)        { b.code = code }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.buf.Write(p) }

// withDesktopFlag marks the config document the app serves. One web bundle is
// shared with `gadak serve`. desktop=true is "this is the app"; windowChrome
// is whether traffic lights sit in the content (the sidebar row reserves
// their corner only then). A browser tab gets neither.
func withDesktopFlag(doc []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		return nil, err
	}
	m["desktop"] = true
	m["windowChrome"] = windowChrome()
	return json.Marshal(m)
}
