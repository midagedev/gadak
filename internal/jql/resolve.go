package jql

import "strings"

// ResolvePeople replaces assignee/reporter display names and account ids
// with emails from the mirror. Unresolved values move to Unsupported rather
// than silently matching nothing. currentUser() becomes opts email, or is
// refused when no identity is configured.
func ResolvePeople(res *Result, people []Person, me string) {
	if res == nil {
		return
	}
	res.Filters.AssigneeEmail = resolveList(res.Filters.AssigneeEmail, people, me, "assignee", &res.Unsupported)
	res.Filters.ReporterEmail = resolveList(res.Filters.ReporterEmail, people, me, "reporter", &res.Unsupported)
}

func resolveList(vals []string, people []Person, me, field string, unsupported *[]string) []string {
	if len(vals) == 0 {
		return vals
	}
	var out []string
	for _, v := range vals {
		if strings.EqualFold(v, "currentUser()") {
			if me == "" {
				addUnsup(unsupported, field+" = currentUser() (no configured identity)")
				continue
			}
			out = appendUniqueFold(out, me)
			continue
		}
		if looksLikeEmail(v) {
			out = appendUniqueFold(out, v)
			continue
		}
		matches := lookupPerson(v, people)
		switch len(matches) {
		case 1:
			out = appendUniqueFold(out, matches[0])
		case 0:
			addUnsup(unsupported, field+" = "+v+" (no one in the mirror matches)")
		default:
			addUnsup(unsupported, field+" = "+v+" (ambiguous in the mirror)")
		}
	}
	return out
}

func lookupPerson(v string, people []Person) []string {
	var emails []string
	seen := map[string]bool{}
	for _, p := range people {
		if p.Email == "" {
			continue
		}
		if eqFold(v, p.Email) || eqFold(v, p.Name) || eqFold(v, p.DisplayName) || eqFold(v, p.AccountID) {
			k := strings.ToLower(p.Email)
			if seen[k] {
				continue
			}
			seen[k] = true
			emails = append(emails, p.Email)
		}
	}
	return emails
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
		add(it.ReporterEmail, it.Reporter, "")
	}
	out := make([]Person, 0, len(seen))
	for _, p := range seen {
		out = append(out, p)
	}
	return out
}
