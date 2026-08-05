package fields

import (
	"encoding/json"
	"strings"
)

// IsFilled reports whether a Jira field value counts as "set" on an issue.
// null, "", [], and {} are empty; 0 and false are filled (user-supplied values).
func IsFilled(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// Non-JSON garbage that is still present — treat as filled.
		return true
	}
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case bool:
		return true
	case float64:
		return true
	default:
		return true
	}
}

// IsFilledAny reports whether a decoded JSON value counts as set (same rules as
// IsFilled, for values already unmarshaled into issues.custom).
func IsFilledAny(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case bool:
		return true
	case float64:
		return true
	case json.Number:
		return true
	default:
		return true
	}
}
