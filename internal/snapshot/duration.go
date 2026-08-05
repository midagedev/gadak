package snapshot

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ParseWindow parses a human window like "90d", "12w", "720h", or any
// time.ParseDuration string. Empty returns (0, nil).
func ParseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d < 0 {
			return 0, fmt.Errorf("duration must be non-negative: %q", s)
		}
		return d, nil
	}
	// Single number + unit; d and w are not in time.ParseDuration.
	i := 0
	for i < len(s) && (unicode.IsDigit(rune(s[i])) || s[i] == '.') {
		i++
	}
	if i == 0 || i == len(s) {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("duration must be non-negative: %q", s)
	}
	unit := s[i:]
	var mult time.Duration
	switch unit {
	case "ns":
		mult = time.Nanosecond
	case "us", "µs":
		mult = time.Microsecond
	case "ms":
		mult = time.Millisecond
	case "s":
		mult = time.Second
	case "m":
		mult = time.Minute
	case "h":
		mult = time.Hour
	case "d":
		mult = 24 * time.Hour
	case "w":
		mult = 7 * 24 * time.Hour
	default:
		return 0, fmt.Errorf("invalid duration unit %q in %q (want ns|us|ms|s|m|h|d|w)", unit, s)
	}
	return time.Duration(n * float64(mult)), nil
}
