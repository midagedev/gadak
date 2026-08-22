package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
)

const createUsage = "usage: gadak create [--] <SUMMARY> | --batch - [--project KEY] [--type NAME-or-id] [--priority NAME-or-id] [--due YYYY-MM-DD] [--parent KEY] [--label L]... [--attach FILE]... [-m <text|->] [--field alias=value]... [--json]"

// createBatchShape is the one-line reminder printed when a --batch line is
// not an object we can file. Field names match createBatchLine.
const createBatchShape = `{"summary": "...", "type"?: "...", "project"?: "...", "labels"?: [...], "description"?: "plain text", "attach"?: ["path", ...], "priority"?: "...", "parent"?: "ABC-1", "due"?: "YYYY-MM-DD", "fields"?: {"alias": value}}`

// labelFlags collects repeated --label values.
type labelFlags []string

func (l *labelFlags) String() string { return strings.Join(*l, ", ") }

func (l *labelFlags) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func cmdCreate(args []string) error {
	fs := newFlagSet("create")
	projectFlag := fs.String("project", "", "project key; omitted uses the configured default, else the sole configured project")
	typeFlag := fs.String("type", "", "issue type name or id from createmeta; omitted uses the configured default id, else the project's only type")
	priorityFlag := fs.String("priority", "", "priority name or id")
	dueFlag := fs.String("due", "", "due date (YYYY-MM-DD)")
	parentFlag := fs.String("parent", "", "parent issue key")
	var labels labelFlags
	fs.Var(&labels, "label", "label (repeatable)")
	var attachFiles labelFlags
	fs.Var(&attachFiles, "attach", "file to upload after create (repeatable)")
	text := fs.String("m", "", "description as plain text; `-` reads it from stdin")
	var fieldFlags labelFlags
	fs.Var(&fieldFlags, "field", fieldFlagUsage)
	asJSON := fs.Bool("json", false, "emit JSON")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs summary, and may set type, project, labels, description, attach, priority, parent, due, fields")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("create", fs))
		return nil
	}
	// Flags may sit before, after, or between summary words. parseAround keeps
	// an unquoted multi-word summary intact the way cmdTransition joins a
	// status name. An unknown dash-token is rejected; a summary that starts
	// with `-` goes after `--`.
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}

	if *batch != "" {
		if *batch != "-" {
			return fmt.Errorf("--batch only accepts - (JSON lines on stdin)")
		}
		if *text == "-" {
			return fmt.Errorf("--batch - already reads stdin; -m - cannot be used together")
		}
		if strings.TrimSpace(strings.Join(pos, " ")) != "" {
			return usageError("create", "usage: gadak create: --batch and a summary are mutually exclusive")
		}
		fieldRaws, ferr := parseAliasFieldRaws(fieldFlags)
		if ferr != nil {
			return ferr
		}
		if err := refuseSignedCreateLabels(labels); err != nil {
			return err
		}
		return cmdCreateBatch(*projectFlag, *typeFlag, *text, *priorityFlag, *parentFlag, *dueFlag, []string(labels), []string(attachFiles), fieldRaws, *asJSON)
	}

	summary := strings.TrimSpace(strings.Join(pos, " "))
	if summary == "" {
		return usageError("create", createUsage)
	}
	if err := refuseSignedCreateLabels(labels); err != nil {
		return err
	}
	if _, err := parseParentKey(*parentFlag, "create"); err != nil {
		return err
	}
	if _, err := parseDueDate(*dueFlag, "create"); err != nil {
		return err
	}

	body := *text
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	if len(attachFiles) > 0 {
		if err := validateAttachPaths(attachFiles); err != nil {
			return err
		}
	}
	fieldRaws, err := parseAliasFieldRaws(fieldFlags)
	if err != nil {
		return err
	}

	return withCreateSession(*projectFlag, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		key, extra, err := createOn(ctx, cfg, c, src, *projectFlag, *typeFlag, summary, body, *priorityFlag, *parentFlag, *dueFlag, []string(labels), attachFiles, fieldRaws)
		if err != nil {
			return err
		}
		err = emitAfterWrite(ctx, cfg, db, src, key, *asJSON, extra)
		var missed writeNotMirroredError
		if errors.As(err, &missed) {
			// The write landed; the new key is outside what this mirror lists.
			// Print it and exit 0 — retrying would create a second issue.
			fmt.Fprintf(os.Stderr, "warning: %s\n", missed.Error())
			if *asJSON {
				body := map[string]any{"created": map[string]string{"key": key}}
				if att, ok := extra["attached"]; ok {
					body["attached"] = att
				}
				if res, ok := extra["resolved"]; ok {
					body["resolved"] = res
				}
				return json.NewEncoder(os.Stdout).Encode(body)
			}
			fmt.Println(key)
			return nil
		}
		return err
	})
}

