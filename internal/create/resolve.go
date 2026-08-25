// Package create is the single owner of project, issue-type, and priority
// resolution for gadak create (CLI and REST). Both surfaces must call these
// functions so a default cannot exist on one path and not the other.
//
// Need* errors carry catalogue data only. The CLI formats flag names;
// REST maps them to stable wire codes. This package does not name CLI flags.
package create

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/origin"
)

// Source names recorded on --json / the REST create response so a caller
// can see why a project or type was chosen. Alias is a match that was not
// the catalog id or display name — untranslatedName, a hierarchy-derived
// epic/subtask token, or the small standard-name locale table.
const (
	SourceFlag   = "flag"
	SourceConfig = "config"
	SourceSole   = "sole"
	SourceAlias  = "alias"
)

// Resolved is one filled create field and where it came from.
type Resolved struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// Project resolves the create project:
//  1. explicit flag / request field
//  2. profile DefaultProject
//  3. the sole configured project
//  4. otherwise fail — inventing a project is not a default.
func Project(want string, cfg *config.Config) (Resolved, error) {
	if p := strings.TrimSpace(want); p != "" {
		return Resolved{Value: p, Source: SourceFlag}, nil
	}
	if cfg != nil {
		if p := strings.TrimSpace(cfg.DefaultProject); p != "" {
			return Resolved{Value: p, Source: SourceConfig}, nil
		}
	}
	var configured []string
	if cfg != nil {
		configured = cfg.Projects
	}
	if len(configured) == 1 && strings.TrimSpace(configured[0]) != "" {
		return Resolved{Value: configured[0], Source: SourceSole}, nil
	}
	if len(configured) == 0 {
		return Resolved{}, &NeedProjectError{}
	}
	return Resolved{}, &NeedProjectError{Configured: copyStrings(configured)}
}

// Type resolves the create issue type:
//  1. explicit flag / request field (matchType: id, name, then aliases)
//  2. profile DefaultIssueTypeID, matched by id only
//  3. the project has exactly one createmeta type
//  4. otherwise fail
//
// types[0] is deliberately not a fallback. The web dialog may pick
// types[0] because the person can see and change it before submit. A
// headless CLI or REST caller never sees that value, so the same
// fallback would silently file as the wrong type.
func Type(want string, types []origin.CreateMetaIssueType, cfg *config.Config, project string) (Resolved, error) {
	named := namedTypes(types)
	if w := strings.TrimSpace(want); w != "" {
		id, src, err := matchType(w, types)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Value: id, Source: src}, nil
	}
	if cfg != nil {
		if id := strings.TrimSpace(cfg.DefaultIssueTypeID); id != "" {
			for _, t := range types {
				if t.ID == id {
					return Resolved{Value: t.ID, Source: SourceConfig}, nil
				}
			}
			where := "createmeta"
			if p := strings.TrimSpace(project); p != "" {
				where = p
			}
			return Resolved{}, fmt.Errorf("configured default issue type id %s is not available in %s — available: %s", id, where, FormatTypes(named))
		}
	}
	if len(types) == 1 && strings.TrimSpace(types[0].ID) != "" {
		return Resolved{Value: types[0].ID, Source: SourceSole}, nil
	}
	return Resolved{}, &NeedTypeError{Available: named}
}

// matchType is the single owner of --type / issue_type matching (GDK-741).
// Steps are exclusive: a later step runs only when every earlier step
// produced zero hits. Two or more hits at the same step is a hard error —
// filing under the wrong type is worse than asking for an id.
//
//  1. exact id (today's id match; SourceFlag)
//  2. name, case-insensitive (today's name match; SourceFlag)
//  3. untranslatedName, case-insensitive, only when it differs from name
//  4. structural aliases from the catalog: "epic" → hierarchyLevel >= 1;
//     "subtask" / "sub-task" / "sub task" → subtask == true
//  5. the small standard-name locale table (Bug/Task/Story/Epic/Subtask)
func matchType(want string, types []origin.CreateMetaIssueType) (id, source string, err error) {
	if hits := typesMatching(types, func(t origin.CreateMetaIssueType) bool { return t.ID == want }); len(hits) > 0 {
		return settleType(want, SourceFlag, hits)
	}
	if hits := typesMatching(types, func(t origin.CreateMetaIssueType) bool { return strings.EqualFold(t.Name, want) }); len(hits) > 0 {
		return settleType(want, SourceFlag, hits)
	}
	if hits := typesMatching(types, untranslatedNameMatch(want)); len(hits) > 0 {
		return settleType(want, SourceAlias, hits)
	}
	if hits := structuralMatches(want, types); len(hits) > 0 {
		return settleType(want, SourceAlias, hits)
	}
	if hits := localeAliasMatches(want, types); len(hits) > 0 {
		return settleType(want, SourceAlias, hits)
	}
	// The hint is phrased as a fact about the catalog, not about the site:
	// on an English Jira "names on this site are localised" would be wrong,
	// and a hint that is sometimes false is worse than none.
	return "", "", fmt.Errorf("no issue type matching %q — type names follow the site's own language; an id always works — available: %s", want, FormatTypes(namedTypes(types)))
}

