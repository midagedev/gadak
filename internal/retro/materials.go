package retro

// Retrospective materials: the second half of the document `gadak retro`
// serves. The nine-row table answers "how much";
// these answer "what actually happened" — the events inside a bucket, the
// ones that were not supposed to happen, what the closures were made of,
// what got old, what moved without being seen, and whether last cycle's
// action items moved their own metric.
//
// Everything here is a decomposition of samples Compute already took, never
// a second query answering the same question a different way: closed_by_type,
// closed_by_epic, unplanned and cycle_points all partition the bucket's
// existing closed/cycle item sets, so their counts sum back to the row above
// them (TestMaterialsClosedDecompositionsSumToClosed). Nothing here is a
// score and nothing is per person (THEORY.md: G9 progress, not score) — a
// surprise is a shape with a reason attached, not a demerit.
//
// Compute owns the loads it already does and hands them over; this file adds
// only what the table never needed (issue metadata, the sprint changelog,
// the retro-action issues). Statuses are keyed by id through status_catalog
// and by status_category, never by the display name beside them.

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/store"
)

// MaxEvents caps one bucket's event list. A bucket is a week or a sprint of
// one team; past a few hundred rows the list has stopped being a thing a
// person reads and the reader wants the table above it instead.
const MaxEvents = 300

// MaxActions caps the retro-action list: fifty is more open action items
// than a retrospective practice survives, so the cap is a guard, not a page.
const MaxActions = 50

// reversalTransitions is how many status moves inside one bucket make an
// issue a "reversal": four is two round trips' worth of churn, past the
// ordinary open→progress→done→(reopen) an honest issue can show.
const reversalTransitions = 4

/* ── the shapes ── */

// AgingItem is one in-progress issue and how long it has been there.
type AgingItem struct {
	Key         string  `json:"key"`
	Days        float64 `json:"days"`
	Summary     string  `json:"summary"`
	IssueTypeID string  `json:"issue_type_id"`
}

// Aging is the whole in-progress tail, oldest first, with the percentile the
// table's "wip age p85" row shows. It is report-level, not per bucket: the
// question "what is old" is asked of now, not of a window that has closed.
type Aging struct {
	P85            *float64    `json:"p85_days"`
	Items          []AgingItem `json:"items"`
	ItemsTruncated bool        `json:"items_truncated,omitempty"`
}

// Action is one issue labelled retro-action: something a past
// retrospective decided to do. Metric/Then/Now are the honest part — an
// action that names a metric is measured against it, and one that does not
// says so with an empty name rather than a number nobody can check.
type Action struct {
	Key            string   `json:"key"`
	Summary        string   `json:"summary"`
	StatusCategory string   `json:"status_category"`
	CreatedAt      string   `json:"created_at"`
	ResolvedAt     *string  `json:"resolved_at"`
	Metric         string   `json:"metric"`
	Then           *float64 `json:"then"`
	Now            *float64 `json:"now"`
}

// Event is one thing that happened inside a bucket, in the bucket's own
// order. Kind is the vocabulary the renderer switches on; Detail is the one
// extra string that kind carries (a comment's author, a sprint's name).
type Event struct {
	At     string `json:"at"`
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// Event kinds, the closed vocabulary Event.Kind uses.
const (
	EventCreated   = "created"
	EventStarted   = "started"
	EventResolved  = "resolved"
	EventReopened  = "reopened"
	EventSprintIn  = "sprint_in"
	EventSprintOut = "sprint_out"
	EventComment   = "comment"
)

// Surprise is an event the plan did not have: work that came back, work that
// churned, work that arrived after the sprint had started, work that was
// carried in. Detail carries the reason where the mirror has one — a reopen
// reason, a carry count, an arrival time — because a surprise without its
// reason is an accusation.
type Surprise struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	// Summary is the issue's title, carried beside the key so the surface can
	// name the work instead of listing identifiers (GDK-1737). Empty when the
	// mirror has no title for that item, which is the honest answer.
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
}

