package jira

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
