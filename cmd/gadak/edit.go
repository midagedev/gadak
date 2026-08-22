package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/create"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/jirafields"
	"github.com/midagedev/gadak/internal/origin"
	"github.com/midagedev/gadak/internal/store"
	syncer "github.com/midagedev/gadak/internal/sync"
	"github.com/midagedev/gadak/internal/transition"
)

const editUsage = "usage: gadak edit <KEY> [--summary S] [-m <text|->] [--label +x|-x]... [--component +x|-x]... [--fix-version +id-or-name|-id-or-name]... [--priority NAME-or-id] [--due YYYY-MM-DD|none] [--parent KEY|none] [--field alias=value]... [--json] | --batch -"

// fieldFlagUsage is the FlagSet description for create/edit --field.
// Parse rule matches parseTransitionFieldFlags: JSON if valid, otherwise a string.
const fieldFlagUsage = "configured field alias=value (repeatable); JSON is parsed, otherwise a string"

func cmdEdit(args []string) error {
	fs := newFlagSet("edit")
	summary := fs.String("summary", "", "replace the summary")
	text := fs.String("m", "", "replace the description as plain text; `-` reads stdin; empty clears")
	var labels labelFlags
	fs.Var(&labels, "label", "`+name` or `-name` (repeatable)")
	var components labelFlags
	fs.Var(&components, "component", "`+name` or `-name` (repeatable)")
	var fixVersions labelFlags
	fs.Var(&fixVersions, "fix-version", "`+id-or-name` or `-id-or-name` (repeatable)")
	priority := fs.String("priority", "", "priority name or id")
	due := fs.String("due", "", "due date (YYYY-MM-DD); `none` clears")
	parent := fs.String("parent", "", "parent issue key; `none` clears")
	var fieldFlags labelFlags
	fs.Var(&fieldFlags, "field", fieldFlagUsage)
	asJSON := fs.Bool("json", false, "emit JSON")
	batch := fs.String("batch", "", "JSON lines from stdin (`-` only); each object needs key, plus any edit axis")
	if wantsHelp(args) {
		fmt.Fprint(os.Stdout, formatHelp("edit", fs))
		return nil
	}
	pos, err := parseAround(fs, args)
	if err != nil {
		return err
	}

	var hasSummary, hasM, hasLabel, hasComponent, hasFixVersion, hasPriority, hasParent, hasDue, hasField bool
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
		case "fix-version":
			hasFixVersion = true
		case "priority":
			hasPriority = true
		case "parent":
			hasParent = true
		case "due":
			hasDue = true
		case "field":
			hasField = true
		}
	})

	if *batch != "" {
		if err := rejectBatchFlag(*batch, func() string {
			if hasM {
				return *text
			}
			return ""
		}()); err != nil {
			return err
		}
		if len(pos) != 0 {
			return usageError("edit", "usage: gadak edit: --batch and a key are mutually exclusive")
		}
		base, err := parseEditChange(hasSummary, hasM, hasLabel, hasComponent, hasFixVersion, hasPriority, hasParent, hasDue, hasField,
			*summary, *text, *priority, *due, *parent, labels, components, fixVersions, fieldFlags)
		if err != nil {
			return err
		}
		return runEditBatch(*asJSON, base)
	}

	if len(pos) != 1 {
		return usageError("edit", editUsage)
	}
	key := normalizeKey(pos[0])

	if !hasSummary && !hasM && !hasLabel && !hasComponent && !hasFixVersion && !hasPriority && !hasParent && !hasDue && !hasField {
		return usageError("edit", editUsage)
	}

	body := *text
	if hasM && body == "-" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	ch, err := parseEditChange(hasSummary, hasM, hasLabel, hasComponent, hasFixVersion, hasPriority, hasParent, hasDue, hasField,
		*summary, body, *priority, *due, *parent, labels, components, fixVersions, fieldFlags)
	if err != nil {
		return err
	}
	ch.key = key
	return mutate(key, *asJSON, func(ctx context.Context, c origin.Writer, src string) (map[string]any, error) {
		return nil, applyEditChange(ctx, c, src, ch)
	})
}

// editChange is one edit write, shared by the single-key path and --batch.
type editChange struct {
	key                                           string
	hasSummary, hasM, hasLabel, hasComponent      bool
	hasFixVersion, hasPriority, hasParent, hasDue bool
	hasField                                      bool
	summary, body, priority, dueDate, parentKey   string
	clearDue, clearParent                         bool
	labels, components, fixVersions               []string
	labelOps, componentOps                        []any
	fieldRaws                                     map[string]json.RawMessage
	fieldCfg                                      *config.Config
}

