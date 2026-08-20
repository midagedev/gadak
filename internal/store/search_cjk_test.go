package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// GDK-259 / docs/decisions/0009: CJK mid-compound search via a cjk_bigram
// fourth column. The fixture isolates the compound case — KO-MID's title
// carries 결제 only inside 간편결제, so a token-start (prefix) MATCH cannot
// find it and a passing test proves the bigram path, not an accidental
// standalone token. The previous round contaminated this exact fixture with a
// standalone 결제 token; every row here is written to keep the isolation.
func seedCJKCompound(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://fixture.invalid"}); err != nil {
		t.Fatal(err)
	}
	b := Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			{
				// 결제 appears nowhere in this row except inside the compound.
				Item: Item{
					ID: "jira:ko-mid", SourceID: "jira", Kind: "issue", ExternalID: "ko-mid",
					Key:       "KO-MID",
					Title:     "간편결제 실패",
					BodyText:  "모바일 웹에서 카드 등록 직후 재현된다",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KO", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:ko-pre", SourceID: "jira", Kind: "issue", ExternalID: "ko-pre",
					Key:       "KO-PRE",
					Title:     "결제내역 조회 오류",
					BodyText:  "월별 결제내역 보고서가 비어 있다",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KO", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				Item: Item{
					ID: "jira:en-idem", SourceID: "jira", Kind: "issue", ExternalID: "en-idem",
					Key:       "EN-IDEM",
					Title:     "idempotency key handling fails",
					BodyText:  "Duplicate webhook events create duplicate charges.",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "EN", IssueType: "Bug", IssueTypeID: "10004",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
			{
				// Space-separated tokens: reachable with explicit FTS5 syntax
				// on any index shape, so the operator pass-through has a row
				// to find both before and after the rewrite change.
				Item: Item{
					ID: "jira:ko-op", SourceID: "jira", Kind: "issue", ExternalID: "ko-op",
					Key:       "KO-OP",
					Title:     "알림 채널 개선",
					BodyText:  "웹훅 재시도 정책을 문서화했다",
					CreatedAt: ago(1), UpdatedAt: ago(1),
				},
				Issue: Issue{
					ProjectKey: "KO", IssueType: "Task", IssueTypeID: "10002",
					Status: "To Do", StatusID: "1", StatusCategory: "new",
				},
			},
		},
	}
	if _, err := db.UpsertIssues(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

// Korean mid-compound: 결제 is the tail of the single token 간편결제, so the
// prefix rewrite misses it (FAIL-first on a 3-column items_fts).
func TestSearchCJKMidCompound(t *testing.T) {
	db := openTemp(t)
	seedCJKCompound(t, db)

	res, err := db.Search(context.Background(), "결제", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KO-MID") {
		t.Errorf("Search(%q) = %v, want KO-MID (결제 inside 간편결제 — mid-compound)", "결제", res.Keys)
	}
	// Field attribution must survive the bigram path: without a CJK
	// containment check in ftsColumnPrefixHit the row is returned but its
	// match (field + snippet) is omitted.
	m, ok := res.Matches["KO-MID"]
	if !ok {
		t.Fatalf("matches missing KO-MID: %+v", res.Matches)
	}
	if m.Field != "title" {
		t.Errorf("KO-MID field = %q, want title", m.Field)
	}
}

// The prefix behavior that existed before the bigram column must not regress:
// 결제 still token-start hits 결제내역.
func TestSearchCJKPrefixStillHits(t *testing.T) {
	db := openTemp(t)
	seedCJKCompound(t, db)

	res, err := db.Search(context.Background(), "결제", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KO-PRE") {
		t.Errorf("Search(%q) = %v, want KO-PRE (결제내역 token-start)", "결제", res.Keys)
	}
}

// English prefix path is untouched by the CJK rewrite.
func TestSearchCJKFixtureEnglishPrefix(t *testing.T) {
	db := openTemp(t)
	seedCJKCompound(t, db)

	res, err := db.Search(context.Background(), "idempot", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "EN-IDEM") {
		t.Errorf("Search(%q) = %v, want EN-IDEM", "idempot", res.Keys)
	}
}

// English infix stays broken on purpose (0009 §4 A-en3/B: `ency` precision
// 0.30–0.34 was measured and rejected). A-col must not grow this.
func TestSearchEnglishInfixStaysEmpty(t *testing.T) {
	db := openTemp(t)
	seedCJKCompound(t, db)

	res, err := db.Search(context.Background(), "ency", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 0 {
		t.Errorf("Search(%q) = %v, want 0 hits (English infix precision lock)", "ency", res.Keys)
	}
}

// One CJK rune keeps today's token-start rewrite — an exact-"결" rewrite was
// measured to return 0 against a bigram index (0009 §2d) and would be a
// silent regression; this is that guard. Note the kept "결"* also matches
// bigram tokens that START with the rune (cjk_bigram's 결제), so a 1-rune
// query recalls compounds too — inherent to A-col, and no 0009 §3c sensor
// constrains 1-rune precision. Asserting KO-MID absent here would encode a
// contract the design does not make.
func TestSearchCJKSingleRuneTokenStart(t *testing.T) {
	db := openTemp(t)
	seedCJKCompound(t, db)

	res, err := db.Search(context.Background(), "결", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KO-PRE") {
		t.Errorf("Search(%q) = %v, want KO-PRE (token-start 결제내역 — exact-%q rewrite would return 0)", "결", res.Keys, "결")
	}
}

// Explicit FTS5 syntax passes through the rewrite untouched, on the same path
// as before.
func TestSearchCJKOperatorPassthrough(t *testing.T) {
	db := openTemp(t)
	seedCJKCompound(t, db)

	res, err := db.Search(context.Background(), "웹훅 AND 재시도", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KO-OP") {
		t.Errorf("Search(%q) = %v, want KO-OP (space-separated tokens, no rewrite)", "웹훅 AND 재시도", res.Keys)
	}
}

// The rewrite shapes ftsPrefixQuery produces, pinned directly.
func TestFTSPrefixQueryCJK(t *testing.T) {
	if got, want := ftsPrefixQuery("간편결제"), `"간편" "편결" "결제"`; got != want {
		t.Errorf("ftsPrefixQuery(간편결제) = %q, want %q (AND of overlapping bigrams)", got, want)
	}
	if got, want := ftsPrefixQuery("결제"), `"결제"`; got != want {
		t.Errorf("ftsPrefixQuery(결제) = %q, want %q (single bigram, exact)", got, want)
	}
	if got, want := ftsPrefixQuery("결"), `"결"*`; got != want {
		t.Errorf("ftsPrefixQuery(결) = %q, want %q (1-rune CJK keeps token-start)", got, want)
	}
	if got, want := ftsPrefixQuery("idempot"), `"idempot"*`; got != want {
		t.Errorf("ftsPrefixQuery(idempot) = %q, want %q", got, want)
	}
	if got, want := ftsPrefixQuery("웹훅 AND 재시도"), `웹훅 AND 재시도`; got != want {
		t.Errorf("ftsPrefixQuery(operator query) = %q, want %q (passthrough)", got, want)
	}
	if got, want := ftsPrefixQuery(`"quoted" passthrough`), `"quoted" passthrough`; got != want {
		t.Errorf("ftsPrefixQuery(quoted) = %q, want %q (passthrough)", got, want)
	}
	// Mixed scripts are not all-CJK terms: keep the prefix form.
	if got, want := ftsPrefixQuery("결제API"), `"결제API"*`; got != want {
		t.Errorf("ftsPrefixQuery(결제API) = %q, want %q (mixed term keeps prefix)", got, want)
	}
}

// The bigram helper itself: runs, run boundaries, and the one-rune rule.
func TestCJKBigrams(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"간편결제", []string{"간편", "편결", "결제"}}, // overlapping
		{"결제", []string{"결제"}},               // one bigram
		{"결", nil},                           // 1-rune run emits nothing (unigrams rejected)
		{"간편 결제", []string{"간편", "결제"}},      // space breaks the run
		{"간편결제API재시도", []string{"간편", "편결", "결제", "재시", "시도"}}, // Latin breaks the run
		{"idempotency key", nil}, // no CJK
		{"", nil},
		{"결제. 결제!", []string{"결제", "결제"}}, // CJK punctuation breaks runs
	}
	for _, c := range cases {
		got := cjkBigrams(c.in)
		if len(got) != len(c.want) {
			t.Errorf("cjkBigrams(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("cjkBigrams(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
	if got, want := FTSCJKBigramColumn("간편결제", "no cjk here", "재시도한다"), "간편 편결 결제 재시 시도 도한 한다"; got != want {
		t.Errorf("FTSCJKBigramColumn = %q, want %q", got, want)
	}
	if got := FTSCJKBigramColumn("", "", ""); got != "" {
		t.Errorf("FTSCJKBigramColumn(empty) = %q, want empty", got)
	}
	if !isCJKTerm("간편결제") || isCJKTerm("결제API") || isCJKTerm("") {
		t.Error("isCJKTerm: wholly-CJK terms only")
	}
}

// Field attribution follows the index: CJK bigram containment is a column
// hit (so mid-compound rows keep their snippet), English mid-token is not.
func TestFTSColumnPrefixHitCJK(t *testing.T) {
	if !ftsColumnPrefixHit("간편결제 실패", "결제") {
		t.Error("결제 vs 간편결제: bigram containment must count as a hit")
	}
	if !ftsColumnPrefixHit("간편결제 실패", "편결") {
		t.Error("편결 vs 간편결제: mid-run fragment bigram must hit")
	}
	// Scattered bigrams hit the MATCH too (each is a token in cjk_bigram);
	// attribution mirrors the index rather than requiring one substring.
	if !ftsColumnPrefixHit("간편 토끼 편결 나비 결제", "간편결제") {
		t.Error("간편결제 vs scattered bigrams: attribution must mirror what MATCH can hit")
	}
	// English mid-token stays a non-hit (TestFTSColumnPrefixHit pins the rest).
	if ftsColumnPrefixHit("idempotency key handling", "ency") {
		t.Error("ency vs idempotency: English mid-token must stay a non-hit (precision lock)")
	}
	// 1-rune CJK is not a bigram query: token-start only.
	if ftsColumnPrefixHit("간편결제 실패", "결") {
		t.Error("결 vs 간편결제: 1-rune CJK is not bigram containment")
	}
	if !ftsColumnPrefixHit("결제내역 조회", "결") {
		t.Error("결 vs 결제내역: 1-rune token-start must still hit")
	}
}

// The rebuild INSERT must fill the fourth column (0009's named trap: a
// 3-column rebuild leaves CJK mid-match silently empty until the next
// per-row write). Also pins the "stranger opens the file" contract —
// items_fts MATCH '결제' hits without any gadak-side rewriting.
func TestRebuildItemsFTSKeepsCJKMidMatch(t *testing.T) {
	db := openTemp(t)
	seedCJKCompound(t, db)

	var hits int
	if err := db.sql.QueryRow(`SELECT count(*) FROM items_fts WHERE items_fts MATCH '결제'`).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits < 2 {
		t.Fatalf("raw MATCH '결제' = %d hits, want ≥2 (KO-MID + KO-PRE) before rebuild", hits)
	}
	if _, err := db.rebuildItemsFTS(context.Background()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if err := db.sql.QueryRow(`SELECT count(*) FROM items_fts WHERE items_fts MATCH '결제'`).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if hits < 2 {
		t.Errorf("raw MATCH '결제' = %d hits after rebuild, want ≥2 — rebuild dropped cjk_bigram", hits)
	}
	res, err := db.Search(context.Background(), "결제", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "KO-MID") || !containsKey(res.Keys, "KO-PRE") {
		t.Errorf("Search(결제) after rebuild = %v, want KO-MID and KO-PRE", res.Keys)
	}
}

// The 0009 §3c precision ceiling, measured on a throwaway copy of the
// committed examples/demo.db (never the original): Open migrates it to the
// current level and rebuilds items_fts into the cjk_bigram shape; English
// behavior must come out byte-identical. Rebuild wall time is logged, not
// gated — 0009 measured 13.5 ms at this size.
func TestDemoPrecisionGateCJKColumn(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "demo.db"))
	if err != nil {
		t.Skipf("examples/demo.db not present: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gadak.db")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open snapshot copy (migrate + FTS rebuild): %v", err)
	}
	defer db.Close()
	elapsed := time.Since(start)

	var uv int
	if err := db.sql.QueryRow(`PRAGMA user_version`).Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if uv != len(migrations) {
		t.Errorf("user_version = %d, want %d (schemaV25 must move the documented level)", uv, len(migrations))
	}
	var schemaVer int
	if err := db.sql.QueryRow(`SELECT schema_version FROM sync_state LIMIT 1`).Scan(&schemaVer); err != nil {
		t.Fatalf("sync_state read: %v", err)
	}
	if schemaVer != len(migrations) {
		t.Errorf("sync_state.schema_version = %d, want %d", schemaVer, len(migrations))
	}
	var hasBigramCol bool
	err = db.sql.QueryRow(`SELECT COUNT(*) > 0 FROM pragma_table_info('items_fts') WHERE name = 'cjk_bigram'`).Scan(&hasBigramCol)
	if err != nil || !hasBigramCol {
		t.Fatalf("items_fts lacks cjk_bigram after repair rebuild (err=%v)", err)
	}

	for q, want := range map[string]int{
		"ency":              0,  // English infix lock: A-col must not grow this
		"idempot*":          14, // prefix unchanged
		"webhook AND retry": 16, // operator query unchanged
		"auth*":             35, // prefix set must not grow
	} {
		var got int
		if err := db.sql.QueryRow(`SELECT count(*) FROM items_fts WHERE items_fts MATCH ?`, q).Scan(&got); err != nil {
			t.Fatalf("MATCH %q: %v", q, err)
		}
		if got != want {
			t.Errorf("demo MATCH %q = %d hits, want %d", q, got, want)
		}
	}
	t.Logf("demo copy (%d items) migrated v15→v%d and items_fts rebuilt in %s", itemCount(t, db), len(migrations), elapsed)
}

func itemCount(t *testing.T, db *DB) int {
	t.Helper()
	var n int
	if err := db.sql.QueryRow(`SELECT count(*) FROM items`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
