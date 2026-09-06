package main

// gadak retro — the weekly retrospective docs/project/THEORY.md calls its
// instrument: one table, read from the mirror plus local.db, definitions
// printed under the numbers every time. Six rows: sessions, resume (median),
// wip age p85, in progress, closed, mismatch. Columns are ISO weeks (Monday
// 00:00 local, oldest left, the current partial week last) and a change
// column against the previous week. Everything is a read: the mirror, the
// status_catalog lookup, and local.visits; nothing here writes or touches a
// network. Statuses are keyed by id through status_catalog and by
// status_category — never by the display name beside them.
//
// Degradation is a dash, never an error: an empty status_catalog blanks the
// weeks only the changelog can answer, an unparseable timestamp drops one row,
// and a workspace with no self identity resolves resume against any author on
// the issues the session visited (the footer says so).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/store"
)

const retroUsageLine = "usage: gadak retro [--since 14d|<N>d|<N>w] [--json]"

// retroDefaultSince is two ISO weeks, enough for a "this week against last
// week" read without paging.
const retroDefaultSince = "14d"

// retroSessionGap splits person reads into sessions: a counted visit more
// than this after the previous one starts a new session. Exactly the gap is
// still the same session — strictly greater, so a test can sit on the line.
const retroSessionGap = 30 * time.Minute

// retroMaxSinceDays caps --since at one year of weeks.
const retroMaxSinceDays = 365

// retroDoneWords is the mismatch heuristic: a comment containing one of these
// on an issue that is not done now. A heuristic, not a parser — the row says
// so in its definition, and the words cover the languages this workspace
// writes (English, Korean, Japanese). CJK matching is substring containment;
// the words are long enough that false hits inside other words are rare.
var retroDoneWords = []string{
	"done", "fixed", "merged", "resolved", "shipped",
	"완료", "해결", "머지",
	"完了", "修正済み", "対応済み",
}

var retroSinceRe = regexp.MustCompile(`^([0-9]+)([dw])$`)

// parseRetroSince accepts <N>d or <N>w, 1..365 days equivalent. Anything else
// is a usage error (the caller wraps it), never a silent default.
func parseRetroSince(s string) (time.Duration, error) {
	m := retroSinceRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("--since wants <N>d or <N>w (for example 14d, 30d, 8w), 1 to %d days", retroMaxSinceDays)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("--since wants a number of days or weeks, 1 or more")
	}
	days := n
	if m[2] == "w" {
		days = 7 * n
	}
	if days > retroMaxSinceDays {
		return 0, fmt.Errorf("--since is capped at %d days (got %d)", retroMaxSinceDays, days)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// retroEpochSQL is store.currentEpochSQL (internal/store/origin_scope.go),
// unexported there: the visits read runs on the mirror handle where local.db
// is the attached schema, so the epoch clause needs the local. prefix. Keep
// the two in lockstep — a visit recorded under a retired origin must not
// count as a session in this one (GDK-418).
const retroEpochSQL = `COALESCE((SELECT CAST(v AS INTEGER) FROM local.local_meta WHERE k = 'origin_epoch'), 0)`

func cmdRetro(args []string) error {
	fs := newFlagSet("retro")
	sinceFlag := fs.String("since", retroDefaultSince, "how far back the table reaches: 14d, 30d, 4w (1 to 365 days)")
	asJSON := fs.Bool("json", false, "emit the same numbers as one JSON document")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("retro", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usageError("retro", retroUsageLine)
	}
	since, err := parseRetroSince(*sinceFlag)
	if err != nil {
		return usageError("retro", err.Error())
	}
	db, err := openReadOnly()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)

	// Self identity comes from the same config the feed uses. Missing config
	// is a warning, not a failure: resume degrades to any-author writes on
	// visited issues and the footer names the branch.
	cfg, cfgErr := config.Load()
	me := store.FeedIdentityOf(cfg)
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read the workspace config; resume counts any author on visited issues: %v\n", cfgErr)
		me = store.FeedIdentity{}
	}
	rep, err := computeRetro(db, me, since, time.Now())
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(os.Stdout).Encode(rep.jsonDoc())
	}
	fmt.Print(rep.table())
	return nil
}

/* ── buckets ── */

// retroBucket is one ISO week column: From is Monday 00:00 local; the last
// bucket runs to now and is partial. The metric fields are per bucket:
// pointers are nil where the answer does not exist (no data for it), which
// the table prints as a dash and --json emits as null.
type retroBucket struct {
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
}