func (e editChange) empty() bool {
	return !e.hasSummary && !e.hasM && !e.hasLabel && !e.hasComponent && !e.hasFixVersion && !e.hasPriority && !e.hasParent && !e.hasDue && !e.hasField
}

func parseEditChange(hasSummary, hasM, hasLabel, hasComponent, hasFixVersion, hasPriority, hasParent, hasDue, hasField bool,
	summary, body, priority, due, parent string, labels, components, fixVersions, fieldFlags []string) (editChange, error) {
	var ch editChange
	ch.hasSummary, ch.hasM, ch.hasLabel, ch.hasComponent = hasSummary, hasM, hasLabel, hasComponent
	ch.hasFixVersion, ch.hasPriority, ch.hasParent, ch.hasDue, ch.hasField = hasFixVersion, hasPriority, hasParent, hasDue, hasField
	ch.summary, ch.body, ch.priority = summary, body, priority
	ch.labels, ch.components, ch.fixVersions = labels, components, fixVersions

	if hasSummary && strings.TrimSpace(summary) == "" {
		return ch, usageError("edit", editUsage)
	}
	if hasDue {
		if strings.TrimSpace(due) == "none" {
			ch.clearDue = true
		} else {
			dueDate, derr := parseDueDate(due, "edit")
			if derr != nil {
				return ch, derr
			}
			if dueDate == "" {
				return ch, usageError("edit", editUsage)
			}
			ch.dueDate = dueDate
		}
	}
	if hasParent {
		if strings.TrimSpace(parent) == "none" {
			ch.clearParent = true
		} else {
			parentKey, perr := parseParentKey(parent, "edit")
			if perr != nil {
				return ch, perr
			}
			if parentKey == "" {
				return ch, usageError("edit", editUsage)
			}
			ch.parentKey = parentKey
		}
	}
	var err error
	if hasLabel {
		ch.labelOps, err = labelUpdateOps(labels)
		if err != nil {
			return ch, err
		}
	}
	if hasComponent {
		ch.componentOps, err = componentUpdateOps(components)
		if err != nil {
			return ch, err
		}
	}
	if hasFixVersion {
		// +/- is local, same as --label/--component, so a bare value never
		// opens a write session or fetches the catalog.
		if _, err = signedUpdateOps("--fix-version", fixVersions, func(op, name string) any {
			return nil
		}); err != nil {
			return ch, err
		}
	}
	if hasField {
		ch.fieldRaws, err = parseAliasFieldRaws(fieldFlags)
		if err != nil {
			return ch, err
		}
		ch.fieldCfg, err = config.Load()
		if err != nil {
			return ch, err
		}
		if err := checkConfiguredAliases(ch.fieldCfg, ch.fieldRaws); err != nil {
			return ch, err
		}
	}
	return ch, nil
}

func applyEditChange(ctx context.Context, c origin.Writer, src string, ch editChange) error {
	key := ch.key
	fields := map[string]any{}
	update := map[string]any{}
	if ch.hasSummary {
		fields["summary"] = strings.TrimSpace(ch.summary)
	}
	if ch.hasM {
		if strings.TrimSpace(ch.body) == "" {
			fields["description"] = nil
		} else {
			fields["description"] = jira.Doc(ch.body, nil)
		}
	}
	if ch.hasLabel {
		update["labels"] = ch.labelOps
	}
	if ch.hasComponent {
		update["components"] = ch.componentOps
	}
	if ch.hasFixVersion {
		ops, ferr := fixVersionUpdateOps(ctx, c, key, ch.fixVersions)
		if ferr != nil {
			return ferr
		}
		update["fixVersions"] = ops
	}
	if ch.hasPriority {
		list, err := c.PriorityCatalog(ctx)
		if err != nil {
			return err
		}
		id, err := create.Priority(ch.priority, list)
		if err != nil {
			return formatCreateError(err)
		}
		fields["priority"] = map[string]string{"id": id}
	}
	if ch.hasParent {
		if ch.clearParent {
			fields["parent"] = nil
		} else {
			fields["parent"] = map[string]string{"key": ch.parentKey}
		}
	}
	if ch.hasDue {
		if ch.clearDue {
			fields["duedate"] = nil
		} else {
			fields["duedate"] = ch.dueDate
		}
	}
	if len(ch.fieldRaws) > 0 {
		meta, merr := c.EditMeta(ctx, key)
		if merr != nil {
			return merr
		}
		custom, cerr := resolveEditAliasFields(ctx, c, src, ch.fieldCfg, meta, ch.fieldRaws, key)
		if cerr != nil {
			return cerr
		}
		for id, v := range custom {
			fields[id] = v
		}
	}
	// A --parent the origin refuses gets the mirror's hierarchy answer,
	// the same one create gives. withParentHint owns the "is this a
	// parent rejection" test because the field key differs by verb
	// (create: parent/parentId, edit: pid — GDK-525).
	err := c.EditIssue(ctx, key, fields, update)
	err = withParentHint(ctx, err, ch.parentKey)
	return withComponentHint(ctx, c, key, err, ch.hasComponent)
}

