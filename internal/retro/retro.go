// Package retro computes the weekly retrospective docs/project/THEORY.md
// calls its instrument: one table, read from the mirror plus local.db,
// definitions printed under the numbers every time. Six rows: sessions,
// resume (median), wip age p85, in progress, closed, mismatch. Columns are
// ISO weeks (Monday 00:00 local, oldest left, the current partial week last)
// and a change column against the previous week. Everything is a read: the
// mirror, the status_catalog lookup, and local.visits; nothing here writes
// or touches a network. Statuses are keyed by id through status_catalog and
// by status_category — never by the display name beside them.
//
// Degradation is a dash, never an error: an empty status_catalog blanks the
// weeks only the changelog can answer, an unparseable timestamp drops one
// row, and a workspace with no self identity resolves resume against any
// author on the issues the session visited (the footer says so).
//
// Every count carries the issue keys behind it (Bucket.ClosedKeys and
// friends), so a number is never a dead end: the CLI opens a cell's keys in
// the running app and the server serves the same document. Key slices are
// collected in lockstep with each count increment — a count is the length
// of its slice by construction, mismatch included, where two matching
// comments on one issue leave the key twice.
package retro

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

// MaxSinceDays caps --since at one year of weeks.
const MaxSinceDays = 365

// SessionGap splits person reads into sessions: a counted visit more
// than this after the previous one starts a new session. Exactly the gap is
// still the same session — strictly greater, so a test can sit on the line.
// Exported because the server's session strip (internal/server/read.go)
// walks the same boundary — one owner for the rule.
const SessionGap = 30 * time.Minute

// MaxJSONKeys caps each key array of the JSON document. It matches
// internal/jql MaxKeys, the ceiling a keys view may open, so a cell that
// fits the document also fits the app.
const MaxJSONKeys = 500

// DoneWords is the mismatch heuristic: a comment containing one of these
// on an issue that is not done now. A heuristic, not a parser — the row says
// so in its definition, and the words cover the languages this workspace
// writes (English, Korean, Japanese, Chinese).
//
// Matching is HasDoneWord's job, not a plain Contains: an English word must
// stand on its own (2026-09-06 — "abandoned" contains "done", "unresolved"
// contains "resolved"), and a CJK word must not be carrying a negation
// prefix ("미완료" contains "완료", "未完了" contains "完了"). Both classes
// pointed the signal backwards, at exactly the comments saying the work is
// NOT finished.
var DoneWords = []string{
	"done", "fixed", "merged", "resolved", "shipped",
	"완료", "해결", "머지", "반영", "배포",
	"完了", "修正済み", "対応済み",
	"已完成", "已解决", "已修复",
}

// negationPrefixes are the one-character CJK negations that glue to the
// front of a done word and reverse it. Korean and Japanese negate this way,
// so a prefix check is the whole guard for those languages.
var negationPrefixes = []rune{'미', '未', '불', '非', '无', '無'}

// negationSuffixes follow a done word and reverse it at a distance: Korean
// "완료되지 않았다", Japanese "対応済みではない". Only the text immediately
// after the match is examined, so a later sentence cannot cancel an earlier
// claim.
var negationSuffixes = []string{"되지 않", "하지 않", "지 않", "안 됨", "안됨", "ではない", "されていない", "していない", "ていない"}

// englishNegators cancel an English done word when they sit just before it:
// "not fixed", "isn't done", "never merged".
var englishNegators = []string{"not", "n't", "no", "never", "isn't", "wasn't", "aren't", "yet"}

var sinceRe = regexp.MustCompile(`^([0-9]+)([dw])$`)

