package sync

import (
	"fmt"
	"strings"

	"github.com/midagedev/gadak/internal/atlhttp"
)

// requestBreakdownTaker is the optional client surface behind the per-pass
// request breakdown line. *jira.Client and *confluence.Client
// implement it; the Linear client does not and its passes simply print no
// line — the kinds are Jira/Confluence routes.
type requestBreakdownTaker interface {
	TakeRequestBreakdown() atlhttp.BreakdownSnapshot
}

// formatRequestBreakdown renders one pass's request mix:
//
//	sync: requests  search-pages=34 (61.2s)  issue-comment=812 (203.4s)  …  total=2412 (638.0s)
//
// Rows come in the classifier's canonical order with zero-count kinds
// omitted; sleep (the Confluence client's politeness pause) is appended in
// milliseconds when any slept, because no request meter sees it.
func formatRequestBreakdown(s atlhttp.BreakdownSnapshot) string {
	var b strings.Builder
	b.WriteString("sync: requests")
	for _, k := range s.Kinds {
		fmt.Fprintf(&b, "  %s=%d (%s)", k.Kind, k.Count, formatWallMS(k.WallMS))
	}
	fmt.Fprintf(&b, "  total=%d (%s)", s.Total, formatWallMS(s.WallMS))
	if s.SleepMS > 0 {
		fmt.Fprintf(&b, "  sleep=%d", s.SleepMS)
	}
	return b.String()
}

// formatWallMS renders milliseconds as seconds with one decimal — the
// breakdown line's unit ("61.2s", "638.0s").
func formatWallMS(ms int64) string {
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}

// printRequestBreakdown takes the client's per-kind tally and logs the
// request mix line for one pass. Instrumentation, like FlushAPIUsage: it
// never fails the sync, and a pass with no requests prints nothing.
func printRequestBreakdown(opts Options, bt requestBreakdownTaker) {
	s := bt.TakeRequestBreakdown()
	if s.Total == 0 {
		return
	}
	opts.logf("%s", formatRequestBreakdown(s))
}
