package snapshot

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"time"
)

// Synthetic issue lifetimes for --spread (GDK-1720).
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
// The fix is to stop treating the source span as a fact worth preserving and
// synthesize one: a per-issue lifetime drawn from a log-normal, keyed by
// (seed, source item id, clone sequence) so it is deterministic and stable
// across regenerations. The issue's own events keep their relative order and
// spacing — they are still linearly mapped through mapOrEven — they are just
// mapped onto a span measured in days instead of seconds.
//
// Only the end moves. created_at stays exactly where the even placement put
// it, because the sprint columns read it: carryover_count counts the sprints
// that began after an issue was created, and "joined after the sprint began"
// is first_sprint_at against start_at. Pulling created_at backwards to hold
// the resolved_at curve still would have rewritten both.
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

// issueNoise is a deterministic uniform in [0,1) for one issue and one axis.
// FNV-1a over the tuple rather than a sequential RNG: the value must not
// depend on how many issues were planned before this one, so that adding an
// issue to the source does not reshuffle every other issue's history.
func issueNoise(seed int64, itemID string, cloneSeq int, salt string) float64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d\x00%s\x00%d\x00%s", seed, itemID, cloneSeq, salt)
	return float64(h.Sum64()>>11) / float64(uint64(1)<<53)
}

// synthLifetime draws the destination span for one issue, saturated into the
// headroom between its placed created_at and now.
func synthLifetime(seed int64, itemID string, cloneSeq int, headroom time.Duration) time.Duration {
	u := issueNoise(seed, itemID, cloneSeq, "lifetime")
	if u < 1e-9 {
		u = 1e-9
	}
	if u > 1-1e-9 {
		u = 1 - 1e-9
	}
	z := math.Sqrt2 * math.Erfinv(2*u-1)
	hours := lifetimeMedianHours * math.Exp(lifetimeSigma*z)
	if hours > lifetimeMaxHours {
		hours = lifetimeMaxHours
	}
	return softCap(time.Duration(hours*float64(time.Hour)), headroom)
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

// placeEvents re-spaces an issue's history across its synthetic lifetime.
//
// Stretching the source's own spacing (the old linear map) is not enough on
// the demo fixture: the seed wrote each issue's status transitions one second
// apart inside a span that its updated stamp made hours long, so a proportional
// stretch onto five days still left the transitions minutes apart while the
// idle tail took the rest. The gaps themselves have to be redrawn.
//
// What is preserved is order and ties: rows are grouped by the instant they
// share in the source (a resolution row and its status row are one beat and
// stay one beat), the groups keep their source sequence, and the last group
// lands on dstHi so updated_at is the issue's own last event. The gaps between
// groups are exponential draws keyed by (seed, item, clone, group index) —
// uneven, the way real work is, and deterministic.
func placeEvents(p *plannedIssue, ch children, seed int64) eventPlacement {
	type slot struct {
		table string // "h" changelog, "cn"/"cu" comment created/updated, "a" attachment
		idx   int
		src   time.Time
		ok    bool
	}
	var slots []slot
	add := func(table string, idx int, raw string) {
		t, ok := parseTime(raw)
		slots = append(slots, slot{table: table, idx: idx, src: t, ok: ok})
	}
	srcID := p.src.itemID
	changes := ch.changelogBy[srcID]
	for i, row := range changes {
		add("h", i, asString(row["at"]))
	}
	comms := ch.commentsBy[srcID]
	for i, row := range comms {
		add("cn", i, asString(row["created_at"]))
		add("cu", i, asString(row["updated_at"]))
	}
	atts := ch.attachmentsBy[srcID]
	for i, row := range atts {
		add("a", i, asString(row["created_at"]))
	}

	out := eventPlacement{
		changelog:   make([]string, len(changes)),
		commentNew:  make([]string, len(comms)),
		commentUpd:  make([]string, len(comms)),
		attachments: make([]string, len(atts)),
	}

	// Distinct source instants, oldest first. Unparsable stamps ride the very
	// first group rather than being invented into the middle of the history.
	seen := map[time.Time]bool{}
	var instants []time.Time
	for _, s := range slots {
		if s.ok && !seen[s.src] {
			seen[s.src] = true
			instants = append(instants, s.src)
		}
	}
	sort.Slice(instants, func(i, j int) bool { return instants[i].Before(instants[j]) })
	if len(instants) == 0 {
		return out
	}

	// Exponential gap weights, cumulative and normalized so the last group is
	// exactly dstHi. One weight per group: created→g1, g1→g2, … g(n-1)→gn.
	weights := make([]float64, len(instants))
	sum := 0.0
	for i := range weights {
		u := issueNoise(seed, srcID, p.cloneSeq, fmt.Sprintf("gap%d", i))
		if u < 1e-9 {
			u = 1e-9
		}
		// -ln(u) is Exponential(1); the +0.15 floor keeps a beat from landing
		// on top of the one before it.
		weights[i] = -math.Log(u) + 0.15
		sum += weights[i]
	}
	span := p.dstHi.Sub(p.dstLo)
	at := make(map[time.Time]string, len(instants))
	acc := 0.0
	for i, inst := range instants {
		acc += weights[i]
		frac := acc / sum
		if i == len(instants)-1 {
			frac = 1
		}
		at[inst] = formatTime(p.dstLo.Add(time.Duration(frac * float64(span))))
	}

	first := at[instants[0]]
	for _, s := range slots {
		v := first
		if s.ok {
			v = at[s.src]
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
