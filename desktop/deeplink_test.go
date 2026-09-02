package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/deeplink"
)

// The grammar's own cases live in internal/deeplink. What is tested here is
// everything between a parsed link and the window: the action table, the path
// composition, the argv scan, the silence/log split, and the one fact that
// lives in two files at once — the scheme.

// Every profile the tests below name as a mount exists on the imaginary
// machine except the ones spelled "elsewhere" (GDK-1309).
func stubProfiles(t *testing.T) {
	t.Helper()
	saved := profileExists
	profileExists = func(name string) bool { return name != "elsewhere" }
	t.Cleanup(func() { profileExists = saved })
}

func TestDeepLinkTarget(t *testing.T) {
	stubProfiles(t)
	for _, tc := range []struct {
		name, raw, served, want string
	}{
		{
			// The path `gadak views open` would have handed out for the same
			// view on the same mirror: primary profile, no /w/ segment.
			name: "primary profile navigates to the root mount",
			raw:  "gadak://view?ks=NMA-1,NMA-2", served: "",
			want: "/#/?ks=NMA-1,NMA-2",
		},
		{
			name: "a named profile becomes its workspace mount",
			raw:  "gadak://view/w/work?ks=NMA-1", served: "",
			want: "/w/work/#/?ks=NMA-1",
		},
		{
			// The app is already running that profile, so it is the root
			// mount here — the same link means different paths in different
			// windows, and that is the point of passing served.
			name: "the profile the app already serves is the root mount",
			raw:  "gadak://view/w/work?ks=NMA-1", served: "work",
			want: "/#/?ks=NMA-1",
		},
		{
			// GitHub #85 / GDK-1309: /w/default can never be served (the
			// primary has no profiles/ directory), so the primary is the
			// root mount on every server — this is the Settings… ⌘, link.
			name: "the primary profile seen from a named app is the root mount",
			raw:  "gadak://view?pj=NMA&sc=inprogress", served: "work",
			want: "/#/?pj=NMA&sc=inprogress",
		},
		{
			// Percent-encoding survives the whole trip, not just the parser.
			name: "encoding is preserved through composition",
			raw:  "gadak://view?q=%ED%95%9C%EA%B8%80", served: "",
			want: "/#/?q=%ED%95%9C%EA%B8%80",
		},
		{
			// issue=KEY is a view parameter, not a separate action: the
			// detail panel is the one non-list surface with a URL today.
			name: "an issue detail is a view parameter",
			raw:  "gadak://view/w/oss?issue=GDK-119", served: "",
			want: "/w/oss/#/?issue=GDK-119",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deepLinkTarget(tc.raw, tc.served)
			if err != nil {
				t.Fatalf("deepLinkTarget(%q, %q) error: %v", tc.raw, tc.served, err)
			}
			if got != tc.want {
				t.Fatalf("deepLinkTarget(%q, %q) = %q, want %q", tc.raw, tc.served, got, tc.want)
			}
		})
	}
}

func TestDeepLinkTargetRejects(t *testing.T) {
	stubProfiles(t)
	for _, tc := range []struct {
		name, raw string
		is        error // nil = any error will do
	}{
		{"another scheme", "https://example.com/?ks=A-1", deeplink.ErrNotGadak},
		{"traversal", "gadak://view/w/../?x=1", deeplink.ErrMalformed},
		{"a view with no hash", "gadak://view", nil},
		{"a view with a subject", "gadak://view/w/oss/GDK-119", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deepLinkTarget(tc.raw, "")
			if err == nil {
				t.Fatalf("deepLinkTarget(%q) = %q, want an error", tc.raw, got)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Fatalf("deepLinkTarget(%q) error = %v, want %v", tc.raw, err, tc.is)
			}
		})
	}
}

// TestUnsupportedActionIsItsOwnAnswer covers the case this design exists for:
// a link from a newer gadak naming a surface this build has never heard of.
// It must not read as a broken link — the user's link is fine, the app is old
// — and it must not read as a parse failure either, or the grammar would have
// to change every time an action is added.
func TestUnsupportedActionIsItsOwnAnswer(t *testing.T) {
	for _, raw := range []string{
		"gadak://settings?tab=sync",
		"gadak://setup",
		"gadak://doc/w/oss/SPACE-1234",
		"gadak://person/w/oss/dev@example.com",
	} {
		_, err := deepLinkTarget(raw, "")
		if !errors.Is(err, errUnsupportedAction) {
			t.Fatalf("deepLinkTarget(%q) error = %v, want errUnsupportedAction", raw, err)
		}
		if errors.Is(err, deeplink.ErrMalformed) {
			t.Fatalf("deepLinkTarget(%q) called a well-formed link malformed", raw)
		}
		// The message is what the user sees in the log; it has to point at the
		// app's version rather than blame their link.
		if !strings.Contains(err.Error(), "newer Gadak") {
			t.Fatalf("deepLinkTarget(%q) error %q does not send the user to an upgrade", raw, err)
		}
	}
}