// createBatchLine is one stdin object for --batch -. Absent optional fields
// fall back to the matching flag; a present empty labels/attach array is an
// override (no labels / no files), not a fall-through. Empty priority and
// parent fall through the same way type and project do.
type createBatchLine struct {
	Summary     string                     `json:"summary"`
	Type        string                     `json:"type"`
	Project     string                     `json:"project"`
	Labels      *[]string                  `json:"labels"`
	Description string                     `json:"description"`
	Attach      *[]string                  `json:"attach"`
	Priority    string                     `json:"priority"`
	Parent      string                     `json:"parent"`
	Due         string                     `json:"due"`
	Fields      map[string]json.RawMessage `json:"fields"`
}

func cmdCreateBatch(projectFlag, typeFlag, defaultBody, defaultPriority, defaultParent, defaultDue string, defaultLabels, defaultAttach []string, defaultFields map[string]json.RawMessage, asJSON bool) error {
	return withCreateSession(projectFlag, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
		sc := bufio.NewScanner(os.Stdin)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			raw := strings.TrimSpace(sc.Text())
			if raw == "" {
				continue
			}
			var rec createBatchLine
			if err := json.Unmarshal([]byte(raw), &rec); err != nil {
				return fmt.Errorf("line %d: invalid JSON: %v\nexpected %s", lineNo, err, createBatchShape)
			}
			summary := strings.TrimSpace(rec.Summary)
			if summary == "" {
				return fmt.Errorf("line %d: missing summary\nexpected %s", lineNo, createBatchShape)
			}
			projectWant := rec.Project
			if strings.TrimSpace(projectWant) == "" {
				projectWant = projectFlag
			}
			typeWant := rec.Type
			if strings.TrimSpace(typeWant) == "" {
				typeWant = typeFlag
			}
			labels := defaultLabels
			if rec.Labels != nil {
				labels = *rec.Labels
			}
			body := rec.Description
			if strings.TrimSpace(body) == "" {
				body = defaultBody
			}
			attach := defaultAttach
			if rec.Attach != nil {
				attach = *rec.Attach
			}
			priorityWant := rec.Priority
			if strings.TrimSpace(priorityWant) == "" {
				priorityWant = defaultPriority
			}
			parentWant := rec.Parent
			if strings.TrimSpace(parentWant) == "" {
				parentWant = defaultParent
			}
			dueWant := rec.Due
			if strings.TrimSpace(dueWant) == "" {
				dueWant = defaultDue
			}
			fieldRaws := mergeFieldRaws(defaultFields, rec.Fields)
			w, lineSrc := c, src
			if want := strings.TrimSpace(projectWant); want != "" {
				if routed, rerr := resolveCreateSource(ctx, cfg, db, want); rerr != nil {
					return fmt.Errorf("line %d: %w", lineNo, rerr)
				} else if routed != src {
					nw, werr := origin.WriterFor(cfg, routed)
					if werr != nil {
						return fmt.Errorf("line %d: %w", lineNo, werr)
					}
					w, lineSrc = nw, routed
				}
			}
			key, extra, err := createOn(ctx, cfg, w, lineSrc, projectWant, typeWant, summary, body, priorityWant, parentWant, dueWant, labels, attach, fieldRaws)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			if err := emitBatchLine(ctx, cfg, db, lineSrc, key, summary, asJSON, extra); err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
		}
		if err := sc.Err(); err != nil {
			return fmt.Errorf("reading --batch stdin: %w", err)
		}
		return nil
	})
}

