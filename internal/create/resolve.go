// Package create is the single owner of project and issue-type resolution
// for gadak create (CLI and REST). Both surfaces must call these functions
// so a default cannot exist on one path and not the other.
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
		return Resolved{}, fmt.Errorf("pass --project")
	}
	return Resolved{}, fmt.Errorf("pass --project, configured: %s", strings.Join(configured, ", "))
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
	return Resolved{}, fmt.Errorf("pass --type, available: %s", FormatTypes(types))
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
func MetaFor(meta []jira.CreateMetaProject, project string) (jira.CreateMetaProject, []jira.NamedID, error) {
	for _, p := range meta {
		if strings.EqualFold(p.Key, project) {
			return p, p.IssueTypes, nil
		}
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
