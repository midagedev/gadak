package snapshot

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"
)

func planIssues(src []issueRow, scale int) ([]plannedIssue, rotationStats) {
	n := len(src)
	target := n
	if scale > n {
		target = scale
	}
	out := make([]plannedIssue, 0, target)
	for i := 0; i < n; i++ {
		out = append(out, plannedIssue{src: src[i], cloneSeq: 0})
	}
	// GDK-1558: a clone copies its source whole, so before rotation a slice
	// narrowed to one (assignee, priority) pair — "my Highest issues in
	// progress" — was one title repeated once per clone. Rotating both facets
	// through the value pools the source itself carries is what makes every
	// narrow slice mix sources. The plan is built once from src, so it stays a
	// pure function of source order and k (Seed keys lifetimes, not this plan).
	plan := newFacetPlan(src)

	seq := 1
	for len(out) < target {
		// off cycles the sources in rounds, so srcPos is the source's own
		// position and k its own clone counter (1, 2, …) — not the global
		// cloneSeq, which keeps rising and would hand every source in a round
		// the same rotation offset.
		off := len(out) - n
		srcPos := off % n
		k := off/n + 1
		p := plannedIssue{src: src[srcPos], cloneSeq: seq}
		p.rotAssignee, p.rotPriority = plan.at(srcPos, k)
		out = append(out, p)
		seq++
	}
	return out, plan.stats()
}

// rotationStats is what the rotation reports about itself, so `gadak snapshot`
// can say over what it rotated instead of leaving the reader to open the
// output in SQLite (GDK-1558).
type rotationStats struct {
	assignees, priorities, sequenceLen int
}

// facetPlan holds the two value pools and, for each, a sequence of pool
// indices as long as the source itself. The sequence is the whole point: a
// flat cycle through the pool hands every value an equal share, which turned
// the shipped demo's realistic Medium-heavy priority menu into a near-uniform
// one (measured: Medium 50.9% of the source, 23.4% of a flat-cycle 20k
// snapshot). Each sequence instead repeats a value exactly as many times as
// the source carries it, spread evenly, so the output keeps the source's
// shape.
type facetPlan struct {
	people    []assigneeTriple
	prios     []priorityTriple
	peopleSeq []int // pool indices, one entry per source row
	prioSeq   []int
	prioStep  int
}

func newFacetPlan(src []issueRow) facetPlan {
	people, peopleW := assigneePool(src)
	prios, priosW := priorityPool(src)
	plan := facetPlan{
		people:    people,
		prios:     prios,
		peopleSeq: smoothSequence(peopleW),
		prioSeq:   smoothSequence(priosW),
	}
	// Both sequences are exactly len(src) long, so walking them at the same
	// rate would lock them in phase: the pair a clone gets would depend only
	// on (srcPos + k), and the rare corners of the grid would never be filled.
	// Measured on examples/demo.db at 20k: stepping in lockstep realised 23 of
	// the 25 (assignee, priority) cells, leaving Alex/Highest and Priya/Lowest
	// empty. Advancing the priority sequence by a stride coprime to its length
	// realises all 25 and leaves both marginals untouched — every stride still
	// sweeps the whole sequence once per round of sources.
	plan.prioStep = coprimeStride(len(people), len(plan.prioSeq))
	return plan
}

func (p facetPlan) stats() rotationStats {
	return rotationStats{
		assignees:   len(p.people),
		priorities:  len(p.prios),
		sequenceLen: len(p.peopleSeq),
	}
}

// at picks the (assignee, priority) bags for the k-th clone of the source at
// srcPos. Neighbouring sources start at different phases, so a run of clones
// from one source walks the sequence rather than sitting in one cell. Returns
// nil, nil when a pool is degenerate — the caller then leaves the clone's
// facets as the source wrote them.
func (p facetPlan) at(srcPos, k int) (*assigneeTriple, *priorityTriple) {
	if len(p.peopleSeq) == 0 || len(p.prioSeq) == 0 {
		return nil, nil
	}
	a := p.people[p.peopleSeq[(srcPos+k)%len(p.peopleSeq)]]
	pr := p.prios[p.prioSeq[(srcPos+k*p.prioStep)%len(p.prioSeq)]]
	return &a, &pr
}