// createOne resolves project/type/priority/parent, POSTs the issue, and uploads
// attachments. Attach paths and the parent key shape are validated before any
// Jira call. The caller prints / refreshes.
func refuseSignedCreateLabels(labels []string) error {
	for _, l := range labels {
		if l != "" && (l[0] == '+' || l[0] == '-') {
			return fmt.Errorf("create takes plain label names; +/- add/remove syntax belongs to gadak edit")
		}
	}
	return nil
}

func createOn(ctx context.Context, cfg *config.Config, c origin.Writer, src, projectWant, typeWant, summary, body, priorityWant, parentWant, dueWant string, labels, attach []string, fieldRaws map[string]json.RawMessage) (string, map[string]any, error) {
	if src == "linear" {
		return createLinearOne(ctx, cfg, c, projectWant, typeWant, summary, body, priorityWant, parentWant, dueWant, labels, attach, fieldRaws)
	}
	jc, ok := c.(*jira.Client)
	if !ok {
		return "", nil, fmt.Errorf("create: origin writer is not a Jira client")
	}
	return createOne(ctx, cfg, jc, projectWant, typeWant, summary, body, priorityWant, parentWant, dueWant, labels, attach, fieldRaws)
}

func createLinearOne(ctx context.Context, cfg *config.Config, c origin.Writer, projectWant, typeWant, summary, body, priorityWant, parentWant, dueWant string, labels, attach []string, fieldRaws map[string]json.RawMessage) (string, map[string]any, error) {
	if err := refuseSignedCreateLabels(labels); err != nil {
		return "", nil, err
	}
	if len(labels) > 0 {
		return "", nil, fmt.Errorf("linear: field %q is not supported on create", "labels")
	}
	if len(fieldRaws) > 0 {
		return "", nil, fmt.Errorf("linear: custom fields are not supported on create")
	}
	if len(attach) > 0 {
		if err := validateAttachPaths(attach); err != nil {
			return "", nil, err
		}
	}
	parentKey, err := parseParentKey(parentWant, "create")
	if err != nil {
		return "", nil, err
	}
	if parentKey != "" {
		return "", nil, fmt.Errorf("linear: field %q is not supported on create", "parent")
	}
	due, err := parseDueDate(dueWant, "create")
	if err != nil {
		return "", nil, err
	}
	projRes, err := create.Project(projectWant, cfg)
	if err != nil {
		catalog, cerr := c.CreateMeta(ctx, createMetaScope(cfg))
		if cerr == nil {
			err = create.FillNeedProject(err, catalog)
		}
		return "", nil, formatCreateError(err)
	}
	meta, err := c.CreateMeta(ctx, []string{projRes.Value})
	if err != nil {
		return "", nil, err
	}
	proj, types, err := create.MetaFor(meta, projRes.Value, cfg)
	if err != nil && create.FormatProjectKeys(meta) == "" {
		if catalog, cerr := c.CreateMeta(ctx, nil); cerr == nil && len(catalog) > 0 {
			proj, types, err = create.MetaFor(catalog, projRes.Value, cfg)
		}
	}
	if err != nil {
		return "", nil, err
	}
	typeRes := create.Resolved{Value: "issue", Source: create.SourceSole}
	if strings.TrimSpace(typeWant) != "" {
		typeRes, err = create.Type(typeWant, types, cfg, projRes.Value)
		if err != nil {
			return "", nil, formatCreateError(err)
		}
	}
	fields := map[string]any{
		"project": map[string]any{"key": proj.Key},
		"summary": summary,
	}
	if strings.TrimSpace(body) != "" {
		fields["description"] = jira.Doc(body, nil)
	}
	if p := strings.TrimSpace(priorityWant); p != "" {
		list, err := c.PriorityCatalog(ctx)
		if err != nil {
			return "", nil, err
		}
		id, err := create.Priority(p, list)
		if err != nil {
			return "", nil, formatCreateError(err)
		}
		fields["priority"] = create.PriorityField(id)
	}
	if due != "" {
		fields["duedate"] = due
	}
	key, err := c.CreateIssue(ctx, fields)
	if err != nil {
		return "", nil, err
	}
	extra := map[string]any{
		"created": map[string]string{"key": key},
		"resolved": map[string]any{
			"project":    projRes,
			"issue_type": typeRes,
		},
	}
	if len(attach) > 0 {
		attached, err := uploadAttachPaths(ctx, c, key, attach)
		if err != nil {
			var p *attachPartialError
			if errors.As(err, &p) {
				return key, extra, fmt.Errorf("created %s, but attaching %s failed: %w", key, p.failed, p.err)
			}
			return key, extra, fmt.Errorf("created %s, but attaching failed: %w", key, err)
		}
		extra["attached"] = attached
	}
	return key, extra, nil
}