var editBatchFields = []string{"key", "summary", "description", "labels", "components", "fix_versions", "priority", "due", "parent", "fields"}

func runEditBatch(asJSON bool, base editChange) error {
	return runWriteBatch("edit", asJSON, func(raw string) batchResult {
		obj, key, err := parseBatchLine(raw, editBatchFields)
		if err != nil {
			return batchErr(key, false, err)
		}
		ch, err := overlayEditJSON(base, obj)
		if err != nil {
			return batchErr(key, false, err)
		}
		ch.key = key
		if ch.empty() {
			return batchErr(key, false, errors.New("nothing to edit — JSON line needs one of summary, description, labels, components, fix_versions, priority, due, parent, fields"))
		}
		var wrote bool
		err = withKeyWriteSession(key, func(ctx context.Context, cfg *config.Config, db *store.DB, c origin.Writer, src string) error {
			if err := applyEditChange(ctx, c, src, ch); err != nil {
				return err
			}
			wrote = true
			return syncer.RefreshIssue(ctx, cfg, db, key, src)
		})
		if err != nil {
			return batchErr(key, wrote, err)
		}
		return batchOK(key, true)
	})
}

func overlayEditJSON(base editChange, obj map[string]json.RawMessage) (editChange, error) {
	ch := base
	if s, ok, err := jsonStringField(obj, "summary"); err != nil {
		return ch, err
	} else if ok {
		if strings.TrimSpace(s) == "" {
			return ch, usageError("edit", editUsage)
		}
		ch.hasSummary = true
		ch.summary = s
	}
	if s, ok, err := jsonStringField(obj, "description"); err != nil {
		return ch, err
	} else if ok {
		ch.hasM = true
		ch.body = s
	}
	if v, ok, err := jsonStringArrayField(obj, "labels"); err != nil {
		return ch, err
	} else if ok {
		ops, err := labelUpdateOps(v)
		if err != nil {
			return ch, err
		}
		ch.hasLabel = true
		ch.labels = v
		ch.labelOps = ops
	}
	if v, ok, err := jsonStringArrayField(obj, "components"); err != nil {
		return ch, err
	} else if ok {
		ops, err := componentUpdateOps(v)
		if err != nil {
			return ch, err
		}
		ch.hasComponent = true
		ch.components = v
		ch.componentOps = ops
	}
	if v, ok, err := jsonStringArrayField(obj, "fix_versions"); err != nil {
		return ch, err
	} else if ok {
		if _, err = signedUpdateOps("--fix-version", v, func(op, name string) any {
			return nil
		}); err != nil {
			return ch, err
		}
		ch.hasFixVersion = true
		ch.fixVersions = v
	}
	if s, ok, err := jsonStringField(obj, "priority"); err != nil {
		return ch, err
	} else if ok {
		ch.hasPriority = true
		ch.priority = s
	}
	if s, ok, err := jsonStringField(obj, "due"); err != nil {
		return ch, err
	} else if ok {
		if strings.TrimSpace(s) == "none" {
			ch.hasDue = true
			ch.clearDue = true
			ch.dueDate = ""
		} else {
			dueDate, derr := parseDueDate(s, "edit")
			if derr != nil {
				return ch, derr
			}
			if dueDate == "" {
				return ch, usageError("edit", editUsage)
			}
			ch.hasDue = true
			ch.clearDue = false
			ch.dueDate = dueDate
		}
	}
	if s, ok, err := jsonStringField(obj, "parent"); err != nil {
		return ch, err
	} else if ok {
		if strings.TrimSpace(s) == "none" {
			ch.hasParent = true
			ch.clearParent = true
			ch.parentKey = ""
		} else {
			parentKey, perr := parseParentKey(s, "edit")
			if perr != nil {
				return ch, perr
			}
			if parentKey == "" {
				return ch, usageError("edit", editUsage)
			}
			ch.hasParent = true
			ch.clearParent = false
			ch.parentKey = parentKey
		}
	}
	if v, ok, err := jsonObjectField(obj, "fields"); err != nil {
		return ch, err
	} else if ok {
		merged := mergeFieldRaws(ch.fieldRaws, v)
		cfg := ch.fieldCfg
		if cfg == nil {
			var lerr error
			cfg, lerr = config.Load()
			if lerr != nil {
				return ch, lerr
			}
		}
		if err := checkConfiguredAliases(cfg, merged); err != nil {
			return ch, err
		}
		ch.hasField = true
		ch.fieldRaws = merged
		ch.fieldCfg = cfg
	}
	return ch, nil
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

type signedToken struct {
	op, token string
}

// fixVersionUpdateOps turns --fix-version +v2.5 / --fix-version -10012 into
// Jira update verbs keyed by version id. All-digit tokens are ids and skip
// the catalog; names resolve against GET /project/{key}/versions once.
func fixVersionUpdateOps(ctx context.Context, c origin.Writer, issueKey string, values []string) ([]any, error) {
	parsed, err := signedUpdateOps("--fix-version", values, func(op, name string) any {
		return signedToken{op: op, token: name}
	})
	if err != nil {
		return nil, err
	}
	needCatalog := false
	for _, p := range parsed {
		if !transition.AllASCIIDigits(p.(signedToken).token) {
			needCatalog = true
			break
		}
	}
	var catalog []jira.Version
	if needCatalog {
		pk := projectKeyFromIssueKey(issueKey)
		if pk == "" {
			return nil, fmt.Errorf("cannot derive project key from %q", issueKey)
		}
		catalog, err = c.ProjectVersions(ctx, pk)
		if err != nil {
			return nil, err
		}
	}
	ops := make([]any, 0, len(parsed))
	for _, p := range parsed {
		tok := p.(signedToken)
		id, err := resolveFixVersionID(tok.token, catalog)
		if err != nil {
			return nil, err
		}
		ops = append(ops, map[string]any{tok.op: map[string]string{"id": id}})
	}
	return ops, nil
}

func projectKeyFromIssueKey(key string) string {
	i := strings.LastIndex(key, "-")
	if i <= 0 {
		return ""
	}
	return key[:i]
}

func resolveFixVersionID(token string, catalog []jira.Version) (string, error) {
	if transition.AllASCIIDigits(token) {
		return token, nil
	}
	var hits []jira.Version
	for _, v := range catalog {
		if strings.EqualFold(v.Name, token) {
			hits = append(hits, v)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0].ID, nil
	case 0:
		return "", fmt.Errorf("no fix version matching %q — available: %s", token, formatVersionCatalog(catalog))
	default:
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.ID)
		}
		return "", fmt.Errorf("fix version %q is ambiguous — matches: %s", token, strings.Join(ids, ", "))
	}
}

