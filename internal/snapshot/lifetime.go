package snapshot

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"time"
)

// Synthetic issue lifetimes and state-aware placement for --spread
// (GDK-1720, GDK-1739). This file is the single owner of *when* a spread
// issue's events land; clone.go decides which issues exist, build.go writes
// them.
//
// applySpread used to keep each issue's own created→updated duration and only
// move the pair onto the destination window. On the demo fixture that is a
// no-op that compounds: the committed file is its own source (`make
// demo-fixture` is circular), and the seed it grew from wrote every issue's
// whole history inside a few seconds. So every regeneration reproduced 534
// issues whose status transitions sat 1–2s apart, cycle_hours ≈ 0.0002, and a
// retro whose cycle percentiles, scatter, burn-up and scope lines were all
// false on the one mirror most people ever open.
//
// GDK-1720 fixed the span by synthesizing a per-issue lifetime, drawn from a
// log-normal keyed by (seed, source item id, clone sequence) so it is
// deterministic and stable across regenerations. That made the spans real but
// left them blind to the state each issue ended in, which is the defect
// GDK-1739 closes: created_at was placed evenly across the window whatever the
// issue's status, so an issue still in progress had entered progress near the
// window's start — measured on the shipped file, WIP age p50 46.8 days, p85
// 69.1, climbing seven days a week — while a closed issue's whole In Progress
// → Done gap was one exponential draw inside a five-day lifetime, cycle p50
// 1.1 days. Two products on one screen.
//
// So the placement now reads each issue's own status history in *category*
// terms (status_catalog, never a display name — project CLAUDE.md) and works
// backwards from the state the issue is in:
//
//   - in progress: the first transition into an in-progress category is put
//     wipAge before now, and created_at an exponential draw before that.
//   - done: the In Progress → Done gap is a log-normal cycle time, hung off
//     the resolution instant the default placement already chose — so the
//     weekly closed rate, which the burn-up and "closed this week" read, keeps
//     the shape it had — and created_at follows the start backwards.
//   - anything else: unchanged, the even placement over the window.
//
// created_at moves for the first two. It used to be pinned because the sprint
// columns read it (carryover_count counts sprints that began after an issue
// was created, "joined after the sprint began" is first_sprint_at against
// start_at), but pinning it is exactly what made a five-day cycle impossible
// for an issue whose created_at sat at the far end of the window. The sprint
// derivation is a gate on this instead: internal/snapshot's own tests and the
// sprint e2e spec.
const (
	// exp(mu): the median synthetic lifetime, 5 days.
	lifetimeMedianHours = 560.0
	// sigma of the log-normal, in log-hours. 1.2 puts p85 near 18 days and
	// p95 near 38 — the tail a cycle-time scatter needs to be worth drawing.
	lifetimeSigma = 1.05
	// Hard ceiling. Without it the extreme draw over ~500 samples runs to
	// most of a year and one dot owns the scatter's y-axis.
	lifetimeMaxHours = 45 * 24.0
)

// State-aware placement constants (GDK-1739, 2026-09-10). The bands they aim
// at are the ones internal/snapshot/fixture_flow_test.go measures on the
// shipped file: WIP age p50 3–10d / p85 ≤30d / max ≤60d, cycle p50 3–8d /
// p85 10–25d. A log-normal with median 5 days and sigma 1.0 lands p85 at
// 5·e^1.04 ≈ 14 days and p95 near 26, the middle of both bands, and leaves the
// aging chart the couple of genuinely stale items it exists to show.
const (
	wipAgeMedianHours = 5 * 24.0
	wipAgeSigma       = 1.0
	// 144 in-progress draws reach roughly p99.7, which an uncapped sigma of
	// 1.0 puts near 80 days. 55 keeps the oldest inside the contract's 60
	// without flattening the tail.
	wipAgeMaxHours = 55 * 24.0

	cycleMedianHours = 5 * 24.0
	cycleSigma       = 1.0
	// The scatter contract (TestDemoFixtureCycleTimesAreRealistic) wants the
	// longest cycle between 10 and 60 days.
	cycleMaxHours = 40 * 24.0

	// created_at → the first In Progress transition: an exponential wait, the
	// time an issue sat in the backlog before someone picked it up.
	pickupMeanHours = 3 * 24.0
	pickupMaxHours  = 30 * 24.0

	// Beats never land on top of each other: an anchored instant is kept at
	// least this far from the boundary it was pushed against, so created_at <
	// started_at < resolved_at survives millisecond formatting.
	minBeatGap = time.Hour
)

