package jql

import "strings"

// ResolvePeople replaces assignee/reporter display names and account ids
// with the matched person's account id (email fallback). Unresolved values
// move to Unsupported rather than silently matching nothing. currentUser()
// becomes me, which this wrapper treats as an email.
//
// Prefer ResolveIdentity when the configured account id is known.
func ResolvePeople(res *Result, people []Person, me string) {
	ResolveIdentity(res, people, Identity{Email: me})
}

// ResolveIdentity is the identity owner: currentUser() becomes AccountID
// when set, otherwise Email; roster lookups return AccountID when the
// matched person has one. Email-less roster rows are not skipped.
func ResolveIdentity(res *Result, people []Person, me Identity) {
	if res == nil {
		return
	}
	res.Filters.AssigneeEmail = resolveList(res.Filters.AssigneeEmail, people, me, "assignee", &res.Unsupported)
	res.Filters.ReporterEmail = resolveList(res.Filters.ReporterEmail, people, me, "reporter", &res.Unsupported)
}

func resolveList(vals []string, people []Person, me Identity, field string, unsupported *[]string) []string {
	if len(vals) == 0 {
		return vals
	}
	var out []string
	for _, v := range vals {
		if strings.EqualFold(v, "currentUser()") {
			ident := currentUserIdent(me, people)
			if ident == "" {
				addUnsup(unsupported, field+" = currentUser() (no account email — set one on a connected workspace, or drop currentUser())")
				continue
			}
			out = appendUniqueFold(out, ident)
			continue
		}
		matches := lookupPerson(v, people)
		switch len(matches) {
		case 1:
			out = appendUniqueFold(out, matches[0])
		case 0:
			if looksLikeEmail(v) {
				out = appendUniqueFold(out, v)
				continue
			}
			addUnsup(unsupported, field+" = "+v+" (not in the mirror)")
		default:
			addUnsup(unsupported, field+" = "+v+" (ambiguous in the mirror)")
		}
	}
	return out
}

func currentUserIdent(me Identity, people []Person) string {
	if me.AccountID != "" {
		return me.AccountID
	}
	if me.Email == "" {
		return ""
	}
	if matches := lookupPerson(me.Email, people); len(matches) == 1 {
		return matches[0]
	}
	return me.Email
}

func lookupPerson(v string, people []Person) []string {
	var ids []string
	seen := map[string]bool{}
	for _, p := range people {
		if !personMatches(v, p) {
			continue
		}
		ident := personIdent(p)
		if ident == "" {
			continue
		}
		k := strings.ToLower(ident)
		if seen[k] {
			continue
		}
		seen[k] = true
		ids = append(ids, ident)
	}
	return ids
}

func personMatches(v string, p Person) bool {
	return eqFold(v, p.Email) || eqFold(v, p.Name) || eqFold(v, p.DisplayName) || eqFold(v, p.AccountID)
}

func personIdent(p Person) string {
	if p.AccountID != "" {
		return p.AccountID
	}
	if p.Email != "" {
		return p.Email
	}
	if p.Name != "" {
		return p.Name
	}
	return p.DisplayName
}

func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1
}

func appendUniqueFold(dst []string, v string) []string {
	for _, d := range dst {
		if eqFold(d, v) {
			return dst
		}
	}
	return append(dst, v)
}

func addUnsup(list *[]string, msg string) {
	for _, u := range *list {
		if u == msg {
			return
		}
	}
	*list = append(*list, msg)
}

// PeopleFromIssues builds the resolver roster from mirrored issues.
func PeopleFromIssues(issues []Issue) []Person {
	seen := map[string]Person{}
	add := func(email, name, id string) {
		if email == "" && name == "" && id == "" {
			return
		}
		key := strings.ToLower(email)
		if key == "" {
			key = strings.ToLower(name) + "|" + id
		}
		p := seen[key]
		if email != "" {
			p.Email = email
		}
		if name != "" && p.Name == "" {
			p.Name = name
		}
		if id != "" && p.AccountID == "" {
			p.AccountID = id
		}
		seen[key] = p
	}
	for _, it := range issues {
		add(it.AssigneeEmail, it.Assignee, it.AssigneeID)
		add(it.ReporterEmail, it.Reporter, it.ReporterID)
	}
	out := make([]Person, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	return out
}
