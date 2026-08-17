package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
)

const createUsage = "usage: gadak create [--] <SUMMARY> | --batch - [--project KEY] [--type NAME-or-id] [--priority NAME-or-id] [--parent KEY] [--label L]... [--attach FILE]... [-m <text|->] [--json]"

// createBatchShape is the one-line reminder printed when a --batch line is
// not an object we can file. Field names match createBatchLine.
const createBatchShape = `{"summary": "...", "type"?: "...", "project"?: "...", "labels"?: [...], "description"?: "plain text", "attach"?: ["path", ...], "priority"?: "...", "parent"?: "ABC-1"}`

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
	parentFlag := fs.String("parent", "", "parent issue key")
	var labels labelFlags
	fs.Var(&labels, "label", "label (repeatable)")
	var attachFiles labelFlags
	fs.Var(&attachFiles, "attach", "file to upload after create (repeatable)")
	text := fs.String("m", "", "description as plain text; `-` reads it from stdin")
	asJSON := fs.Bool("json", false, "emit JSON")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs summary, and may set type, project, labels, description, attach, priority, parent")
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
		return cmdCreateBatch(*projectFlag, *typeFlag, *text, *priorityFlag, *parentFlag, []string(labels), []string(attachFiles), *asJSON)
	}

	summary := strings.TrimSpace(strings.Join(pos, " "))
	if summary == "" {
		return usageError("create", createUsage)
	}
	if _, err := parseParentKey(*parentFlag, "create"); err != nil {
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

	return withWriteSession(func(ctx context.Context, cfg *config.Config, db *store.DB, c *jira.Client) error {
		key, extra, err := createOne(ctx, cfg, c, *projectFlag, *typeFlag, summary, body, *priorityFlag, *parentFlag, []string(labels), attachFiles)
		if err != nil {
			return err
		}
		err = emitAfterWrite(ctx, cfg, db, c, key, *asJSON, extra)
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
	Summary     string    `json:"summary"`
	Type        string    `json:"type"`
	Project     string    `json:"project"`
	Labels      *[]string `json:"labels"`
	Description string    `json:"description"`
	Attach      *[]string `json:"attach"`
	Priority    string    `json:"priority"`
	Parent      string    `json:"parent"`
}

func cmdCreateBatch(projectFlag, typeFlag, defaultBody, defaultPriority, defaultParent string, defaultLabels, defaultAttach []string, asJSON bool) error {
	return withWriteSession(func(ctx context.Context, cfg *config.Config, db *store.DB, c *jira.Client) error {
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
			key, extra, err := createOne(ctx, cfg, c, projectWant, typeWant, summary, body, priorityWant, parentWant, labels, attach)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			if err := emitBatchLine(ctx, cfg, db, c, key, summary, asJSON, extra); err != nil {
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
func createOne(ctx context.Context, cfg *config.Config, c *jira.Client, projectWant, typeWant, summary, body, priorityWant, parentWant string, labels, attach []string) (string, map[string]any, error) {
	if len(attach) > 0 {
		if err := validateAttachPaths(attach); err != nil {
			return "", nil, err
		}
	}
	parentKey, err := parseParentKey(parentWant, "create")
	if err != nil {
		return "", nil, err
	}
	projRes, err := create.Project(projectWant, cfg)
	if err != nil {
		return "", nil, err
	}
	meta, err := c.CreateMeta(ctx, []string{projRes.Value})
	if err != nil {
		return "", nil, err
	}
	proj, types, err := create.MetaFor(meta, projRes.Value)
	if err != nil {
		return "", nil, err
	}
	typeRes, err := create.Type(typeWant, types, cfg, projRes.Value)
	if err != nil {
		return "", nil, err
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
		id, err := resolvePriority(p, list)
		if err != nil {
			return "", nil, err
		}
		fields["priority"] = map[string]string{"id": id}
	}
	if parentKey != "" {
		fields["parent"] = map[string]string{"key": parentKey}
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

// emitBatchLine refreshes the new key the same way emitAfterWrite does.
// Text is KEY<tab>summary (batch contract); --json reuses emitAfterWrite.
func emitBatchLine(ctx context.Context, cfg *config.Config, db *store.DB, c *jira.Client, key, summary string, asJSON bool, extra map[string]any) error {
	if asJSON {
		err := emitAfterWrite(ctx, cfg, db, c, key, true, extra)
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
	if err := syncer.SyncIssue(ctx, cfg, db, key, syncer.Options{Client: c}); err != nil {
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
func parseParentKey(raw, cmd string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if !looksLikeIssueKey(raw) {
		return "", fmt.Errorf("gadak %s --parent %q is not a Jira key (want ABC-123)", cmd, raw)
	}
	return normalizeKey(raw), nil
}

// formatCreateTypes is the Name (id N) list used by resolvePriority in edit.go
// (same package). Type resolution itself lives in internal/create.
func formatCreateTypes(types []jira.NamedID) string {
	return create.FormatTypes(types)
}
