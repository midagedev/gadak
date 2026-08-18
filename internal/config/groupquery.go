package config

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ValidateGroupQuery accepts empty (disabled) or a single SELECT/WITH.
// Writes, PRAGMA, ATTACH, and multi-statement payloads are refused here so a
// bad save fails before the derived view tries to run it.
func ValidateGroupQuery(q string) error {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	s := stripSQLComments(q)
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("groupQuery is empty after comments")
	}
	body := strings.TrimRight(s, " \t\n\r;")
	if strings.Contains(body, ";") {
		return errors.New("groupQuery must be one SELECT or WITH")
	}
	kw := firstSQLKeyword(body)
	switch strings.ToUpper(kw) {
	case "SELECT", "WITH":
		return nil
	case "":
		return errors.New("groupQuery is empty")
	default:
		return fmt.Errorf("groupQuery must be SELECT or WITH (got %q)", kw)
	}
}

func stripSQLComments(s string) string {
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
		if i+1 < len(s) && s[i] == '-' && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			if i+1 < len(s) {
				i += 2
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func firstSQLKeyword(s string) string {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && unicode.IsSpace(rune(s[i])) {
		i++
	}
	j := i
	for j < len(s) && (unicode.IsLetter(rune(s[j])) || s[j] == '_') {
		j++
	}
	return s[i:j]
}
