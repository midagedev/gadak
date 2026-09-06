package store

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// CategoryDone is the only status category with meaning to the derived rules.
// Every rule keys on the category, never on a status name: the internal tool
// this was extracted from matched "Reopened" / "다시 열림" and broke on any other
// site or account language (contracts/sync.md, "Localization hazard").
const CategoryDone = "done"

// DeriveInput is everything the derived-field rules need. Changelog entries
// carry only status ids, so the id -> category map has to come from the site's
// status list, which the connector supplies per batch.
type DeriveInput struct {
	// CreatedAt is the item's creation stamp (ISO-8601 UTC). An issue born
	// into an in-progress status has no transition to find, so its start is
	// its creation — see the started_at rule below.
	CreatedAt string
	// NoHistory says the origin supplies no changelog at all (Linear), so an
	// empty Changelog carries no information about the initial status and the
	// born-in-progress rule must not guess from CurrentCategory.
	NoHistory       bool
	Changelog       []ChangeEntry
	Categories      map[string]string // status id -> new | inprogress | done
	CurrentCategory string            // the issue's category right now
	Priority        string
	Priorities      []string // site priority names, most urgent first
	Comments        []Comment
	Links           []Link
	// UpdatedAt is the item's updated stamp (items.updated_at). It feeds
	// last_activity_at — the mirror's own notion of when the origin last
	// touched the issue, which can outrank every changelog entry.
	UpdatedAt string
}

// Derived holds the columns gadak computes because the source does not provide
// them. Rules are documented in docs/DERIVE.md, and a test there requires every
// field of this struct to appear in that file.
type Derived struct {
	StatusChangedAt   *string
	ResolvedAt        *string
	ReopenCount       int
	ReopenedAt        *string
	ReopenReason      string
	AssigneeChangedAt *string
	CommentCount      int
	PriorityRank      int
	ClonedFrom        string
	// StartedAt is the first transition into an in-progress category. Nil
	// when the issue never entered progress (cycle_hours is nil with it).
	StartedAt *string
	// CycleHours is ResolvedAt minus StartedAt in hours, kept only while the
	// issue is done now and the span is positive — the CycleTimeP85Hours
	// rule, stored instead of walked. Nil otherwise.
	CycleHours *float64
	// LastActivityAt is the newest of the item's updated stamp, the newest
	// changelog entry and the newest comment. Nil when all three are absent.
	// ISO-8601 UTC strings compare lexicographically, so "newest" is a string
	// max. Not a live duration: it is a stamp, and aging against it is the
	// reader's query.
	LastActivityAt *string
}

