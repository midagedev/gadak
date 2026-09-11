package store

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// CategoryDone is the only status category with meaning to the derived rules.
// Every rule keys on the category, never on a status name: the internal tool
// this was extracted from matched "Reopened" / "다시 열림" and broke on any other
// site or account language (contracts/sync.md, "Localization hazard").
const CategoryDone = "done"

// categoryNew names the remaining status category; CategoryInProgress lives
// in durations.go, where the lifecycle spans first needed it. The reopen rule
// below reasons about all three as categories, never status names.
const categoryNew = "new"

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
	// StartedAtHint / ResolvedAtHint are the flow stamps an origin with no
	// changelog carries on the issue itself (Linear: Issue.startedAt, and
	// completedAt — else canceledAt — as the finish), ISO-8601 UTC or empty.
	// They are consulted only under NoHistory: an origin that supplies a
	// changelog derives both columns from transitions and ignores them. They
	// are stamps, not history — status_changed_at, reopen_count and
	// reopened_at never come from them, and the undone-resolution and
	// positive-span rules below still run after them.
	StartedAtHint  string
	ResolvedAtHint string
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
	// CarryoverCount is how many times the issue was carried into a sprint
	// after the first — the number of distinct sprints it has ever entered,
	// minus one. Nil, never 0, when the origin supplies no changelog: an
	// issue that was never carried and an issue whose history cannot be read
	// are different answers, and the reopen columns' plain int does not
	// distinguish them (GDK-1694, and GDK-1690's rule).
	CarryoverCount *int
	// FirstSprintID / FirstSprintAt are the first sprint the issue entered
	// and when. The scope-creep question — was this added after the sprint
	// began? — is FirstSprintAt against that sprint's start_at, so the stamp
	// is the column with leverage, not the count. The id lives in the
	// sprints(source_id, id) space.
	FirstSprintID *int64
	FirstSprintAt *string
	// BlockedHours is the time the issue has spent flagged, in hours — every
	// closed flag interval summed (GDK-1449). A flag still up does not
	// contribute: only a clear closes an interval, so the number never moves
	// while the user watches it. 0.0 with a changelog and no flag rows means
	// never flagged; nil, never 0, under NoHistory — the carryover_count
	// doctrine: never blocked and cannot be read are different answers.
	BlockedHours *float64
	// BlockedSince is the open flag interval's start — when the issue was
	// last flagged and never unflagged. Nil when no flag is up, and nil under
	// NoHistory with BlockedHours (an origin that cannot say how long cannot
	// say since when). A stamp, not a live duration: aging against it is the
	// reader's query, and the flag's own age is `now − blocked_since`, which
	// is why the stamp is the column.
	BlockedSince *string
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

	// Sprint membership, accumulated across the pass — see the "sprint" case.
	var seenSprints map[int64]bool
	sprintEntries := 0

	// The open flag interval, if one is up — see the "flagged" case.
	var blockedStart *string
	var blockedHours float64

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
			if ReopenTransition(from, to) {
				d.ReopenCount++
				d.ReopenedAt = &at
			}
		case "assignee":
			d.AssigneeChangedAt = &at
		case "sprint":
			// Two origin shapes, one rule. Jira Cloud keeps the whole
			// membership as a growing list ("12" -> "12, 13"); the built-in
			// tracker states the single sprint moved into ("12" -> "13").
			// Counting ids that are new to this issue answers both the same
			// way, and it also refuses to count a removal ("12, 13" -> "12")
			// or a re-add of a sprint the issue already visited.
			for _, id := range sprintIDs(e.ToID) {
				if seenSprints == nil {
					seenSprints = map[int64]bool{}
				}
				if seenSprints[id] {
					continue
				}
				seenSprints[id] = true
				sprintEntries++
				if sprintEntries == 1 {
					first := id
					firstAt := at
					d.FirstSprintID = &first
					d.FirstSprintAt = &firstAt
				}
			}
		case "flagged":
			// Jira's Flagged is a checkbox: a set names its option
			// ("Impediment" by default) in to_value/to_id, a clear leaves both
			// empty — no status-category map to consult, the values
			// themselves are the whole verdict. A set while already up does
			// not restart the interval (there is no gap to measure); a clear
			// with nothing up closes nothing (the set predates the mirror's
			// changelog horizon, and an unmeasurable interval is skipped, not
			// guessed). Entries are sorted, so the last write wins.
			if e.ToValue != "" || e.ToID != "" {
				if blockedStart == nil {
					blockedStart = &at
				}
			} else if blockedStart != nil {
				if start, ok := parseStamp(*blockedStart); ok {
					if end, ok := parseStamp(at); ok {
						if h := end.Sub(start).Hours(); h > 0 {
							blockedHours += h
						}
					}
				}
				blockedStart = nil
			}
		}
		// LastActivityAt seeds from the newest changelog entry — any field,
		// because a priority edit is activity too: entries are sorted, so the
		// last non-empty At wins.
		d.LastActivityAt = &at
	}

	// Carry-over is a changelog answer, so an origin that supplies none
	// leaves it NULL rather than 0 (see the field comment).
	if !in.NoHistory {
		n := sprintEntries - 1
		if n < 0 {
			n = 0
		}
		d.CarryoverCount = &n
	}

	// Blocked time is a changelog answer the same way: with a history the
	// column is always set (0.0 = never flagged), without one it stays NULL —
	// and the open interval's start is gated with it, because an origin that
	// cannot say how long cannot say since when either (the carryover gate's
	// shape: NoHistory decides, not the entry count).
	if !in.NoHistory {
		h := blockedHours
		d.BlockedHours = &h
		d.BlockedSince = blockedStart
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

	// Flow hints (Linear): an origin with no changelog still stamps the two
	// instants on the issue itself, and those stamps are the whole truth
	// available — the transition rules above had nothing to say. The
	// timeline columns (status_changed_at, reopen_count, reopened_at) stay
	// untouched: a stamp is not a transition, and no synthetic history is
	// invented from it.
	if in.NoHistory && in.StartedAtHint != "" {
		at := in.StartedAtHint
		d.StartedAt = &at
	}
	if in.NoHistory && in.ResolvedAtHint != "" {
		at := in.ResolvedAtHint
		d.ResolvedAt = &at
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

// ReopenTransition is the single owner of what a "reopened" move is: the
// category going backwards. Leaving done is the classic reopen; leaving
// in-progress back to new is its sibling on workflows whose resolved states
// sit in the in-progress category — there 'QA testing → Reopened' is how work
// comes back, and under the done-only rule this replaced, those rows read
// reopen_count = 0 (59% of the real reopens on the mirror that reported it).
// Unknown ids stay what they always were: not done, not new — a category the
// map cannot name never counts as the destination of a backwards move, so an
// uncatalogued status cannot invent a reopen.
//
// Every consumer of the verdict routes through this one function (GDK-1753):
// Derive's ReopenCount/ReopenedAt columns, the server's per-history-row
// is_reopen on the detail wire, the CLI --derive listing, and the retro
// materials. A second spelling of the predicate anywhere else is what the
// sourcelint gate TestReopenVerdictHasOneOwner exists to keep from coming
// back.
func ReopenTransition(from, to string) bool {
	if from == CategoryDone {
		return to != CategoryDone
	}
	return from == CategoryInProgress && to == categoryNew
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

// sprintIDs splits a changelog sprint value into ids. Jira sends the whole
// membership as a comma-separated list with a space ("12, 13"); the built-in
// tracker sends one id. Neither shape is promised, so both separators are
// accepted and every element is trimmed. A non-numeric element is dropped
// rather than guessed at: the sprints table keys on an integer id.
func sprintIDs(v string) []int64 {
	if v == "" {
		return nil
	}
	var out []int64
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}