func createOne(ctx context.Context, cfg *config.Config, c *jira.Client, projectWant, typeWant, summary, body, priorityWant, parentWant, dueWant string, labels, attach []string, fieldRaws map[string]json.RawMessage) (string, map[string]any, error) {
	if err := refuseSignedCreateLabels(labels); err != nil {
		return "", nil, err
	}
	if len(attach) > 0 {
		if err := validateAttachPaths(attach); err != nil {
			return "", nil, err
		}
	}
	parentKey, err := parseParentKey(parentWant, "create")
	if err != nil {
		return "", nil, err
	}
	due, err := parseDueDate(dueWant, "create")
	if err != nil {
		return "", nil, err
	}
	projRes, err := create.Project(projectWant, cfg)
	if err != nil {
		// NeedProjectError is local config ambiguity. Probe the origin so a
		// pairing/dial failure is not relabeled as a missing --project
		// (GDK-453). A reachable origin that still cannot resolve a project
		// keeps the flag sentence, and an empty configured list is filled
		// from createmeta so a paired workspace can print origin keys
		// (GDK-467) without copying home defaults into this profile.
		if _, _, perr := c.Projects(ctx, 1); perr != nil && origin.IsPairingFailure(perr) {
			return "", nil, perr
		}
		catalog, cerr := c.CreateMeta(ctx, createMetaScope(cfg))
		if cerr == nil {
			err = create.FillNeedProject(err, catalog)
		}
		return "", nil, formatCreateError(err)
	}
	meta, err := c.CreateMeta(ctx, []string{projRes.Value})
	if err != nil {
		return "", nil, err
	}
	proj, types, err := create.MetaForWithCatalog(ctx, c, meta, projRes.Value, cfg)
	if err != nil {
		return "", nil, err
	}
	typeRes, err := create.Type(typeWant, types, cfg, projRes.Value)
	if err != nil {
		return "", nil, formatCreateError(err)
	}

	fields := map[string]any{
		"project":   map[string]string{"key": proj.Key},
		"issuetype": map[string]string{"id": typeRes.Value},
		"summary":   summary,
	}
	if strings.TrimSpace(body) != "" {
		fields["description"] = jira.Doc(body, nil)
	}
	if len(labels) > 0 {
		fields["labels"] = labels
	}
	if p := strings.TrimSpace(priorityWant); p != "" {
		list, err := c.PriorityCatalog(ctx)
		if err != nil {
			return "", nil, err
		}
		id, err := create.Priority(p, list)
		if err != nil {
			return "", nil, formatCreateError(err)
		}
		fields["priority"] = create.PriorityField(id)
	}
	if parentKey != "" {
		fields["parent"] = map[string]string{"key": parentKey}
	}
	if due != "" {
		fields["duedate"] = due
	}
	if len(fieldRaws) > 0 {
		if err := checkConfiguredAliases(cfg, fieldRaws); err != nil {
			return "", nil, err
		}
		list, err := c.CreateFields(ctx, proj.Key, typeRes.Value)
		if err != nil {
			return "", nil, err
		}
		custom, err := resolveCreateAliasFields(ctx, c, "", cfg, list, fieldRaws)
		if err != nil {
			return "", nil, err
		}
		for id, v := range custom {
			fields[id] = v
		}
	}

	key, err := c.CreateIssue(ctx, fields)
	if err != nil {
		return "", nil, withParentHint(ctx, err, parentKey)
	}
	extra := map[string]any{
		"created": map[string]string{"key": key},
		"resolved": map[string]any{
			"project":    projRes,
			"issue_type": typeRes,
		},
	}
	if len(attach) > 0 {
		attached, err := uploadAttachPaths(ctx, c, key, attach)
		if err != nil {
			var p *attachPartialError
			if errors.As(err, &p) {
				return key, extra, fmt.Errorf("created %s, but attaching %s failed: %w", key, p.failed, p.err)
			}
			return key, extra, fmt.Errorf("created %s, but attaching failed: %w", key, err)
		}
		extra["attached"] = attached
	}
	return key, extra, nil
}

