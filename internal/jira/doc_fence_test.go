package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

// GDK-1178: a markdown fence in a body must become an ADF codeBlock, not
// three literal paragraphs — the issue detail's run button keys on codeBlock.
func TestDocFencedBlockIsCodeBlock(t *testing.T) {
	raw := Doc("before\n```sh\ngadak sql \"x\"\nsecond\n```\nafter", map[string]string{"김현철": "acc"})
	var doc struct {
		Content []struct {
			Type    string            `json:"type"`
			Attrs   map[string]any    `json:"attrs"`
			Content []json.RawMessage `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Content) != 3 {
		t.Fatalf("want 3 top nodes, got %d: %s", len(doc.Content), raw)
	}
	if doc.Content[1].Type != "codeBlock" {
		t.Fatalf("want codeBlock, got %q: %s", doc.Content[1].Type, raw)
	}
	if got := doc.Content[1].Attrs["language"]; got != "sh" {
		t.Fatalf("language = %v", got)
	}
	var text struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(doc.Content[1].Content[0], &text); err != nil {
		t.Fatal(err)
	}
	if text.Text != "gadak sql \"x\"\nsecond" {
		t.Fatalf("code text = %q", text.Text)
	}
}

// A fence with no info string, and an unclosed fence running to end of text.
func TestDocFenceVariants(t *testing.T) {
	raw := Doc("```\nx\n```", nil)
	if !strings.Contains(string(raw), `"codeBlock"`) {
		t.Fatalf("closed bare fence: %s", raw)
	}
	raw = Doc("a\n```\nx", nil)
	if !strings.Contains(string(raw), `"codeBlock"`) {
		t.Fatalf("unclosed fence: %s", raw)
	}
}

// GDK-894 stays: an @Name inside a fence is code, not a mention.
func TestDocMentionInsideFenceStaysText(t *testing.T) {
	raw := Doc("```\n@김현철\n```", map[string]string{"김현철": "acc"})
	if strings.Contains(string(raw), `"mention"`) {
		t.Fatalf("mention leaked into code block: %s", raw)
	}
}
