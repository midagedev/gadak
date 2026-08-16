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
// Callers join it ahead of the rest of the path, so the empty string is the
// correct answer for the primary profile and not a missing value.
func Prefix(profile, served string) string {
	if ProfileEq(profile, served) {
		return ""
	}
	name := profile
	if name == "" || name == "default" {
		name = "default"
	}
	return "/w/" + name
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
