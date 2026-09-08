package main

import (
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/store"
)

// GDK-1645: an origin can answer 204 and change nothing (Jira Server on a
// standard issue's parent, measured), and the write path used to print the
// refreshed row as success. The refreshed row is the evidence, so it is
// compared before it is printed: a field this edit asked to change that
// reads the same before and after is a dropped write, not a success. A
// field already at the requested value (pre == requested) is idempotent and
// passes. First slice: parent, summary, due date, priority — the fields
// whose equality is one string. Labels, components, versions and the
// description are left to the row's own printout.
func verifyEditLanded(pre, post *store.IssueLite, ch editChange) error {
	if pre == nil || post == nil {
		return nil // not mirrored before the write: nothing to compare against
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	type check struct {
		field, before, after, asked string
		changeAsked                 bool
	}
	checks := []check{}
	if ch.hasParent {
		asked := ch.parentKey
		if ch.clearParent {
			asked = ""
		}
		checks = append(checks, check{"parent", deref(pre.ParentKey), deref(post.ParentKey), asked, true})
	}
	if ch.hasSummary {
		checks = append(checks, check{"summary", pre.Summary, post.Summary, strings.TrimSpace(ch.summary), true})
	}
	if ch.hasDue {
		asked := ch.dueDate
		if ch.clearDue {
			asked = ""
		}
		checks = append(checks, check{"due date", deref(pre.Duedate), deref(post.Duedate), asked, true})
	}
	if ch.hasPriority {
		// The token may be an id or a display name; either matching the
		// row before the write means nothing was meant to move.
		tok := strings.TrimSpace(ch.priority)
		unchanged := pre.PriorityID == post.PriorityID && deref(pre.Priority) == deref(post.Priority)
		already := tok == pre.PriorityID || strings.EqualFold(tok, deref(pre.Priority))
		if unchanged && !already {
			return droppedWriteError(ch.key, "priority", deref(pre.Priority), tok)
		}
	}
	for _, c := range checks {
		if c.before == c.after && c.before != c.asked {
			return droppedWriteError(ch.key, c.field, c.before, c.asked)
		}
	}
	return nil
}

func droppedWriteError(key, field, still, asked string) error {
	show := func(s string) string {
		if s == "" {
			return "empty"
		}
		return fmt.Sprintf("%q", s)
	}
	return fmt.Errorf("edit %s: the origin accepted the write, but %s did not change — still %s, asked %s. The origin dropped the field without an error; check its screen configuration for this issue type", key, field, show(still), show(asked))
}

// lookupOne is lookup for a single key; nil when the mirror has no row.
func lookupOne(db *store.DB, key string) *store.IssueLite {
	lites, err := lookup(db, []string{key})
	if err != nil || len(lites) == 0 {
		return nil
	}
	return &lites[0]
}
