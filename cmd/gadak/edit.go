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
	"github.com/midagedev/gadak/internal/origin"
)

const editUsage = "usage: gadak edit <KEY> [--summary S] [-m <text|->] [--label +x|-x]... [--component +x|-x]... [--priority NAME-or-id] [--due YYYY-MM-DD|none] [--parent KEY|none] [--json]"

func cmdEdit(args []string) error {
	fs := newFlagSet("edit")
	summary := fs.String("summary", "", "replace the summary")
	text := fs.String("m", "", "replace the description as plain text; `-` reads stdin; empty clears")
	var labels labelFlags
	fs.Var(&labels, "label", "`+name` or `-name` (repeatable)")
	var components labelFlags
	fs.Var(&components, "component", "`+name` or `-name` (repeatable)")
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

	var hasSummary, hasM, hasLabel, hasComponent, hasPriority, hasParent, hasDue bool
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "summary":
			hasSummary = true
		case "m":
			hasM = true
		case "label":
			hasLabel = true
		case "component":
			hasComponent = true
		case "priority":
			hasPriority = true
		case "parent":
			hasParent = true
		case "due":
			hasDue = true
		}
	})
	if !hasSummary && !hasM && !hasLabel && !hasComponent && !hasPriority && !hasParent && !hasDue {
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
	var componentOps []any
	if hasComponent {
		componentOps, err = componentUpdateOps(components)
		if err != nil {
			return err
		}
	}

	return mutate(key, *asJSON, func(ctx context.Context, c origin.Writer) (map[string]any, error) {
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
		if hasComponent {
			update["components"] = componentOps
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
		// A --parent the origin refuses gets the mirror's hierarchy answer,
		// the same one create gives. withParentHint owns the "is this a
		// parent rejection" test because the field key differs by verb
		// (create: parent/parentId, edit: pid — GDK-525).
		err := c.EditIssue(ctx, key, fields, update)
		err = withParentHint(ctx, err, parentKey)
		return nil, withComponentHint(ctx, c, key, err, hasComponent)
	})
}

// labelUpdateOps turns --label +x / --label -y into Jira update verbs.
// A value that does not start with + or - is refused so we never guess
// add-vs-replace and wipe the existing set.
func labelUpdateOps(labels []string) ([]any, error) {
	return signedUpdateOps("--label", labels, func(op, name string) any {
		return map[string]string{op: name}
	})
}

// componentUpdateOps is the components sibling of labelUpdateOps. Jira's
// update op value is {"name": X}, not a bare string.
func componentUpdateOps(values []string) ([]any, error) {
	return signedUpdateOps("--component", values, func(op, name string) any {
		return map[string]any{op: map[string]string{"name": name}}
	})
}

// signedUpdateOps is the shared +name / -name parser for edit's set-valued
// update flags. wrap builds one Jira update verb; the flag name is for the
// refusal so --label and --component never share an error that names the
// other flag.
func signedUpdateOps(flag string, values []string, wrap func(op, name string) any) ([]any, error) {
	ops := make([]any, 0, len(values))
	for _, raw := range values {
		if raw == "" || (raw[0] != '+' && raw[0] != '-') {
			return nil, fmt.Errorf("%s needs +name or -name (add or remove); got %q", flag, raw)
		}
		name := strings.TrimSpace(raw[1:])
		if name == "" {
			return nil, fmt.Errorf("%s needs +name or -name (add or remove); got %q", flag, raw)
		}
		op := "add"
		if raw[0] == '-' {
			op = "remove"
		}
		ops = append(ops, wrap(op, name))
	}
	return ops, nil
}

// withComponentHint appends editmeta's component names when an edit that
// sent --component failed. The GET runs only on that failure path; a
// missing components field, empty allowedValues, or a failed GET leaves
// the origin error unchanged (GDK-517, same shape as withParentHint).
func withComponentHint(ctx context.Context, c origin.Writer, key string, err error, asked bool) error {
	if err == nil || !asked {
		return err
	}
	names := componentHintNames(ctx, c, key)
	if len(names) == 0 {
		return err
	}
	return fmt.Errorf("%w\navailable components: %s", err, strings.Join(names, ", "))
}

func componentHintNames(ctx context.Context, c origin.Writer, key string) []string {
	meta, err := c.EditMeta(ctx, key)
	if err != nil {
		return nil
	}
	field, ok := meta["components"]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(field.AllowedValues))
	for _, v := range field.AllowedValues {
		name := v.Name
		if name == "" {
			name = v.Value
		}
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}
