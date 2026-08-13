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
	Changelog       []ChangeEntry
	Categories      map[string]string // status id -> new | inprogress | done
	CurrentCategory string            // the issue's category right now
	Priority        string
	Priorities      []string // site priority names, most urgent first
	Comments        []Comment
	Links           []Link
}

// Derived holds the columns gadak computes because the source does not provide
// them. Rules are documented in data-model.md, "Derived field rules".
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
		switch e.Field {
		case "status":
			at := e.At
			d.StatusChangedAt = &at
			from := in.Categories[e.FromID]
			to := in.Categories[e.ToID]
			if to == CategoryDone {
				d.ResolvedAt = &at
			}
			if from == CategoryDone && to != CategoryDone {
				d.ReopenCount++
				d.ReopenedAt = &at
			}
		case "assignee":
			at := e.At
			d.AssigneeChangedAt = &at
		}
	}

	// A resolution that was undone is not a resolution date.
	if in.CurrentCategory != CategoryDone {
		d.ResolvedAt = nil
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
// an inward link whose type name contains "clone" (Jira's default "Cloners"
// type). Caveat: link type names are site configuration created in the site's
// language, so a site whose clone type carries a non-English name derives
// nothing here — there is no language-stable id to key on.
func clonedFrom(links []Link) string {
	for _, l := range links {
		if l.Direction == "inward" && strings.Contains(strings.ToLower(l.Type), "clone") {
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