// ParseSince accepts <N>d or <N>w, 1..365 days equivalent. Anything else
// is a usage error (the caller wraps it), never a silent default.
func ParseSince(s string) (time.Duration, error) {
	m := sinceRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("--since wants <N>d or <N>w (for example 14d, 30d, 8w), 1 to %d days", MaxSinceDays)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("--since wants a number of days or weeks, 1 or more")
	}
	days := n
	if m[2] == "w" {
		days = 7 * n
	}
	if days > MaxSinceDays {
		return 0, fmt.Errorf("--since is capped at %d days (got %d)", MaxSinceDays, days)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

/* ── buckets ── */

// Bucket is one ISO week column: From is Monday 00:00 local; the last
// bucket runs to now and is partial. The metric fields are per bucket:
// pointers are nil where the answer does not exist (no data for it), which
// the table prints as a dash and JSON emits as null.
type Bucket struct {
	From    time.Time
	To      time.Time
	Partial bool

	Sessions int
	Resume   *float64 // median seconds to the first own write
	ResumeK  int      // sessions in this bucket that had a write
	ResumeN  int      // sessions in this bucket
	WipP85   *float64 // days, nearest-rank 85th percentile
	InProg   *int
	Closed   *int
	Mismatch int

	// The issue keys behind the counts, sorted by key, one entry per
	// counted item (mismatch: per counted comment), so each count is the
	// length of its slice.
	ClosedKeys     []string
	InProgressKeys []string
	MismatchKeys   []string
}

// Label is the bucket's column title and the name --open reports:
// 08-24..08-31, or 08-31..now for the current partial week.
func (b Bucket) Label() string {
	to := b.To.Format("01-02")
	if b.Partial {
		to = "now"
	}
	return b.From.Format("01-02") + ".." + to
}

// monday is the Monday 00:00 (in t location) of the ISO week holding t.
func monday(t time.Time) time.Time {
	y, m, d := t.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	off := (int(day.Weekday()) + 6) % 7 // Sunday reads as 6 days back
	return day.AddDate(0, 0, -off)
}

// Buckets lays the weeks from the Monday of the first week since reaches
// up to now. At least one bucket comes back: an empty span (now exactly on
// a Monday midnight) still gets the single partial week.
func Buckets(now time.Time, since time.Duration) []Bucket {
	b := monday(now.Add(-since))
	var out []Bucket
	for ; b.Before(now); b = b.AddDate(0, 0, 7) {
		out = append(out, Bucket{From: b})
	}
	if len(out) == 0 {
		out = append(out, Bucket{From: monday(now.Add(-since))})
	}
	for i := range out {
		if i+1 < len(out) {
			out[i].To = out[i+1].From
		}
	}
	last := &out[len(out)-1]
	last.To = now
	last.Partial = true
	return out
}

/* ── loaded rows ── */

type visit struct {
	kind   string
	key    string
	source string
	at     time.Time
}

type write struct {
	at       time.Time
	item     string
	author   string
	authorID string
}

type comment struct {
	write
	body string
}

type statusRow struct {
	at     time.Time
	fromID string
	toID   string
	seq    string // changelog id: deterministic tie-break for equal at
}

type item struct {
	id       string
	sourceID string
	kind     string
	key      string
}

// Report is everything the table and the JSON document render.
type Report struct {
	Buckets      []Bucket
	CLIFallback  bool // no ui/unknown visits; sessions came from cli rows
	SelfResolved bool // a FeedIdentity was available for resume
	CatalogEmpty bool // status_catalog has no rows
}

/* ── loading and computing ── */

// epochSQL is store.currentEpochSQL (internal/store/origin_scope.go),
// unexported there: the visits read runs on the mirror handle where local.db
// is the attached schema, so the epoch clause needs the local. prefix. Keep
// the two in lockstep — a visit recorded under a retired origin must not
// count as a session in this one (GDK-418).
const epochSQL = `COALESCE((SELECT CAST(v AS INTEGER) FROM local.local_meta WHERE k = 'origin_epoch'), 0)`

// parseTime is the timestamp ladder for every column this package reads:
// ISOMilli first (the stores own format), then RFC 3339 variants, then the
// bare shapes an older sync might have left. Unparseable reads as absent —
// the row drops, the report never errors on one value.
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{config.ISOMilli, time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Compute reads the mirror plus local.visits and fills the buckets. db is
// a handle on the mirror with local.db attached (store.OpenReadOnly gives
// the CLI one; the server opens its own the same way).
func Compute(ctx context.Context, db *sql.DB, me store.FeedIdentity, since time.Duration, now time.Time) (Report, error) {
	rep := Report{Buckets: Buckets(now, since)}
	rep.SelfResolved = me != store.FeedIdentity{}
	first := rep.Buckets[0].From

	// Visits: epoch-scoped, prefiltered by day prefix (ISOMilli starts with
	// the date, so a string bound is a real bound), then filtered exactly on
	// parsed times — bucket edges are local midnight and the strings are UTC.
	dayPrefix := first.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	var visits []visit
	vrows, err := db.QueryContext(ctx, `SELECT COALESCE(kind,''), COALESCE(key,''), COALESCE(viewed_at,''), COALESCE(source,'')
		FROM local.visits WHERE origin_epoch = `+epochSQL+` AND viewed_at >= ?`, dayPrefix)
	if err != nil {
		return rep, err
	}
	for vrows.Next() {
		var kind, key, at, source string
		if err := vrows.Scan(&kind, &key, &at, &source); err != nil {
			vrows.Close()
			return rep, err
		}
		t, ok := parseTime(at)
		// Keep one session gap before the first bucket: a read inside the
		// window that follows a read just outside it by less than the gap is
		// the same session, not a new one at the window edge. Sessions that
		// start before the first bucket are dropped by the bucket loop.
		if !ok || t.Before(first.Add(-SessionGap)) || t.After(now) {
			continue
		}
		visits = append(visits, visit{kind: kind, key: key, source: source, at: t})
	}
	if err := vrows.Err(); err != nil {
		vrows.Close()
		return rep, err
	}
	vrows.Close()

	// Items and issues: the key<->item and item<->category maps every other
	// pass walks.
	itemByID := map[string]item{}
	keyItems := map[string][]string{}
	irows, err := db.QueryContext(ctx, `SELECT id, COALESCE(source_id,''), COALESCE(kind,''), COALESCE(key,'') FROM items`)
	if err != nil {
		return rep, err
	}
	for irows.Next() {
		var it item
		if err := irows.Scan(&it.id, &it.sourceID, &it.kind, &it.key); err != nil {
			irows.Close()
			return rep, err
		}
		itemByID[it.id] = it
		keyItems[it.key] = append(keyItems[it.key], it.id)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return rep, err
	}
	irows.Close()

	issCategory := map[string]string{}
	issChangedAt := map[string]string{}
	srows, err := db.QueryContext(ctx, `SELECT item_id, COALESCE(status_category,''), COALESCE(status_changed_at,'') FROM issues`)
	if err != nil {
		return rep, err
	}
	for srows.Next() {
		var id, cat, changed string
		if err := srows.Scan(&id, &cat, &changed); err != nil {
			srows.Close()
			return rep, err
		}
		issCategory[id] = cat
		issChangedAt[id] = changed
	}
	if err := srows.Err(); err != nil {
		srows.Close()
		return rep, err
	}
	srows.Close()

	// status_catalog: status id -> category, scoped per source because two
	// sources can reuse one id.
	cat := map[string]string{}
	crows, err := db.QueryContext(ctx, `SELECT COALESCE(source_id,''), COALESCE(status_id,''), COALESCE(category,'') FROM status_catalog`)
	if err != nil {
		return rep, err
	}
	for crows.Next() {
		var source, id, category string
		if err := crows.Scan(&source, &id, &category); err != nil {
			crows.Close()
			return rep, err
		}
		cat[source+"\x00"+id] = category
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return rep, err
	}
	crows.Close()
	rep.CatalogEmpty = len(cat) == 0

	// Status changelog, whole history before now: wip age and closed walk it
	// backwards from each week end.
	statusByItem := map[string][]statusRow{}
	clog, err := db.QueryContext(ctx, `SELECT item_id, COALESCE(id,''), COALESCE(at,''), COALESCE(from_id,''), COALESCE(to_id,'')
		FROM changelog WHERE field = 'status'`)
	if err != nil {
		return rep, err
	}
	for clog.Next() {
		var item, id, at, fromID, toID string
		if err := clog.Scan(&item, &id, &at, &fromID, &toID); err != nil {
			clog.Close()
			return rep, err
		}
		t, ok := parseTime(at)
		if !ok || t.After(now) {
			continue
		}
		statusByItem[item] = append(statusByItem[item], statusRow{at: t, fromID: fromID, toID: toID, seq: id})
	}
	if err := clog.Err(); err != nil {
		clog.Close()
		return rep, err
	}
	clog.Close()
	for item := range statusByItem {
		rows := statusByItem[item]
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].at.Equal(rows[j].at) {
				return rows[i].seq < rows[j].seq
			}
			return rows[i].at.Before(rows[j].at)
		})
		statusByItem[item] = rows
	}

	// Writes (any changelog field) and comments, window-prefiltered the same
	// way visits were: resume needs them between session starts, mismatch
	// needs comment bodies inside each week.
	writePrefix := first.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	var writes []write
	var comments []comment
	wrows, err := db.QueryContext(ctx, `SELECT item_id, COALESCE(at,''), COALESCE(author,''), COALESCE(author_id,'')
		FROM changelog WHERE at >= ?`, writePrefix)
	if err != nil {
		return rep, err
	}
	for wrows.Next() {
		var w write
		var at string
		if err := wrows.Scan(&w.item, &at, &w.author, &w.authorID); err != nil {
			wrows.Close()
			return rep, err
		}
		t, ok := parseTime(at)
		if !ok || t.Before(first) || t.After(now) {
			continue
		}
		w.at = t
		writes = append(writes, w)
	}
	if err := wrows.Err(); err != nil {
		wrows.Close()
		return rep, err
	}
	wrows.Close()

	mrows, err := db.QueryContext(ctx, `SELECT item_id, COALESCE(created_at,''), COALESCE(author,''), COALESCE(author_id,''), COALESCE(body_text,'')
		FROM comments WHERE created_at >= ?`, writePrefix)
	if err != nil {
		return rep, err
	}
	for mrows.Next() {
		var c comment
		var at string
		if err := mrows.Scan(&c.item, &at, &c.author, &c.authorID, &c.body); err != nil {
			mrows.Close()
			return rep, err
		}
		t, ok := parseTime(at)
		if !ok || t.Before(first) || t.After(now) {
			continue
		}
		c.at = t
		comments = append(comments, c)
	}
	if err := mrows.Err(); err != nil {
		mrows.Close()
		return rep, err
	}
	mrows.Close()

	sort.SliceStable(writes, func(i, j int) bool { return writes[i].at.Before(writes[j].at) })
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].at.Before(comments[j].at) })

	/* sessions */

	counted := make([]visit, 0, len(visits))
	for _, v := range visits {
		if v.source == store.VisitSourceUI || v.source == "" {
			counted = append(counted, v)
		}
	}
	if len(counted) == 0 {
		for _, v := range visits {
			if v.source == store.VisitSourceCLI {
				counted = append(counted, v)
			}
		}
		rep.CLIFallback = len(counted) > 0
	}
	sort.SliceStable(counted, func(i, j int) bool { return counted[i].at.Before(counted[j].at) })

	type session struct {
		start     time.Time
		issueKeys map[string]bool
	}
	var sessions []session
	for i, v := range counted {
		newSession := i == 0 || v.at.Sub(counted[i-1].at) > SessionGap
		if newSession {
			sessions = append(sessions, session{start: v.at, issueKeys: map[string]bool{}})
		}
		if v.kind == store.VisitKindIssue {
			sessions[len(sessions)-1].issueKeys[v.key] = true
		}
	}

	for bi := range rep.Buckets {
		b := &rep.Buckets[bi]
		var resumes []float64
		for si := range sessions {
			s := sessions[si]
			if s.start.Before(b.From) || !s.start.Before(b.To) {
				continue
			}
			b.Sessions++
			b.ResumeN++
			end := now
			if si+1 < len(sessions) {
				end = sessions[si+1].start
			}
			visited := map[string]bool{}
			for key := range s.issueKeys {
				for _, id := range keyItems[key] {
					visited[id] = true
				}
			}
			if secs, ok := sessionResume(writes, comments, visited, s.start, end, me, rep.SelfResolved); ok {
				resumes = append(resumes, secs)
				b.ResumeK++
			}
		}
		if med, ok := Median(resumes); ok {
			b.Resume = &med
		}
	}

	/* wip age p85, in progress, closed, mismatch */

	for bi := range rep.Buckets {
		b := &rep.Buckets[bi]
		if b.Partial {
			// The current week answers from the issues rows themselves:
			// status_category and status_changed_at are live, no catalog
			// needed. Closed still needs the changelog below.
			count := 0
			var ages []float64
			for item := range issCategory {
				if issCategory[item] != store.CategoryInProgress {
					continue
				}
				count++
				b.InProgressKeys = append(b.InProgressKeys, itemByID[item].key)
				if t, ok := parseTime(issChangedAt[item]); ok && !t.After(now) {
					ages = append(ages, now.Sub(t).Hours()/24)
				}
			}
			n := count
			b.InProg = &n
			if p, ok := P85(ages); ok {
				b.WipP85 = &p
			}
		} else if !rep.CatalogEmpty {
			// A finished week answers from the changelog: the last status row
			// at or before week end, mapped through status_catalog.
			var ages []float64
			for item, rows := range statusByItem {
				it, ok := itemByID[item]
				if !ok {
					continue
				}
				if lastStatusCategory(rows, cat, it.sourceID, b.To) != store.CategoryInProgress {
					continue
				}
				if at, ok := lastStatusAt(rows, b.To); ok {
					ages = append(ages, b.To.Sub(at).Hours()/24)
					b.InProgressKeys = append(b.InProgressKeys, it.key)
				}
			}
			n := len(ages)
			b.InProg = &n
			if p, ok := P85(ages); ok {
				b.WipP85 = &p
			}
		}

		if !rep.CatalogEmpty {
			closed := map[string]bool{}
			for item, rows := range statusByItem {
				it, ok := itemByID[item]
				if !ok {
					continue
				}
				for _, r := range rows {
					if r.at.Before(b.From) || !r.at.Before(b.To) {
						continue
					}
					if cat[it.sourceID+"\x00"+r.toID] == store.CategoryDone &&
						cat[it.sourceID+"\x00"+r.fromID] != store.CategoryDone {
						closed[item] = true
					}
				}
			}
			n := len(closed)
			b.Closed = &n
			for item := range closed {
				b.ClosedKeys = append(b.ClosedKeys, itemByID[item].key)
			}
		}

		for _, c := range comments {
			if c.at.Before(b.From) || !c.at.Before(b.To) {
				continue
			}
			if issCategory[c.item] == "" || issCategory[c.item] == store.CategoryDone {
				continue // not an issue, or done now
			}
			if HasDoneWord(c.body) {
				b.Mismatch++
				b.MismatchKeys = append(b.MismatchKeys, itemByID[c.item].key)
			}
		}
		sort.Strings(b.ClosedKeys)
		sort.Strings(b.InProgressKeys)
		sort.Strings(b.MismatchKeys)
	}
	return rep, nil
}

