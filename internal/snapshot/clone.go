package snapshot

import (
	"fmt"
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
	// pure function of source order and k (Seed is still unused).
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

func applySpread(planned []plannedIssue, window time.Duration, now time.Time) {
	if window <= 0 || len(planned) == 0 {
		return
	}
	start := now.Add(-window)

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
		srcUpdated, okU := parseTime(p.src.updatedAt)
		if !okC {
			srcCreated = start
		}
		if !okU || srcUpdated.Before(srcCreated) {
			srcUpdated = srcCreated
		}
		dstCreated := createds[i]
		dur := srcUpdated.Sub(srcCreated)
		dstUpdated := dstCreated.Add(dur)
		if dstUpdated.Before(dstCreated) {
			dstUpdated = dstCreated
		}

		p.useMap = true
		p.srcLo, p.srcHi = srcCreated, srcUpdated
		p.dstLo, p.dstHi = dstCreated, dstUpdated
		p.zeroSpan = !srcUpdated.After(srcCreated)

		p.createdAt = formatTime(dstCreated)
		p.updatedAt = formatTime(dstUpdated)
		p.itemCreatedAt = p.createdAt
		p.itemUpdatedAt = p.updatedAt
		p.itemSyncedAt = formatTime(now)

		// Remap issue-level optional timestamps.
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
