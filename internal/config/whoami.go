package config

import "strings"

// WhoAmI is "who am I in this workspace" — the three keys every surface that
// answers "is this mine?" already matches on, in one struct.
//
// There is no fourth key and no display name: names localize and collide, the
// same rule the status/priority columns live under. A field is empty when this
// workspace cannot answer it — a built-in workspace with no credential has
// only ActorSlug, a site that hides emails has only AccountID.
type WhoAmI struct {
	// AccountID is the origin's own id for the person (Jira accountId,
	// Linear user id). Matched first because it never changes.
	AccountID string
	// Email is the credential's email, matched case-insensitively when the
	// id is missing on one side.
	Email string
	// ActorSlug is the built-in tracker's attribution handle (ResolveActor) —
	// the identity a write is stamped with when there is no credential at all.
	ActorSlug string
}

// Empty reports whether this workspace could not answer the question at all.
// The identity views match nothing in that case, which is the honest answer:
// with no key, no row is provably mine.
func (w WhoAmI) Empty() bool {
	return w.AccountID == "" && w.Email == "" && w.ActorSlug == ""
}

// ResolveWhoAmI is the single owner of "who am I" (GDK-1438). It reads the
// same fields the web's GET auth/me/ answers from (server.handleMe) plus the
// actor slug the built-in tracker attributes writes to, so the SQL views, the
// web client and retro's self-match cannot disagree about the person.
//
// The Members fallback for AccountID is handleMe's: a config written before
// the /myself call stamped Config.AccountID may still carry the id on the
// member row for its own email.
func ResolveWhoAmI(c *Config) WhoAmI {
	if c == nil {
		return WhoAmI{}
	}
	w := WhoAmI{
		AccountID: strings.TrimSpace(c.AccountID),
		Email:     strings.TrimSpace(c.Email),
	}
	if w.AccountID == "" && w.Email != "" {
		for _, m := range c.Members {
			if m.Email == c.Email && strings.TrimSpace(m.JiraAccountID) != "" {
				w.AccountID = strings.TrimSpace(m.JiraAccountID)
				break
			}
		}
	}
	if actor, ok := ResolveActor(c); ok {
		w.ActorSlug = strings.TrimSpace(actor.Slug)
	}
	return w
}
