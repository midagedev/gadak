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
	if strings.TrimSpace(q) == "" {
		return nil
	}
	switch verdict, kw := sqlhint.ClassifySingleSelect(q); verdict {
	case sqlhint.SingleSelectOK:
		return nil
	case sqlhint.SingleSelectEmpty:
		return errors.New("groupQuery is empty after comments")
	case sqlhint.SingleSelectMultiStatement:
		return errors.New("groupQuery must be one SELECT or WITH")
	case sqlhint.SingleSelectOtherKeyword:
		return fmt.Errorf("groupQuery must be SELECT or WITH (got %q)", kw)
	default:
		return errors.New("groupQuery is empty")
	}
}