func settleType(want, source string, hits []origin.CreateMetaIssueType) (string, string, error) {
	if len(hits) == 1 {
		return hits[0].ID, source, nil
	}
	return "", "", fmt.Errorf("issue type %q matches more than one catalog type: %s — an id settles it", want, FormatTypes(namedTypes(hits)))
}

func typesMatching(types []origin.CreateMetaIssueType, ok func(origin.CreateMetaIssueType) bool) []origin.CreateMetaIssueType {
	var hits []origin.CreateMetaIssueType
	for _, t := range types {
		if ok(t) {
			hits = append(hits, t)
		}
	}
	return hits
}

func untranslatedNameMatch(want string) func(origin.CreateMetaIssueType) bool {
	return func(t origin.CreateMetaIssueType) bool {
		u := strings.TrimSpace(t.UntranslatedName)
		if u == "" || strings.EqualFold(u, t.Name) {
			return false
		}
		return strings.EqualFold(u, want)
	}
}

func structuralMatches(want string, types []origin.CreateMetaIssueType) []origin.CreateMetaIssueType {
	var hits []origin.CreateMetaIssueType
	switch strings.ToLower(want) {
	case "epic":
		// Level 1 exactly. Jira's Epic is level 1; 2 and above are the
		// premium parent tiers (Initiative, Theme). `>= 1` let `--type epic`
		// succeed on a project whose only parent-level type is Initiative,
		// and file there — the wrong-type outcome this matcher refuses to
		// risk everywhere else. A sole Initiative now misses this step,
		// misses the locale table, and is refused with the catalog.
		for _, t := range types {
			if t.HierarchyLevel == 1 {
				hits = append(hits, t)
			}
		}
	case "subtask", "sub-task", "sub task":
		for _, t := range types {
			if t.Subtask {
				hits = append(hits, t)
			}
		}
	}
	return hits
}

// standardTypeLocales maps the five issue-type names Jira ships onto
// display names this repository has actually measured. Incomplete on
// purpose: a name we have not seen must not invent a site convention
// (기능 and 요청 on the reporting site are that site's own types).
//
// Korean: live createmeta for GDK on 2026-08-26
// (`GET /rest/api/3/issue/createmeta?projectKeys=GDK`). On that site
// untranslatedName equals name because the types were authored in Korean,
// so this table — not untranslatedName — is what closes `--type Bug`.
var standardTypeLocales = map[string][]string{
	"bug":     {"버그"},
	"task":    {"작업"},
	"story":   {"스토리"},
	"epic":    {"에픽"},
	"subtask": {"하위 작업"},
}

func localeAliasMatches(want string, types []origin.CreateMetaIssueType) []origin.CreateMetaIssueType {
	locales := standardTypeLocales[strings.ToLower(want)]
	if len(locales) == 0 {
		return nil
	}
	var hits []origin.CreateMetaIssueType
	for _, t := range types {
		for _, loc := range locales {
			if strings.EqualFold(t.Name, loc) {
				hits = append(hits, t)
				break
			}
		}
	}
	return hits
}

func namedTypes(types []origin.CreateMetaIssueType) []origin.NamedID {
	return origin.CreateMetaProject{IssueTypes: types}.NamedTypes()
}

// MetaFor picks the createmeta project (case-insensitive key) and its types.
// A miss lists the keys in meta the same way Type lists the issue-type catalog.
// Types stay as CreateMetaIssueType so Type can see hierarchy and
// untranslatedName; NamedID flattening is FormatTypes / NeedTypeError only.
func MetaFor(meta []origin.CreateMetaProject, project string, cfg *config.Config) (origin.CreateMetaProject, []origin.CreateMetaIssueType, error) {
	for _, p := range meta {
		if strings.EqualFold(p.Key, project) {
			return p, p.IssueTypes, nil
		}
	}
	suffix := availableProjectsSuffix(meta)
	if cfg != nil && cfg.IsStandalone() {
		return origin.CreateMetaProject{}, nil, fmt.Errorf("project %s does not exist in this workspace%s", project, suffix)
	}
	return origin.CreateMetaProject{}, nil, fmt.Errorf("this credential cannot create issues in %s%s", project, suffix)
}

