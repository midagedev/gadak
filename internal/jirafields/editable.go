package jirafields

import "github.com/midagedev/gadak/internal/jira"

// editKind maps a Jira field schema onto the editors the UI has. A field
// whose schema is none of them has no editor and is left out of editmeta.
// textarea is deliberately empty: Cloud v3 stores it as ADF, not a string.
func editKind(m jira.FieldMeta) string {
	switch {
	case m.Schema.Type == "option":
		return "option"
	case m.Schema.Type == "user":
		return "user"
	case m.Schema.Type == "array" && m.Schema.Items == "version":
		return "version_array"
	case m.Schema.Type == "array" && m.Schema.Items == "option":
		return "multi_option"
	case m.Schema.Type == "string" && customTail(m.Schema.Custom) != "textarea":
		return "text"
	case m.Schema.Type == "number":
		return "number"
	case m.Schema.Type == "date":
		return "date"
	}
	return ""
}

// ResolveEditable is the editmeta field-key interpreter: first candidate
// present in meta whose schema editKind understands, or whose fallbackKind
// (the configured spec kind) can stand in. CLI edit, REST GET/PATCH fields,
// `gadak issue --editmeta`, and create --field all go through here.
func ResolveEditable(candidates []string, meta map[string]jira.FieldMeta, fallbackKind string) (id, kind string, ok bool) {
	for _, cand := range candidates {
		m, present := meta[cand]
		if !present {
			continue
		}
		k := editKind(m)
		if k == "" {
			k = fallbackKind
		}
		if k == "" {
			continue
		}
		return cand, k, true
	}
	return "", "", false
}

// FieldMetaFromCreate projects a createmeta list row onto the editmeta shape
// ResolveEditable consumes, so create --field uses the same interpreter.
func FieldMetaFromCreate(f jira.CreateFieldMeta) jira.FieldMeta {
	var m jira.FieldMeta
	m.Required = f.Required
	m.Schema.Type = f.Schema.Type
	m.Schema.Items = f.Schema.Items
	m.Schema.Custom = f.Schema.Custom
	m.AllowedValues = f.AllowedValues
	return m
}
