package store

import (
	"database/sql/driver"
	"fmt"
	"regexp"

	"modernc.org/sqlite"
)

func init() {
	// SQLite's X REGEXP Y is implemented as regexp(Y, X).
	sqlite.MustRegisterDeterministicScalarFunction("regexp", 2, sqliteRegexp)
}

func sqliteRegexp(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	pat, ok := sqliteText(args[0])
	if !ok {
		return nil, nil
	}
	text, ok := sqliteText(args[1])
	if !ok {
		return nil, nil
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, fmt.Errorf("REGEXP: %w", err)
	}
	if re.MatchString(text) {
		return int64(1), nil
	}
	return int64(0), nil
}

func sqliteText(v driver.Value) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case []byte:
		return string(t), true
	default:
		return fmt.Sprint(t), true
	}
}
