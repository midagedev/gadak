package deeplink

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// Hash examples come from the existing handoff convention: uifocus.Write
// documents the hash as "query string, no #/?", and internal/server tests
// round-trip "ks=BBB-1" and "pj=NMA&sc=inprogress" through it.

// exactCapLink builds a link of exactly maxInputLen bytes from valid parts:
// the 13-byte "gadak://view?" prefix, four two-byte "k=" keys, three "&",
// and four values sharing the rest. It fails the test rather than silently
// testing a different length if a limit ever changes.
func exactCapLink(t *testing.T) string {
	t.Helper()
	const prefix = "gadak://view?"
	v := (maxInputLen - len(prefix) - 8 - 3) / 4
	if got := len(prefix) + 8 + 3 + 4*v; got != maxInputLen {
		t.Fatalf("exact-cap construction lands on %d bytes, want %d", got, maxInputLen)
	}
	if v > maxValueLen {
		t.Fatalf("exact-cap values %d bytes exceed the %d-byte value limit", v, maxValueLen)
	}
	val := strings.Repeat("v", v)
	return prefix + "a=" + val + "&b=" + val + "&c=" + val + "&d=" + val
}

// nParamsQuery is a query of n distinct valid parameters, p1=1 … pn=1.
func nParamsQuery(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("p%d=1", i+1)
	}
	return strings.Join(parts, "&")
}

