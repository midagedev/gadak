package fields

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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

// builtinEditable is the always-on system write aliases. Lowest priority:
// cfg.Fields overlays the same alias, leftover EditableFields still wins last.
var builtinEditable = map[string]EditableAlias{
	"components": {IDs: []string{"components"}, Kind: "component_array"},
}

// EditableAliases builds the write allowlist from built-in system aliases,
// then Fields (Kind set), then leftover EditableFields (legacy wins on
// alias collision). A nil config is "no config at all" and returns empty.
func EditableAliases(cfg *config.Config) map[string]EditableAlias {
	out := map[string]EditableAlias{}
	if cfg == nil {
		return out
	}
	for alias, ea := range builtinEditable {
		ids := append([]string(nil), ea.IDs...)
		out[alias] = EditableAlias{IDs: ids, Kind: ea.Kind}
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

// FieldValue shapes the client's `string | string[] | number | null` into what
// Jira wants for that kind. A null clears the field, which is what the
// editor's clear does. textarea (ADF) is not a kind here — Cloud v3 needs a
// separate document write.
func FieldValue(kind string, raw json.RawMessage) (any, error) {
	if isScalarKind(kind) {
		return scalarValue(kind, raw)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return ValueFromIDs(kind, nil), nil
	}
	if isMultiKind(kind) {
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
	if isMultiKind(kind) {
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

func isMultiKind(kind string) bool {
	return kind == "version_array" || kind == "multi_option" || kind == "option-array" || kind == "component_array"
}

func isScalarKind(kind string) bool {
	return kind == "text" || kind == "number" || kind == "date"
}

func scalarValue(kind string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	switch kind {
	case "text":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		return s, nil
	case "number":
		return numberValue(raw)
	case "date":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		if !dateOnlyLiteral(s) {
			return nil, fmt.Errorf("date %q is not a date (want YYYY-MM-DD)", s)
		}
		return s, nil
	default:
		return nil, fmt.Errorf("unsupported scalar kind %q", kind)
	}
}

func numberValue(raw json.RawMessage) (any, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if i, err := n.Int64(); err == nil {
			return i, nil
		}
		f, err := n.Float64()
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	// Locale marks (comma, space, NBSP) are rejected so "1,5" never becomes 15.
	if strings.ContainsAny(s, ", \u00a0") {
		return nil, fmt.Errorf("number %q is not locale-independent", s)
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// dateOnlyLiteral is YYYY-MM-DD with month 01–12 and day 01–31. It does not
// parse into a timestamp — date-only values stay date-only.
func dateOnlyLiteral(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i := 0; i < 10; i++ {
		if i == 4 || i == 7 {
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	month := int(s[5]-'0')*10 + int(s[6]-'0')
	day := int(s[8]-'0')*10 + int(s[9]-'0')
	return month >= 1 && month <= 12 && day >= 1 && day <= 31
}