// CreateMetaSource is the origin verb MetaForWithCatalog uses on the
// empty-filter fallback. origin.Writer and *jira.Client both satisfy it.
type CreateMetaSource interface {
	CreateMeta(ctx context.Context, projects []string) ([]origin.CreateMetaProject, error)
}

// MetaForWithCatalog is MetaFor, then the site catalog when the first
// answer had no keys to list (Jira's createmeta filtered to a missing key
// is empty). The catalog is fetched only on that fallback path, scoped to
// the profile's projects. CLI and REST both route here so they print the
// same available list.
func MetaForWithCatalog(ctx context.Context, c CreateMetaSource, meta []origin.CreateMetaProject, project string, cfg *config.Config) (origin.CreateMetaProject, []origin.CreateMetaIssueType, error) {
	p, types, err := MetaFor(meta, project, cfg)
	if err == nil || FormatProjectKeys(meta) != "" {
		return p, types, err
	}
	var scope []string
	if cfg != nil {
		scope = cfg.Projects
	}
	catalog, cerr := c.CreateMeta(ctx, scope)
	if cerr != nil || len(catalog) == 0 {
		return p, types, err
	}
	return MetaFor(catalog, project, cfg)
}

// ProjectKeys is the createable project-key list from a createmeta payload.
func ProjectKeys(meta []origin.CreateMetaProject) []string {
	out := make([]string, 0, len(meta))
	seen := map[string]bool{}
	for _, p := range meta {
		k := strings.TrimSpace(p.Key)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// FormatProjectKeys is the "KEY, KEY" list used in project-resolution errors.
func FormatProjectKeys(meta []origin.CreateMetaProject) string {
	return strings.Join(ProjectKeys(meta), ", ")
}

func availableProjectsSuffix(meta []origin.CreateMetaProject) string {
	keys := FormatProjectKeys(meta)
	if keys == "" {
		return ""
	}
	return " — available: " + keys
}

// FillNeedProject copies origin-known keys onto an empty NeedProjectError
// so a paired workspace (no local DefaultProject / Projects) can still
// print `configured: STD, IDEA`. Catalog empty leaves the error unchanged.
func FillNeedProject(err error, catalog []origin.CreateMetaProject) error {
	var np *NeedProjectError
	if !errors.As(err, &np) || np == nil || len(np.Configured) > 0 {
		return err
	}
	keys := ProjectKeys(catalog)
	if len(keys) == 0 {
		return err
	}
	return &NeedProjectError{Configured: copyStrings(keys)}
}

// FormatTypes is the "Name (id N); …" list used in type-resolution errors.
func FormatTypes(types []origin.NamedID) string {
	parts := make([]string, 0, len(types))
	for _, t := range types {
		parts = append(parts, fmt.Sprintf("%s (id %s)", t.Name, t.ID))
	}
	return strings.Join(parts, "; ")
}

// Priority resolves a user-supplied name or id against the site catalog.
// Names follow the account language; the return is always the catalog id.
func Priority(want string, list []origin.NamedID) (id string, err error) {
	want = strings.TrimSpace(want)
	if want == "" {
		return "", &NeedPriorityError{Available: copyNamed(list)}
	}
	for _, p := range list {
		if p.ID == want || strings.EqualFold(p.Name, want) {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("no priority matching %q — available: %s", want, FormatTypes(list))
}

// PriorityField is the Jira fields.priority value: always an id, never a
// localized name. Create and edit both send this shape.
func PriorityField(id string) map[string]string {
	return map[string]string{"id": id}
}

// NeedProjectError is returned when create cannot resolve a project.
// Configured is the profile project list (may be empty). Surfaces format this.
type NeedProjectError struct {
	Configured []string
}

func (e *NeedProjectError) Error() string {
	if e == nil || len(e.Configured) == 0 {
		return "project required"
	}
	return "project required, configured: " + strings.Join(e.Configured, ", ")
}

// NeedTypeError is returned when create cannot resolve an issue type.
// Available is the createmeta catalog. Surfaces format this.
type NeedTypeError struct {
	Available []origin.NamedID
}

func (e *NeedTypeError) Error() string {
	if e == nil {
		return "issue type required"
	}
	return "issue type required, available: " + FormatTypes(e.Available)
}

// NeedPriorityError is returned when a priority name or id was required but empty.
// Available is the site catalog. Surfaces format this.
type NeedPriorityError struct {
	Available []origin.NamedID
}

func (e *NeedPriorityError) Error() string {
	if e == nil {
		return "priority required"
	}
	return "priority required, available: " + FormatTypes(e.Available)
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyNamed(in []origin.NamedID) []origin.NamedID {
	if in == nil {
		return nil
	}
	out := make([]origin.NamedID, len(in))
	copy(out, in)
	return out
}
