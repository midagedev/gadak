package retro

// The text sections under the table: aging, surprises, closed by type, and
// the one-paragraph summary that closes every run. The table answers how
// much; these answer what, and the summary answers "so what happened" in one
// sentence a person can paste into a standup.
//
// Same posture as the table above them — every number carries its issue keys
// or is a count of a list printed right there, no scores, no people. English,
// like the definitions: this is the CLI's own footer, and a surface that
// translates writes its own from the JSON document.

import (
	"fmt"
	"strconv"
	"strings"
)

// agingTop caps the aging section. Ten is a screen; the full list is in the
// JSON document and behind `--open aging`.
const agingTop = 10

// Explanations are the --explain paragraphs: three sentences per section —
// what it is, why it is here, how to read it. Keyed by the section heading
// the sections print, so the flag adds prose without moving anything.
func (r Report) Explanations() map[string]string {
	b := r.BucketNoun()
	return map[string]string{
		"aging": "What: every issue in progress right now, oldest first, aged from its last status change. " +
			"Why: one in-progress count per " + b + " hides which items have stopped moving, and the tail is where the queue actually is. " +
			"How to read: the p85 is the line most of the queue sits under, so a row far above it is one item, not a trend.",
		"surprises": "What: the events this " + b + " had that no plan contained — work that came back, work that churned through statuses, work that arrived or was carried in. " +
			"Why: a retrospective that only reads the totals discusses the " + b + " that was planned, not the one that happened. " +
			"How to read: each line carries the reason the mirror recorded, so start from the reason and not from the count.",
		"closed by type": "What: the " + b + "'s closures grouped by issue type. " +
			"Why: closing nine issues means something different when eight are bugs than when eight are features. " +
			"How to read: the groups sum to the closed row above, so a shift in the mix is visible even when the total does not move.",
	}
}

// Sections is the text under the table: the three material sections, and
// then the one-line summary. explain adds the paragraph under each heading.
func (r Report) Sections(explain bool) string {
	var b strings.Builder
	ex := r.Explanations()
	head := func(name string) {
		b.WriteString("\n" + name + ":\n")
		if explain {
			b.WriteString("  " + ex[name] + "\n")
		}
	}

	head("aging")
	if len(r.Aging.Items) == 0 {
		b.WriteString("  nothing in progress\n")
	} else {
		if r.Aging.P85 != nil {
			fmt.Fprintf(&b, "  p85 %s over %d in progress\n", FormatDays(*r.Aging.P85), len(r.Aging.Items))
		}
		for i, it := range r.Aging.Items {
			if i >= agingTop {
				fmt.Fprintf(&b, "  … %d more\n", len(r.Aging.Items)-agingTop)
				break
			}
			fmt.Fprintf(&b, "  %-10s %8s  %s\n", it.Key, FormatDays(it.Days), it.Summary)
		}
	}

	head("surprises")
	any := false
	for _, bk := range r.Buckets {
		if len(bk.Surprises) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(&b, "  %s\n", bk.Label())
		for _, s := range bk.Surprises {
			// Key first because it is what a reader pastes, then the title so
			// the line names the work instead of an identifier (GDK-1746) —
			// the aging section above prints the same pair. The detail the
			// mirror recorded stays last: a surprise without its reason is an
			// accusation.
			line := fmt.Sprintf("    %-18s %-10s", s.Kind, s.Key)
			if s.Summary != "" {
				line += " " + s.Summary
			}
			if s.Detail != "" {
				line += " — " + s.Detail
			}
			b.WriteString(strings.TrimRight(line, " ") + "\n")
		}
	}
	if !any {
		b.WriteString("  none\n")
	}

	head("closed by type")
	any = false
	for _, bk := range r.Buckets {
		if len(bk.ClosedByType) == 0 {
			continue
		}
		any = true
		parts := make([]string, 0, len(bk.ClosedByType))
		for _, tc := range bk.ClosedByType {
			name := tc.IssueType
			if name == "" {
				name = "type " + tc.IssueTypeID
			}
			parts = append(parts, fmt.Sprintf("%s %d", name, tc.Count))
		}
		fmt.Fprintf(&b, "  %-24s %s\n", bk.Label(), strings.Join(parts, ", "))
	}
	if !any {
		b.WriteString("  nothing closed\n")
	}

	b.WriteString("\n" + r.Summary() + "\n")
	return b.String()
}

// Summary is the last line of every run: one sentence about the last bucket,
// the thing a reader repeats out loud. Clauses that have nothing to say are
// left out rather than printed as zeros — "reopened 0" is noise, and a
// sentence made only of zeros would say the bucket was empty when it was
// merely quiet.
func (r Report) Summary() string {
	if len(r.Buckets) == 0 {
		return "nothing to report"
	}
	last := r.Buckets[len(r.Buckets)-1]
	var parts []string
	if last.Closed != nil {
		s := "closed " + strconv.Itoa(*last.Closed)
		if last.Unplanned.Count > 0 {
			s += fmt.Sprintf(" (%d unplanned)", last.Unplanned.Count)
		}
		parts = append(parts, s)
	}
	byKind := map[string]int{}
	for _, s := range last.Surprises {
		byKind[s.Kind]++
	}
	// GDK-1690: on an origin with no changelog this is unknown, not zero, and
	// the sentence leaves out what it cannot say rather than printing a number
	// that reads as "no regressions".
	if n := byKind[SurpriseReopened]; n > 0 && !r.ReopenUnavailable {
		parts = append(parts, "reopened "+strconv.Itoa(n))
	}
	if n := byKind[SurpriseReversal]; n > 0 {
		parts = append(parts, "churned "+strconv.Itoa(n))
	}
	if len(r.Aging.Items) > 0 {
		parts = append(parts, "oldest in progress "+FormatDays(r.Aging.Items[0].Days))
	}
	if n := byKind[SurpriseAddedAfterStart]; n > 0 {
		parts = append(parts, "added after sprint start "+strconv.Itoa(n))
	}
	if n := byKind[SurpriseCarried]; n > 0 {
		parts = append(parts, "carried in "+strconv.Itoa(n))
	}
	if len(parts) == 0 {
		return last.Label() + ": nothing moved"
	}
	return last.Label() + ": " + strings.Join(parts, ", ")
}
