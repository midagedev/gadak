// Package transition is the single owner of the gadak transition write
// (CLI, including `gadak close`, and REST). Both surfaces call Apply so
// identifier resolution, required screen fields, resolution lookup,
// field-alias remap, category-token idempotency, and the origin write
// cannot exist on one path and not the other.
//
// Refused errors carry catalogue data only. The CLI formats flag names;
// REST maps them to HTTP 400. This package does not name CLI flags.
//
// Mirror refresh is not here: it is the write-through tail shared by every
// write verb (CLI mutate / server mutate), not a property of this verb.
package transition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/fields"
	"github.com/midagedev/gadak/internal/jira"
	"github.com/midagedev/gadak/internal/statuscat"
)

// Origin is the origin verbs Apply needs. origin.Writer satisfies it.
type Origin interface {
	Transitions(ctx context.Context, key string) ([]jira.Transition, error)
	Transition(ctx context.Context, key, transitionID string, fields map[string]any, comment json.RawMessage) error
}

// Request is one transition write. Target is whatever PickTransition accepts:
// transition id, target status id, transition/status name, or a status
// category token (new|inprogress|done). Fields are already parsed (CLI
// --field, REST JSON); aliases are remapped here. Resolution is a name or
// id; a name is resolved from the transition's allowedValues, else the
// origin's resolution catalog.
type Request struct {
	Key        string
	Target     string
	Resolution string
	Fields     map[string]any
	Comment    string
}

// Result is a successful Apply. Changed is false when Target was a category
// token and the origin already reports that category: no POST (and no
// comment). A named or id miss is still an error, not a no-op.
type Result struct {
	Changed bool `json:"changed"`
}

// issueStatusReader is GET /issue/{key}?fields=status,assignee — the origin
// truth claim already uses. Optional so Linear (no method) keeps the old
// pick-miss error instead of inventing a new request. Looked up only after
// PickTransition fails a category token.
type issueStatusReader interface {
	IssueStatus(ctx context.Context, key string) (jira.Status, *jira.User, error)
}

// Refused is a caller-side refusal: the origin was not written. Adapters
// map this to a 400 / CLI message; origin errors pass through unchanged.
type Refused struct {
	Msg string
}

func (e *Refused) Error() string { return e.Msg }

// RequiredFieldsError is a Refused whose screen lists required fields the
// request did not supply. CLI appends flag names; REST uses Error as-is.
type RequiredFieldsError struct {
	Key             string
	TransitionName  string
	FormattedFields []string
}

func (e *RequiredFieldsError) Error() string {
	return fmt.Sprintf("%s %s requires: %s", e.Key, e.TransitionName, strings.Join(e.FormattedFields, "; "))
}

// IsRefused reports whether err is a caller-side refusal (bad identifier,
// missing required field, unknown resolution). Origin errors are not.
func IsRefused(err error) bool {
	var r *Refused
	var q *RequiredFieldsError
	return errors.As(err, &r) || errors.As(err, &q)
}

// Apply lists the issue's transitions, picks one, assembles fields, and
// calls the origin. A category-token target (new|inprogress|done) that the
// origin already reports is a no-op success (Changed=false). It does not
// refresh the mirror — that tail is mutate.
func Apply(ctx context.Context, o Origin, cfg *config.Config, req Request) (Result, error) {
	list, err := o.Transitions(ctx, req.Key)
	if err != nil {
		return Result{}, err
	}
	id, noop, pickErr, err := resolveTransition(ctx, o, req.Key, req.Target, list)
	if err != nil {
		return Result{}, err
	}
	if noop {
		return Result{Changed: false}, nil
	}
	if pickErr != nil {
		return Result{}, &Refused{Msg: pickErr.Error()}
	}
	selected := byID(list, id)
	assembled, err := assembleFields(ctx, o, cfg, selected, req.Resolution, req.Fields)
	if err != nil {
		return Result{}, err
	}
	if missing := missingRequired(selected, assembled); len(missing) > 0 {
		parts := make([]string, 0, len(missing))
		for _, k := range missing {
			parts = append(parts, formatField(k, selected.Fields[k]))
		}
		return Result{}, &RequiredFieldsError{Key: req.Key, TransitionName: selected.Name, FormattedFields: parts}
	}
	var comment json.RawMessage
	if strings.TrimSpace(req.Comment) != "" {
		comment = jira.Doc(req.Comment, nil)
	}
	if err := o.Transition(ctx, req.Key, id, assembled, comment); err != nil {
		if hint := describeFields(selected); hint != "" {
			return Result{}, fmt.Errorf("%w\n%s", err, hint)
		}
		return Result{}, err
	}
	return Result{Changed: true}, nil
}

