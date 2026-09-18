package store

import (
	"context"
	"testing"
)

/*
 * A Latin or digit run glued to CJK on its LEFT disappears from the index
 * (GDK-1978).
 *
 * The tokenizer is `porter unicode61 remove_diacritics 2`, and unicode61
 * counts Han, kana and Hangul as token characters exactly like letters. So a
 * run with no separator before it is not a token at all — it is swallowed into
 * the CJK token that precedes it. Measured with fts5vocab on the three
 * spellings of the same sentence:
 *
 *	INSERT ('追跡issueはNMB-110で、顧客が')      -- ja
 *	INSERT ('추적 이슈는 NMB-110이며 고객이')     -- ko
 *	INSERT ('The tracking issue is NMB-110, and') -- en
 *	SELECT term FROM fts5vocab(t,'row') →
 *	  110 | 110で | 110이며 | and | is | issu | it | nmb | the | track
 *	  追跡issueはnmb | 顧客が | 고객이 | 이슈는 | 추적
 *
 * `nmb` exists for en and ko and not for ja. The query the app sends is
 * `"NMB-110"*` — the phrase [nmb, 110*] — so the `*` guards the RIGHT edge
 * only: `110이며` prefix-matches `110*` and Korean survives, while Japanese
 * dies on the left edge where nothing guards. Korean is not luckier by rule;
 * it is luckier by orthography, because it puts a space before a foreign
 * token. Counting the swallowed runs across the recording fixtures: en 0
 * items, ko 2, ja 334 of 605 — 625 distinct runs no Japanese reader can find.
 *
 * `cjk_bigram` (GDK-259, docs/decisions/0009) solved the mirror image of this:
 * a CJK word inside a longer CJK token. Its own doc comment says non-CJK runes
 * break a run, which is precisely the boundary this defect lives on — the
 * bigram column deliberately emits nothing for the Latin part, and no other
 * column picks it up.
 *
 * The cases below are the discriminating four, not a sample. Left-glued is the
 * defect; right-glued is what `*` already covers; a separator before the run
 * (punctuation, which is NOT a CJK rune) must stay a plain token and must not
 * need rescuing; and English must gain nothing at all, because a fix that
 * widens English recall is a precision regression wearing a bug fix's clothes.
 */

func seedLatinInCJK(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://fixture.invalid"}); err != nil {
		t.Fatal(err)
	}
	b := Batch{
		Categories: fixtureCategories,
		Records: []IssueRecord{
			// The defect, in the spelling measured in the ja fixture's
			// page 131154. Both edges are glued: は before NMB, で after 110.
			newBundle("JA-GLUED", "ワークスペース切り替え", "Bug",
				"追跡issueはNMB-110で、顧客が請求について問い合わせる主な原因の一つになっている"),
			// Korean, same sentence: a space before the key, a particle glued
			// after it. This one already works and must keep working.
			newBundle("KO-SPACED", "워크스페이스 전환", "Bug",
				"추적 이슈는 NMB-110이며 고객이 빌링 문의를 올리는 주요 원인 중 하나다"),
			// English, same sentence. The control.
			newBundle("EN-PLAIN", "Workspace switcher", "Bug",
				"The tracking issue is NMB-110, and it is one of the main reasons customers ask about billing."),
			// Punctuation before the run. `。` is not in cjkRanges and
			// unicode61 treats it as a separator, so NMB is already its own
			// token here — nothing is swallowed and nothing needs emitting.
			newBundle("JA-PUNCT", "リリース手順", "Task",
				"デプロイは完了。NMB-140を参照のこと"),
		},
	}
	if _, err := db.UpsertIssues(context.Background(), b); err != nil {
		t.Fatal(err)
	}
}

// The defect itself: the key is in the body, in Japanese, and searching for it
// finds nothing.
//
// FAIL-first (measured 2026-09-18 on the pre-fix source):
//
//	Search("NMB-110") = [EN-PLAIN KO-SPACED], want JA-GLUED among them
//	Search("issue")   = [EN-PLAIN], want JA-GLUED
func TestSearchFindsAKeyGluedToCJKOnItsLeft(t *testing.T) {
	db := openTemp(t)
	seedLatinInCJK(t, db)

	res, err := db.Search(context.Background(), "NMB-110", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"JA-GLUED", "KO-SPACED", "EN-PLAIN"} {
		if !containsKey(res.Keys, want) {
			t.Errorf("Search(%q) = %v, want %s among them", "NMB-110", res.Keys, want)
		}
	}
	// The same sentence's other swallowed run. `issue` sits between 追跡 and
	// は, so it is inside the same token the key was — one fix has to reach
	// both, or it is a rule about keys rather than about the boundary.
	res, err = db.Search(context.Background(), "issue", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "JA-GLUED") {
		t.Errorf("Search(%q) = %v, want JA-GLUED (issue is swallowed by 追跡issueは)", "issue", res.Keys)
	}
}

// A run that already has a separator before it must not be reached through
// some new path that also reaches things it should not. It is a plain token
// today; this pins that the fix leaves it one.
func TestSearchKeyAfterPunctuationIsUnchanged(t *testing.T) {
	db := openTemp(t)
	seedLatinInCJK(t, db)

	res, err := db.Search(context.Background(), "NMB-140", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "JA-PUNCT") {
		t.Errorf("Search(%q) = %v, want JA-PUNCT", "NMB-140", res.Keys)
	}
	if containsKey(res.Keys, "JA-GLUED") {
		t.Errorf("Search(%q) = %v, must not reach JA-GLUED — it holds NMB-110", "NMB-140", res.Keys)
	}
}

// The precision half, and the reason the emission rule is "adjacent to CJK"
// rather than "every Latin run". An index that carried every Latin run twice
// would double the English posting lists for no recall at all — English text
// has no CJK to be glued to, so the new column must be empty for EN-PLAIN.
// Measured on the fixtures: en contributes 0 items.
func TestLatinRunsAreOnlyRescuedWhereCJKSwallowedThem(t *testing.T) {
	db := openTemp(t)
	seedLatinInCJK(t, db)

	// A word that appears only in the English row must still hit exactly one
	// row: rescuing runs must not give a second posting to text nothing ate.
	res, err := db.Search(context.Background(), "billing", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 1 || !containsKey(res.Keys, "EN-PLAIN") {
		t.Errorf("Search(%q) = %v, want exactly [EN-PLAIN]", "billing", res.Keys)
	}
	// And a key that is in none of the rows stays in none of them.
	res, err = db.Search(context.Background(), "NMB-999", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Keys) != 0 {
		t.Errorf("Search(%q) = %v, want no hits", "NMB-999", res.Keys)
	}
}
