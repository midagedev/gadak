package deeplink

/*
 * GDK-873: the gadak:// grammar has one owner (this package) and now two
 * implementations — the Go parser here, and the phone's parseGadakUrl in
 * mobile/src/lib/deeplink.ts, which cannot import Go.
 *
 * A second implementation of a grammar is exactly the drift this repo has
 * already paid for once (protocol-version.ts carried in two copies). The
 * fix is the same one internal/pairing uses for offer vectors and
 * internal/server uses for the REST goldens: the owner emits a table, the
 * other implementation consumes the same bytes, and one commit turns both
 * suites red.
 *
 * The table is emitted from THIS parser, so it is never hand-maintained:
 *
 *	go test ./internal/deeplink/ -run TestGrammarVectors -update
 *
 * Add a case to grammarVectors, regenerate, and the phone's test picks it
 * up on the next run with no edit on that side. A case whose expectation
 * someone "fixes" by hand in the JSON is overwritten on the next -update,
 * which is the point: the Go parser is the definition.
 *
 * The phone deliberately does NOT reimplement the action registry (that is
 * desktop/deeplink.go's job on the desk, and the phone's own screen table
 * on the phone) — only the grammar, which is the half that must never
 * change because links outlive installs.
 */

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var vectorsUpdate = flag.Bool("update", false, "rewrite testdata/vectors.json from this parser")

// vector is one input and what the owning parser makes of it. Outcome is
// "ok", "not_gadak", or "malformed" — the three answers Parse gives, named
// so the phone can assert the classification and not just the failure.
// Callers must be able to tell not_gadak from malformed: one is silence,
// the other is a message.
type vector struct {
	Name    string `json:"name"`
	Input   string `json:"input"`
	Outcome string `json:"outcome"`
	Action  string `json:"action,omitempty"`
	Profile string `json:"profile,omitempty"`
	Subject string `json:"subject,omitempty"`
	Hash    string `json:"hash,omitempty"`
	// Why records the rule that fired, for a human reading a red diff. Not
	// asserted across languages — the messages are Go's wording.
	Why string `json:"why,omitempty"`
}

// grammarVectors is the shared table. Every case here is a rule in
// deeplink.go's package comment; the names are what a failure prints on
// both sides.
var grammarVectors = []struct{ name, input string }{
	// ── the shapes that must work ──
	{"view with a hash", "gadak://view?issue=GDK-119"},
	{"view on a named profile", "gadak://view/w/oss?issue=GDK-119&pj=GDK"},
	{"view with several axes", "gadak://view?pj=GDK&sc=inprogress&q=crash"},
	{"a discovered-field axis", "gadak://view?f.requirement_11461=yes"},
	{"the action folds case", "gadak://VIEW?issue=GDK-119"},
	{"one trailing slash before the query", "gadak://view/w/oss/?issue=GDK-119"},
	{"a bare subject", "gadak://issue/GDK-119"},
	{"a profile and a subject", "gadak://issue/w/oss/GDK-119"},
	{"an unknown action still parses", "gadak://timeline?issue=GDK-119"},
	{"no query at all", "gadak://setup"},
	{"the default profile is a name like any other", "gadak://view/w/default?issue=GDK-119"},
	{"percent-encoding is passed through untouched", "gadak://view?q=one%20two"},
	{"a parameter with no value", "gadak://view?issue="},

	// ── not ours: the caller stays silent ──
	{"another app's scheme", "myapp://view?issue=GDK-119"},
	{"an https link", "https://example.com/browse/GDK-119"},
	{"empty input", ""},
	{"a bare issue key", "GDK-119"},

	// ── ours, and refused ──
	{"the opaque form has no action", "gadak:view?issue=GDK-119"},
	{"a port is not part of an action", "gadak://view:7777?issue=GDK-119"},
	{"userinfo is not allowed", "gadak://user@view?issue=GDK-119"},
	{"a fragment is not allowed", "gadak://view?issue=GDK-119#panel"},
	{"a bare trailing hash is still a fragment", "gadak://view?issue=GDK-119#"},
	{"an action with a dot could be a hostname", "gadak://view.example.com?issue=GDK-119"},
	{"an uppercase-only action is folded, but an underscore is not a word", "gadak://view_all?issue=GDK-119"},
	{"an empty profile after /w/", "gadak://view/w/?issue=GDK-119"},
	{"a traversal as the profile", "gadak://view/w/../x?issue=GDK-119"},
	{"a traversal as the subject", "gadak://issue/..?x=1"},
	{"a dot as the subject", "gadak://issue/.?x=1"},
	{"an encoded separator cannot smuggle a segment", "gadak://issue/a%2Fb?x=1"},
	{"more path segments than the grammar allows", "gadak://view/w/oss/GDK-119/extra?x=1"},
	{"two segments without the /w/ introducer", "gadak://view/a/b?x=1"},
	{"a duplicate parameter key", "gadak://view?issue=GDK-119&issue=GDK-120"},
	{"an uppercase parameter key", "gadak://view?Issue=GDK-119"},
	{"a parameter key starting with a digit", "gadak://view?1issue=GDK-119"},
	{"a control byte in the query", "gadak://view?q=a\x01b"},
	{"over the input length limit", "gadak://view?q=" + strings.Repeat("a", maxInputLen)},
	{"over the value length limit", "gadak://view?q=" + strings.Repeat("a", maxValueLen+1)},
	{"over the parameter count limit", "gadak://view?" + manyParams(maxParams+1)},
	{"over the key length limit", "gadak://view?" + strings.Repeat("k", maxKeyLen+1) + "=1"},
}

