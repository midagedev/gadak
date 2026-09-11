package main

// The agent-facing write verbs — comment, transition, close, assign,
// claim — and the write session they share. Writes go to the origin first
// and re-read the issue into the mirror afterwards, in that order — the
// same write-through shape internal/server/write.go implements for the
// UI, because a write the origin rejected must not leave a trace locally.
// Split from agent.go (GDK-1771).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/midagedev/gadak/internal/fields"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/midagedev/gadak/internal/adf"
	"github.com/midagedev/gadak/internal/claim"
	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/linear"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/server"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
	"github.com/midagedev/gadak/internal/workspace"
)

var errNoCredential = config.NotConfiguredWith("writes go to the origin, not to the mirror")

// writeNotMirroredError is the lookup miss after a write Jira already accepted.
// mutate returns it (non-zero). create prints the new key with this wording
// and exits 0 — the write happened.
type writeNotMirroredError struct{ Key string }

func (e writeNotMirroredError) Error() string {
	return fmt.Sprintf("write applied to %s, but it is not in the mirror — is it outside the configured projects?", e.Key)
}

// writerForAgentWrite is the CLI's WriterFor mint: the actor-trailer
// decorator over origin.WriterFor, so every agent-authored comment,
// transition comment, and created issue on a Jira or Linear origin carries
// the "— via gadak · <actor>" line when an actor resolves (origin.WithActorTrailer
// owns every no-wrap rule: no actor, actor.trailer false, built-in origin).
// withCreateSession, withKeyWriteSession and create --batch's per-line
// reroute all mint here — three sites, one helper, no drift.
func writerForAgentWrite(cfg *config.Config, src string) (origin.Writer, error) {
	w, err := origin.WriterFor(cfg, src)
	if err != nil {
		return nil, err
	}
	return origin.WithActorTrailer(w, cfg), nil
}

// withCreateSession is create's write session: HasCredential (which counts
// a Linear apiKey) then WriterFor routed by --project / Linear-only.
// Mutate uses withKeyWriteSession — it already has a key.
func withCreateSession(project string, fn func(context.Context, *config.Config, *store.DB, origin.Writer, string) error) error {
	warnWorkspaceIfEnv()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.HasCredential() {
		return errNoCredential
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)
	ctx := context.Background()
	src, err := origin.ResolveCreateSource(ctx, cfg, db, project)
	if err != nil {
		return err
	}
	c, err := writerForAgentWrite(cfg, src)
	if err != nil {
		return origin.FoldPairedError(cfg, err)
	}
	return origin.FoldPairedError(cfg, fn(ctx, cfg, db, c, src))
}

// withKeyWriteSession is create's sibling routed per key: the mirror says
// which origin owns the row (store.KeySource — a "MID-5" can be Linear or
// Jira, the shape cannot tell), and the credential gate is that origin's.
//
// The empty-key check is the single owner for every key-addressed write
// (edit, comment, transition, assign, claim, link, unlink, attach): a blank
// key used to fall through to the origin as PUT /issue/ (GDK-1593 pt 3),
// so it fires before anything else this session might do.
func withKeyWriteSession(key string, fn func(context.Context, *config.Config, *store.DB, origin.Writer, string) error) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("empty issue key — a write needs the issue it applies to (ABC-123)")
	}
	warnWorkspaceIfEnv()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	warnIfStale(db)
	ctx := context.Background()
	src, err := db.KeySource(ctx, key)
	if err != nil {
		if errors.Is(err, store.ErrKeyAmbiguous) {
			return err
		}
		src = ""
	}
	if src != "linear" && !cfg.HasAtlassianCredential() {
		return errNoCredential
	}
	c, err := writerForAgentWrite(cfg, src)
	if err != nil {
		return origin.FoldPairedError(cfg, err)
	}
	return origin.FoldPairedError(cfg, fn(ctx, cfg, db, c, src))
}

