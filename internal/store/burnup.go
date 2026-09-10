package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrSprintNotFound is what SprintBurnup answers for an id the mirror's
// sprints table does not hold. The server maps it to 404 and the CLI prints
// it beside the pointer to `gadak sprint list`, which has the ids.
var ErrSprintNotFound = errors.New("store: no such sprint in the mirror")

// burnupMaxDays bounds the daily series. A sprint longer than this is not a
// sprint — it is a corrupt window or a far-future end date — and an
// unbounded day loop over hostile stamps is how a read endpoint learns to
// allocate. The series is truncated, not errored: the days it did have are
// true and the reader can see where it stops.
const burnupMaxDays = 366

// BurnupDay is one calendar day of the burn-up, UTC-bucketed: an event
// counts from the day its stamp lands in. Scope is how many issues were in
// the sprint that day; started and completed are counted inside that scope,
// so 0 ≤ completed ≤ started ≤ scope holds by construction.
type BurnupDay struct {
	Date      string `json:"date"`
	Scope     int    `json:"scope"`
	Started   int    `json:"started"`
	Completed int    `json:"completed"`
}

// BurnupDoc is the daily reconstruction of one sprint from the changelog —
// the data the sparkline and `gadak sprint show` read (GDK-1710). Days is
// empty on a sprint whose window cannot be placed (no start_at and no
// sprint-field event anywhere) or that has not started yet; those are
// different from a day series and the callers say so rather than drawing an
// empty chart. SourceKind is the sprint's own source kind ("linear", …) so
// the origin-capability judgement — an origin with no changelog cannot
// answer this question at all — stays with its single owner
// (retro.OriginSuppliesChangelog), one package up.
type BurnupDoc struct {
	ID         int64       `json:"id"`
	Name       string      `json:"name"`
	State      string      `json:"state"`
	StartAt    string      `json:"start_at,omitempty"`
	EndAt      string      `json:"end_at,omitempty"`
	CompleteAt string      `json:"complete_at,omitempty"`
	SourceKind string      `json:"source_kind"`
	Days       []BurnupDay `json:"days"`
}