// smoothSequence spreads weighted pool entries over a sequence of length
// sum(weights), each entry appearing exactly its weight and recurring at even
// spacing proportional to its share. This is smooth weighted round-robin (the
// nginx/LVS scheduler): add every weight to its running credit, hand the slot
// to the largest credit, then charge that winner the total. Ties go to the
// lower index, so the sequence is a pure function of the weights.
func smoothSequence(weights []int) []int {
	total := 0
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total == 0 {
		return nil
	}
	credit := make([]int, len(weights))
	out := make([]int, 0, total)
	for range total {
		best := -1
		for i, w := range weights {
			if w <= 0 {
				continue
			}
			credit[i] += w
			if best < 0 || credit[i] > credit[best] {
				best = i
			}
		}
		credit[best] -= total
		out = append(out, best)
	}
	return out
}

// coprimeStride is the smallest stride at or above base that shares no factor
// with length, so stepping by it visits every position of a length-long
// sequence. base is the assignee-pool size: one priority step per sweep of the
// people is the spacing the flat cycle used, and this only nudges it up to the
// nearest value that does not phase-lock.
func coprimeStride(base, length int) int {
	if length <= 1 {
		return 1
	}
	s := base
	if s < 1 {
		s = 1
	}
	for gcd(s, length) != 1 {
		s++
	}
	return s
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func assigneeOf(r issueRow) assigneeTriple {
	return assigneeTriple{
		name:  textOf(r.issueCols["assignee"]),
		id:    textOf(r.issueCols["assignee_id"]),
		email: textOf(r.issueCols["assignee_email"]),
	}
}

func priorityOf(r issueRow) priorityTriple {
	return priorityTriple{
		name: textOf(r.issueCols["priority"]),
		id:   textOf(r.issueCols["priority_id"]),
		rank: parseRank(r.issueCols["priority_rank"]),
	}
}

// assigneePool is the distinct assignee bags the source carries, the
// unassigned one included when the source has unassigned issues, alongside how
// many source rows carry each — the weights the sequence is spread by. Sorted
// by name then id, so the pool — and with it the whole plan — is stable across
// builds no matter what order the rows arrived in.
func assigneePool(src []issueRow) ([]assigneeTriple, []int) {
	count := map[assigneeTriple]int{}
	var out []assigneeTriple
	for _, r := range src {
		a := assigneeOf(r)
		if count[a] == 0 {
			out = append(out, a)
		}
		count[a]++
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name.less(out[j].name)
		}
		return out[i].id.less(out[j].id)
	})
	weights := make([]int, len(out))
	for i, a := range out {
		weights[i] = count[a]
	}
	return out, weights
}

// priorityPool is the distinct priority bags the source carries, most urgent
// first, alongside how many source rows carry each. priority_rank is 1-based
// and 0 means "unset, or not in the site's list" (docs/DERIVE.md), so 0 sorts
// after every real rank instead of ahead of Highest. Ties break on the display
// name for stability only — nothing keys on it (project CLAUDE.md: never key
// logic on display names).
func priorityPool(src []issueRow) ([]priorityTriple, []int) {
	count := map[priorityTriple]int{}
	var out []priorityTriple
	for _, r := range src {
		p := priorityOf(r)
		if count[p] == 0 {
			out = append(out, p)
		}
		count[p]++
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := rankOrder(out[i].rank), rankOrder(out[j].rank)
		if ri != rj {
			return ri < rj
		}
		return out[i].name.less(out[j].name)
	})
	weights := make([]int, len(out))
	for i, p := range out {
		weights[i] = count[p]
	}
	return out, weights
}

// parseRank reads priority_rank out of the column bag. The destination column
// is INTEGER NOT NULL, so an unreadable or missing value is 0 — the same
// "unknown" the rest of the codebase spells.
func parseRank(v any) int {
	n, err := strconv.Atoi(asString(v))
	if err != nil {
		return 0
	}
	return n
}

func rankOrder(rank int) int {
	if rank <= 0 {
		return 1 << 30
	}
	return rank
}

