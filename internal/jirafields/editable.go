package jirafields

import "github.com/midagedev/gadak/internal/jira"

// EditKind maps a Jira field schema onto the editors the UI has. A field
// whose schema is none of them has no editor and is left out of editmeta.
func EditKind(m jira.FieldMeta) string {
	switch {
	case m.Schema.Type == "option":
		return "option"
	case m.Schema.Type == "user":
		return "user"
	case m.Schema.Type == "array" && m.Schema.Items == "version":
		return "version_array"
	case m.Schema.Type == "array" && m.Schema.Items == "option":
		return "multi_option"
	}
	return ""
}

// ResolveEditableID picks the first candidate id present in editmeta.
func ResolveEditableID(candidates []string, meta map[string]jira.FieldMeta) (id, kind string, ok bool) {
	for _, cand := range candidates {
		m, present := meta[cand]
		if !present {
			continue
		}
		k := EditKind(m)
		if k == "" {
			continue
		}
		return cand, k, true
	}
	return "", "", false
}