// sessionResume walks the merged write streams in time order for the
// first write inside [start, end) that belongs to the session: with a self
// identity only the configured account counts, without one any author counts
// on the condition the write lands on an issue the session visited.
func sessionResume(writes []write, comments []comment, visitedItems map[string]bool,
	start, end time.Time, me store.FeedIdentity, selfResolved bool) (float64, bool) {
	i, j := 0, 0
	for i < len(writes) || j < len(comments) {
		var w write
		var at time.Time
		switch {
		case j >= len(comments):
			w, at = writes[i], writes[i].at
			i++
		case i >= len(writes):
			w, at = comments[j].write, comments[j].at
			j++
		case !writes[i].at.After(comments[j].at):
			w, at = writes[i], writes[i].at
			i++
		default:
			w, at = comments[j].write, comments[j].at
			j++
		}
		if at.Before(start) {
			continue
		}
		if !at.Before(end) {
			return 0, false // walked past the session window: no own write in it
		}
		var mine bool
		if selfResolved {
			mine = store.IsSelfActor(me, w.authorID, w.author)
		} else {
			mine = visitedItems[w.item]
		}
		if mine {
			return at.Sub(start).Seconds(), true
		}
	}
	return 0, false
}

// lastStatusAt is the newest status row at or before end.
func lastStatusAt(rows []statusRow, end time.Time) (time.Time, bool) {
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].at.After(end) {
			return rows[i].at, true
		}
	}
	return time.Time{}, false
}

