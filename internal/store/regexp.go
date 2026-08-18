package store

import (
	"database/sql/driver"
	"fmt"
	"regexp"
	"sync"

	"modernc.org/sqlite"
)

// Compiled patterns, keyed by the pattern text. SQLite calls this function
// once per row, and a groupQuery runs over the whole mirror — compiling the
// same handful of patterns tens of thousands of times is the cost this
// avoids. Go's regexp is RE2, so a pattern cannot backtrack catastrophically;
// the only thing worth holding onto is the compilation.
var reCache sync.Map // string → *regexp.Regexp or error

func compileRegexp(pat string) (*regexp.Regexp, error) {
	if v, ok := reCache.Load(pat); ok {
		if re, ok := v.(*regexp.Regexp); ok {
			return re, nil
		}
		return nil, v.(error)
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		reCache.Store(pat, err)
		return nil, err
	}
	reCache.Store(pat, re)
	return re, nil
}

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
	re, err := compileRegexp(pat)
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
