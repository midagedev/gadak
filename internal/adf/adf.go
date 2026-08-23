// Package adf inspects Atlassian Document Format (ADF) JSON without importing
// Jira types, so store, origin, and CLI can share it without crossing the
// store/jira firewall (docs/ARCHITECTURE.md).
//
// PlainText flattens a document for FTS. IsSimple is the format-loss gate
// (doc / paragraph / text / hardBreak, no marks) ported from
// web/src/lib/adf.ts isSimpleAdf.
package adf

import (
	"encoding/json"
	"strings"
)

// blockNode is the set of ADF node types that end a line. Anything else is
// inline, so its text runs on.
var blockNode = map[string]bool{
	"paragraph": true, "heading": true, "listItem": true, "blockquote": true,
	"codeBlock": true, "tableRow": true, "rule": true, "panel": true,
	"mediaSingle": true, "mediaGroup": true, "taskItem": true, "decisionItem": true,
}

// PlainText flattens an ADF document to plain text: it is what FTS indexes and
// what makes a repro-steps custom field searchable. A field that holds a bare
// string (the older wiki-markup shape) passes through unchanged.
func PlainText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var doc any
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	var b strings.Builder
	flatten(&b, doc)
	return strings.TrimSpace(b.String())
}

func flatten(b *strings.Builder, node any) {
	switch v := node.(type) {
	case []any:
		for _, child := range v {
			flatten(b, child)
		}
	case map[string]any:
		kind, _ := v["type"].(string)
		switch kind {
		case "text":
			if s, ok := v["text"].(string); ok {
				b.WriteString(s)
			}
		case "hardBreak":
			b.WriteString("\n")
		case "mention", "emoji":
			// Both keep their visible label in attrs.text.
			if attrs, ok := v["attrs"].(map[string]any); ok {
				if s, ok := attrs["text"].(string); ok {
					b.WriteString(s)
				}
			}
		}
		flatten(b, v["content"])
		if blockNode[kind] {
			b.WriteString("\n")
		}
	}
}

// simpleNode is the walk shape for IsSimple. Mirrors web/src/lib/adf.ts
// SIMPLE_ADF_TYPES + walkSimple (doc / paragraph / text / hardBreak, no marks).
type simpleNode struct {
	Type    string            `json:"type"`
	Marks   []json.RawMessage `json:"marks"`
	Content []simpleNode      `json:"content"`
}

// IsSimple is the Go port of web/src/lib/adf.ts isSimpleAdf: empty/null is
// simple; otherwise only doc/paragraph/text/hardBreak with no marks. A
// plain-text replace of a non-simple document would drop formatting, so
// callers refuse unless the user passed force.
func IsSimple(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	var n simpleNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return false
	}
	return walkSimple(n)
}

func walkSimple(n simpleNode) bool {
	switch n.Type {
	case "doc", "paragraph", "text", "hardBreak":
	default:
		return false
	}
	if len(n.Marks) > 0 {
		return false
	}
	for _, c := range n.Content {
		if !walkSimple(c) {
			return false
		}
	}
	return true
}