// emitAfterWrite is the write-through tail: re-read the issue into the mirror
// and print the refreshed row. A failure between the write and the re-read is
// a stale cache, not a failed write: the origin already accepted it.
func emitAfterWrite(ctx context.Context, cfg *config.Config, db *store.DB, src, key string, asJSON bool, extra map[string]any) error {
	if err := syncer.RefreshIssue(ctx, cfg, db, key, src); err != nil {
		return emitWriteAppliedMirrorStale(db, key, asJSON, extra, err)
	}
	lites, err := lookup(db, []string{key})
	if err != nil {
		return err
	}
	if len(lites) == 0 {
		return writeNotMirroredError{Key: key}
	}
	if asJSON {
		body := map[string]any{"issue": lites[0]}
		for k, v := range extra {
			body[k] = v
		}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	// The refreshed row is every write's confirmation except one: a comment
	// changes nothing the row carries, so the comment write prints its own
	// line instead (GDK-1019).
	if line := commentAddedLine(key, extra); line != "" {
		fmt.Println(line)
		return nil
	}
	fmt.Println(summaryLine(lites[0]))
	return nil
}

// commentBodyCols is the excerpt budget on the comment confirmation line —
// display columns, not runes, so clip's CJK rule holds here too.
const commentBodyCols = 60

// commentAddedLine is the text success line for the one write whose effect
// the refreshed TSV row cannot show: a comment. `gadak page comment` already
// prints this shape (`comment <id> added`); the excerpt is what tells a
// caller with no session which text landed (GDK-1019). edited/deleted are
// the `comment edit` / `comment rm` verbs (GDK-1647) — rm prints no excerpt
// because there is no body left to quote. Empty when extra is not a comment
// write — every other verb keeps its summary row. The body is the origin's
// echo (postComment's extra), not the text that was sent.
func commentAddedLine(key string, extra map[string]any) string {
	m, ok := extra["comment"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := m["comment_id"].(string)
	body, _ := m["body"].(string)
	switch {
	case m["deleted"] == true:
		return fmt.Sprintf("%s\tcomment %s deleted", key, id)
	case m["edited"] == true:
		return fmt.Sprintf("%s\tcomment %s edited: %q", key, id, clip(body, commentBodyCols))
	default:
		return fmt.Sprintf("%s\tcomment %s added: %q", key, id, clip(body, commentBodyCols))
	}
}

// writeAppliedMirrorStaleMessage is the one author of the CLI warning for a
// landed write whose re-read failed. Call sites classify with errors.Is;
// they do not hand-write this sentence.
func writeAppliedMirrorStaleMessage(key string, err error) string {
	return fmt.Sprintf("write applied to %s, but the mirror did not refresh (run `gadak sync`): %v", key, err)
}

func warnWriteAppliedMirrorStale(key string, err error) {
	fmt.Fprintln(os.Stderr, writeAppliedMirrorStaleMessage(key, err))
}

func emitWriteAppliedMirrorStale(db *store.DB, key string, asJSON bool, extra map[string]any, err error) error {
	return emitWriteAppliedMirrorStaleFor(db, key, key, asJSON, extra, err)
}

func emitWriteAppliedMirrorStaleFor(db *store.DB, warnKey, rowKey string, asJSON bool, extra map[string]any, err error) error {
	warnWriteAppliedMirrorStale(warnKey, err)
	lites, _ := lookup(db, []string{rowKey})
	if asJSON {
		body := map[string]any{"mirror_stale": true}
		if len(lites) > 0 {
			body["issue"] = lites[0]
		}
		for k, v := range extra {
			body[k] = v
		}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	// The comment landed even though the mirror did not refresh — and this is
	// the branch where a caller most needs to be told the write itself
	// succeeded, so the confirmation line leads here too (GDK-1019).
	if line := commentAddedLine(rowKey, extra); line != "" {
		fmt.Println(line)
		return nil
	}
	if len(lites) == 0 {
		fmt.Println(rowKey)
		return nil
	}
	fmt.Println(summaryLine(lites[0]))
	return nil
}

func batchAfterWrite(key string, changed bool, err error) batchResult {
	if err == nil {
		return batchOK(key, changed)
	}
	if errors.Is(err, syncer.ErrMirrorStale) {
		warnWriteAppliedMirrorStale(key, err)
		return batchStale(key, changed)
	}
	return batchErr(key, changed, err)
}

// mutate is the whole write-through shape: call the origin that owns the
// key, re-read the issue into the mirror, then print the refreshed row.
// verb is the ledger name (GDK-1440): the row local.agent_writes gets when
// the origin accepts the write — dry runs and refusals never reach the
// recording, which sits after fn's nil and before the write-through tail.
func mutate(verb, key string, asJSON bool, fn func(context.Context, origin.Writer, string) (map[string]any, error)) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		extra, err := fn(ctx, c, src)
		if err != nil {
			return err
		}
		recordAgentWrite(ctx, db, key, verb)
		return emitAfterWrite(ctx, cfg, db, src, key, asJSON, extra)
	})
}

// recordAgentWrite lands one row in local.agent_writes (GDK-1440) — the
// ledger of what agent sessions actually wrote, keyed by whatever the verb
// addresses. Best-effort by design: the origin already accepted the write,
// so a history failure must not turn a landed write into an error; it says
// so on stderr and the write stands. Callers place it after the origin call
// returns nil, so a refused or dry-run write never reaches it.
func recordAgentWrite(ctx context.Context, db *store.DB, key, verb string) {
	if db == nil {
		return
	}
	if err := db.RecordAgentWrite(ctx, key, verb, store.VisitSourceCLI); err != nil {
		fmt.Fprintf(os.Stderr, "gadak: local history: %v (the write itself landed)\n", err)
	}
}

// recordAPIWrite is `gadak api --write`'s ledger row (GDK-1440): the raw
// escape hatch records what it can honestly know — the route as typed under
// verb "api", not a guessed issue key or verb (a POST there can create
// anything). Only a 2xx counts; the origin refused everything else. Rides
// the store handle the usage flush already opened in both api branches.
func recordAPIWrite(ctx context.Context, db *store.DB, method, path string, mutating bool, status int) {
	if !mutating || status < 200 || status >= 300 {
		return
	}
	recordAgentWrite(ctx, db, path, "api")
}