// issueNoise is a deterministic uniform in [0,1) for one issue and one axis.
// FNV-1a over the tuple rather than a sequential RNG: the value must not
// depend on how many issues were planned before this one, so that adding an
// issue to the source does not reshuffle every other issue's history.
func issueNoise(seed int64, itemID string, cloneSeq int, salt string) float64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d\x00%s\x00%d\x00%s", seed, itemID, cloneSeq, salt)
	return float64(h.Sum64()>>11) / float64(uint64(1)<<53)
}

// clampUnit keeps a noise draw off both ends so the inverse transforms below
// stay finite.
func clampUnit(u float64) float64 {
	if u < 1e-9 {
		return 1e-9
	}
	if u > 1-1e-9 {
		return 1 - 1e-9
	}
	return u
}

// logNormalDur is the log-normal quantile at u: median in hours, sigma in
// log-hours, truncated at maxHours.
func logNormalDur(u, median, sigma, maxHours float64) time.Duration {
	u = clampUnit(u)
	z := math.Sqrt2 * math.Erfinv(2*u-1)
	hours := median * math.Exp(sigma*z)
	if hours > maxHours {
		hours = maxHours
	}
	return time.Duration(hours * float64(time.Hour))
}

// expDur is the exponential quantile at u with the given mean, truncated.
func expDur(u, meanHours, maxHours float64) time.Duration {
	u = clampUnit(u)
	hours := -meanHours * math.Log(u)
	if hours > maxHours {
		hours = maxHours
	}
	return time.Duration(hours * float64(time.Hour))
}

// synthLifetime draws the destination span for one issue, saturated into the
// headroom between its placed created_at and now.
func synthLifetime(seed int64, itemID string, cloneSeq int, headroom time.Duration) time.Duration {
	d := logNormalDur(issueNoise(seed, itemID, cloneSeq, "lifetime"),
		lifetimeMedianHours, lifetimeSigma, lifetimeMaxHours)
	return softCap(d, headroom)
}

// softCap maps d into [0, limit) so no stamp lands after now, and does it
// with a saturating curve rather than min(): a hard clamp would pile every
// recently-created issue's last event on exactly now, which reads as a spike
// in the newest week of every weekly chart. Below about a third of limit the
// curve is within a few percent of identity.
func softCap(d, limit time.Duration) time.Duration {
	if limit <= 0 || d <= 0 {
		return 0
	}
	return time.Duration(float64(limit) * (1 - math.Exp(-float64(d)/float64(limit))))
}

// eventPlacement holds the destination stamp for every dated child row of one
// issue, indexed exactly the way children.commentsBy / attachmentsBy /
// changelogBy hold them, so insertIssueBundle can look each row up by its
// position in the same slice.
type eventPlacement struct {
	changelog   []string
	commentNew  []string
	commentUpd  []string
	attachments []string
}

// eventSlot is one dated child row: which table it came from, its index in
// that table's slice for this issue, and the source instant it carries.
type eventSlot struct {
	table string // "h" changelog, "cn"/"cu" comment created/updated, "a" attachment
	idx   int
	src   time.Time
	ok    bool
}

// history is one issue's dated rows reduced to what placement acts on: the
// distinct source instants ("beats"), in order, each with a deterministic gap
// weight, plus a map back from every row to the beat it rides.
//
// Stretching the source's own spacing (the pre-GDK-1720 linear map) is not
// enough on the demo fixture: the seed wrote each issue's status transitions
// one second apart inside a span its updated stamp made hours long, so a
// proportional stretch onto five days still left the transitions minutes apart
// while the idle tail took the rest. The gaps themselves have to be redrawn.
//
// What is preserved is order and ties: rows sharing an instant in the source
// (a resolution row and its status row are one beat) stay one beat, and the
// beats keep their source sequence.
type history struct {
	slots   []eventSlot
	beats   []time.Time // distinct source instants, oldest first
	beatOf  map[time.Time]int
	weights []float64 // gap weight *before* beat i; len == len(beats)
	// chBeat maps a changelog row's index to its beat, or -1 when the row
	// carries no readable stamp.
	chBeat   []int
	nComment int
	nAtt     int
}

