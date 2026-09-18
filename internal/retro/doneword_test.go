package retro

import "testing"

/*
 * The guards on HasDoneWord, row by row.
 *
 * FAIL-first: every `false` row below returned true against the rule shipped
 * in 4210295f — plain substring containment on the lowercased body. That is
 * what makes this table worth having: each of those bodies says the work is
 * NOT finished, and the signal was pointing at them. Measured 2026-09-06
 * before the guards existed:
 *
 *	"미완료" true · "완료되지 않음" true · "未完了" true · "not fixed" true
 *	"unresolved" true · "unmerged" true · "abandoned" true · "is this done?" true
 *
 * web/src/lib/done-words.test.ts carries the same table; the two copies are
 * lockstep by construction (that test parses this file's word list).
 */
func TestHasDoneWordGuards(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		// claims
		{"Merged and deployed, closing this.", true},
		{"this PR was MERGED a moment ago", true},
		{"작업 완료 — QA까지 확인했습니다", true},
		{"対応済み as of comment 4", true},
		{"已解决，已上线", true},
		{"스테이징에 반영했습니다", true},
		{"resolved the crash in 1.2", true},
		{"배포 완료", true},

		// A Korean done word that is a loanword must stand alone too
		// (GDK-1946). `나머지` (the rest) contains `머지` (merge) and
		// `머지않아` (before long) is its own adjective, neither of which
		// says anything was merged. The guard is per-word and not a blanket
		// left-boundary rule, because the Sino-Korean done words compound:
		// `배포완료됨` is written without a space and is an ordinary done
		// claim on a Korean corporate Jira, so the three claim rows below
		// are as load-bearing as the two false ones.
		{"나머지 리전은 계속 진행 중", false},
		{"칩은 세 개만 표시되고 나머지에 대한 표시는 없음", false},
		{"머지않아 배포합니다", false},
		{"스테이징 매크로 팩에 머지함", true},
		{"배포완료됨", true},
		{"작업완료, QA 넘깁니다", true},

		// English words must stand alone
		{"abandoned this approach", false},
		{"UNDONE — reconsidering the approach", false},
		{"unresolved, still digging", false},
		{"unmerged as of this morning", false},
		{"unfixed", false},

		// English negators just before the word
		{"not fixed yet", false},
		{"not done yet", false},
		{"it isn't fixed", false},
		{"never merged", false},

		// CJK negation, by prefix and by suffix
		{"미완료", false},
		{"아직 미완료입니다", false},
		{"완료되지 않음", false},
		{"未完了のまま", false},
		{"対応済みではない", false},
		{"해결 안 됨", false},

		// a question is not a claim
		{"is this done?", false},
		{"이거 완료됐나요?", false},

		// quoted and fenced text is someone else's word
		{"> done\nnot from my side", false},
		{"```\ndone\n```", false},

		// unchanged behaviour
		{"incomplete", false},
		{"still fixing the edge case", false},
		{"not yet — waiting on review", false},
		{"", false},
		{"   ", false},
	}
	for _, c := range cases {
		if got := HasDoneWord(c.body); got != c.want {
			t.Errorf("HasDoneWord(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

// A negation in a later sentence must not cancel an earlier claim: the
// suffix check is anchored to the text right after the word.
func TestHasDoneWordSuffixIsAnchored(t *testing.T) {
	if !HasDoneWord("완료했습니다. 배포는 아직 되지 않았습니다") {
		t.Error("a later negation must not reach back over a finished claim")
	}
}

/*
 * TestDoneWordPendingClauses closes the second half of GDK-1428: on a Korean
 * corporate Jira the mismatch row measured 44 / 134 / 153 / 201 / 71 hits a
 * week against closures of 54 / 101 / 182 / 244 / 99, while the English OSS
 * mirror stayed at 0–23. A row that noisy takes the whole table's credibility
 * with it.
 *
 * The first narrowing (recency — only a comment newer than the issue's last
 * status change counts) had already landed. What was left is the shape the
 * issue names: a done word inside a subordinate clause is about work that has
 * NOT happened yet. "검토 완료 후 진행하겠습니다" schedules the work; "완료되면
 * 알려주세요" asks to be told. Neither claims anything is finished, and both
 * are ordinary Korean office vocabulary, so they fired every week.
 *
 * The guard is the mirror image of negatedSuffix — anchored right after the
 * word, no byte window — so it stays expressible identically in the
 * TypeScript copy where indices are UTF-16.
 *
 * FAIL-first: every `false` row below returned true against the pre-fix rule.
 */
func TestDoneWordPendingClauses(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		// The clause is about work that has not happened yet.
		{"검토 완료 후 진행하겠습니다", false},
		{"QA 완료 후에 배포합니다", false},
		{"완료되면 알려주세요", false},
		{"완료하면 코멘트 남겨주세요", false},
		{"리뷰 완료 시 머지하겠습니다", false},
		{"완료 예정입니다", false},
		{"이번 주에 완료할 예정", false},
		{"내일까지 완료해야 합니다", false},
		{"완료되는 대로 공유드리겠습니다", false},
		{"배포 전에 다시 확인하겠습니다", false},
		{"完了後にリリースします", false},
		{"完了次第ご連絡します", false},
		{"完了予定です", false},

		// Still claims: the assertion is not a clause about the future.
		{"완료했습니다", true},
		{"작업 완료됐습니다", true},
		{"완료되었습니다, 확인 부탁드립니다", true},
		{"머지 완료", true},
		{"배포 완료했습니다", true},
		{"対応済みです", true},
		{"完了しました", true},

		// The recency guard and the negation guards are untouched by this one.
		{"미완료 상태입니다", false},
		{"완료되지 않았습니다", false},
		{"not fixed yet", false},
		{"Merged and deployed, closing this.", true},
	}
	for _, c := range cases {
		if got := HasDoneWord(c.body); got != c.want {
			t.Errorf("HasDoneWord(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

/*
 * TestDoneWordParticleClauses closes GDK-1943. The release clips record over
 * one fixture translated three ways, and the mismatch row read 0/0/4→9 while
 * every other column agreed — counting comments directly on the three
 * fixtures gave 7 (en) / 36 (ko) / 7 (ja) on the same work. The largest
 * family was one sentence, "지난주 페이지네이션 변경이 반영된 뒤부터 시작됨",
 * duplicated across the fixture's rows: a sequence clause read as a claim
 * because three mechanisms each held part of the rule and disagreed at the
 * edges — the suffix list knew "뒤에" but not "뒤부터", the following-rune
 * guard read 부 as the head of a longer word, and neither ran at all past
 * the adnominal 된 in front of the marker.
 *
 * The fix is one owner (clauseMarkerFollows) with the closed particle class
 * and the bridge into it. FAIL-first: every `false` row below returned true
 * against the pre-fix rule, and the locale gate held the 36-vs-7 gap
 * (doneword_locale_gate_test.go).
 */
func TestDoneWordParticleClauses(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		// A particle after the marker keeps it a clause marker, and the
		// adnominal bridge lets the rule see the marker at all.
		{"지난주 페이지네이션 변경이 반영된 뒤부터 시작됨.", false},
		{"머지된 후에 재시도하겠습니다", false},
		{"반영된 후까지 로그를 확인했습니다", false},
		{"배포 후부터 알림이 두 배로 옵니다", false},
		{"완료 후로 미루겠습니다", false},
		{"검토 완료 시부터 다음 단계입니다", false},

		// A Hangul syllable that is not a particle still heads a longer
		// word — these are the rows that say the class is a particle class,
		// not a script check.
		{"완료된 시점을 기록했습니다", true},
		{"완료 후보 목록을 정리했습니다", true},
		{"머지된 내용을 확인했습니다", true},

		// The bridge without a marker is a completed claim.
		{"스테이징에 반영됐습니다", true},
	}
	for _, c := range cases {
		if got := HasDoneWord(c.body); got != c.want {
			t.Errorf("HasDoneWord(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}
