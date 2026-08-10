package fields

import (
	"encoding/json"

	"github.com/midagedev/scry/internal/config"
)

// UserAccountIDsSuffix names the companion value stored beside a user-role
// alias. The display value remains under the alias; IDs are always a []string
// so single- and multi-user fields share one filter contract.
const UserAccountIDsSuffix = "_account_ids"

// SpecIDs is the flat shape store and sync use to coalesce without depending on
// the full FieldSpec when only alias+ids+role are needed.
type SpecIDs struct {
	Alias string
	IDs   []string
	Role  string
}

// SpecIDsFrom projects config field specs onto the coalesce shape.
func SpecIDsFrom(specs []config.FieldSpec) []SpecIDs {
	if len(specs) == 0 {
		return nil
	}
	out := make([]SpecIDs, 0, len(specs))
	for _, s := range specs {
		if s.Alias == "" || len(s.IDs) == 0 {
			continue
		}
		ids := make([]string, len(s.IDs))
		copy(ids, s.IDs)
		out = append(out, SpecIDs{Alias: s.Alias, IDs: ids, Role: s.Role})
	}
	return out
}

// BodyFieldIDs returns the union of explicit body field ids and every id on a
// role=body spec, de-duplicated in first-seen order.
func BodyFieldIDs(bodyFields []string, specs []config.FieldSpec) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, id := range bodyFields {
		add(id)
	}
	for _, s := range specs {
		if s.Role != "body" {
			continue
		}
		for _, id := range s.IDs {
			add(id)
		}
	}
	return out
}

// Coalesce lands the first filled value for each spec under its alias.
// IDs are tried in order; null/""/[]/{} are skipped.
//
// Values are stored display-ready: body-role values keep their raw document
// (the detail panel renders ADF), everything else flattens to a string or a
// []string — filters, chips, badges and FTS all consume plain text, and the
// write path resolves option ids through editmeta, never through the mirror.
func Coalesce(specs []SpecIDs, extra map[string]json.RawMessage) map[string]any {
	if len(specs) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, s := range specs {
		for _, id := range s.IDs {
			raw, ok := extra[id]
			if !ok || !IsFilled(raw) {
				continue
			}
			var source any
			if json.Unmarshal(raw, &source) != nil {
				continue
			}
			v := source
			if s.Role != "body" {
				v = DisplayValue(v)
				if !IsFilledAny(v) {
					continue
				}
			}
			out[s.Alias] = v
			if s.Role == "user" {
				if ids := UserAccountIDs(source); len(ids) > 0 {
					out[s.Alias+UserAccountIDsSuffix] = ids
				}
			}
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// UserAccountIDs extracts Jira accountId values from one user object or an
// array of users. Order is preserved and duplicates are removed.
func UserAccountIDs(v any) []string {
	seen := map[string]bool{}
	var out []string
	var visit func(any)
	visit = func(value any) {
		switch t := value.(type) {
		case []any:
			for _, el := range t {
				visit(el)
			}
		case map[string]any:
			if id, ok := t["accountId"].(string); ok && id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	visit(v)
	return out
}

// DisplayValue flattens a raw Jira field value onto display text: option
// `{value}`, version/component `{name}`, user `{displayName}` become their
// string; arrays flatten element-wise; scalars pass through unchanged.
func DisplayValue(v any) any {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, el := range t {
			if s, ok := DisplayValue(el).(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]any:
		for _, k := range []string{"value", "name", "displayName"} {
			if s, ok := t[k].(string); ok && s != "" {
				return s
			}
		}
		return nil
	case float64, bool, string, nil:
		return v
	default:
		return v
	}
}

// CoalesceSpecs is Coalesce over config.FieldSpec values.
func CoalesceSpecs(specs []config.FieldSpec, extra map[string]json.RawMessage) map[string]any {
	return Coalesce(SpecIDsFrom(specs), extra)
}