// TestEveryActionIsRegistered guards the one thing the table cannot say about
// itself: that an action constant the parser exports has a resolver here. A
// constant added without an entry compiles fine and fails only at runtime,
// on a link, in front of a user.
func TestEveryActionIsRegistered(t *testing.T) {
	if _, ok := actions[deeplink.ActionView]; !ok {
		t.Fatalf("deeplink.ActionView has no resolver in the actions table")
	}
}

func TestFirstDeepLinkArg(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no url at all — a plain open -a Gadak", []string{"/Applications/Gadak.app/Contents/MacOS/gadak-desktop"}, ""},
		{"appended at the end, where wails puts it",
			[]string{"gadak-desktop", "gadak://view?ks=A-1"}, "gadak://view?ks=A-1"},
		{"not positional — a flag may sit between",
			[]string{"gadak-desktop", "--flag", "gadak://view?ks=A-1"}, "gadak://view?ks=A-1"},
		{"scheme case is not the OS's promise",
			[]string{"gadak-desktop", "GADAK://view?ks=A-1"}, "GADAK://view?ks=A-1"},
		{"another app's url is not ours",
			[]string{"gadak-desktop", "https://example.com"}, ""},
		{"an action we do not know is still ours to handle",
			[]string{"gadak-desktop", "gadak://settings?tab=sync"}, "gadak://settings?tab=sync"},
		{"the first one wins", []string{"gadak-desktop", "gadak://view?a=1", "gadak://view?b=2"},
			"gadak://view?a=1"},
		{"empty argv", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstDeepLinkArg(tc.args); got != tc.want {
				t.Fatalf("firstDeepLinkArg(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestDeepLinkNavigator(t *testing.T) {
	stubProfiles(t)
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"a good link navigates", "gadak://view/w/work?ks=A-1", "/w/work/#/?ks=A-1"},
		// Not ours: another app's scheme can reach a shared handler, and
		// hijacking the window for it would be the bug.
		{"another scheme is ignored", "https://example.com/", ""},
		{"a mailto is ignored", "mailto:x@example.com", ""},
		// Ours but refused: the window must not move. A hostile page's link
		// getting a navigation out of us is exactly what the parser is for.
		{"a traversal attempt does not navigate", "gadak://view/w/../?x=1", ""},
		{"an empty query does not navigate", "gadak://view", ""},
		{"an unsupported action does not navigate", "gadak://settings?tab=sync", ""},
		{"an empty string does not navigate", "", ""},
		// GitHub #85: well-formed, names a workspace this machine does not
		// have. Navigating would land on a bare 404 with no way back.
		{"a workspace not on this machine does not navigate", "gadak://view/w/elsewhere?issue=GADAK-1234", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			calls := 0
			deepLinkNavigator("", func(p string) { got = p; calls++ })(tc.raw)
			if tc.want == "" && calls != 0 {
				t.Fatalf("navigated to %q for %q; want no navigation", got, tc.raw)
			}
			if tc.want != "" && got != tc.want {
				t.Fatalf("navigated to %q for %q, want %q", got, tc.raw, tc.want)
			}
		})
	}
}

// TestBundleRegistersTheScheme is the cross-artifact check. The handler above
// and the bundle's Info.plist are the two halves of the feature, they live in
// a .go and a .sh file that nothing else connects, and a disagreement between
// them produces no error anywhere: macOS simply never delivers the URL, and
// the link does nothing. Reading the script is crude but it is where the
// truth is — the plist is generated, so there is no checked-in plist to read.
func TestBundleRegistersTheScheme(t *testing.T) {
	body, err := os.ReadFile("build-app.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "<key>CFBundleURLTypes</key>") {
		t.Fatal("build-app.sh writes no CFBundleURLTypes: the OS will never deliver a gadak:// URL")
	}
	if want := "<string>" + deeplink.Scheme + "</string>"; !strings.Contains(s, want) {
		t.Fatalf("build-app.sh does not register the %q scheme (looked for %s)", deeplink.Scheme, want)
	}
}

// TestNoSuchWorkspaceIsToldToTheUser: the one refusal that gets a dialog. A
// mistyped or hostile link stays log-only; a link from another machine's
// gadak used to become the 404 page, so the user needs the sentence.
func TestNoSuchWorkspaceIsToldToTheUser(t *testing.T) {
	stubProfiles(t)
	shown := ""
	saved := showDeepLinkRefusal
	showDeepLinkRefusal = func(text string) { shown = text }
	t.Cleanup(func() { showDeepLinkRefusal = saved })
	navigated := false
	nav := deepLinkNavigator("", func(string) { navigated = true })

	nav("gadak://view/w/elsewhere?issue=GADAK-1234")
	if navigated {
		t.Fatal("navigated to a workspace that is not here")
	}
	if !strings.Contains(shown, "elsewhere") || !strings.Contains(shown, "not on this machine") {
		t.Fatalf("dialog text %q, want the workspace name and the reason", shown)
	}

	shown = ""
	nav("gadak://view/w/../?x=1")
	if shown != "" {
		t.Fatalf("a malformed link raised a dialog: %q", shown)
	}
}
