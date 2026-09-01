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

func typeCatalog() []jira.CreateMetaIssueType {
	return []jira.CreateMetaIssueType{
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
		func() error { _, e := Type("", typeCatalog(), nil, "NMB"); return e }(),
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
	if _, err := Type("", typeCatalog(), nil, "NMB"); !errors.As(err, &nt) || len(nt.Available) != 3 {
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

// TestMetaForLocalOriginDoesNotAssumeCredential is GDK-390: a missing
// createmeta project on local-origin is "not in this workspace", not a
// credential failure. Connected keeps the existing sentence.
//
// FAIL-first: MetaFor always names a credential.
func TestMetaForLocalOriginDoesNotAssumeCredential(t *testing.T) {
	_, _, err := MetaFor(nil, "IDEA", &config.Config{Kind: config.KindLocalOrigin})
	if err == nil {
		t.Fatal("missing project must error")
	}
	if strings.Contains(err.Error(), "credential") {
		t.Fatalf("local-origin must not assume a credential: %v", err)
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
	_, _, err := MetaFor(catalog, "NOPE", &config.Config{Kind: config.KindLocalOrigin})
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
	cfg := &config.Config{Kind: config.KindLocalOrigin, Projects: []string{"STD", "IDEA"}}

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

// koreanSiteTypes is the createmeta catalog measured 2026-08-26 against the
// live GDK site (gadak --profile oss api GET /rest/api/3/issue/createmeta
// --query projectKeys=GDK). Names were authored in Korean, so name equals
// untranslatedName; 기능 and 요청 are site inventions, not Jira built-ins.
func koreanSiteTypes() []jira.CreateMetaIssueType {
	return []jira.CreateMetaIssueType{
		{ID: "10001", Name: "에픽", UntranslatedName: "에픽", HierarchyLevel: 1},
		{ID: "10002", Name: "하위 작업", UntranslatedName: "하위 작업", Subtask: true, HierarchyLevel: -1},
		{ID: "10003", Name: "작업", UntranslatedName: "작업"},
		{ID: "10004", Name: "스토리", UntranslatedName: "스토리"},
		{ID: "10005", Name: "기능", UntranslatedName: "기능"},
		{ID: "10006", Name: "요청", UntranslatedName: "요청"},
		{ID: "10007", Name: "버그", UntranslatedName: "버그"},
	}
}

// TestTypeEnglishBugMatchesKoreanCatalog is GDK-741: --type Bug on a site
// whose types are named in Korean must resolve to id 10007, not a round-trip.
//
// FAIL-first (unmodified matchType):
// Type(Bug) on Korean catalog: no issue type matching "Bug" — available: 에픽 (id 10001); 하위 작업 (id 10002); 작업 (id 10003); 스토리 (id 10004); 기능 (id 10005); 요청 (id 10006); 버그 (id 10007)
func TestTypeEnglishBugMatchesKoreanCatalog(t *testing.T) {
	got, err := Type("Bug", koreanSiteTypes(), nil, "STD")
	if err != nil {
		t.Fatalf("Type(Bug) on Korean catalog: %v", err)
	}
	if got.Value != "10007" {
		t.Fatalf("issuetype id = %q, want 10007 (resolved %+v)", got.Value, got)
	}
	if got.Source != SourceAlias {
		t.Fatalf("source = %q, want %s", got.Source, SourceAlias)
	}
}

// TestTypeAliasAmbiguousRefuses is GDK-741: two catalog types that both
// match one alias must error naming both, not pick the first.
//
// FAIL-first (unmodified matchType): must refuse as a collision, not a miss:
// "no issue type matching \"Bug\" — available: 버그 (id 10007); 버그 (id 10017)"
func TestTypeAliasAmbiguousRefuses(t *testing.T) {
	types := []jira.CreateMetaIssueType{
		{ID: "10007", Name: "버그"},
		{ID: "10017", Name: "버그"},
	}
	_, err := Type("Bug", types, nil, "STD")
	if err == nil {
		t.Fatal("two 버그 types matching Bug must not pick one")
	}
	msg := err.Error()
	for _, want := range []string{"10007", "10017", "버그", "more than one", "an id settles it"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ambiguous error %q missing %q", msg, want)
		}
	}
	if strings.Contains(msg, "no issue type matching") {
		t.Errorf("must refuse as a collision, not a miss: %q", msg)
	}
	if strings.Contains(msg, "--") {
		t.Errorf("shared error named a flag: %v", err)
	}
}

func TestTypeStructuralEpicAndSubtask(t *testing.T) {
	cat := koreanSiteTypes()
	cases := []struct {
		want, id string
	}{
		{"epic", "10001"},
		{"Epic", "10001"},
		{"subtask", "10002"},
		{"sub-task", "10002"},
		{"sub task", "10002"},
		{"Sub-Task", "10002"},
	}
	for _, tc := range cases {
		got, err := Type(tc.want, cat, nil, "STD")
		if err != nil {
			t.Fatalf("Type(%q): %v", tc.want, err)
		}
		if got.Value != tc.id || got.Source != SourceAlias {
			t.Fatalf("Type(%q) = %+v, want id=%s source=%s", tc.want, got, tc.id, SourceAlias)
		}
	}
}

func TestTypeDoesNotInventSiteTypes(t *testing.T) {
	cat := koreanSiteTypes()
	for _, want := range []string{"Request", "Feature", "요청아님"} {
		_, err := Type(want, cat, nil, "STD")
		if err == nil {
			t.Errorf("%q must not invent a site type", want)
			continue
		}
		if !strings.Contains(err.Error(), "no issue type matching") {
			t.Errorf("%q: %v", want, err)
		}
	}
	// Display names still match at step 2.
	got, err := Type("기능", cat, nil, "STD")
	if err != nil || got.Value != "10005" || got.Source != SourceFlag {
		t.Fatalf("local name 기능: %+v %v", got, err)
	}
}

func TestTypeUntranslatedNameWhenDifferent(t *testing.T) {
	types := []jira.CreateMetaIssueType{
		{ID: "10007", Name: "버그", UntranslatedName: "Bug"},
	}
	got, err := Type("Bug", types, nil, "STD")
	if err != nil || got.Value != "10007" || got.Source != SourceAlias {
		t.Fatalf("untranslatedName: %+v %v", got, err)
	}
	// Equal to name: step 3 does not fire; step 2 already matched, source is flag.
	same := []jira.CreateMetaIssueType{
		{ID: "10007", Name: "버그", UntranslatedName: "버그"},
	}
	got, err = Type("버그", same, nil, "STD")
	if err != nil || got.Value != "10007" || got.Source != SourceFlag {
		t.Fatalf("name==untranslatedName must stay flag: %+v %v", got, err)
	}
}

func TestTypeNameAndIDStayFlagNotAlias(t *testing.T) {
	cat := koreanSiteTypes()
	got, err := Type("10007", cat, nil, "STD")
	if err != nil || got.Value != "10007" || got.Source != SourceFlag {
		t.Fatalf("id: %+v %v", got, err)
	}
	got, err = Type("버그", cat, nil, "STD")
	if err != nil || got.Value != "10007" || got.Source != SourceFlag {
		t.Fatalf("name: %+v %v", got, err)
	}
	// A catalog that already has English Bug must keep resolving "Bug" to
	// that type (step 2), not to a later locale alias.
	mixed := []jira.CreateMetaIssueType{
		{ID: "10004", Name: "Bug"},
		{ID: "10007", Name: "버그"},
	}
	got, err = Type("Bug", mixed, nil, "STD")
	if err != nil || got.Value != "10004" || got.Source != SourceFlag {
		t.Fatalf("existing English name must not move to alias: %+v %v", got, err)
	}
}

func TestTypeUnmatchedMentionsLocalisedAndID(t *testing.T) {
	_, err := Type("Nope", koreanSiteTypes(), nil, "STD")
	if err == nil {
		t.Fatal("expected unmatched")
	}
	msg := err.Error()
	for _, want := range []string{
		`no issue type matching "Nope"`,
		"type names follow the site's own language",
		"an id always works",
		"에픽 (id 10001)",
		"버그 (id 10007)",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	if strings.Contains(msg, "--") {
		t.Errorf("shared error named a flag: %v", err)
	}
}

func TestTypeStructuralAmbiguousRefuses(t *testing.T) {
	// Two types at level 1. The fixture used to be 에픽@1 + 이니셔티브@2,
	// which was ambiguous only because "epic" matched `>= 1`; under `== 1`
	// that pair resolves to 에픽, which is the right answer and no longer
	// exercises this contract. Two level-1 types is the collision the test
	// was written for.
	types := []jira.CreateMetaIssueType{
		{ID: "10001", Name: "에픽", HierarchyLevel: 1},
		{ID: "10012", Name: "대형 작업", HierarchyLevel: 1},
	}
	_, err := Type("epic", types, nil, "STD")
	if err == nil {
		t.Fatal("two level-1 types must not pick one")
	}
	msg := err.Error()
	for _, want := range []string{"10001", "10012", "에픽", "대형 작업", "more than one"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in %q", want, msg)
		}
	}
}

func TestTypeStructuralEpicIsLevelOneNotAnyParent(t *testing.T) {
	// Jira's Epic is hierarchy level 1; 2 and above are the premium parent
	// tiers. A project whose only parent-level type is Initiative must not
	// answer `--type epic` with it — that is the wrong-type outcome the
	// collision rule exists to prevent, arriving by a different door.
	only := []jira.CreateMetaIssueType{
		{ID: "10003", Name: "작업"},
		{ID: "10011", Name: "이니셔티브", HierarchyLevel: 2},
	}
	if got, err := Type("epic", only, nil, "STD"); err == nil {
		t.Fatalf("a sole level-2 type answered --type epic with %q", got.Value)
	}

	// And with a real Epic present, the deeper tier does not make it
	// ambiguous: level 1 is the only candidate.
	both := append([]jira.CreateMetaIssueType{{ID: "10001", Name: "에픽", HierarchyLevel: 1}}, only...)
	got, err := Type("epic", both, nil, "STD")
	if err != nil {
		t.Fatalf("Epic alongside Initiative: %v", err)
	}
	if got.Value != "10001" || got.Source != SourceAlias {
		t.Fatalf("got %+v; want 10001 via %s", got, SourceAlias)
	}
}

func TestTypeLocaleAliasWhenNoHierarchy(t *testing.T) {
	// 에픽 at level 0: step 4 misses, step 5 still maps Epic.
	types := []jira.CreateMetaIssueType{
		{ID: "10001", Name: "에픽"},
		{ID: "10003", Name: "작업"},
	}
	got, err := Type("Epic", types, nil, "STD")
	if err != nil || got.Value != "10001" || got.Source != SourceAlias {
		t.Fatalf("step-5 epic: %+v %v", got, err)
	}
	got, err = Type("Task", types, nil, "STD")
	if err != nil || got.Value != "10003" || got.Source != SourceAlias {
		t.Fatalf("step-5 task: %+v %v", got, err)
	}
}

func TestTypeStoryAlias(t *testing.T) {
	got, err := Type("story", koreanSiteTypes(), nil, "STD")
	if err != nil || got.Value != "10004" || got.Source != SourceAlias {
		t.Fatalf("%+v %v", got, err)
	}
}