// SprintBurnup reconstructs the sprint's scope/started/completed series from
// the mirror's own rows. Nothing is stored — data-model.md keeps
// time-in-status a computed answer, and the daily buckets are the same
// class of question (GDK-591's rule, one window wider).
//
// The walk is a state replay, not a tally: per issue it replays sprint
// membership (field='sprint' changelog rows — arrival is the id appearing
// in to_id, departure its disappearance, which reads both origin shapes the
// carry-over rule reads) and status (field='status' rows mapped through the
// status id → category catalog, never a display name), then evaluates both
// at each day boundary. That is why scope can dip and completed can hold
// while scope does: both are "what was true that day", not running totals.
//
// `now` is an input so tests and the server agree on which day an open
// sprint's series ends on.
func (db *DB) SprintBurnup(ctx context.Context, sprintID int64, now time.Time) (BurnupDoc, error) {
	// The sprint row and its source kind. The id is unique enough in
	// practice — `gadak sprint list` prints it bare and every verb takes it
	// bare — but the lookup stays honest about it: one row per (source, id)
	// and a mirror can hold several sources.
	var doc BurnupDoc
	var sourceID, kind string
	err := db.sql.QueryRowContext(ctx, `
		SELECT s.source_id, s.name, s.state,
		       COALESCE(s.start_at, ''), COALESCE(s.end_at, ''), COALESCE(s.complete_at, ''),
		       COALESCE(src.kind, '')
		FROM sprints s LEFT JOIN sources src ON src.id = s.source_id
		WHERE s.id = ?`, sprintID).
		Scan(&sourceID, &doc.Name, &doc.State, &doc.StartAt, &doc.EndAt, &doc.CompleteAt, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return BurnupDoc{}, ErrSprintNotFound
	}
	if err != nil {
		return BurnupDoc{}, err
	}
	doc.ID = sprintID
	doc.SourceKind = kind
	doc.Days = []BurnupDay{}

	cats, err := db.StatusCategories(ctx)
	if err != nil {
		return BurnupDoc{}, err
	}

	// Membership events: every sprint-field row, filtered in Go to this
	// sprint's arrivals and departures. sprintIDs reads both origin shapes
	// (the whole-membership list and the single moved-to id), the same
	// helper the carry-over rule uses — one owner for "was this id in the
	// value".
	var members []memberEvent
	sprintRows, err := db.sql.QueryContext(ctx, `
		SELECT item_id, COALESCE(at,''), COALESCE(from_id,''), COALESCE(to_id,'')
		FROM changelog WHERE field = 'sprint'`)
	if err != nil {
		return BurnupDoc{}, err
	}
	for sprintRows.Next() {
		var item, at, from, to string
		if err := sprintRows.Scan(&item, &at, &from, &to); err != nil {
			sprintRows.Close()
			return BurnupDoc{}, err
		}
		if at == "" {
			continue
		}
		fromHas, toHas := idInSprintValue(from, sprintID), idInSprintValue(to, sprintID)
		if fromHas == toHas {
			continue // renamed, or a move between other sprints
		}
		members = append(members, memberEvent{at: at, in: toHas, item: item})
	}
	if err := sprintRows.Err(); err != nil {
		sprintRows.Close()
		return BurnupDoc{}, err
	}
	sprintRows.Close()

	// The issues whose status history matters: everyone who ever touched the
	// sprint, plus everyone sitting in it now (a trimmed history still owes
	// its current row). created_at and the current category seed the replay
	// of an issue whose changelog starts mid-flight.
	type issueState struct {
		item       string
		currentCat string
		created    string
		statuses   []stampCat // category after each status event, oldest first
		memberAt   []memberEvent
		seated     bool // in the sprint by the current row alone
		initialCat string
		startedAt  string
	}
	involved := map[string]*issueState{}
	get := func(item string) *issueState {
		st := involved[item]
		if st == nil {
			st = &issueState{item: item}
			involved[item] = st
		}
		return st
	}
	currentRows, err := db.sql.QueryContext(ctx, `
		SELECT item_id, COALESCE(status_category,''), COALESCE(created_at,'')
		FROM issues_raw WHERE sprint_id = ?`, sprintID)
	if err != nil {
		return BurnupDoc{}, err
	}
	for currentRows.Next() {
		var st issueState
		if err := currentRows.Scan(&st.item, &st.currentCat, &st.created); err != nil {
			currentRows.Close()
			return BurnupDoc{}, err
		}
		st.seated = true
		involved[st.item] = &st
	}
	if err := currentRows.Err(); err != nil {
		currentRows.Close()
		return BurnupDoc{}, err
	}
	currentRows.Close()
	for _, m := range members {
		st := get(m.item)
		st.memberAt = append(st.memberAt, memberEvent{at: m.at, in: m.in})
	}

	statusRows, err := db.sql.QueryContext(ctx, `
		SELECT c.item_id, c.at, COALESCE(c.from_id,''), COALESCE(c.to_id,'')
		FROM changelog c
		WHERE c.field = 'status' AND c.at != ''
		  AND c.item_id IN (
		      SELECT item_id FROM issues_raw WHERE sprint_id = ?
		      UNION
		      SELECT item_id FROM changelog WHERE field = 'sprint')
		ORDER BY c.at`, sprintID)
	if err != nil {
		return BurnupDoc{}, err
	}
	for statusRows.Next() {
		var item, at, from, to string
		if err := statusRows.Scan(&item, &at, &from, &to); err != nil {
			statusRows.Close()
			return BurnupDoc{}, err
		}
		st, ok := involved[item]
		if !ok {
			continue // status history of an issue this sprint never touched
		}
		if len(st.statuses) == 0 {
			// The from-side of the oldest entry is the state before history
			// began — the same seed Derive's born-in-progress rule reads.
			st.initialCat = cats[from]
		}
		st.statuses = append(st.statuses, stampCat{at: at, cat: cats[to]})
	}
	if err := statusRows.Err(); err != nil {
		statusRows.Close()
		return BurnupDoc{}, err
	}
	statusRows.Close()

	// Per-issue "when did work begin": the first transition into in-progress
	// or done, else the creation stamp when the issue was already underway
	// (or finished) before its history starts. Empty means it never did.
	for _, st := range involved {
		base := st.initialCat
		if len(st.statuses) == 0 {
			base = st.currentCat
		}
		for _, sc := range st.statuses {
			if sc.cat == CategoryInProgress || sc.cat == CategoryDone {
				st.startedAt = sc.at
				break
			}
		}
		if st.startedAt == "" && (base == CategoryInProgress || base == CategoryDone) {
			st.startedAt = st.created
		}
	}

	// The window. Start: the sprint's own start, else the earliest event —
	// an undated sprint still has days when its issues moved. End: the later
	// of the completion and the planned end, clamped to today (a planned end
	// still in the future is not a day the sprint has lived through). Events
	// after the end extend it — scope creep after a close is exactly what a
	// retro wants to see — capped at burnupMaxDays from the start.
	startDay := dayOf(doc.StartAt)
	if startDay == "" {
		for _, m := range members {
			if d := dayOf(m.at); d != "" && (startDay == "" || d < startDay) {
				startDay = d
			}
		}
	}
	if startDay == "" {
		return doc, nil
	}
	today := now.UTC().Format("2006-01-02")
	endDay := dayOf(doc.CompleteAt)
	if d := dayOf(doc.EndAt); d != "" && (endDay == "" || d > endDay) {
		endDay = d
	}
	if endDay == "" || endDay > today {
		endDay = today
	}
	for _, m := range members {
		if d := dayOf(m.at); d > endDay {
			endDay = d
		}
	}
	if endDay < startDay {
		// Not started yet: no day has happened. A chart of zero days says
		// nothing and the caller must not draw one.
		return doc, nil
	}
	if capped := addDays(startDay, burnupMaxDays); endDay > capped {
		endDay = capped
	}

	for day := startDay; ; day = addDays(day, 1) {
		var scope, started, completed int
		for _, st := range involved {
			// Events govern when history exists — they are how scope dips.
			// An issue with no event at all was never in by this replay's
			// evidence, except the one the current row seats: trimmed
			// history still owes today's row, so it counts for the whole
			// window. (Real rows agree: an issue the last event removes
			// no longer holds this sprint_id.)
			var in bool
			if len(st.memberAt) > 0 {
				in = inSprintOn(st.memberAt, day)
			} else {
				in = st.seated
			}
			if !in {
				continue
			}
			scope++
			done := categoryOn(st.statuses, st.initialCat, st.currentCat, day) == CategoryDone
			if st.startedAt != "" && dayOf(st.startedAt) <= day {
				started++
			} else if done {
				// Born-done is the one path with no transition to point at —
				// a finished issue has begun by definition, so the stacked
				// bands can never invert: completed ≤ started ≤ scope always.
				started++
			}
			if done {
				completed++
			}
		}
		doc.Days = append(doc.Days, BurnupDay{Date: day, Scope: scope, Started: started, Completed: completed})
		if day == endDay {
			break
		}
	}
	return doc, nil
}

