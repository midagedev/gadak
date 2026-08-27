//go:build !windows

package term

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

// The coalescing backlog of GDK-1042. The row the old design got wrong:
// a burst of many small PTY reads — the measured shape was ~230 bytes per
// read, 10k lines of `seq`, dropped at 59 KB — must be coalesced, not cut.
// FAIL-first evidence for the first test is in the round report: run
// against the per-chunk channel bound, it failed with
// "attachment dropped on a 64000-byte burst: {Kind:2 Code:0 Reason:slow_client}".

// A shell that never writes, so every byte in a backlog below is the
// test's own and comparisons are exact.
func quietSession(t *testing.T) *Session {
	t.Helper()
	m := testManager(t, Config{RingBytes: 4096})
	return shellSession(t, m, Options{Shell: "/bin/sh", Args: []string{"-c", "sleep 300"}})
}

// 1000 chunks of ~64 bytes — far past the old 256-chunk bound, ~64 KB
// total — must neither drop the attachment nor lose a byte. One Take
// returns all of it, in order.
func TestAttachmentCoalescesSmallChunksPastTheOldCountBound(t *testing.T) {
	s := quietSession(t)
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	chunk := bytes.Repeat([]byte("x"), 63)
	chunk = append(chunk, '\n')
	for i := 0; i < 1000; i++ {
		s.emit(chunk)
		want.Write(chunk)
	}
	select {
	case <-a.Done():
		t.Fatalf("attachment dropped on a %d-byte burst: %+v", want.Len(), a.End())
	default:
	}
	if end := a.End(); end != (End{}) {
		t.Fatalf("end %+v; want the zero value — never dropped", end)
	}
	got := a.Take()
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("Take returned %d bytes; want all %d, in order", len(got), want.Len())
	}
	if again := a.Take(); again != nil {
		t.Fatalf("Take after a full drain returned %d bytes; want nil", len(again))
	}
	// The counters the probe and e2e read back: coalescing did work, and
	// the backlog never approached the 4 MiB bound.
	if info := s.Info(); info.CoalescedChunks != 999 {
		t.Fatalf("CoalescedChunks %d; want 999 (every emit after the first landed on a non-empty backlog)", info.CoalescedChunks)
	}
	if info := s.Info(); info.BacklogMaxBytes != int64(want.Len()) {
		t.Fatalf("BacklogMaxBytes %d; want %d", info.BacklogMaxBytes, want.Len())
	}
}

// The bound still drops: a client past AttachBytes is ended with
// EndDropped/ReasonSlow and the session says so. Protection, keyed on the
// quantity that means "gone".
func TestAttachmentOverAttachBytesIsDropped(t *testing.T) {
	const bound = 512
	m := testManager(t, Config{AttachBytes: bound, RingBytes: 4096})
	s := shellSession(t, m, Options{Shell: "/bin/sh", Args: []string{"-c", "sleep 300"}})
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("y"), 64)
	for i := 0; i < bound/len(chunk)+3; i++ {
		s.emit(chunk)
	}
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("attachment past AttachBytes was never dropped")
	}
	if end := a.End(); end.Kind != EndDropped || end.Reason != ReasonSlow {
		t.Fatalf("end %+v; want dropped/%s", end, ReasonSlow)
	}
	if info := s.Info(); info.DroppedAttachments != 1 {
		t.Fatalf("DroppedAttachments %d; want 1", info.DroppedAttachments)
	}
}

// Wake is edge-triggered: a producer that outruns its reader signals at
// most once, and emit never blocks on the way. One Take drains everything.
func TestWakeIsEdgeTriggeredAndTakeDrainsEverything(t *testing.T) {
	s := quietSession(t)
	a, err := s.Attach()
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	for i := 0; i < 100; i++ {
		chunk := []byte(fmt.Sprintf("c%03d\n", i))
		s.emit(chunk) // no reader anywhere: a blocking push would hang here
		want.Write(chunk)
	}
	if n := len(a.Wake()); n != 1 {
		t.Fatalf("%d wakes pending after 100 emits with no reader; want exactly 1", n)
	}
	if got := a.Take(); !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("Take returned %d bytes; want all %d", len(got), want.Len())
	}
	// A backlog pending at the end stays readable: the drain-a-once
	// contract terminalFlush depends on.
	a.Detach()
	select {
	case <-a.Done():
	default:
		t.Fatal("Detach did not end the attachment")
	}
	if got := a.Take(); got != nil {
		t.Fatalf("Take after drain returned %d bytes; want nil", len(got))
	}
}
