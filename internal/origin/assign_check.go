package origin

import "strings"

// The assign preflight's pure decision (GDK-2063 CLI, GDK-2073 web/phone):
// before an assign write is sent, one read of the origin's current assignee
// is compared with the mirror's issues.assignee_id — optimistic concurrency
// for a tracker with no etag. The mirror is a cache, and a write that trusts
// it can silently replace a person the caller never saw: in the incident a
// 15-hour-old mirror showed no assignee while a colleague had taken the
// issue, and the assign overwrote them.
//
// DecideAssign is the one owner of that comparison, so the CLI's assignOne
// and the server's assignee route cannot drift on what "stale" means. The
// read itself, and the "read failed → refuse unless force" branch, stay at
// each caller — this file only judges facts it is handed.

// AssignVerdict is what the assign preflight decided.
type AssignVerdict int

const (
	// AssignWrite sends the write.
	AssignWrite AssignVerdict = iota
	// AssignNoop: the origin already holds the target, so nothing is sent —
	// whatever the cache says and whatever force says, because sending the
	// same assignee changes nothing.
	AssignNoop
	// AssignStale: the origin's holder is not the one the cache showed.
	// Replacing them unseen is the stale-overwrite defect; the caller
	// refuses (CLI exit 75, REST 409 assignee_changed) unless force.
	AssignStale
)

// AssigneeID is the holder's id, "" for unassigned (nil or no id). The one
// spelling of "who holds the issue" the preflight compares.
func AssigneeID(u *User) string {
	if u == nil {
		return ""
	}
	return u.ID()
}

// AssigneeDisplayName is how a report names a holder: the display name, the
// id when the origin sent none, "" when there is no holder. Callers that
// need prose for "no holder" own that wording themselves — the CLI says
// "(nobody)", the REST 409 sends current_assignee: null.
func AssigneeDisplayName(u *User) string {
	if u == nil || u.ID() == "" {
		return ""
	}
	if name := strings.TrimSpace(u.DisplayName); name != "" {
		return name
	}
	return u.ID()
}

// DecideAssign judges the preflight read. No-op first — the target already
// holds the issue on the origin, cache and force notwithstanding. Stale only
// when the caller did not force, the mirror has a row to compare, and that
// row's assignee id differs from the origin's holder. current is the origin's
// holder as read: a caller whose read failed handles that refusal itself and
// does not consult this (a nil current here would read as "unassigned", which
// is not a fact the caller has).
func DecideAssign(current *User, cacheAssigneeID string, cacheRowKnown bool, target string, force bool) AssignVerdict {
	if AssigneeID(current) == target {
		return AssignNoop
	}
	if !force && cacheRowKnown && cacheAssigneeID != AssigneeID(current) {
		return AssignStale
	}
	return AssignWrite
}