// maxMentionWords is the longest @-candidate we will ask the origin about.
// Three words covers "Dana Whitfield" plus one trailing word we may have to
// reject; unbounded combinatorics are not allowed.
func cmdComment(args []string) error {
	// Subverbs dispatch on the first raw positional, before the comment flag
	// set parses anything: `comment rm` owns --yes, which the plain-comment
	// set would reject as unknown (recipes/page precedent). A key always
	// carries a dash ("NMB-140"), so no key can collide with a subverb.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "edit":
			return cmdCommentEdit(args[1:])
		case "rm":
			return cmdCommentRm(args[1:])
		}
	}
	fs := newFlagSet("comment")
	text := fs.String("m", "", "comment body; `-` reads it from stdin")
	adfFile := fs.String("adf-file", "", "comment body as an ADF JSON document file, sent to the origin as it is; exclusive with -m and positional text")
	asJSON := fs.Bool("json", false, "emit JSON")
	dryRun := fs.Bool("dry-run", false, "print the request this comment would send and exit; nothing reaches the origin")
	internal := fs.Bool("internal", false, "post as a JSM internal comment")
	var visRaw labelFlags
	fs.Var(&visRaw, "visibility", "restrict to role=NAME or group=NAME (once)")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs key and body")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("comment", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	vis, err := parseCommentVisibility(visRaw)
	if err != nil {
		return err
	}
	if *batch != "" {
		if err := rejectBatchFlag(*batch, *text); err != nil {
			return err
		}
		if len(pos) != 0 {
			return usageError("comment", "usage: gadak comment: --batch and a key/body are mutually exclusive")
		}
		return runCommentBatch(*asJSON, *internal, vis, *text)
	}
	if len(pos) == 0 {
		return usageError("comment", commentUsage)
	}
	key := fields.CanonicalKey(pos[0])
	body := *text
	// Trailing positional words are the body, like create's positional
	// SUMMARY (GDK-315). With -m too it is ambiguous — refuse.
	if len(pos) > 1 {
		if body != "" {
			return usageError("comment", "comment body given twice — positional text and -m; pick one")
		}
		body = strings.Join(pos[1:], " ")
	}
	if *adfFile != "" {
		// GDK-1395: the raw round trip. The file is the whole body, so text
		// beside it is a second body — refuse rather than pick.
		if body != "" {
			return usageError("comment", "usage: gadak comment: --adf-file is exclusive with -m and positional text — the file is the whole body")
		}
		doc, err := readADFFile(*adfFile)
		if err != nil {
			return fmt.Errorf("comment %s: %w", key, err)
		}
		return foldDryRun(mutate("comment", key, *asJSON, func(ctx context.Context, c origin.Writer, _ string) (map[string]any, error) {
			if *dryRun {
				// The file is the body the origin would carry verbatim, so
				// the plan carries it verbatim too — no mention pass runs on
				// a document the caller already wrote (GDK-1446).
				if err := emitDryRun("comment", map[string]any{"body_adf": doc}, key); err != nil {
					return nil, err
				}
				return nil, errDryRun
			}
			return postCommentDoc(ctx, c, key, doc, vis, *internal)
		}))
	}
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("empty comment — pass -m <text>, or -m - to read stdin, or gadak comment KEY <text>")
	}
	return foldDryRun(mutate("comment", key, *asJSON, func(ctx context.Context, c origin.Writer, _ string) (map[string]any, error) {
		if *dryRun {
			// Run the body reader a real write runs — placeholder refusal, the
			// mention pass — so a plan that would refuse refuses instead of
			// promising a write the origin would reject (GDK-1446). The plan
			// itself carries the typed text, the person's words, not the ADF
			// the origin would wrap them in.
			if _, err := commentBodyDoc(ctx, c, key, body); err != nil {
				return nil, err
			}
			req := map[string]any{"body": body}
			if vis != nil {
				req["visibility"] = vis.Type + "=" + vis.Value
			}
			if *internal {
				req["internal"] = true
			}
			if err := emitDryRun("comment", req, key); err != nil {
				return nil, err
			}
			return nil, errDryRun
		}
		return postComment(ctx, c, key, body, vis, *internal)
	}))
}

const commentUsage = "usage: gadak comment <KEY> [<text> | -m <text|-> | --adf-file F] [--visibility role=NAME|group=NAME] [--internal] [--json] [--dry-run] | --batch -\n" +
	"       gadak comment edit <KEY> <ID> [-m <text|-> | --adf-file F] [--json]\n" +
	"       gadak comment rm <KEY> <ID> --yes [--json]"

const commentEditUsage = "usage: gadak comment edit <KEY> <ID> [-m <text|-> | --adf-file F] [--json]"

const commentRmUsage = "usage: gadak comment rm <KEY> <ID> --yes [--json]"

// cmdCommentEdit is `gadak comment edit <KEY> <ID>` (GDK-1647): replace a
// comment's body through the origin. The body arrives exactly the way
// `comment` takes one — -m, -m -, --adf-file, and the same empty-body
// refusal — through the post's own reader (commentBodyDoc), not a second
// one; an edit sends what a post sends.
func cmdCommentEdit(args []string) error {
	fs := newFlagSet("comment")
	text := fs.String("m", "", "comment body; `-` reads it from stdin")
	adfFile := fs.String("adf-file", "", "comment body as an ADF JSON document file, sent to the origin as it is; exclusive with -m")
	asJSON := fs.Bool("json", false, "emit JSON")
	dryRun := fs.Bool("dry-run", false, "print the request this edit would send and exit; nothing reaches the origin")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("comment", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return usageError("comment", commentEditUsage)
	}
	key := fields.CanonicalKey(pos[0])
	id := commentOriginID(pos[1])
	if id == "" {
		return usageError("comment", commentEditUsage)
	}
	body := *text
	if *adfFile != "" {
		// The file is the whole body (GDK-1395); text beside it is a second
		// body — refuse rather than pick.
		if body != "" {
			return usageError("comment", "usage: gadak comment edit: --adf-file is exclusive with -m — the file is the whole body")
		}
		doc, err := readADFFile(*adfFile)
		if err != nil {
			return fmt.Errorf("comment %s: %w", key, err)
		}
		return foldDryRun(mutate("comment edit", key, *asJSON, func(ctx context.Context, c origin.Writer, _ string) (map[string]any, error) {
			if *dryRun {
				if err := emitDryRun("comment edit", map[string]any{"comment_id": id, "body_adf": doc}, key); err != nil {
					return nil, err
				}
				return nil, errDryRun
			}
			return editCommentDoc(ctx, c, key, id, doc)
		}))
	}
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("empty comment — pass -m <text>, or -m - to read stdin, or --adf-file F")
	}
	return foldDryRun(mutate("comment edit", key, *asJSON, func(ctx context.Context, c origin.Writer, _ string) (map[string]any, error) {
		if *dryRun {
			// Same validation contract as comment's dry run (GDK-1446): the
			// body reader runs, the plan carries the typed text.
			if _, err := commentBodyDoc(ctx, c, key, body); err != nil {
				return nil, err
			}
			if err := emitDryRun("comment edit", map[string]any{"comment_id": id, "body": body}, key); err != nil {
				return nil, err
			}
			return nil, errDryRun
		}
		return editComment(ctx, c, key, id, body)
	}))
}

