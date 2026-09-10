package fields

// AllASCIIDigits reports whether s is one or more ASCII digits — the "the
// user typed a bare id, not a name" discriminator in catalog resolution
// (transition resolutions, edit value tokens, wiki link targets). It lived
// in internal/transition until GDK-688: origin and edit both need it without
// the write vocabulary, so it belongs with the other field-token helpers.
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
