package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDocTurnsMentionsIntoNodes(t *testing.T) {
	ids := map[string]string{"김현철": "acc-hc", "김현": "acc-other"}
	got := string(Doc("@김현철 확인 부탁\n두 번째 줄 a@b.com", ids))

	// The longer name wins, or the mention notifies the wrong person.
	if !strings.Contains(got, `"id":"acc-hc"`) || strings.Contains(got, "acc-other") {
		t.Fatalf("wrong account matched: %s", got)
	}
	if !strings.Contains(got, `"type":"mention"`) || !strings.Contains(got, `"text":"@김현철"`) {
		t.Fatalf("no mention node: %s", got)
	}
	// An email address is not a mention.
	if strings.Count(got, `"type":"mention"`) != 1 {
		t.Fatalf("mention count: %s", got)
	}

	var doc struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("not valid ADF: %v", err)
	}
	// GDK-1384: the body is markdown — a single newline is a hardBreak inside
	// one paragraph, not a second paragraph (a blank line would be).
	if doc.Type != "doc" || doc.Version != 1 || len(doc.Content) != 1 {
		t.Fatalf("shape: %+v", doc)
	}
	inl := doc.Content[0].Content
	if len(inl) != 4 || inl[0].Type != "mention" || inl[2].Type != "hardBreak" || inl[3].Text != "두 번째 줄 a@b.com" {
		t.Fatalf("inline shape: %+v", inl)
	}
	// An empty body is still a legal document.
	if !json.Valid(Doc("", nil)) {
		t.Fatal("empty doc invalid")
	}
}

// GDK-894: the same body can carry a real mention and the same token quoted
// as code. Candidate extraction skips code regions, so substitution must skip
// them too — a web-composed mentions map (resolved by the client, not the
// CLI) would otherwise summon a person inside a code quote.
func TestDocLeavesCodeQuotedMentionsAsText(t *testing.T) {
	ids := map[string]string{"Dana": "acc-dana"}

	span := string(Doc("plain @Dana and `@Dana` tail", ids))
	if n := strings.Count(span, `"type":"mention"`); n != 1 {
		t.Fatalf("inline code span must not become a mention node (got %d): %s", n, span)
	}
	// GDK-1384: the span is a text node carrying the code mark; the
	// backticks are syntax, not text.
	if !strings.Contains(span, `"marks":[{"type":"code"}],"text":"@Dana"`) {
		t.Fatalf("code span text lost: %s", span)
	}

	fence := string(Doc("before\n```\n@Dana\n```\nafter", ids))
	if strings.Contains(fence, `"type":"mention"`) {
		t.Fatalf("fenced @Dana must stay text: %s", fence)
	}
	if !strings.Contains(fence, "@Dana") {
		t.Fatalf("fence text lost: %s", fence)
	}
}
