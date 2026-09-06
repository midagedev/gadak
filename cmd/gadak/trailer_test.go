package main

// CLI actor trailer — contract ↔ assertion map (FAIL-first per row: before
// writerForAgentWrite routed through origin.WithActorTrailer, every body
// below reached the fake origin verbatim, so each trailer assertion fails
// on the pre-change source).
//
//	T1 `gadak comment KEY -m` with GADAK_ACTOR set posts a body whose
//	   last paragraph is "— via gadak · Claude Test (claude:test)"
//	   TestCommentStampsActorTrailer
//	T2 actor.trailer false in the workspace config → no trailer
//	   TestCommentTrailerOffInConfig
//	T3 no actor resolved → no trailer (the ambient default every other
//	   comment test already pins; made explicit here with the env cleared)
//	   TestCommentNoActorNoTrailer
//	T4 `gadak create -m` stamps the description's last paragraph
//	   TestCreateStampsActorTrailerOnDescription
//	T5 the status actor row carries "· trailer on" / "· trailer off"
//	   TestStatusActorRowNamesTrailerSwitch

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// commentBodyLastParagraph decodes the ADF the fake Jira recorded for a
// comment POST and returns the text of its final paragraph.
func commentBodyLastParagraph(t *testing.T, raw string) (string, int) {
	t.Helper()
	var sent struct {
		Body struct {
			Content []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"content"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(raw), &sent); err != nil {
		t.Fatalf("comment body not JSON: %v: %s", err, raw)
	}
	if len(sent.Body.Content) == 0 {
		return "", 0
	}
	last := sent.Body.Content[len(sent.Body.Content)-1]
	var b strings.Builder
	for _, c := range last.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String(), len(sent.Body.Content)
}

func TestCommentStampsActorTrailer(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")

	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "hello"})
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	sent := f.bodies["POST /issue/NMB-1/comment"]
	last, n := commentBodyLastParagraph(t, sent)
	if n != 2 || last != "— via gadak · Claude Test (claude:test)" {
		t.Fatalf("comment body: %d paragraphs, last %q (raw %s)", n, last, sent)
	}
}

func TestCommentTrailerOffInConfig(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")

	no := false
	cfg.Actor = &config.ActorConfig{Trailer: &no}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "hello"})
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	sent := f.bodies["POST /issue/NMB-1/comment"]
	if strings.Contains(sent, "via gadak") {
		t.Fatalf("actor.trailer=false still stamped: %s", sent)
	}
	last, n := commentBodyLastParagraph(t, sent)
	if n != 1 || last != "hello" {
		t.Fatalf("body changed with the trailer off: %d paragraphs, last %q", n, last)
	}
}

func TestCommentNoActorNoTrailer(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	t.Setenv("GADAK_ACTOR", "")
	t.Setenv("CLAUDECODE", "")

	if _, err := capture(t, func() error {
		return cmdComment([]string{"NMB-1", "-m", "hello"})
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	sent := f.bodies["POST /issue/NMB-1/comment"]
	if strings.Contains(sent, "via gadak") {
		t.Fatalf("no actor resolved, still stamped: %s", sent)
	}
}

func TestCreateStampsActorTrailerOnDescription(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")

	if _, err := capture(t, func() error {
		return cmdCreate([]string{
			"Fix", "the", "flaky", "gate",
			"--project", "NMB", "--type", "Task",
			"-m", "body",
		})
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	var sent struct {
		Fields struct {
			Description struct {
				Content []struct {
					Type    string `json:"type"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"content"`
			} `json:"description"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(f.bodies["POST /issue"]), &sent); err != nil {
		t.Fatalf("create body not JSON: %v: %s", err, f.bodies["POST /issue"])
	}
	blocks := sent.Fields.Description.Content
	if len(blocks) == 0 {
		t.Fatal("create body has no description blocks")
	}
	last := blocks[len(blocks)-1]
	var b strings.Builder
	for _, c := range last.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	if got := b.String(); got != "— via gadak · Claude Test (claude:test)" {
		t.Fatalf("description last paragraph = %q, want the trailer (raw %s)", got, f.bodies["POST /issue"])
	}
}

func TestStatusActorRowNamesTrailerSwitch(t *testing.T) {
	f := newFakeJira(t)
	cfg := mirror(t, f.URL)
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")

	out, err := capture(t, func() error { return cmdStatus([]string{}) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "· trailer on") {
		t.Fatalf("status actor row must name the trailer switch:\n%s", out)
	}

	no := false
	cfg.Actor = &config.ActorConfig{Trailer: &no}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	out, err = capture(t, func() error { return cmdStatus([]string{}) })
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "· trailer off") {
		t.Fatalf("status actor row must name the switch off:\n%s", out)
	}
}

// T6: `gadak transition KEY done -m text` — the one comment path that goes
// through internal/transition (read-only this round), so the trailer has to
// survive a layer the decorator never sees. The wire shape is Jira's
// update.comment[0].add.body. FAIL-first: without the routing, the body's
// last paragraph is the typed text.
func TestTransitionCommentStampsActorTrailer(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")

	if _, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "-m", "closing this out"})
	}); err != nil {
		t.Fatalf("transition: %v", err)
	}
	var sent struct {
		Update struct {
			Comment []struct {
				Add struct {
					Body json.RawMessage `json:"body"`
				} `json:"add"`
			} `json:"comment"`
		} `json:"update"`
	}
	if err := json.Unmarshal([]byte(f.bodies["POST /issue/NMB-1/transitions"]), &sent); err != nil {
		t.Fatalf("transition body not JSON: %v: %s", err, f.bodies["POST /issue/NMB-1/transitions"])
	}
	if len(sent.Update.Comment) != 1 {
		t.Fatalf("transition comment count = %d, want 1", len(sent.Update.Comment))
	}
	body := sent.Update.Comment[0].Add.Body
	var doc struct {
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("comment ADF not JSON: %v: %s", err, body)
	}
	if len(doc.Content) == 0 {
		t.Fatal("transition comment has no blocks")
	}
	last := doc.Content[len(doc.Content)-1]
	var b strings.Builder
	for _, c := range last.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	if got := b.String(); got != "— via gadak · Claude Test (claude:test)" {
		t.Fatalf("transition comment last paragraph = %q, want the trailer (raw %s)", got, body)
	}
}
