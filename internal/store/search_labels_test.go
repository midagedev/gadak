package store

import (
	"context"
	"strings"
	"testing"
)

// GDK-1021 recurrence gates: labels are a searchable items_fts column and the
// tokenizer stems English variants. Each test here fails on the pre-GDK-1021
// index shape (4 columns, unicode61) — the label rows are built so nothing but
// the labels column can match, and the stem rows so nothing but a stemmer can
// close the singular/plural gap.

// seedLabelOnly builds rows whose ONLY payments signal is a label, plus the
// ranking family for one shared needle: LT title-only, LL labels-only, LB one
// body occurrence, LC one comment occurrence.
func seedLabelOnly(t *testing.T, db *DB) {
	t.Helper()
	if err := db.UpsertSource(context.Background(), Source{ID: "jira", Kind: "jira", BaseURL: "https://fixture.invalid"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSource(context.Background(), Source{ID: "confluence", Kind: "confluence", BaseURL: "https://fixture.invalid/wiki"}); err != nil {
		t.Fatal(err)
	}
	issue := func(id, key, title, body string, labels []string) IssueRecord {
		return IssueRecord{
			Item: Item{
				ID: "jira:" + id, SourceID: "jira", Kind: "issue", ExternalID: id,
				Key: key, Title: title, BodyText: body,
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Issue: Issue{
				ProjectKey: "LAB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
				Labels: labels,
			},
		}
	}
	recs := []IssueRecord{
		// The label-only row: "payments" appears nowhere in title/body/comments.
		issue("lab-1", "LAB-1", "Card statement rendering",
			"Quarterly statement PDF misses pages.", []string{"payments"}),
		// Stem variants, one each direction. porter("payments") == porter("payment"),
		// so each doc matches the other's query form; unicode61 cannot.
		issue("lab-s", "LAB-S", "Charge retry loop",
			"The payment pipeline drops the second attempt.", nil),
		issue("lab-p", "LAB-P", "Chargeback window",
			"Duplicate payments created one invoice twice.", nil),
		// Korean mid-compound in a label: 결제 exists only inside 간편결제 —
		// the title deliberately carries no 결제 of its own.
		issue("lab-ko", "LAB-KO", "모바일 오류 보고",
			"재현 절차를 정리했다.", []string{"간편결제"}),
		// Ranking family — one shared needle, one field each.
		issue("lab-lt", "LAB-LT", "RankNeedleLMN lives only in this title",
			"Generic description with no ranking token.", nil),
		issue("lab-ll", "LAB-LL", "Ordinary summary without the ranking token",
			"Generic description with no ranking token.", []string{"rankneedlelmn"}),
		issue("lab-lb", "LAB-LB", "Ordinary summary without the ranking token",
			"One body occurrence of RankNeedleLMN only.", nil),
		{
			Item: Item{
				ID: "jira:lab-lc", SourceID: "jira", Kind: "issue", ExternalID: "lab-lc",
				Key: "LAB-LC", Title: "Ordinary summary without the ranking token",
				BodyText:  "Generic description with no ranking token.",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			},
			Issue: Issue{
				ProjectKey: "LAB", IssueType: "Bug", IssueTypeID: "10004",
				Status: "To Do", StatusID: "1", StatusCategory: "new",
			},
			Comments: []Comment{{
				ID: "jira:lab-lc1", ExternalID: "lab-lc1", Author: "Ada",
				BodyText:  "Comment carries RankNeedleLMN once.",
				CreatedAt: ago(1), UpdatedAt: ago(1),
			}},
		},
	}
	if _, err := db.UpsertIssues(context.Background(), Batch{Categories: fixtureCategories, Records: recs}); err != nil {
		t.Fatal(err)
	}
	// Page with a label-only signal for "billing".
	if _, err := db.UpsertPages(context.Background(), []PageRecord{{
		Item: Item{
			ID: "confluence:9101", SourceID: "confluence", Kind: "page", ExternalID: "9101",
			Key: "9101", Title: "Operations handbook", BodyText: "Escalation paths and on-call rota.",
			Author: "Pat", CreatedAt: ago(2), UpdatedAt: ago(1),
		},
		Page: Page{SpaceKey: "ENG", Version: 1, Status: "current", Labels: []string{"billing-runbook"}},
	}}); err != nil {
		t.Fatal(err)
	}
}

// The GDK-1021 headline: an issue whose only "payments" signal is a label is
// found, and --json's matches attribute it to the labels field.
func TestSearchFindsLabelOnlyIssue(t *testing.T) {
	db := openTemp(t)
	seedLabelOnly(t, db)

	res, err := db.Search(context.Background(), "payments", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "LAB-1") {
		t.Errorf("Search(payments) = %v, want LAB-1 (label-only hit — labels column missing from items_fts)", res.Keys)
	}
	m, ok := res.Matches["LAB-1"]
	if !ok {
		t.Fatalf("matches missing LAB-1: %+v", res.Matches)
	}
	if m.Field != "labels" {
		t.Errorf("LAB-1 field = %q, want labels", m.Field)
	}
	if !strings.Contains(m.Snippet, "payments") {
		t.Errorf("LAB-1 snippet = %q, want the label text in it", m.Snippet)
	}
}

// Stem variants both ways: payments→payment and payment→payments.
func TestSearchStemVariantsMatchBothWays(t *testing.T) {
	db := openTemp(t)
	seedLabelOnly(t, db)

	sing, err := db.Search(context.Background(), "payments", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(sing.Keys, "LAB-S") {
		t.Errorf("Search(payments) = %v, want LAB-S (body says 'payment' — stemmer missing)", sing.Keys)
	}
	plural, err := db.Search(context.Background(), "payment", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(plural.Keys, "LAB-P") {
		t.Errorf("Search(payment) = %v, want LAB-P (body says 'payments' — stemmer missing)", plural.Keys)
	}
}

// A Korean label participates in mid-compound search: 결제 inside the 간편결제
// label is found via the cjk_bigram projection of the labels text.
func TestSearchKoreanLabelMidCompound(t *testing.T) {
	db := openTemp(t)
	seedLabelOnly(t, db)

	res, err := db.Search(context.Background(), "결제", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(res.Keys, "LAB-KO") {
		t.Errorf("Search(결제) = %v, want LAB-KO (결제 only inside the 간편결제 label)", res.Keys)
	}
	if m, ok := res.Matches["LAB-KO"]; !ok || m.Field != "labels" {
		t.Errorf("LAB-KO match = %+v (ok=%v), want field labels", m, ok)
	}
}

// Page labels are searched too — the labels column carries both projections.
func TestSearchFindsPageByLabel(t *testing.T) {
	db := openTemp(t)
	seedLabelOnly(t, db)

	res, err := db.Search(context.Background(), "billing-runbook", 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range res.Pages {
		if p.Key == "9101" {
			found = true
		}
	}
	if !found {
		t.Errorf("Search(billing-runbook) pages = %+v, want page 9101 (label-only page hit)", res.Pages)
	}
}

// Ranking contract for the labels column (ftsBM25Labels = 8.0): one label hit
// sits below a title hit and above a body hit and a comment hit of the same
// term. Measured 2026-09-10; if this fails after renumbering, re-measure — do
// not nudge the weight to make one ordering pass.
func TestSearchRelevanceLabelBetweenTitleAndBody(t *testing.T) {
	db := openTemp(t)
	seedLabelOnly(t, db)

	res, err := db.Search(context.Background(), "RankNeedleLMN", 20)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, k := range res.Keys {
		switch k {
		case "LAB-LT", "LAB-LL", "LAB-LB", "LAB-LC":
			got = append(got, k)
		}
	}
	want := "LAB-LT,LAB-LL,LAB-LB,LAB-LC"
	if strings.Join(got, ",") != want {
		t.Fatalf("field order = %v (all keys %v), want %v (title > labels > body > comment)", got, res.Keys, want)
	}
}