// Preview answers what Apply would do without writing: the resolved
// transition id when one would fire (changed=true), or the category no-op
// (changed=false, empty id). It exists so a dry-run cannot drift from the
// real write — both run resolveTransition. A pick miss that is not a no-op
// returns the pick error.
func Preview(ctx context.Context, o Origin, key, target string) (id string, changed bool, err error) {
	list, err := o.Transitions(ctx, key)
	if err != nil {
		return "", false, err
	}
	id, noop, pickErr, err := resolveTransition(ctx, o, key, target, list)
	if err != nil {
		return "", false, err
	}
	if noop {
		return "", false, nil
	}
	if pickErr != nil {
		return "", false, pickErr
	}
	return id, true, nil
}

// resolveTransition is the one owner of "what does this target mean right
// now". It picks against list, and gates a category-token target on the
// origin's current status in BOTH pick outcomes: a miss (the workflow offers
// nothing toward that category — the GDK-500 no-op) and a hit (a self-loop
// workflow keeps a done→done transition available while the issue is already
// done, so a retry would fire again and double-post its comment — GDK-632,
// caught on a real site). Category targets cost one IssueStatus read per
// write; named/id targets never pay it. pickErr is non-nil only when the
// pick missed and the miss is not a no-op; err is an origin failure.
func resolveTransition(ctx context.Context, o Origin, key, target string, list []jira.Transition) (id string, noop bool, pickErr, err error) {
	id, perr := jira.PickTransition(key, target, list)
	if _, isCategory := jira.StatusCategoryToken(target); isCategory {
		there, nerr := alreadyInCategory(ctx, o, key, target)
		if nerr != nil {
			return "", false, nil, nerr
		}
		if there {
			return "", true, nil, nil
		}
	}
	if perr != nil {
		return "", false, perr, nil
	}
	return id, false, nil, nil
}

func alreadyInCategory(ctx context.Context, o Origin, key, target string) (bool, error) {
	token, ok := jira.StatusCategoryToken(target)
	if !ok {
		return false, nil
	}
	r, ok := o.(issueStatusReader)
	if !ok {
		return false, nil
	}
	st, _, err := r.IssueStatus(ctx, key)
	if err != nil {
		return false, err
	}
	cat, ok := statuscat.KnownCategory(st.StatusCategory.Key)
	if !ok || cat != token {
		return false, nil
	}
	return true, nil
}

func byID(list []jira.Transition, id string) jira.Transition {
	for _, t := range list {
		if t.ID == id {
			return t
		}
	}
	return jira.Transition{ID: id}
}

func assembleFields(ctx context.Context, o Origin, cfg *config.Config, selected jira.Transition, resolution string, raw map[string]any) (map[string]any, error) {
	out := copyFields(raw)
	remapAliases(cfg, out)
	if r := strings.TrimSpace(resolution); r != "" {
		val, err := resolveResolution(ctx, o, selected, r)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = map[string]any{}
		}
		out["resolution"] = val
	}
	return out, nil
}

