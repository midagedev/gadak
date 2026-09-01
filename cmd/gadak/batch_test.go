package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// TestCommentBatchEmitsEnvelope is the GDK-501 replacement for the pre-change
// FAIL-first: `gadak comment --batch -` used to be unknown flag / exit 2
// (scratch-501-failfirst.log). The same invocation now prints an envelope.
func TestCommentBatchEmitsEnvelope(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1","body":"hello from batch"}`+"\n")

	out, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "-"})
	})
	if err != nil {
		t.Fatalf("comment --batch -: %v\n%s", err, out)
	}
	_, rows := parseTSVEnvelope(t, out)
	if len(rows) != 1 {
		t.Fatalf("rows %d in %q", len(rows), out)
	}
	if rows[0].Key != "NMB-1" || !rows[0].OK || !rows[0].Changed || rows[0].Error != "" {
		t.Fatalf("row %+v", rows[0])
	}
	if body := f.bodies["POST /issue/NMB-1/comment"]; !strings.Contains(body, "hello from batch") {
		t.Fatalf("comment not sent: %s", body)
	}
}

// TestCommentBatchTryAllSecondFailsThirdSucceeds is a new contract: try-all
// could not exist before --batch was accepted (the verb died at flag parse).
func TestCommentBatchTryAllSecondFailsThirdSucceeds(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		`{"key":"NMB-1","body":"one"}`+"\n"+
		`{not-json}`+"\n"+
		`{"key":"NMB-1","body":"three"}`+"\n")

	out, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "-"})
	})
	if err == nil {
		t.Fatal("mixed batch must be non-nil error after envelopes")
	}
	if !strings.Contains(err.Error(), "1 of 3 failed") {
		t.Fatalf("summary %v", err)
	}
	_, rows := parseTSVEnvelope(t, out)
	if len(rows) != 3 {
		t.Fatalf("want 3 envelope rows, got %d in %q", len(rows), out)
	}
	if !rows[0].OK || rows[0].Key != "NMB-1" || !rows[0].Changed {
		t.Fatalf("line 1 %+v", rows[0])
	}
	if rows[1].OK || rows[1].Error == "" || !strings.Contains(rows[1].Error, "invalid JSON") {
		t.Fatalf("line 2 must be a parse failure: %+v", rows[1])
	}
	if !rows[2].OK || rows[2].Key != "NMB-1" || !rows[2].Changed {
		t.Fatalf("line 3 %+v", rows[2])
	}
	if n := countCalls(f, "POST /issue/NMB-1/comment"); n != 2 {
		t.Fatalf("POSTs %d in %v", n, f.calls)
	}
	if !strings.Contains(f.bodies["POST /issue/NMB-1/comment"], "three") {
		t.Fatalf("third line must have been posted: %s", f.bodies["POST /issue/NMB-1/comment"])
	}
}

func TestCommentBatchUnknownFieldIsLineError(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		`{"key":"NMB-1","body":"ok"}`+"\n"+
		`{"key":"NMB-1","body":"nope","mystery":true}`+"\n"+
		`{"key":"NMB-1","body":"after"}`+"\n")

	out, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "-"})
	})
	if err == nil {
		t.Fatal("unknown field must fail the batch")
	}
	_, rows := parseTSVEnvelope(t, out)
	if len(rows) != 3 {
		t.Fatalf("rows %d in %q", len(rows), out)
	}
	if rows[1].OK || !strings.Contains(rows[1].Error, "unknown field") || !strings.Contains(rows[1].Error, "mystery") {
		t.Fatalf("line 2 %+v", rows[1])
	}
	if rows[1].Key != "NMB-1" {
		t.Fatalf("unknown-field row must still name the key: %+v", rows[1])
	}
	if !rows[2].OK {
		t.Fatalf("line 3 %+v", rows[2])
	}
	if n := countCalls(f, "POST /issue/NMB-1/comment"); n != 2 {
		t.Fatalf("POSTs %d", n)
	}
}

func TestCommentBatchJSONEnvelope(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1","body":"json row"}`+"\n")

	out, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "-", "--json"})
	})
	if err != nil {
		t.Fatalf("json batch: %v\n%s", err, out)
	}
	rows := parseJSONEnvelope(t, out)
	if len(rows) != 1 || rows[0].Key != "NMB-1" || !rows[0].OK || !rows[0].Changed || rows[0].Error != "" {
		t.Fatalf("rows %+v from %q", rows, out)
	}
	if strings.Contains(out, `"issue"`) {
		t.Fatalf("batch --json must be the envelope, not IssueLite: %s", out)
	}
}

func TestWriteBatchEmptyStdinIsUsage(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, "")

	_, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "-"})
	})
	if err == nil || !strings.Contains(err.Error(), "usage: gadak comment --batch -") {
		t.Fatalf("empty stdin: %v", err)
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("empty stdin reached Jira: %v", f.calls)
	}
}