// cmdCommentRm is `gadak comment rm <KEY> <ID> --yes` (GDK-1647). --yes is
// required: a delete is not recoverable — there is no trash, and on the
// built-in tracker the comment leaves the persist file, which is the record.
func cmdCommentRm(args []string) error {
	fs := newFlagSet("comment")
	yes := fs.Bool("yes", false, "delete the comment — without it, rm explains and refuses")
	asJSON := fs.Bool("json", false, "emit JSON")
	dryRun := fs.Bool("dry-run", false, "print the delete this rm would send and exit; nothing reaches the origin")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("comment", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return usageError("comment", commentRmUsage)
	}
	if !*yes {
		return usageError("comment", "comment rm deletes the comment at the origin — that is not recoverable, and on the built-in tracker it leaves the persist file (no trash); re-run with --yes to delete")
	}
	key := fields.CanonicalKey(pos[0])
	id := commentOriginID(pos[1])
	if id == "" {
		return usageError("comment", commentRmUsage)
	}
	if *dryRun {
		// The delete needs no read to plan — the id is the whole request —
		// so there is no origin session to open (GDK-1446).
		return emitDryRun("comment rm", map[string]any{"comment_id": id}, key)
	}
	return mutate("comment rm", key, *asJSON, func(ctx context.Context, c origin.Writer, _ string) (map[string]any, error) {
		ed, err := origin.AsCommentEditor(c)
		if err != nil {
			return nil, err
		}
		if err := ed.DeleteComment(ctx, key, id); err != nil {
			return nil, err
		}
		return map[string]any{"comment": map[string]any{
			"comment_id": id,
			"deleted":    true,
		}}, nil
	})
}

// commentOriginID accepts what a read hands out: `gadak sql` returns the
// mirror's namespaced comment id (`jira:91653`, `standalone-jira:7`,
// `linear:<uuid>`), while `gadak issue` prints the origin's own (`91653`).
// The namespace prefixes carry no colon, so stripping at the first colon
// admits both spellings (GDK-1647).
func commentOriginID(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[i+1:]
	}
	return id
}

var commentBatchFields = []string{"key", "body", "internal", "visibility"}

func postComment(ctx context.Context, c origin.Writer, key, body string, vis *jira.CommentVisibility, internal bool) (map[string]any, error) {
	doc, err := commentBodyDoc(ctx, c, key, body)
	if err != nil {
		return nil, err
	}
	return postCommentDoc(ctx, c, key, doc, vis, internal)
}

// commentBodyDoc is the comment body reader post and edit share (GDK-1647):
// placeholder refusal, the mention pass, then the origin's body dialect —
// a document everywhere but Jira Server, where the typed characters ride
// verbatim (GDK-1637). An edit sends what a post sends.
func commentBodyDoc(ctx context.Context, c origin.Writer, key, body string) (json.RawMessage, error) {
	// The origin decides what a comment body is: a document, or the typed
	// characters (GDK-1637). A config that will not load leaves both the
	// refusal and the value on the markdown default, which is what every
	// origin but Jira Server wants and what this path did before.
	cfg, _ := config.Load()
	if err := origin.RefuseBodyPlaceholders(cfg, body); err != nil {
		return nil, fmt.Errorf("comment %s: %w", key, err)
	}
	mentions, resolved, unresolved, err := resolveCommentMentions(ctx, c, body)
	if err != nil {
		return nil, err
	}
	noticeResolvedMentions(resolved)
	warnUnresolvedMentions(unresolved)
	// A Jira Server comment is a wiki-markup string, not a document
	// (GDK-1637). The mention pass above still runs — it resolves names
	// for the notice, and jira.Doc's mention nodes simply have nowhere to
	// go on an origin whose comment field is text.
	return origin.BodyValue(cfg, body, jira.Doc(body, mentions)), nil
}

// postCommentDoc posts a finished ADF document — postComment's tail, and the
// whole of --adf-file (GDK-1395), where no mention pass runs because the
// document already says what it says.
func postCommentDoc(ctx context.Context, c origin.Writer, key string, doc json.RawMessage, vis *jira.CommentVisibility, internal bool) (map[string]any, error) {
	created, err := c.AddComment(ctx, key, doc, vis, internal)
	if err != nil {
		return nil, err
	}
	return map[string]any{"comment": map[string]any{
		"comment_id": created.ID,
		"author":     created.Author.DisplayName,
		"body":       adf.PlainText(created.Body),
	}}, nil
}

// editComment is postComment's edit twin (GDK-1647): same body reader, then
// UpdateComment through the CommentEditor face. The unsupported-origin
// refusal (ErrNoCommentEdit) surfaces from AsCommentEditor unchanged, the
// same way `gadak link` surfaces ErrNoIssueLinks.
func editComment(ctx context.Context, c origin.Writer, key, id, body string) (map[string]any, error) {
	doc, err := commentBodyDoc(ctx, c, key, body)
	if err != nil {
		return nil, err
	}
	return editCommentDoc(ctx, c, key, id, doc)
}

// editCommentDoc sends a finished ADF document — editComment's tail, and the
// whole of --adf-file, where no mention pass runs because the document
// already says what it says (the postCommentDoc split, mirrored).
func editCommentDoc(ctx context.Context, c origin.Writer, key, id string, doc json.RawMessage) (map[string]any, error) {
	ed, err := origin.AsCommentEditor(c)
	if err != nil {
		return nil, err
	}
	updated, err := ed.UpdateComment(ctx, key, id, doc)
	if err != nil {
		return nil, err
	}
	return map[string]any{"comment": map[string]any{
		"comment_id": updated.ID,
		"author":     updated.Author.DisplayName,
		"body":       adf.PlainText(updated.Body),
		"edited":     true,
	}}, nil
}

func runCommentBatch(asJSON, internalDefault bool, visDefault *jira.CommentVisibility, bodyDefault string) error {
	return runWriteBatch("comment", asJSON, false, func(raw string) batchResult {
		obj, key, err := parseBatchLine(raw, commentBatchFields)
		if err != nil {
			return batchErr(key, false, err)
		}
		body := bodyDefault
		if s, ok, err := jsonStringField(obj, "body"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			body = s
		}
		if strings.TrimSpace(body) == "" {
			return batchErr(key, false, errors.New("empty comment — JSON line needs \"body\""))
		}
		internal := internalDefault
		if v, ok, err := jsonBoolField(obj, "internal"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			internal = v
		}
		vis := visDefault
		if s, ok, err := jsonStringField(obj, "visibility"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			parsed, verr := parseCommentVisibility([]string{s})
			if verr != nil {
				return batchErr(key, false, verr)
			}
			vis = parsed
		}
		var wrote bool
		err = withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
			if _, err := postComment(ctx, c, key, body, vis, internal); err != nil {
				return err
			}
			recordAgentWrite(ctx, db, key, "comment")
			wrote = true
			return syncer.RefreshIssue(ctx, cfg, db, key, src)
		})
		return batchAfterWrite(key, wrote, err)
	})
}

