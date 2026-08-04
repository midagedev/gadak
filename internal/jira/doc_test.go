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
	if doc.Type != "doc" || doc.Version != 1 || len(doc.Content) != 2 {
		t.Fatalf("shape: %+v", doc)
	}
	if doc.Content[1].Content[0].Text != "두 번째 줄 a@b.com" {
		t.Fatalf("second line: %q", doc.Content[1].Content[0].Text)
	}
	// An empty body is still a legal document.
	if !json.Valid(Doc("", nil)) {
		t.Fatal("empty doc invalid")
	}
}
