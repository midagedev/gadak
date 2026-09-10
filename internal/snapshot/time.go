package snapshot

import (
	"time"

	"github.com/midagedev/gadak/internal/config"
)

func formatTime(t time.Time) string {
	return t.UTC().Format(config.ISOMilli)
}

// parseTime accepts the ISO forms Jira and gadak write, including numeric
// offsets. The table is config.ParseTimestamp's (GDK-1130) — this used to
// be a private eight-layout copy, one of four that drifted apart; the
// owner's three layouts cover the same set (RFC3339Nano's optional
// fraction subsumes the colon-offset spellings).
func parseTime(s string) (time.Time, bool) {
	return config.ParseTimestamp(s)
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
