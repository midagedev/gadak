package create

import (
	"errors"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

func priList() []jira.NamedID {
	return []jira.NamedID{
		{ID: "1", Name: "Highest"},
		{ID: "2", Name: "High"},
		{ID: "3", Name: "Medium"},
	}
}

func TestPriorityMatchesNameAndID(t *testing.T) {
	list := priList()
	cases := []struct {
		want, id string
	}{
		{"High", "2"},
		{"high", "2"},
		{"1", "1"},
		{"  Medium ", "3"},
	}
	for _, tc := range cases {
		id, err := Priority(tc.want, list)
		if err != nil || id != tc.id {
			t.Fatalf("Priority(%q) = %q, %v; want %q", tc.want, id, err, tc.id)
		}
	}
}

func TestPriorityMatchesLocalizedName(t *testing.T) {
	list := []jira.NamedID{
		{ID: "1", Name: "가장 높음"},
		{ID: "2", Name: "높음"},
	}
	id, err := Priority("높음", list)
	if err != nil || id != "2" {
		t.Fatalf("got %q %v", id, err)
	}
	if _, err := Priority("High", list); err == nil {
		t.Fatal("English name must not match a Korean catalog")
	}
}

func TestPriorityEmptyListsCatalog(t *testing.T) {
	_, err := Priority("", priList())
	var need *NeedPriorityError
	if !errors.As(err, &need) {
		t.Fatalf("empty: %v (want NeedPriorityError)", err)
	}
	if !strings.Contains(FormatTypes(need.Available), "Highest (id 1)") {
		t.Fatalf("catalog: %+v", need.Available)
	}
	if strings.Contains(err.Error(), "--") {
		t.Fatalf("shared error named a flag: %v", err)
	}
}

func TestNeedErrorsDoNotNameCLIFlags(t *testing.T) {
	errs := []error{
		func() error { _, e := Project("", &config.Config{}); return e }(),
		func() error { _, e := Project("", &config.Config{Projects: []string{"NMA", "NMB"}}); return e }(),
		func() error { _, e := Type("", priList(), nil, "NMB"); return e }(),
		func() error { _, e := Priority("", priList()); return e }(),
	}
	for i, err := range errs {
		if err == nil {
			t.Errorf("%d: nil error", i)
			continue
		}
		if strings.Contains(err.Error(), "--") {
			t.Errorf("%d: shared error named a flag: %v", i, err)
		}
	}
	var np *NeedProjectError
	if _, err := Project("", &config.Config{Projects: []string{"NMA", "NMB"}}); !errors.As(err, &np) || strings.Join(np.Configured, ",") != "NMA,NMB" {
		t.Fatalf("NeedProjectError: %v", err)
	}
	var nt *NeedTypeError
	if _, err := Type("", priList(), nil, "NMB"); !errors.As(err, &nt) || len(nt.Available) != 3 {
		t.Fatalf("NeedTypeError: %v", err)
	}
}

func TestPriorityUnmatchedListsCatalog(t *testing.T) {
	_, err := Priority("Urgent", priList())
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{`no priority matching "Urgent"`, "Highest (id 1)", "High (id 2)", "Medium (id 3)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestPriorityFieldIsID(t *testing.T) {
	got := PriorityField("2")
	if got["id"] != "2" || len(got) != 1 {
		t.Fatalf("%v", got)
	}
	if _, ok := got["name"]; ok {
		t.Fatalf("name leaked: %v", got)
	}
}

// TestMetaForStandaloneDoesNotAssumeCredential is GDK-390: a missing
// createmeta project on standalone is "not in this workspace", not a
// credential failure. Connected keeps the existing sentence.
//
// FAIL-first: MetaFor always names a credential.
func TestMetaForStandaloneDoesNotAssumeCredential(t *testing.T) {
	_, _, err := MetaFor(nil, "IDEA", &config.Config{Kind: config.KindStandalone})
	if err == nil {
		t.Fatal("missing project must error")
	}
	if strings.Contains(err.Error(), "credential") {
		t.Fatalf("standalone must not assume a credential: %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist in this workspace") {
		t.Fatalf("got %v", err)
	}
}

func TestMetaForConnectedKeepsCredentialMessage(t *testing.T) {
	_, _, err := MetaFor(nil, "IDEA", &config.Config{})
	if err == nil {
		t.Fatal("missing project must error")
	}
	if !strings.Contains(err.Error(), "this credential cannot create issues in IDEA") {
		t.Fatalf("connected wording changed: %v", err)
	}
}
