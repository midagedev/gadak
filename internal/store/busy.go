package store

import "strings"

// BusyHolderHint is the user-facing clause appended to SQLITE_BUSY errors.
// One owner (GDK-754 / GDK-740): sync's death path and the mirror-stale
// re-read warning both go through WithBusyHint, so the sentence cannot
// drift. It does not scan other processes — doctor lists holders separately.
const BusyHolderHint = "another gadak process (app/serve/CLI) holds this profile's mirror; close it or retry"

// IsBusy reports whether err is SQLITE_BUSY (5) or SQLITE_BUSY_SNAPSHOT (517).
// Match on the driver's Code(), never on prose (store.go sqliteBusy).
func IsBusy(err error) bool {
	return sqliteBusy(err)
}

// WithBusyHint wraps a SQLITE_BUSY error so Error() names the likely holder.
// Non-busy errors and nil pass through. Already-hinted values are unchanged.
func WithBusyHint(err error) error {
	if err == nil || !sqliteBusy(err) {
		return err
	}
	if strings.Contains(err.Error(), BusyHolderHint) {
		return err
	}
	return &busyHintError{err: err}
}

type busyHintError struct {
	err error
}

func (e *busyHintError) Error() string {
	if e == nil || e.err == nil {
		return BusyHolderHint
	}
	return e.err.Error() + "; " + BusyHolderHint
}

func (e *busyHintError) Unwrap() error { return e.err }
