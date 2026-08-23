package sync

import (
	"errors"
	"testing"
)

type busyCodeError struct {
	code int
	msg  string
}

func (e busyCodeError) Error() string { return e.msg }
func (e busyCodeError) Code() int     { return e.code }

func TestMirrorStaleUnwrapsSQLITEBUSY(t *testing.T) {
	inner := busyCodeError{code: 5, msg: "database is locked (5)"}
	err := MirrorStale(inner)
	if err == nil {
		t.Fatal("MirrorStale(inner) = nil")
	}
	if !errors.Is(err, ErrMirrorStale) {
		t.Fatalf("errors.Is(ErrMirrorStale)=false; %v", err)
	}
	if err.Error() != inner.Error() {
		t.Fatalf("Error() %q, want inner sentence %q", err.Error(), inner.Error())
	}
	var se interface {
		error
		Code() int
	}
	if !errors.As(err, &se) {
		t.Fatalf("errors.As Code() failed; %v", err)
	}
	if se.Code() != 5 {
		t.Fatalf("Code() = %d, want 5 (SQLITE_BUSY)", se.Code())
	}
}

func TestMirrorStaleNilAndIdempotent(t *testing.T) {
	if MirrorStale(nil) != nil {
		t.Fatal("MirrorStale(nil) must stay nil")
	}
	inner := errors.New("re-read failed")
	wrapped := MirrorStale(inner)
	again := MirrorStale(wrapped)
	if again != wrapped {
		t.Fatal("MirrorStale must not double-wrap")
	}
}