func mergeFieldRaws(base, overlay map[string]json.RawMessage) map[string]json.RawMessage {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

// formatCreateError turns shared Need* catalogue data into the CLI flag +
// catalog sentences. The shared package must not compose those flags itself.
func formatCreateError(err error) error {
	var np *create.NeedProjectError
	if errors.As(err, &np) {
		if len(np.Configured) == 0 {
			return fmt.Errorf("pass --project")
		}
		return fmt.Errorf("pass --project, configured: %s", strings.Join(np.Configured, ", "))
	}
	var nt *create.NeedTypeError
	if errors.As(err, &nt) {
		return fmt.Errorf("pass --type, available: %s", create.FormatTypes(nt.Available))
	}
	var npri *create.NeedPriorityError
	if errors.As(err, &npri) {
		return fmt.Errorf("pass --priority, available: %s", create.FormatTypes(npri.Available))
	}
	return err
}

// createMetaScope is the createmeta projectKeys filter: the profile list
// when set, otherwise empty so the origin returns every createable project
// (paired workspaces have no local Projects copy — GDK-467).
func createMetaScope(cfg *config.Config) []string {
	if cfg == nil || len(cfg.Projects) == 0 {
		return nil
	}
	return cfg.Projects
}

// emitBatchLine refreshes the new key the same way emitAfterWrite does.
// Text is KEY<tab>summary (batch contract); --json reuses emitAfterWrite.
func emitBatchLine(ctx context.Context, cfg *config.Config, db *store.DB, src, key, summary string, asJSON bool, extra map[string]any) error {
	if asJSON {
		err := emitAfterWrite(ctx, cfg, db, src, key, true, extra)
		var missed writeNotMirroredError
		if errors.As(err, &missed) {
			fmt.Fprintf(os.Stderr, "warning: %s\n", missed.Error())
			body := map[string]any{"created": map[string]string{"key": key}}
			if att, ok := extra["attached"]; ok {
				body["attached"] = att
			}
			if res, ok := extra["resolved"]; ok {
				body["resolved"] = res
			}
			return json.NewEncoder(os.Stdout).Encode(body)
		}
		return err
	}
	if err := refreshAfterWrite(ctx, cfg, db, src, key); err != nil {
		return fmt.Errorf("write applied to %s, but the mirror did not refresh (run `gadak sync`): %w", key, err)
	}
	lites, err := lookup(db, []string{key})
	if err != nil {
		return err
	}
	if len(lites) == 0 {
		fmt.Fprintf(os.Stderr, "warning: %s\n", writeNotMirroredError{Key: key}.Error())
		fmt.Printf("%s\t%s\n", key, summary)
		return nil
	}
	fmt.Printf("%s\t%s\n", lites[0].IssueKey, lites[0].Summary)
	return nil
}

// parseParentKey accepts a Jira issue key (ABC-123). Empty means omitted.
// The wording matches gadak_show issue (internal/mcp/tools.go).
// parseDueDate accepts a calendar date YYYY-MM-DD. Empty means omitted.
// Callers that accept the clear sentinel (`none`) must branch before this.
func parseDueDate(raw, cmd string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if !fields.DateOnlyLiteral(s) {
		return "", fmt.Errorf("gadak %s --due %q is not a date (want YYYY-MM-DD)", cmd, raw)
	}
	return s, nil
}

func parseParentKey(raw, cmd string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if !looksLikeIssueKey(raw) {
		return "", fmt.Errorf("gadak %s --parent %q is not a Jira key (want ABC-123)", cmd, raw)
	}
	return normalizeKey(raw), nil
}

// parentRejection reports whether err is the origin refusing the parent we
// sent. The field key differs by verb, and both shapes were measured against
// a real Cloud site on 2026-08-21: POST /issue answers with `parent` AND
// `parentId`, PUT /issue/{key} answers with `pid`. The messages themselves
// are localized per account, so the keys are the only stable part — this is
// the one place that knows them. GDK-424 tested `parent` inline in the
// create path, which could never have matched the edit path (GDK-525).
func parentRejection(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// "pid:" keeps the colon so ordinary words containing "pid" (rapid,
	// insipid) in an unrelated message cannot claim to be this rejection.
	return strings.Contains(s, "parent") || strings.Contains(s, "pid:")
}

// withParentHint appends the mirror's hierarchy answer to a parent rejection
// and returns every other error untouched. Both write verbs that can send a
// parent go through here, so a new surface cannot inherit the bare 400.
func withParentHint(ctx context.Context, err error, parentKey string) error {
	if err == nil || parentKey == "" || !parentRejection(err) {
		return err
	}
	if hint := parentHierarchyHint(ctx, parentKey); hint != "" {
		return fmt.Errorf("%w\n%s", err, hint)
	}
	return err
}

// parentHierarchyHint reads the rejected parent from the mirror and states
// the hierarchy rule Jira's 400 leaves out. Best-effort: no mirror, no row,
// or any error returns "" and the origin error stands alone.
func parentHierarchyHint(_ context.Context, parentKey string) string {
	db, err := openReadOnly()
	if err != nil {
		return ""
	}
	defer db.Close()
	var issueType, projectKey string
	var level int
	err = db.QueryRow(
		`SELECT COALESCE(issue_type, ''), COALESCE(hierarchy_level, 0), COALESCE(project_key, '') FROM issues WHERE key = ?`,
		parentKey).Scan(&issueType, &level, &projectKey)
	if err != nil {
		return ""
	}
	// A parent sits exactly one level above its child, so a level-1 parent
	// (an epic) is refused for another epic — the case the old early return
	// answered with silence (GDK-525).
	if level >= 1 {
		below := fmt.Sprintf("level-%d", level-1)
		if level == 1 {
			below = "level-0 (standard types such as Task, Bug or Story)"
		}
		return fmt.Sprintf("hint: %s is %q (hierarchy level %d) — a parent sits exactly one level above its child, so %s can only parent %s issues. Two issues at the same level cannot be parent and child.",
			parentKey, issueType, level, parentKey, below)
	}
	hint := fmt.Sprintf("hint: %s is %q (hierarchy level %d) — a standard issue can only sit under a level-1 parent (an epic); only sub-task types can sit under %s. Pick an epic as --parent, or use a sub-task issue type.",
		parentKey, issueType, level, parentKey)
	if extra := openEpicHint(db, projectKey); extra != "" {
		return hint + "\n" + extra
	}
	return hint
}

// openEpicHint names up to three open level-1 issues in the rejected parent's
// project. Empty project, query failure, or zero rows return "" so the base
// hint still stands. Filter is hierarchy_level + status_category, never a
// localized type name (GDK-330).
func openEpicHint(db *sql.DB, projectKey string) string {
	if projectKey == "" {
		return ""
	}
	rows, err := db.Query(
		`SELECT key, COALESCE(summary, '') FROM issues_full
		 WHERE hierarchy_level = 1 AND status_category != 'done' AND project_key = ?
		 ORDER BY updated_at DESC LIMIT 3`,
		projectKey)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var key, summary string
		if err := rows.Scan(&key, &summary); err != nil {
			return ""
		}
		parts = append(parts, fmt.Sprintf("%s %q", key, clip(summary, 60)))
	}
	if err := rows.Err(); err != nil || len(parts) == 0 {
		return ""
	}
	return "open epics in " + projectKey + ": " + strings.Join(parts, ", ")
}