// Surprise kinds, the closed vocabulary Surprise.Kind uses.
const (
	SurpriseReopened        = "reopened"
	SurpriseReversal        = "reversal"
	SurpriseAddedAfterStart = "added_after_start"
	SurpriseCarried         = "carried"
)

// TypeCount is one row of closed_by_type: what the bucket's closures were
// made of, by issue type id (never by the display name beside it).
type TypeCount struct {
	IssueTypeID string   `json:"issue_type_id"`
	IssueType   string   `json:"issue_type"`
	Count       int      `json:"count"`
	Keys        []string `json:"keys"`
}

// EpicCount is one row of closed_by_epic. EpicKey "" is the issues under no
// epic; the surface names that row in the reader's language, not this file.
type EpicCount struct {
	EpicKey string   `json:"epic_key"`
	Title   string   `json:"title"`
	Count   int      `json:"count"`
	Keys    []string `json:"keys"`
}

// KeyCount is a count that carries its keys — the shape every number in this
// package takes, so a number is never a dead end.
type KeyCount struct {
	Count int      `json:"count"`
	Keys  []string `json:"keys"`
}

// KeySet is a key list whose length is the whole answer.
type KeySet struct {
	Keys []string `json:"keys"`
}

// CyclePoint is one sample behind the cycle p50/p85 cells: the row that lets
// a reader see the distribution instead of two percentiles of it.
type CyclePoint struct {
	Key        string  `json:"key"`
	Summary    string  `json:"summary"`
	ResolvedAt string  `json:"resolved_at"`
	Days       float64 `json:"days"`
}

/* ── loaded rows ── */

// issueMeta is the per-issue columns the materials need and the table never
// did. One read of the issues view fills it for every issue.
type issueMeta struct {
	key          string
	summary      string
	issueType    string
	issueTypeID  string
	epicKey      string
	created      string
	updated      string
	category     string
	changedAt    string
	resolvedAt   string
	reopenReason string
	carryover    int
	sprintID     int64
}

// sprintRow is one sprint changelog entry: which sprint an issue left and
// which it joined, at one instant.
type sprintRow struct {
	at     time.Time
	item   string
	fromID string
	toID   string
	from   string
	to     string
}

// actionRow is a retro-action issue as it comes off disk, before the metric
// line is parsed and the then/now values are read out of the buckets.
type actionRow struct {
	key        string
	summary    string
	category   string
	created    string
	createdAt  time.Time
	resolvedAt string
	body       string
}