func formatVersionCatalog(list []jira.Version) string {
	named := make([]jira.NamedID, 0, len(list))
	for _, v := range list {
		named = append(named, jira.NamedID{ID: v.ID, Name: v.Name})
	}
	return transition.FormatNamedIDs(named)
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

// parseAliasFieldRaws turns --field alias=value into JSON payloads.
// The split/parse rule is parseTransitionFieldFlags: JSON if valid, otherwise
// a string. Re-marshal so fields.FieldValue can consume json.RawMessage.
func parseAliasFieldRaws(raw []string) (map[string]json.RawMessage, error) {
	parsed, err := parseTransitionFieldFlags(raw)
	if err != nil {
		return nil, err
	}
	if len(parsed) == 0 {
		return nil, nil
	}
	out := make(map[string]json.RawMessage, len(parsed))
	for k, v := range parsed {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		out[k] = b
	}
	return out, nil
}

func checkConfiguredAliases(cfg *config.Config, raws map[string]json.RawMessage) error {
	allow := fields.EditableAliases(cfg)
	for alias := range raws {
		if _, ok := allow[alias]; !ok {
			return unknownFieldAliasError(alias, allow)
		}
	}
	return nil
}

func unknownFieldAliasError(alias string, allow map[string]fields.EditableAlias) error {
	names := make([]string, 0, len(allow))
	for a := range allow {
		names = append(names, a)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Errorf("unknown field alias %q — no configured aliases; run `gadak fields --apply`, then `gadak issue KEY --editmeta`", alias)
	}
	return fmt.Errorf("unknown field alias %q — configured: %s; run `gadak fields --apply`, then `gadak issue KEY --editmeta`", alias, strings.Join(names, ", "))
}

func resolveEditAliasFields(ctx context.Context, c origin.Writer, src string, cfg *config.Config, meta map[string]jira.FieldMeta, raws map[string]json.RawMessage, key string) (map[string]any, error) {
	allow := fields.EditableAliases(cfg)
	out := make(map[string]any, len(raws))
	for alias, raw := range raws {
		ea := allow[alias]
		id, kind, present := jirafields.ResolveEditable(ea.IDs, meta, ea.Kind)
		if !present {
			return nil, fmt.Errorf("field %q is not editable on %s — `gadak issue %s --editmeta`", alias, key, key)
		}
		val, err := wrapAliasValue(ctx, c, src, kind, raw, meta[id])
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", alias, err)
		}
		out[id] = val
	}
	return out, nil
}

