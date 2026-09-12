package retro

// The named cells of a retro report — the list `gadak retro --open` accepts
// and the key set behind each one.
//
// This lives here rather than in cmd/gadak because it is now read by two
// surfaces: the CLI's --open, and the MCP gadak_retro tool, which has no
// window to open and answers the keys instead. A second copy in internal/mcp
// would be exactly the drift internal/mcp/tools.go descriptions have no gate
// against — the tool's metric enum is GENERATED from OpenMetrics below, so a
// description cannot teach a metric this package does not answer.

import (
	"slices"
	"sort"
	"strings"
)

// OpenMetrics are the --open values, in help order. Each names a cell of the
// table by its row.
//
// The last five name the material lists under the table rather than a cell of
// it (materials.go). `aging` is the one that is not per bucket — it is
// measured at now — so a week does not apply to it and saying so is better
// than quietly ignoring it.
var OpenMetrics = []string{"closed", "in-progress", "mismatch", "cycle",
	"aging", "unplanned", "surprises", "seen-not-moved", "moved-not-seen"}

// ReportMetrics are the OpenMetrics answered by the report rather than by one
// bucket.
var ReportMetrics = []string{"aging"}

// IsOpenMetric reports whether m names a cell this package can answer.
func IsOpenMetric(m string) bool { return slices.Contains(OpenMetrics, m) }

// JoinOpenMetrics is the value list an "unknown metric" message prints.
func JoinOpenMetrics() string { return strings.Join(OpenMetrics, ", ") }

// KeysFor is the key set behind one cell of one bucket.
func KeysFor(b Bucket, metric string) []string {
	switch metric {
	case "closed":
		return b.ClosedKeys
	case "in-progress":
		return b.InProgressKeys
	case "mismatch":
		return b.MismatchKeys
	case "cycle":
		return b.CycleKeys
	case "unplanned":
		return b.Unplanned.Keys
	case "surprises":
		keys := make([]string, 0, len(b.Surprises))
		seen := map[string]bool{}
		for _, s := range b.Surprises {
			if !seen[s.Key] {
				seen[s.Key] = true
				keys = append(keys, s.Key)
			}
		}
		sort.Strings(keys)
		return keys
	case "seen-not-moved":
		return b.SeenNotMoved.Keys
	case "moved-not-seen":
		return b.MovedNotSeen.Keys
	}
	return nil
}

// ReportKeysFor is the key set behind a report-level metric: aging is measured
// at now, not inside a bucket, so it has no week. The bool is false for every
// metric that is per bucket.
func ReportKeysFor(rep Report, metric string) ([]string, bool) {
	if !slices.Contains(ReportMetrics, metric) {
		return nil, false
	}
	keys := make([]string, 0, len(rep.Aging.Items))
	for _, it := range rep.Aging.Items {
		keys = append(keys, it.Key)
	}
	sort.Strings(keys)
	return keys, true
}
