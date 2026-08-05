package fields

import (
	"encoding/json"

	"github.com/midagedev/scry/internal/config"
)

// SpecIDs is the flat shape store and sync use to coalesce without depending on
// the full FieldSpec when only alias+ids are needed.
type SpecIDs struct {
	Alias string
	IDs   []string
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
		out = append(out, SpecIDs{Alias: s.Alias, IDs: ids})
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
			var v any
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
			out[s.Alias] = v
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CoalesceSpecs is Coalesce over config.FieldSpec values.
func CoalesceSpecs(specs []config.FieldSpec, extra map[string]json.RawMessage) map[string]any {
	return Coalesce(SpecIDsFrom(specs), extra)
}
