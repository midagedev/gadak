package create

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestMetaForListsAvailableKeys(t *testing.T) {
	catalog := []jira.CreateMetaProject{
		{Key: "STD"}, {Key: "IDEA"},
	}
	_, _, err := MetaFor(catalog, "NOPE", &config.Config{Kind: config.KindStandalone})
	if err == nil {
		t.Fatal("missing project must error")
	}
	for _, want := range []string{"does not exist in this workspace", "available:", "STD", "IDEA"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestFillNeedProjectFromCatalog(t *testing.T) {
	err := FillNeedProject(&NeedProjectError{}, []jira.CreateMetaProject{{Key: "STD"}, {Key: "IDEA"}})
	var np *NeedProjectError
	if !errors.As(err, &np) || strings.Join(np.Configured, ", ") != "STD, IDEA" {
		t.Fatalf("got %v", err)
	}
	kept := FillNeedProject(&NeedProjectError{Configured: []string{"NMA"}}, []jira.CreateMetaProject{{Key: "STD"}})
	if !errors.As(kept, &np) || np.Configured[0] != "NMA" {
		t.Fatalf("must not overwrite configured: %v", kept)
	}
}

// TestMetaForWithCatalogFetchesOnlyOnFallback pins the consolidated
// fallback (GDK-616): MetaFor alone, and only when the filtered answer had
// no keys to list, one site-catalog fetch so the error names what exists.
// CLI create and REST handleCreate both route through this function.
func TestMetaForWithCatalogFetchesOnlyOnFallback(t *testing.T) {
	fetches := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/rest/api/3/issue/createmeta") {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusBadRequest)
			return
		}
		fetches++
		_, _ = w.Write([]byte(`{"projects":[{"key":"STD","name":"Std"},{"key":"IDEA","name":"Ideas"}]}`))
	}))
	t.Cleanup(srv.Close)
	c := jira.New(srv.URL, "someone@example.com", "secret-token")
	cfg := &config.Config{Kind: config.KindStandalone, Projects: []string{"STD", "IDEA"}}

	// Hit: no catalog fetch.
	p, _, err := MetaForWithCatalog(context.Background(), c, []jira.CreateMetaProject{{Key: "STD"}}, "STD", cfg)
	if err != nil || p.Key != "STD" {
		t.Fatalf("hit: key=%q err=%v", p.Key, err)
	}
	if fetches != 0 {
		t.Fatalf("happy path fetched the catalog %d time(s)", fetches)
	}

	// Miss that already lists keys: no catalog fetch either.
	_, _, err = MetaForWithCatalog(context.Background(), c, []jira.CreateMetaProject{{Key: "STD"}}, "NOPE", cfg)
	if err == nil || !strings.Contains(err.Error(), "available: STD") {
		t.Fatalf("miss wording: %v", err)
	}
	if fetches != 0 {
		t.Fatalf("listed miss fetched the catalog %d time(s)", fetches)
	}

	// Filtered-to-empty miss (createmeta on a missing key): one catalog
	// fetch, and the error lists the site keys.
	_, _, err = MetaForWithCatalog(context.Background(), c, nil, "NOPE", cfg)
	if err == nil {
		t.Fatal("empty miss must error")
	}
	for _, want := range []string{"does not exist in this workspace", "available: STD, IDEA"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	if fetches != 1 {
		t.Fatalf("catalog fetches = %d, want 1", fetches)
	}
}
