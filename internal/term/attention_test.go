//go:build !windows

package term

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

/*
 * The "a human is wanted here" bit (GDK-1163).
 *
 * The strip needs to tell "working" from "blocked", and the signal that
 * already exists for it is the BEL byte: a shell, an agent, or a TUI rings
 * the terminal when it wants a person. idleForReap cannot answer this — it
 * answers "may I reap you", and an agent sitting at a prompt with a child
 * process on the tty is "busy" to that question and "waiting for you" to
 * this one.
 *
 * pump/emit is the single reader of the PTY, so the ring write is the one
 * place the bit can be set without a second opinion; attaching is what
 * lowers it, because a person looking at the session is the answer.
 */

// waitAttention polls the session's own Info for the bit — the state the
// assertion is about, not a proxy for it.
func waitAttention(t *testing.T, s *Session, want bool, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if got := s.Info().NeedsAttention; got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("NeedsAttention = %v, want %v after %s", !want, want, within)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ① A BEL through the ring raises the bit; output without one does not;
// attaching lowers it.
func TestBellRaisesAttentionAttachLowersIt(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Quiet output first. The split literal keeps the marker out of the
	// echoed command line, so the wait lands on the shell's output.
	if _, err := s.Write([]byte("printf 'qu''iet\\n'\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	readUntil(t, a, "quiet", 5*time.Second)
	if s.Info().NeedsAttention {
		t.Fatal("output without a BEL raised NeedsAttention")
	}

	if _, err := s.Write([]byte("printf '\\a'\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitAttention(t, s, true, 5*time.Second)

	// A person arriving is the answer to the question the bell asked.
	a.Detach()
	a2, err := s.Attach()
	if err != nil {
		t.Fatalf("Attach again: %v", err)
	}
	defer a2.Detach()
	if s.Info().NeedsAttention {
		t.Fatal("attaching did not lower NeedsAttention")
	}
}

// ② The bit rides the list row next to the issue binding, under the name
// the client reads. Both are what one strip row is made of.
func TestSnapshotCarriesAttentionWithIssueKey(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	s.SetIssueKey("GDK-1163")
	if _, err := s.Write([]byte("printf '\\a'\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	waitAttention(t, s, true, 5*time.Second)

	var row Info
	found := false
	for _, info := range m.Snapshot() {
		if info.ID == s.ID() {
			row, found = info, true
		}
	}
	if !found {
		t.Fatalf("Snapshot has no row for %s", s.ID())
	}
	if row.IssueKey != "GDK-1163" {
		t.Fatalf("IssueKey = %q, want GDK-1163", row.IssueKey)
	}
	if !row.NeedsAttention {
		t.Fatal("Snapshot row lost NeedsAttention")
	}

	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"needs_attention":true`) {
		t.Fatalf("wire name changed: %s", raw)
	}
}

// ③ omitempty, like its neighbours: a quiet session's row must not grow a
// field for every state it is not in.
func TestAttentionOmittedWhenUnset(t *testing.T) {
	raw, err := json.Marshal(Info{ID: "x"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), "needs_attention") {
		t.Fatalf("needs_attention is not omitempty: %s", raw)
	}
}