// loadIssueMeta reads the per-issue columns the materials need. It is one
// pass over the issues view, the same posture as Compute's loaders: one
// defer rows.Close(), one rows.Err().
func loadIssueMeta(ctx context.Context, db *sql.DB) (map[string]issueMeta, error) {
	rows, err := db.QueryContext(ctx, `SELECT item_id, COALESCE(key,''), COALESCE(summary,''),
		COALESCE(issue_type,''), COALESCE(issue_type_id,''), COALESCE(epic_key,''),
		COALESCE(created_at,''), COALESCE(updated_at,''), COALESCE(status_category,''),
		COALESCE(status_changed_at,''), COALESCE(resolved_at,''), COALESCE(reopen_reason,''),
		COALESCE(carryover_count,0), COALESCE(sprint_id,0)
		FROM issues`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]issueMeta{}
	for rows.Next() {
		var id string
		var m issueMeta
		if err := rows.Scan(&id, &m.key, &m.summary, &m.issueType, &m.issueTypeID, &m.epicKey,
			&m.created, &m.updated, &m.category, &m.changedAt, &m.resolvedAt, &m.reopenReason,
			&m.carryover, &m.sprintID); err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, rows.Err()
}

// loadSprintLog reads the sprint changelog for the window. from_id/to_id are
// sprint ids and from_value/to_value their names: a row with both is a move,
// one with an empty from is an arrival, one with an empty to a departure.
func loadSprintLog(ctx context.Context, db *sql.DB, first, now time.Time) ([]sprintRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT item_id, COALESCE(at,''), COALESCE(from_id,''),
		COALESCE(to_id,''), COALESCE(from_value,''), COALESCE(to_value,'')
		FROM changelog WHERE field = 'sprint' AND at >= ?`,
		first.UTC().AddDate(0, 0, -1).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sprintRow
	for rows.Next() {
		var r sprintRow
		var at string
		if err := rows.Scan(&r.item, &at, &r.fromID, &r.toID, &r.from, &r.to); err != nil {
			return nil, err
		}
		t, ok := parseTime(at)
		if !ok || t.Before(first) || t.After(now) {
			continue
		}
		r.at = t
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadActions reads the issues labelled retro-action, newest first. The
// label is matched through json_each — labels is a JSON array column, and a
// LIKE on the serialized text would match a label that merely contains this
// one.
func loadActions(ctx context.Context, db *sql.DB) ([]actionRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT i.key, COALESCE(i.summary,''),
		COALESCE(i.status_category,''), COALESCE(i.created_at,''), COALESCE(i.resolved_at,''),
		COALESCE(i.description_text,'')
		FROM issues i, json_each(COALESCE(i.labels,'[]')) j
		WHERE j.value = 'retro-action'
		ORDER BY i.created_at DESC, i.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []actionRow
	for rows.Next() {
		var a actionRow
		if err := rows.Scan(&a.key, &a.summary, &a.category, &a.created, &a.resolvedAt, &a.body); err != nil {
			return nil, err
		}
		a.createdAt, _ = parseTime(a.created)
		out = append(out, a)
	}
	return out, rows.Err()
}

/* ── computing ── */

// materialsInput is what Compute already loaded, handed over rather than
// read a second time. A second read would be a second answer to a question
// the table already answered — the decompositions must partition exactly the
// samples the rows above them counted.
type materialsInput struct {
	itemByID     map[string]item
	issCycle     map[string]cycleRow
	cat          map[string]string
	statusByItem map[string][]statusRow
	comments     []comment
	writes       []write
	visits       []visit
	now          time.Time
}

// computeMaterials fills the material fields of every bucket and the
// report-level aging and actions. It never fails the report: a missing
// optional table is an error from the loader (the caller propagates), but an
// absent value inside a row is an omission, exactly as the table treats one.
func computeMaterials(ctx context.Context, db *sql.DB, rep *Report, in materialsInput) error {
	meta, err := loadIssueMeta(ctx, db)
	if err != nil {
		return err
	}
	first := rep.Buckets[0].From
	sprintLog, err := loadSprintLog(ctx, db, first, in.now)
	if err != nil {
		return err
	}
	actions, err := loadActions(ctx, db)
	if err != nil {
		return err
	}

	// Epic titles come from the same map, keyed by the epic's own key: an
	// epic is an issue, so its summary is already loaded.
	titleByKey := map[string]string{}
	for _, m := range meta {
		if m.key != "" {
			titleByKey[m.key] = m.summary
		}
	}

	rep.Aging = computeAging(meta, in.now)

	// Visits are the "seen" half of seen/moved. Only issue reads count — a
	// board or a search is not having looked at an issue — and an empty set
	// means the two rows cannot be answered at all, which the notes say
	// rather than reporting "you saw nothing".
	var issueVisits []visit
	for _, v := range in.visits {
		if v.kind == store.VisitKindIssue && v.key != "" {
			issueVisits = append(issueVisits, v)
		}
	}
	rep.VisitsEmpty = len(issueVisits) == 0

	for bi := range rep.Buckets {
		b := &rep.Buckets[bi]
		b.Events, b.EventsTruncated = bucketEvents(b, meta, in, sprintLog)
		b.Surprises = bucketSurprises(b, meta, in, sprintLog, rep.BySprint)
		b.ClosedByType, b.ClosedByEpic = closedBreakdowns(b, meta, titleByKey)
		b.Unplanned = bucketUnplanned(b, meta)
		b.CyclePoints = bucketCyclePoints(b, meta, in.issCycle)
		b.SeenNotMoved, b.MovedNotSeen = seenMoved(b, meta, issueVisits, in, rep.VisitsEmpty)
	}

	rep.Actions = resolveActions(actions, rep.Buckets)
	return nil
}

// computeAging is the in-progress tail measured at now: every issue whose
// live category is in progress, aged from its last status change (an issue
// whose mirror never recorded one falls back to updated_at, and one with
// neither cannot be aged and is left out rather than shown as zero days).
func computeAging(meta map[string]issueMeta, now time.Time) Aging {
	var items []AgingItem
	var days []float64
	for _, m := range meta {
		if m.category != store.CategoryInProgress {
			continue
		}
		stamp := m.changedAt
		if stamp == "" {
			stamp = m.updated
		}
		t, ok := parseTime(stamp)
		if !ok || t.After(now) {
			continue
		}
		d := roundDays(now.Sub(t).Hours() / 24)
		items = append(items, AgingItem{Key: m.key, Days: d, Summary: m.summary, IssueTypeID: m.issueTypeID})
		days = append(days, d)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Days != items[j].Days {
			return items[i].Days > items[j].Days
		}
		return items[i].Key < items[j].Key
	})
	out := Aging{Items: items}
	if p, ok := P85(days); ok {
		v := roundDays(p)
		out.P85 = &v
	}
	if len(out.Items) > MaxJSONKeys {
		out.Items = out.Items[:MaxJSONKeys]
		out.ItemsTruncated = true
	}
	if out.Items == nil {
		out.Items = []AgingItem{}
	}
	return out
}

// inBucket is the half-open [From, To) test every window in this package
// uses — the same bound the closed row and RECIPES.md state.
func inBucket(b *Bucket, t time.Time) bool {
	return !t.Before(b.From) && t.Before(b.To)
}

// bucketEvents is everything that happened inside the bucket, oldest first:
// creations, the status moves that mean something (entering progress,
// entering done, leaving done), sprint arrivals and departures, and
// comments. Ties break on key then kind, so the list is stable across runs.
func bucketEvents(b *Bucket, meta map[string]issueMeta, in materialsInput, sprintLog []sprintRow) ([]Event, bool) {
	var out []Event
	add := func(at time.Time, key, kind, detail string) {
		if key == "" {
			return
		}
		out = append(out, Event{At: at.Format(time.RFC3339), Key: key, Kind: kind, Detail: detail})
	}
	for _, m := range meta {
		if t, ok := parseTime(m.created); ok && inBucket(b, t) {
			add(t, m.key, EventCreated, "")
		}
	}
	for itemID, rows := range in.statusByItem {
		it, ok := in.itemByID[itemID]
		if !ok {
			continue
		}
		for _, r := range rows {
			if !inBucket(b, r.at) {
				continue
			}
			to := in.cat[it.sourceID+"\x00"+r.toID]
			from := in.cat[it.sourceID+"\x00"+r.fromID]
			switch {
			case to == store.CategoryDone && from != store.CategoryDone:
				add(r.at, it.key, EventResolved, "")
			case from == store.CategoryDone && to != store.CategoryDone:
				add(r.at, it.key, EventReopened, "")
			case to == store.CategoryInProgress && from != store.CategoryInProgress:
				add(r.at, it.key, EventStarted, "")
			}
		}
	}
	for _, s := range sprintLog {
		if !inBucket(b, s.at) {
			continue
		}
		it, ok := in.itemByID[s.item]
		if !ok {
			continue
		}
		if s.fromID != "" {
			add(s.at, it.key, EventSprintOut, s.from)
		}
		if s.toID != "" {
			add(s.at, it.key, EventSprintIn, s.to)
		}
	}
	for _, c := range in.comments {
		if !inBucket(b, c.at) {
			continue
		}
		if it, ok := in.itemByID[c.item]; ok {
			add(c.at, it.key, EventComment, c.author)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Kind < out[j].Kind
	})
	truncated := false
	if len(out) > MaxEvents {
		out = out[:MaxEvents]
		truncated = true
	}
	if out == nil {
		out = []Event{}
	}
	return out, truncated
}

// bucketSurprises is the subset of the bucket nobody planned. Sprint-only
// kinds stay empty under week columns: "added after the sprint started" has
// no meaning when the column is a Monday.
func bucketSurprises(b *Bucket, meta map[string]issueMeta, in materialsInput, sprintLog []sprintRow, bySprint bool) []Surprise {
	var out []Surprise
	// The one place a Surprise is built. Every kind goes through it, so the
	// title travels with the key by construction rather than by four
	// remembered assignments (GDK-1737).
	add := func(kind, itemID, key, detail string) {
		out = append(out, Surprise{Kind: kind, Key: key, Summary: meta[itemID].summary, Detail: detail})
	}
	for itemID, rows := range in.statusByItem {
		it, ok := in.itemByID[itemID]
		if !ok {
			continue
		}
		moves := 0
		reopened := false
		for _, r := range rows {
			if !inBucket(b, r.at) {
				continue
			}
			moves++
			if in.cat[it.sourceID+"\x00"+r.fromID] == store.CategoryDone &&
				in.cat[it.sourceID+"\x00"+r.toID] != store.CategoryDone {
				reopened = true
			}
		}
		if reopened {
			add(SurpriseReopened, itemID, it.key, meta[itemID].reopenReasonOr())
		}
		if moves >= reversalTransitions {
			add(SurpriseReversal, itemID, it.key, strconv.Itoa(moves))
		}
	}
	if bySprint && b.SprintID != 0 {
		for _, s := range sprintLog {
			if !inBucket(b, s.at) || s.toID != strconv.FormatInt(b.SprintID, 10) {
				continue
			}
			if it, ok := in.itemByID[s.item]; ok {
				add(SurpriseAddedAfterStart, s.item, it.key, s.at.Format(time.RFC3339))
			}
		}
		for _, m := range meta {
			if m.sprintID == b.SprintID && m.carryover >= 1 {
				out = append(out, Surprise{Kind: SurpriseCarried, Key: m.key, Summary: m.summary, Detail: strconv.Itoa(m.carryover)})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	if out == nil {
		out = []Surprise{}
	}
	return out
}

// reopenReasonOr is the detail a reopened surprise carries. The column is
// derived at sync time and is often empty; an empty detail is the honest
// answer, not a guess at why.
func (m issueMeta) reopenReasonOr() string { return m.reopenReason }

// closedBreakdowns partitions the bucket's closed sample two ways. Both walk
// closedItems — the item ids the closed row counted — so every closure lands
// in exactly one row of each list and the counts sum back to closed.
func closedBreakdowns(b *Bucket, meta map[string]issueMeta, titleByKey map[string]string) ([]TypeCount, []EpicCount) {
	byType := map[string]*TypeCount{}
	byEpic := map[string]*EpicCount{}
	for _, itemID := range b.closedItems {
		m := meta[itemID]
		if m.key == "" {
			continue
		}
		t, ok := byType[m.issueTypeID]
		if !ok {
			t = &TypeCount{IssueTypeID: m.issueTypeID, IssueType: m.issueType}
			byType[m.issueTypeID] = t
		}
		t.Count++
		t.Keys = append(t.Keys, m.key)

		e, ok := byEpic[m.epicKey]
		if !ok {
			e = &EpicCount{EpicKey: m.epicKey, Title: titleByKey[m.epicKey]}
			byEpic[m.epicKey] = e
		}
		e.Count++
		e.Keys = append(e.Keys, m.key)
	}
	types := make([]TypeCount, 0, len(byType))
	for _, t := range byType {
		sort.Strings(t.Keys)
		types = append(types, *t)
	}
	sort.SliceStable(types, func(i, j int) bool {
		if types[i].Count != types[j].Count {
			return types[i].Count > types[j].Count
		}
		return types[i].IssueTypeID < types[j].IssueTypeID
	})
	epics := make([]EpicCount, 0, len(byEpic))
	for _, e := range byEpic {
		sort.Strings(e.Keys)
		epics = append(epics, *e)
	}
	sort.SliceStable(epics, func(i, j int) bool {
		if epics[i].Count != epics[j].Count {
			return epics[i].Count > epics[j].Count
		}
		return epics[i].EpicKey < epics[j].EpicKey
	})
	return types, epics
}

// bucketUnplanned is work that both arrived and left inside the bucket: it
// was not on the board when the bucket opened. A subset of closed by
// construction — it walks the same closedItems.
func bucketUnplanned(b *Bucket, meta map[string]issueMeta) KeyCount {
	out := KeyCount{Keys: []string{}}
	for _, itemID := range b.closedItems {
		m := meta[itemID]
		if m.key == "" {
			continue
		}
		if t, ok := parseTime(m.created); ok && inBucket(b, t) {
			out.Keys = append(out.Keys, m.key)
		}
	}
	sort.Strings(out.Keys)
	out.Count = len(out.Keys)
	return out
}

// bucketCyclePoints is the cycle sample with its values: the same items
// CycleKeys names, one row each, so the reader sees the distribution the two
// percentiles summarize.
func bucketCyclePoints(b *Bucket, meta map[string]issueMeta, issCycle map[string]cycleRow) []CyclePoint {
	out := make([]CyclePoint, 0, len(b.cycleItems))
	for _, itemID := range b.cycleItems {
		m := meta[itemID]
		ci, ok := issCycle[itemID]
		if !ok || m.key == "" {
			continue
		}
		at := ci.resolved
		if t, ok := parseTime(ci.resolved); ok {
			at = t.Format(time.RFC3339)
		}
		out = append(out, CyclePoint{Key: m.key, Summary: m.summary, ResolvedAt: at, Days: roundDays(ci.hours / 24)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// seenMoved is the two halves of attention against activity: issues the
// person opened in this bucket that no changelog row touched, and issues the
// changelog moved that the person never opened. With no recorded issue
// visits neither question can be answered, so both come back empty and the
// report carries a note saying why — an empty "seen" would otherwise make
// every moved issue look unseen.
func seenMoved(b *Bucket, meta map[string]issueMeta, issueVisits []visit, in materialsInput, visitsEmpty bool) (KeySet, KeySet) {
	seen := KeySet{Keys: []string{}}
	moved := KeySet{Keys: []string{}}
	if visitsEmpty {
		return seen, moved
	}
	visitedKeys := map[string]bool{}
	for _, v := range issueVisits {
		if inBucket(b, v.at) {
			visitedKeys[v.key] = true
		}
	}
	movedKeys := map[string]bool{}
	for _, w := range in.writes {
		if !inBucket(b, w.at) {
			continue
		}
		if m, ok := meta[w.item]; ok && m.key != "" {
			movedKeys[m.key] = true
		}
	}
	for k := range visitedKeys {
		if !movedKeys[k] {
			seen.Keys = append(seen.Keys, k)
		}
	}
	for k := range movedKeys {
		if !visitedKeys[k] {
			moved.Keys = append(moved.Keys, k)
		}
	}
	sort.Strings(seen.Keys)
	sort.Strings(moved.Keys)
	return seen, moved
}

// metricLine matches the first line of a retro-action's description when it
// names the metric the action is about: "metric: wip age max". The name is
// the table's own row name, so a reader can point at the row the action was
// supposed to move.
func parseActionMetric(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		rest, ok := strings.CutPrefix(strings.ToLower(trimmed), "metric:")
		if !ok {
			return ""
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

// bucketMetric reads one table row out of a bucket by its printed name. An
// unknown name — an action naming a metric this report does not have — reads
// as absent, never as zero.
func bucketMetric(b Bucket, name string) *float64 {
	f := func(v *float64) *float64 {
		if v == nil {
			return nil
		}
		out := roundDays(*v)
		return &out
	}
	i := func(v *int) *float64 {
		if v == nil {
			return nil
		}
		out := float64(*v)
		return &out
	}
	switch name {
	case "sessions":
		out := float64(b.Sessions)
		return &out
	case "resume (median)":
		if b.Resume == nil {
			return nil
		}
		out := math.Round(*b.Resume*1000) / 1000
		return &out
	case "wip age p85":
		return f(b.WipP85)
	case "wip age max":
		return f(b.WipMax)
	case "in progress":
		return i(b.InProg)
	case "closed":
		return i(b.Closed)
	case "cycle p50":
		return f(b.CycleP50)
	case "cycle p85":
		return f(b.CycleP85)
	case "mismatch":
		out := float64(b.Mismatch)
		return &out
	}
	return nil
}

// resolveActions turns the loaded retro-action issues into the document's
// rows: newest first, capped, with then/now read out of the buckets. "Then"
// is the value in the bucket the action was created in — what the team was
// looking at when they decided — and "now" is the last bucket's. An action
// created outside the report's window has no "then": the honest answer is
// null, not the nearest bucket.
func resolveActions(rows []actionRow, buckets []Bucket) []Action {
	out := make([]Action, 0, len(rows))
	for _, r := range rows {
		if len(out) >= MaxActions {
			break
		}
		a := Action{
			Key:            r.key,
			Summary:        r.summary,
			StatusCategory: r.category,
			CreatedAt:      r.created,
			Metric:         parseActionMetric(r.body),
		}
		if t, ok := parseTime(r.created); ok {
			a.CreatedAt = t.Format(time.RFC3339)
		}
		if r.resolvedAt != "" {
			v := r.resolvedAt
			if t, ok := parseTime(r.resolvedAt); ok {
				v = t.Format(time.RFC3339)
			}
			a.ResolvedAt = &v
		}
		if a.Metric != "" && len(buckets) > 0 {
			for i := range buckets {
				if !r.createdAt.IsZero() && inBucket(&buckets[i], r.createdAt) {
					a.Then = bucketMetric(buckets[i], a.Metric)
					break
				}
			}
			a.Now = bucketMetric(buckets[len(buckets)-1], a.Metric)
		}
		out = append(out, a)
	}
	return out
}

// materialDefinitions are the sentences the footer and the JSON document
// print for the names this file adds — the same contract the nine rows have:
// every name says exactly what it counts, in the bucket's own noun.
func (r Report) materialDefinitions() [][2]string {
	b := r.BucketNoun()
	// The two sprint-only surprise kinds are described only where they can
	// occur. Under week columns they are empty by construction, and a footer
	// that explains "joined the sprint after it began" beside columns headed
	// by dates describes a table that is not there (the same rule the nine
	// rows follow, GDK-1693).
	surprises := "events the plan did not have: reopened (a done issue left done during the " + b +
		", detail is the recorded reopen reason) and reversal (four or more status moves inside the " + b + ")"
	if r.BySprint {
		surprises += ", plus added_after_start (joined this sprint after it began) and carried (arrived carrying a carryover count)"
	}
	return [][2]string{
		{"aging", "every issue in progress now, oldest first, aged from its last status change (updated_at where the mirror recorded none); p85_days is the nearest-rank 85th percentile of the same ages"},
		{"events", "what happened inside the " + b + ", oldest first: created, started (entered a progress status), resolved (entered a done status), reopened (left one), sprint_in, sprint_out, and comment (detail is the author)"},
		{"surprises", surprises},
		{"closed_by_type", "the " + b + "'s closures grouped by issue type id, largest group first; the counts sum to closed"},
		{"closed_by_epic", "the same closures grouped by epic key, largest group first; the empty epic key is the closures under no epic"},
		{"unplanned", "issues that were both created and closed inside the " + b + " — work that was not on the board when it opened"},
		{"cycle_points", "one row per cycle sample behind cycle p50 and cycle p85: the issue, when it resolved, and its cycle in days"},
		{"seen_not_moved", "issues opened during the " + b + " (local visits) that no changelog row touched in it"},
		{"moved_not_seen", "issues the changelog moved during the " + b + " that were never opened in it"},
		{"actions", "issues labelled retro-action, newest first: what a past retrospective decided to do; an action whose description opens with \"metric: <row name>\" carries that row's value in the " + b + " it was created in and its value now"},
	}
}