func copyFields(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func keepsScreenKey(key string) bool {
	if key == "resolution" || strings.HasPrefix(strings.ToLower(key), "customfield_") {
		return true
	}
	return false
}

func remapAliases(cfg *config.Config, out map[string]any) {
	if cfg == nil || len(out) == 0 {
		return
	}
	allow := fields.EditableAliases(cfg)
	for key, val := range out {
		if keepsScreenKey(key) {
			continue
		}
		ea, ok := allow[key]
		if !ok || len(ea.IDs) == 0 {
			continue
		}
		id := ea.IDs[0]
		if id == "" || id == key {
			continue
		}
		delete(out, key)
		out[id] = val
	}
}

// resolutionCatalog is the origin method that lists GET /resolution.
// *jira.Client implements it; Linear does not (screen fields are refused
// on Transition).
type resolutionCatalog interface {
	Resolutions(context.Context) ([]jira.NamedID, error)
}

func resolveResolution(ctx context.Context, o Origin, selected jira.Transition, want string) (map[string]string, error) {
	want = strings.TrimSpace(want)
	if want == "" {
		return nil, &Refused{Msg: "empty resolution"}
	}
	if AllASCIIDigits(want) {
		return map[string]string{"id": want}, nil
	}
	var catalog []jira.NamedID
	if f, ok := selected.Fields["resolution"]; ok {
		catalog = f.AllowedValues
	} else {
		rc, ok := o.(resolutionCatalog)
		if !ok {
			return nil, &Refused{Msg: fmt.Sprintf("no resolution matching %q — this origin does not expose a resolution catalog", want)}
		}
		var err error
		catalog, err = rc.Resolutions(ctx)
		if err != nil {
			return nil, err
		}
	}
	id, err := matchResolution(want, catalog)
	if err != nil {
		return nil, err
	}
	return map[string]string{"id": id}, nil
}

func matchResolution(want string, list []jira.NamedID) (string, error) {
	var hits []jira.NamedID
	for _, p := range list {
		if p.ID == want || strings.EqualFold(p.Name, want) {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0].ID, nil
	case 0:
		return "", &Refused{Msg: fmt.Sprintf("no resolution matching %q — available: %s", want, FormatNamedIDs(list))}
	default:
		return "", &Refused{Msg: fmt.Sprintf("resolution %q is ambiguous — matches: %s", want, FormatNamedIDs(hits))}
	}
}

// FormatNamedIDs joins a catalog's names (ids where the name is empty) with
// ", " — the "available: …" list in resolution errors. Single owner; the
// `gadak agent` / `gadak edit` surfaces import it (GDK-619).
func FormatNamedIDs(list []jira.NamedID) string {
	if len(list) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(list))
	for _, n := range list {
		if n.Name != "" {
			parts = append(parts, n.Name)
		} else {
			parts = append(parts, n.ID)
		}
	}
	return strings.Join(parts, ", ")
}

// AllASCIIDigits reports whether s is one or more ASCII digits — the "the
// user typed a bare id, not a name" discriminator in catalog resolution.
func AllASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func missingRequired(t jira.Transition, provided map[string]any) []string {
	if len(t.Fields) == 0 {
		return nil
	}
	var missing []string
	for k, f := range t.Fields {
		if !f.Required {
			continue
		}
		if _, ok := provided[k]; ok {
			continue
		}
		missing = append(missing, k)
	}
	sort.Strings(missing)
	return missing
}

func formatField(key string, f jira.TransitionField) string {
	part := key
	if f.Name != "" {
		part += " (" + f.Name + ")"
	}
	if names := FormatNamedIDs(f.AllowedValues); names != "" && names != "(none)" {
		part += " — allowed: " + names
	}
	return part
}

func describeFields(t jira.Transition) string {
	if len(t.Fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(t.Fields))
	for k := range t.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, formatField(k, t.Fields[k]))
	}
	return "this transition exposes: " + strings.Join(parts, "; ")
}
