package workspace

// The URL prefix rule for a profile, in the package that already owns /w/.
//
// Three callers need it and none of them can be the owner: `gadak views open`
// builds a link to a running serve, and the desktop app turns a gadak:// deep
// link into a window URL. Both must produce the same path for the same
// profile, or a link built by one and opened by the other lands somewhere
// else. It lived in cmd/gadak's package main, where the desktop module could
// not reach it — which is how a second copy gets written.

// Prefix is the path segment that selects `profile` on a server whose primary
// profile is `served`: "" when they are the same mirror, "/w/<name>" when the
// profile has to be reached as a workspace mount.
//
// The primary profile ("" or "default") is "" on every server, whatever
// `served` is. It used to become /w/default on a named server, and that
// mount can never answer: the existence gate reads directory names under
// profiles/ and the primary lives at the root with no directory, so the
// desktop's Settings… menu (a link with no /w/ segment) navigated a named
// workspace into a bare 404 with no way back (GitHub #85, GDK-1309). The
// deep-link grammar already says a segment-less link is the primary mirror,
// and the primary mirror is `/` on any serve.
//
// Callers join it ahead of the rest of the path, so the empty string is the
// correct answer for the primary profile and not a missing value.
func Prefix(profile, served string) string {
	if ProfileEq(profile, served) || ProfileEq(profile, "") {
		return ""
	}
	return "/w/" + profile
}

// ProfileEq reports whether two profile names mean the same mirror. The
// primary profile answers to both "" and "default" — the config layer stores
// it empty, humans and URLs write it out — so a comparison that misses this
// sends `gadak views open` to /w/default on the very server it is talking to.
func ProfileEq(got, want string) bool {
	norm := func(s string) string {
		if s == "default" {
			return ""
		}
		return s
	}
	return norm(got) == norm(want)
}
