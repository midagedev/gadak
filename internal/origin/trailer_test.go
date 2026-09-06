package origin

// Actor trailer — contract ↔ assertion map (clause numbers are this file's
// own; each row names its FAIL-first evidence: the assertion fails on the
// pre-change source because the decorator did not exist / returned the
// writer unchanged).
//
//	A1 ActorTrailer text: name → "— via gadak · Name (slug)";
//	   no name → "— via gadak · slug"; empty slug → ""
//	   TestActorTrailer (FAIL-first: pre-change the function did not
//	   exist; with name absent the row pins that no "()"-suffix appears)
//	A2 WithActorTrailer wrap decision: no actor / built-in origin /
//	   actor.trailer false → the same writer (pointer); actor + jira or
//	   linear → wrapped
//	   TestWithActorTrailerWrapDecision (FAIL-first: pre-change every case
//	   returned the same pointer, so the "wrapped" row fails)
//	A3 append: two paragraphs → three, last is the trailer; already
//	   stamped (same actor) → byte-identical; different actor stamped →
//	   appended; last node not a paragraph → appended
//	   TestAppendActorTrailer (FAIL-first: pre-change appendActorTrailer
//	   did not exist — the count and idempotence rows cannot pass)
//	A4 3-class defense: null body, not-a-doc, no content array →
//	   returned unchanged
//	   TestAppendActorTrailer (defense rows; FAIL-first as A3)
//	A5 AddComment carries the trailer; Transition nil comment stays nil;
//	   Transition comment stamped; CreateIssue without/nil description
//	   unchanged, with description stamped and the caller's map not
//	   mutated; EditIssue and UpdateFields never stamped
//	   TestActorTrailerWriterVerbs (FAIL-first: pre-change the verbs
//	   received the body verbatim — the last-paragraph row fails)
//	A6 the optional faces forward through the wrapper: a wrapped writer
//	   that has CreateFieldCatalog still answers AsCreateFieldCatalog;
//	   one without still gets ErrNoCreateFields
//	   TestActorTrailerForwardsFaces (FAIL-first: interface embedding
//	   alone drops the face — the hit row fails without the forwarding)
//	A7 Linear: AddComment's markdown on the wire ends with the trailer
//	   line; CreateIssue's description too
//	   TestLinearWriterTrailerRoundTrip (FAIL-first: pre-change the wire
//	   body ended with the user's last line)

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/claim"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

const testTrailer = "— via gadak · Claude Test (claude:test)"

// recordingWriter is a Writer that records the three bodies the decorator
// stamps and succeeds at everything.
type recordingWriter struct {
	addBody     json.RawMessage
	trFields    map[string]any
	trComment   json.RawMessage
	trCommentOK bool
	created     map[string]any
	edited      map[string]any
	editedUpd   map[string]any
	updated     map[string]any
}

func (w *recordingWriter) CreateMeta(ctx context.Context, projects []string) ([]CreateMetaProject, error) {
	return nil, nil
}
func (w *recordingWriter) CreateIssue(ctx context.Context, fields map[string]any) (string, error) {
	w.created = fields
	return "NEW-1", nil
}
func (w *recordingWriter) EditMeta(ctx context.Context, key string) (map[string]FieldMeta, error) {
	return nil, nil
}
func (w *recordingWriter) UpdateFields(ctx context.Context, key string, fields map[string]any) error {
	w.updated = fields
	return nil
}
func (w *recordingWriter) EditIssue(ctx context.Context, key string, fields, update map[string]any) error {
	w.edited, w.editedUpd = fields, update
	return nil
}
func (w *recordingWriter) Transitions(ctx context.Context, key string) ([]Transition, error) {
	return nil, nil
}
func (w *recordingWriter) Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error {
	w.trFields, w.trComment, w.trCommentOK = fields, comment, true
	return nil
}
func (w *recordingWriter) AddComment(ctx context.Context, key string, body json.RawMessage, visibility *CommentVisibility, internal bool) (Comment, error) {
	w.addBody = body
	return Comment{ID: "c-1"}, nil
}
func (w *recordingWriter) SetAssignee(ctx context.Context, key, accountID string) error { return nil }
func (w *recordingWriter) SearchUsers(ctx context.Context, query string) ([]User, error) {
	return nil, nil
}
func (w *recordingWriter) PriorityCatalog(ctx context.Context) ([]NamedID, error) {
	return nil, nil
}
func (w *recordingWriter) Upload(ctx context.Context, key, filename string, file io.Reader) ([]Attachment, error) {
	return nil, nil
}

