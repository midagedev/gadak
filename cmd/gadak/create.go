package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/store"
)

const createUsage = "usage: gadak create <SUMMARY> [--project KEY] [--type NAME-or-id] [--label L]... [-m <text|->] [--json]"

// labelFlags collects repeated --label values.
type labelFlags []string

func (l *labelFlags) String() string { return strings.Join(*l, ", ") }

func (l *labelFlags) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func cmdCreate(args []string) error {
	fs := newFlagSet("create")
	projectFlag := fs.String("project", "", "project key; omitted uses the sole configured project")
	typeFlag := fs.String("type", "", "issue type name or id from createmeta")
	var labels labelFlags
	fs.Var(&labels, "label", "label (repeatable)")
	text := fs.String("m", "", "description as plain text; `-` reads it from stdin")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("create", fs))
		return nil
	}
	// Flags may sit before, after, or between summary words. parseAround keeps
	// an unquoted multi-word summary intact the way cmdTransition joins a
	// status name, and treats an unknown leading dash as summary text.
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	summary := strings.TrimSpace(strings.Join(pos, " "))
	if summary == "" {
		return usageError("create", createUsage)
	}

	body := *text
	if body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}

	return withWriteSession(func(ctx context.Context, cfg *config.Config, db *store.DB, c *jira.Client) error {
		project, err := resolveCreateProject(*projectFlag, cfg.Projects)
		if err != nil {
			return err
		}
		meta, err := c.CreateMeta(ctx, []string{project})
		if err != nil {
			return err
		}
		proj, types, err := createMetaFor(meta, project)
		if err != nil {
			return err
		}
		typeID, err := resolveCreateType(*typeFlag, types)
		if err != nil {
			return err
		}

		fields := map[string]any{
			"project":   map[string]string{"key": proj.Key},
			"issuetype": map[string]string{"id": typeID},
			"summary":   summary,
		}
		if strings.TrimSpace(body) != "" {
			fields["description"] = jira.Doc(body, nil)
		}
		if len(labels) > 0 {
			fields["labels"] = []string(labels)
		}

		key, err := c.CreateIssue(ctx, fields)
		if err != nil {
			return err
		}
		extra := map[string]any{"created": map[string]string{"key": key}}
		err = emitAfterWrite(ctx, cfg, db, c, key, *asJSON, extra)
		var missed writeNotMirroredError
		if errors.As(err, &missed) {
			// The write landed; the new key is outside what this mirror lists.
			// Print it and exit 0 — retrying would create a second issue.
			fmt.Fprintf(os.Stderr, "warning: %s\n", missed.Error())
			if *asJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"created": map[string]string{"key": key},
				})
			}
			fmt.Println(key)
			return nil
		}
		return err
	})
}

// resolveCreateProject uses --project, or the sole configured project key.
// Zero or two-or-more configured keys without --project is an error that
// lists what is configured.
func resolveCreateProject(flag string, configured []string) (string, error) {
	if p := strings.TrimSpace(flag); p != "" {
		return p, nil
	}
	if len(configured) == 1 && strings.TrimSpace(configured[0]) != "" {
		return configured[0], nil
	}
	if len(configured) == 0 {
		return "", errors.New("pass --project")
	}
	return "", fmt.Errorf("pass --project, configured: %s", strings.Join(configured, ", "))
}

func createMetaFor(meta []jira.CreateMetaProject, project string) (jira.CreateMetaProject, []jira.NamedID, error) {
	for _, p := range meta {
		if strings.EqualFold(p.Key, project) {
			return p, p.IssueTypes, nil
		}
	}
	return jira.CreateMetaProject{}, nil, fmt.Errorf("this credential cannot create issues in %s", project)
}

func resolveCreateType(want string, types []jira.NamedID) (string, error) {
	want = strings.TrimSpace(want)
	if want == "" {
		return "", fmt.Errorf("pass --type, available: %s", formatCreateTypes(types))
	}
	for _, t := range types {
		if t.ID == want || strings.EqualFold(t.Name, want) {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no issue type matching %q — available: %s", want, formatCreateTypes(types))
}

func formatCreateTypes(types []jira.NamedID) string {
	parts := make([]string, 0, len(types))
	for _, t := range types {
		parts = append(parts, fmt.Sprintf("%s (id %s)", t.Name, t.ID))
	}
	return strings.Join(parts, "; ")
}