// statusCategories is status_id → category, read off the source's own issue
// rows. The snapshot's own status_catalog is derived from exactly this pair
// (deriveStatusCatalog), and it is the only legitimate way to ask what a
// changelog transition means: `to_value = 'In Progress'` is silently zero rows
// on a localized site (project CLAUDE.md).
func statusCategories(planned []plannedIssue) map[string]string {
	out := map[string]string{}
	for i := range planned {
		id := asString(planned[i].src.issueCols["status_id"])
		cat := asString(planned[i].src.issueCols["status_category"])
		if id != "" && cat != "" {
			out[id] = cat
		}
	}
	return out
}

// statusFlow is where the two beats that define flow sit in one issue's
// changelog: the first transition into an in-progress category (started_at,
// and so the left edge of both WIP age and cycle time), the first and the last
// into done (the one to precede with a synthetic start, and resolved_at). All
// are -1 when the history has none.
type statusFlow struct {
	firstIP, firstDone, lastDone int
}

// flowOf picks by timestamp, not by slice position: loadTableMaps orders the
// changelog by row id for determinism, which on a mirrored history is not the
// order the transitions happened in. Reading "the last done row" off the slice
// would have hung resolved_at on whichever Done row sorted last by id.
// store.Derive resolves the same beats by sorting on At, and these two must
// agree — it is Derive that writes the published columns.
//
// A row with no readable stamp is skipped, which is also what Derive does.
func flowOf(rows []map[string]any, cats map[string]string) statusFlow {
	f := statusFlow{firstIP: -1, firstDone: -1, lastDone: -1}
	var ipAt, firstDoneAt, lastDoneAt time.Time
	for i, row := range rows {
		if asString(row["field"]) != "status" {
			continue
		}
		at, ok := parseTime(asString(row["at"]))
		if !ok {
			continue
		}
		switch cats[asString(row["to_id"])] {
		case "inprogress":
			if f.firstIP < 0 || at.Before(ipAt) {
				f.firstIP, ipAt = i, at
			}
		case "done":
			if f.firstDone < 0 || at.Before(firstDoneAt) {
				f.firstDone, firstDoneAt = i, at
			}
			if f.lastDone < 0 || at.After(lastDoneAt) {
				f.lastDone, lastDoneAt = i, at
			}
		}
	}
	return f
}

// inProgressStatus is the in-progress status a source uses, with its display
// name: the id carried by the most of that source's in-progress issues. Ties
// break on the id so the pick is a pure function of the source.
func inProgressStatus(planned []plannedIssue) map[string][2]string {
	type tally struct {
		count int
		name  string
	}
	per := map[string]map[string]*tally{}
	for i := range planned {
		p := &planned[i]
		if asString(p.src.issueCols["status_category"]) != "inprogress" {
			continue
		}
		id := asString(p.src.issueCols["status_id"])
		if id == "" {
			continue
		}
		src := asString(p.src.itemCols["source_id"])
		if per[src] == nil {
			per[src] = map[string]*tally{}
		}
		t := per[src][id]
		if t == nil {
			t = &tally{name: asString(p.src.issueCols["status"])}
			per[src][id] = t
		}
		t.count++
	}
	out := map[string][2]string{}
	for src, ids := range per {
		best, bestT := "", (*tally)(nil)
		keys := make([]string, 0, len(ids))
		for id := range ids {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			if bestT == nil || ids[id].count > bestT.count {
				best, bestT = id, ids[id]
			}
		}
		if bestT != nil {
			out[src] = [2]string{best, bestT.name}
		}
	}
	return out
}

