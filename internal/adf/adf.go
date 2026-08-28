// Package adf inspects Atlassian Document Format (ADF) JSON without importing
// Jira types, so store, origin, and CLI can share it without crossing the
// store/jira firewall (docs/ARCHITECTURE.md).
//
// PlainText flattens a document for FTS. IsSimple is the format-loss gate
// (doc / paragraph / text / hardBreak, no marks) ported from
// web/src/lib/adf.ts isSimpleAdf; FormatLoss names what a plain-text replace
// would destroy, for the refusal that prints it.
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

// simpleTypes is the node-type set a plain-text round trip preserves. jira.Doc
// over a plain body emits exactly doc / paragraph / text (edit -m passes no
// mentions or media); hardBreak joins the set because it is visually a line
// break, the ported original's call (web/src/lib/adf.ts SIMPLE_ADF_TYPES).
// walkSimple and FormatLoss share this one owner so the two cannot drift.
var simpleTypes = map[string]bool{
	"doc": true, "paragraph": true, "text": true, "hardBreak": true,
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
	if !simpleTypes[n.Type] {
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

// unreadableDescription is FormatLoss's sentinel for a description that does
// not parse: the caller refuses rather than destroy a body it could not read.
const unreadableDescription = "unreadable description JSON"

// FormatLoss lists what a plain-text replace of raw would destroy: node types
// outside the IsSimple set, plus mark names (strong, link, …), deduped in
// first-appearance order — edit -m's refusal prints them so the user sees what
// the replace would drop (GDK-1001). Empty means a plain-text replace loses
// nothing. A bare string (the older wiki-markup shape PlainText passes
// through) is already plain. A document that does not parse is reported,
// never waved through.
func FormatLoss(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var s string
	if json.Unmarshal([]byte(raw), &s) == nil {
		return nil
	}
	var n simpleNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return []string{unreadableDescription}
	}
	var out []string
	seen := map[string]bool{}
	collectLoss(n, seen, &out)
	return out
}

func collectLoss(n simpleNode, seen map[string]bool, out *[]string) {
	if !simpleTypes[n.Type] {
		noteLoss(n.Type, seen, out)
	}
	for _, m := range n.Marks {
		var mark struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(m, &mark) == nil && mark.Type != "" {
			noteLoss(mark.Type, seen, out)
			continue
		}
		noteLoss("marks", seen, out)
	}
	for _, c := range n.Content {
		collectLoss(c, seen, out)
	}
}

func noteLoss(kind string, seen map[string]bool, out *[]string) {
	if kind == "" {
		kind = "unknown node"
	}
	if !seen[kind] {
		seen[kind] = true
		*out = append(*out, kind)
	}
}
