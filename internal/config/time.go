package config

// ISOMilli is the millisecond-precision UTC ISO-8601 layout every timestamp
// gadak writes — store columns, token expiry, usage flush — and that the
// `delta` cursor contract depends on. Milliseconds are not decoration: a
// whole-second cursor would drop a row written in the same second the
// cursor was taken.
const ISOMilli = "2006-01-02T15:04:05.000Z"