// ensureStartTransitions gives every closed issue an In Progress transition to
// have taken time over (GDK-1739).
//
// 110 of the shipped fixture's 166 closed issues went straight to Done in the
// changelog — the seed never wrote the middle of the story — so started_at and
// cycle_hours were NULL on two thirds of the closed set and the cycle-time
// scatter drew 56 dots. No re-timing can fix that: the row does not exist. So
// one is written, immediately before the first Done transition, carrying the
// from-status the Done row used to carry; the Done row is re-pointed at it, so
// the chain still reads Backlog → In Progress → Done.
//
// This is idempotent by construction rather than by a marker: on the next
// regeneration the issue *has* an in-progress transition, so flowOf finds it
// and nothing is added. The row rides the `synth:` id namespace so a reader
// can tell it apart, and it is only written under --spread, the path that is
// already synthesizing this fixture's history.
func ensureStartTransitions(planned []plannedIssue, ch children, cats map[string]string) {
	ip := inProgressStatus(planned)
	done := map[string]bool{}
	for i := range planned {
		p := &planned[i]
		srcID := p.src.itemID
		if done[srcID] {
			continue
		}
		done[srcID] = true
		if asString(p.src.issueCols["status_category"]) != "done" {
			continue
		}
		pair, ok := ip[asString(p.src.itemCols["source_id"])]
		if !ok {
			continue
		}
		rows := ch.changelogBy[srcID]
		f := flowOf(rows, cats)
		if f.firstIP >= 0 || f.lastDone < 0 {
			continue
		}
		// The *first* Done transition is the one to precede — on a Done → Done
		// history it is not the one resolved_at reads.
		firstDone := f.firstDone
		at, okT := parseTime(asString(rows[firstDone]["at"]))
		if !okT {
			continue
		}
		// A source instant of its own, so it becomes its own beat. Only the
		// order matters — placement redraws every gap.
		taken := map[time.Time]bool{}
		for _, row := range rows {
			if t, ok := parseTime(asString(row["at"])); ok {
				taken[t] = true
			}
		}
		start := at.Add(-time.Second)
		for taken[start] {
			start = start.Add(-time.Second)
		}
		row := maps.Clone(rows[firstDone])
		row["id"] = "synth:start"
		row["at"] = formatTime(start)
		row["to_id"] = pair[0]
		row["to_value"] = pair[1]
		// The Done row now leaves In Progress rather than the backlog.
		rows[firstDone]["from_id"] = pair[0]
		rows[firstDone]["from_value"] = pair[1]
		// Oldest-first is what flowOf and newHistory read, and insertIssueBundle
		// walks the same slice, so the row is spliced in place rather than
		// appended.
		grown := make([]map[string]any, 0, len(rows)+1)
		grown = append(grown, rows[:firstDone]...)
		grown = append(grown, row)
		grown = append(grown, rows[firstDone:]...)
		ch.changelogBy[srcID] = grown
	}
}