func TestWriteBatchRejectsNonDash(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "comments.jsonl"})
	})
	if err == nil || !strings.Contains(err.Error(), "--batch only accepts -") {
		t.Fatalf("non-dash: %v", err)
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("reached Jira: %v", f.calls)
	}
}

func TestWriteBatchMDashMutualExclusion(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1","body":"unused"}`+"\n")
	_, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "-", "-m", "-"})
	})
	if err == nil || !strings.Contains(err.Error(), "-m - cannot be used together") {
		t.Fatalf("mutual exclusion: %v", err)
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("reached Jira: %v", f.calls)
	}
}

func TestWriteBatchLineLimitRefusesBeforeWrites(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	var b strings.Builder
	for i := 0; i < batchLineLimit+1; i++ {
		fmt.Fprintf(&b, `{"key":"NMB-1","body":"line %d"}`+"\n", i)
	}
	withStdin(t, b.String())

	_, err := capture(t, func() error {
		return cmdComment([]string{"--batch", "-"})
	})
	if err == nil {
		t.Fatal("51 lines must be refused")
	}
	if !strings.Contains(err.Error(), "50") || !strings.Contains(err.Error(), "origin API courtesy") {
		t.Fatalf("limit error must name 50 and the reason: %v", err)
	}
	if !strings.Contains(err.Error(), "not a gadak database limit") {
		t.Fatalf("must say it is not a gadak DB limit: %v", err)
	}
	if f.called("POST /issue/NMB-1/comment") {
		t.Fatalf("limit must refuse before any write: %v", f.calls)
	}
}

func TestTransitionBatchDryRunDoesNotWrite(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1","target":"done"}`+"\n")

	out, err := capture(t, func() error {
		return cmdTransition([]string{"--batch", "-", "--dry-run", "--json"})
	})
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	mustNotTransition(t, f, "NMB-1")
	rows := parseJSONEnvelope(t, out)
	if len(rows) != 1 || !rows[0].OK || !rows[0].Changed || rows[0].TransitionID != "31" {
		t.Fatalf("dry-run row %+v from %q", rows, out)
	}
}

func TestTransitionBatchDryRunAlreadyDoneIsNoop(t *testing.T) {
	f := alreadyDoneOrigin(t)
	withStdin(t, `{"key":"NMB-1","target":"done"}`+"\n")

	out, err := capture(t, func() error {
		return cmdTransition([]string{"--batch", "-", "--dry-run", "--json"})
	})
	if err != nil {
		t.Fatalf("dry-run no-op: %v\n%s", err, out)
	}
	mustNotTransition(t, f, "NMB-1")
	rows := parseJSONEnvelope(t, out)
	if len(rows) != 1 || !rows[0].OK || rows[0].Changed || rows[0].TransitionID != "" {
		t.Fatalf("no-op row %+v from %q", rows, out)
	}
}

func TestTransitionDryRunRequiresBatch(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	_, err := capture(t, func() error {
		return cmdTransition([]string{"NMB-1", "done", "--dry-run"})
	})
	if err == nil || !strings.Contains(err.Error(), "--dry-run requires --batch") {
		t.Fatalf("dry-run without batch: %v", err)
	}
	mustNotTransition(t, f, "NMB-1")
}

func TestTransitionBatchTryAllMixed(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, ""+
		`{"key":"NMB-1","target":"done"}`+"\n"+
		`{"key":"NMB-1","target":"no-such-status"}`+"\n"+
		`{"key":"NMB-1","target":"31"}`+"\n")

	out, err := capture(t, func() error {
		return cmdTransition([]string{"--batch", "-"})
	})
	if err == nil {
		t.Fatal("mixed transition batch must fail overall")
	}
	_, rows := parseTSVEnvelope(t, out)
	if len(rows) != 3 {
		t.Fatalf("rows %d in %q", len(rows), out)
	}
	if !rows[0].OK || !rows[0].Changed {
		t.Fatalf("line 1 %+v", rows[0])
	}
	if rows[1].OK || rows[1].Error == "" {
		t.Fatalf("line 2 %+v", rows[1])
	}
	if !rows[2].OK || !rows[2].Changed {
		t.Fatalf("line 3 %+v", rows[2])
	}
	if n := countCalls(f, "POST /issue/NMB-1/transitions"); n != 2 {
		t.Fatalf("transition POSTs %d in %v", n, f.calls)
	}
}

func TestAssignBatchUnassign(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1","assignee":"-"}`+"\n")

	out, err := capture(t, func() error {
		return cmdAssign([]string{"--batch", "-"})
	})
	if err != nil {
		t.Fatalf("assign batch: %v\n%s", err, out)
	}
	_, rows := parseTSVEnvelope(t, out)
	if len(rows) != 1 || !rows[0].OK || !rows[0].Changed {
		t.Fatalf("row %+v", rows)
	}
	if !f.called("PUT /issue/NMB-1/assignee") {
		t.Fatalf("assign not sent: %v", f.calls)
	}
}

