package create

import (
	"errors"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/origin"
)

// The write-gap round's pure-resolution gates (2026-09-10). GDK-1733's issue
// body names this package as the fix point for the paired-workspace defaults,
// so the decision rules live here and the CLI test file pins only the wiring.

func TestSoleCatalogProject(t *testing.T) {
	cases := []struct {
		name    string
		catalog []origin.CreateMetaProject
		wantKey string
		wantOK  bool
	}{
		{"one project is the default", []origin.CreateMetaProject{{Key: "STD"}}, "STD", true},
		{"two projects still refuse", []origin.CreateMetaProject{{Key: "NMB"}, {Key: "GDK"}}, "", false},
		{"empty catalog refuses", nil, "", false},
		{"blank key refuses", []origin.CreateMetaProject{{Key: "  "}}, "", false},
	}
	for _, tc := range cases {
		res, ok := SoleCatalogProject(tc.catalog)
		if ok != tc.wantOK {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if res.Value != tc.wantKey || res.Source != SourceCatalog {
			t.Errorf("%s: %+v, want value %q source %q", tc.name, res, tc.wantKey, SourceCatalog)
		}
	}
}

func TestParentCatalogProject(t *testing.T) {
	catalog := []origin.CreateMetaProject{{Key: "NMB"}, {Key: "GDK"}}

	res, ok := ParentCatalogProject("NMB-1", catalog)
	if !ok || res.Value != "NMB" || res.Source != SourceParent {
		t.Fatalf("parent NMB-1 → %+v ok=%v, want NMB/"+SourceParent, res, ok)
	}
	// Case-insensitive both ways: a lowercase prefix names the same project.
	if res, ok := ParentCatalogProject("nmb-42", catalog); !ok || res.Value != "NMB" {
		t.Fatalf("parent nmb-42 → %+v ok=%v, want NMB", res, ok)
	}
	// A parent key outside the catalog does not invent a project.
	if _, ok := ParentCatalogProject("ZZZ-1", catalog); ok {
		t.Fatal("unknown prefix must not resolve")
	}
	// A bare word is not an issue key; the choice stays open.
	if _, ok := ParentCatalogProject("NMB", catalog); ok {
		t.Fatal("dashless parent value must not resolve")
	}
	if _, ok := ParentCatalogProject("", catalog); ok {
		t.Fatal("empty parent must not resolve")
	}
}

func TestAmbiguousTypeErrorIsTyped(t *testing.T) {
	types := []origin.CreateMetaIssueType{
		{ID: "10000", Name: "Epic"}, {ID: "10005", Name: "Epic"},
	}
	_, _, err := matchType("Epic", types)
	var amb *AmbiguousTypeError
	if !errors.As(err, &amb) {
		t.Fatalf("err = %v (%T), want *AmbiguousTypeError", err, err)
	}
	if amb.Want != "Epic" || len(amb.Hits) != 2 {
		t.Fatalf("AmbiguousTypeError = %+v, want Want=Epic Hits=2", amb)
	}
	// The sentence the CLI prints is unchanged — a surface that formats the
	// error must not gain or lose a word.
	for _, want := range []string{"more than one catalog type", "an id settles it"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err %q missing %q", err, want)
		}
	}

	// One hit is not ambiguous: the match resolves as before.
	id, src, err := matchType("Epic", types[:1])
	if err != nil || id != "10000" || src != SourceFlag {
		t.Fatalf("single hit → %q %s %v", id, src, err)
	}
}