func applySpread(planned []plannedIssue, window time.Duration, now time.Time, seed int64, ch children) {
	if window <= 0 || len(planned) == 0 {
		return
	}
	start := now.Add(-window)
	cats := statusCategories(planned)
	ensureStartTransitions(planned, ch, cats)

	// How far before the window an issue's created_at may be pulled to give a
	// closed issue room for a full cycle. The window is a presentation choice
	// — 90 days of *activity* — and an issue worked for five weeks was opened
	// before it. Bounded so nothing lands in a different year than the data.
	floor := start.Add(-time.Duration((cycleMaxHours + pickupMaxHours) * float64(time.Hour)))

	// Collect original created times for the first occurrence of each source
	// (clones share their source's relative placement index by sequence order).
	// We place ALL planned issues' created' evenly/linearly by their position
	// in the planned list (which preserves original order, then clones).
	n := len(planned)
	createds := make([]time.Time, n)
	if n == 1 {
		createds[0] = start
	} else {
		// Prefer linear map of original created range onto [start, now] for
		// originals; clones continue the sequence evenly after.
		// Simpler contract: even spacing preserves order and spans the window.
		for i := 0; i < n; i++ {
			frac := float64(i) / float64(n-1)
			createds[i] = start.Add(time.Duration(frac * float64(window)))
		}
	}

	for i := range planned {
		p := &planned[i]
		srcCreated, okC := parseTime(p.src.createdAt)
		if !okC {
			srcCreated = start
		}
		// The source high-water mark is the newest stamp the issue actually
		// carries, not items.updated_at alone: mapTime clamps anything past
		// srcHi onto dstHi, so an issue whose last changelog entry sits after
		// its updated stamp would collapse its own tail onto one instant.
		srcUpdated := srcCreated
		if u, ok := parseTime(p.src.updatedAt); ok && u.After(srcUpdated) {
			srcUpdated = u
		}
		events := 0
		for _, row := range ch.changelogBy[p.src.itemID] {
			events++
			if t, ok := parseTime(asString(row["at"])); ok && t.After(srcUpdated) {
				srcUpdated = t
			}
		}
		for _, row := range ch.commentsBy[p.src.itemID] {
			events++
			for _, f := range []string{"created_at", "updated_at"} {
				if t, ok := parseTime(asString(row[f])); ok && t.After(srcUpdated) {
					srcUpdated = t
				}
			}
		}
		for _, row := range ch.attachmentsBy[p.src.itemID] {
			events++
			if t, ok := parseTime(asString(row["created_at"])); ok && t.After(srcUpdated) {
				srcUpdated = t
			}
		}

		dstCreated := createds[i]
		// An issue with no history of its own gets no synthetic lifetime:
		// there is nothing to spread inside the span, and inventing one only
		// ages a row that never moved.
		dur := srcUpdated.Sub(srcCreated)
		if events > 0 {
			dur = synthLifetime(seed, p.src.itemID, p.cloneSeq, now.Sub(dstCreated))
		}
		if dur < 0 {
			dur = 0
		}
		dstUpdated := dstCreated.Add(dur)

		h := newHistory(p.src.itemID, p.cloneSeq, ch, seed)
		// State-aware anchoring (GDK-1739). The default spread above is the
		// provisional pass: it chooses the resolution instant a closed issue
		// keeps, so the weekly closed rate is not moved by giving the issue a
		// cycle time.
		var anchors map[int]time.Time
		if !h.empty() {
			times := h.place(dstCreated, dstUpdated, nil)
			dstCreated, dstUpdated, anchors = shapeFlow(p, h, times, cats, ch, now, floor, seed, dstCreated, dstUpdated)
		}

		p.useMap = true
		p.srcLo, p.srcHi = srcCreated, srcUpdated
		p.dstLo, p.dstHi = dstCreated, dstUpdated
		p.zeroSpan = !srcUpdated.After(srcCreated)

		if h.empty() {
			p.events = h.emit(nil)
		} else {
			p.events = h.emit(h.place(dstCreated, dstUpdated, anchors))
		}

		p.createdAt = formatTime(dstCreated)
		p.updatedAt = formatTime(dstUpdated)
		p.itemCreatedAt = p.createdAt
		p.itemUpdatedAt = p.updatedAt
		p.itemSyncedAt = formatTime(now)

		// Remap issue-level optional timestamps. status_changed_at,
		// resolved_at and reopened_at are re-derived from the placed changelog
		// by store.BackfillFlow before the snapshot is published (GDK-1684,
		// GDK-1720), so these are a floor, not the published value.
		if v, ok := p.src.issueCols["status_changed_at"].(string); ok && v != "" {
			p.statusChangedAt = mapOrEven(v, p, 0, 1)
		}
		if v, ok := p.src.issueCols["resolved_at"].(string); ok && v != "" {
			p.resolvedAt = mapOrEven(v, p, 0, 1)
		}
		if v, ok := p.src.issueCols["reopened_at"].(string); ok && v != "" {
			p.reopenedAt = mapOrEven(v, p, 0, 1)
		}
		if v, ok := p.src.issueCols["assignee_changed_at"].(string); ok && v != "" {
			p.assigneeChangedAt = mapOrEven(v, p, 0, 1)
		}
	}
}

// pullBack is created_at against the instant work started on the issue. The
// even placement over the window is kept whenever it already sits before the
// start — an issue opened ten weeks ago and picked up yesterday is the normal
// case, and rewriting its created_at would flatten the age distribution the
// backlog view reads. It is pulled back only when the even placement lands
// after the start, by an exponential wait: the time the issue sat before
// someone took it. floor bounds how far outside the window that may reach.
func pullBack(even, start time.Time, u float64, floor time.Time) time.Time {
	created := even
	if want := start.Add(-expDur(u, pickupMeanHours, pickupMaxHours)); created.After(want) {
		created = want
	}
	if created.Before(floor) {
		created = floor
	}
	return created
}

