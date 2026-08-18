package fields

import (
	"encoding/json"

	"github.com/midagedev/gadak/internal/config"
)

// EditableAlias maps an alias onto candidate field ids and a preferred kind.
// Specs with an empty Kind are display-only and do not enter the write path.
// Leftover EditableFields (in-memory / settings PUT that skipped LoadFor)
// still overlay and win on alias collision so those callers keep working.
type EditableAlias struct {
	IDs  []string
	Kind string // preferred kind from the spec when set; empty → use editmeta
}

// EditableAliases builds the write allowlist from Fields (Kind set) plus
// leftover EditableFields (legacy wins on alias collision).
func EditableAliases(cfg *config.Config) map[string]EditableAlias {
	out := map[string]EditableAlias{}
	if cfg == nil {
		return out
	}
	for _, s := range cfg.Fields {
		if s.Kind == "" || s.Alias == "" || len(s.IDs) == 0 {
			continue
		}
		ids := append([]string(nil), s.IDs...)
		out[s.Alias] = EditableAlias{IDs: ids, Kind: s.Kind}
	}
	for alias, id := range cfg.EditableFields {
		if alias == "" || id == "" {
			continue
		}
		// Leftover in-memory map wins: replace any Fields entry for the alias.
		out[alias] = EditableAlias{IDs: []string{id}}
	}
	return out
}

// FieldValue shapes the client's `string | string[] | null` into what Jira wants
// for that kind. A null clears the field, which is what the editor's clear does.
func FieldValue(kind string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return ValueFromIDs(kind, nil), nil
	}
	if kind == "version_array" || kind == "multi_option" {
		var ids []string
		if err := json.Unmarshal(raw, &ids); err != nil {
			return nil, err
		}
		return ValueFromIDs(kind, ids), nil
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		return nil, err
	}
	if id == "" {
		return ValueFromIDs(kind, nil), nil
	}
	return ValueFromIDs(kind, []string{id}), nil
}

// ValueFromIDs shapes selected id(s) into the Jira field payload for kind.
// An empty selection clears: multi kinds become []any{}, others become nil.
func ValueFromIDs(kind string, ids []string) any {
	if kind == "version_array" || kind == "multi_option" {
		out := make([]any, 0, len(ids))
		for _, id := range ids {
			out = append(out, map[string]string{"id": id})
		}
		return out
	}
	if len(ids) == 0 || ids[0] == "" {
		return nil
	}
	if kind == "user" {
		return map[string]string{"accountId": ids[0]}
	}
	return map[string]string{"id": ids[0]}
}
