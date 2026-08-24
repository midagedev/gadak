package jira

import (
	"testing"
)

func TestISOTimeNormalizesToUTC(t *testing.T) {
	if got := ISOTime("2026-08-04T18:15:00.482+0900"); got != "2026-08-04T09:15:00.482Z" {
		t.Errorf("offset timestamp: %q", got)
	}
	if got := ISOTime("2026-08-04T09:15:00Z"); got != "2026-08-04T09:15:00.000Z" {
		t.Errorf("rfc3339: %q", got)
	}
	if got := ISOTime("whenever"); got != "whenever" {
		t.Errorf("unparseable should pass through: %q", got)
	}
}