// lastStatusCategory maps that newest row to its category through the
// catalog. An unmapped id reads as the empty category — not in progress.
func lastStatusCategory(rows []statusRow, cat map[string]string, sourceID string, end time.Time) string {
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].at.After(end) {
			return cat[sourceID+"\x00"+rows[i].toID]
		}
	}
	return ""
}

// HasDoneWord reports whether a comment claims the work is finished.
//
// It is a guarded match, not a substring test. Plain containment read every
// negation as its opposite — measured on the shipped rule, "미완료",
// "완료되지 않음", "未完了", "not fixed", "unresolved", "unmerged",
// "abandoned" and "is this done?" all came back true, and each of those is a
// comment saying the work is NOT done. The guards, in the order they run:
//
//   - a question is not a claim: a body whose last sentence ends in "?" or
//     "？" is rejected outright;
//   - quoted lines (Markdown "> ") and fenced code blocks are stripped —
//     quoting someone else's "done" is not saying it;
//   - an English word must stand alone (word boundaries), so "abandoned"
//     and "unresolved" no longer carry "done" and "resolved";
//   - a CJK word must not carry a negation prefix (미 未 불 非 无 無) or be
//     followed by a negation ("되지 않", "ではない").
//
// The result is a candidate, never a fact (THEORY.md T5). It is precise
// enough for an affordance that costs one dismissal when wrong; a count
// shown to a steward needs more than this.
func HasDoneWord(body string) bool {
	text := stripQuotedAndCode(body)
	if strings.TrimSpace(text) == "" {
		return false
	}
	if endsWithQuestion(text) {
		return false
	}
	low := strings.ToLower(text)
	for _, w := range DoneWords {
		if isASCIIWord(w) {
			if matchEnglishWord(low, w) {
				return true
			}
			continue
		}
		if matchCJKWord(text, w) {
			return true
		}
	}
	return false
}