// parseCommentVisibility accepts --visibility role=NAME or group=NAME once.
// A second occurrence or a value that is not that shape is a usage error
// (FlagSet ExitOnError would os.Exit on Set error, so validation is here).
func parseCommentVisibility(vals []string) (*jira.CommentVisibility, error) {
	if len(vals) == 0 {
		return nil, nil
	}
	if len(vals) > 1 {
		return nil, usageError("comment", "--visibility may be given only once")
	}
	typ, val, ok := strings.Cut(vals[0], "=")
	if !ok || (typ != "role" && typ != "group") || strings.TrimSpace(val) == "" {
		return nil, usageError("comment", "--visibility needs role=NAME or group=NAME")
	}
	return &jira.CommentVisibility{Type: typ, Value: val}, nil
}

func commentMark(c store.DetailComment) string {
	var parts []string
	if c.VisibilityType != "" {
		parts = append(parts, fmt.Sprintf("[restricted: %s %s]", c.VisibilityType, c.VisibilityValue))
	}
	if c.JsdPublic != nil && !*c.JsdPublic {
		parts = append(parts, "[internal]")
	}
	return strings.Join(parts, " ")
}

const transitionUsage = "usage: gadak transition <KEY> <transition-id|status-id|name|new|inprogress|done> [--resolution name|id] [--field key=JSON]... [-m text] [--json] [--dry-run] | --batch - [--dry-run]"

const closeUsage = "usage: gadak close <KEY> [--resolution name|id] [--field key=JSON]... [-m text] [--json] [--dry-run]"

func newTransitionFlags(name string) (*flag.FlagSet, *bool, *string, *labelFlags, *string, *bool) {
	fs := newFlagSet(name)
	asJSON := fs.Bool("json", false, "emit JSON")
	resolution := fs.String("resolution", "", "resolution name or id; a name is resolved from the transition's allowedValues, else GET /resolution")
	var fieldFlags labelFlags
	fs.Var(&fieldFlags, "field", "screen field key from `gadak transition KEY` (not a configured alias); key=JSON (repeatable); a value that is not JSON is sent as a string")
	text := fs.String("m", "", "comment posted with the transition; `-` reads it from stdin")
	dryRun := fs.Bool("dry-run", false, "print the resolved transition id (or the no-op) this write would send and exit; nothing reaches the origin")
	return fs, asJSON, resolution, &fieldFlags, text, dryRun
}

func cmdTransition(args []string) error {
	fs, asJSON, resolution, fieldFlags, text, dryRun := newTransitionFlags("transition")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs key and target")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("transition", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if *batch != "" {
		if err := rejectBatchFlag(*batch, *text); err != nil {
			return err
		}
		if len(pos) != 0 {
			return usageError("transition", "usage: gadak transition: --batch and a key/target are mutually exclusive")
		}
		parsed, err := parseTransitionFieldFlags(*fieldFlags)
		if err != nil {
			return err
		}
		return runTransitionBatch(*asJSON, *dryRun, *resolution, parsed, *text)
	}
	if len(pos) < 1 {
		return usageError("transition", transitionUsage)
	}
	key := fields.CanonicalKey(pos[0])
	if len(pos) < 2 {
		if strings.TrimSpace(*resolution) != "" || len(*fieldFlags) > 0 || *text != "" {
			return usageError("transition", transitionUsage)
		}
		return listTransitions(key, *asJSON)
	}
	// Trailing words join the target so an unquoted `In Review` still works.
	want := strings.TrimSpace(strings.Join(pos[1:], " "))
	body, err := readTransitionComment(*text)
	if err != nil {
		return err
	}
	parsed, err := parseTransitionFieldFlags(*fieldFlags)
	if err != nil {
		return err
	}
	return applyTransitionWrite("transition", key, want, *resolution, parsed, body, *asJSON, *dryRun)
}

func cmdClose(args []string) error {
	fs, asJSON, resolution, fieldFlags, text, dryRun := newTransitionFlags("close")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("close", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("close", closeUsage)
	}
	key := fields.CanonicalKey(pos[0])
	body, err := readTransitionComment(*text)
	if err != nil {
		return err
	}
	parsed, err := parseTransitionFieldFlags(*fieldFlags)
	if err != nil {
		return err
	}
	return applyTransitionWrite("close", key, "done", *resolution, parsed, body, *asJSON, *dryRun)
}

func readTransitionComment(text string) (string, error) {
	if text != "-" {
		return text, nil
	}
	buf, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	body := string(buf)
	if strings.TrimSpace(body) == "" {
		return "", errors.New("empty comment — pass -m <text>, or -m - to read stdin")
	}
	return body, nil
}

// applyTransitionWrite is transition's and close's shared write path. verb
// names the command the person typed (the dry-run note and plan carry it);
// close and transition send the same request. dryRun resolves the transition
// through Preview — the same resolveTransition the write runs, so a plan
// cannot name an id the write would not fire — and prints it instead of
// sending (GDK-1446).
func applyTransitionWrite(verb, key, want, resolution string, fields map[string]any, comment string, asJSON bool, dryRun bool) error {
	return foldDryRun(withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		if dryRun {
			id, changed, err := transition.Preview(ctx, c, key, want, transition.MirrorStatusUse(ctx, db, key))
			if err != nil {
				return formatTransitionError(err, cfg)
			}
			req := map[string]any{}
			if !changed {
				// A category token the issue already reports is a no-op the
				// write would not send; the plan says that instead of an id.
				req["noop"] = true
			} else {
				req["transition_id"] = id
				if strings.TrimSpace(resolution) != "" {
					req["resolution"] = resolution
				}
				if len(fields) > 0 {
					req["fields"] = fields
				}
				if strings.TrimSpace(comment) != "" {
					req["comment"] = comment
				}
			}
			if err := emitDryRun(verb, req, key); err != nil {
				return err
			}
			return errDryRun
		}
		res, err := applyTransition(ctx, c, cfg, db, key, want, resolution, fields, comment)
		if err != nil {
			return err
		}
		recordAgentWrite(ctx, db, key, verb)
		return emitTransitionResult(ctx, cfg, db, src, key, want, comment, asJSON, res)
	}))
}