// stampCat is one status transition: the category in force from `at` on.
type stampCat struct {
	at, cat string
}

// memberEvent is one sprint-membership change: in the sprint from `at` on
// (arrival) or out of it from `at` on (departure). Item names the issue the
// walk's first pass groups by; the per-issue replay slices drop it.
type memberEvent struct {
	at   string
	in   bool
	item string
}

// idInSprintValue says whether the sprint id appears in a changelog sprint
// value — sprintIDs with the question narrowed to membership.
func idInSprintValue(v string, id int64) bool {
	for _, got := range sprintIDs(v) {
		if got == id {
			return true
		}
	}
	return false
}

// dayOf reads the UTC calendar day of a mirror stamp, "" when it is empty
// or unparseable. Days, not instants: a sprint bucket is a calendar day and
// parseStamp keeps the instant only to be thrown away here.
func dayOf(stamp string) string {
	t, ok := parseStamp(stamp)
	if !ok {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// addDays steps a YYYY-MM-DD string one day at a time — the series walker.
// String stepping stays inside the day grammar and cannot drift across a
// DST boundary the way a 24h add can.
func addDays(day string, n int) string {
	t, ok := parseStamp(day + "T00:00:00Z")
	if !ok {
		return day
	}
	return t.UTC().AddDate(0, 0, n).Format("2006-01-02")
}

// inSprintOn replays membership to a day: the newest arrival/departure
// stamped on or before it decides, and an issue with no event yet was not
// in the sprint.
func inSprintOn(events []memberEvent, day string) bool {
	in := false
	for _, e := range events {
		if dayOf(e.at) <= day {
			in = e.in
		}
	}
	return in
}

// categoryOn replays the status history to a day: the category of the
// newest transition stamped on or before it, else the seed category (the
// from-side of the oldest entry, falling back to the current row for an
// issue with no history at all).
func categoryOn(statuses []stampCat, initial, current string, day string) string {
	cat := ""
	for _, sc := range statuses {
		if dayOf(sc.at) <= day {
			cat = sc.cat
		}
	}
	if cat != "" {
		return cat
	}
	if len(statuses) > 0 {
		return initial
	}
	return current
}