// retroMonday is the Monday 00:00 (in t location) of the ISO week holding t.
func retroMonday(t time.Time) time.Time {
	y, m, d := t.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	off := (int(day.Weekday()) + 6) % 7 // Sunday reads as 6 days back
	return day.AddDate(0, 0, -off)
}

// retroBuckets lays the weeks from the Monday of the first week --since
// reaches up to now. At least one bucket comes back: an empty span (now
// exactly on a Monday midnight) still gets the single partial week.
func retroBuckets(now time.Time, since time.Duration) []retroBucket {
	b := retroMonday(now.Add(-since))
	var out []retroBucket
	for ; b.Before(now); b = b.AddDate(0, 0, 7) {
		out = append(out, retroBucket{From: b})
	}
	if len(out) == 0 {
		out = append(out, retroBucket{From: retroMonday(now.Add(-since))})
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

type retroVisit struct {
	kind   string
	key    string
	source string
	at     time.Time
}

type retroWrite struct {
	at       time.Time
	item     string
	author   string
	authorID string
}

type retroComment struct {
	retroWrite
	body string
}

type retroStatusRow struct {
	at     time.Time
	fromID string
	toID   string
	seq    string // changelog id: deterministic tie-break for equal at
}

type retroItem struct {
	id       string
	sourceID string
	kind     string
	key      string
}

// retroReport is everything the table and the JSON document render.
type retroReport struct {
	buckets      []retroBucket
	cliFallback  bool // no ui/unknown visits; sessions came from cli rows
	selfResolved bool // a FeedIdentity was available for resume
	catalogEmpty bool // status_catalog has no rows
}

/* ── loading and computing ── */

// parseRetroTime is the timestamp ladder for every column this command
// reads: ISOMilli first (the stores own format), then RFC 3339 variants,
// then the bare shapes an older sync might have left. Unparseable reads as
// absent — the row drops, the command never errors on one value.
func parseRetroTime(s string) (time.Time, bool) {
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

// computeRetro reads the mirror plus local.visits and fills the buckets.
func computeRetro(db *sql.DB, me store.FeedIdentity, since time.Duration, now time.Time) (retroReport, error) {
	rep := retroReport{buckets: retroBuckets(now, since)}
	rep.selfResolved = me != store.FeedIdentity{}
	first := rep.buckets[0].From

	// Visits: epoch-scoped, prefiltered by day prefix (ISOMilli starts with
	// the date, so a string bound is a real bound), then filtered exactly on
	// parsed times — bucket edges are local midnight and the strings are UTC.
	dayPrefix := first.UTC().AddDate(0, 0, -1).Format("2006-01-02")
	var visits []retroVisit
	vrows, err := db.Query(`SELECT COALESCE(kind,''), COALESCE(key,''), COALESCE(viewed_at,''), COALESCE(source,'')
		FROM local.visits WHERE origin_epoch = `+retroEpochSQL+` AND viewed_at >= ?`, dayPrefix)
	if err != nil {
		return rep, err
	}
	for vrows.Next() {
		var kind, key, at, source string
		if err := vrows.Scan(&kind, &key, &at, &source); err != nil {
			vrows.Close()
			return rep, err
		}
		t, ok := parseRetroTime(at)
		// Keep one session gap before the first bucket: a read inside the
		// window that follows a read just outside it by less than the gap is
		// the same session, not a new one at the window edge. Sessions that
		// start before the first bucket are dropped by the bucket loop.
		if !ok || t.Before(first.Add(-retroSessionGap)) || t.After(now) {
			continue
		}
		visits = append(visits, retroVisit{kind: kind, key: key, source: source, at: t})
	}
	if err := vrows.Err(); err != nil {
		vrows.Close()
		return rep, err
	}
	vrows.Close()

	// Items and issues: the key<->item and item<->category maps every other
	// pass walks.
	itemByID := map[string]retroItem{}
	keyItems := map[string][]string{}
	irows, err := db.Query(`SELECT id, COALESCE(source_id,''), COALESCE(kind,''), COALESCE(key,'') FROM items`)
	if err != nil {
		return rep, err
	}
	for irows.Next() {
		var it retroItem
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
	srows, err := db.Query(`SELECT item_id, COALESCE(status_category,''), COALESCE(status_changed_at,'') FROM issues`)
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
	crows, err := db.Query(`SELECT COALESCE(source_id,''), COALESCE(status_id,''), COALESCE(category,'') FROM status_catalog`)
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
	rep.catalogEmpty = len(cat) == 0

	// Status changelog, whole history before now: wip age and closed walk it
	// backwards from each week end.
	statusByItem := map[string][]retroStatusRow{}
	clog, err := db.Query(`SELECT item_id, COALESCE(id,''), COALESCE(at,''), COALESCE(from_id,''), COALESCE(to_id,'')
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
		t, ok := parseRetroTime(at)
		if !ok || t.After(now) {
			continue
		}
		statusByItem[item] = append(statusByItem[item], retroStatusRow{at: t, fromID: fromID, toID: toID, seq: id})
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
	var writes []retroWrite
	var comments []retroComment
	wrows, err := db.Query(`SELECT item_id, COALESCE(at,''), COALESCE(author,''), COALESCE(author_id,'')
		FROM changelog WHERE at >= ?`, writePrefix)
	if err != nil {
		return rep, err
	}
	for wrows.Next() {
		var w retroWrite
		var at string
		if err := wrows.Scan(&w.item, &at, &w.author, &w.authorID); err != nil {
			wrows.Close()
			return rep, err
		}
		t, ok := parseRetroTime(at)
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

	mrows, err := db.Query(`SELECT item_id, COALESCE(created_at,''), COALESCE(author,''), COALESCE(author_id,''), COALESCE(body_text,'')
		FROM comments WHERE created_at >= ?`, writePrefix)
	if err != nil {
		return rep, err
	}
	for mrows.Next() {
		var c retroComment
		var at string
		if err := mrows.Scan(&c.item, &at, &c.author, &c.authorID, &c.body); err != nil {
			mrows.Close()
			return rep, err
		}
		t, ok := parseRetroTime(at)
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

	counted := make([]retroVisit, 0, len(visits))
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
		rep.cliFallback = len(counted) > 0
	}
	sort.SliceStable(counted, func(i, j int) bool { return counted[i].at.Before(counted[j].at) })

	type retroSession struct {
		start     time.Time
		issueKeys map[string]bool
	}
	var sessions []retroSession
	for i, v := range counted {
		newSession := i == 0 || v.at.Sub(counted[i-1].at) > retroSessionGap
		if newSession {
			sessions = append(sessions, retroSession{start: v.at, issueKeys: map[string]bool{}})
		}
		if v.kind == store.VisitKindIssue {
			sessions[len(sessions)-1].issueKeys[v.key] = true
		}
	}

	for bi := range rep.buckets {
		b := &rep.buckets[bi]
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
			if secs, ok := retroSessionResume(writes, comments, visited, s.start, end, me, rep.selfResolved); ok {
				resumes = append(resumes, secs)
				b.ResumeK++
			}
		}
		if med, ok := retroMedian(resumes); ok {
			b.Resume = &med
		}
	}

	/* wip age p85, in progress, closed, mismatch */

	for bi := range rep.buckets {
		b := &rep.buckets[bi]
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
				if t, ok := parseRetroTime(issChangedAt[item]); ok && !t.After(now) {
					ages = append(ages, now.Sub(t).Hours()/24)
				}
			}
			n := count
			b.InProg = &n
			if p, ok := retroP85(ages); ok {
				b.WipP85 = &p
			}
		} else if !rep.catalogEmpty {
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
				}
			}
			n := len(ages)
			b.InProg = &n
			if p, ok := retroP85(ages); ok {
				b.WipP85 = &p
			}
		}

		if !rep.catalogEmpty {
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
		}

		for _, c := range comments {
			if c.at.Before(b.From) || !c.at.Before(b.To) {
				continue
			}
			if issCategory[c.item] == "" || issCategory[c.item] == store.CategoryDone {
				continue // not an issue, or done now
			}
			if retroHasDoneWord(c.body) {
				b.Mismatch++
			}
		}
	}
	return rep, nil
}

// retroSessionResume walks the merged write streams in time order for the
// first write inside [start, end) that belongs to the session: with a self
// identity only the configured account counts, without one any author counts
// on the condition the write lands on an issue the session visited.
func retroSessionResume(writes []retroWrite, comments []retroComment, visitedItems map[string]bool,
	start, end time.Time, me store.FeedIdentity, selfResolved bool) (float64, bool) {
	i, j := 0, 0
	for i < len(writes) || j < len(comments) {
		var w retroWrite
		var at time.Time
		switch {
		case j >= len(comments):
			w, at = writes[i], writes[i].at
			i++
		case i >= len(writes):
			w, at = comments[j].retroWrite, comments[j].at
			j++
		case !writes[i].at.After(comments[j].at):
			w, at = writes[i], writes[i].at
			i++
		default:
			w, at = comments[j].retroWrite, comments[j].at
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
func lastStatusAt(rows []retroStatusRow, end time.Time) (time.Time, bool) {
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].at.After(end) {
			return rows[i].at, true
		}
	}
	return time.Time{}, false
}

// lastStatusCategory maps that newest row to its category through the
// catalog. An unmapped id reads as the empty category — not in progress.
func lastStatusCategory(rows []retroStatusRow, cat map[string]string, sourceID string, end time.Time) string {
	for i := len(rows) - 1; i >= 0; i-- {
		if !rows[i].at.After(end) {
			return cat[sourceID+"\x00"+rows[i].toID]
		}
	}
	return ""
}

// retroHasDoneWord is the mismatch substring test, lowercased for the
// English words and untouched for CJK (ToLower is a no-op there).
func retroHasDoneWord(body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}
	low := strings.ToLower(body)
	for _, w := range retroDoneWords {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// retroMedian is the middle value, or the mean of the two middle values for
// an even count.
func retroMedian(vals []float64) (float64, bool) {
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

// retroP85 is the nearest-rank 85th percentile. Integer rank arithmetic
// ((85n+99)/100) on purpose: 85*20/100 in floats lands on
// 17.000000000000004 and a ceil rounds a hair high.
func retroP85(vals []float64) (float64, bool) {
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

// formatRetroSeconds is the resume cell: 4m 10s / 1h 02m / 45s.
func formatRetroSeconds(secs float64) string {
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

// retroDefinitions is the footer under every table (and the definitions
// object of --json): a metric name and the sentence that fixes what it
// counts. Branch lines (which visits fed sessions, whether self resolved,
// the empty-catalog note) are rows with their own names so both surfaces
// print the same strings.
func (r retroReport) definitions() [][2]string {
	defs := [][2]string{
		{"sessions", "person reads — visits with source ui or unknown — split where the gap to the previous read exceeds 30m; a session counts in the week it started"},
	}
	if r.cliFallback {
		defs = append(defs, [2]string{"sessions source", "sessions from cli visits (no ui reads recorded)"})
	} else {
		defs = append(defs, [2]string{"sessions source", "sessions from ui+unknown visits"})
	}
	defs = append(defs, [2]string{"resume (median)",
		"time from a session start to its first write — a changelog entry or comment by the configured account — counted only before the next session starts; sessions without a write are excluded (the cell shows k of n)"})
	if !r.selfResolved {
		defs = append(defs, [2]string{"self", "unresolved — any author on visited issues"})
	}
	defs = append(defs,
		[2]string{"wip age p85", "nearest-rank 85th percentile of the days an issue had been in progress at week end"},
		[2]string{"in progress", "issues in progress at week end"},
		[2]string{"closed", "issues that entered a done status during the week (status ids resolved through status_catalog)"},
		[2]string{"mismatch", "comments containing a done-word on issues not done now (heuristic: done-words in comments on unfinished issues)"},
		[2]string{"change", "percentage for resume and wip age p85, signed count for the rest; n/a when the previous week has no value"},
	)
	if r.catalogEmpty {
		defs = append(defs, [2]string{"status_catalog", "empty — weeks before the current one show no value for wip age p85 and in progress, and closed shows none everywhere; a sync fills the table"})
	}
	return defs
}

// retroBucketHeader is the column title: 08-24..08-31, or 08-31..now.
func retroBucketHeader(b retroBucket) string {
	to := b.To.Format("01-02")
	if b.Partial {
		to = "now"
	}
	return b.From.Format("01-02") + ".." + to
}

// retroChangePct is the change cell for resume and wip age p85.
func retroChangePct(prev, cur *float64) string {
	if prev == nil || cur == nil || *prev == 0 {
		return "n/a"
	}
	pct := int(math.Round((*cur - *prev) / *prev * 100))
	if pct == 0 {
		return "0%"
	}
	return fmt.Sprintf("%+d%%", pct)
}

// retroChangeInt is the change cell for the always-computed counts.
func retroChangeInt(prev, cur int) string {
	return fmt.Sprintf("%+d", cur-prev)
}

// retroChangeCount is the change cell for in progress / closed: nil is a
// missing week, not a zero.
func retroChangeCount(prev, cur *int) string {
	if prev == nil || cur == nil {
		return "n/a"
	}
	return fmt.Sprintf("%+d", *cur-*prev)
}

// table renders the text output: header row, six metric rows, then the
// definitions footer — every time, in every invocation (C5).
func (r retroReport) table() string {
	buckets := r.buckets
	header := make([]string, 0, len(buckets)+2)
	header = append(header, "metric")
	for _, b := range buckets {
		header = append(header, retroBucketHeader(b))
	}
	header = append(header, "change")

	resumeCell := func(b retroBucket) string {
		if b.Resume == nil {
			return "—"
		}
		s := formatRetroSeconds(*b.Resume)
		if b.ResumeK < b.ResumeN {
			s += fmt.Sprintf(" (%d of %d)", b.ResumeK, b.ResumeN)
		}
		return s
	}
	pct := func(get func(*retroBucket) *float64) func(int) string {
		return func(i int) string { return retroChangePct(get(&buckets[i-1]), get(&buckets[i])) }
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
		func(i int) string { return retroChangeInt(buckets[i-1].Sessions, buckets[i].Sessions) })
	metricRow("resume (median)",
		func(i int) string { return resumeCell(buckets[i]) },
		pct(func(b *retroBucket) *float64 { return b.Resume }))
	metricRow("wip age p85",
		func(i int) string {
			if buckets[i].WipP85 == nil {
				return "—"
			}
			return fmt.Sprintf("%.1fd", *buckets[i].WipP85)
		},
		pct(func(b *retroBucket) *float64 { return b.WipP85 }))
	metricRow("in progress",
		func(i int) string {
			if buckets[i].InProg == nil {
				return "—"
			}
			return strconv.Itoa(*buckets[i].InProg)
		},
		func(i int) string { return retroChangeCount(buckets[i-1].InProg, buckets[i].InProg) })
	metricRow("closed",
		func(i int) string {
			if buckets[i].Closed == nil {
				return "—"
			}
			return strconv.Itoa(*buckets[i].Closed)
		},
		func(i int) string { return retroChangeCount(buckets[i-1].Closed, buckets[i].Closed) })
	metricRow("mismatch",
		func(i int) string { return strconv.Itoa(buckets[i].Mismatch) },
		func(i int) string { return retroChangeInt(buckets[i-1].Mismatch, buckets[i].Mismatch) })

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
	for _, d := range r.definitions() {
		b.WriteString(d[0] + ": " + d[1] + "\n")
	}
	return b.String()
}

// utf8len is the rune count a column width needs.
func utf8len(s string) int { return len([]rune(s)) }

/* ── JSON ── */

// retroBucketJSON is one element of buckets. The keys are the table row
// names verbatim, so a consumer reads the same words the footer defines.
type retroBucketJSON struct {
	From       string   `json:"from"`
	To         string   `json:"to"`
	Partial    bool     `json:"partial"`
	Sessions   int      `json:"sessions"`
	Resume     *float64 `json:"resume (median)"`
	WipP85     *float64 `json:"wip age p85"`
	InProgress *int     `json:"in progress"`
	Closed     *int     `json:"closed"`
	Mismatch   int      `json:"mismatch"`
}

// retroJSONDoc is the --json document: buckets plus the same definitions
// strings the footer prints.
type retroJSONDoc struct {
	Buckets     []retroBucketJSON `json:"buckets"`
	Definitions map[string]string `json:"definitions"`
}

func (r retroReport) jsonDoc() retroJSONDoc {
	round := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	out := retroJSONDoc{
		Buckets:     make([]retroBucketJSON, 0, len(r.buckets)),
		Definitions: map[string]string{},
	}
	for _, d := range r.definitions() {
		out.Definitions[d[0]] = d[1]
	}
	for _, b := range r.buckets {
		j := retroBucketJSON{
			From:       b.From.Format(time.RFC3339),
			To:         b.To.Format(time.RFC3339),
			Partial:    b.Partial,
			Sessions:   b.Sessions,
			InProgress: ptrOrNil(b.InProg),
			Closed:     ptrOrNil(b.Closed),
			Mismatch:   b.Mismatch,
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