func applyTransition(ctx context.Context, c origin.Writer, cfg *config.Config, db *store.DB, key, want, resolution string, fields map[string]any, comment string) (transition.Result, error) {
	res, err := transition.Apply(ctx, c, cfg, transition.Request{
		Key:        key,
		Target:     want,
		Resolution: resolution,
		Fields:     fields,
		Comment:    comment,
		StatusUse:  transition.MirrorStatusUse(ctx, db, key),
	})
	if err := formatTransitionError(err, cfg); err != nil {
		return transition.Result{}, err
	}
	return res, nil
}

var transitionBatchFields = []string{"key", "target", "resolution", "fields", "comment"}

func runTransitionBatch(asJSON, dryRun bool, resolutionDefault string, fieldsDefault map[string]any, commentDefault string) error {
	return runWriteBatch("transition", asJSON, false, func(raw string) batchResult {
		obj, key, err := parseBatchLine(raw, transitionBatchFields)
		if err != nil {
			return batchErr(key, false, err)
		}
		want, ok, err := jsonStringField(obj, "target")
		if err != nil {
			return batchErr(key, false, err)
		}
		if !ok || strings.TrimSpace(want) == "" {
			return batchErr(key, false, errors.New("JSON line needs \"target\""))
		}
		want = strings.TrimSpace(want)
		resolution := resolutionDefault
		if s, ok, err := jsonStringField(obj, "resolution"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			resolution = s
		}
		fields := fieldsDefault
		if v, ok, err := jsonAnyObjectField(obj, "fields"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			fields = v
		}
		comment := commentDefault
		if s, ok, err := jsonStringField(obj, "comment"); err != nil {
			return batchErr(key, false, err)
		} else if ok {
			comment = s
		}
		if dryRun {
			var id string
			var changed bool
			err = withKeyWriteSession(key, func(ctx context.Context, _ *config.Config, db *store.DB, c origin.Writer, _ string) error {
				var perr error
				id, changed, perr = transition.Preview(ctx, c, key, want, transition.MirrorStatusUse(ctx, db, key))
				return perr
			})
			if err != nil {
				return batchErr(key, false, err)
			}
			return batchDryRun(key, id, changed)
		}
		var changed bool
		err = withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
			res, err := applyTransition(ctx, c, cfg, db, key, want, resolution, fields, comment)
			if err != nil {
				return err
			}
			recordAgentWrite(ctx, db, key, "transition")
			changed = res.Changed
			return syncer.RefreshIssue(ctx, cfg, db, key, src)
		})
		return batchAfterWrite(key, changed, err)
	})
}

func emitTransitionResult(ctx context.Context, cfg *config.Config, db *store.DB, src, key, want, comment string, asJSON bool, res transition.Result) error {
	extra := map[string]any{"changed": res.Changed}
	if res.Changed {
		return emitAfterWrite(ctx, cfg, db, src, key, asJSON, extra)
	}
	token, ok := transition.StatusCategoryToken(want)
	if !ok {
		token = want
	}
	if asJSON {
		if err := syncer.RefreshIssue(ctx, cfg, db, key, src); err != nil {
			return emitWriteAppliedMirrorStale(db, key, true, extra, err)
		}
		lites, err := lookup(db, []string{key})
		if err != nil {
			return err
		}
		if len(lites) == 0 {
			return writeNotMirroredError{Key: key}
		}
		body := map[string]any{"issue": lites[0], "changed": false}
		return json.NewEncoder(os.Stdout).Encode(body)
	}
	fmt.Fprintf(os.Stdout, "already %s — nothing to do\n", token)
	if strings.TrimSpace(comment) != "" {
		fmt.Fprintln(os.Stdout, "comment not posted")
	}
	return nil
}

// formatTransitionError adds CLI flag names to core refusals that do not
// name them (the core is shared with REST).
func formatTransitionError(err error, cfg *config.Config) error {
	if err == nil {
		return nil
	}
	var req *transition.RequiredFieldsError
	if errors.As(err, &req) {
		return fmt.Errorf("%w — pass --resolution NAME or --field resolution={\"id\":...}", req)
	}
	// A Built-in workflow seeded before 0.20.1 has no resolution on its done
	// screen, and issuetap answers in Cloud's words — "not on the appropriate
	// screen" sends a person to fix a screen this tracker does not have
	// (GDK-1347). Say what it is.
	var api *jira.APIError
	if cfg != nil && cfg.OriginType() == config.OriginGadak && errors.As(err, &api) &&
		strings.Contains(api.Errors["resolution"], "appropriate screen") {
		return fmt.Errorf("this workspace's built-in workflow has no resolution field on the transition — run without --resolution (done sets the default resolution); workspaces created since 0.20.1 accept one (%w)", err)
	}
	return err
}

// listTransitions is `gadak transition KEY` with no target: print what
// pickTransition would accept, instead of a usage dump (GDK-466).
func listTransitions(key string, asJSON bool) error {
	return withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		list, err := c.Transitions(ctx, key)
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"key":         key,
				"transitions": jsonList(list),
				"categories":  jsonList(transition.ReachableCategories(list)),
				// duplicate_destinations names the groups a category token
				// folds (GDK-1356), so a shell-less agent can see why two rows
				// look identical without attempting a write to find out.
				"duplicate_destinations": jsonList(transition.DuplicateDestinations(list)),
			})
		}
		if len(list) == 0 {
			fmt.Fprintf(os.Stdout, "%s has no available transitions for this credential\n", key)
			return nil
		}
		fmt.Printf("available: %s\n", transition.JoinTransitions(list))
		if cats := transition.ReachableCategories(list); len(cats) > 0 {
			fmt.Printf("also accepts a status category: %s\n", strings.Join(cats, ", "))
		}
		if dup := transition.FormatDuplicateDestinations(list); dup != "" {
			fmt.Printf("same destination name and category, so a category token folds them into one — say a transition id or a target status id to pick a particular one:\n%s\n", dup)
		}
		return nil
	})
}

