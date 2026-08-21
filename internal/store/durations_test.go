package store

import (
	"testing"
	"time"

	"github.com/midagedev/gadak/internal/config"
)

func statusTo(at, toID string) DetailChange {
	return DetailChange{At: at, Field: "status", ToID: toID}
}

func lifecycleCats() map[string]string {
	return map[string]string{"10": "new", "20": "inprogress", "30": "done"}
}

var (
	t0 = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	t1 = t0.Add(48 * time.Hour)
	t2 = t1.Add(5 * time.Hour)
)

func iso(t time.Time) string { return t.Format(config.ISOMilli) }

func TestDurationsFullLifecycle(t *testing.T) {
	in := DurationsInput{
		Created: iso(t0),
		Changelog: []DetailChange{
			statusTo(iso(t1), "20"),
			statusTo(iso(t2), "30"),
		},
		Categories: lifecycleCats(),
		Now:        t2.Add(time.Hour),
	}
	got := Durations(in)
	if got.Wait == nil || *got.Wait != 48*time.Hour {
		t.Fatalf("wait: %v", got.Wait)
	}
	if got.Progress == nil || *got.Progress != 5*time.Hour {
		t.Fatalf("progress: %v — done ends the run, not Now", got.Progress)
	}
	if line := got.Line(); line != "wait 2d · progress 5h" {
		t.Fatalf("line: %q", line)
	}
}

func TestDurationsStillInProgress(t *testing.T) {
	now := t1.Add(90 * time.Minute)
	got := Durations(DurationsInput{
		Created:    iso(t0),
		Changelog:  []DetailChange{statusTo(iso(t1), "20")},
		Categories: lifecycleCats(),
		Now:        now,
	})
	if got.Wait == nil || *got.Wait != 48*time.Hour {
		t.Fatalf("wait: %v", got.Wait)
	}
	if got.Progress == nil || *got.Progress != 90*time.Minute {
		t.Fatalf("progress: %v — in progress means until Now", got.Progress)
	}
}

// Never entered progress: neither span exists, and the caller omits the
// line entirely — kv skips empties.
func TestDurationsNeverInProgress(t *testing.T) {
	got := Durations(DurationsInput{
		Created:    iso(t0),
		Changelog:  []DetailChange{statusTo(iso(t1), "30")},
		Categories: lifecycleCats(),
		Now:        t2,
	})
	if got.Wait != nil || got.Progress != nil {
		t.Fatalf("spans: %+v", got)
	}
	if line := got.Line(); line != "" {
		t.Fatalf("line: %q", line)
	}
}

// A reopened issue restarts the progress clock at its latest entry into
// in-progress; the wait stays first-entry — that is when work first began.
func TestDurationsReopenRestartsProgress(t *testing.T) {
	again := t2.Add(2 * time.Hour)
	now := again.Add(30 * time.Minute)
	got := Durations(DurationsInput{
		Created: iso(t0),
		Changelog: []DetailChange{
			statusTo(iso(t1), "20"),
			statusTo(iso(t2), "30"),
			statusTo(iso(again), "20"),
		},
		Categories: lifecycleCats(),
		Now:        now,
	})
	if got.Wait == nil || *got.Wait != 48*time.Hour {
		t.Fatalf("wait: %v", got.Wait)
	}
	if got.Progress == nil || *got.Progress != 30*time.Minute {
		t.Fatalf("progress: %v — latest in-progress entry, not first", got.Progress)
	}
}

// Reopened and finished again: the run ends at the done after the latest
// in-progress entry, not the one before it.
func TestDurationsReopenThenDone(t *testing.T) {
	again := t2.Add(2 * time.Hour)
	done2 := again.Add(time.Hour)
	got := Durations(DurationsInput{
		Created: iso(t0),
		Changelog: []DetailChange{
			statusTo(iso(t1), "20"),
			statusTo(iso(t2), "30"),
			statusTo(iso(again), "20"),
			statusTo(iso(done2), "30"),
		},
		Categories: lifecycleCats(),
		Now:        done2.Add(24 * time.Hour),
	})
	if got.Progress == nil || *got.Progress != time.Hour {
		t.Fatalf("progress: %v", got.Progress)
	}
}

// A status id the category map cannot resolve counts as not-in-progress —
// the same rule Derive applies so an unknown id can never invent a span.
func TestDurationsUnknownCategoryNeverInvents(t *testing.T) {
	got := Durations(DurationsInput{
		Created:    iso(t0),
		Changelog:  []DetailChange{statusTo(iso(t1), "99")},
		Categories: lifecycleCats(),
		Now:        t2,
	})
	if got.Wait != nil || got.Progress != nil {
		t.Fatalf("spans: %+v", got)
	}
}

// The walk tolerates the real changelog's noise: non-status fields, status
// rows without a stamp, and an unsorted history array.
func TestDurationsIgnoresNoiseAndSorts(t *testing.T) {
	now := t1.Add(time.Hour)
	got := Durations(DurationsInput{
		Created: iso(t0),
		Changelog: []DetailChange{
			{At: iso(now), Field: "assignee", ToValue: "Me"},
			statusTo("", "20"),
			statusTo(iso(t1), "20"),
		},
		Categories: lifecycleCats(),
		Now:        now,
	})
	if got.Wait == nil || *got.Wait != 48*time.Hour {
		t.Fatalf("wait: %v", got.Wait)
	}
	if got.Progress == nil || *got.Progress != time.Hour {
		t.Fatalf("progress: %v", got.Progress)
	}
}

// Stamps that predate ISOMilli normalization (RFC3339 with an offset)
// still parse; a wait that lands negative from stamp jitter clamps to 0.
func TestDurationsLegacyStampsAndClamp(t *testing.T) {
	created := t0.Add(time.Second).Format(time.RFC3339)
	got := Durations(DurationsInput{
		Created:    created,
		Changelog:  []DetailChange{statusTo(iso(t0), "20")},
		Categories: lifecycleCats(),
		Now:        t1,
	})
	if got.Wait == nil || *got.Wait != 0 {
		t.Fatalf("wait: %v — negative jitter clamps to zero", got.Wait)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d   time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{3 * time.Minute, "3m"},
		{90 * time.Minute, "1h"},
		{5 * time.Hour, "5h"},
		{25 * time.Hour, "1d"},
		{72 * time.Hour, "3d"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.d); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
