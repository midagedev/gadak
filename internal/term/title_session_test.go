//go:build !windows

package term

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The subtitle end to end (GDK-1389): a shell setting its window title
// reaches the roster row, through the same emit that decides the bell —
// and does not ask for a person on the way.
func TestSessionCarriesWindowTitle(t *testing.T) {
	m := testManager(t, Config{})
	s := shellSession(t, m, Options{})
	a, err := s.Attach()
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer a.Detach()

	// Split literal so the marker cannot be satisfied by the echo of the
	// command line itself — the attention_test.go precedent.
	if _, err := s.Write([]byte("printf '\\033]0;running the build\\007%s\\n' ti''tled\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	readUntil(t, a, "titled", 5*time.Second)

	deadline := time.Now().Add(5 * time.Second)
	var info Info
	for {
		info = s.Info()
		if info.Title == "running the build" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Info.Title = %q, want %q", info.Title, "running the build")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if info.TitleAt.IsZero() {
		t.Fatal("TitleAt is zero for a title that arrived")
	}
	if info.NeedsAttention {
		t.Fatal("a window title raised NeedsAttention")
	}
	if info.Name != "" {
		t.Fatalf("the title became the session's name: %q", info.Name)
	}

	// It rides the list row the client actually reads, under the wire
	// names the client reads it by.
	var row Info
	for _, r := range m.Snapshot() {
		if r.ID == s.ID() {
			row = r
		}
	}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"title":"running the build"`) {
		t.Fatalf("Snapshot row lost the title: %s", raw)
	}
	if !strings.Contains(string(raw), `"title_at":`) {
		t.Fatalf("Snapshot row lost title_at: %s", raw)
	}
}

// A session that never set a title must not grow two fields for a state it
// is not in — the omitempty rule its neighbours keep (attention_test.go ③).
func TestTitleOmittedWhenUnset(t *testing.T) {
	raw, err := json.Marshal(Info{ID: "x"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), "title") {
		t.Fatalf("title is not omitted when unset: %s", raw)
	}
}
