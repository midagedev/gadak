// Package statuscat is the single owner of the status-category contract:
// the three tokens data-model.md documents (new | inprogress | done), their
// mapping from Jira's REST statusCategory keys, and the reverse.
//
// The table is deliberately not part of internal/jira (GDK-686): the JQL
// dialect keys on these tokens and its package doc promises it does not talk
// to Jira, so owning the vocabulary in the REST client made the parser link
// net/http (same reason jirafields was split from fields for the store
// firewall, docs/ARCHITECTURE.md:79). Stdlib only — nothing here may grow a
// dependency. Never key on status display names: they are localized
// per account ("진행 중"). The web mirrors this fold in
// web/src/lib/view-config.ts (effectiveCategory — saved-view status_category
// axes and raw transition keys need it); this package stays the single
// owner.
package statuscat

// KnownCategory maps a Jira statusCategory key or a gadak token onto the
// three values data-model.md documents. Unlike Category, unknown keys are
// not folded to "new": write resolvers refuse those so a damaged payload
// cannot move an issue.
func KnownCategory(key string) (string, bool) {
	switch key {
	case "done":
		return "done", true
	case "indeterminate", "inprogress":
		return "inprogress", true
	case "new":
		return "new", true
	default:
		return "", false
	}
}

// Category maps Jira's statusCategory key onto the three values data-model.md
// documents. An unknown key becomes "new", which can only ever miss a reopen,
// never invent one.
func Category(key string) string {
	if cat, ok := KnownCategory(key); ok {
		return cat
	}
	return "new"
}

// CategoryKey is the reverse of Category: a gadak token (or a Jira key
// Category would accept) onto Jira's REST statusCategory key. inprogress
// becomes indeterminate, the key Cloud actually stores.
func CategoryKey(token string) string {
	switch Category(token) {
	case "inprogress":
		return "indeterminate"
	case "done":
		return "done"
	default:
		return "new"
	}
}
