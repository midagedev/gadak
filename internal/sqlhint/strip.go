package sqlhint

import (
	"strings"
	"unicode"
)

// StripComments removes -- line comments and /* */ block comments outside of
// string literals and double-quoted identifiers. A block comment is replaced
// with a space so "SELECT/*x*/1" stays two tokens.
func StripComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\'' {
			b.WriteByte('\'')
			i++
			for i < len(s) {
				if s[i] == '\'' {
					b.WriteByte('\'')
					i++
					if i < len(s) && s[i] == '\'' {
						b.WriteByte('\'')
						i++
						continue
					}
					break
				}
				b.WriteByte(s[i])
				i++
			}
			continue
		}
		if s[i] == '"' {
			b.WriteByte('"')
			i++
			for i < len(s) {
				c := s[i]
				b.WriteByte(c)
				i++
				if c == '"' {
					if i < len(s) && s[i] == '"' {
						b.WriteByte('"')
						i++
						continue
					}
					break
				}
			}
			continue
		}
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < len(s) {
				i += 2
			}
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// FirstKeyword returns the leading ASCII identifier of s after skipping
// leading space. Strip comments first (StripComments); this does not.
func FirstKeyword(s string) string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s == "" {
		return ""
	}
	end := 0
	for end < len(s) {
		r := s[end]
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_' {
			end++
			continue
		}
		break
	}
	return s[:end]
}
