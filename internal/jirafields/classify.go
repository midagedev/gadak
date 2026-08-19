// Package jirafields classifies and discovers Jira custom fields for the mirror.
// Pure functions only — no network, no database.
//
// Split from internal/fields so store can import the source-neutral helpers
// without pulling Jira REST types or atlhttp onto its dependency graph
// (docs/ARCHITECTURE.md:79).
package jirafields

import (
	"strings"

	"github.com/midagedev/gadak/internal/jira"
)

// Classify maps a Jira field schema onto a display role and editor kind.
// ok=false marks fields gadak should never surface (plugin plumbing, ranks).
func Classify(f jira.FieldInfo) (role, kind string, ok bool) {
	t := f.Schema.Type
	items := f.Schema.Items
	tail := customTail(f.Schema.Custom)
	name := f.Name

	// Exclusions first.
	switch t {
	case "any", "team", "atlas-project":
		return "", "", false
	}
	if strings.HasPrefix(t, "sd-") {
		return "", "", false
	}
	if t == "array" && (items == "json" || items == "any") {
		return "", "", false
	}
	if strings.HasPrefix(name, "[CHART]") {
		return "", "", false
	}

	if tail == "textarea" {
		return "body", "", true
	}
	if t == "option" {
		return "facet", "option", true
	}
	if t == "option-with-child" {
		return "facet", "", true // cascading not supported as an editor
	}
	if t == "array" && items == "option" {
		return "facet", "multi_option", true
	}
	if t == "array" && items == "version" {
		return "facet", "version_array", true
	}
	if t == "array" && items == "component" {
		return "facet", "", true
	}
	if t == "array" && items == "string" {
		return "facet", "", true
	}
	if t == "user" {
		return "user", "user", true
	}
	if t == "array" && items == "user" {
		return "user", "", true
	}
	switch t {
	case "string":
		return "plain", "text", true
	case "number":
		return "plain", "number", true
	case "date":
		return "plain", "date", true
	case "datetime", "url":
		return "plain", "", true
	}
	// Everything else surfaces as plain.
	return "plain", "", true
}

// customTail is the segment after the last colon in schema.custom
// (e.g. com.atlassian…:textarea → textarea).
func customTail(custom string) string {
	if i := strings.LastIndex(custom, ":"); i >= 0 {
		return custom[i+1:]
	}
	return custom
}