func parseTransitionFieldFlags(raw []string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(raw))
	for _, item := range raw {
		key, val, ok := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("--field expects key=JSON, got %q", item)
		}
		var parsed any
		if err := json.Unmarshal([]byte(val), &parsed); err != nil {
			out[key] = val
		} else {
			out[key] = parsed
		}
	}
	return out, nil
}

const assignUsage = "usage: gadak assign <KEY> <email|name|accountId|-> [--json] [--dry-run] | --batch -"

var assignBatchFields = []string{"key", "assignee"}

func cmdAssign(args []string) error {
	fs := newFlagSet("assign")
	asJSON := fs.Bool("json", false, "emit JSON")
	dryRun := fs.Bool("dry-run", false, "print the assignee id this write would send and exit; nothing reaches the origin")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs key and assignee")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("assign", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if *batch != "" {
		if err := rejectBatchFlag(*batch, ""); err != nil {
			return err
		}
		if len(pos) != 0 {
			return usageError("assign", "usage: gadak assign: --batch and a key/assignee are mutually exclusive")
		}
		return runAssignBatch(*asJSON)
	}
	if len(pos) < 2 {
		return usageError("assign", assignUsage)
	}
	key, who := fields.CanonicalKey(pos[0]), strings.TrimSpace(strings.Join(pos[1:], " "))

	return foldDryRun(withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		if *dryRun {
			// The account resolution a real write pays is the plan's whole
			// content: the id, or null for `-` (unassign) — the request the
			// origin would receive (GDK-1446).
			id, err := resolveAccount(ctx, c, who, src)
			if err != nil {
				return err
			}
			var assignee any = id
			if id == "" {
				assignee = nil // `-`: resolveAccount's empty id is the unassign
			}
			req := map[string]any{"assignee": assignee}
			if err := emitDryRun("assign", req, key); err != nil {
				return err
			}
			return errDryRun
		}
		if err := assignTo(ctx, c, src, key, who); err != nil {
			return err
		}
		recordAgentWrite(ctx, db, key, "assign")
		return emitAfterWrite(ctx, cfg, db, src, key, *asJSON, nil)
	}))
}

func assignTo(ctx context.Context, c origin.Writer, src, key, who string) error {
	id, err := resolveAccount(ctx, c, who, src)
	if err != nil {
		return err
	}
	return c.SetAssignee(ctx, key, id)
}

func runAssignBatch(asJSON bool) error {
	return runWriteBatch("assign", asJSON, false, func(raw string) batchResult {
		obj, key, err := parseBatchLine(raw, assignBatchFields)
		if err != nil {
			return batchErr(key, false, err)
		}
		who, ok, err := jsonStringField(obj, "assignee")
		if err != nil {
			return batchErr(key, false, err)
		}
		if !ok {
			return batchErr(key, false, errors.New("JSON line needs \"assignee\" (`\"-\"` unassigns)"))
		}
		who = strings.TrimSpace(who)
		if who == "" {
			return batchErr(key, false, errors.New("JSON line needs \"assignee\" (`\"-\"` unassigns)"))
		}
		var wrote bool
		err = withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
			if err := assignTo(ctx, c, src, key, who); err != nil {
				return err
			}
			recordAgentWrite(ctx, db, key, "assign")
			wrote = true
			return syncer.RefreshIssue(ctx, cfg, db, key, src)
		})
		return batchAfterWrite(key, wrote, err)
	})
}

// exitClaimConflict is the refusal exit: another actor holds the issue in
// progress. Its own code, not 1, so an agent branches without parsing
// stderr — the holder's name is still on stderr for the human. 75 is
// EX_TEMPFAIL: refused for now, retry after the holder finishes or pass
// --take-over.
const exitClaimConflict = 75

// cmdClaim is `gadak claim KEY`: take an issue as yours — assignee plus the
// in-progress transition in one step (internal/claim). On built-in and
// paired origins that is one atomic call; on connected Cloud there is no
// such route, the two writes run as a fallback, and the caller is told so.
// A claim someone else holds is refused with exit 75 rather than silently
// replacing them.
func cmdClaim(args []string) error {
	fs := newFlagSet("claim")
	asJSON := fs.Bool("json", false, "emit JSON")
	dryRun := fs.Bool("dry-run", false, "print the assignee and in-progress transition this claim would send and exit; nothing reaches the origin")
	takeOver := fs.Bool("take-over", false, "claim even when another assignee holds the issue in progress (replaces them)")
	trans := fs.String("transition", "", "which in-progress transition to take when more than one lands there (id, name, or status id)")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("claim", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("claim", "usage: gadak claim <KEY> [--transition <id|name>] [--take-over] [--json]")
	}
	key := fields.CanonicalKey(pos[0])

	return foldDryRun(withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		o, ok := c.(claim.Origin)
		if !ok {
			return fmt.Errorf("claim has no counterpart on this issue's origin (%s) — it is a Jira-workflow verb: assignee plus the in-progress transition; `gadak transition` and `gadak assign` are the two halves", src)
		}
		if *dryRun {
			// The claim's two halves as a plan: the account id (Myself —
			// the read the real claim pays too) and the in-progress target.
			// A holder conflict is the origin's runtime refusal, the same
			// boundary transition's dry run has with required fields; the
			// real write still refuses (exit 75) rather than replacing.
			me, err := o.Myself(ctx)
			if err != nil {
				return err
			}
			target := *trans
			if target == "" {
				target = "inprogress"
			}
			req := map[string]any{"assignee": me.ID(), "transition": target}
			if *takeOver {
				req["take_over"] = true
			}
			if err := emitDryRun("claim", req, key); err != nil {
				return err
			}
			return errDryRun
		}
		res, err := claim.Apply(ctx, o, cfg, claim.Request{
			Key: key, TransitionID: *trans, TakeOver: *takeOver,
			// Same mirror tiebreak the transition write below passes
			// (GDK-1521): a bare claim on a folded group lands on the
			// destination the project actually uses.
			StatusUse: transition.MirrorStatusUse(ctx, db, key),
		})
		if err != nil {
			var taken *claim.TakenError
			if errors.As(err, &taken) {
				return &exitCodeError{code: exitClaimConflict, msg: taken.Error()}
			}
			// GDK-1174: a board with two in-progress statuses fails every
			// bare claim — the candidates are already in the error; the
			// flag that picks one is this command's, so it is named here.
			var amb *transition.AmbiguousTransitionError
			if errors.As(err, &amb) && *trans == "" {
				return fmt.Errorf("%w\npass --transition <id|name> to choose one", err)
			}
			return err
		}
		if !res.Atomic {
			fmt.Fprintf(os.Stderr, "warning: this origin has no atomic claim — assignee and in-progress transition were two calls, so a concurrent claim could interleave\n")
		}
		recordAgentWrite(ctx, db, key, "claim")
		// GDK-1158: inside a gadak pane, reflect the landed claim into the
		// session this command ran in, so the serve can say which issue a
		// terminal is for. Best-effort by contract — the origin write above
		// is the claim, and a missing serve or an unknown session must not
		// turn it into an error.
		extra := map[string]any{"claim": res}
		bound := ""
		if session := os.Getenv("GADAK_TERMINAL_SESSION"); session != "" {
			bound = bindClaimToTerminalSession(session, key)
		}
		if bound != "" {
			extra["session"] = bound
		}
		if err := emitAfterWrite(ctx, cfg, db, src, key, *asJSON, extra); err != nil {
			return err
		}
		if bound != "" && !*asJSON {
			short := bound
			if len(short) > 8 {
				short = short[:8]
			}
			fmt.Printf("bound to session %s…\n", short)
		}
		return nil
	}))
}