func TestEditBatchLabels(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1","labels":["+regression"]}`+"\n")

	out, err := capture(t, func() error {
		return cmdEdit([]string{"--batch", "-"})
	})
	if err != nil {
		t.Fatalf("edit batch: %v\n%s", err, out)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"add":"regression"`) {
		t.Fatalf("label add missing: %s", body)
	}
	_, rows := parseTSVEnvelope(t, out)
	if len(rows) != 1 || !rows[0].OK || !rows[0].Changed {
		t.Fatalf("row %+v", rows)
	}
}

func TestEditBatchCLILabelDefault(t *testing.T) {
	f := newFakeJira(t)
	mirror(t, f.URL)
	withStdin(t, `{"key":"NMB-1"}`+"\n")

	out, err := capture(t, func() error {
		return cmdEdit([]string{"--batch", "-", "--label", "+regression"})
	})
	if err != nil {
		t.Fatalf("edit batch flag default: %v\n%s", err, out)
	}
	body := f.bodies["PUT /issue/NMB-1"]
	if !strings.Contains(body, `"add":"regression"`) {
		t.Fatalf("CLI --label default missing: %s", body)
	}
}

func TestTransitionBatchLocalOriginRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GADAK_HOME", home)
	t.Setenv("HOME", home)
	t.Setenv("GADAK_ACTOR", "agent-a|Agent A")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	clearCredentialEnv(t)
	config.SetProfile("")
	t.Cleanup(func() {
		_ = origin.Close()
		config.SetProfile("")
	})

	if _, err := capture(t, func() error { return cmdInit([]string{"--local", "--json"}) }); err != nil {
		t.Fatalf("init --standalone: %v", err)
	}
	created, err := capture(t, func() error { return cmdCreate([]string{"batch roundtrip"}) })
	if err != nil {
		t.Fatalf("create: %v\n%s", err, created)
	}
	key := strings.Split(strings.TrimSpace(strings.Split(created, "\n")[0]), "\t")[0]

	withStdin(t, fmt.Sprintf(
		`{"key":%q,"target":"done"}`+"\n"+
			`{"key":%q,"target":"done"}`+"\n"+
			`{"key":"NOSUCH-1","target":"done"}`+"\n",
		key, key))

	out, err := capture(t, func() error {
		return cmdTransition([]string{"--batch", "-", "--json"})
	})
	if err == nil {
		t.Fatalf("mixed local-origin batch must be non-zero:\n%s", out)
	}
	if !strings.Contains(err.Error(), "1 of 3 failed") {
		t.Fatalf("summary %v", err)
	}
	rows := parseJSONEnvelope(t, out)
	if len(rows) != 3 {
		t.Fatalf("rows %d in %q", len(rows), out)
	}
	if rows[0].Key != key || !rows[0].OK || !rows[0].Changed {
		t.Fatalf("success row %+v", rows[0])
	}
	if rows[1].Key != key || !rows[1].OK || rows[1].Changed {
		t.Fatalf("no-op row %+v", rows[1])
	}
	if rows[2].Key != "NOSUCH-1" || rows[2].OK || rows[2].Error == "" {
		t.Fatalf("fail row %+v", rows[2])
	}

	issue, err := capture(t, func() error { return cmdIssue([]string{key, "--json"}) })
	if err != nil {
		t.Fatalf("issue after batch: %v\n%s", err, issue)
	}
	var doc struct {
		Issue struct {
			StatusCategory string `json:"status_category"`
		} `json:"issue"`
	}
	if err := json.Unmarshal([]byte(issue), &doc); err != nil {
		t.Fatalf("decode issue: %v\n%s", err, issue)
	}
	if doc.Issue.StatusCategory != "done" {
		t.Fatalf("status_category %q, want done", doc.Issue.StatusCategory)
	}
}

func parseTSVEnvelope(t *testing.T, out string) (header string, rows []batchResult) {
	t.Helper()
	lines := nonEmptyLines(out)
	if len(lines) == 0 {
		t.Fatalf("empty envelope:\n%s", out)
	}
	header = lines[0]
	if header != "key\tok\tchanged\terror" {
		t.Fatalf("header %q", header)
	}
	for _, line := range lines[1:] {
		cols := strings.Split(line, "\t")
		if len(cols) != 4 {
			t.Fatalf("row %q want 4 cols", line)
		}
		rows = append(rows, batchResult{
			Key:     cols[0],
			OK:      cols[1] == "true",
			Changed: cols[2] == "true",
			Error:   cols[3],
		})
	}
	return header, rows
}

func parseJSONEnvelope(t *testing.T, out string) []batchResult {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var rows []batchResult
	for dec.More() {
		var r batchResult
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("decode %q: %v", out, err)
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		t.Fatalf("no JSON envelope rows in %q", out)
	}
	return rows
}
