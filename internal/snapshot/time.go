package snapshot

import (
	"strings"
	"time"
)

// timeLayout is the millisecond-UTC form store.Now and the rest of gadak write.
const timeLayout = "2006-01-02T15:04:05.000Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

// parseTime accepts the ISO forms Jira and gadak write, including numeric offsets.
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		timeLayout,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// mapTime linearly maps t from [srcLo, srcHi] onto [dstLo, dstHi].
// When the source span is zero, returns dstLo (caller may place evenly).
func mapTime(t, srcLo, srcHi, dstLo, dstHi time.Time) time.Time {
	srcSpan := srcHi.Sub(srcLo)
	if srcSpan <= 0 {
		return dstLo
	}
	frac := float64(t.Sub(srcLo)) / float64(srcSpan)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return dstLo.Add(time.Duration(frac * float64(dstHi.Sub(dstLo))))
}

// mapTimeString remaps a timestamp string; unparsable values are left as-is.
func mapTimeString(s string, srcLo, srcHi, dstLo, dstHi time.Time) string {
	t, ok := parseTime(s)
	if !ok {
		return s
	}
	return formatTime(mapTime(t, srcLo, srcHi, dstLo, dstHi))
}