// shapeFlow turns the provisional placement into the state-aware one: it
// returns the issue's created_at, its last-event instant, and the beats that
// must be pinned. See the file comment in lifetime.go for why each state is
// shaped the way it is.
func shapeFlow(p *plannedIssue, h history, times []time.Time, cats map[string]string,
	ch children, now, floor time.Time, seed int64, dstLo, dstHi time.Time,
) (time.Time, time.Time, map[int]time.Time) {
	f := flowOf(ch.changelogBy[p.src.itemID], cats)
	category := asString(p.src.issueCols["status_category"])
	noise := func(salt string) float64 { return issueNoise(seed, p.src.itemID, p.cloneSeq, salt) }

	beat := func(row int) int {
		if row < 0 || row >= len(h.chBeat) {
			return -1
		}
		return h.chBeat[row]
	}
	last := len(h.beats) - 1

	switch category {
	case "inprogress":
		gIP := beat(f.firstIP)
		if gIP < 0 {
			return dstLo, dstHi, nil
		}
		age := logNormalDur(noise("wipage"), wipAgeMedianHours, wipAgeSigma, wipAgeMaxHours)
		tIP := now.Add(-age)
		created := pullBack(dstLo, tIP, noise("pickup"), floor)
		if !tIP.After(created.Add(minBeatGap)) {
			tIP = created.Add(minBeatGap)
		}
		if gIP == last {
			return created, tIP, nil
		}
		// Whatever the issue did after it started — comments, a reassignment —
		// happened between then and now, not all at once at either end.
		tail := now.Sub(tIP)
		hi := tIP.Add(time.Duration((0.15 + 0.7*noise("tail")) * float64(tail)))
		if !hi.After(tIP.Add(minBeatGap)) {
			hi = tIP.Add(minBeatGap)
		}
		return created, hi, map[int]time.Time{gIP: tIP}

	case "done":
		gIP, gDone := beat(f.firstIP), beat(f.lastDone)
		if gIP < 0 || gDone < 0 || gIP >= gDone {
			return dstLo, dstHi, nil
		}
		tDone := times[gDone]
		cycle := logNormalDur(noise("cycle"), cycleMedianHours, cycleSigma, cycleMaxHours)
		tIP := tDone.Add(-cycle)
		created := pullBack(dstLo, tIP, noise("pickup"), floor)
		if !tIP.After(created.Add(minBeatGap)) {
			tIP = created.Add(minBeatGap)
		}
		if !tDone.After(tIP.Add(minBeatGap)) {
			tDone = tIP.Add(minBeatGap)
		}
		hi := dstHi
		if gDone == last || !hi.After(tDone) {
			hi = tDone
		}
		anchors := map[int]time.Time{gIP: tIP}
		if gDone != last {
			anchors[gDone] = tDone
		}
		return created, hi, anchors
	}
	return dstLo, dstHi, nil
}

func mapOrEven(s string, p *plannedIssue, idx, total int) string {
	if !p.useMap {
		return s
	}
	if p.zeroSpan {
		if total <= 1 {
			return formatTime(p.dstLo)
		}
		frac := float64(idx) / float64(total-1)
		return formatTime(p.dstLo.Add(time.Duration(frac * float64(p.dstHi.Sub(p.dstLo)))))
	}
	return mapTimeString(s, p.srcLo, p.srcHi, p.dstLo, p.dstHi)
}

func maxKeyNums(issues []issueRow) map[string]int {
	m := map[string]int{}
	for _, is := range issues {
		proj, num, ok := splitKey(is.key)
		if !ok {
			proj = is.projectKey
			if proj == "" {
				proj = "SNAP"
			}
		}
		if num > m[proj] {
			m[proj] = num
		}
		// Also track by project_key column.
		if is.projectKey != "" && num > m[is.projectKey] {
			m[is.projectKey] = num
		}
	}
	return m
}

func nextKey(projectKey, srcKey string, next map[string]int) string {
	proj, _, ok := splitKey(srcKey)
	if !ok || proj == "" {
		proj = projectKey
	}
	if proj == "" {
		proj = "SNAP"
	}
	next[proj]++
	return fmt.Sprintf("%s-%d", proj, next[proj])
}

func splitKey(key string) (string, int, bool) {
	i := strings.LastIndex(key, "-")
	if i <= 0 || i == len(key)-1 {
		return "", 0, false
	}
	n, err := strconv.Atoi(key[i+1:])
	if err != nil {
		return "", 0, false
	}
	return key[:i], n, true
}