// manyParams builds n distinct well-formed parameters.
func manyParams(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString("k")
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString(string(rune('a' + i/26)))
		b.WriteString("=1")
	}
	return b.String()
}

func vectorsPath() string { return filepath.Join("testdata", "vectors.json") }

// TestGrammarVectors runs the table through Parse and compares against the
// committed JSON — the same bytes mobile/src/lib/deeplink.test.ts reads.
func TestGrammarVectors(t *testing.T) {
	got := make([]vector, 0, len(grammarVectors))
	for _, c := range grammarVectors {
		v := vector{Name: c.name, Input: c.input}
		link, err := Parse(c.input)
		switch {
		case err == nil:
			v.Outcome = "ok"
			v.Action, v.Profile, v.Subject, v.Hash = link.Action, link.Profile, link.Subject, link.Hash
		case errors.Is(err, ErrNotGadak):
			v.Outcome = "not_gadak"
		case errors.Is(err, ErrMalformed):
			v.Outcome = "malformed"
			v.Why = strings.TrimPrefix(err.Error(), ErrMalformed.Error()+": ")
		default:
			t.Fatalf("%s: Parse returned an error of no known class: %v", c.name, err)
		}
		got = append(got, v)
	}
	want, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *vectorsUpdate {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vectorsPath(), want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(vectorsPath())
	if err != nil {
		t.Fatalf("vectors missing (%v). The phone's parser reads this file; regenerate with\n\n\tgo test ./internal/deeplink/ -run TestGrammarVectors -update", err)
	}
	if !bytes.Equal(have, want) {
		t.Fatalf("the gadak:// grammar moved.\nmobile/src/lib/deeplink.ts parses the same table, so regenerating this file WILL turn the phone suite red until that parser agrees — which is the point.\nRegenerate with\n\n\tgo test ./internal/deeplink/ -run TestGrammarVectors -update\n\ncommitted:\n%s\nthis parser now says:\n%s", have, want)
	}
}

// TestGrammarVectorsCoverBothOutcomes keeps the table from decaying into a
// happy-path list: a vectors file with no refusals would let the phone
// accept anything and stay green.
func TestGrammarVectorsCoverBothOutcomes(t *testing.T) {
	data, err := os.ReadFile(vectorsPath())
	if err != nil {
		t.Fatal(err)
	}
	var vs []vector
	if err := json.Unmarshal(data, &vs); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, v := range vs {
		counts[v.Outcome]++
	}
	for _, outcome := range []string{"ok", "not_gadak", "malformed"} {
		if counts[outcome] < 3 {
			t.Errorf("only %d %q vectors; the table must exercise every outcome or the phone's parser can be wrong and green", counts[outcome], outcome)
		}
	}
}
