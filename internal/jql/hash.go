package jql

import (
	"net/url"
	"strings"
)

// Hash is the gadak UI view query string (no leading #/?). It matches
// web/src/lib/view-config.ts configToParams: same short keys, same
// omit-defaults rule. The SPA parseConfig of these params is the view.
func Hash(f Filter, d Display) string {
	p := url.Values{}
	addList := func(key string, vals []string) {
		if len(vals) == 0 {
			return
		}
		p.Set(key, strings.Join(vals, ","))
	}
	addList("sc", f.StatusCategory)
	addList("st", f.Status)
	addList("as", f.AssigneeEmail)
	addList("rp", f.ReporterEmail)
	addList("gr", f.TeamGroup)
	addList("lb", f.Labels)
	addList("pr", f.Priority)
	addList("sv", f.Severity)
	addList("ty", f.IssueType)
	addList("co", f.Components)
	addList("fx", f.FixVersions)
	addList("qr", f.QARun)
	addList("qs", f.QASuite)
	addList("qi", f.QAImpact)
	addList("ds", f.DeployState)
	addList("pj", f.JiraProject)
	addList("spj", f.SourceProject)
	addList("pjn", f.JiraProjectNot)
	addList("spjn", f.SourceProjectNot)
	addList("ks", f.Keys)
	addList("pk", f.Parent)
	addList("sid", f.SprintIDs)
	addList("sst", f.SprintState)
	for alias, vals := range f.Fields {
		if len(vals) > 0 {
			p.Set("f."+alias, strings.Join(vals, ","))
		}
	}
	var flags []string
	if f.Reopened {
		flags = append(flags, "reopened")
	}
	if f.Unassigned {
		flags = append(flags, "unassigned")
	}
	if f.Stale {
		flags = append(flags, "stale")
	}
	if len(flags) > 0 {
		p.Set("fl", strings.Join(flags, ","))
	}
	if f.CreatedFrom != nil && *f.CreatedFrom != "" {
		p.Set("cf", *f.CreatedFrom)
	}
	if f.CreatedTo != nil && *f.CreatedTo != "" {
		p.Set("ct", *f.CreatedTo)
	}
	if f.UpdatedFrom != nil && *f.UpdatedFrom != "" {
		p.Set("uf", *f.UpdatedFrom)
	}
	if f.UpdatedTo != nil && *f.UpdatedTo != "" {
		p.Set("ut", *f.UpdatedTo)
	}
	if f.DueFrom != nil && *f.DueFrom != "" {
		p.Set("df", *f.DueFrom)
	}
	if f.DueTo != nil && *f.DueTo != "" {
		p.Set("dt", *f.DueTo)
	}
	if f.ResolvedFrom != nil && *f.ResolvedFrom != "" {
		p.Set("rf", *f.ResolvedFrom)
	}
	if f.ResolvedTo != nil && *f.ResolvedTo != "" {
		p.Set("rt", *f.ResolvedTo)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		p.Set("q", q)
	}
	if d.GroupBy != "" && d.GroupBy != "status_category" {
		p.Set("g", d.GroupBy)
	}
	if d.Sort != "" && d.Sort != "updated" {
		p.Set("s", d.Sort)
	}
	if d.Dir != "" && d.Dir != "desc" {
		p.Set("d", d.Dir)
	}
	h := p.Encode()
	// Encode() turns commas into %2C. The pinned ks= contract (and the web
	// parser's split-on-comma) is the literal comma form; leave other axes
	// encoded so existing hashes stay byte-identical.
	if len(f.Keys) > 0 {
		joined := strings.Join(f.Keys, ",")
		h = strings.Replace(h, "ks="+url.QueryEscape(joined), "ks="+joined, 1)
	}
	if len(f.Parent) > 0 {
		joined := strings.Join(f.Parent, ",")
		h = strings.Replace(h, "pk="+url.QueryEscape(joined), "pk="+joined, 1)
	}
	return h
}

// QueryURL is the fragment a gadak window applies to an already-encoded Hash().
func QueryURL(hash string) string {
	if hash == "" {
		return "#/"
	}
	return "#/?" + hash
}

// HashURL is the fragment a gadak window applies: "#/?" + Hash.
func HashURL(f Filter, d Display) string {
	return QueryURL(Hash(f, d))
}