func TestParse(t *testing.T) {
	exactCap := exactCapLink(t)
	overCap := exactCap + "&e=x"
	if len(overCap) <= maxInputLen {
		t.Fatalf("over-cap construction is only %d bytes", len(overCap))
	}

	tests := []struct {
		name    string
		raw     string
		want    Link
		wantErr error // sentinel, matched with errors.Is; nil means accept
	}{
		// ── Accepted shapes ──
		{
			name: "bare shape",
			raw:  "gadak://view?ks=BBB-1",
			want: Link{Action: "view", Hash: "ks=BBB-1"},
		},
		{
			name: "bare shape, trailing slash",
			raw:  "gadak://view/?ks=BBB-1",
			want: Link{Action: "view", Hash: "ks=BBB-1"},
		},
		{
			name: "profile shape",
			raw:  "gadak://view/w/oss?ks=NMA-1,NMA-2",
			want: Link{Action: "view", Profile: "oss", Hash: "ks=NMA-1,NMA-2"},
		},
		{
			name: "profile shape, trailing slash",
			raw:  "gadak://view/w/oss/?pj=NMA&sc=inprogress",
			want: Link{Action: "view", Profile: "oss", Hash: "pj=NMA&sc=inprogress"},
		},
		{
			// The same handoff composeServeURL builds: /w/<name> from
			// workspace.Prefix, then the hash. Parsing must recover both.
			name: "round trip with the composeServeURL convention",
			raw:  "gadak://view/w/work?ks=NMA-1,NMA-2",
			want: Link{Action: "view", Profile: "work", Hash: "ks=NMA-1,NMA-2"},
		},
		{
			// config treats "default" as the root, same as "".
			name: "default profile spelled out",
			raw:  "gadak://view/w/default?ks=A-1",
			want: Link{Action: "view", Profile: "default", Hash: "ks=A-1"},
		},
		{
			name: "percent-encoded value stays verbatim",
			raw:  "gadak://view?q=%ED%95%9C%EA%B8%80",
			want: Link{Action: "view", Hash: "q=%ED%95%9C%EA%B8%80"},
		},
		{
			// %26 and %3D must come back byte-identical: proof the hash is
			// not split on "&"/"=" and rejoined.
			name: "encoded ampersand and equals in value stay verbatim",
			raw:  "gadak://view?q=a%26b%3Dc&sc=done",
			want: Link{Action: "view", Hash: "q=a%26b%3Dc&sc=done"},
		},
		{
			// ";" is not a separator here, matching the UI's URLSearchParams
			// read, where it lands in the value.
			name: "semicolon is part of the value",
			raw:  "gadak://view?q=a;b&ks=A-1",
			want: Link{Action: "view", Hash: "q=a;b&ks=A-1"},
		},
		{
			// Measured: url.Parse does not validate query escapes, and the
			// UI's URLSearchParams is the lenient decoder — verbatim is the
			// contract.
			name: "invalid percent escape in value still verbatim",
			raw:  "gadak://view?q=%zz&ks=A-1",
			want: Link{Action: "view", Hash: "q=%zz&ks=A-1"},
		},
		{
			// Discovered-field axes serialize as f.<alias> (view-config.ts);
			// parameter order is preserved, never re-sorted.
			name: "dynamic f-dot axis, order preserved",
			raw:  "gadak://view/w/oss?f.assignee=me&ks=NMA-1",
			want: Link{Action: "view", Profile: "oss", Hash: "f.assignee=me&ks=NMA-1"},
		},
		{
			// The key shape has to admit a discovered-field alias that is an
			// ASCIISlug of a real Jira field name, not just the two-letter
			// static aliases.
			name: "long f-dot axis from a wordy custom field",
			raw:  "gadak://view?f.acceptance_criteria_verification_status=done",
			want: Link{Action: "view", Hash: "f.acceptance_criteria_verification_status=done"},
		},
		{
			// Measured (TestStdlibURLCaseBehavior): net/url lowercases the
			// scheme itself, so this fold is belt-and-braces.
			name: "uppercase scheme",
			raw:  "GADAK://view?ks=A-1",
			want: Link{Action: "view", Hash: "ks=A-1"},
		},
		{
			// Measured: net/url preserves host case, so the action fold is
			// load-bearing — and the action comes back lowercased.
			name: "uppercase action is folded",
			raw:  "gadak://VIEW?ks=A-1",
			want: Link{Action: "view", Hash: "ks=A-1"},
		},
		{
			// Path stays case-sensitive: profile names are directory names.
			name: "mixed-case scheme and action, path untouched",
			raw:  "Gadak://ViEw/w/oss?ks=A-1",
			want: Link{Action: "view", Profile: "oss", Hash: "ks=A-1"},
		},
		{
			// Validation runs on the decoded path, so over-encoding a
			// harmless name decodes to the name.
			name: "over-encoded profile decodes",
			raw:  "gadak://view/w/%6Fss?x=1",
			want: Link{Action: "view", Profile: "oss", Hash: "x=1"},
		},
		{
			name: "profile at the 64-character limit",
			raw:  "gadak://view/w/" + strings.Repeat("p", 64) + "?x=1",
			want: Link{Action: "view", Profile: strings.Repeat("p", 64), Hash: "x=1"},
		},
		{
			name: "key at the length limit",
			raw:  "gadak://view?k" + strings.Repeat("a", maxKeyLen-1) + "=1",
			want: Link{Action: "view", Hash: "k" + strings.Repeat("a", maxKeyLen-1) + "=1"},
		},
		{
			name: "value at the 512-byte limit",
			raw:  "gadak://view?q=" + strings.Repeat("v", maxValueLen),
			want: Link{Action: "view", Hash: "q=" + strings.Repeat("v", maxValueLen)},
		},
		{
			name: "parameters at the 32-param limit",
			raw:  "gadak://view?" + nParamsQuery(maxParams),
			want: Link{Action: "view", Hash: nParamsQuery(maxParams)},
		},
		{
			name: "input at the 2048-byte limit",
			raw:  exactCap,
			want: Link{Action: "view", Hash: exactCap[len("gadak://view?"):]},
		},

		// ── The grammar beyond today's one action ──
		// These are the reason the parser does not own the action list. Each
		// is a link a newer gadak might emit; this version must read it and
		// hand it on, so the handler is the only place that has to learn a
		// new action.
		{
			name: "an action this version does not implement still parses",
			raw:  "gadak://settings?tab=sync",
			want: Link{Action: "settings", Hash: "tab=sync"},
		},
		{
			name: "an action with no query at all",
			raw:  "gadak://setup",
			want: Link{Action: "setup"},
		},
		{
			name: "a hyphenated action",
			raw:  "gadak://new-issue/w/oss",
			want: Link{Action: "new-issue", Profile: "oss"},
		},
		{
			name: "a subject with no profile",
			raw:  "gadak://issue/GDK-119",
			want: Link{Action: "issue", Subject: "GDK-119"},
		},
		{
			name: "a subject after a profile",
			raw:  "gadak://issue/w/oss/GDK-119",
			want: Link{Action: "issue", Profile: "oss", Subject: "GDK-119"},
		},
		{
			name: "a subject after a profile, trailing slash",
			raw:  "gadak://doc/w/oss/SPACE-1234/",
			want: Link{Action: "doc", Profile: "oss", Subject: "SPACE-1234"},
		},
		{
			// A person is addressed by email, so the subject shape has to
			// admit one — reserved now so adding the action later is a table
			// entry and not a grammar change.
			name: "an email subject",
			raw:  "gadak://person/w/oss/dev%2Bqa@example.com",
			want: Link{Action: "person", Profile: "oss", Subject: "dev+qa@example.com"},
		},
		{
			name: "a subject with a query",
			raw:  "gadak://issue/GDK-119?tab=comments",
			want: Link{Action: "issue", Subject: "GDK-119", Hash: "tab=comments"},
		},

		// ── Not ours: ErrNotGadak, the caller stays silent ──
		{
			name:    "the web URL itself is not a deep link",
			raw:     "https://127.0.0.1:7777/w/work/#/?ks=NMA-1,NMA-2",
			wantErr: ErrNotGadak,
		},
		{name: "another app's scheme", raw: "mailto:someone@example.com", wantErr: ErrNotGadak},
		{name: "no scheme at all", raw: "view?x=1", wantErr: ErrNotGadak},
		{name: "empty input", raw: "", wantErr: ErrNotGadak},
		{name: "lookalike scheme", raw: "gadakx://view?x=1", wantErr: ErrNotGadak},

		// ── Ours but malformed: ErrMalformed ──
		{
			// An action must be a plain word, so a link can never name a host.
			name:    "an action that looks like a hostname",
			raw:     "gadak://evil.example.com?x=1",
			wantErr: ErrMalformed,
		},
		{name: "action with port", raw: "gadak://view:7777?x=1", wantErr: ErrMalformed},
		{name: "action with an underscore", raw: "gadak://new_issue?x=1", wantErr: ErrMalformed},
		{name: "no authority", raw: "gadak:view?x=1", wantErr: ErrMalformed},
		{name: "userinfo", raw: "gadak://u@view?x=1", wantErr: ErrMalformed},
		{name: "too many path segments", raw: "gadak://view/w/a/b/c?x=1", wantErr: ErrMalformed},
		{name: "two segments with no /w/", raw: "gadak://view/a/b?x=1", wantErr: ErrMalformed},
		{name: "empty profile after /w/", raw: "gadak://view/w/?x=1", wantErr: ErrMalformed},
		{name: "empty profile, no trailing slash", raw: "gadak://view/w?x=1", wantErr: ErrMalformed},
		{name: "profile may not start with a dot", raw: "gadak://view/w/.hidden?x=1", wantErr: ErrMalformed},
		{name: "profile with a bad character", raw: "gadak://view/w/oss$?x=1", wantErr: ErrMalformed},
		{
			name:    "profile over 64 characters",
			raw:     "gadak://view/w/" + strings.Repeat("p", 65) + "?x=1",
			wantErr: ErrMalformed,
		},
		{name: "subject with a path separator survives as segments", raw: "gadak://issue/a/b?x=1", wantErr: ErrMalformed},
		{name: "subject with a bad character", raw: "gadak://issue/GDK$119?x=1", wantErr: ErrMalformed},

		// Traversal: the one input class that could escape a directory.
		{name: "traversal: dot-dot profile with slash", raw: "gadak://view/w/../?x=1", wantErr: ErrMalformed},
		{name: "traversal: bare dot-dot profile", raw: "gadak://view/w/..?x=1", wantErr: ErrMalformed},
		{name: "traversal: encoded slash after dot-dot", raw: "gadak://view/w/..%2f?x=1", wantErr: ErrMalformed},
		{name: "traversal: uppercase encoded slash", raw: "gadak://view/w/..%2Fetc?x=1", wantErr: ErrMalformed},
		{name: "traversal: fully encoded dot-dot", raw: "gadak://view/w/%2e%2e?x=1", wantErr: ErrMalformed},
		{name: "traversal: half-encoded dot-dot", raw: "gadak://view/w/.%2e?x=1", wantErr: ErrMalformed},
		{name: "traversal: encoded slash adds a segment", raw: "gadak://view/w/oss%2f..?x=1", wantErr: ErrMalformed},
		{name: "traversal: dot-dot subject", raw: "gadak://issue/..?x=1", wantErr: ErrMalformed},
		{name: "traversal: dot-dot subject after a profile", raw: "gadak://issue/w/oss/..?x=1", wantErr: ErrMalformed},

		{name: "fragment attached", raw: "gadak://view?x=1#y", wantErr: ErrMalformed},
		{
			// url.Parse reports an empty fragment no differently from none
			// (measured), so the raw-# check is what rejects this.
			name: "empty fragment marker", raw: "gadak://view?x=1#", wantErr: ErrMalformed,
		},
		{
			// net/url rejects control bytes itself (measured); the case is
			// ours by prefix, so it must surface as malformed, not not-ours.
			name: "control byte in query", raw: "gadak://view?x=1\x01", wantErr: ErrMalformed,
		},
		{name: "input over the 2048-byte limit", raw: overCap, wantErr: ErrMalformed},
		{
			name:    "parameters over the 32-param limit",
			raw:     "gadak://view?" + nParamsQuery(maxParams+1),
			wantErr: ErrMalformed,
		},
		{
			name:    "key over the length limit",
			raw:     "gadak://view?k" + strings.Repeat("a", maxKeyLen) + "=1",
			wantErr: ErrMalformed,
		},
		{name: "uppercase key", raw: "gadak://view?Q=1", wantErr: ErrMalformed},
		{name: "key starting with a digit", raw: "gadak://view?1q=1", wantErr: ErrMalformed},
		{name: "trailing separator makes an empty key", raw: "gadak://view?ks=A-1&", wantErr: ErrMalformed},
		{
			name:    "value over the 512-byte limit",
			raw:     "gadak://view?q=" + strings.Repeat("v", maxValueLen+1),
			wantErr: ErrMalformed,
		},
		{name: "duplicate key", raw: "gadak://view?ks=NMA-1&ks=NMA-2", wantErr: ErrMalformed},
		{
			name:    "duplicate key with others between",
			raw:     "gadak://view?sc=done&q=x&sc=new",
			wantErr: ErrMalformed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Parse(%q) error = %v, want %v", tt.raw, err, tt.wantErr)
				}
				if got != (Link{}) {
					t.Fatalf("error case returned %+v, want the zero Link", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error = %v, want none", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestEmptyQueryIsTheActionsRule pins the split the grammar rests on: a link
// with no query is well-formed, and whether it means anything is the
// handler's call. `view` cannot act without a hash and its handler refuses
// (desktop/deeplink_test.go); `gadak://setup` will carry no hash at all, and
// a parser that rejected it would have to be changed to add that action.
func TestEmptyQueryIsTheActionsRule(t *testing.T) {
	for _, raw := range []string{"gadak://view", "gadak://view?", "gadak://view/w/oss"} {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q) = %v, want a parsed link with an empty hash", raw, err)
		}
		if got.Hash != "" {
			t.Fatalf("Parse(%q) hash = %q, want empty", raw, got.Hash)
		}
	}
}

// TestComposeParseRoundTrip is the contract between the two halves of the
// feature: `gadak views open` emits a link with Compose, and the desktop app
// reads it with Parse. They run in different processes — on different
// machines, at different versions — so a shape change that breaks only one of
// them has nothing else to catch it.
func TestComposeParseRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		action, prefix, hash string
		wantProfile          string
	}{
		{ActionView, "", "ks=NMA-1,NMA-2", ""},
		{ActionView, "/w/work", "ks=NMA-1", "work"},
		{ActionView, "/w/default", "pj=NMA&sc=inprogress", "default"},
		{ActionView, "", "q=%ED%95%9C%EA%B8%80", ""},
		{ActionView, "/w/oss", "f.acceptance_criteria_verification_status=done&s=updated_at&d=desc", "oss"},
		// A value carrying the delimiters, encoded — the case where a
		// composer that re-encoded, or a parser that split and rejoined,
		// would round-trip to something different.
		{ActionView, "", "q=a%26b%3Dc", ""},
		// An action this version does not handle still survives the trip, so
		// a newer gadak's link is readable by this parser.
		{"settings", "/w/oss", "tab=sync", "oss"},
		{"setup", "", "", ""},
	} {
		link := Compose(tc.action, tc.prefix, tc.hash)
		got, err := Parse(link)
		if err != nil {
			t.Fatalf("Compose(%q, %q, %q) = %q, which Parse rejects: %v",
				tc.action, tc.prefix, tc.hash, link, err)
		}
		if got.Action != tc.action {
			t.Fatalf("%q round-tripped to action %q, want %q", link, got.Action, tc.action)
		}
		if got.Profile != tc.wantProfile {
			t.Fatalf("%q round-tripped to profile %q, want %q", link, got.Profile, tc.wantProfile)
		}
		if got.Hash != tc.hash {
			t.Fatalf("%q round-tripped to hash %q, want %q", link, got.Hash, tc.hash)
		}
	}
}

func TestComposeViewWithoutHash(t *testing.T) {
	// A view with no hash has nothing to show, and a link that looks like it
	// works but does nothing is worse than no link — "" is the caller's
	// signal that there is nothing to hand over.
	if got := Compose(ActionView, "/w/work", ""); got != "" {
		t.Fatalf("Compose(view) with an empty hash = %q, want %q", got, "")
	}
}

// TestStdlibURLCaseBehavior pins the net/url behavior Parse's folds rely on
// (measured 2026-08-16, go1.26.4 darwin/arm64): the parser lowercases the
// scheme but preserves host case. If this ever changes, the comments on the
// EqualFold and ToLower calls in Parse change truth with it.
func TestStdlibURLCaseBehavior(t *testing.T) {
	u, err := url.Parse("GADAK://View/w/oss?x=1")
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "gadak" {
		t.Fatalf("scheme %q, want parser-lowercased %q", u.Scheme, "gadak")
	}
	if u.Host != "View" {
		t.Fatalf("host %q, want case preserved %q", u.Host, "View")
	}
}
