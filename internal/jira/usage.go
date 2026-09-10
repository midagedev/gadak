package jira

import (
	"github.com/midagedev/gadak/internal/atlhttp"
)

// Usage is a point-in-time snapshot of this client's outbound Jira traffic.
// Counters are process-local until a caller persists them (see store.api_usage).
//
// Requests counts every HTTP attempt, including retries: that is the unit that
// draws from Jira's rate budget. This is our own call volume, not Jira's
// remaining point pool — the site does not expose that.
//
// Usage, TakeUsage and TakeRequestBreakdown promote from the embedded
// atlhttp.UsageBox (GDK-1780): one owner for the meter reads both
// Atlassian-family clients share.
type Usage = atlhttp.Usage