func newHistory(p *plannedIssue, ch children, seed int64) history {
	var h history
	srcID := p.src.itemID
	changes := ch.changelogBy[srcID]
	comms := ch.commentsBy[srcID]
	atts := ch.attachmentsBy[srcID]
	h.nComment, h.nAtt = len(comms), len(atts)

	add := func(table string, idx int, raw string) {
		t, ok := parseTime(raw)
		h.slots = append(h.slots, eventSlot{table: table, idx: idx, src: t, ok: ok})
	}
	for i, row := range changes {
		add("h", i, asString(row["at"]))
	}
	for i, row := range comms {
		add("cn", i, asString(row["created_at"]))
		add("cu", i, asString(row["updated_at"]))
	}
	for i, row := range atts {
		add("a", i, asString(row["created_at"]))
	}

	// Distinct source instants, oldest first. Unparsable stamps ride the very
	// first beat rather than being invented into the middle of the history.
	seen := map[time.Time]bool{}
	for _, s := range h.slots {
		if s.ok && !seen[s.src] {
			seen[s.src] = true
			h.beats = append(h.beats, s.src)
		}
	}
	sort.Slice(h.beats, func(i, j int) bool { return h.beats[i].Before(h.beats[j]) })
	h.beatOf = make(map[time.Time]int, len(h.beats))
	for i, b := range h.beats {
		h.beatOf[b] = i
	}

	h.chBeat = make([]int, len(changes))
	for i := range h.chBeat {
		h.chBeat[i] = -1
	}
	for _, s := range h.slots {
		if s.table == "h" && s.ok {
			h.chBeat[s.idx] = h.beatOf[s.src]
		}
	}

	// Exponential gap weights. One per beat: created→b0, b0→b1, … The +0.15
	// floor keeps a beat from landing on top of the one before it.
	h.weights = make([]float64, len(h.beats))
	for i := range h.weights {
		u := clampUnit(issueNoise(seed, srcID, p.cloneSeq, fmt.Sprintf("gap%d", i)))
		h.weights[i] = -math.Log(u) + 0.15
	}
	return h
}

func (h history) empty() bool { return len(h.beats) == 0 }

// place lays the beats between dstLo (the issue's created_at, exclusive) and
// dstHi, which the last beat lands on exactly so updated_at is the issue's own
// last event. anchors pins named beats to named instants; every other beat is
// spread inside the segment its neighbouring anchors leave, in proportion to
// its gap weight. With no anchors this is the plain cumulative-weight spread
// GDK-1720 shipped.
//
// An anchor that would invert the history — not strictly after the previous
// fixed instant, or not before dstHi — is dropped rather than honoured, so a
// caller's arithmetic can never produce a resolved-before-started row.
func (h history) place(dstLo, dstHi time.Time, anchors map[int]time.Time) []time.Time {
	n := len(h.beats)
	out := make([]time.Time, n)
	if n == 0 {
		return out
	}
	fixed := make([]time.Time, n)
	isFixed := make([]bool, n)
	prevT := dstLo
	for i := 0; i < n-1; i++ {
		t, ok := anchors[i]
		if !ok || !t.After(prevT) || !t.Before(dstHi) {
			continue
		}
		fixed[i], isFixed[i] = t, true
		prevT = t
	}
	fixed[n-1], isFixed[n-1] = dstHi, true

	segLo, segLoT := -1, dstLo
	for i := 0; i < n; i++ {
		if !isFixed[i] {
			continue
		}
		total := 0.0
		for j := segLo + 1; j <= i; j++ {
			total += h.weights[j]
		}
		span := fixed[i].Sub(segLoT)
		acc := 0.0
		for j := segLo + 1; j <= i; j++ {
			acc += h.weights[j]
			if j == i || total <= 0 {
				out[j] = fixed[i]
				continue
			}
			out[j] = segLoT.Add(time.Duration(acc / total * float64(span)))
		}
		segLo, segLoT = i, fixed[i]
	}
	return out
}

// emit turns per-beat instants back into the per-row string stamps
// insertIssueBundle looks up by position.
func (h history) emit(times []time.Time) eventPlacement {
	out := eventPlacement{
		changelog:   make([]string, len(h.chBeat)),
		commentNew:  make([]string, h.nComment),
		commentUpd:  make([]string, h.nComment),
		attachments: make([]string, h.nAtt),
	}
	if len(times) == 0 {
		return out
	}
	first := formatTime(times[0])
	for _, s := range h.slots {
		v := first
		if s.ok {
			v = formatTime(times[h.beatOf[s.src]])
		}
		switch s.table {
		case "h":
			out.changelog[s.idx] = v
		case "cn":
			out.commentNew[s.idx] = v
		case "cu":
			out.commentUpd[s.idx] = v
		case "a":
			out.attachments[s.idx] = v
		}
	}
	// A comment cannot be edited before it was written.
	for i := range out.commentUpd {
		if out.commentUpd[i] < out.commentNew[i] {
			out.commentUpd[i] = out.commentNew[i]
		}
	}
	return out
}
