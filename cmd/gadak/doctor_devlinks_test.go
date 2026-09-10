package main

import (
	"strings"
	"testing"

	"github.com/midagedev/gadak/internal/config"
)

// GDK-1496: zero rows in dev_links meant two different things and doctor
// printed neither. The line must name which one, so the reader is never left
// deciding between "no pull requests" and "this workspace never asked".
//
// FAIL-first: formatDoctorDevLinks did not exist before this change — the
// package did not compile, which is this test's red.
func TestFormatDoctorDevLinks(t *testing.T) {
	// The field case: a connected Cloud mirror with 11,792 issues and an
	// empty table. "0 rows" alone was the whole answer before.
	cloudOff := formatDoctorDevLinks(doctorDevLinks{Origin: config.OriginJira})
	if !strings.Contains(cloudOff, "not synced") {
		t.Fatalf("cloud default line %q must say the table is not synced", cloudOff)
	}
	if !strings.Contains(cloudOff, "devStatus") {
		t.Fatalf("cloud default line %q must name the setting that turns it on", cloudOff)
	}

	// Linear has no development panel; pointing at devStatus there would be
	// advice that cannot work.
	linear := formatDoctorDevLinks(doctorDevLinks{Origin: config.OriginLinear})
	if !strings.Contains(linear, "not synced") || strings.Contains(linear, "devStatus") {
		t.Fatalf("linear line %q must say not synced without offering the flag", linear)
	}

	// On, and genuinely empty: that is a real zero and must not read as a
	// misconfiguration.
	onEmpty := formatDoctorDevLinks(doctorDevLinks{Mirrored: true, Origin: config.OriginGadak})
	if strings.Contains(onEmpty, "not synced") {
		t.Fatalf("a mirrored workspace with no PRs must not read as unsynced: %q", onEmpty)
	}
	if !strings.Contains(onEmpty, "0 rows") {
		t.Fatalf("mirrored line %q must carry the counts", onEmpty)
	}

	on := formatDoctorDevLinks(doctorDevLinks{Mirrored: true, Rows: 12, Issues: 5, Origin: config.OriginGadak})
	if !strings.Contains(on, "12 rows") || !strings.Contains(on, "5 issues") {
		t.Fatalf("mirrored line %q must carry both counts", on)
	}

	// Rows left behind by a workspace that has since turned the fetch off
	// are stale, not current — the line says so rather than counting them
	// as a healthy sync.
	stale := formatDoctorDevLinks(doctorDevLinks{Rows: 3, Issues: 2, Origin: config.OriginJira})
	if !strings.Contains(stale, "stale") {
		t.Fatalf("unsynced line with rows %q must call them stale", stale)
	}
}