// stripQuotedAndCode removes Markdown quote lines and fenced code blocks.
// A comment that quotes an earlier "done" is reporting, not claiming.
func stripQuotedAndCode(body string) string {
	var out []string
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(trimmed, ">") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// endsWithQuestion reports whether the body's last non-empty sentence is a
// question. "is this done?" asks; it does not claim.
func endsWithQuestion(text string) bool {
	t := strings.TrimRight(strings.TrimSpace(text), " \t)]\"'”’")
	return strings.HasSuffix(t, "?") || strings.HasSuffix(t, "？")
}

// isASCIIWord distinguishes the English vocabulary from the CJK one: the
// two need different match rules, and the word itself says which it is.
func isASCIIWord(w string) bool {
	for _, r := range w {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// matchEnglishWord finds w standing on its own — letters and digits on
// either side disqualify — and rejects it when a negator sits just before.
func matchEnglishWord(low, w string) bool {
	for i := 0; i+len(w) <= len(low); i++ {
		if low[i:i+len(w)] != w {
			continue
		}
		if i > 0 && isWordByte(low[i-1]) {
			continue
		}
		if end := i + len(w); end < len(low) && isWordByte(low[end]) {
			continue
		}
		if englishNegatedBefore(low[:i]) {
			continue
		}
		return true
	}
	return false
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// englishNegatedBefore looks at the last few words before a match. Three is
// the window: "not yet fixed" and "is not fixed" both negate, a sentence
// two clauses back does not.
func englishNegatedBefore(before string) bool {
	fields := strings.Fields(before)
	if len(fields) > 3 {
		fields = fields[len(fields)-3:]
	}
	for _, f := range fields {
		f = strings.Trim(f, ".,;:!?()[]\"'")
		for _, n := range englishNegators {
			if f == n || (strings.HasSuffix(n, "n't") && strings.HasSuffix(f, "n't")) {
				return true
			}
		}
	}
	return false
}

// matchCJKWord finds w and rejects it when a negation prefix precedes it or
// a negation follows it. Case is untouched: ToLower is a no-op on CJK.
func matchCJKWord(text, w string) bool {
	for i := 0; i+len(w) <= len(text); i++ {
		if text[i:i+len(w)] != w {
			continue
		}
		if negatedPrefix(text[:i]) {
			continue
		}
		if negatedSuffix(text[i+len(w):]) {
			continue
		}
		return true
	}
	return false
}

func negatedPrefix(before string) bool {
	r, _ := utf8.DecodeLastRuneInString(before)
	if r == utf8.RuneError {
		return false
	}
	for _, n := range negationPrefixes {
		if r == n {
			return true
		}
	}
	return false
}

// negatedSuffix looks at the text immediately after the word, anchored: the
// negation has to start right there (spaces aside), so "완료되지 않았다" is
// caught and a negation in the next sentence is never borrowed. Anchoring is
// the whole bound — no byte window, which is also what keeps this rule
// expressible identically in the TypeScript copy, where indices are UTF-16.
func negatedSuffix(after string) bool {
	rest := strings.TrimLeft(after, " \t")
	for _, n := range negationSuffixes {
		if strings.HasPrefix(rest, n) {
			return true
		}
	}
	return false
}

// Median is the middle value, or the mean of the two middle values for
// an even count.
func Median(vals []float64) (float64, bool) {
	if len(vals) == 0 {
		return 0, false
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2], true
	}
	return (s[n/2-1] + s[n/2]) / 2, true
}

// P85 is the nearest-rank 85th percentile. Integer rank arithmetic
// ((85n+99)/100) on purpose: 85*20/100 in floats lands on
// 17.000000000000004 and a ceil rounds a hair high.
func P85(vals []float64) (float64, bool) {
	if len(vals) == 0 {
		return 0, false
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)
	rank := (85*len(s) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return s[rank-1], true
}

/* ── rendering ── */

// FormatSeconds is the resume cell: 4m 10s / 1h 02m / 45s. Exported for
// the surfaces that render the same document (the CLI test checks its
// table against JSON with it).
func FormatSeconds(secs float64) string {
	s := int(math.Round(secs))
	switch {
	case s >= 3600:
		return fmt.Sprintf("%dh %02dm", s/3600, (s%3600)/60)
	case s >= 60:
		return fmt.Sprintf("%dm %02ds", s/60, s%60)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// Definitions is the footer under every table (and the definitions
// object of JSON): a metric name and the sentence that fixes what it
// counts. Branch lines (which visits fed sessions, whether self resolved,
// the empty-catalog note) are rows with their own names so both surfaces
// print the same strings.
func (r Report) Definitions() [][2]string {
	defs := [][2]string{
		{"sessions", "person reads — visits with source ui or unknown — split where the gap to the previous read exceeds 30m; a session counts in the week it started"},
	}
	if r.CLIFallback {
		defs = append(defs, [2]string{"sessions source", "sessions from cli visits (no ui reads recorded)"})
	} else {
		defs = append(defs, [2]string{"sessions source", "sessions from ui+unknown visits"})
	}
	defs = append(defs, [2]string{"resume (median)",
		"time from a session start to its first write — a changelog entry or comment by the configured account — counted only before the next session starts; sessions without a write are excluded (the cell shows k of n)"})
	if !r.SelfResolved {
		defs = append(defs, [2]string{"self", "unresolved — any author on visited issues"})
	}
	defs = append(defs,
		[2]string{"wip age p85", "nearest-rank 85th percentile of the days an issue had been in progress at week end"},
		[2]string{"in progress", "issues in progress at week end"},
		[2]string{"closed", "issues that entered a done status during the week (status ids resolved through status_catalog)"},
		[2]string{"mismatch", "comments claiming the work is finished on issues not done now (heuristic: a done-word standing on its own, negations and quoted text excluded)"},
		[2]string{"change", "percentage for resume and wip age p85, signed count for the rest; n/a when the previous week has no value"},
	)
	if r.CatalogEmpty {
		defs = append(defs, [2]string{"status_catalog", "empty — weeks before the current one show no value for wip age p85 and in progress, and closed shows none everywhere; a sync fills the table"})
	}
	return defs
}

// changePct is the change cell for resume and wip age p85.
func changePct(prev, cur *float64) string {
	if prev == nil || cur == nil || *prev == 0 {
		return "n/a"
	}
	pct := int(math.Round((*cur - *prev) / *prev * 100))
	if pct == 0 {
		return "0%"
	}
	return fmt.Sprintf("%+d%%", pct)
}

// changeInt is the change cell for the always-computed counts.
func changeInt(prev, cur int) string {
	return fmt.Sprintf("%+d", cur-prev)
}

// changeCount is the change cell for in progress / closed: nil is a
// missing week, not a zero.
func changeCount(prev, cur *int) string {
	if prev == nil || cur == nil {
		return "n/a"
	}
	return fmt.Sprintf("%+d", *cur-*prev)
}

// Table renders the text output: header row, six metric rows, then the
// definitions footer — every time, in every invocation.
func (r Report) Table() string {
	buckets := r.Buckets
	header := make([]string, 0, len(buckets)+2)
	header = append(header, "metric")
	for _, b := range buckets {
		header = append(header, b.Label())
	}
	header = append(header, "change")

	resumeCell := func(b Bucket) string {
		if b.Resume == nil {
			// Sessions happened but none wrote: the definition promises k of
			// n, and "— (0 of 3)" is the reader's only way to tell "no
			// sessions" from "sessions without a write" (real workspace,
			// 2026-09-06: a bare dash next to sessions=1 read as a bug).
			if b.ResumeN > 0 {
				return fmt.Sprintf("— (0 of %d)", b.ResumeN)
			}
			return "—"
		}
		s := FormatSeconds(*b.Resume)
		if b.ResumeK < b.ResumeN {
			s += fmt.Sprintf(" (%d of %d)", b.ResumeK, b.ResumeN)
		}
		return s
	}
	pct := func(get func(*Bucket) *float64) func(int) string {
		return func(i int) string { return changePct(get(&buckets[i-1]), get(&buckets[i])) }
	}

	rows := [][]string{header}
	metricRow := func(name string, cell func(int) string, change func(int) string) {
		row := make([]string, 0, len(buckets)+2)
		row = append(row, name)
		for i := range buckets {
			row = append(row, cell(i))
		}
		if change == nil || len(buckets) < 2 {
			row = append(row, "")
		} else {
			row = append(row, change(len(buckets)-1))
		}
		rows = append(rows, row)
	}
	metricRow("sessions",
		func(i int) string { return strconv.Itoa(buckets[i].Sessions) },
		func(i int) string { return changeInt(buckets[i-1].Sessions, buckets[i].Sessions) })
	metricRow("resume (median)",
		func(i int) string { return resumeCell(buckets[i]) },
		pct(func(b *Bucket) *float64 { return b.Resume }))
	metricRow("wip age p85",
		func(i int) string {
			if buckets[i].WipP85 == nil {
				return "—"
			}
			return fmt.Sprintf("%.1fd", *buckets[i].WipP85)
		},
		pct(func(b *Bucket) *float64 { return b.WipP85 }))
	metricRow("in progress",
		func(i int) string {
			if buckets[i].InProg == nil {
				return "—"
			}
			return strconv.Itoa(*buckets[i].InProg)
		},
		func(i int) string { return changeCount(buckets[i-1].InProg, buckets[i].InProg) })
	metricRow("closed",
		func(i int) string {
			if buckets[i].Closed == nil {
				return "—"
			}
			return strconv.Itoa(*buckets[i].Closed)
		},
		func(i int) string { return changeCount(buckets[i-1].Closed, buckets[i].Closed) })
	metricRow("mismatch",
		func(i int) string { return strconv.Itoa(buckets[i].Mismatch) },
		func(i int) string { return changeInt(buckets[i-1].Mismatch, buckets[i].Mismatch) })

	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if utf8len(cell) > widths[i] {
				widths[i] = utf8len(cell)
			}
		}
	}
	var b strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8len(cell)))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("definitions:\n")
	for _, d := range r.Definitions() {
		b.WriteString(d[0] + ": " + d[1] + "\n")
	}
	return b.String()
}

// utf8len is the rune count a column width needs.
func utf8len(s string) int { return len([]rune(s)) }

/* ── JSON ── */

// BucketKeys is the keys object of one JSON bucket: the issue keys behind
// each count, capped at MaxJSONKeys per array with KeysTruncated set when
// any array was cut.
type BucketKeys struct {
	Closed        []string `json:"closed"`
	InProgress    []string `json:"in progress"`
	Mismatch      []string `json:"mismatch"`
	KeysTruncated bool     `json:"keys_truncated,omitempty"`
}

// BucketJSON is one element of buckets. The keys are the table row
// names verbatim, so a consumer reads the same words the footer defines.
type BucketJSON struct {
	From       string     `json:"from"`
	To         string     `json:"to"`
	Partial    bool       `json:"partial"`
	Sessions   int        `json:"sessions"`
	Resume     *float64   `json:"resume (median)"`
	WipP85     *float64   `json:"wip age p85"`
	InProgress *int       `json:"in progress"`
	Closed     *int       `json:"closed"`
	Mismatch   int        `json:"mismatch"`
	Keys       BucketKeys `json:"keys"`
}

// Doc is the JSON document: buckets plus the same definitions
// strings the footer prints.
type Doc struct {
	Buckets     []BucketJSON      `json:"buckets"`
	Definitions map[string]string `json:"definitions"`
}

// capKeys caps one key array at MaxJSONKeys.
func capKeys(keys []string) ([]string, bool) {
	if keys == nil {
		return []string{}, false
	}
	if len(keys) <= MaxJSONKeys {
		return keys, false
	}
	return keys[:MaxJSONKeys], true
}

func (r Report) JSON() Doc {
	round := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	out := Doc{
		Buckets:     make([]BucketJSON, 0, len(r.Buckets)),
		Definitions: map[string]string{},
	}
	for _, d := range r.Definitions() {
		out.Definitions[d[0]] = d[1]
	}
	for _, b := range r.Buckets {
		j := BucketJSON{
			From:       b.From.Format(time.RFC3339),
			To:         b.To.Format(time.RFC3339),
			Partial:    b.Partial,
			Sessions:   b.Sessions,
			InProgress: ptrOrNil(b.InProg),
			Closed:     ptrOrNil(b.Closed),
			Mismatch:   b.Mismatch,
		}
		var truncated bool
		j.Keys.Closed, truncated = capKeys(b.ClosedKeys)
		j.Keys.InProgress, _ = capKeys(b.InProgressKeys)
		j.Keys.Mismatch, _ = capKeys(b.MismatchKeys)
		if truncated || len(b.ClosedKeys) > MaxJSONKeys ||
			len(b.InProgressKeys) > MaxJSONKeys || len(b.MismatchKeys) > MaxJSONKeys {
			j.Keys.KeysTruncated = true
		}
		if b.Resume != nil {
			v := round(*b.Resume)
			j.Resume = &v
		}
		if b.WipP85 != nil {
			v := round(*b.WipP85)
			j.WipP85 = &v
		}
		out.Buckets = append(out.Buckets, j)
	}
	return out
}

func ptrOrNil(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
