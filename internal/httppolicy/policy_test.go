package httppolicy

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestIsRetryable(t *testing.T) {
	// Matches atlhttp TestDoRawRetryPolicyByStatus's read column.
	want := map[int]bool{
		200: false, 201: false, 400: false, 401: false, 403: false, 404: false,
		422: false, 429: true, 500: true, 501: false, 502: true, 503: true,
		504: true, 505: false,
	}
	for code, retry := range want {
		if got := IsRetryable(code); got != retry {
			t.Errorf("IsRetryable(%d) = %v, want %v", code, got, retry)
		}
	}
}

func TestIsRetryableWrite(t *testing.T) {
	// Matches atlhttp TestDoRawRetryPolicyByStatus's mutating column.
	want := map[int]bool{
		200: false, 429: true, 500: false, 502: false, 503: true, 504: false,
	}
	for code, retry := range want {
		if got := IsRetryableWrite(code); got != retry {
			t.Errorf("IsRetryableWrite(%d) = %v, want %v", code, got, retry)
		}
	}
}

func TestMaxBodyIs64MiB(t *testing.T) {
	if MaxBody != 64<<20 {
		t.Errorf("MaxBody = %d, want 64<<20", MaxBody)
	}
}

func TestSnippet(t *testing.T) {
	if got := Snippet([]byte("  hi\n")); got != "hi" {
		t.Errorf("trim = %q", got)
	}
	s := strings.Repeat("a", 400)
	if got := Snippet([]byte(s)); got != s {
		t.Errorf("400-char body must not grow an ellipsis")
	}
	if got := Snippet([]byte(s + "b")); got != s+"…" {
		t.Errorf("truncate = len %d", len(got))
	}
}

func TestWaitRetryAfterBeatsBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := Wait(ctx, 0, 0, "1", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Wait returned nil, want DeadlineExceeded — Retry-After:1 must beat Backoff=0")
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("elapsed %s: Wait returned too fast to have used Retry-After:1", elapsed)
	}
}

func TestWaitRetryAfterZeroUsesBackoff(t *testing.T) {
	start := time.Now()
	if err := Wait(context.Background(), time.Millisecond, 0, "0", nil); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("Retry-After 0 must not sleep a whole second (elapsed %s)", elapsed)
	}
}

func TestWaitRecordsMeter(t *testing.T) {
	var m Meter
	if err := Wait(context.Background(), 2*time.Millisecond, 0, "", &m); err != nil {
		t.Fatal(err)
	}
	if got := m.Snapshot().WaitMS; got <= 0 {
		t.Errorf("WaitMS = %d, want > 0", got)
	}
}

func TestWaitZeroDelayReturnsCtxErr(t *testing.T) {
	// d<=0 (Backoff 0, Retry-After empty or "0") returns ctx.Err() without sleeping.
	if err := Wait(context.Background(), 0, 0, "", nil); err != nil {
		t.Errorf("background + zero delay: %v", err)
	}
	if err := Wait(context.Background(), 0, 0, "0", nil); err != nil {
		t.Errorf("Retry-After 0: %v", err)
	}
}