// Derive computes every derived field in one pass over the issue's changelog.
// A status id missing from the category map counts as not-done, which can only
// ever miss a reopen — never invent one.
func Derive(in DeriveInput) Derived {
	d := Derived{
		CommentCount: len(in.Comments),
		PriorityRank: priorityRank(in.Priority, in.Priorities),
		ClonedFrom:   clonedFrom(in.Links),
	}

	entries := make([]ChangeEntry, len(in.Changelog))
	copy(entries, in.Changelog)
	// ISO-8601 UTC sorts lexicographically, so oldest-first is a string sort and
	// the last write of each field below is the newest one.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].At < entries[j].At })

	for _, e := range entries {
		if e.At == "" {
			continue
		}
		at := e.At
		switch e.Field {
		case "status":
			d.StatusChangedAt = &at
			from := in.Categories[e.FromID]
			to := in.Categories[e.ToID]
			if to == CategoryInProgress && d.StartedAt == nil {
				d.StartedAt = &at
			}
			if to == CategoryDone {
				d.ResolvedAt = &at
			}
			if from == CategoryDone && to != CategoryDone {
				d.ReopenCount++
				d.ReopenedAt = &at
			}
		case "assignee":
			d.AssigneeChangedAt = &at
		}
		// LastActivityAt seeds from the newest changelog entry — any field,
		// because a priority edit is activity too: entries are sorted, so the
		// last non-empty At wins.
		d.LastActivityAt = &at
	}

	// Born in progress (2026-09-07, flow canon: work item age counts from the
	// moment work started): an issue created straight into an in-progress
	// status never has a transition INTO progress, so the loop above leaves
	// started_at empty — or, worse, pins it to a later re-entry (In Progress →
	// Review → In Progress). The earliest known category decides: the from-side
	// of the oldest status entry when there is one, else the current category
	// when the origin does supply a history and this issue simply has none. An
	// origin with no history (Linear) is excluded — an empty changelog there
	// says nothing about where the issue began, and a guessed start would be a
	// confident wrong number where NULL is the honest one.
	if !in.NoHistory && in.CreatedAt != "" {
		initial := in.CurrentCategory
		for _, e := range entries {
			if e.Field == "status" && e.At != "" {
				initial = in.Categories[e.FromID]
				break
			}
		}
		if initial == CategoryInProgress {
			at := in.CreatedAt
			d.StartedAt = &at
		}
	}

	// A resolution that was undone is not a resolution date.
	if in.CurrentCategory != CategoryDone {
		d.ResolvedAt = nil
	}
	// Cycle time only exists for a finished issue with a measurable span —
	// the CycleTimeP85Hours rule, so the stored column and any later walk of
	// the same changelog cannot disagree. A non-positive span (reopened,
	// never re-finished) is not a cycle and must not drag a percentile down.
	if in.CurrentCategory == CategoryDone && d.StartedAt != nil && d.ResolvedAt != nil {
		if start, ok := parseStamp(*d.StartedAt); ok {
			if done, ok := parseStamp(*d.ResolvedAt); ok {
				if hours := done.Sub(start).Hours(); hours > 0 {
					d.CycleHours = &hours
				}
			}
		}
	}
	// The item's own updated stamp and the newest comment compete with the
	// changelog for last_activity_at. All stamps are ISO-8601 UTC, so the
	// comparison is a string max.
	if in.UpdatedAt != "" && (d.LastActivityAt == nil || in.UpdatedAt > *d.LastActivityAt) {
		at := in.UpdatedAt
		d.LastActivityAt = &at
	}
	for _, c := range in.Comments {
		if c.CreatedAt == "" {
			continue
		}
		if d.LastActivityAt == nil || c.CreatedAt > *d.LastActivityAt {
			at := c.CreatedAt
			d.LastActivityAt = &at
		}
	}
	if d.ReopenedAt != nil {
		d.ReopenReason = reopenReason(in.Comments, *d.ReopenedAt)
	}
	return d
}

// reopenReason is the body of the earliest comment written at or after the
// last reopen — a heuristic: on teams where the person reopening explains why
// in a comment, this surfaces that explanation. Timestamps are ISO-8601 UTC
// (the connector normalizes them), so string comparison is chronological.
func reopenReason(comments []Comment, reopenedAt string) string {
	best := ""
	bestAt := ""
	for _, c := range comments {
		if c.CreatedAt == "" || c.CreatedAt < reopenedAt {
			continue
		}
		if bestAt == "" || c.CreatedAt < bestAt {
			bestAt = c.CreatedAt
			best = c.BodyText
		}
	}
	const maxLen = 1000
	if len(best) > maxLen {
		// Do not split a UTF-8 sequence mid-rune: back up to a rune start.
		i := maxLen
		for i > 0 && !utf8.RuneStart(best[i]) {
			i--
		}
		best = best[:i]
	}
	return best
}

// clonedFrom is the key of the issue this one was cloned from: the target of
// an OUTWARD link whose type name contains "clone" (Jira's default "Cloners"
// type). The clone is the issue that displays the outward phrase — "clones
// <origin>" — so the origin's key sits behind the outward direction; the
// inward side ("is cloned by") belongs to the origin and names the clone.
// This was inverted until GDK-1214 (the GDK-1204 mirror-image class): twelve
// of twelve Cloners rows on a production mirror put outward on the newer
// issue of each pair. Caveat: link type names are site configuration created
// in the site's language, so a site whose clone type carries a non-English
// name derives nothing here — there is no language-stable id to key on.
func clonedFrom(links []Link) string {
	for _, l := range links {
		if l.Direction == "outward" && strings.Contains(strings.ToLower(l.Type), "clone") {
			return l.TargetKey
		}
	}
	return ""
}

// priorityRank is the 1-based position in the site's priority list; 0 when the
// priority is unset or not in the list.
func priorityRank(priority string, list []string) int {
	if priority == "" {
		return 0
	}
	for i, p := range list {
		if p == priority {
			return i + 1
		}
	}
	return 0
}
