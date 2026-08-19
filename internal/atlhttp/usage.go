package atlhttp

import "github.com/midagedev/gadak/internal/httppolicy"

// Usage is the host-neutral HTTP usage snapshot. The struct lives in
// httppolicy so Linear reports the same type without importing this
// package (path joining / Authorization host pinning stay here).
//
// Callers (jira.Usage, confluence.Usage, sync.usageTaker, store.api_usage)
// keep using this name.
type Usage = httppolicy.Usage

// Meter holds atomic counters shared by concurrent DoRaw goroutines.
type Meter = httppolicy.Meter
