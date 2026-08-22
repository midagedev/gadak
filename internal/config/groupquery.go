package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/sqlhint"
)

// ValidateGroupQuery accepts empty (disabled) or a single SELECT/WITH.
// Writes, PRAGMA, ATTACH, and multi-statement payloads are refused here so a
// bad save fails before the derived view tries to run it.
func ValidateGroupQuery(q string) error {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	s := sqlhint.StripComments(q)
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("groupQuery is empty after comments")
	}
	body := strings.TrimRight(s, " \t\n\r;")
	if strings.Contains(body, ";") {
		return errors.New("groupQuery must be one SELECT or WITH")
	}
	kw := sqlhint.FirstKeyword(body)
	switch strings.ToUpper(kw) {
	case "SELECT", "WITH":
		return nil
	case "":
		return errors.New("groupQuery is empty")
	default:
		return fmt.Errorf("groupQuery must be SELECT or WITH (got %q)", kw)
	}
}