// claimBindTimeout bounds the loopback POST that reflects a claim into the
// session it ran in. A human is waiting on the claim, not on the binding.
const claimBindTimeout = time.Second

// bindClaimToTerminalSession reflects a just-landed claim into the terminal
// session this command ran in (GDK-1158). Manager.Create sets
// GADAK_TERMINAL_SESSION in every pane shell, so a claim typed anywhere
// else never gets here (and the caller's empty check never fires). The
// serve is found the way `views open` finds it — discoverServes, the
// serveaddr walk plus identity probe (views.go) — restricted to this CLI
// profile's own serve, because a session id is valid nowhere else; a
// session the serve does not know is an old id from a restarted serve, and
// trying further serves would only re-learn that. Every failure answers ""
// so the claim stands.
func bindClaimToTerminalSession(session, key string) string {
	if session == "" {
		return ""
	}
	want := config.Profile()
	for _, hit := range discoverServes() {
		if !workspace.ProfileEq(hit.profile, want) {
			continue
		}
		if postTerminalIssueBinding(hit.base, session, key) {
			return session
		}
		return ""
	}
	return ""
}

// postTerminalIssueBinding is the one loopback call: the terminal surface's
// issue-binding route (internal/server, GDK-1158). False covers every
// failure shape — no dial, wrong status, timeout — because the caller
// treats them all the same: the claim stands, the binding is just not
// reflected.
func postTerminalIssueBinding(base, session, key string) bool {
	payload, err := json.Marshal(map[string]string{"issue_key": key})
	if err != nil {
		return false
	}
	url := strings.TrimRight(base, "/") + server.TermBase + "sessions/" + session + "/issue/"
	ctx, cancel := context.WithTimeout(context.Background(), claimBindTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	// Bounded like the identity probe (internal/origin/advertise.go): the
	// client carries the timeout too, so a hang that ignores ctx cannot
	// outlive it.
	client := &http.Client{Timeout: claimBindTimeout}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// resolveAccount turns an email, display name, or account id into an origin
// account id: `-` unassigns. Jira rows may use the configured member
// directory (JiraAccountID) without a network call — email first, then exact
// account id. Linear rows must not — that id is a Jira account, and Linear
// assign wants a Linear user UUID from Writer.SearchUsers. Account id
// comparison is case-sensitive; email is not.
func resolveAccount(ctx context.Context, c origin.Writer, who, source string) (string, error) {
	if who == "-" {
		return "", nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if source != "linear" {
		for _, m := range cfg.Members {
			if strings.EqualFold(m.Email, who) && m.JiraAccountID != "" {
				return m.JiraAccountID, nil
			}
		}
		for _, m := range cfg.Members {
			if m.JiraAccountID != "" && m.JiraAccountID == who {
				return m.JiraAccountID, nil
			}
		}
	}
	users, err := c.SearchUsers(ctx, who)
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if strings.EqualFold(u.Email, who) {
			return u.ID(), nil
		}
	}
	for _, u := range users {
		if u.ID() == who {
			return u.ID(), nil
		}
	}
	// A site that hides emails answers with no email to match on, so a single hit
	// is taken at its word and an ambiguous one is refused rather than guessed.
	if len(users) == 1 {
		return users[0].ID(), nil
	}
	if len(users) == 0 {
		// Linear SearchUsers is name/email contains; a UUID from the
		// mirror (issues.assignee_id) used to miss, and this hint told
		// the user to do the thing that just failed. Accept
		// the UUID as the id so the hint matches the behavior.
		if source == "linear" && linear.LooksLikeID(who) {
			return who, nil
		}
		return "", fmt.Errorf("no user on this issue's origin matches %q — look up issues.assignee_id in the mirror or `gadak issue KEY --editmeta`", who)
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, fmt.Sprintf("%s <%s>", u.DisplayName, u.Email))
	}
	return "", fmt.Errorf("%q matches %d users — be more specific: %s", who, len(users), strings.Join(names, "; "))
}

// cmdOpen jumps from a key in the terminal to the issue's page on its origin
// — Jira's /browse/KEY on a Jira workspace, the page Linear minted (stored
// in items.url) on a Linear one, the live serve on the built-in tracker.
// One branch per origin type, no fallback across them (GDK-1308). gadak is
// the fast path for reading; this is the escape hatch for everything the
// mirror deliberately does not do (boards, admin, workflow). The web's
// issueOriginUrl resolves the same way (GDK-1149).
