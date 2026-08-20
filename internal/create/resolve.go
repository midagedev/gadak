// Package create is the single owner of project, issue-type, and priority
// resolution for gadak create (CLI and REST). Both surfaces must call these
// functions so a default cannot exist on one path and not the other.
//
// Need* errors carry catalogue data only. The CLI formats flag names;
// REST maps them to stable wire codes. This package does not name CLI flags.
package create

import (
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/config"
	"github.com/midagedev/gadak/internal/jira"
)

// Source names recorded on --json / the REST create response so a caller
// can see why a project or type was chosen.
const (
	SourceFlag   = "flag"
	SourceConfig = "config"
	SourceSole   = "sole"
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
//  1. explicit flag / request field (name or id, same as today's CLI)
//  2. profile DefaultIssueTypeID, matched by id only
//  3. the project has exactly one createmeta type
//  4. otherwise fail
//
// types[0] is deliberately not a fallback. The web dialog may pick
// types[0] because the person can see and change it before submit. A
// headless CLI or REST caller never sees that value, so the same
// fallback would silently file as the wrong type.
func Type(want string, types []jira.NamedID, cfg *config.Config, project string) (Resolved, error) {
	if w := strings.TrimSpace(want); w != "" {
		id, err := matchType(w, types)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Value: id, Source: SourceFlag}, nil
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
			return Resolved{}, fmt.Errorf("configured default issue type id %s is not available in %s — available: %s", id, where, FormatTypes(types))
		}
	}
	if len(types) == 1 && strings.TrimSpace(types[0].ID) != "" {
		return Resolved{Value: types[0].ID, Source: SourceSole}, nil
	}
	return Resolved{}, &NeedTypeError{Available: copyNamed(types)}
}

func matchType(want string, types []jira.NamedID) (string, error) {
	for _, t := range types {
		if t.ID == want || strings.EqualFold(t.Name, want) {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("no issue type matching %q — available: %s", want, FormatTypes(types))
}

// MetaFor picks the createmeta project (case-insensitive key) and its types.
func MetaFor(meta []jira.CreateMetaProject, project string, cfg *config.Config) (jira.CreateMetaProject, []jira.NamedID, error) {
	for _, p := range meta {
		if strings.EqualFold(p.Key, project) {
			return p, p.IssueTypes, nil
		}
	}
	if cfg != nil && cfg.IsStandalone() {
		return jira.CreateMetaProject{}, nil, fmt.Errorf("project %s does not exist in this workspace", project)
	}
	return jira.CreateMetaProject{}, nil, fmt.Errorf("this credential cannot create issues in %s", project)
}

// FormatTypes is the "Name (id N); …" list used in type-resolution errors.
func FormatTypes(types []jira.NamedID) string {
	parts := make([]string, 0, len(types))
	for _, t := range types {
		parts = append(parts, fmt.Sprintf("%s (id %s)", t.Name, t.ID))
	}
	return strings.Join(parts, "; ")
}

// Priority resolves a user-supplied name or id against the site catalog.
// Names follow the account language; the return is always the catalog id.
func Priority(want string, list []jira.NamedID) (id string, err error) {
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
	Available []jira.NamedID
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
	Available []jira.NamedID
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

func copyNamed(in []jira.NamedID) []jira.NamedID {
	if in == nil {
		return nil
	}
	out := make([]jira.NamedID, len(in))
	copy(out, in)
	return out
}