// recordingWriterWithFaces adds one optional face, for the forwarding hit
// row; a bare recordingWriter is the miss row.
type recordingWriterWithFaces struct {
	recordingWriter
}

func (w *recordingWriterWithFaces) CreateFields(ctx context.Context, projectIDOrKey, issueTypeID string) ([]CreateFieldMeta, error) {
	return []CreateFieldMeta{{FieldID: "cf-1"}}, nil
}

// lastParagraphText reads the plain text of an ADF doc's final paragraph.
func lastParagraphText(t *testing.T, raw json.RawMessage) (string, int) {
	t.Helper()
	var doc struct {
		Content []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	if len(doc.Content) == 0 {
		return "", 0
	}
	last := doc.Content[len(doc.Content)-1]
	var b strings.Builder
	for _, c := range last.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String(), len(doc.Content)
}

func TestActorTrailer(t *testing.T) {
	cases := []struct {
		name string
		in   config.ResolvedActor
		want string
	}{
		{"name and slug", config.ResolvedActor{Slug: "claude:test", Name: "Claude Test"}, "— via gadak · Claude Test (claude:test)"},
		{"slug only", config.ResolvedActor{Slug: "claude:test"}, "— via gadak · claude:test"},
		{"empty slug", config.ResolvedActor{Name: "Claude Test"}, ""},
		{"whitespace slug", config.ResolvedActor{Slug: "   "}, ""},
	}
	for _, tc := range cases {
		if got := ActorTrailer(tc.in); got != tc.want {
			t.Errorf("%s: ActorTrailer = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestWithActorTrailerWrapDecision(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "")
	t.Setenv("CLAUDECODE", "")
	inner := &recordingWriter{}

	// No actor at all: the same writer comes back.
	if got := WithActorTrailer(inner, &config.Config{Site: "https://x.example.com"}); got != Writer(inner) {
		t.Fatalf("no actor: writer was wrapped (%T)", got)
	}

	// Actor from config, Jira workspace: wrapped.
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")
	jiraCfg := &config.Config{Site: "https://x.example.com"}
	if got := WithActorTrailer(inner, jiraCfg); got == Writer(inner) {
		t.Fatal("actor + jira origin: writer was not wrapped")
	}

	// Built-in origin (local or paired): the header already carries the
	// actor — no trailer, same writer.
	for _, kind := range []string{config.OriginGadak, config.KindLocalOrigin} {
		cfg := &config.Config{Kind: kind}
		if got := WithActorTrailer(inner, cfg); got != Writer(inner) {
			t.Fatalf("origin kind %q: writer was wrapped (%T)", kind, got)
		}
	}

	// Linear workspace: wrapped (the write goes out under a person's key).
	linCfg := &config.Config{Kind: config.OriginLinear}
	if got := WithActorTrailer(inner, linCfg); got == Writer(inner) {
		t.Fatal("actor + linear origin: writer was not wrapped")
	}

	// The switch off: same writer, on any origin.
	f := false
	offCfg := &config.Config{Site: "https://x.example.com", Actor: &config.ActorConfig{Trailer: &f}}
	if got := WithActorTrailer(inner, offCfg); got != Writer(inner) {
		t.Fatalf("actor.trailer=false: writer was wrapped (%T)", got)
	}
}

func TestAppendActorTrailer(t *testing.T) {
	twoParas := []byte(`{"type":"doc","version":1,"content":[` +
		`{"type":"paragraph","content":[{"type":"text","text":"one"}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}`)
	cases := []struct {
		name string
		in   string
		// wantNil: the input must come back byte-identical.
		wantIdentical bool
		wantLast      string
		wantBlocks    int
	}{
		{"two paragraphs get a third", string(twoParas), false, testTrailer, 3},
		{"already stamped stays", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]},{"type":"paragraph","content":[{"type":"text","text":"— via gadak · Claude Test (claude:test)"}]}]}`, true, testTrailer, 2},
		{"another actor stamped appends", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"— via gadak · Grok (grok:aa11)"}]}]}`, false, testTrailer, 2},
		{"null body unchanged", `null`, true, "", 0},
		{"not a doc unchanged", `{"type":"paragraph","content":[]}`, true, "", 0},
		{"no content array unchanged", `{"type":"doc","version":1}`, true, "", 0},
		{"empty unchanged", ``, true, "", 0},
		{"last node a heading still appends", `{"type":"doc","version":1,"content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"head"}]}]}`, false, testTrailer, 2},
	}
	for _, tc := range cases {
		out := appendActorTrailer(json.RawMessage(tc.in), testTrailer)
		if tc.wantIdentical {
			if string(out) != tc.in {
				t.Errorf("%s: body changed:\n got %s\nwant %s", tc.name, out, tc.in)
			}
			continue
		}
		last, n := lastParagraphText(t, out)
		if tc.wantBlocks != 0 && n != tc.wantBlocks {
			t.Errorf("%s: %d blocks, want %d (%s)", tc.name, n, tc.wantBlocks, out)
		}
		if last != tc.wantLast {
			t.Errorf("%s: last paragraph %q, want %q", tc.name, last, tc.wantLast)
		}
	}
}