func resolveCreateAliasFields(ctx context.Context, c origin.Writer, src string, cfg *config.Config, list []jira.CreateFieldMeta, raws map[string]json.RawMessage) (map[string]any, error) {
	meta := make(map[string]jira.FieldMeta, len(list))
	for _, f := range list {
		meta[f.FieldID] = jirafields.FieldMetaFromCreate(f)
	}
	allow := fields.EditableAliases(cfg)
	out := make(map[string]any, len(raws))
	for alias, raw := range raws {
		ea := allow[alias]
		id, kind, present := jirafields.ResolveEditable(ea.IDs, meta, ea.Kind)
		if !present {
			return nil, fmt.Errorf("field %q is not available for this project and type — `gadak issue KEY --editmeta`", alias)
		}
		val, err := wrapAliasValue(ctx, c, src, kind, raw, meta[id])
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", alias, err)
		}
		out[id] = val
	}
	return out, nil
}

func wrapAliasValue(ctx context.Context, c origin.Writer, src, kind string, raw json.RawMessage, meta jira.FieldMeta) (any, error) {
	if kind == "user" {
		if len(raw) == 0 || string(raw) == "null" {
			return fields.FieldValue(kind, raw)
		}
		tok, ok := scalarToken(raw)
		if !ok {
			return fields.FieldValue(kind, raw)
		}
		id, err := resolveAccount(ctx, c, tok, src)
		if err != nil {
			return nil, err
		}
		return fields.ValueFromIDs("user", []string{id}), nil
	}
	if kind == "" {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	mapped, err := mapAllowedRaw(raw, meta)
	if err != nil {
		return nil, err
	}
	return fields.FieldValue(kind, mapped)
}

func mapAllowedRaw(raw json.RawMessage, meta jira.FieldMeta) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" || len(meta.AllowedValues) == 0 {
		return raw, nil
	}
	var tokens []string
	if err := json.Unmarshal(raw, &tokens); err == nil {
		ids := make([]string, len(tokens))
		for i, tok := range tokens {
			id, err := matchAllowedID(tok, meta)
			if err != nil {
				return nil, err
			}
			ids[i] = id
		}
		return json.Marshal(ids)
	}
	tok, ok := scalarToken(raw)
	if !ok {
		return raw, nil
	}
	id, err := matchAllowedID(tok, meta)
	if err != nil {
		return nil, err
	}
	return json.Marshal(id)
}

func scalarToken(raw json.RawMessage) (string, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String(), true
	}
	return "", false
}

func matchAllowedID(token string, meta jira.FieldMeta) (string, error) {
	token = strings.TrimSpace(token)
	var nameHits []string
	for _, v := range meta.AllowedValues {
		if v.ID == token {
			return v.ID, nil
		}
		label := v.Value
		if label == "" {
			label = v.Name
		}
		if strings.EqualFold(label, token) {
			nameHits = append(nameHits, v.ID)
		}
	}
	switch len(nameHits) {
	case 1:
		return nameHits[0], nil
	case 0:
		return "", fmt.Errorf("no value matching %q — available: %s", token, formatAllowedValues(meta))
	default:
		return "", fmt.Errorf("field value %q is ambiguous — matches: %s", token, strings.Join(nameHits, ", "))
	}
}

func formatAllowedValues(meta jira.FieldMeta) string {
	parts := make([]string, 0, len(meta.AllowedValues))
	for _, v := range meta.AllowedValues {
		label := v.Value
		if label == "" {
			label = v.Name
		}
		if label == "" {
			label = v.ID
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}
