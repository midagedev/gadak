package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/jira"
)

const editUsage = "usage: gadak edit <KEY> [--summary S] [-m <text|->] [--label +x|-x]... [--priority NAME-or-id] [--due YYYY-MM-DD|none] [--parent KEY|none] [--json]"

func cmdEdit(args []string) error {
	fs := newFlagSet("edit")
	summary := fs.String("summary", "", "replace the summary")
	text := fs.String("m", "", "replace the description as plain text; `-` reads stdin; empty clears")
	var labels labelFlags
	fs.Var(&labels, "label", "`+name` or `-name` (repeatable)")
	priority := fs.String("priority", "", "priority name or id")
	due := fs.String("due", "", "due date (YYYY-MM-DD); `none` clears")
	parent := fs.String("parent", "", "parent issue key; `none` clears")
	asJSON := fs.Bool("json", false, "emit JSON")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("edit", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageError("edit", editUsage)
	}
	key := normalizeKey(pos[0])

	var hasSummary, hasM, hasLabel, hasPriority, hasParent, hasDue bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "summary":
			hasSummary = true
		case "m":
			hasM = true
		case "label":
			hasLabel = true
		case "priority":
			hasPriority = true
		case "parent":
			hasParent = true
		case "due":
			hasDue = true
		}
	})
	if !hasSummary && !hasM && !hasLabel && !hasPriority && !hasParent && !hasDue {
		return usageError("edit", editUsage)
	}
	if hasSummary && strings.TrimSpace(*summary) == "" {
		return usageError("edit", editUsage)
	}

	var dueDate string
	var clearDue bool
	if hasDue {
		if strings.TrimSpace(*due) == "none" {
			clearDue = true
		} else {
			var derr error
			dueDate, derr = parseDueDate(*due, "edit")
			if derr != nil {
				return derr
			}
			if dueDate == "" {
				return usageError("edit", editUsage)
			}
		}
	}

	var parentKey string
	var clearParent bool
	if hasParent {
		if strings.TrimSpace(*parent) == "none" {
			clearParent = true
		} else {
			var perr error
			parentKey, perr = parseParentKey(*parent, "edit")
			if perr != nil {
				return perr
			}
			if parentKey == "" {
				return usageError("edit", editUsage)
			}
		}
	}

	body := *text
	if hasM && body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}

	var labelOps []any
	if hasLabel {
		labelOps, err = labelUpdateOps(labels)
		if err != nil {
			return err
		}
	}

	return mutate(key, *asJSON, func(ctx context.Context, c *jira.Client) (map[string]any, error) {
		fields := map[string]any{}
		update := map[string]any{}
		if hasSummary {
			fields["summary"] = strings.TrimSpace(*summary)
		}
		if hasM {
			if strings.TrimSpace(body) == "" {
				fields["description"] = nil
			} else {
				fields["description"] = jira.Doc(body, nil)
			}
		}
		if hasLabel {
			update["labels"] = labelOps
		}
		if hasPriority {
			list, err := c.PriorityCatalog(ctx)
			if err != nil {
				return nil, err
			}
			id, err := create.Priority(*priority, list)
			if err != nil {
				return nil, formatCreateError(err)
			}
			fields["priority"] = map[string]string{"id": id}
		}
		if hasParent {
			if clearParent {
				fields["parent"] = nil
			} else {
				fields["parent"] = map[string]string{"key": parentKey}
			}
		}
		if hasDue {
			if clearDue {
				fields["duedate"] = nil
			} else {
				fields["duedate"] = dueDate
			}
		}
		return nil, c.EditIssue(ctx, key, fields, update)
	})
}

// labelUpdateOps turns --label +x / --label -y into Jira update verbs.
// A value that does not start with + or - is refused so we never guess
// add-vs-replace and wipe the existing set.
func labelUpdateOps(labels []string) ([]any, error) {
	ops := make([]any, 0, len(labels))
	for _, raw := range labels {
		if raw == "" || (raw[0] != '+' && raw[0] != '-') {
			return nil, fmt.Errorf("--label needs +name or -name (add or remove); got %q", raw)
		}
		name := strings.TrimSpace(raw[1:])
		if name == "" {
			return nil, fmt.Errorf("--label needs +name or -name (add or remove); got %q", raw)
		}
		if raw[0] == '+' {
			ops = append(ops, map[string]string{"add": name})
		} else {
			ops = append(ops, map[string]string{"remove": name})
		}
	}
	return ops, nil
}
