package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/midagedev/gadak/internal/deeplink"
	"github.com/midagedev/gadak/internal/jql"
	"github.com/midagedev/gadak/internal/workspace"
)

// gadak:// links, from the OS to this window.
//
// The app already had one way to be told what to show — the uifocus file the
// CLI writes and the running UI polls. This path deliberately does not use
// it. uifocus is consumed per workspace mount (internal/server/focus.go takes
// for its own profile), so a hash written for profile X sits unread while the
// window shows profile Y, and the link would look like it did nothing. The
// desktop app owns its webview; navigating it is the one answer that is right
// whichever workspace is on screen.
//
// macOS delivers the URL by two different routes and both land here:
//
//   - app already running: the Apple Event reaches wails' protocol handler,
//     which emits events.Common.ApplicationLaunchedWithUrl.
//   - app not running, or a second launch: LaunchServices starts a second
//     process, wails captures the pending kAEGetURL there and appends the URL
//     to SecondInstanceData.Args before exiting, so it arrives in the
//     OnSecondInstanceLaunch callback of the instance that was already up.
//
// Nothing here is macOS-specific in Go terms. Which GOOS emits
// ApplicationLaunchedWithUrl is owned by coldStartDecisionFor in main.go
// (GDK-293): Windows does, when wails sees a single "://" argument
// (pkg/application/application_windows.go in the pinned wails module);
// GTK4 Linux never does, so argv is applied instead. The same split is
// in the platform table in README.md.

// errUnsupportedAction reports a well-formed link naming something this build
// does not implement. It is deliberately distinct from a malformed link: the
// grammar is fixed but the set of actions grows, so this is what an older app
// says when it meets a newer link, and the message has to send the user to an
// upgrade rather than suggest their link is wrong.
var errUnsupportedAction = errors.New("this link needs a newer Gadak")

// actions is the whole registry of what a gadak:// link may ask for. Adding a
// surface is an entry here plus a resolver — the parser in internal/deeplink
// knows no action names at all, on purpose, so a new one never becomes a
// grammar change.
//
// Everything in this table must be a *navigation*. The scheme carries no
// verbs (see internal/deeplink's package comment): any web page can put a
// gadak:// link on it, and the worst one may achieve is that the user looks
// at the wrong thing. An action that submits, writes, or confirms does not
// belong here however convenient it would be.
//
// Empty today beyond `view`, and that is a fact about the UI rather than a
// choice about the scheme. Measured 2026-08-16: the view hash — which
// includes `issue=KEY` for the detail panel — is the only part of the web app
// that has a URL at all. Documents, people, settings tabs, and onboarding are
// opened by store calls with nothing to name them. Those become entries here
// when they become addressable, not before.
var actions = map[string]func(l deeplink.Link, served string) (string, error){
	deeplink.ActionView: resolveView,
}

// resolveView turns a view link into the path to navigate to: the same two
// pieces `gadak views open` uses for the http:// URL it prints, in the same
// order, so a link built by the CLI and one typed by hand land in the same
// place.
//
// served is which mirror this window is already showing, and it is why the
// same link is a different path in different windows: /w/work in one, / in
// the one already on "work".
func resolveView(l deeplink.Link, served string) (string, error) {
	if l.Hash == "" {
		// The grammar allows a link with no query — a future `gadak://setup`
		// will have none — so refusing it is this action's rule, not the
		// parser's. Raising a window onto no particular view is a different
		// feature from showing one, and silently doing it would make a
		// mistyped link look like it worked.
		return "", fmt.Errorf("a view link needs a hash: %s", deeplink.Compose(
			deeplink.ActionView, "/w/<profile>", "<filters>"))
	}
	if l.Subject != "" {
		// `view` addresses a list, and the list is described entirely by the
		// hash. A subject here means the link meant some other action.
		return "", fmt.Errorf("a view link takes no subject, got %q", l.Subject)
	}
	prefix := workspace.Prefix(l.Profile, served)
	if prefix != "" && !profileExists(l.Profile) {
		// The grammar accepts any well-formed name, so a link minted on
		// another machine parses fine here — and its mount answers a bare
		// text 404 that replaces the app with no way back (GitHub #85,
		// GDK-1309). Refuse before navigating; the navigator tells the user.
		return "", fmt.Errorf("%w: %q", errNoSuchWorkspace, l.Profile)
	}
	return prefix + "/" + jql.QueryURL(l.Hash), nil
}

// profileExists is workspace.ProfileExists behind a var so the resolver's
// decision is testable without a profiles/ directory on disk.
var profileExists = workspace.ProfileExists

// errNoSuchWorkspace is the refusal for a link whose workspace is not on this
// machine — the one refusal a user sees, because the link is well-formed and
// the only other outcome was the 404 page.
var errNoSuchWorkspace = errors.New("this link's workspace is not on this machine (a link from someone else's gadak?)")

// showDeepLinkRefusal surfaces a refusal the user should read. main wires it
// to a window-attached dialog; the default is silent so tests and headless
// paths stay quiet.
var showDeepLinkRefusal = func(text string) {}

// deepLinkTarget turns a gadak:// URL into the path to navigate this window
// to, for an app whose primary mirror is served.
//
// Errors keep the parser's classification — deeplink.ErrNotGadak for a URL
// that is not ours, something wrapping deeplink.ErrMalformed for one that is
// — and add errUnsupportedAction for a link this build cannot honour.
func deepLinkTarget(raw, served string) (string, error) {
	link, err := deeplink.Parse(raw)
	if err != nil {
		return "", err
	}
	resolve, ok := actions[link.Action]
	if !ok {
		return "", fmt.Errorf("%w (action %q)", errUnsupportedAction, link.Action)
	}
	return resolve(link, served)
}

// firstDeepLinkArg returns the first gadak:// argument in a second instance's
// argv, or "" when there is none.
//
// It scans rather than reading a fixed position: wails appends the captured
// URL to os.Args, so its index depends on how the process was started, and a
// normal `open -a Gadak` carries no URL at all.
func firstDeepLinkArg(args []string) string {
	prefix := deeplink.Scheme + "://"
	for _, a := range args {
		if strings.HasPrefix(strings.ToLower(a), prefix) {
			return a
		}
	}
	return ""
}

// deepLinkNavigator returns the handler both delivery routes call. navigate
// is what actually moves the window; it is a parameter so the decision logic
// above it is testable without an application.
//
// A URL that is not ours passes in silence — another app's scheme can reach a
// shared handler and is not an error. One that is ours but refused is logged
// with the reason, because the log is the only place a user whose link did
// nothing can find out why.
func deepLinkNavigator(served string, navigate func(path string)) func(raw string) {
	return func(raw string) {
		if raw == "" {
			return
		}
		target, err := deepLinkTarget(raw, served)
		if err != nil {
			if !errors.Is(err, deeplink.ErrNotGadak) {
				log.Printf("deep link refused: %v", err)
			}
			if errors.Is(err, errNoSuchWorkspace) {
				showDeepLinkRefusal(err.Error())
			}
			return
		}
		log.Printf("deep link → %s", target)
		navigate(target)
	}
}
