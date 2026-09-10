package confluence

import (
	"github.com/midagedev/gadak/internal/atlhttp"
)

// Usage is a point-in-time snapshot of this client's outbound Confluence traffic.
// Counters are process-local until a caller persists them (see store.api_usage).
//
// Requests counts every HTTP attempt, including retries: that is the unit that
// draws from Confluence's rate budget.
//
// Usage, TakeUsage and TakeRequestBreakdown promote from the embedded
// atlhttp.UsageBox (GDK-1780): one owner for the meter reads both
// Atlassian-family clients share.
type Usage = atlhttp.Usage
