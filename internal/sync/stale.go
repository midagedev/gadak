package sync

import "errors"

// ErrMirrorStale is the cause of a write-through re-read failure: the
// origin already accepted the write; only the mirror did not refresh.
// Classify with errors.Is(err, ErrMirrorStale). Error() of a wrapped
// value is the underlying re-read sentence verbatim — the same shape as
// origin.unsupportedError (GDK-685 / GDK-740). The sentinel's own text
// never prefixes that sentence, so SQLITE_BUSY stays recoverable via
// errors.As / Unwrap.
var ErrMirrorStale = errors.New("sync: mirror stale")

// mirrorStaleError wraps a re-read failure. Error() is the inner sentence
// verbatim; Unwrap returns the inner error; Is matches ErrMirrorStale.
// origin.unsupportedError only unwraps to its sentinel because the
// sentence is a string with no further cause — here the cause is the
// re-read error (SQLITE_BUSY must stay reachable).
type mirrorStaleError struct {
	err error
}

func (e *mirrorStaleError) Error() string {
	if e == nil || e.err == nil {
		return ErrMirrorStale.Error()
	}
	return e.err.Error()
}

func (e *mirrorStaleError) Unwrap() error { return e.err }

func (e *mirrorStaleError) Is(target error) bool {
	return target == ErrMirrorStale
}

// MirrorStale wraps a re-read failure so callers classify with errors.Is.
// nil stays nil; an error already in the class is returned unchanged.
func MirrorStale(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrMirrorStale) {
		return err
	}
	return &mirrorStaleError{err: err}
}