func TestActorTrailerWriterVerbs(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")
	inner := &recordingWriter{}
	w := WithActorTrailer(inner, &config.Config{Site: "https://x.example.com"})
	ctx := context.Background()

	body := adf.FromMarkdown("one\n\ntwo")
	if _, err := w.AddComment(ctx, "NMB-1", body, nil, false); err != nil {
		t.Fatal(err)
	}
	last, n := lastParagraphText(t, inner.addBody)
	if n != 3 || last != testTrailer {
		t.Fatalf("AddComment: %d blocks, last %q (body %s)", n, last, inner.addBody)
	}

	// Idempotent through the verb: a body that already carries this
	// actor's line goes to the origin byte-identical.
	stamped := appendActorTrailer(adf.FromMarkdown("one"), testTrailer)
	if _, err := w.AddComment(ctx, "NMB-1", stamped, nil, false); err != nil {
		t.Fatal(err)
	}
	if string(inner.addBody) != string(stamped) {
		t.Fatalf("AddComment double-stamped:\n got %s\nwant %s", inner.addBody, stamped)
	}

	// A nil transition comment stays nil — the trailer never invents a
	// comment.
	if err := w.Transition(ctx, "NMB-1", "31", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !inner.trCommentOK || len(inner.trComment) != 0 {
		t.Fatalf("nil comment became %q", inner.trComment)
	}
	if err := w.Transition(ctx, "NMB-1", "31", nil, adf.FromMarkdown("closing")); err != nil {
		t.Fatal(err)
	}
	if last, _ := lastParagraphText(t, inner.trComment); last != testTrailer {
		t.Fatalf("transition comment last paragraph %q", last)
	}

	// CreateIssue: absent and nil descriptions stay that way.
	for name, fields := range map[string]map[string]any{
		"no description":   {"summary": "s"},
		"null description": {"summary": "s", "description": nil},
	} {
		if _, err := w.CreateIssue(ctx, fields); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d, ok := inner.created["description"]; ok && d != nil {
			t.Errorf("%s: description appeared as %v", name, d)
		}
		if _, ok := inner.created["description"]; ok && fields["description"] == nil {
			// nil stays present-but-nil, not removed
		}
	}
	// A stamped create: the caller's map is not mutated.
	fields := map[string]any{"summary": "s", "description": adf.FromMarkdown("body text")}
	rawBefore := string(fields["description"].(json.RawMessage))
	if _, err := w.CreateIssue(ctx, fields); err != nil {
		t.Fatal(err)
	}
	sent, ok := inner.created["description"].(json.RawMessage)
	if !ok {
		t.Fatalf("description lost RawMessage type: %T", inner.created["description"])
	}
	if last, _ := lastParagraphText(t, sent); last != testTrailer {
		t.Fatalf("create description last paragraph %q (%s)", last, sent)
	}
	if string(fields["description"].(json.RawMessage)) != rawBefore {
		t.Fatal("caller's fields map was mutated")
	}

	// EditIssue and UpdateFields are never stamped — editing a body to add
	// the line would rewrite a person's text.
	editFields := map[string]any{"description": adf.FromMarkdown("edited")}
	if err := w.EditIssue(ctx, "NMB-1", editFields, nil); err != nil {
		t.Fatal(err)
	}
	if string(inner.edited["description"].(json.RawMessage)) != string(editFields["description"].(json.RawMessage)) {
		t.Fatal("EditIssue body was stamped")
	}
	updFields := map[string]any{"description": adf.FromMarkdown("updated")}
	if err := w.UpdateFields(ctx, "NMB-1", updFields); err != nil {
		t.Fatal(err)
	}
	if string(inner.updated["description"].(json.RawMessage)) != string(updFields["description"].(json.RawMessage)) {
		t.Fatal("UpdateFields body was stamped")
	}
}

func TestActorTrailerForwardsFaces(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")
	ctx := context.Background()

	// Hit: the wrapped writer still answers the optional face — without
	// the forwarding methods, interface embedding drops it and create
	// --field would refuse on a Jira origin.
	hit := WithActorTrailer(&recordingWriterWithFaces{}, &config.Config{Site: "https://x.example.com"})
	cf, err := AsCreateFieldCatalog(hit)
	if err != nil {
		t.Fatalf("CreateFieldCatalog lost through the wrapper: %v", err)
	}
	list, err := cf.CreateFields(ctx, "NMB", "10001")
	if err != nil || len(list) != 1 || list[0].FieldID != "cf-1" {
		t.Fatalf("forwarded CreateFields = %v, %v", list, err)
	}

	// Miss: a writer without the face keeps the same refusal sentence — it
	// now surfaces from the verb call (the wrapper must satisfy the face to
	// forward it), not from the As* assertion, but the words and the
	// propagation are unchanged for every caller that treats As*'s error
	// and the verb's error alike (create.go does: both are returns).
	miss := WithActorTrailer(&recordingWriter{}, &config.Config{Site: "https://x.example.com"})
	cfMiss, err := AsCreateFieldCatalog(miss)
	if err != nil {
		t.Fatalf("assertion itself now fails: %v", err)
	}
	if _, err := cfMiss.CreateFields(ctx, "NMB", "10001"); err == nil || !strings.Contains(err.Error(), "create-time field metadata") {
		t.Fatalf("miss sentence changed: %v", err)
	}
}

func TestLinearWriterTrailerRoundTrip(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")
	lw, rec := testLinearWriter(t)
	w := WithActorTrailer(lw, &config.Config{Kind: config.OriginLinear})
	ctx := context.Background()

	if _, err := w.AddComment(ctx, "FIX-1", adf.FromMarkdown("## Repro\n\n- step one\n- step two\n\n**bold** and `code`"), nil, false); err != nil {
		t.Fatal(err)
	}
	var vars struct {
		Input struct {
			Body string `json:"body"`
		} `json:"input"`
	}
	if err := json.Unmarshal(rec.lastVars, &vars); err != nil {
		t.Fatalf("comment variables: %v: %s", err, rec.lastVars)
	}
	lines := strings.Split(strings.TrimRight(vars.Input.Body, "\n"), "\n")
	if len(lines) == 0 || lines[len(lines)-1] != testTrailer {
		t.Fatalf("linear comment body's last line = %q, want the trailer; body:\n%s", lines[len(lines)-1], vars.Input.Body)
	}

	var docAny any
	if err := json.Unmarshal(adf.FromMarkdown("created by an agent"), &docAny); err != nil {
		t.Fatal(err)
	}
	if _, err := w.CreateIssue(ctx, map[string]any{
		"project":     map[string]any{"key": "FIX"},
		"summary":     "trailer probe",
		"description": docAny,
	}); err != nil {
		t.Fatal(err)
	}
	var create struct {
		Input struct {
			Description string `json:"description"`
		} `json:"input"`
	}
	if err := json.Unmarshal(rec.lastVars, &create); err != nil {
		t.Fatalf("create variables: %v: %s", err, rec.lastVars)
	}
	if !strings.HasSuffix(create.Input.Description, testTrailer) {
		t.Fatalf("linear create description = %q, want it to end with the trailer", create.Input.Description)
	}
}

// The two wrapper shapes keep the ad-hoc face assertions honest — contract ↔
// assertion. claim (cmd/gadak agent.go) and transition's alreadyInCategory
// (internal/transition apply.go, read-only this round) both probe the Writer
// with a type assertion where a MISSING method is the signal, not an error:
// claim's refusal names the two halves a Linear workspace should use, and
// alreadyInCategory's miss degrades to "run the transition". So the Jira
// shape must satisfy claim.Origin (FAIL-first: with only the base wrapper it
// does not — gadak claim would refuse on every Cloud workspace with an actor)
// and the Linear shape must not (FAIL-first on the reverse regression: a
// wrapper that always carried Claim would turn claim's usage sentence into a
// runtime unsupported error on Linear).
func TestTrailerShapesKeepClaimFaceHonest(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")
	ctx := context.Background()

	jc := jira.New("https://x.example.com", "e", "t")
	jc.Retries, jc.Backoff = 1, 0 // dead endpoint; refuse fast like linearwriter_verbs_test
	jw := WithActorTrailer(
		newJiraWriter(jc),
		&config.Config{Site: "https://x.example.com"},
	)
	if _, ok := jw.(claim.Origin); !ok {
		t.Fatalf("a wrapped jiraWriter must still satisfy claim.Origin (got %T)", jw)
	}
	// The same method transition's alreadyInCategory probes for (unexported
	// there, same signature here): present on the Jira shape.
	if _, ok := jw.(interface {
		IssueStatus(ctx context.Context, key string) (jira.Status, *jira.User, error)
	}); !ok {
		t.Fatalf("a wrapped jiraWriter must keep IssueStatus (got %T)", jw)
	}

	lw, _ := testLinearWriter(t)
	wrapped := WithActorTrailer(lw, &config.Config{Kind: config.OriginLinear})
	if _, ok := wrapped.(claim.Origin); ok {
		t.Fatalf("a wrapped linearWriter must NOT satisfy claim.Origin — the CLI's refusal sentence depends on the miss (got %T)", wrapped)
	}
	// Sanity that the jira shape delegates rather than stubs: Myself on the
	// wrapped jira client is the client's own call (a dead endpoint answers
	// an error, never a panic or a zero-success).
	if _, err := jw.(claim.Origin).Myself(ctx); err == nil {
		t.Fatal("Myself against a dead endpoint must error, not succeed")
	}
}

// The two wrapper shapes — contract ↔ assertion (FAIL-first: with the
// faces missing, the jira-family rows fail on the type assertions; with
// the faces on the plain shape too, the linear rows fail — absence is the
// signal for claim and the resolution catalog, so each face lives on
// exactly one shape):
//
//	A8 a wrapped jira-family writer still satisfies claim.Origin (gadak
//	   claim's probe) and the resolution-catalog probe transition's
//	   --resolution name lookup keys on   → TestActorTrailerWrapperShapes
//	A9 a wrapped non-jira writer (the Linear shape) keeps BOTH probes'
//	   misses, so the refusal sentences stand   → TestActorTrailerWrapperShapes
func TestActorTrailerWrapperShapes(t *testing.T) {
	t.Setenv("GADAK_ACTOR", "claude:test|Claude Test")

	jw := newJiraWriter(jira.New("https://x.example.com", "a@b.c", "tok"))
	wrapped := WithActorTrailer(jw, &config.Config{Site: "https://x.example.com"})
	if _, ok := wrapped.(claim.Origin); !ok {
		t.Fatal("wrapped jira-family writer no longer satisfies claim.Origin — gadak claim would refuse on every Jira workspace with an actor")
	}
	if _, ok := wrapped.(interface {
		Resolutions(context.Context) ([]jira.NamedID, error)
	}); !ok {
		t.Fatal("wrapped jira-family writer lost the resolution catalog — transition --resolution <name> would refuse")
	}
	if _, ok := wrapped.(interface {
		IssueStatus(context.Context, string) (jira.Status, *jira.User, error)
	}); !ok {
		t.Fatal("wrapped jira-family writer lost IssueStatus — the already-in-category no-op guard would go dark (GDK-632)")
	}

	// The Linear shape: a writer that is not jira-family keeps the probe
	// misses — claim's refusal and the resolution-catalog refusal are the
	// answers those surfaces gave before the trailer existed.
	plain := WithActorTrailer(&recordingWriter{}, &config.Config{Kind: config.OriginLinear})
	if _, ok := plain.(claim.Origin); ok {
		t.Fatal("plain wrapper satisfies claim.Origin — the linear claim refusal would be skipped")
	}
	if _, ok := plain.(interface {
		Resolutions(context.Context) ([]jira.NamedID, error)
	}); ok {
		t.Fatal("plain wrapper satisfies the resolution-catalog probe — the linear refusal sentence would change")
	}
}
